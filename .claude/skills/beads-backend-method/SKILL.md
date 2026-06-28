---
name: beads-backend-method
description: Adds a Reader/Writer operation across the beads backend abstraction in internal/beads: extends the interface in client.go, implements it in the SQLite clients (reads) or CLI clients (writes), wires the MockClient, and adds bd+br conformance coverage. Use when the user says 'add a backend method', 'support a new br/bd operation', 'read/write a new beads field', or 'wire a new beads CLI command'. Capabilities: classifies read-vs-write, mirrors the frozen bd / evolving br split, enforces TDD with fake-binary CLI tests and seeded SQLite tests. Do NOT use for pure UI/Bubble Tea changes, theme tweaks, or anything outside internal/beads.
paths:
  - internal/beads/*.go
---
# Add a beads Backend Method

The `internal/beads` package wraps two issue-tracker CLIs — `bd` (Go, **FROZEN** at v0.38.0) and `br` (Rust, **EVOLVING**, the active backend). One Go interface fronts both. Reads hit SQLite directly; writes shell out to the CLI.

## Critical

1. **Classify the operation FIRST. This decides every file you touch.**
   - **Read** (returns data, no mutation) → goes in the `Reader` interface. Implemented *directly via SQLite* in `bd_sqlite.go` AND `br_sqlite.go`. CLI clients do NOT implement `Reader`.
   - **Write** (mutates state) → goes in the `Writer` interface. Implemented by shelling out in `bd_cli.go` AND `br_cli.go`. The SQLite clients implement it by *delegating* to their embedded `writer` (`return c.writer.X(...)`).
2. **The interface is shared, so you MUST touch bd files too.** `bdCLIClient`/`bdSQLiteClient` and the `br*` clients all satisfy the same `Client` interface. Adding a method to `Reader`/`Writer` will not compile until every implementor has it. "FROZEN" means *do not add bd-only behavior or features* — it does NOT exempt bd from satisfying the interface. Mirror the `br` implementation in the `bd` file exactly.
3. **TDD is mandatory (red-green-refactor).** Write the failing test before the implementation. `make check-test` must pass before you close the bead. See `AGENTS.md` → Testing Requirements.
4. **Never modify the `bd`/`br` source** (sibling `../beads`, `../beads_rust`). They are third-party; only the abacus wrapper changes.
5. **Respect Go size limits** (production files 500 lines, functions 60 lines / 40 statements). `bd_cli.go` and `br_*` files are already large — keep new methods tight; extract a helper if a method exceeds 60 lines.

## Instructions

### Step 1 — Extend the interface in `internal/beads/client.go`

Add ONE method signature to the correct interface block. Match the existing `ctx context.Context` first-arg convention and the multi-return shape of neighbors.

- Read → add to `Reader` (e.g. `Search(ctx context.Context, query string) ([]LiteIssue, error)`).
- Write → add to `Writer` (e.g. `Assign(ctx context.Context, issueID, assignee string) error`).

Reuse the existing model types from `types.go` (`LiteIssue`, `FullIssue`, `Comment`, `Dependency`, `Dependent`). Do NOT invent a new struct unless the data genuinely doesn't fit one.

**Validation gate:** Run `go build ./internal/beads/`. It MUST fail with `*bdCLIClient does not implement Writer` (write) or `*brSQLiteClient does not implement Reader` (read). A failing build here confirms the interface changed and lists exactly which clients you must update next.

### Step 2A — WRITE: implement in `br_cli.go`, then mirror in `bd_cli.go`

Uses Step 1's signature. Copy the shape of an existing write method (e.g. `UpdateStatus`, `AddLabel`). Pattern:

```go
func (c *brCLIClient) Assign(ctx context.Context, issueID, assignee string) error {
	if strings.TrimSpace(issueID) == "" {
		return fmt.Errorf("issue id is required for assign")
	}
	_, err := c.run(ctx, "update", issueID, "--assignee", assignee)
	if err != nil {
		return fmt.Errorf("run br update: %w", err)
	}
	return nil
}
```

Rules drawn from existing code:
- Validate every required string arg with `strings.TrimSpace(x) == ""` → `fmt.Errorf("X is required for <op>")`. Apply defaults the way `Create` does (`issueType = "task"`, `depType = "blocks"`).
- Invoke the CLI through `c.run(ctx, args...)` only — never `exec.Command` directly. `run` prepends `c.dbArgs` and (for br) sets `cmd.Dir`.
- Wrap the run error as `fmt.Errorf("run br <subcommand>: %w", err)`.
- **br vs bd argument differences (do not blur them):** br takes the title *positionally* (`"create", title`) while bd uses `--title`; br accepts a single `--set-labels` with comma-joined values while bd accepts repeated `--set-labels` flags (see the comment block in `br_cli.go` `UpdateFull`). Copy each backend's existing arg style.
- If you need the created/updated record back, append `--json`, then `jsonBytes := extractJSON(out)` (shared helper, handles warning-prefixed output), then `json.Unmarshal(jsonBytes, &v)`. Guard `jsonBytes == nil` and truncate error snippets with `maxErrorSnippetLen`, exactly as `CreateFull` does.
- Mirror the identical method in `bd_cli.go` with `bd`-style args and `fmt.Errorf("run bd ...")`.

**Validation gate:** `go build ./internal/beads/` — the CLI-client errors from Step 1 disappear. Remaining errors will name the SQLite clients (Step 2C).

### Step 2B — READ: implement in `br_sqlite.go`, then mirror in `bd_sqlite.go`

Uses Step 1's signature. Copy the shape of `Comments` / `Export`. Pattern:

```go
func (c *brSQLiteClient) Search(ctx context.Context, query string) ([]LiteIssue, error) {
	db, err := c.openDB(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(ctx, `
		SELECT id FROM issues
		WHERE status != 'tombstone' AND (deleted_at IS NULL) AND title LIKE ?
		ORDER BY created_at, id
	`, "%"+query+"%")
	if err != nil {
		return nil, fmt.Errorf("query issues: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []LiteIssue
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan issue id: %w", err)
		}
		out = append(out, LiteIssue{ID: id})
	}
	return out, rows.Err()
}
```

Rules drawn from existing code:
- Open with `c.openDB(ctx)`, `defer db.Close()`. Never hold the DB open across calls — every read opens its own read-only (`mode=ro`) connection via the DSN built in `buildSQLiteDSN`.
- Always filter `WHERE status != 'tombstone' AND (deleted_at IS NULL)` for issue queries.
- Wrap nullable TEXT columns in `COALESCE(col, '')` (see `loadBdIssues` / `brLoadIssues`).
- Use `?` placeholders + `QueryContext` args for any user value — never string-concatenate into SQL.
- Return `rows.Err()` at the end; `defer rows.Close()`.
- Read only columns that exist in BOTH schemas. br is a 43-column superset of bd; abacus reads only the shared subset. The `dependencies` table stores the relationship in a `type` column (abacus normalizes the bd `dep_type` / br `dependency_type` JSON naming difference at the SQL layer).
- Mirror the method in `bd_sqlite.go`. If br needs row-normalization (cf. `normalizeBrComment` / `scanBrComment`), put that ONLY in the br file; bd scans inline.

**Validation gate:** `go build ./internal/beads/` — SQLite-client errors clear.

### Step 2C — WRITE only: add delegation in `br_sqlite.go` and `bd_sqlite.go`

The SQLite clients implement `Writer` by delegating to the embedded `writer`. Add a one-line passthrough next to the other delegations (bottom of each file):

```go
func (c *brSQLiteClient) Assign(ctx context.Context, issueID, assignee string) error {
	return c.writer.Assign(ctx, issueID, assignee)
}
```

Add the identical line to `bdSQLiteClient`. (Reads skip this step — they have no CLI delegate.)

**Validation gate:** `go build ./internal/beads/` now succeeds with zero errors. Do not proceed until it does.

### Step 3 — Wire `MockClient` in `mock.go`

The mock must satisfy `Client`. For the new method add FOUR things, copying an existing method's quartet (e.g. `UpdateStatus` for a simple write, `Create` for one with a return value):
1. A `XFn func(context.Context, ...) (...)` field in the `MockClient` struct.
2. An `XCallCount int` field.
3. A `XCallArgs` recorder. Simple arg lists use `[][]string` (cf. `AddLabelCallArgs`); rich arg sets get a dedicated `XCallArg` struct (cf. `CreateCallArg`).
4. The method itself: lock `m.mu`, increment count, append args, unlock, then `if m.XFn == nil { <default> }` else call `m.XFn(...)`.

Default when `Fn` is nil: **writes** return `nil` (no-op); **reads** return `ErrMockNotImplemented`; `Create` returns `"ab-mock"`; `CreateFull` returns a populated mock `FullIssue`.

**Validation gate:** `go vet ./internal/beads/` and `go build ./...` pass — confirms `MockClient` still satisfies `Client` and no caller broke.

### Step 4 — Tests (write the FAILING test before Steps 2–3 per TDD; this step finalizes them)

- **Write unit test** in `br_cli_test.go` (and `bd_cli_test.go`): use the `writeTestScript` helper to drop a fake `#!/bin/sh` binary that logs `"$@"` to a file, then `NewBrCLIClient(WithBrBinaryPath(script))`, call your method, and assert the logged args contain the expected subcommand (cf. `TestBdCLIClient_UpdatePriority` asserting `"update ab-prio --priority=3"`).
- **Read unit test** in `br_sqlite_test.go` (and `bd_sqlite_test.go`): use `testBrDB(t)` + `seedTestData(t, dbPath)`, construct `NewBrSQLiteClient(dbPath)`, call your method, assert returned fields (cf. `TestBrSQLiteClient_Export`). If you read a column not in the test schema, add it to `createTestBrDB`.
- **Mock test** in `mock_test.go`: assert `XCallCount`/`XCallArgs` are recorded and that setting `XFn` is invoked (cf. `TestMockClient_UpdatePriority_*`).
- **Conformance test** in `conformance_test.go` (file is `//go:build integration`): if the operation must agree across backends, add a test that skips when `exec.LookPath("br")`/`"bd"` fails, performs the op against both via `setupBackendTestDB(t, "br"|"bd")`, and compares JSON fields or DB state (cf. `TestDependencyTypeConformance`).

**Validation gate:** `make check-test` (unit + lint) passes, then `make test-integration` passes (requires `bd` and `br` on PATH). If integration binaries are absent, state that conformance was NOT run live and name the command the user must run.

### Step 5 — Close out

Run `ubs $(git diff --name-only)` (exit 0), commit only the files you changed plus `.beads/`, push, and confirm the GitHub build before closing the bead.

## Examples

**User says:** "Add a backend method to assign an issue to someone."

**Actions taken:**
1. Classify: mutation → **Writer**.
2. `client.go`: add `Assign(ctx context.Context, issueID, assignee string) error` to `Writer`. `go build` fails: `*brCLIClient does not implement Writer (missing Assign)`.
3. Write failing test `TestBrCLIClient_Assign` (fake-binary, asserts logged `update ab-x --assignee bob`). Red.
4. Implement `Assign` in `br_cli.go` (`c.run(ctx, "update", issueID, "--assignee", assignee)`), mirror in `bd_cli.go`. Test green.
5. Add delegation `return c.writer.Assign(...)` in `br_sqlite.go` and `bd_sqlite.go`.
6. `mock.go`: add `AssignFn`, `AssignCallCount`, `AssignCallArgs [][]string`, and the `Assign` method (default no-op).
7. `make check-test` green → conformance test optional (assign is symmetric) → `make test-integration`.

**Result:** `Client.Assign` works on both backends; UI can call `client.Assign(ctx, id, who)`; mock records the call; build green.

**User says:** "Read a new field — expose each issue's `close_reason`."

**Actions taken:** `close_reason` is already scanned in `brLoadIssues`/`loadBdIssues` and lives on `FullIssue.CloseReason` — no backend method needed; surface the existing field in the UI. (Demonstrates: check `types.go` + the `*LoadIssues` SELECT before adding anything.)

## Common Issues

- **`cannot use ... (*brSQLiteClient) as beads.Client value ... missing method X`** — you added X to the interface but not to every implementor. Writes need X in `bd_cli.go`, `br_cli.go`, AND delegations in both `*_sqlite.go`; reads need X in both `*_sqlite.go`; the mock always needs X. Run `go build ./internal/beads/` and fix each named type.
- **`*beads.MockClient does not implement beads.Client`** — Step 3 incomplete. Add the method to `mock.go` (a struct field alone is not enough).
- **CLI test flakes with `fork/exec ... : no such file or directory` or `permission denied`** — always create the fake binary via `writeTestScript` (it `chmod`s 0755, fsyncs the file + parent dir, and sleeps 10ms for CI). Do not hand-roll `os.WriteFile`.
- **`no JSON found in br create output`** — the CLI printed warnings before JSON, or you forgot `--json`. Parse with the shared `extractJSON(out)` (string-aware brace scanner), never `json.Unmarshal(out, ...)` on raw output.
- **Read returns empty though rows exist** — your `WHERE` is missing `status != 'tombstone' AND (deleted_at IS NULL)`, or the test DB lacks the column. Add the column to `createTestBrDB` and re-seed.
- **br write fails with `NOT_INITIALIZED` / workspace not found** — br discovers its workspace by walking up from cwd for `.beads/`. `NewBrSQLiteClient` derives `WithBrWorkDir` from the db path via `deriveWorkDirFromDBPath`; for a `Writer`-only client set `WithBrWorkDir(dir)` explicitly.
- **Conformance test fails on dependency type** — br emits `dependency_type`, bd emits `dep_type` in JSON; the SQLite layer reads the `type` column instead. Assert on DB state or the normalized `Dependency.Type`, not on raw backend JSON key names.
- **`make test-integration` skips everything** — `bd` and/or `br` not on PATH. Install both binaries; conformance tests `t.Skip` when `exec.LookPath` misses either, so a 'pass' with skips is NOT verified parity — say so.