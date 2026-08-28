// Package printqueue serializes print jobs per printer: jobs for the same
// printer are delivered in order by a dedicated worker, while different
// printers print concurrently. A slow or offline printer therefore cannot
// delay receipts for the other printers a device serves.
//
// Workers are created lazily per printer and exit after an idle timeout, so
// goroutines stay bounded by the number of printers actually in use.
package printqueue

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/kayorddx/print-service/internal/model"
)

const (
	// DefaultQueueSize is the per-printer queue capacity. The queue only
	// bridges the hub receive loop and the worker; it is not a store.
	DefaultQueueSize = 64
	// DefaultIdleTimeout is how long a worker stays alive after its last
	// job before shutting down.
	DefaultIdleTimeout = 30 * time.Second
	// fallbackPort matches the server's EPSON raw-print default for
	// messages that omit the port.
	fallbackPort = 9100
)

// Handler processes one job. It is called on the printer's dedicated
// worker goroutine and may block (dials, writes); the queue only guarantees
// that jobs for the same printer run sequentially.
type Handler func(ctx context.Context, msg model.PrintMessage)

// Queue dispatches print jobs to per-printer workers. It is safe for
// concurrent use.
type Queue struct {
	ctx     context.Context
	handler Handler
	logger  *slog.Logger
	size    int
	idle    time.Duration

	mu     sync.Mutex
	queues map[string]chan model.PrintMessage
}

// New returns a queue whose workers run under ctx and process jobs with
// handler. Defaults: DefaultQueueSize per printer, DefaultIdleTimeout.
func New(ctx context.Context, handler Handler, logger *slog.Logger) *Queue {
	return &Queue{
		ctx:     ctx,
		handler: handler,
		logger:  logger,
		size:    DefaultQueueSize,
		idle:    DefaultIdleTimeout,
		queues:  make(map[string]chan model.PrintMessage),
	}
}

// Enqueue queues a job for its printer and reports whether it was accepted.
// A false return means that printer's queue is full; the job is dropped
// loudly by the caller rather than stalling the hub receive loop.
func (q *Queue) Enqueue(msg model.PrintMessage) bool {
	if msg.Port <= 0 {
		msg.Port = fallbackPort
	}
	key := printerKey(msg)

	q.mu.Lock()
	ch, ok := q.queues[key]
	if !ok {
		ch = make(chan model.PrintMessage, q.size)
		q.queues[key] = ch
		go q.worker(key, ch)
	}
	// Send under the lock (never blocks: buffered + select) so a worker
	// idling out concurrently cannot discard the queue with our job in it.
	select {
	case ch <- msg:
		q.mu.Unlock()
		return true
	default:
		q.mu.Unlock()
		return false
	}
}

// worker processes jobs for one printer until ctx is canceled or the queue
// has been idle long enough, then removes itself.
func (q *Queue) worker(key string, ch chan model.PrintMessage) {
	defer q.remove(key, ch)
	for {
		select {
		case <-q.ctx.Done():
			return
		case msg := <-ch:
			q.handler(q.ctx, msg)
		case <-time.After(q.idle):
			if q.removeIfIdle(key, ch) {
				return
			}
		}
	}
}

// removeIfIdle removes the queue unless jobs arrived since the worker went
// idle. It reports whether the worker should stop.
func (q *Queue) removeIfIdle(key string, ch chan model.PrintMessage) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(ch) > 0 {
		return false // work arrived; keep going
	}
	if cur, ok := q.queues[key]; ok && cur == ch {
		delete(q.queues, key)
	}
	return true
}

// remove is the unconditional cleanup on shutdown.
func (q *Queue) remove(key string, ch chan model.PrintMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if cur, ok := q.queues[key]; ok && cur == ch {
		delete(q.queues, key)
	}
}

func printerKey(msg model.PrintMessage) string {
	return net.JoinHostPort(msg.IPAddress, strconv.Itoa(msg.Port))
}
