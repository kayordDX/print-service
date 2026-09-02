package hubclient

import (
	"context"
	"fmt"
	"log/slog"
)

// signalrLogger routes the SignalR client's internal logging through the
// app's slog logger. Without this the library defaults to unconditional
// debug output on stderr, flooding the journal with every protocol frame
// regardless of LOG_LEVEL. The library filters only info+ when constructed
// with debug=false, so debug=true is passed and the slog level gate does
// the filtering: silent at info, full protocol trace at debug.
type signalrLogger struct {
	logger *slog.Logger
}

// Log implements the library's StructuredLogger interface (go-kit style
// alternating key/value pairs). The go-kit level value is mapped by string
// so no go-kit import is needed.
func (l signalrLogger) Log(keyvals ...interface{}) error {
	if len(keyvals) == 0 {
		return nil
	}
	if len(keyvals)%2 != 0 {
		padded := make([]interface{}, 0, len(keyvals)+1)
		padded = append(padded, keyvals...)
		keyvals = append(padded, nil)
	}
	level := slog.LevelInfo
	attrs := make([]any, 0, len(keyvals))
	for i := 0; i < len(keyvals); i += 2 {
		key := fmt.Sprint(keyvals[i])
		value := keyvals[i+1]
		switch key {
		case "ts":
			// slog stamps its own time.
			continue
		case "level":
			switch fmt.Sprint(value) {
			case "debug":
				level = slog.LevelDebug
			case "warn":
				level = slog.LevelWarn
			case "error":
				level = slog.LevelError
			default:
				level = slog.LevelInfo
			}
		default:
			attrs = append(attrs, slog.Any(key, value))
		}
	}
	l.logger.Log(context.Background(), level, "signalr", attrs...)
	return nil
}
