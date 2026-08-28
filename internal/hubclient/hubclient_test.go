package hubclient

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/philippseith/signalr"

	"github.com/kayorddx/print-service/internal/model"
)

// probeReport captures one ReportPrinterProbe invocation server-side.
type probeReport struct {
	printerID     int
	reachable     bool
	latencyMillis int64
}

// testHub mimics Pos.Api's PrinterHub just enough to exercise the wire
// contract end to end: it sends ReceivePrint/SyncPrinters to whoever asks
// (Ping) and records everything the device reports.
type testHub struct {
	signalr.Hub

	print   model.PrintMessage
	targets []model.PrinterTarget

	mu          sync.Mutex
	scanStarted int
	scanResults []string
	probes      []probeReport
}

// Ping is invoked by the device once it is connected; the hub answers with
// the print job and printer assignment.
func (h *testHub) Ping() {
	h.Clients().Caller().Send("ReceivePrint", h.print)
	h.Clients().Caller().Send("SyncPrinters", h.targets)
}

func (h *testHub) ReportScanStarted() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.scanStarted++
}

func (h *testHub) ReportScanResult(output string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.scanResults = append(h.scanResults, output)
}

func (h *testHub) ReportPrinterProbe(printerID int, reachable bool, latencyMs int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.probes = append(h.probes, probeReport{printerID, reachable, latencyMs})
}

func (h *testHub) snapshot() (int, []string, []probeReport) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.scanStarted, h.scanResults, h.probes
}

// startTestServer serves the hub over HTTP on a random local port.
func startTestServer(t *testing.T, hub *testHub) (baseURL string, shutdown func()) {
	t.Helper()
	serverCtx, cancelServer := context.WithCancel(context.Background())
	t.Cleanup(cancelServer)

	server, err := signalr.NewServer(serverCtx, signalr.HubFactory(func() signalr.HubInterface {
		// Return the populated instance so the test can observe every
		// invocation (SimpleHubFactory would hand out zero-valued copies).
		return hub
	}))
	if err != nil {
		t.Fatalf("signalr.NewServer(): %v", err)
	}
	mux := http.NewServeMux()
	server.MapHTTP(signalr.WithHTTPServeMux(mux), "/printer-hub")
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	return httpServer.URL, cancelServer
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestIntegrationRoundTrip connects the real client to a real (in-process)
// SignalR server and verifies the full contract: receiving a print job with
// base64-decoded byte chunks, receiving the printer assignment, and
// reporting scans and probes back.
func TestIntegrationRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wantMsg := model.PrintMessage{
		PrinterName:       "Front desk",
		IPAddress:         "10.0.0.3",
		Port:              9100,
		PrintInstructions: [][]byte{{0x1b, '@'}, []byte("receipt body")},
	}
	wantTargets := []model.PrinterTarget{
		{PrinterID: 7, Name: "Front desk", IPAddress: "10.0.0.3", Port: 9100},
		{PrinterID: 8, Name: "Bar", IPAddress: "10.0.0.4", Port: 9100},
	}

	hub := &testHub{print: wantMsg, targets: wantTargets}
	baseURL, _ := startTestServer(t, hub)

	prints := make(chan model.PrintMessage, 1)
	syncs := make(chan []model.PrinterTarget, 1)
	client, err := New(ctx, baseURL, "kpos_pk_8f3a91c2.supersecret", Callbacks{
		OnPrint:        func(m model.PrintMessage) { prints <- m },
		OnSyncPrinters: func(targets []model.PrinterTarget) { syncs <- targets },
	}, slog.Default())
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	client.Start()
	defer client.Stop()

	// Wait until connected, then ask the server to push a job + sync.
	if err := <-client.client.WaitForState(ctx, signalr.ClientConnected); err != nil {
		t.Fatalf("client never connected: %v", err)
	}
	if err := <-client.client.Send("Ping"); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	select {
	case got := <-prints:
		if !reflect.DeepEqual(got, wantMsg) {
			t.Errorf("ReceivePrint delivered %+v, want %+v", got, wantMsg)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for ReceivePrint")
	}
	select {
	case got := <-syncs:
		if !reflect.DeepEqual(got, wantTargets) {
			t.Errorf("SyncPrinters delivered %+v, want %+v", got, wantTargets)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for SyncPrinters")
	}

	// Device -> server reporting.
	client.ReportPrinterProbe(7, true, 12)
	client.ReportScanStarted()
	client.ReportScanResult("scan output")

	waitFor(t, "ReportPrinterProbe", func() bool {
		_, _, probes := hub.snapshot()
		return len(probes) == 1 && probes[0] == probeReport{7, true, 12}
	})
	waitFor(t, "ReportScanStarted and ReportScanResult", func() bool {
		started, results, _ := hub.snapshot()
		return started == 1 && len(results) == 1 && results[0] == "scan output"
	})
}
