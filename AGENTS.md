# AGENTS.md

Kayord POS print service: one static Go binary that runs on a Pi, connects
outbound to the POS API SignalR hub (`/printer-hub`), prints raw ESC/POS to
network thermal printers, scans subnets natively (no nmap), and reports
printer reachability.

## Commands

Go version is pinned project-locally via mise (`.mise.toml`).

```bash
mise run test    # go test ./...
mise run vet     # go vet ./...
mise run fmt     # gofmt
mise run build   # build to bin/
mise run all     # fmt + vet + test + build
```

Plain `go build ./...` / `go test ./...` are equivalent.

## Rules

- Every change must pass `mise run all`: gofmt-clean, vet-clean, all tests
  green, build succeeds.
- Write table-driven tests next to the code (`*_test.go`). New code needs
  tests.
- Idiomatic Go only: handle every error (wrap with `%w`), thread `context`,
  no panics across package boundaries, no globals.
- Stdlib first. The only allowed third-party dep is the SignalR client.
  Run `mise run tidy` after changing deps.
- `internal/model` is the wire contract with Pos.Api. Keep it in sync with
  `docs/Print-service.md` / `docs/Print.md` (kayordDX/pos repo, `aspire`
  branch). Never rename JSON fields; the `"nmap"` action value is legacy
  but load-bearing.
- Never log secrets (API key secret); log the key id only.
