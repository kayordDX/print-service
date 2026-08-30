// Package model defines the wire contract between the POS API printer hub
// and this device. It must stay in sync with docs/Print.md in the pos repo
// (Pos.Api Features.Printer.PrintMessage and PrinterHub.PrinterTarget).
//
// The SignalR JSON hub protocol uses camelCase property names and .NET
// serializes byte[] as base64 strings — Go's encoding/json does the same for
// []byte, so plain struct tags are all that is needed.
package model

// PrintMessage mirrors Pos.Api Features.Printer.PrintMessage.
// It is either a print job (PrintInstructions holds raw ESC/POS bytes
// pre-rendered by the server) or a scan request (Action == "nmap").
type PrintMessage struct {
	// Action is empty for print jobs; "nmap" requests a network scan.
	// The legacy value is kept for wire compatibility — do NOT rename it.
	Action string `json:"action,omitempty"`
	// PrinterName is a human-readable label used for log lines only.
	PrinterName string `json:"printerName"`
	// IPAddress is the printer IP, or a "192.168.1.*"-style pattern for scans.
	IPAddress string `json:"ipAddress"`
	// Port is the printer's TCP port (raw 9100 for EPSON-compatible printers).
	Port int `json:"port"`
	// PrintInstructions holds the raw ESC/POS byte chunks to write, in order.
	// Empty for scan requests.
	PrintInstructions [][]byte `json:"printInstructions"`
	// JobID uniquely identifies this print job server-side. The device
	// echoes it back via ReportPrintResult so the server can track
	// success/failure and dedup (e.g. during dual-transport migration).
	// Empty on legacy servers: the device then skips result reporting.
	JobID string `json:"jobId,omitempty"`
}

// DeviceInfo is the report the device sends via ReportDeviceInfo when the
// server invokes RequestDeviceInfo. Everything except the first two fields
// is best effort: a field the device cannot determine is left empty.
type DeviceInfo struct {
	// Hostname is the machine's host name (os.Hostname).
	Hostname string `json:"hostname"`
	// Platform is the compiled platform, "GOOS/GOARCH" — e.g. "linux/arm64".
	Platform string `json:"platform"`
	// OSVersion is the pretty OS name, e.g. "Debian GNU/Linux 12
	// (bookworm)". Linux only; empty elsewhere.
	OSVersion string `json:"osVersion,omitempty"`
	// GoVersion is the Go toolchain the binary was built with.
	GoVersion string `json:"goVersion,omitempty"`
	// AppVersion is the print-service version: the build module version or,
	// for local builds, the short VCS revision; "dev" as a last resort.
	AppVersion string `json:"appVersion,omitempty"`
	// NumCPU is the number of usable logical CPUs.
	NumCPU int `json:"numCpu"`
	// UptimeSeconds is how long this service has been running.
	UptimeSeconds int64 `json:"uptimeSeconds"`
	// Interfaces lists the up, non-loopback network interfaces with their
	// current addresses — this is where the "current IP address" lives.
	Interfaces []DeviceInterface `json:"interfaces,omitempty"`
}

// DeviceInterface is one network interface inside a DeviceInfo report.
type DeviceInterface struct {
	Name string `json:"name"`
	// MAC is the hardware address; empty for interfaces without one (e.g.
	// tunnels).
	MAC string `json:"mac,omitempty"`
	// IPv4 holds the interface's IPv4 addresses, e.g. "192.168.1.23".
	IPv4 []string `json:"ipv4,omitempty"`
	// IPv6 holds the interface's IPv6 addresses, e.g. "fe80::1234".
	IPv6 []string `json:"ipv6,omitempty"`
}

// PrinterTarget mirrors Pos.Api PrinterHub.PrinterTarget. The server sends
// the device its printer set via SyncPrinters; the device probes each
// target and reports reachability back.
type PrinterTarget struct {
	PrinterID int    `json:"printerId"`
	Name      string `json:"name"`
	IPAddress string `json:"ipAddress"`
	Port      int    `json:"port"`
}
