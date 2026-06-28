---
paths:
  - internal/beads/**
---

# Beads backend conventions

- `client.go` splits `Reader` (List/Show/Export/Comments) and `Writer` (mutations); `Client` embeds both.
- Reads route by store kind: legacy SQLite uses `bd_sqlite.go`/`br_sqlite.go`; Dolt-backed stores (bd ≥ 0.58 / v1.0.5+) use `bd_dolt.go`/`br_dolt.go` via CLI JSON.
- Writes shell out via CLI (`bd_cli.go`/`br_cli.go`).
- Every new interface method MUST be added to `MockClient` (`mock.go`) and covered by `conformance_test.go` for bd+br parity.
- `backend.go` auto-detects bd/br and resolves store kind via `ProbeContext`/`detectStoreKind`; min versions in `version_check.go` (br >= 0.1.7, bd >= 0.30.0). Schema compatibility is gated by `BackendContext.SchemaVersion`.
- `br`/`bd` are third-party (`../beads`, `../beads_rust`) — never edit their source.
