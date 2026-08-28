// Package probe reports printer reachability: every probe interval the
// device dials each printer the server assigned to it (via SyncPrinters)
// and reports whether the printer's TCP port accepted the connection.
package probe

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/kayorddx/print-service/internal/model"
)

// DefaultDialTimeout bounds each reachability check. Probes are frequent,
// so they must be much cheaper than an actual print.
const DefaultDialTimeout = 500 * time.Millisecond

// Store holds the printer targets assigned by the server. It is safe for
// concurrent use: the hub receive loop swaps the set while the prober reads.
type Store struct {
	mu      sync.RWMutex
	targets []model.PrinterTarget
}

// NewStore returns an empty target store.
func NewStore() *Store { return &Store{} }

// Set atomically replaces the assigned target set.
func (s *Store) Set(targets []model.PrinterTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targets = targets
}

// Get returns the current target set.
func (s *Store) Get() []model.PrinterTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.targets
}

// Prober periodically probes all assigned printers and reports each result.
// The zero values of dialTimeout are filled with defaults by NewProber.
type Prober struct {
	store       *Store
	interval    time.Duration
	dialTimeout time.Duration
	report      func(ctx context.Context, printerID int, reachable bool, latencyMillis int64)
	connected   func() bool
	logger      *slog.Logger
}

// NewProber returns a prober for store that runs every interval, reports
// results through report, and skips rounds while connected reports false
// (results would be meaningless while the hub is down anyway).
func NewProber(
	store *Store,
	interval time.Duration,
	report func(ctx context.Context, printerID int, reachable bool, latencyMillis int64),
	connected func() bool,
	logger *slog.Logger,
) *Prober {
	return &Prober{
		store:       store,
		interval:    interval,
		dialTimeout: DefaultDialTimeout,
		report:      report,
		connected:   connected,
		logger:      logger,
	}
}

// Run blocks until ctx is canceled, probing the assigned printers every
// interval. Targets whose probes the server no longer sends simply stop
// being reported once the server replaces the set.
func (p *Prober) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		p.probeAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// probeAll runs one probe round, skipping it while the hub is disconnected.
func (p *Prober) probeAll(ctx context.Context) {
	targets := p.store.Get()
	if len(targets) == 0 || p.connected == nil || !p.connected() {
		return
	}

	// Spread probes across the interval instead of bursting all dials at
	// once; a restaurant LAN with a dozen printers stays quiet.
	stagger := p.interval / time.Duration(len(targets)+1)

	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t model.PrinterTarget) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(i) * stagger):
			}
			p.probeOne(ctx, t)
		}(i, t)
	}
	wg.Wait()
}

// probeOne dials a single printer and reports the outcome.
func (p *Prober) probeOne(ctx context.Context, t model.PrinterTarget) {
	addr := net.JoinHostPort(t.IPAddress, strconv.Itoa(t.Port))

	dialer := net.Dialer{Timeout: p.dialTimeout}
	began := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	latencyMillis := time.Since(began).Milliseconds()
	if err != nil {
		p.logger.Warn("printer unreachable",
			"printerId", t.PrinterID, "name", t.Name, "addr", addr,
			"latencyMs", latencyMillis, "error", err)
		p.report(ctx, t.PrinterID, false, latencyMillis)
		return
	}
	_ = conn.Close()

	p.logger.Debug("printer reachable",
		"printerId", t.PrinterID, "name", t.Name, "addr", addr, "latencyMs", latencyMillis)
	p.report(ctx, t.PrinterID, true, latencyMillis)
}
