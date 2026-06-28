---
name: bubbletea-overlay
description: Scaffolds a new Bubble Tea modal overlay in internal/ui/ following the overlay_*.go pattern: a struct with Init/Update/View/Layer, XxxChangedMsg/XxxCancelledMsg message types built with NewOverlayBuilder, a footerHint slice, an OverlayType enum entry, and the full wiring across app.go, keys.go, update_keys.go, update_overlay.go, footer.go, and view.go. Use when the user says 'new overlay', 'add a modal', 'add a picker', 'confirm dialog', or 'add a selection popup' to the TUI. Do NOT use for editing the tree pane, detail pane, toasts, or any non-modal UI — those do not go through the OverlayType/Layer system.
---
# Bubble Tea Overlay

Scaffold a new modal overlay in `internal/ui/` that matches the existing `overlay_*.go` pattern (e.g. `overlay_priority.go`, `overlay_delete.go`). An overlay is a self-contained struct rendered as a centered `Layer`, driven by `XxxChangedMsg`/`XxxCancelledMsg` messages, and wired into the `App` at 7 fixed touchpoints.

## Critical

- **TDD is mandatory (AGENTS.md).** Write `overlay_<name>_test.go` FIRST with a failing test (mirror `overlay_priority_test.go`: construct via `NewXxxOverlay(...)`, assert initial state, feed `tea.KeyMsg` to `Update`, assert the returned `tea.Cmd` produces the expected `XxxChangedMsg`). Only then write production code. `make check-test` must pass before the bead is closed.
- **Never compute widths by hand.** Always build View content with `NewOverlayBuilder(size, 0)` from `overlay_base.go`. Manual `strings.Repeat` / lipgloss `.Width()` math produces mis-sized dividers (see the warning comment at the top of `overlay_base.go`).
- **Never copy a `Layer()` body.** Always `return BaseOverlayLayer(m.View, width, height, topMargin, bottomMargin)`.
- **All 7 wiring touchpoints are required.** Miss one and the overlay silently fails (won't open, won't take keys, renders blank, or shows the wrong footer). The Common Issues section maps each symptom to the missing touchpoint.
- File ≤500 lines, function ≤60 lines, cyclomatic ≤10 (AGENTS.md). Split into `overlay_<name>_view.go` / `_handlers.go` / `_types.go` if you exceed this.
- Do not delete or rename existing overlays. New file only.

## Instructions

Replace `Xxx`/`xxx`/`<name>` with your overlay name (e.g. `Assignee`, `assignee`, `assignee`). The running example below is a single-select picker like `PriorityOverlay`.

### Step 1 — Failing test

Create `internal/ui/overlay_<name>_test.go` (`package ui`). Mirror `overlay_priority_test.go`:
```go
func TestNewXxxOverlay(t *testing.T) {
	o := NewXxxOverlay("ab-123", "Title", current)
	if o.selected != wantIdx { t.Errorf("got %d want %d", o.selected, wantIdx) }
}
func TestXxxOverlayEnterEmitsChanged(t *testing.T) {
	o := NewXxxOverlay("ab-123", "Title", current)
	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := cmd().(XxxChangedMsg); !ok { t.Fatal("expected XxxChangedMsg") }
}
```
**Gate:** `make test VERBOSE=1` shows these tests FAIL to compile/run (red). Do not proceed until red.

### Step 2 — Overlay file

