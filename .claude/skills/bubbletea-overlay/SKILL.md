---
name: bubbletea-overlay
description: Scaffolds a new Bubble Tea modal overlay in internal/ui/ following the overlay_*.go pattern: a pointer-receiver struct with Init/Update/View/Layer, paired XxxChangedMsg/XxxCancelledMsg message types, an OverlayBuilder-rendered View, footer hints, an OverlayType enum entry, a KeyMap binding, and full wiring across app.go, keys.go, update_keys.go, update_overlay.go, update_commands.go, update_messages.go, and footer.go. Use when the user says 'new overlay', 'add a modal', 'add a picker', 'selection overlay', or 'confirm dialog' for the abacus TUI. Do NOT use for editing the tree/detail panes, the status bar, toasts, or any non-modal UI.
paths:
  - internal/ui/overlay_*.go
  - internal/ui/update_*.go
  - internal/ui/app.go
  - internal/ui/keys.go
  - internal/ui/footer.go
---
# bubbletea-overlay

Scaffold a new modal overlay in `internal/ui/`. Overlays are self-contained `tea.Model`-style components (`*XxxOverlay`) that the `App` model holds, routes keys to, and reacts to via message types. The canonical references are `internal/ui/overlay_priority.go` (selection picker) and `internal/ui/overlay_delete.go` (confirm dialog).

## Critical

- **TDD is mandatory.** Write `internal/ui/overlay_<name>_test.go` with a failing test FIRST (red), then implement (green), then refactor. Never write overlay code before a failing test. See AGENTS.md.
- **An overlay is NOT done until 9 wiring points exist.** A struct alone does nothing — the overlay never opens, never receives keys, and never closes unless you wire all of: enum, model field, key binding, open handler, key dispatch, key delegation, message handler, footer hints, async command. Missing any one is a silent bug.
- **Overlay `Update` returns `(*XxxOverlay, tea.Cmd)`** — pointer receiver, concrete pointer type (NOT `tea.Model`). Side effects (the actual `br` mutation) are emitted as a `tea.Cmd` returning `XxxChangedMsg`, never performed inline.
- **Closing the overlay is the App's job, not the overlay's.** The overlay emits `XxxChangedMsg`/`XxxCancelledMsg`; `handleOverlayMsg` in `update_overlay.go` sets `m.activeOverlay = OverlayNone` and nils the field.
- **Never exceed Go size limits** (AGENTS.md): file ≤500 lines, function ≤60 lines, complexity ≤10. If `View` grows, extract `render*` helpers like `overlay_delete.go` does.
- Run `make check-test` before claiming done. Verify visually with `scripts/tui-test.sh` (see step 11).

## Instructions

### Step 1 — Write the failing test
Create `internal/ui/overlay_<name>_test.go`. Mirror `overlay_priority_test.go`. At minimum assert: constructor sets initial state; pressing `enter`/the confirm key returns a cmd whose msg is `XxxChangedMsg` with correct fields; pressing `esc` returns `XxxCancelledMsg`; `View()` contains the issue ID. Run `make test VERBOSE=1` and confirm it FAILS to compile/pass.
Verify: test exists and fails before proceeding to Step 2.

### Step 2 — Create the overlay file
Create `internal/ui/overlay_<name>.go`. Use `overlay_priority.go` as the template verbatim. Required shape:

```go
package ui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type XxxOverlay struct {
	issueID    string
	issueTitle string
	// ...selection/input state
}

// XxxChangedMsg is emitted when the user confirms.
type XxxChangedMsg struct {
	IssueID string
	NewXxx  string // the chosen value
}

// XxxCancelledMsg is emitted when the user dismisses.
type XxxCancelledMsg struct{}

func NewXxxOverlay(issueID, issueTitle string /*, current X */) *XxxOverlay {
	return &XxxOverlay{issueID: issueID, issueTitle: issueTitle}
}

func (m *XxxOverlay) Init() tea.Cmd { return nil }

func (m *XxxOverlay) Update(msg tea.Msg) (*XxxOverlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
			// move selection
		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
			// move selection
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return m, m.confirm()
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			return m, func() tea.Msg { return XxxCancelledMsg{} }
		}
	}
	return m, nil
}

func (m *XxxOverlay) confirm() tea.Cmd {
	issueID := m.issueID
	newXxx := m.selectedValue()
	return func() tea.Msg { return XxxChangedMsg{IssueID: issueID, NewXxx: newXxx} }
}

func (m *XxxOverlay) View() string {
	b := NewOverlayBuilder(OverlaySizeNarrow, 0) // Narrow=24 picker, Standard=48 dialog, Wide=64 form
	header := styleID().Render(m.issueID) + styleStatsDim().Render(" › ") + styleStatsDim().Render("Xxx")
	b.Line(header)
	b.Line(b.Divider())
	// ...render options with styleStatusSelected()/styleStatusOption()
	return b.Build() // use b.BuildDanger() for destructive dialogs
}

func (m *XxxOverlay) Layer(width, height, topMargin, bottomMargin int) Layer {
	return BaseOverlayLayer(m.View, width, height, topMargin, bottomMargin)
}
```

