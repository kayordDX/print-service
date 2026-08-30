// Package scan implements native subnet scanning: concurrent TCP-connect
// checks that replace the external nmap binary the previous .NET service
// shelled out to. No root privileges and no external tools are required.
package scan

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// dialTimeout is deliberately short: unreachable hosts are the common
	// case during a scan, and waiting seconds for each would make a /24
	// crawl even with concurrency.
	dialTimeout = 300 * time.Millisecond
	// maxConcurrent bounds simultaneously open dials so a scan does not
	// exhaust file descriptors on a small device.
	maxConcurrent = 128
	// maxCandidates bounds pattern expansion to keep a runaway pattern
	// (e.g. "*.*.*.*") from melting the network.
	maxCandidates = 1 << 14 // 16384 hosts
)

// Run expands ipPattern and concurrently checks whether candidate:port
// accepts TCP connections. ipPattern accepts a single IP ("192.168.1.50"),
// an octet wildcard or range ("192.168.1.*", "192.168.1.10-200") and CIDR
// notation ("192.168.1.0/24"). It returns a human-readable summary that the
// POS admin UI displays verbatim.
func Run(ctx context.Context, ipPattern string, port int) (string, error) {
	start := time.Now()

	candidates, err := Expand(ipPattern)
	if err != nil {
		return "", err
	}

	type hit struct {
		host    string
		latency time.Duration
	}

	hits := make([]hit, len(candidates))
	var hitsMu sync.Mutex

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
loop:
	for i, host := range candidates {
		select {
		case <-ctx.Done():
			break loop
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(i int, host string) {
			defer wg.Done()
			defer func() { <-sem }()

			dialer := net.Dialer{Timeout: dialTimeout}
			began := time.Now()
			conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err != nil {
				return
			}
			_ = conn.Close()
			latency := time.Since(began)

			hitsMu.Lock()
			hits[i] = hit{host: host, latency: latency}
			hitsMu.Unlock()
		}(i, host)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("scan of %s canceled: %w", ipPattern, err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Scan of %s port %d\n", ipPattern, port)
	fmt.Fprintf(&b, "Started: %s\n", start.Format(time.RFC1123))
	fmt.Fprintf(&b, "Checked %d hosts in %s\n\n", len(candidates), time.Since(start).Round(time.Millisecond))

	open := make([]hit, 0, len(hits))
	for _, h := range hits {
		if h.host != "" {
			open = append(open, h)
		}
	}
	if len(open) == 0 {
		b.WriteString("No hosts with an open port found.\n")
		return b.String(), nil
	}
	b.WriteString("Open ports:\n")
	for _, h := range open {
		fmt.Fprintf(&b, "  %s open (%s)\n", net.JoinHostPort(h.host, strconv.Itoa(port)), h.latency.Round(time.Millisecond))
	}
	return b.String(), nil
}

// Expand resolves an IP pattern into individual candidate addresses, in
// ascending order.
func Expand(pattern string) ([]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("empty IP pattern: want a single IP, 192.168.1.*-style wildcard, octet range like 192.168.1.10-200, or CIDR")
	}

	if _, ipnet, err := net.ParseCIDR(pattern); err == nil {
		return expandCIDR(ipnet)
	}

	parts := strings.Split(pattern, ".")
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid IP pattern %q: want a single IP, 192.168.1.*-style wildcard, octet range like 192.168.1.10-200, or CIDR", pattern)
	}
	octets := make([][]int, 4)
	total := 1
	for i, part := range parts {
		octet, err := expandOctet(part)
		if err != nil {
			return nil, fmt.Errorf("invalid IP pattern %q: %w", pattern, err)
		}
		octets[i] = octet
		total *= len(octet)
		if total > maxCandidates {
			return nil, fmt.Errorf("IP pattern %q expands to %d hosts, more than the maximum of %d", pattern, total, maxCandidates)
		}
	}

	candidates := make([]string, 0, total)
	for a := range octets[0] {
		for b := range octets[1] {
			for c := range octets[2] {
				for d := range octets[3] {
					candidates = append(candidates, fmt.Sprintf("%d.%d.%d.%d", octets[0][a], octets[1][b], octets[2][c], octets[3][d]))
				}
			}
		}
	}
	return candidates, nil
}

// expandOctet expands one dotted-quad component: "*" (0-255), "a-b" or a
// plain number.
func expandOctet(part string) ([]int, error) {
	switch {
	case part == "*":
		octet := make([]int, 256)
		for i := range octet {
			octet[i] = i
		}
		return octet, nil
	case strings.Contains(part, "-"):
		lo, hi, _ := strings.Cut(part, "-")
		l, err1 := strconv.Atoi(lo)
		h, err2 := strconv.Atoi(hi)
		if err1 != nil || err2 != nil || l < 0 || h > 255 || l > h {
			return nil, fmt.Errorf("invalid octet range %q: want 0-255", part)
		}
		octet := make([]int, 0, h-l+1)
		for i := l; i <= h; i++ {
			octet = append(octet, i)
		}
		return octet, nil
	default:
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return nil, fmt.Errorf("invalid octet %q: want 0-255, * or a range like 10-200", part)
		}
		return []int{n}, nil
	}
}

// expandCIDR enumerates all usable host addresses of an IPv4 CIDR range,
// skipping the network and broadcast addresses where applicable.
func expandCIDR(ipnet *net.IPNet) ([]string, error) {
	base := ipnet.IP.To4()
	if base == nil {
		return nil, fmt.Errorf("only IPv4 ranges are supported, got %q", ipnet.String())
	}
	ones, bits := ipnet.Mask.Size()
	hostBits := bits - ones
	// Compare host bits directly: 1<<hostBits overflows 32-bit int (the
	// armv6 build) for hostBits >= 32 and would silently skip the check.
	if hostBits > 14 {
		return nil, fmt.Errorf("range %q is too large to scan (max %d hosts)", ipnet.String(), maxCandidates)
	}

	first, last := 0, 1<<hostBits
	if hostBits >= 2 { // skip network and broadcast
		first++
		last--
	}

	candidates := make([]string, 0, last-first)
	for i := first; i < last; i++ {
		ip := make(net.IP, 4)
		copy(ip, base)
		binary.BigEndian.PutUint32(ip, binary.BigEndian.Uint32(ip)+uint32(i))
		candidates = append(candidates, ip.String())
	}
	return candidates, nil
}
