// Package deviceinfo collects information about the machine the print
// service runs on: platform, host name, current network addresses, versions
// and uptime. It is gathered on demand when the server asks for a device
// report, so the POS admin UI can see which box is actually connected.
//
// Gathering is best effort and never fails: a field the machine cannot
// provide (e.g. an OS name on an unsupported platform) is simply left empty.
package deviceinfo

import (
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/kayorddx/print-service/internal/model"
)

// Gather collects the device report. start is the process start time, used
// for the uptime field. It is fast (syscalls and one small file read), so
// it is safe to call on the hub receive loop.
func Gather(start time.Time) model.DeviceInfo {
	hostname, _ := os.Hostname()
	return model.DeviceInfo{
		Hostname:      hostname,
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
		OSVersion:     osRelease(),
		GoVersion:     runtime.Version(),
		AppVersion:    version(),
		NumCPU:        runtime.NumCPU(),
		UptimeSeconds: int64(time.Since(start).Seconds()),
		Interfaces:    interfaces(),
	}
}

// version returns the service version: the module version stamped at build
// time, falling back to the short VCS revision for local builds and to
// "dev" when nothing is known.
func version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return s.Value[:7]
		}
	}
	return "dev"
}

// osRelease returns the pretty OS name from /etc/os-release (Linux only,
// best effort): "Debian GNU/Linux 12 (bookworm)" on a typical Pi, empty on
// platforms without the file.
func osRelease() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	return parseOSRelease(string(data))
}

// parseOSRelease extracts PRETTY_NAME from os-release content, falling back
// to NAME; it returns "" when neither is present.
func parseOSRelease(content string) string {
	fields := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		fields[key] = strings.Trim(value, `"'`)
	}
	if v := fields["PRETTY_NAME"]; v != "" {
		return v
	}
	return fields["NAME"]
}

// interfaces lists the up, non-loopback network interfaces with their MAC
// and current IP addresses. The LAN address the server sees in scan and
// probe traffic is normally the first IPv4 entry of the first interface.
func interfaces() []model.DeviceInterface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []model.DeviceInterface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		entry := model.DeviceInterface{Name: iface.Name}
		if len(iface.HardwareAddr) > 0 {
			entry.MAC = iface.HardwareAddr.String()
		}
		for _, addr := range addrs {
			var ip net.IP
			switch a := addr.(type) {
			case *net.IPNet:
				ip = a.IP
			case *net.IPAddr:
				ip = a.IP
			default:
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				entry.IPv4 = append(entry.IPv4, ip4.String())
			} else {
				entry.IPv6 = append(entry.IPv6, ip.String())
			}
		}
		// An interface without addresses (e.g. a downed tunnel) carries no
		// diagnostic value.
		if len(entry.IPv4) == 0 && len(entry.IPv6) == 0 {
			continue
		}
		out = append(out, entry)
	}
	return out
}
