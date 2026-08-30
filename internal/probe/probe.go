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

// DefaultFailureThreshold is how many consecutive failed probes are needed
// before a printer is reported unreachable. One missed probe (a Wi-Fi blip,
// a momentary collision) does not flip the badge in the admin UI.
const DefaultFailureThreshold = 2

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
type Prober struct {
	store       *Store
	interval    time.Duration
	dialTimeout time.Duration
	report      func(ctx context.Context, printerID int, reachable bool, latencyMillis int64)
	connected   func() bool
	logger      *slog.Logger

	mu               sync.Mutex
	failures         map[int]int // printerID -> consecutive failures
	failureThreshold int
}

// NewProber returns a prober for store that runs every interval, reports
// results through report, and skips rounds while connected reports false
// (results would be meaningless while the hub is down anyway).
// A printer is only reported unreachable after
// DefaultFailureThreshold consecutive failed probes.
func NewProber(
	store *Store,
	interval time.Duration,
	report func(ctx context.Context, printerID int, reachable bool, latencyMillis int64),
	connected func() bool,
	logger *slog.Logger,
) *Prober {
	return &Prober{
		store:            store,
		interval:         interval,
		dialTimeout:      DefaultDialTimeout,
		report:           report,
		connected:        connected,
		logger:           logger,
		failures:         make(map[int]int),
		failureThreshold: DefaultFailureThreshold,
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

// probeOne dials a single printer. Success resets the failure counter and
// is reported immediately; failure is only reported once it persists for
// failureThreshold consecutive probes (hysteresis against transient blips).
func (p *Prober) probeOne(ctx context.Context, t model.PrinterTarget) {
	addr := net.JoinHostPort(t.IPAddress, strconv.Itoa(t.Port))

	dialer := net.Dialer{Timeout: p.dialTimeout}
	began := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	latencyMillis := time.Since(began).Milliseconds()

	p.mu.Lock()
	if err == nil {
		p.failures[t.PrinterID] = 0
	} else {
		p.failures[t.PrinterID]++
	}
	consecutive := p.failures[t.PrinterID]
	report := err == nil || consecutive >= p.failureThreshold
	p.mu.Unlock()

	if !report {
		p.logger.Debug("printer probe failed, waiting for threshold",
			"printerId", t.PrinterID, "name", t.Name, "addr", addr,
			"consecutiveFailures", consecutive)
		return
	}
	if err != nil {
		p.logger.Warn("printer unreachable",
			"printerId", t.PrinterID, "name", t.Name, "addr", addr,
			"consecutiveFailures", consecutive, "latencyMs", latencyMillis, "error", err)
	}
	if err == nil {
		_ = conn.Close()
		p.logger.Debug("printer reachable",
			"printerId", t.PrinterID, "name", t.Name, "addr", addr, "latencyMs", latencyMillis)
	}
	p.report(ctx, t.PrinterID, err == nil, latencyMillis)
}
