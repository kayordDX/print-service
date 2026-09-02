package probe

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kayorddx/print-service/internal/model"
)

func TestStoreSetGet(t *testing.T) {
	t.Parallel()
	s := NewStore()
	if got := s.Get(); len(got) != 0 {
		t.Errorf("fresh store has %d targets, want 0", len(got))
	}
	want := []model.PrinterTarget{{PrinterID: 1, IPAddress: "192.168.1.50", Port: 9100}}
	s.Set(want)
	if got := s.Get(); len(got) != 1 || got[0].PrinterID != 1 {
		t.Errorf("Get() = %+v", got)
	}
	// A later sync replaces the set entirely (stale targets drop out).
	s.Set(nil)
	if got := s.Get(); len(got) != 0 {
		t.Errorf("Get() after clearing sync = %+v, want empty", got)
	}
}

// startAcceptingListener binds an arbitrary free port (rarely 9100) and
// accepts connections until the test ends.
func startAcceptingListener(t *testing.T) (host string, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	serve(ln)
	return splitAddr(t, ln.Addr().String())
}

// serve accepts (and immediately closes) connections until ln is closed.
func serve(ln net.Listener) {
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
}

// closedPort returns a host:port with nothing listening behind it.
func closedPort(t *testing.T) (host string, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, port = splitAddr(t, ln.Addr().String())
	_ = ln.Close()
	return host, port
}

func splitAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return host, port
}

type report struct {
	printerID     int
	reachable     bool
	latencyMillis int64
}

// collectReports gathers probe reports from a channel with a deadline.
func collectReports(t *testing.T, ch <-chan report, n int) []report {
	t.Helper()
	var got []report
	deadline := time.After(2 * time.Second)
	for len(got) < n {
		select {
		case r := <-ch:
			got = append(got, r)
		case <-deadline:
			t.Fatalf("timed out waiting for reports: got %d of %d", len(got), n)
		}
	}
	return got
}

