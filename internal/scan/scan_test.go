package scan

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExpandSingleIP(t *testing.T) {
	t.Parallel()
	got, err := Expand("192.168.1.50")
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if len(got) != 1 || got[0] != "192.168.1.50" {
		t.Errorf("Expand() = %v, want [192.168.1.50]", got)
	}
}

func TestExpandWildcard(t *testing.T) {
	t.Parallel()
	got, err := Expand("10.0.0.*")
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if len(got) != 256 {
		t.Fatalf("Expand() produced %d candidates, want 256", len(got))
	}
	if got[0] != "10.0.0.0" || got[255] != "10.0.0.255" {
		t.Errorf("Expand() endpoints = %q, %q", got[0], got[255])
	}
}

func TestExpandOctetRange(t *testing.T) {
	t.Parallel()
	got, err := Expand("192.168.1.10-12")
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	want := []string{"192.168.1.10", "192.168.1.11", "192.168.1.12"}
	if len(got) != len(want) {
		t.Fatalf("Expand() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Expand()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandCIDRSkipsNetworkAndBroadcast(t *testing.T) {
	t.Parallel()
	got, err := Expand("192.168.1.4/30")
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	want := []string{"192.168.1.5", "192.168.1.6"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Expand() = %v, want %v", got, want)
	}
}

func TestExpandErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "empty", pattern: "   ", want: "empty IP pattern"},
		{name: "not an ip", pattern: "kitchen-printer", want: "invalid IP pattern"},
		{name: "octet out of range", pattern: "192.168.1.999", want: "invalid octet"},
		{name: "bad range", pattern: "192.168.1.200-100", want: "invalid octet range"},
		{name: "wildcard too broad", pattern: "*.*.*.*", want: "more than the maximum"},
		{name: "cidr too large", pattern: "10.0.0.0/8", want: "too large to scan"},
		{name: "ipv6 cidr", pattern: "2001:db8::/32", want: "IPv4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Expand(tt.pattern)
			if err == nil {
				t.Fatalf("Expand(%q) succeeded, want error containing %q", tt.pattern, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Expand(%q) error = %v, want it to contain %q", tt.pattern, err, tt.want)
			}
		})
	}
}

func freePort(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return host, port
}

func TestRunFindsOpenPort(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	output, err := Run(context.Background(), host, port)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output, net.JoinHostPort(host, strconv.Itoa(port))) {
		t.Errorf("Run() output does not mention the open port:\n%s", output)
	}
	if !strings.Contains(output, "Open ports:") {
		t.Errorf("Run() output does not list open ports:\n%s", output)
	}
}

func TestRunReportsNoHosts(t *testing.T) {
	t.Parallel()
	// A port we just released is (almost certainly) closed.
	host, port := freePort(t)

	output, err := Run(context.Background(), host, port)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output, "No hosts with an open port found") {
		t.Errorf("Run() output = %q, want the no-hosts message", output)
	}
}

func TestRunCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, "10.0.0.*", 9100); err == nil {
		t.Fatal("Run() with a canceled context succeeded, want error")
	}
}

// TestExpandPerformance guards against accidental quadratic expansion.
func TestExpandLargeRange(t *testing.T) {
	t.Parallel()
	start := time.Now()
	if _, err := Expand("10.1.0-63.1-254"); err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Expand() took %s, want well under a second", elapsed)
	}
}
