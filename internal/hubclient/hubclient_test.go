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

// printResult captures one ReportPrintResult invocation server-side.
type printResult struct {
	jobID  string
	ok     bool
	detail string
}

// testHub mimics Pos.Api's PrinterHub just enough to exercise the wire
// contract end to end: it sends ReceivePrint/SyncPrinters/RotateKey to
// whoever asks (Ping/TriggerRotate) and records everything the device
// reports.
type testHub struct {
	signalr.Hub

	print   model.PrintMessage
	targets []model.PrinterTarget
	newKey  string

	mu           sync.Mutex
	scanStarted  int
	scanResults  []string
	probes       []probeReport
	printResults []printResult
	keyAcks      []bool
}

// Ping is invoked by the device once it is connected; the hub answers with
// the print job and printer assignment.
func (h *testHub) Ping() {
	h.Clients().Caller().Send("ReceivePrint", h.print)
	h.Clients().Caller().Send("SyncPrinters", h.targets)
}

// TriggerRotate is invoked by the device to make the hub push a new key.
func (h *testHub) TriggerRotate() {
	h.Clients().Caller().Send("RotateKey", h.newKey)
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

func (h *testHub) ReportPrintResult(jobID string, ok bool, detail string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.printResults = append(h.printResults, printResult{jobID, ok, detail})
}

func (h *testHub) ReportKeyRotated(ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.keyAcks = append(h.keyAcks, ok)
}

func (h *testHub) snapshot() (int, []string, []probeReport, []printResult, []bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.scanStarted, h.scanResults, h.probes, h.printResults, h.keyAcks
}

// startTestServer serves the hub over HTTP on a random local port.
func startTestServer(t *testing.T, hub *testHub) string {
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
	return httpServer.URL
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
// base64-decoded byte chunks, receiving the printer assignment, receiving a
// pushed key rotation, and reporting scans, probes and print results back.
func TestIntegrationRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wantMsg := model.PrintMessage{
		PrinterName:       "Front desk",
		IPAddress:         "10.0.0.3",
		Port:              9100,
		PrintInstructions: [][]byte{{0x1b, '@'}, []byte("receipt body")},
		JobID:             "job-42",
	}
	wantTargets := []model.PrinterTarget{
		{PrinterID: 7, Name: "Front desk", IPAddress: "10.0.0.3", Port: 9100},
		{PrinterID: 8, Name: "Bar", IPAddress: "10.0.0.4", Port: 9100},
	}
	wantNewKey := "kpos_pk_r0t4ted.brandnewsecret"

	hub := &testHub{print: wantMsg, targets: wantTargets, newKey: wantNewKey}
	baseURL := startTestServer(t, hub)

	prints := make(chan model.PrintMessage, 1)
	syncs := make(chan []model.PrinterTarget, 1)
	rotations := make(chan string, 1)
	client, err := New(ctx, baseURL, func() string { return "kpos_pk_8f3a91c2.supersecret" }, Callbacks{
		OnPrint:        func(m model.PrintMessage) { prints <- m },
		OnSyncPrinters: func(targets []model.PrinterTarget) { syncs <- targets },
		OnRotateKey:    func(newAPIKey string) { rotations <- newAPIKey },
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
	client.ReportPrintResult("job-42", true, "")

	waitFor(t, "ReportPrinterProbe", func() bool {
		_, _, probes, _, _ := hub.snapshot()
		return len(probes) == 1 && probes[0] == probeReport{7, true, 12}
	})
	waitFor(t, "ReportScanStarted and ReportScanResult", func() bool {
		started, results, _, _, _ := hub.snapshot()
		return started == 1 && len(results) == 1 && results[0] == "scan output"
	})
	waitFor(t, "ReportPrintResult", func() bool {
		_, _, _, printResults, _ := hub.snapshot()
		return len(printResults) == 1 && printResults[0] == printResult{"job-42", true, ""}
	})

	// Key rotation: server pushes RotateKey, the device callback sees the
	// new key and the ack reaches the server.
	if err := <-client.client.Send("TriggerRotate"); err != nil {
		t.Fatalf("TriggerRotate failed: %v", err)
	}
	select {
	case got := <-rotations:
		if got != wantNewKey {
			t.Errorf("RotateKey delivered %q, want %q", got, wantNewKey)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for RotateKey")
	}
	client.ReportKeyRotated(true)
	waitFor(t, "ReportKeyRotated", func() bool {
		_, _, _, _, acks := hub.snapshot()
		return len(acks) == 1 && acks[0]
	})
}