func TestProberReportsReachability(t *testing.T) {
	t.Parallel()
	upHost, upPort := startAcceptingListener(t)
	downHost, downPort := closedPort(t)

	reports := make(chan report, 16)
	p := NewProber(
		NewStore(),
		20*time.Millisecond,
		func(_ context.Context, printerID int, reachable bool, latencyMillis int64) {
			reports <- report{printerID, reachable, latencyMillis}
		},
		func() bool { return true },
		slog.Default(),
	)
	p.dialTimeout = time.Second
	p.store.Set([]model.PrinterTarget{
		{PrinterID: 1, Name: "up", IPAddress: upHost, Port: upPort},
		{PrinterID: 2, Name: "down", IPAddress: downHost, Port: downPort},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	// The up printer reports round 1; the down printer only after the
	// failure threshold (2 rounds) — so 2 reports total may take 3 rounds.
	got := collectReports(t, reports, 3)
	cancel()
	<-done

	byID := map[int]report{}
	for _, r := range got {
		byID[r.printerID] = r
	}
	if r := byID[1]; !r.reachable || r.latencyMillis < 0 {
		t.Errorf("reachable printer report = %+v, want reachable with non-negative latency", r)
	}
	if r := byID[2]; r.reachable {
		t.Errorf("unreachable printer report = %+v, want unreachable", r)
	}
}

// TestProbeHysteresis drives probeOne directly: a single failure must not
// be reported, the second consecutive failure must be, a success resets
// the counter, and the next single failure is withheld again. One listener
// is opened and closed on the same port to make a single printer flap.
func TestProbeHysteresis(t *testing.T) {
	t.Parallel()
	host, port := closedPort(t)
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	down := model.PrinterTarget{PrinterID: 2, IPAddress: host, Port: port}

	reports := make(chan string, 16)
	p := NewProber(NewStore(), time.Second,
		func(_ context.Context, printerID int, reachable bool, _ int64) {
			reports <- fmt.Sprintf("%d:%v", printerID, reachable)
		},
		func() bool { return true },
		slog.Default(),
	)
	p.dialTimeout = time.Second

	// Start down: one failure is withheld...
	p.probeOne(context.Background(), down)
	select {
	case got := <-reports:
		t.Errorf("single failure reported as %q, want nothing", got)
	case <-time.After(50 * time.Millisecond):
	}

	// ...the second consecutive failure is reported...
	p.probeOne(context.Background(), down)
	select {
	case got := <-reports:
		if got != "2:false" {
			t.Errorf("second failure reported as %q, want 2:false", got)
		}
	case <-time.After(time.Second):
		t.Fatal("second consecutive failure was not reported")
	}

	// ...a success (printer back on the same port) resets the counter and
	// reports immediately...
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("re-listen: %v", err)
	}
	serve(ln)
	p.probeOne(context.Background(), down)
	select {
	case got := <-reports:
		if got != "2:true" {
			t.Errorf("recovery probe reported %q, want 2:true", got)
		}
	case <-time.After(time.Second):
		t.Fatal("recovery was not reported")
	}

	// ...so the next single failure is withheld again.
	_ = ln.Close()
	p.probeOne(context.Background(), down)
	select {
	case got := <-reports:
		t.Errorf("failure after reset reported as %q, want nothing", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestProberSkipsWhileDisconnected(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	connected := false
	calls := 0
	p := NewProber(
		NewStore(),
		20*time.Millisecond,
		func(context.Context, int, bool, int64) {
			t.Error("probe reported while hub disconnected")
		},
		func() bool {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return connected
		},
		slog.Default(),
	)
	p.dialTimeout = 100 * time.Millisecond
	p.store.Set([]model.PrinterTarget{{PrinterID: 1, IPAddress: "127.0.0.1", Port: 1}})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	p.Run(ctx)

	mu.Lock()
	defer mu.Unlock()
	if calls == 0 {
		t.Error("prober never checked hub connectivity")
	}
}

func TestProbeNow(t *testing.T) {
	t.Parallel()
	upHost, upPort := startAcceptingListener(t)
	downHost, downPort := closedPort(t)

	tests := []struct {
		name    string
		targets []model.PrinterTarget
		probeID int
		want    []report
	}{
		{
			name:    "reachable target reports immediately",
			targets: []model.PrinterTarget{{PrinterID: 1, Name: "up", IPAddress: upHost, Port: upPort}},
			probeID: 1,
			want:    []report{{1, true, 0}},
		},
		{
			name:    "unreachable target reports without threshold",
			targets: []model.PrinterTarget{{PrinterID: 2, Name: "down", IPAddress: downHost, Port: downPort}},
			probeID: 2,
			want:    []report{{2, false, 0}},
		},
		{
			name:    "unknown printer id is ignored",
			targets: []model.PrinterTarget{{PrinterID: 1, IPAddress: upHost, Port: upPort}},
			probeID: 99,
		},
		{
			name:    "empty probe set is ignored",
			probeID: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reports := make(chan report, 8)
			p := NewProber(
				NewStore(),
				time.Hour, // the periodic prober never runs in this test
				func(_ context.Context, printerID int, reachable bool, latencyMillis int64) {
					reports <- report{printerID, reachable, latencyMillis}
				},
				func() bool { return true },
				slog.Default(),
			)
			p.dialTimeout = time.Second
			p.store.Set(tt.targets)

			p.ProbeNow(context.Background(), tt.probeID)

			for _, want := range tt.want {
				got := collectReports(t, reports, 1)[0]
				if got.printerID != want.printerID || got.reachable != want.reachable {
					t.Errorf("ProbeNow(%d) reported %+v, want printerID %d reachable %v",
						tt.probeID, got, want.printerID, want.reachable)
				}
				if got.latencyMillis < 0 {
					t.Errorf("ProbeNow(%d) latency %dms is negative", tt.probeID, got.latencyMillis)
				}
			}
			// Known ids report exactly once; unknown ids never report.
			select {
			case got := <-reports:
				t.Errorf("unexpected extra report %+v", got)
			case <-time.After(150 * time.Millisecond):
			}
		})
	}
}

// TestProbeNowDropsOverlapWhileInFlight pins the anti-spawn guard: while a
// printer has an on-demand dial running, further requests for it are
// dropped instead of spawning more goroutines.
func TestProbeNowDropsOverlapWhileInFlight(t *testing.T) {
	t.Parallel()
	upHost, upPort := startAcceptingListener(t)
	reports := make(chan report, 4)
	p := NewProber(
		NewStore(),
		time.Hour,
		func(_ context.Context, printerID int, reachable bool, _ int64) {
			reports <- report{printerID, reachable, 0}
		},
		func() bool { return true },
		slog.Default(),
	)
	p.store.Set([]model.PrinterTarget{{PrinterID: 1, IPAddress: upHost, Port: upPort}})

	p.mu.Lock()
	p.inflight[1] = struct{}{}
	p.mu.Unlock()
	p.ProbeNow(context.Background(), 1)

	select {
	case got := <-reports:
		t.Errorf("overlapping request reported %+v, want nothing", got)
	case <-time.After(100 * time.Millisecond):
	}
}

// drainReports collects reports until the channel stays quiet briefly or
// the overall deadline passes, for tests where the exact report count
// depends on timing (overlapping requests are deduped while in flight).
func drainReports(t *testing.T, ch <-chan report) []report {
	t.Helper()
	var got []report
	idle := time.NewTimer(150 * time.Millisecond)
	defer idle.Stop()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case r := <-ch:
			got = append(got, r)
			if !idle.Stop() {
				<-idle.C
			}
			idle.Reset(150 * time.Millisecond)
		case <-idle.C:
			return got
		case <-deadline.C:
			return got
		}
	}
}

// TestProbeNowConcurrent fires overlapping requests for the same and other
// printers: every printer reports at least once, no printer reports more
// than once per request, and every report says reachable.
func TestProbeNowConcurrent(t *testing.T) {
	t.Parallel()
	upHost, upPort := startAcceptingListener(t)
	targets := []model.PrinterTarget{
		{PrinterID: 1, Name: "a", IPAddress: upHost, Port: upPort},
		{PrinterID: 2, Name: "b", IPAddress: upHost, Port: upPort},
		{PrinterID: 3, Name: "c", IPAddress: upHost, Port: upPort},
	}

	reports := make(chan report, 64)
	p := NewProber(
		NewStore(),
		time.Hour,
		func(_ context.Context, printerID int, reachable bool, latencyMillis int64) {
			reports <- report{printerID, reachable, latencyMillis}
		},
		func() bool { return true },
		slog.Default(),
	)
	p.dialTimeout = time.Second
	p.store.Set(targets)

	const requestsPerPrinter = 5
	var wg sync.WaitGroup
	for i := 0; i < requestsPerPrinter; i++ {
		for _, target := range targets {
			wg.Add(1)
			go func(target model.PrinterTarget) {
				defer wg.Done()
				p.ProbeNow(context.Background(), target.PrinterID)
			}(target)
		}
	}
	wg.Wait()

	counts := make(map[int]int)
	for _, r := range drainReports(t, reports) {
		counts[r.printerID]++
		if !r.reachable {
			t.Errorf("probe report %+v, want reachable", r)
		}
	}
	for _, target := range targets {
		if n := counts[target.PrinterID]; n < 1 || n > requestsPerPrinter {
			t.Errorf("printer %d got %d reports, want between 1 and %d",
				target.PrinterID, n, requestsPerPrinter)
		}
	}
}

// TestProbeNowAlongsidePeriodic runs the periodic prober and interleaves
// on-demand requests: both paths keep reporting and neither starves.
func TestProbeNowAlongsidePeriodic(t *testing.T) {
	t.Parallel()
	upHost, upPort := startAcceptingListener(t)
	targets := []model.PrinterTarget{
		{PrinterID: 1, Name: "a", IPAddress: upHost, Port: upPort},
		{PrinterID: 2, Name: "b", IPAddress: upHost, Port: upPort},
	}

	reports := make(chan report, 64)
	p := NewProber(
		NewStore(),
		15*time.Millisecond,
		func(_ context.Context, printerID int, reachable bool, latencyMillis int64) {
			reports <- report{printerID, reachable, latencyMillis}
		},
		func() bool { return true },
		slog.Default(),
	)
	p.store.Set(targets)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	for i := 0; i < 5; i++ {
		p.ProbeNow(ctx, targets[i%len(targets)].PrinterID)
		time.Sleep(5 * time.Millisecond)
	}

	// Both printers are reachable, so every periodic round reports both —
	// several rounds must have passed while ProbeNow requests ran.
	got := collectReports(t, reports, 6)
	cancel()
	<-done

	counts := make(map[int]int)
	for _, r := range got {
		counts[r.printerID]++
		if !r.reachable {
			t.Errorf("probe report %+v, want reachable", r)
		}
	}
	for _, target := range targets {
		if counts[target.PrinterID] < 2 {
			t.Errorf("printer %d got %d reports, want periodic probing to continue",
				target.PrinterID, counts[target.PrinterID])
		}
	}
}
