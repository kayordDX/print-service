// Package hubclient manages the outbound SignalR connection to the POS API
// printer hub: authentication via the print service API key, receiving print
// jobs and printer assignments, reporting scan and probe results, and
// automatic reconnection with exponential backoff.
package hubclient

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/philippseith/signalr"

	"github.com/kayorddx/print-service/internal/model"
)

// negotiateTimeout bounds each connection attempt (HTTP negotiate plus
// WebSocket upgrade). The client's own backoff handles retries.
const negotiateTimeout = 15 * time.Second

// Callbacks route server-initiated invocations into the application. The
// callbacks are called on the hub receive loop; implementations must not
// block for long.
type Callbacks struct {
	// OnPrint is called for every print job or scan request from the server.
	OnPrint func(model.PrintMessage)
	// OnSyncPrinters is called whenever the server assigns this device its
	// printer set (on connect and whenever the set changes).
	OnSyncPrinters func([]model.PrinterTarget)
}

// Receiver implements the server -> device hub methods. Method names must
// match the invocations sent by Pos.Api's PrinterHub (matching is
// case-insensitive, mirroring the .NET client).
type Receiver struct {
	signalr.Receiver // embed for access to Server() inside receiver methods

	callbacks Callbacks
	logger    *slog.Logger
}

// ReceivePrint is invoked by the server with a print job or scan request.
func (r *Receiver) ReceivePrint(msg model.PrintMessage) {
	if r.callbacks.OnPrint == nil {
		return
	}
	r.logger.Debug("print job received",
		"action", msg.Action, "printerName", msg.PrinterName,
		"ip", msg.IPAddress, "port", msg.Port, "chunks", len(msg.PrintInstructions))
	r.callbacks.OnPrint(msg)
}

// SyncPrinters is invoked by the server with the printers this device is
// responsible for probing.
func (r *Receiver) SyncPrinters(targets []model.PrinterTarget) {
	if r.callbacks.OnSyncPrinters == nil {
		return
	}
	r.logger.Info("printer assignment received", "count", len(targets))
	r.callbacks.OnSyncPrinters(targets)
}

// Client wraps the SignalR client with fire-and-forget reporting helpers
// for the device -> server hub methods.
type Client struct {
	client signalr.Client
	logger *slog.Logger
}

// New builds (but does not start) the hub client. The returned client
// reconnects automatically forever; the server re-joins this connection to
// its groups on every (re)connect, so there is no resubscribe logic.
func New(ctx context.Context, baseURL, apiKey string, callbacks Callbacks, logger *slog.Logger) (*Client, error) {
	// Headers reach the negotiate POST and the WebSocket upgrade; the
	// server's PrinterKey scheme accepts "Authorization: Bearer kpos_...".
	headers := func() http.Header {
		h := http.Header{}
		h.Set("Authorization", "Bearer "+apiKey)
		return h
	}
	connect := func() (signalr.Connection, error) {
		connCtx, cancel := context.WithTimeout(ctx, negotiateTimeout)
		defer cancel()
		return signalr.NewHTTPConnection(connCtx, baseURL+"/printer-hub", signalr.WithHTTPHeaders(headers))
	}

	client, err := signalr.NewClient(ctx,
		signalr.WithConnector(connect),
		signalr.WithReceiver(&Receiver{callbacks: callbacks, logger: logger}),
		// An unattended device must never give up: an invalid key at
		// startup, API restarts or network loss all retry with backoff.
		// MaxElapsedTime=0 disables the default 15-minute give-up.
		signalr.WithBackoff(func() backoff.BackOff {
			b := backoff.NewExponentialBackOff()
			b.MaxElapsedTime = 0
			return b
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create signalr client: %w", err)
	}
	return &Client{client: client, logger: logger}, nil
}

// Start launches the connection loop and logs every connection state change.
func (c *Client) Start() {
	states := make(chan signalr.ClientState, 16)
	cancel := c.client.ObserveStateChanged(states)
	go func() {
		defer cancel()
		for state := range states {
			c.logger.Info("hub connection state changed", "state", stateName(state))
		}
	}()
	c.client.Start()
}

// Stop ends the connection and the reconnect loop.
func (c *Client) Stop() { c.client.Stop() }

// Connected reports whether the hub is currently usable.
func (c *Client) Connected() bool { return c.client.State() == signalr.ClientConnected }

// send is fire-and-forget: the result channel is drained and errors logged.
func (c *Client) send(method string, args ...any) {
	go func() {
		if err := <-c.client.Send(method, args...); err != nil {
			c.logger.Warn("report to hub failed", "method", method, "error", err)
		}
	}()
}

// ReportScanStarted tells the server a scan has started so the admin UI can
// show progress. Invokes PrinterHub.ReportScanStarted.
func (c *Client) ReportScanStarted() { c.send("ReportScanStarted") }

// ReportScanResult delivers the human-readable scan summary. Invokes
// PrinterHub.ReportScanResult.
func (c *Client) ReportScanResult(output string) { c.send("ReportScanResult", output) }

// ReportPrinterProbe delivers one reachability probe result. Invokes
// PrinterHub.ReportPrinterProbe.
func (c *Client) ReportPrinterProbe(printerID int, reachable bool, latencyMillis int64) {
	c.send("ReportPrinterProbe", printerID, reachable, latencyMillis)
}

// stateName maps client states to stable, human-readable log values.
func stateName(s signalr.ClientState) string {
	switch s {
	case signalr.ClientCreated:
		return "created"
	case signalr.ClientConnecting:
		return "connecting"
	case signalr.ClientConnected:
		return "connected"
	case signalr.ClientClosed:
		return "closed"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}
