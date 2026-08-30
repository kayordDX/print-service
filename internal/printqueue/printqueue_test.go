package printqueue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kayorddx/print-service/internal/model"
)

func job(ip string, port int, marker string) model.PrintMessage {
	return model.PrintMessage{PrinterName: marker, IPAddress: ip, Port: port}
}

// gatedHandler blocks each job for printer "slow" until its gate is closed;
// other printers complete immediately. It records the completion order.
func gatedHandler(t *testing.T, order *[]string, mu *sync.Mutex) (Handler, func(marker string)) {
	t.Helper()
	gates := map[string]chan struct{}{}
	var gatesMu sync.Mutex
	wait := func(marker string) {
		gatesMu.Lock()
		gate, ok := gates[marker]
		gatesMu.Unlock()
		if ok {
			<-gate
		}
	}
	return func(_ context.Context, msg model.PrintMessage) {
			wait(msg.PrinterName)
			mu.Lock()
			*order = append(*order, msg.PrinterName)
			mu.Unlock()
		}, func(marker string) {
			gatesMu.Lock()
			gates[marker] = make(chan struct{})
			gatesMu.Unlock()
		}
}

func TestSlowPrinterDoesNotBlockOthers(t *testing.T) {
	t.Parallel()
	var (
		mu    sync.Mutex
		order []string
	)
	handler, openGate := gatedHandler(t, &order, &mu)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := New(ctx, handler)
	q.idle = time.Second

	// "slow" jobs block until released; "fast" has no gate.
	openGate("slow")
	if !q.Enqueue(job("10.0.0.1", 9100, "slow")) {
		t.Fatal("Enqueue(slow) rejected")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5; i++ {
			if !q.Enqueue(job("10.0.0.2", 9100, "fast")) {
				t.Error("Enqueue(fast) rejected")
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("jobs for the fast printer were blocked by the slow printer")
	}
}

func TestJobsForSamePrinterRunInOrder(t *testing.T) {
	t.Parallel()
	var (
		mu    sync.Mutex
		order []string
	)
	handler, _ := gatedHandler(t, &order, &mu)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := New(ctx, handler)

	for _, marker := range []string{"a", "b", "c"} {
		if !q.Enqueue(job("10.0.0.1", 9100, marker)) {
			t.Fatalf("Enqueue(%s) rejected", marker)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(order)
		mu.Unlock()
		if n == 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if got, want := order, []string{"a", "b", "c"}; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("completion order = %v, want %v", got, want)
	}
}

func TestFullQueueIsRejected(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	arrived := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := New(ctx, func(context.Context, model.PrintMessage) {
		arrived <- struct{}{}
		<-release
	})
	q.size = 2

	// First job is picked up by the worker (blocked in the handler), so the
	// buffer is empty and can hold exactly q.size more jobs.
	if !q.Enqueue(job("10.0.0.1", 9100, "j")) {
		t.Fatal("Enqueue #1 rejected")
	}
	select {
	case <-arrived:
	case <-time.After(2 * time.Second):
		t.Fatal("worker never started the first job")
	}
	for i := 0; i < q.size; i++ {
		if !q.Enqueue(job("10.0.0.1", 9100, "j")) {
			t.Fatalf("Enqueue #%d rejected before the queue was full", i+2)
		}
	}
	if q.Enqueue(job("10.0.0.1", 9100, "overflow")) {
		t.Error("Enqueue beyond capacity succeeded, want rejection")
	}
	close(release)
}

func TestIdleWorkerIsReplaced(t *testing.T) {
	t.Parallel()
	processed := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := New(ctx, func(_ context.Context, msg model.PrintMessage) { processed <- msg.PrinterName })
	q.idle = 20 * time.Millisecond

	if !q.Enqueue(job("10.0.0.1", 9100, "first")) {
		t.Fatal("Enqueue rejected")
	}
	if got := <-processed; got != "first" {
		t.Fatalf("processed %q, want %q", got, "first")
	}

	// Let the worker idle out and clean up.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		n := len(q.queues)
		q.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	q.mu.Lock()
	if n := len(q.queues); n != 0 {
		t.Fatalf("worker did not idle out: %d queue(s) remain", n)
	}
	q.mu.Unlock()

	// The next job must spin up a fresh worker and complete.
	if !q.Enqueue(job("10.0.0.1", 9100, "second")) {
		t.Fatal("Enqueue after idle rejected")
	}
	select {
	case got := <-processed:
		if got != "second" {
			t.Errorf("processed %q, want %q", got, "second")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("job after idle timeout was never processed")
	}
}

func TestFallbackPortSharesOneWorker(t *testing.T) {
	t.Parallel()
	// A message without a port and one with the explicit default must land
	// on the same printer queue, not two.
	release := make(chan struct{})
	arrived := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := New(ctx, func(context.Context, model.PrintMessage) {
		arrived <- struct{}{}
		<-release
	})
	q.size = 2

	if !q.Enqueue(job("10.0.0.7", 0, "no-port")) {
		t.Fatal("Enqueue(port 0) rejected")
	}
	if !q.Enqueue(job("10.0.0.7", fallbackPort, "explicit")) {
		t.Fatal("Enqueue(explicit port) rejected")
	}
	q.mu.Lock()
	queues := len(q.queues)
	q.mu.Unlock()
	if queues != 1 {
		t.Errorf("got %d queues for the same printer, want 1", queues)
	}
	close(release)
}
