package hubclient

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
)

type capturedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]string
}

type captureHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := capturedRecord{level: r.Level, msg: r.Message, attrs: map[string]string{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = fmt.Sprint(a.Value.Any())
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, rec)
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func capture(t *testing.T, keyvals ...interface{}) capturedRecord {
	t.Helper()
	h := &captureHandler{}
	logger := slog.New(h)
	if err := (signalrLogger{logger: logger}).Log(keyvals...); err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	if len(h.records) != 1 {
		t.Fatalf("Log() produced %d records, want 1", len(h.records))
	}
	return h.records[0]
}

func TestSignalrLoggerLevelMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		signalrLevel string
		want         slog.Level
	}{
		{name: "debug", signalrLevel: "debug", want: slog.LevelDebug},
		{name: "info", signalrLevel: "info", want: slog.LevelInfo},
		{name: "warn", signalrLevel: "warn", want: slog.LevelWarn},
		{name: "error", signalrLevel: "error", want: slog.LevelError},
		{name: "unknown falls back to info", signalrLevel: "loud", want: slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := capture(t, "level", tt.signalrLevel, "protocol", "JSON")
			if rec.level != tt.want {
				t.Errorf("level = %v, want %v", rec.level, tt.want)
			}
			if rec.msg != "signalr" {
				t.Errorf("msg = %q, want %q", rec.msg, "signalr")
			}
			if got := rec.attrs["protocol"]; got != "JSON" {
				t.Errorf("protocol attr = %q, want %q", got, "JSON")
			}
			if _, ok := rec.attrs["level"]; ok {
				t.Error("level keyval leaked into attrs")
			}
		})
	}
}

func TestSignalrLoggerSkipsTimestamp(t *testing.T) {
	t.Parallel()
	rec := capture(t, "ts", "2026-09-02T22:00:00Z", "event", "write", "message", "{}")
	if _, ok := rec.attrs["ts"]; ok {
		t.Error("ts keyval leaked into attrs")
	}
	if got := rec.attrs["event"]; got != "write" {
		t.Errorf("event attr = %q, want %q", got, "write")
	}
}

func TestSignalrLoggerToleratesOddKeyvals(t *testing.T) {
	t.Parallel()
	rec := capture(t, "level", "debug", "dangling")
	if rec.level != slog.LevelDebug {
		t.Errorf("level = %v, want %v", rec.level, slog.LevelDebug)
	}
	if got, ok := rec.attrs["dangling"]; !ok || got != "<nil>" {
		t.Errorf(`dangling attr = %q, %v; want "<nil>", true`, got, ok)
	}
}

func TestSignalrLoggerEmptyKeyvals(t *testing.T) {
	t.Parallel()
	h := &captureHandler{}
	if err := (signalrLogger{logger: slog.New(h)}).Log(); err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	if len(h.records) != 0 {
		t.Errorf("Log() produced %d records, want 0", len(h.records))
	}
}
