# Print Service (Go)

The Kayord POS print service: a small, single-binary Go service that runs on a
Raspberry Pi, a Windows back-office PC, or any always-on box at a restaurant,
connects **outbound** to the POS API's SignalR hub (`/printer-hub`), and:

- prints pre-rendered ESC/POS jobs to EPSON-compatible network thermal
  printers (raw TCP, port 9100),
- performs network scans **natively** (concurrent TCP dials — no nmap, no
  root), when the server sends a scan request,
- probes the printers assigned to it and reports reachability live,
- answers device info requests with its platform, host name, current IP
  addresses, versions and uptime (diagnostics for the POS admin UI).

This is a complete rewrite of the former .NET/Redis service. The wire
contract lives in the pos repo: `docs/Print-service.md` (device side) and
`docs/Print.md` (server side). Keep them in sync when changing
`internal/model`.

## Ground rules

- **Outbound connections only.** The device dials the POS API over
  HTTPS/WSS. No inbound ports, no Redis, no VPN.
- **Stdlib first.** The only third-party dependency is the SignalR client
  ([`philippseith/signalr`](https://github.com/philippseith/signalr)).
- **Config = environment variables.** No config files. Outlet and device
  identity are bound to the API key server-side, never to configuration.
- Fully static binaries: `linux/armv6` (Pi Zero 1), `linux/arm64`
  (Pi Zero 2 W, Pi 4/5), `linux/amd64`, plus `windows/amd64`,
  `windows/arm64`, `darwin/amd64` and `darwin/arm64`.

## Configuration

| Env var                  | Required | Default | Meaning                                                                    |
| ------------------------ | -------- | ------- | -------------------------------------------------------------------------- |
| `POS_BASE_URL`           | yes      | —       | POS API base URL, e.g. `https://api.kayord.com` (no trailing slash)        |
| `POS_API_KEY`            | one of   | —       | `kpos_{keyId}.{secret}` — single-key shorthand; created by an outlet manager in the POS admin UI |
| `POS_API_KEYS`           | one of   | —       | Comma-separated list of `kpos_{keyId}.{secret}` keys — serves multiple outlets from one process; error if both key vars are set |
| `LOG_LEVEL`              | no       | `info`  | `debug` \| `info` \| `warn` \| `error`                                     |
| `PROBE_INTERVAL_SECONDS` | no       | `30`    | Printer reachability probe interval                                        |

One of `POS_API_KEY` or `POS_API_KEYS` is required. With `POS_API_KEYS` the
process runs one fully independent app instance per key — its own hub
connection, probe store and print queue — so several outlets can share one
box; log lines carry the `keyId` of the instance they belong to.

Only the public key id (the part before `.`) is ever logged.

## Development

The Go toolchain is pinned per project via [mise](https://mise.jdx.dev) —
see `.mise.toml`. With mise installed, it is fetched automatically:

```bash
mise install        # installs the pinned Go version (project-local)
mise run all        # fmt + vet + test + build
mise run dev        # run the service locally
```

Or the native go way (equivalent, once the pinned toolchain is active via
`mise exec --` or an activated shell):

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -o bin/print-service .
```

All tasks live in `.mise.toml` under `[tasks]` (`mise run build-all`,
`mise run docker`, `mise run clean`, ...).

Project layout:

```
main.go                     — wiring, print worker, signal handling
internal/config/            — env parsing + validation
internal/model/             — wire contract (PrintMessage, PrinterTarget)
internal/hubclient/         — SignalR connection, receiver, reconnect, reporting
internal/printer/           — raw TCP print
internal/scan/              — native subnet scan (replaces nmap)
internal/probe/             — printer reachability probes
```

## Build & deploy

Cross-compiled static binaries (same matrix as CI):

```bash
mise run build-all
# -> dist/print-service-linux-{armv6,arm64,amd64}
#    dist/print-service-windows-{amd64,arm64}.exe
#    dist/print-service-darwin-{amd64,arm64}
```

### Downloadable release binaries

Pushing a `vX.Y.Z` tag (or running the
[Release workflow](.github/workflows/release.yaml) manually) builds and
attaches archives for every platform to the GitHub release:

| Asset                                         | Platform                   |
| --------------------------------------------- | -------------------------- |
| `print-service-{version}-linux-armv6.tar.gz`  | Pi Zero 1                  |
| `print-service-{version}-linux-arm64.tar.gz`  | Pi Zero 2 W, Pi 4/5        |
| `print-service-{version}-linux-amd64.tar.gz`  | x86-64 Linux               |
| `print-service-{version}-windows-amd64.zip`   | Windows (64-bit Intel/AMD) |
| `print-service-{version}-windows-arm64.zip`   | Windows on ARM             |
| `print-service-{version}-darwin-amd64.tar.gz` | macOS (Intel)              |
| `print-service-{version}-darwin-arm64.tar.gz` | macOS (Apple Silicon)      |

### Windows

The service is a plain console program with outbound connections only, so no
inbound firewall rules are needed — it dials the POS API over HTTPS/WSS and
the printers directly (raw TCP, port 9100).

1. Download and extract `print-service-windows-amd64.zip` from
   [Releases](../../releases) (`windows-arm64.zip` on ARM devices), e.g. to
   `C:\kayord\print-service.exe`.
2. Try it in PowerShell to confirm it connects:

   ```powershell
   $env:POS_BASE_URL = "https://api.kayord.com"
   $env:POS_API_KEY  = "kpos_pk_xxxx.yyyy"   # create in the POS admin UI
   .\print-service.exe
   ```

   Stop with `Ctrl+C` — that is the effective stop signal on Windows
   (`SIGTERM` is never delivered there, so the `Ctrl+C` path is what
   matters).

3. To keep it running unattended, register an auto-start scheduled task (no
   extra software needed). The task runs as `SYSTEM` and restarts the
   program whenever it exits:

   ```powershell
   # Machine-wide env vars, read by the program at start-up. They are
   # visible to local users — keep POS_API_KEY on a restricted machine.
   [Environment]::SetEnvironmentVariable("POS_BASE_URL", "https://api.kayord.com", "Machine")
   [Environment]::SetEnvironmentVariable("POS_API_KEY",  "kpos_pk_xxxx.yyyy", "Machine")

   $action   = New-ScheduledTaskAction -Execute "C:\kayord\print-service.exe"
   $trigger  = New-ScheduledTaskTrigger -AtStartup
   $settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) `
                 -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1)
   Register-ScheduledTask -TaskName "print-service" -Action $action `
     -Trigger $trigger -Settings $settings -User "SYSTEM" -RunLevel Highest
   ```

Start it right away with `Start-ScheduledTask -TaskName "print-service"`.

Docker (multi-arch, from `scratch` — the binary is static and needs nothing
but CA certificates for the outbound HTTPS connection):

```bash
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v6 \
  -t ghcr.io/kayorddx/print-service:latest --push .
```

### systemd (for Pis without Docker)

```ini
# /etc/systemd/system/print-service.service
[Unit]
Description=Kayord print service
After=network-online.target

[Service]
Environment=POS_BASE_URL=https://api.kayord.com
EnvironmentFile=-/etc/kayord/print-service.env   # POS_API_KEY lives here, chmod 600
ExecStart=/usr/local/bin/print-service
Restart=always
RestartSec=5
User=kayord

[Install]
WantedBy=multi-user.target
```

### Compose

```yaml
services:
  print-service:
    image: ghcr.io/kayorddx/print-service:latest
    environment:
      POS_BASE_URL: https://api.kayord.com
      POS_API_KEY: kpos_pk_xxxx.yyyy # create in POS admin UI
    restart: unless-stopped
```
