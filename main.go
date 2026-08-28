// Command print-service is the Kayord POS print service: a single static
// binary that runs on a Raspberry Pi (or any always-on box at a restaurant),
// connects outbound to the POS API printer hub over WSS, prints pre-rendered
// ESC/POS jobs to EPSON-compatible network thermal printers (raw TCP 9100),
// performs native network scans (no nmap) and reports printer reachability.
//
// Configuration comes from the environment; see internal/config for the
// variables. Outlet and device identity are bound to the API key, not to
// configuration.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kayorddx/print-service/internal/config"
	"github.com/kayorddx/print-service/internal/hubclient"
	"github.com/kayorddx/print-service/internal/model"
	"github.com/kayorddx/print-service/internal/printer"
	"github.com/kayorddx/print-service/internal/probe"
	"github.com/kayorddx/print-service/internal/scan"
)

const (
	// printQueueSize bounds queued print jobs. The queue only bridges the
	// hub receive loop and the print worker; it is not a persistent store.
	printQueueSize = 64
	// scanTimeout bounds a single network scan; a /24 finishes in seconds,
	// so a timeout means something is wrong (e.g. a pathological pattern).
	scanTimeout = 5 * time.Minute
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "print-service:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	// Only the public key id is logged — never the secret.
	logger.Info("starting print-service",
		"keyId", cfg.APIKey.KeyID, "baseURL", cfg.BaseURL, "probeInterval", cfg.ProbeInterval)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := newApp(cfg, logger)

	hub, err := hubclient.New(ctx, cfg.BaseURL, cfg.APIKey.Bearer(), hubclient.Callbacks{
		OnPrint:        app.enqueuePrint,
		OnSyncPrinters: app.probeStore.Set,
	}, logger)
	if err != nil {
		return err
	}
	app.hub = hub

	// Print jobs are serialized through a single worker (a thermal printer
	// cannot parallelize anyway); scans and probes run in their own goroutines.
	go app.runPrintWorker(ctx)
	go app.prober.Run(ctx)

	hub.Start()

	<-ctx.Done()
	logger.Info("shutting down")
	hub.Stop()
	return nil
}

// app glues the hub callbacks to the printer, scanner and prober.
type app struct {
	cfg        config.Config
	logger     *slog.Logger
	hub        *hubclient.Client
	probeStore *probe.Store
	prober     *probe.Prober
	prints     chan model.PrintMessage
}

func newApp(cfg config.Config, logger *slog.Logger) *app {
	a := &app{
		cfg:        cfg,
		logger:     logger,
		probeStore: probe.NewStore(),
		prints:     make(chan model.PrintMessage, printQueueSize),
	}
	a.prober = probe.NewProber(a.probeStore, cfg.ProbeInterval, a.reportProbe, a.hubConnected, logger)
	return a
}

// hubConnected reports whether scan results and probe reports can currently
// reach the server. A nil hub (before startup) counts as disconnected.
func (a *app) hubConnected() bool { return a.hub != nil && a.hub.Connected() }

// reportProbe forwards one probe result to the server; errors are logged by
// the hub client.
func (a *app) reportProbe(ctx context.Context, printerID int, reachable bool, latencyMillis int64) {
	a.hub.ReportPrinterProbe(printerID, reachable, latencyMillis)
}

// enqueuePrint queues a print job without ever blocking the hub receive
// loop. The queue is small and the worker fast; if it ever fills up, jobs
// are dropped loudly rather than stalling all hub traffic.
func (a *app) enqueuePrint(msg model.PrintMessage) {
	select {
	case a.prints <- msg:
	default:
		a.logger.Error("print queue full, dropping job",
			"printerName", msg.PrinterName, "ip", msg.IPAddress, "port", msg.Port)
	}
}

// runPrintWorker consumes print jobs until ctx is canceled.
func (a *app) runPrintWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-a.prints:
			a.handlePrint(ctx, msg)
		}
	}
}

// handlePrint dispatches one message: scans run in their own goroutine so a
// long scan never delays printing, everything else is printed directly.
func (a *app) handlePrint(ctx context.Context, msg model.PrintMessage) {
	if msg.Action == "nmap" {
		// Legacy action value kept for wire compatibility: the server asks
		// for a network scan through the same message type.
		go a.runScan(ctx, msg)
		return
	}
	// The server sends the port it has on file for the printer, so any
	// port works; this only covers a message that omitted it.
	if msg.Port <= 0 {
		msg.Port = 9100
	}
	if err := printer.Print(ctx, msg); err != nil {
		a.logger.Error("print failed",
			"printerName", msg.PrinterName, "ip", msg.IPAddress, "port", msg.Port, "error", err)
		return
	}
	a.logger.Info("print delivered",
		"printerName", msg.PrinterName, "ip", msg.IPAddress, "port", msg.Port,
		"chunks", len(msg.PrintInstructions), "bytes", totalBytes(msg))
}

// runScan performs a native network scan and reports start and result to
// the server, mirroring the old nmap flow (status ping, then full output).
func (a *app) runScan(ctx context.Context, msg model.PrintMessage) {
	logger := a.logger.With("pattern", msg.IPAddress, "port", msg.Port)
	logger.Info("scan requested")
	a.hub.ReportScanStarted()

	ctx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()

	output, err := scan.Run(ctx, msg.IPAddress, msg.Port)
	if err != nil {
		logger.Error("scan failed", "error", err)
		output = fmt.Sprintf("Scan of %s port %d failed: %v", msg.IPAddress, msg.Port, err)
	}
	a.hub.ReportScanResult(output)
	logger.Info("scan finished")
}

// totalBytes returns the summed size of all instruction chunks, for logs.
func totalBytes(msg model.PrintMessage) int {
	n := 0
	for _, chunk := range msg.PrintInstructions {
		n += len(chunk)
	}
	return n
}

// newLogger builds the default text logger at the configured level.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
