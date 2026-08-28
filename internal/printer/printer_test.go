package printer

import (
	"bytes"
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/kayorddx/print-service/internal/model"
)

// startFakePrinter starts a TCP listener that collects everything it
// receives and returns the address plus a channel with the received bytes.
func startFakePrinter(t *testing.T) (addr string, received <-chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ch := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		got, err := io.ReadAll(conn)
		if err != nil {
			return
		}
		ch <- got
	}()
	return ln.Addr().String(), ch
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

func TestPrintWritesAllChunksInOrder(t *testing.T) {
	t.Parallel()
	addr, received := startFakePrinter(t)
	host, port := splitAddr(t, addr)

	chunks := [][]byte{[]byte("chunk one "), {0x1b, 0x40}, []byte("chunk two")}
	err := Print(context.Background(), model.PrintMessage{
		PrinterName:       "Test",
		IPAddress:         host,
		Port:              port,
		PrintInstructions: chunks,
	})
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	select {
	case got := <-received:
		if !bytes.Equal(got, bytes.Join(chunks, nil)) {
			t.Errorf("printer received %q, want %q", got, bytes.Join(chunks, nil))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the fake printer to receive bytes")
	}
}

func TestPrintDialError(t *testing.T) {
	t.Parallel()
	// Reserve a port, then close the listener: nothing is listening there,
	// so the dial must fail.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, port := splitAddr(t, ln.Addr().String())
	_ = ln.Close()

	if err := Print(context.Background(), model.PrintMessage{IPAddress: host, Port: port}); err == nil {
		t.Fatal("Print() succeeded on a closed port, want dial error")
	}
}
