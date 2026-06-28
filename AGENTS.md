# Agent Guidelines — abacus

Terminal UI (Go + Bubble Tea) for browsing Beads issue DBs. Entry: `cmd/abacus/main.go`. **Keep this file concise.**

## Hard Rules
- NEVER delete files/dirs without an explicit in-session command. Ask first.
- FORBIDDEN unless approved verbatim this session: `git reset --hard`, `git clean -fd`, `rm -rf`, any overwrite of code/data.
- No bulk codemods or `sed`/regex refactors — edit file-by-file, review diffs.
- Optimize clean architecture over back-compat: migrate callers, delete old code, no v2 clones. Bar for new files is high.

## Commands
```bash
make build              # compile -> ./abacus (ldflags inject version/build)
make check              # go fmt + go vet + golangci-lint (.golangci.yml)
make test               # unit tests only (-short)
make test-integration   # needs bd/br binaries (//go:build integration)
make check-test         # check + unit tests
make ci                 # lint + test + integration + build (pre-commit gate)
./abacus                # run against ./.beads DB; status bar shows [bd]/[br]
```
Append `VERBOSE=1` to any target to surface failures.

## Architecture
- **CLI** `cmd/abacus/`: `main.go` flags -> `startup.go` spinner -> `version_gate.go`. Component demos in `cmd/combobox-demo/`, `cmd/chips-demo/`, `cmd/chipcombobox-demo/`.
- **TUI** `internal/ui/`: Bubble Tea Model/Update/View. `app.go` state; `update_keys.go`/`update_overlay.go` input; `view.go`/`tree.go`/`detail.go` render; `overlay_*.go` modals; `footer.go`/`help.go` chrome; `chips.go`/`combobox.go`/`chipcombobox.go` inputs; `cell_canvas.go`/`surface.go`/`layer.go` compositor.
- **Themes** `internal/ui/theme/`: 20+ themes; `manager.go` registry, `theme.go` interface.
- **Backends** `internal/beads/`: `client.go` = `Reader`+`Writer`; SQLite reads (`bd_sqlite.go`/`br_sqlite.go`) + CLI writes (`bd_cli.go`/`br_cli.go`); `backend.go` autodetect; `mock.go` + `conformance_test.go`.
- **Graph** `internal/graph/builder.go` (deps -> tree, cycle check). **Domain** `internal/domain/` (status/priority/transitions). **Config** `internal/config/` (Viper: `~/.abacus/`, `.abacus/`). Also `internal/update/`, `internal/debug/`, `internal/errors/`.
- Reference docs: `CONTEXT.md` (vocabulary), `docs/UI_PRINCIPLES.md`, `docs/BEAD_MODEL.md`, `docs/spec.md`, `docs/adr/`.

## Conventions
- TDD: write the failing test first. Code must build, lint, and pass tests before a bead closes.
- Go limits: file <=500 (test 800), func <=60 (test 80), <=40 stmts, <=120 cols, complexity <=10. Split via `_view.go`/`_handlers.go`/`_types.go`.
- New overlay -> follow `overlay_*.go`: `Init`/`Update`/`View`/`Layer` + `XxxChangedMsg`/`XxxCancelledMsg`, register in `OverlayType` (`app.go`), wire in `update_keys.go`.

## TUI visual testing
```bash
make build
./scripts/tui-test.sh start
./scripts/tui-test.sh keys 'jjjl'   # j=down, l=expand
./scripts/tui-test.sh view
./scripts/tui-test.sh quit
```
Golden snapshots live in `testdata/ui/golden/`; refresh intentionally with `go test ./internal/ui -run TestOverlayAndToastGoldenSnapshots -update-golden`.

## Issue tracking (Beads / `br`)
```bash
br create ...; br sync --flush-only         # simple create
br show <id>; br comments <id>              # read first; set status in_progress
br label add|remove <id> <label>
br dep add <child> <parent> --type parent-child
```
Standard IDs (`ab-xyz`), no dotted children. Commit `.beads/issues.jsonl` with related code. Test beads descend from `ab-cj3`. See `docs/agents/issue-tracker.md`, `docs/agents/triage-labels.md`. `br`/`bd` are third-party (`../beads`, `../beads_rust`) — never modify.

## MCP Agent Mail
For parallel multi-agent runs, coordinate via MCP Agent Mail: `register_agent` (project_key = repo abs path), `file_reservation_paths` before editing, `send_message`/`fetch_inbox` per thread. Set `AGENT_NAME` so the pre-commit guard blocks conflicting commits.

## Landing the plane
Not done until pushed: `git pull --rebase` -> `br sync` -> `git push` -> confirm `git status` clean. Stage only files you changed (`git add <files>`); run `make ci` first.

<!-- caliber:managed:pre-commit -->
## Before Committing

**IMPORTANT:** Before every git commit, you MUST ensure Caliber syncs agent configs with the latest code changes.

First, check if the pre-commit hook is already installed:
```bash
grep -q "caliber" .git/hooks/pre-commit 2>/dev/null && echo "hook-active" || echo "no-hook"
```

- If **hook-active**: the hook handles sync automatically — just commit normally. Tell the user: "Caliber will sync your agent configs automatically via the pre-commit hook."
- If **no-hook**: run Caliber manually before committing:
  1. Tell the user: "Caliber: Syncing agent configs with your latest changes..."
  2. Run: `caliber refresh && git add CLAUDE.md .claude/ .cursor/ .cursorrules .github/copilot-instructions.md .github/instructions/ AGENTS.md CALIBER_LEARNINGS.md .agents/ .opencode/ 2>/dev/null`
  3. After it completes, briefly tell the user what Caliber updated. Then proceed with the commit.

**Valid `caliber refresh` options:** `--quiet` (suppress output) and `--dry-run` (preview without writing). Do not pass any other flags — options like `--auto-approve`, `--debug`, or `--force` do not exist and will cause errors.

**`caliber config`** takes no flags — it runs an interactive provider setup. Do not pass `--provider`, `--api-key`, or `--endpoint`.

If `caliber` is not found, read `.agents/skills/setup-caliber/SKILL.md` and follow its instructions to install Caliber.
<!-- /caliber:managed:pre-commit -->

<!-- caliber:managed:learnings -->
## Session Learnings

Read `CALIBER_LEARNINGS.md` for patterns and anti-patterns learned from previous sessions.
These are auto-extracted from real tool usage — treat them as project-specific rules.
<!-- /caliber:managed:learnings -->

<!-- caliber:managed:model-config -->
## Model Configuration

Recommended default: `claude-sonnet-4-6` with high effort (stronger reasoning; higher cost and latency than smaller models).
Smaller/faster models trade quality for speed and cost — pick what fits the task.
Pin your choice (`/model` in Claude Code, or `CALIBER_MODEL` when using Caliber with an API provider) so upstream default changes do not silently change behavior.

<!-- /caliber:managed:model-config -->

<!-- caliber:managed:sync -->
## Context Sync

This project uses [Caliber](https://github.com/caliber-ai-org/ai-setup) to keep AI agent configs in sync across Claude Code, Cursor, Copilot, and Codex.
Configs update automatically before each commit via `caliber refresh`.
If the pre-commit hook is not set up, read `.agents/skills/setup-caliber/SKILL.md` and follow the setup instructions.
<!-- /caliber:managed:sync -->
