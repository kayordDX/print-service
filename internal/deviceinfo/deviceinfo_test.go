package deviceinfo

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func TestGather(t *testing.T) {
	t.Parallel()
	start := time.Now().Add(-90 * time.Second)

	info := Gather(start)

	if want := runtime.GOOS + "/" + runtime.GOARCH; info.Platform != want {
		t.Errorf("Platform = %q, want %q", info.Platform, want)
	}
	if want, err := os.Hostname(); err == nil && info.Hostname != want {
		t.Errorf("Hostname = %q, want %q", info.Hostname, want)
	}
	if info.NumCPU != runtime.NumCPU() {
		t.Errorf("NumCPU = %d, want %d", info.NumCPU, runtime.NumCPU())
	}
	if info.GoVersion == "" {
		t.Error("GoVersion is empty")
	}
	if info.AppVersion == "" {
		t.Error("AppVersion is empty")
	}
	// The clock is monotonic, so uptime can only be at least the distance
	// to start; an upper bound would be flaky on a loaded CI box.
	if info.UptimeSeconds < 90 {
		t.Errorf("UptimeSeconds = %d, want >= 90", info.UptimeSeconds)
	}
	if len(info.Interfaces) == 0 {
		// A machine without any up, non-loopback interface cannot reach the
		// hub anyway; tolerate it, but never report loopback.
		t.Log("no non-loopback interfaces found")
	}
	for _, iface := range info.Interfaces {
		if iface.Name == "lo" {
			t.Errorf("loopback interface reported: %+v", iface)
		}
		if len(iface.IPv4) == 0 && len(iface.IPv6) == 0 {
			t.Errorf("interface %s reported without addresses", iface.Name)
		}
	}
}

func TestParseOSRelease(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "pretty name",
			content: "PRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\nNAME=\"Debian GNU/Linux\"\nID=debian\n",
			want:    "Debian GNU/Linux 12 (bookworm)",
		},
		{
			name:    "falls back to NAME",
			content: "NAME=\"Raspbian GNU/Linux\"\nID=raspbian\n",
			want:    "Raspbian GNU/Linux",
		},
		{
			name:    "unquoted value",
			content: "PRETTY_NAME=Fedora Linux 40\n",
			want:    "Fedora Linux 40",
		},
		{
			name:    "neither present",
			content: "ID=ubuntu\nVERSION_ID=24.04\n",
			want:    "",
		},
		{
			name:    "empty",
			content: "",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseOSRelease(tt.content); got != tt.want {
				t.Errorf("parseOSRelease() = %q, want %q", got, tt.want)
			}
		})
	}
}
