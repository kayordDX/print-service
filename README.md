# Print Service (Go)

The Kayord POS print service: a small, single-binary Go service that runs on a
Raspberry Pi (or any always-on box at a restaurant), connects **outbound** to
the POS API's SignalR hub (`/printer-hub`), and:

- prints pre-rendered ESC/POS jobs to EPSON-compatible network thermal
  printers (raw TCP, port 9100),
- performs network scans **natively** (concurrent TCP dials — no nmap, no
  root), when the server sends a scan request,
- probes the printers assigned to it and reports reachability live.

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
  (Pi Zero 2 W, Pi 4/5), `linux/amd64`.

## Configuration

| Env var                  | Required | Default | Meaning                                                             |
|--------------------------|----------|---------|---------------------------------------------------------------------|
| `POS_BASE_URL`           | yes      | —       | POS API base URL, e.g. `https://api.kayord.com` (no trailing slash) |
| `POS_API_KEY`            | yes*     | —       | `kpos_{keyId}.{secret}` — created by an outlet manager in the POS admin UI |
| `POS_API_KEY_FILE`       | yes*     | —       | File holding the current key (chmod 600). Use *either* this or `POS_API_KEY` |
| `LOG_LEVEL`              | no       | `info`  | `debug` \| `info` \| `warn` \| `error`                              |
| `PROBE_INTERVAL_SECONDS` | no       | `30`    | Printer reachability probe interval                                 |

\* One of the two is required. Prefer `POS_API_KEY_FILE`: when set, a key
rotated by the server is persisted there automatically — the device is
self-managing after the initial setup. Only the public key id (the part
before `.`) is ever logged.

### Key rotation

Managers rotate a key in the POS admin UI; the server pushes the new key to
the connected device (`RotateKey`), the device persists it to
`POS_API_KEY_FILE` and acknowledges (`ReportKeyRotated`), and the server
then revokes the old key — no site visit. Rotation requires the device to
have been started with `POS_API_KEY_FILE`.

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

Cross-compiled static binaries:

```bash
mise run build-all   # -> dist/print-service-linux-{armv6,arm64,amd64}
```

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
      POS_API_KEY: kpos_pk_xxxx.yyyy   # create in POS admin UI
    restart: unless-stopped
```

## Local testing

1. Run the POS API locally (pos repo, `aspire` branch or later) and create a
   print service key as a manager (admin UI or Swagger `POST /printerservicekey`).
2. Fake printer: `nc -lk 9100` in another terminal (or a real EPSON on the LAN).
3. Start the service:

   ```bash
   POS_BASE_URL=http://localhost:5117 POS_API_KEY=kpos_... mise run dev
   # or: POS_BASE_URL=... POS_API_KEY=... go run .
   ```

4. Verify:
   - [ ] Server logs show the hub connection + `SyncPrinters` payload
   - [ ] POS admin printers page shows the device online **live**
   - [ ] `POST /printer/test` prints on the fake/real printer
   - [ ] `POST /printer/scan` → scan starts and results appear (no nmap
         installed — proves the native scan path)
   - [ ] Probes reported; unplugging a printer flips `PrinterReachable`
         within one interval
   - [ ] Kill the network for 30s → the service reconnects by itself
   - [ ] Revoked key → connection dropped and cannot re-establish

## How reconnect works

The SignalR client is created with a connector factory and exponential
backoff with **no give-up time** — an unattended device retries forever.
The server re-joins the connection to its groups on every (re)connect, so
there is no resubscribe logic. Print jobs and probes that happen while
disconnected are simply not reported until the next round.
