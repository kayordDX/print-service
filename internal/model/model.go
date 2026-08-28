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
