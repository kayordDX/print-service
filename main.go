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
	"github.com/kayorddx/print-service/internal/printqueue"
	"github.com/kayorddx/print-service/internal/probe"
	"github.com/kayorddx/print-service/internal/scan"
)

// scanTimeout bounds a single network scan; a /24 finishes in seconds, so a
// timeout means something is wrong (e.g. a pathological pattern).
const scanTimeout = 5 * time.Minute

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

	app := newApp(ctx, cfg, logger)

	hub, err := hubclient.New(ctx, cfg.BaseURL, cfg.APIKey.Bearer(), hubclient.Callbacks{
		OnPrint:        app.dispatchPrint,
		OnSyncPrinters: app.probeStore.Set,
	}, logger)
	if err != nil {
		return err
	}
	app.hub = hub

	// Print jobs run on one worker per printer (a slow printer must never
	// delay the others); scans run in their own goroutines.
	go app.prober.Run(ctx)

	hub.Start()

	<-ctx.Done()
	logger.Info("shutting down")
	hub.Stop()
	return nil
}

// app glues the hub callbacks to the printer, scanner and prober.
type app struct {
	ctx        context.Context
	logger     *slog.Logger
	hub        *hubclient.Client
	probeStore *probe.Store
	prober     *probe.Prober
	prints     *printqueue.Queue
}

func newApp(ctx context.Context, cfg config.Config, logger *slog.Logger) *app {
	a := &app{
		ctx:        ctx,
		logger:     logger,
		probeStore: probe.NewStore(),
	}
	a.prints = printqueue.New(ctx, a.handlePrintJob)
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

// dispatchPrint routes one hub message: scans run in their own goroutine so
// a long scan never delays printing, everything else goes to the per
// printer queue.
func (a *app) dispatchPrint(msg model.PrintMessage) {
	if msg.Action == "nmap" {
		// Legacy action value kept for wire compatibility: the server asks
		// for a network scan through the same message type.
		go a.runScan(msg)
		return
	}
	if !a.prints.Enqueue(msg) {
		a.logger.Error("print queue full, dropping job",
			"printerName", msg.PrinterName, "ip", msg.IPAddress, "port", msg.Port,
			"jobId", msg.JobID)
	}
}

// handlePrintJob writes one job to its printer and reports the outcome, so
// the server knows whether the receipt actually printed.
func (a *app) handlePrintJob(ctx context.Context, msg model.PrintMessage) {
	if len(msg.PrintInstructions) == 0 {
		a.logger.Warn("print job without instructions",
			"printerName", msg.PrinterName, "ip", msg.IPAddress, "port", msg.Port,
			"jobId", msg.JobID)
	}
	err := printer.Print(ctx, msg)
	if err != nil {
		a.logger.Error("print failed",
			"printerName", msg.PrinterName, "ip", msg.IPAddress, "port", msg.Port,
			"jobId", msg.JobID, "error", err)
	} else {
		a.logger.Info("print delivered",
			"printerName", msg.PrinterName, "ip", msg.IPAddress, "port", msg.Port,
			"chunks", len(msg.PrintInstructions), "bytes", totalBytes(msg),
			"jobId", msg.JobID)
	}
	if msg.JobID != "" {
		detail := ""
		if err != nil {
			detail = err.Error()
		}
		a.hub.ReportPrintResult(msg.JobID, err == nil, detail)
	}
}

// runScan performs a native network scan and reports start and result to
// the server, mirroring the old nmap flow (status ping, then full output).
// It runs under the app context so a shutdown cancels an in-flight scan.
func (a *app) runScan(msg model.PrintMessage) {
	logger := a.logger.With("pattern", msg.IPAddress, "port", msg.Port)
	logger.Info("scan requested")
	a.hub.ReportScanStarted()

	ctx, cancel := context.WithTimeout(a.ctx, scanTimeout)
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
