// Package printer writes pre-rendered ESC/POS instructions to
// EPSON-compatible network thermal printers over a raw TCP socket.
//
// The POS server renders all ESC/POS bytes; this package is deliberately a
// dumb, reliable TCP relay. Printing is fire-and-forget: errors are reported
// to the caller (which logs them) and the user can simply re-print.
package printer

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/kayorddx/print-service/internal/model"
)

const (
	// dialTimeout bounds how long we wait for the printer to accept the
	// connection. Thermal printers answer within a second when healthy.
	dialTimeout = 5 * time.Second
	// writeTimeout bounds the whole write once connected; receipts are tiny.
	writeTimeout = 10 * time.Second
)

// Print dials the printer at msg.IPAddress:msg.Port and writes all
// instruction chunks in order. The connection is closed before returning.
func Print(ctx context.Context, msg model.PrintMessage) error {
	addr := net.JoinHostPort(msg.IPAddress, strconv.Itoa(msg.Port))

	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial printer %s: %w", addr, err)
	}
	defer conn.Close()

	if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("set write deadline for printer %s: %w", addr, err)
	}
	for i, chunk := range msg.PrintInstructions {
		if _, err := conn.Write(chunk); err != nil {
			return fmt.Errorf("write instruction chunk %d to printer %s: %w", i, addr, err)
		}
	}
	return nil
}