Never hand-roll widths or styles — `OverlayBuilder` (`overlay_base.go`) owns sizing; `styleOverlay*`/`styleStatus*`/`styleID` own colors. `Layer` is always one line delegating to `BaseOverlayLayer`.
Verify: `make build` compiles the new file (wiring errors come next).

### Step 3 — Register the OverlayType enum (`internal/ui/app.go`)
Add a constant to the `iota` block (~line 37):
```go
const (
	OverlayNone OverlayType = iota
	OverlayStatus
	// ...existing...
	OverlayColumns
	OverlayXxx // <-- add at end
)
```
Verify: `OverlayXxx` resolves.

### Step 4 — Add the model field (`internal/ui/app.go`)
In the `App` struct, next to the other overlay pointers (~line 184):
```go
xxxOverlay *XxxOverlay
```
Uses the type from Step 2. Verify: field compiles.

### Step 5 — Add the key binding (`internal/ui/keys.go`)
Add a `Xxx key.Binding` field to the `KeyMap` struct, then register it in the constructor (~line 143) following `Priority`:
```go
Xxx: key.NewBinding(
	key.WithKeys("x"),
	key.WithHelp("x", "Change xxx"),
),
```
Pick a key not already used in `keys.go`. Verify: no duplicate-key collision (grep existing `WithKeys`).

### Step 6 — Add the open handler (`internal/ui/update_keys.go`)
Follow `handlePriorityKey` (~line 336). It reads the current row, constructs the overlay, sets `activeOverlay`:
```go
func (m *App) handleXxxKey() (tea.Model, tea.Cmd) {
	if len(m.visibleRows) > 0 {
		row := m.visibleRows[m.cursor]
		m.xxxOverlay = NewXxxOverlay(row.Node.Issue.ID, row.Node.Issue.Title /*, row.Node.Issue.Xxx */)
		m.activeOverlay = OverlayXxx
	}
	return m, nil
}
```
If the overlay needs async init data (like labels), `return m, m.xxxOverlay.Init()`.
Verify: uses the field (Step 4), enum (Step 3), constructor (Step 2).

### Step 7 — Dispatch the key (`internal/ui/update_keys.go`)
In `handleGlobalKey`'s switch (~line 210), add:
```go
case key.Matches(msg, m.keys.Xxx):
	return m.handleXxxKey()
```
Uses the binding (Step 5) and handler (Step 6). Verify: pressing the key opens the overlay (defer live check to Step 11).

### Step 8 — Delegate keys to the open overlay (`internal/ui/update_keys.go`)
In `delegateToOverlay` (~line 85), add a block so keystrokes reach the overlay while it is active:
```go
if m.activeOverlay == OverlayXxx && m.xxxOverlay != nil {
	m.xxxOverlay, cmd = m.xxxOverlay.Update(msg)
	return cmd, true
}
```
This is why `Update` must return `(*XxxOverlay, tea.Cmd)`. Verify: the reassignment `m.xxxOverlay, cmd = ...` type-checks.

### Step 9 — Handle the result messages (`internal/ui/update_overlay.go`)
In `handleOverlayMsg`'s type switch (follow the `PriorityChangedMsg` block ~line 291), add three cases:
```go
case XxxChangedMsg:
	m.activeOverlay = OverlayNone
	m.xxxOverlay = nil
	m.displayXxxToast(msg.IssueID, msg.NewXxx) // optional toast
	return m, tea.Batch(m.executeXxxChangeCmd(msg.IssueID, msg.NewXxx), scheduleXxxToastTick()), true

case XxxCancelledMsg:
	m.activeOverlay = OverlayNone
	m.xxxOverlay = nil
	return m, nil, true

case xxxUpdateCompleteMsg:
	if msg.err != nil {
		m.lastError = msg.err.Error()
		m.lastErrorSource = errorSourceOperation
		m.showErrorToast = true
		m.errorToastStart = time.Now()
		return m, scheduleErrorToastTick(), true
	}
	return m, m.forceRefresh(), true
```
This is the ONLY place the overlay is closed. Uses messages from Step 2 and the cmd/msg from Step 10.
Verify: all three cases present; closing nils the field.

### Step 10 — Add the async command + complete message
In `internal/ui/update_messages.go`, add (follow `priorityUpdateCompleteMsg` ~line 47):
```go
type xxxUpdateCompleteMsg struct {
	issueID string
	err     error
}
```
In `internal/ui/update_commands.go`, add (follow `executePriorityChangeCmd` ~line 302):
```go
func (m *App) executeXxxChangeCmd(issueID, newXxx string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), statusCommandTimeout)
		defer cancel()
		err := m.client.UpdateXxx(ctx, issueID, newXxx)
		return xxxUpdateCompleteMsg{issueID: issueID, err: err}
	}
}
```
The `m.client.UpdateXxx` call must already exist on the beads client; if not, that is a separate bead — do not invent it here. Verify: command returns the message from this step.