Create `internal/ui/overlay_<name>.go`. Copy the shape of `overlay_priority.go` exactly:
```go
package ui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type XxxOverlay struct {
	issueID    string
	issueTitle string
	selected   int
	options    []xxxOption // omit for a confirm dialog (see overlay_delete.go)
}

type XxxChangedMsg struct {
	IssueID string
	NewXxx  string
}
type XxxCancelledMsg struct{}

func NewXxxOverlay(issueID, issueTitle string, current string) *XxxOverlay {
	return &XxxOverlay{issueID: issueID, issueTitle: issueTitle /* , selected, options */}
}

func (m *XxxOverlay) Init() tea.Cmd { return nil }

func (m *XxxOverlay) Update(msg tea.Msg) (*XxxOverlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
			m.selected = (m.selected + 1) % len(m.options)
		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
			m.selected = (m.selected - 1 + len(m.options)) % len(m.options)
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return m, m.confirm()
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			return m, func() tea.Msg { return XxxCancelledMsg{} }
		}
	}
	return m, nil
}

func (m *XxxOverlay) confirm() tea.Cmd {
	id, val := m.issueID, m.options[m.selected].value
	return func() tea.Msg { return XxxChangedMsg{IssueID: id, NewXxx: val} }
}

func (m *XxxOverlay) View() string {
	b := NewOverlayBuilder(OverlaySizeNarrow, 0) // Narrow|Standard|Wide
	b.HeaderWithContext("Xxx", m.issueID, m.issueTitle)
	for i, opt := range m.options {
		line := "  " + opt.label
		if i == m.selected {
			line = styleStatusSelected().Render(line + "  ←")
		} else {
			line = styleStatusOption().Render(line)
		}
		b.Line(line)
	}
	return b.Build() // use b.BuildDanger() for destructive dialogs
}

func (m *XxxOverlay) Layer(width, height, topMargin, bottomMargin int) Layer {
	return BaseOverlayLayer(m.View, width, height, topMargin, bottomMargin)
}
```
Rules: `confirm()` captures fields into locals BEFORE returning the closure (overlay is nil'd before the cmd runs). Return `tea.Cmd` closures for ALL outgoing messages — never mutate `App` from inside the overlay. For a confirm/danger dialog, drop `options`/`selected`, use `b.BuildDanger()`, add an in-View `b.Footer(m.footerHints())` (see `overlay_delete.go`), and bind a single confirm key + `c/esc` to cancel.
**Gate:** `make build` compiles. Step 1 tests now pass (green). Run `make test VERBOSE=1`.

### Step 3 — Register OverlayType + struct field (`app.go`)

Uses output from Step 2 (the `*XxxOverlay` type). In the `OverlayType` const block (currently ends with `OverlayColumns`), append:
```go
	OverlayXxx
```
In the `App` struct, next to `priorityOverlay *PriorityOverlay`, add:
```go
	xxxOverlay *XxxOverlay
```
**Gate:** `make build` — `undefined: OverlayXxx` must be gone.

### Step 4 — Keybinding (`keys.go`)

Add a field to the keymap struct (near `Priority key.Binding`):
```go
	Xxx key.Binding
```
And in the `key.NewBinding` block (near `Priority:`):
```go
	Xxx: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "Change xxx"),
	),
```
Pick an unused key — grep `key.WithKeys` in `keys.go` to confirm no collision.
**Gate:** `make build` passes.

### Step 5 — Key dispatch + opener (`update_keys.go`)

Three edits, all mirroring `Priority`:
1. In `delegateToOverlay`, add a block (forwards keys while the overlay is open):
```go
	if m.activeOverlay == OverlayXxx && m.xxxOverlay != nil {
		m.xxxOverlay, cmd = m.xxxOverlay.Update(msg)
		return cmd, true
	}
```
2. In `handleGlobalKey`'s `switch`, add a case:
```go
	case key.Matches(msg, m.keys.Xxx):
		return m.handleXxxKey()
```
3. Add the opener (mirror `handlePriorityKey`):
```go
func (m *App) handleXxxKey() (tea.Model, tea.Cmd) {
	if len(m.visibleRows) > 0 {
		row := m.visibleRows[m.cursor]
		m.xxxOverlay = NewXxxOverlay(row.Node.Issue.ID, row.Node.Issue.Title, /* current field */)
		m.activeOverlay = OverlayXxx
	}
	return m, nil
}
```
**Gate:** `make build` passes. (Pressing the key now opens the overlay but it won't render or close yet.)

### Step 6 — Message handlers (`update_overlay.go`)

In `handleOverlayMsg`'s type switch, add (mirror the `PriorityChangedMsg`/`PriorityCancelledMsg` cases). This is what closes the overlay and triggers the side-effect:
```go
	case XxxChangedMsg:
		m.activeOverlay = OverlayNone
		m.xxxOverlay = nil
		return m, m.executeXxxChangeCmd(msg.IssueID, msg.NewXxx), true

	case XxxCancelledMsg:
		m.activeOverlay = OverlayNone
		m.xxxOverlay = nil
		return m, nil, true
```
`executeXxxChangeCmd` is the `br`-calling command (see `executePriorityChangeCmd` in `update_commands.go` for the shape: returns a `tea.Cmd` that runs the backend update and emits a `xxxUpdateCompleteMsg`). Skip it for pure-UI overlays.
**Gate:** `make build` passes. `esc` now closes the overlay; `enter` fires the change.

### Step 7 — Footer hints (`footer.go`)

Add a package-level var (mirror `priorityOverlayFooterHints`):
```go
var xxxOverlayFooterHints = []footerHint{
	{"⏎", "Save"},
	{"esc", "Cancel"},
}
```
And a case in `renderFooter`'s `switch m.activeOverlay`:
```go
	case OverlayXxx:
		hints = xxxOverlayFooterHints
```
**Gate:** `make build`. Without this the footer falls through to global hints.

### Step 8 — Render the Layer (`view.go`)

Add an `else if` branch in the overlay-layer chain (after the `OverlayPriority` branch):
```go
	} else if m.activeOverlay == OverlayXxx && m.xxxOverlay != nil {
		if layer := m.xxxOverlay.Layer(m.width, m.height, headerHeight, bottomMargin); layer != nil {
			overlayLayers = append(overlayLayers, layer)
		}
	}
```
**Gate:** Overlay now actually draws.

### Step 9 — (Optional) mouse routing (`mouse.go`)

Only if the overlay needs click/scroll: add the parallel `case m.activeOverlay == OverlayXxx && m.xxxOverlay != nil:` branches alongside the `OverlayPriority` ones. Keyboard-only overlays skip this.

### Step 10 — Verify end-to-end

```bash
make check-test          # lint + unit tests green
ubs $(git diff --name-only) # bug scan, exit 0
make build
./scripts/tui-test.sh start
./scripts/tui-test.sh keys 'x'   # your hotkey
./scripts/tui-test.sh view       # confirm overlay renders centered
./scripts/tui-test.sh keys $'\e' # esc closes
./scripts/tui-test.sh quit
```
**Gate:** Overlay opens on the hotkey, renders centered with the right footer, `esc` closes it, `enter` applies the change. Per AGENTS.md you must observe this live before claiming done — green tests alone are not sufficient.

## Examples

**User says:** "Add an overlay to set the assignee on the selected bead, opened with `a`."

**Actions taken:**
1. `overlay_assignee_test.go` — failing tests: `NewAssigneeOverlay` pre-selects current assignee; `enter` emits `AssigneeChangedMsg`. (`make test` red.)
2. `overlay_assignee.go` — `AssigneeOverlay` struct (`issueID`, `issueTitle`, `selected`, `options []assigneeOption`), `AssigneeChangedMsg{IssueID, NewAssignee}`, `AssigneeCancelledMsg{}`, `NewAssigneeOverlay`, `Init/Update/View/Layer`. View uses `NewOverlayBuilder(OverlaySizeNarrow, 0)` + `HeaderWithContext` + `styleStatusSelected/styleStatusOption`. (tests green.)
3. `app.go` — `OverlayAssignee` in the enum; `assigneeOverlay *AssigneeOverlay` field.
4. `keys.go` — `Assignee key.Binding` + `key.WithKeys("a")`.
5. `update_keys.go` — `delegateToOverlay` block, `case key.Matches(msg, m.keys.Assignee)`, `handleAssigneeKey`.
6. `update_overlay.go` — `AssigneeChangedMsg` / `AssigneeCancelledMsg` cases; `executeAssigneeChangeCmd` calls `br update --assignee`.
7. `footer.go` — `assigneeOverlayFooterHints` + `case OverlayAssignee`.
8. `view.go` — `else if m.activeOverlay == OverlayAssignee` layer branch.

**Result:** Pressing `a` on a bead opens a centered picker; `j/k` move, `enter` saves via `br` and shows a toast, `esc` cancels. `make check-test` green; verified live with `scripts/tui-test.sh`.

## Common Issues

- **`undefined: OverlayXxx` (build fails):** Step 3 skipped — add the entry to the `OverlayType` const block in `app.go`.
- **`undefined: m.xxxOverlay`:** Struct field missing — add `xxxOverlay *XxxOverlay` to the `App` struct in `app.go` (Step 3).
- **Hotkey does nothing:** Either no `case key.Matches(msg, m.keys.Xxx)` in `handleGlobalKey` (Step 5.2) or the binding is missing in `keys.go` (Step 4). Also check the key isn't already bound — grep `key.WithKeys` in `keys.go`.
- **Overlay opens but ignores j/k/enter/esc:** The `delegateToOverlay` block is missing (Step 5.1). Keys are only forwarded to the overlay through that function.
- **Screen goes blank / overlay never appears:** Missing `else if` branch in `view.go` (Step 8). The `activeOverlay` is set but no `Layer` is collected, so nothing draws.
- **Footer shows global hints instead of overlay hints:** Missing `case OverlayXxx` in `renderFooter` (`footer.go`, Step 7).
- **`esc` doesn't close / `enter` does nothing:** Missing `XxxChangedMsg`/`XxxCancelledMsg` cases in `handleOverlayMsg` (`update_overlay.go`, Step 6). The overlay emits the message but nothing resets `activeOverlay`/the field.
- **Panic `nil pointer` after confirming:** `confirm()` closure read `m.issueID`/`m.options` lazily — capture them into locals BEFORE `return func() tea.Msg {...}`, because the field is set to nil in Step 6 before the cmd executes.
- **Divider too short / box mis-sized:** You computed width manually. Use `NewOverlayBuilder` and `b.Divider()`/`b.Build()` only (see the lipgloss-width warning in `overlay_base.go`).
- **Lint: function too long / file >500 lines:** Split View into `overlay_<name>_view.go` and message logic into `overlay_<name>_handlers.go` (Go convention used by `overlay_create_*.go`).
- **`make test` passes but UI is wrong:** Unit tests don't render. Run `scripts/tui-test.sh` and observe (Step 10) — required by AGENTS.md before closing the bead.