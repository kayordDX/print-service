package model

import (
	"encoding/json"
	"testing"
)

// The SignalR JSON hub protocol is produced by .NET's System.Text.Json:
// camelCase properties and base64-encoded byte arrays. These tests pin the
// exact wire format so a server change cannot slip through unnoticed.
func TestPrintMessageMarshalMatchesDotNet(t *testing.T) {
	t.Parallel()
	msg := PrintMessage{
		PrinterName:       "Front desk",
		IPAddress:         "10.0.0.3",
		Port:              9100,
		PrintInstructions: [][]byte{{0x1b, 0x40}, []byte("Hello")},
	}
	got, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want := `{"printerName":"Front desk","ipAddress":"10.0.0.3","port":9100,` +
		`"printInstructions":["G0A=","SGVsbG8="]}`
	if string(got) != want {
		t.Errorf("json.Marshal() =\n  %s\nwant\n  %s", got, want)
	}
}

func TestPrintMessageActionOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	got, err := json.Marshal(PrintMessage{IPAddress: "1.2.3.4", Port: 9100})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want := `{"printerName":"","ipAddress":"1.2.3.4","port":9100,"printInstructions":null}`
	if string(got) != want {
		t.Errorf("json.Marshal() = %s, want %s", got, want)
	}
}

func TestPrintMessageUnmarshal(t *testing.T) {
	t.Parallel()
	in := `{"action":"nmap","printerName":"Bar","ipAddress":"192.168.1.*","port":9100,` +
		`"printInstructions":["G0A="]}`
	var msg PrintMessage
	if err := json.Unmarshal([]byte(in), &msg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if msg.Action != "nmap" || msg.PrinterName != "Bar" || msg.IPAddress != "192.168.1.*" || msg.Port != 9100 {
		t.Errorf("Unmarshal result = %+v", msg)
	}
	if len(msg.PrintInstructions) != 1 || string(msg.PrintInstructions[0]) != "\x1b@" {
		t.Errorf("PrintInstructions = %v", msg.PrintInstructions)
	}
}

func TestPrinterTargetRoundTrip(t *testing.T) {
	t.Parallel()
	in := `[{"printerId":7,"name":"Bar","ipAddress":"192.168.1.50","port":9100}]`
	var targets []PrinterTarget
	if err := json.Unmarshal([]byte(in), &targets); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(targets) != 1 || targets[0].PrinterID != 7 || targets[0].Name != "Bar" ||
		targets[0].IPAddress != "192.168.1.50" || targets[0].Port != 9100 {
		t.Errorf("targets = %+v", targets)
	}
}