### Step 11 — Footer hints (`internal/ui/footer.go`)
Define the hints var next to `priorityOverlayFooterHints` (~line 60):
```go
var xxxOverlayFooterHints = []footerHint{
	{"⏎", "Save"},
	{"esc", "Cancel"},
}
```
Add a case in `renderFooter`'s `switch m.activeOverlay` (~line 81):
```go
case OverlayXxx:
	hints = xxxOverlayFooterHints
```
Verify (live): `make build && ./scripts/tui-test.sh start`, press your key, `./scripts/tui-test.sh view`, confirm overlay renders centered with footer hints, then `keys 'esc'` closes it, then `quit`.

### Step 12 — Mouse routing (only if clickable)
If the overlay responds to clicks, add cases in `internal/ui/mouse.go` mirroring `OverlayPriority` (~lines 111, 342). Skip for keyboard-only overlays.

## Examples

**User says:** "Add an assignee picker overlay — press `a` to set the assignee on the selected bead."

**Actions taken:**
1. Write `overlay_assignee_test.go`: assert `enter` emits `AssigneeChangedMsg{IssueID, NewAssignee}`, `esc` emits `AssigneeCancelledMsg`. Run `make test` → red.
2. Create `overlay_assignee.go` with `AssigneeOverlay` struct, `AssigneeChangedMsg`/`AssigneeCancelledMsg`, `NewAssigneeOverlay`, `Init/Update/View/Layer`, `confirm()`; `View` uses `NewOverlayBuilder(OverlaySizeNarrow, 0)`.
3. `app.go`: add `OverlayAssignee` to enum; add `assigneeOverlay *AssigneeOverlay` field.
4. `keys.go`: add `Assignee` binding on key `a`.
5. `update_keys.go`: add `handleAssigneeKey`, dispatch case `m.keys.Assignee`, delegation block.
6. `update_overlay.go`: `AssigneeChangedMsg`/`AssigneeCancelledMsg`/`assigneeUpdateCompleteMsg` cases.
7. `update_messages.go` + `update_commands.go`: `assigneeUpdateCompleteMsg` + `executeAssigneeChangeCmd` calling `m.client.UpdateAssignee`.
8. `footer.go`: `assigneeOverlayFooterHints` + `case OverlayAssignee`.
9. `make check-test` green; `scripts/tui-test.sh` shows the picker opening on `a` and closing on `esc`.

**Result:** Pressing `a` opens a narrow centered picker styled identically to the priority overlay; selecting and pressing enter updates the bead via `br` and shows a toast; the index/footer match existing overlays.

## Common Issues

- **Overlay opens but ignores all keys / only `esc` (from global) works.** You skipped Step 8. Without the `delegateToOverlay` block, `Update` is never called. Add the `if m.activeOverlay == OverlayXxx && m.xxxOverlay != nil { m.xxxOverlay, cmd = m.xxxOverlay.Update(msg); return cmd, true }` block.
- **Overlay never closes after confirm/cancel.** You skipped Step 9, or the overlay's `Update` sets `m.activeOverlay` itself. Overlays must NOT touch `App` state — they emit messages; only `handleOverlayMsg` sets `OverlayNone` and nils the field.
- **`cannot use m.xxxOverlay.Update(msg) (value of type tea.Model)`** — your `Update` signature returns `(tea.Model, tea.Cmd)`. It must return `(*XxxOverlay, tea.Cmd)` (concrete pointer), like `overlay_priority.go`.
- **`m.keys.Xxx undefined`** — you added the constructor entry (Step 5) but not the `Xxx key.Binding` field on the `KeyMap` struct. Add both.
- **Two overlays open on one keypress, or the wrong overlay opens.** Duplicate key binding. Run `grep -n 'WithKeys(' internal/ui/keys.go` and pick an unused key.
- **Divider doesn't span the box / content overflows the border.** You bypassed `OverlayBuilder` or hardcoded a width. Always size via `NewOverlayBuilder(size, termWidth)` and use `b.ContentWidth()` for inner text; never call `lipgloss.Width` math yourself (see the warning comment atop `overlay_base.go`).
- **`undefined: m.client.UpdateXxx`** — the beads client has no such method. Beads (`br`) is third-party and read-only here. File a separate bead for the client method; do not add it in this overlay.
- **`make check` fails: function too long / file too long.** `View` exceeded 60 lines or the file passed 500. Extract `render*` helpers returning `[]string` and append with `b.Lines(...)`, as `overlay_delete.go` does with `renderDangerBlock`.