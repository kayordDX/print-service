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
		JobID:             "job-42",
	}
	got, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want := `{"printerName":"Front desk","ipAddress":"10.0.0.3","port":9100,` +
		`"printInstructions":["G0A=","SGVsbG8="],"jobId":"job-42"}`
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
		`"printInstructions":["G0A="],"jobId":"job-7"}`
	var msg PrintMessage
	if err := json.Unmarshal([]byte(in), &msg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if msg.Action != "nmap" || msg.PrinterName != "Bar" || msg.IPAddress != "192.168.1.*" || msg.Port != 9100 || msg.JobID != "job-7" {
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

func TestDeviceInfoMarshalMatchesDotNet(t *testing.T) {
	t.Parallel()
	info := DeviceInfo{
		Hostname:      "pi-front",
		Platform:      "linux/arm64",
		OSVersion:     "Debian GNU/Linux 12 (bookworm)",
		GoVersion:     "go1.27.0",
		AppVersion:    "v1.4.2",
		NumCPU:        4,
		UptimeSeconds: 5400,
		Interfaces: []DeviceInterface{
			{Name: "eth0", MAC: "e4:5f:01:aa:bb:cc", IPv4: []string{"192.168.1.23"}},
			{Name: "wlan0", IPv6: []string{"fe80::1234"}},
		},
	}
	got, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want := `{"hostname":"pi-front","platform":"linux/arm64",` +
		`"osVersion":"Debian GNU/Linux 12 (bookworm)","goVersion":"go1.27.0",` +
		`"appVersion":"v1.4.2","numCpu":4,"uptimeSeconds":5400,` +
		`"interfaces":[{"name":"eth0","mac":"e4:5f:01:aa:bb:cc","ipv4":["192.168.1.23"]},` +
		`{"name":"wlan0","ipv6":["fe80::1234"]}]}`
	if string(got) != want {
		t.Errorf("json.Marshal() =\n  %s\nwant\n  %s", got, want)
	}
}

func TestDeviceInfoOmitEmptyFields(t *testing.T) {
	t.Parallel()
	got, err := json.Marshal(DeviceInfo{Hostname: "box", Platform: "windows/amd64", NumCPU: 8})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want := `{"hostname":"box","platform":"windows/amd64","numCpu":8,"uptimeSeconds":0}`
	if string(got) != want {
		t.Errorf("json.Marshal() = %s, want %s", got, want)
	}
}
