package probe

import (
	"context"
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

type report struct {
	printerID     int
	reachable     bool
	latencyMillis int64
}

func TestProberReportsReachability(t *testing.T) {
	t.Parallel()

	// One reachable printer and one port with nothing behind it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	upHost, upPortStr, _ := net.SplitHostPort(ln.Addr().String())
	upPort, _ := strconv.Atoi(upPortStr)

	freeLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	downHost, downPortStr, _ := net.SplitHostPort(freeLn.Addr().String())
	downPort, _ := strconv.Atoi(downPortStr)
	_ = freeLn.Close()

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

	got := collectReports(t, reports, 2)
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
