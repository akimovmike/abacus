# Multi-select Sequential Beads Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add contiguous ("sequential") multi-selection to the abacus tree so copy and the non-destructive edits (status, priority, labels) run against a whole range of beads at once.

**Architecture:** A single `selectAnchor int` field on `App` defines a contiguous range `visibleRows[min(anchor,cursor)..max]`. No ID set is stored — selected IDs are derived (and deduped) on demand. Overlays stay single-target; bulk edits fan out one `bd` write per selected ID at the message-handler layer via `tea.Batch`. Selection clears on plain navigation, `Esc`, and any `recalcVisibleRows` (refresh / filter / view-mode).

**Tech Stack:** Go, Bubble Tea (charmbracelet), lipgloss, existing `beads.Client` + `beads.MockClient`.

**Spec:** `docs/superpowers/specs/2026-07-03-multi-select-beads-design.md`
**Bead:** ab-qe6x

## Global Constraints

- Go limits: file ≤500 lines (test ≤800), func ≤60 lines (test ≤80), ≤40 stmts, ≤120 cols, cyclomatic complexity ≤10. Split via `_view.go`/`_handlers.go`/`_types.go` if a file grows past the limit.
- TDD: write the failing test first; code must build, `make check` (fmt + vet + golangci-lint) clean, and `make test` pass before a task's final commit.
- Do NOT modify `bd`/`br` — third-party.
- Deduplicate selected issue IDs before ANY bulk write (a multi-parent node appears as more than one visible row).
- Delete, edit, comment, create ignore the selection and act on the cursor row (unchanged).
- Commit only changed files (`git add <files>`). Do not push or sync unless the user explicitly authorizes it.

---

### Task 1: Selection state model and helpers

**Files:**
- Modify: `internal/ui/app.go` (add field to `App` struct ~line 119; init in `NewApp` ~line 299)
- Modify: `internal/ui/state.go:65` (`recalcVisibleRows` — reset anchor)
- Create: `internal/ui/selection.go` (helpers)
- Test: `internal/ui/selection_test.go`

**Interfaces:**
- Produces:
  - `App.selectAnchor int` — index into `visibleRows` where the range began; `-1` = inactive.
  - `func (m *App) selectionActive() bool`
  - `func (m *App) selectionBounds() (lo, hi int)` — inclusive range `[lo,hi]`; returns `(-1,-1)` when inactive or `visibleRows` empty.
  - `func (m *App) rowSelected(i int) bool` — true if index `i` is within the selection range.
  - `func (m *App) selectedIssueIDs() []string` — deduped, in-order `Issue.ID`s across the range; empty slice when inactive.
  - `func (m *App) clearSelection()` — sets `selectAnchor = -1`.

- [ ] **Step 1: Write the failing test**

Create `internal/ui/selection_test.go`:

```go
package ui

import (
	"reflect"
	"testing"

	"abacus/internal/beads"
	"abacus/internal/graph"
)

// rowsFromIDs builds visibleRows from a flat list of issue IDs (no hierarchy).
func rowsFromIDs(ids ...string) []graph.TreeRow {
	rows := make([]graph.TreeRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, graph.TreeRow{
			Node: &graph.Node{Issue: beads.FullIssue{LiteIssue: beads.LiteIssue{ID: id}}},
		})
	}
	return rows
}

func TestSelectionInactiveByDefault(t *testing.T) {
	m := &App{selectAnchor: -1, visibleRows: rowsFromIDs("ab-1", "ab-2")}
	if m.selectionActive() {
		t.Fatal("expected no active selection")
	}
	if got := m.selectedIssueIDs(); len(got) != 0 {
		t.Fatalf("expected no selected IDs, got %v", got)
	}
}

func TestSelectionRangeDownward(t *testing.T) {
	m := &App{selectAnchor: 1, cursor: 3, visibleRows: rowsFromIDs("ab-1", "ab-2", "ab-3", "ab-4", "ab-5")}
	lo, hi := m.selectionBounds()
	if lo != 1 || hi != 3 {
		t.Fatalf("expected bounds [1,3], got [%d,%d]", lo, hi)
	}
	if got := m.selectedIssueIDs(); !reflect.DeepEqual(got, []string{"ab-2", "ab-3", "ab-4"}) {
		t.Fatalf("unexpected selected IDs: %v", got)
	}
	for _, i := range []int{1, 2, 3} {
		if !m.rowSelected(i) {
			t.Fatalf("expected row %d selected", i)
		}
	}
	if m.rowSelected(0) || m.rowSelected(4) {
		t.Fatal("expected rows 0 and 4 unselected")
	}
}

func TestSelectionRangeUpward(t *testing.T) {
	// anchor below cursor — range must still be inclusive and ordered.
	m := &App{selectAnchor: 3, cursor: 1, visibleRows: rowsFromIDs("ab-1", "ab-2", "ab-3", "ab-4")}
	if got := m.selectedIssueIDs(); !reflect.DeepEqual(got, []string{"ab-2", "ab-3", "ab-4"}) {
		t.Fatalf("unexpected selected IDs: %v", got)
	}
}

func TestSelectedIssueIDsDedupesMultiParent(t *testing.T) {
	// Same node (ab-dup) appears twice inside the range via two parents.
	m := &App{selectAnchor: 0, cursor: 2, visibleRows: rowsFromIDs("ab-dup", "ab-2", "ab-dup")}
	if got := m.selectedIssueIDs(); !reflect.DeepEqual(got, []string{"ab-dup", "ab-2"}) {
		t.Fatalf("expected deduped IDs [ab-dup ab-2], got %v", got)
	}
}

func TestClearSelection(t *testing.T) {
	m := &App{selectAnchor: 2, cursor: 4, visibleRows: rowsFromIDs("ab-1", "ab-2", "ab-3", "ab-4", "ab-5")}
	m.clearSelection()
	if m.selectionActive() {
		t.Fatal("expected selection cleared")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui -run 'TestSelection|TestClearSelection|TestSelectedIssueIDs' -v`
Expected: FAIL — `m.selectAnchor` undefined / `selectionActive` undefined.

- [ ] **Step 3: Add the struct field and init**

In `internal/ui/app.go`, inside the `App` struct near `cursor int` (~line 119), add:

```go
	cursor            int
	// selectAnchor marks where a contiguous multi-selection began (index into
	// visibleRows). -1 means no active selection. The selection is the inclusive
	// range between selectAnchor and cursor. Cleared on plain navigation, Esc,
	// and any recalcVisibleRows (refresh/filter/view-mode shift indices).
	selectAnchor int
```

In `NewApp` (`internal/ui/app.go` ~line 299), initialize it where the struct literal / assignments happen. If `App` is built as a struct literal, add `selectAnchor: -1`; otherwise add `app.selectAnchor = -1` before the `return`:

```go
	app.selectAnchor = -1
```

- [ ] **Step 4: Create the helpers**

Create `internal/ui/selection.go`:

```go
package ui

// selectionActive reports whether a contiguous multi-selection is in progress.
func (m *App) selectionActive() bool {
	return m.selectAnchor >= 0
}

// clearSelection ends any active multi-selection.
func (m *App) clearSelection() {
	m.selectAnchor = -1
}

// selectionBounds returns the inclusive [lo, hi] range of selected visibleRows
// indices, or (-1, -1) when no selection is active or the tree is empty.
func (m *App) selectionBounds() (int, int) {
	if !m.selectionActive() || len(m.visibleRows) == 0 {
		return -1, -1
	}
	lo, hi := m.selectAnchor, m.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi > len(m.visibleRows)-1 {
		hi = len(m.visibleRows) - 1
	}
	return lo, hi
}

// rowSelected reports whether the visibleRows index i is within the selection.
func (m *App) rowSelected(i int) bool {
	lo, hi := m.selectionBounds()
	return lo >= 0 && i >= lo && i <= hi
}

// selectedIssueIDs returns the deduped, in-order issue IDs of the selected rows.
// A multi-parent node appearing more than once in the range yields one ID.
func (m *App) selectedIssueIDs() []string {
	lo, hi := m.selectionBounds()
	if lo < 0 {
		return nil
	}
	seen := make(map[string]bool, hi-lo+1)
	ids := make([]string, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		id := m.visibleRows[i].Node.Issue.ID
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
```

- [ ] **Step 5: Reset selection in recalcVisibleRows**

In `internal/ui/state.go`, at the TOP of `recalcVisibleRows` (line 65, before `m.visibleRows = []graph.TreeRow{}`), add:

```go
func (m *App) recalcVisibleRows() {
	m.selectAnchor = -1 // indices shift on refresh/filter/view-mode; drop stale selection
	m.visibleRows = []graph.TreeRow{}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/ui -run 'TestSelection|TestClearSelection|TestSelectedIssueIDs' -v`
Expected: PASS (all 5).

- [ ] **Step 7: Verify build + existing tests still green**

Run: `make check-test`
Expected: fmt/vet/lint clean; all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/app.go internal/ui/state.go internal/ui/selection.go internal/ui/selection_test.go
git commit -m "feat(ui): selection state model for multi-select (ab-qe6x)"
```

---

### Task 2: Extend keys and selection-clear on navigation

**Files:**
- Modify: `internal/ui/keys.go` (add `ExtendDown`, `ExtendUp` to `KeyMap` struct ~line 9; bindings in `DefaultKeyMap` ~line 69)
- Modify: `internal/ui/update_keys.go` (`handleGlobalKey` ~line 109)
- Test: `internal/ui/update_keys_selection_test.go` (create)

**Interfaces:**
- Consumes: `selectionActive`, `clearSelection` (Task 1); `clampCursor`, `prepareTreeKeyboardNavigation`, `updateViewportContent` (existing).
- Produces:
  - `KeyMap.ExtendDown key.Binding` (keys `J`, `shift+down`)
  - `KeyMap.ExtendUp key.Binding` (keys `K`, `shift+up`)
  - `func (m *App) handleExtendSelection(delta int) (tea.Model, tea.Cmd)`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/update_keys_selection_test.go`:

```go
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTreeApp() *App {
	m := &App{
		selectAnchor: -1,
		cursor:       0,
		keys:         DefaultKeyMap(),
		visibleRows:  rowsFromIDs("ab-1", "ab-2", "ab-3", "ab-4"),
	}
	return m
}

func TestExtendDownStartsAndGrowsSelection(t *testing.T) {
	m := newTreeApp()
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	if !m.selectionActive() {
		t.Fatal("expected selection active after J")
	}
	if m.selectAnchor != 0 || m.cursor != 1 {
		t.Fatalf("expected anchor=0 cursor=1, got anchor=%d cursor=%d", m.selectAnchor, m.cursor)
	}
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	if m.selectAnchor != 0 || m.cursor != 2 {
		t.Fatalf("expected anchor=0 cursor=2, got anchor=%d cursor=%d", m.selectAnchor, m.cursor)
	}
}

func TestExtendUpStartsSelection(t *testing.T) {
	m := newTreeApp()
	m.cursor = 3
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")})
	if m.selectAnchor != 3 || m.cursor != 2 {
		t.Fatalf("expected anchor=3 cursor=2, got anchor=%d cursor=%d", m.selectAnchor, m.cursor)
	}
}

func TestPlainNavigationClearsSelection(t *testing.T) {
	m := newTreeApp()
	m.selectAnchor = 0
	m.cursor = 2
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.selectionActive() {
		t.Fatal("expected plain 'j' to clear selection")
	}
	if m.cursor != 3 {
		t.Fatalf("expected cursor to advance to 3, got %d", m.cursor)
	}
}

func TestExtendDoesNotOverrunBottom(t *testing.T) {
	m := newTreeApp()
	m.cursor = 3 // last row
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	if m.cursor != 3 {
		t.Fatalf("expected cursor clamped at 3, got %d", m.cursor)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui -run 'TestExtend|TestPlainNavigationClears' -v`
Expected: FAIL — `m.keys.ExtendDown` undefined / behavior missing.

- [ ] **Step 3: Add the key bindings**

In `internal/ui/keys.go`, add to the `KeyMap` struct (under `// Navigation`, after `PageDown`):

```go
	// Multi-select range extend
	ExtendDown key.Binding
	ExtendUp   key.Binding
```

In `DefaultKeyMap` (after the `PageDown` binding, before `// Actions`):

```go
		ExtendDown: key.NewBinding(
			key.WithKeys("J", "shift+down"),
			key.WithHelp("J / ⇧↓", "Extend selection down"),
		),
		ExtendUp: key.NewBinding(
			key.WithKeys("K", "shift+up"),
			key.WithHelp("K / ⇧↑", "Extend selection up"),
		),
```

- [ ] **Step 4: Wire extend + clear-on-nav into handleGlobalKey**

In `internal/ui/update_keys.go`, add the extend cases to the `switch` in `handleGlobalKey` (place them BEFORE the `Down`/`Up` cases so `J`/`K` match first):

```go
	case key.Matches(msg, m.keys.ExtendDown):
		return m.handleExtendSelection(1)
	case key.Matches(msg, m.keys.ExtendUp):
		return m.handleExtendSelection(-1)
```

Add `m.clearSelection()` as the FIRST line inside each plain-navigation case body — `Down`, `Up`, `Home`, `End`, `PageDown`, `PageUp`. Example for `Down`:

```go
	case key.Matches(msg, m.keys.Down):
		m.clearSelection()
		m.prepareTreeKeyboardNavigation()
		m.cursor++
		m.clampCursor()
		m.updateViewportContent()
```

Apply the same `m.clearSelection()` first-line insertion to `Up`, `Home`, `End`, `PageDown`, `PageUp`.

Add the new handler method (near `handleTreeExpand`, ~line 244):

```go
// handleExtendSelection grows a contiguous selection by moving the cursor by
// delta rows. The first extend from an idle cursor anchors the range at the
// current cursor position.
func (m *App) handleExtendSelection(delta int) (tea.Model, tea.Cmd) {
	if len(m.visibleRows) == 0 {
		return m, nil
	}
	if !m.selectionActive() {
		m.selectAnchor = m.cursor
	}
	m.prepareTreeKeyboardNavigation()
	m.cursor += delta
	m.clampCursor()
	m.updateViewportContent()
	return m, nil
}
```

- [ ] **Step 5: Add Esc-clears-selection**

In `internal/ui/update_keys.go`, at the TOP of the existing `Escape` case (currently first checks `m.showErrorToast`), add a selection check first:

```go
	case key.Matches(msg, m.keys.Escape):
		if m.selectionActive() {
			m.clearSelection()
			return m, nil
		}
		if m.showErrorToast {
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/ui -run 'TestExtend|TestPlainNavigationClears' -v`
Expected: PASS (all 4).

- [ ] **Step 7: Verify build + full suite**

Run: `make check-test`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/keys.go internal/ui/update_keys.go internal/ui/update_keys_selection_test.go
git commit -m "feat(ui): J/K + Shift+arrow extend selection, nav/Esc clears (ab-qe6x)"
```

---

### Task 3: Bulk copy of selected IDs

**Files:**
- Modify: `internal/ui/app.go` (add `copiedCount int` field near copy-toast state ~line 184)
- Modify: `internal/ui/update_keys.go` (`handleCopyKey` ~line 302)
- Modify: `internal/ui/view_toasts.go` (`copyToastLayer` ~line 62)
- Test: `internal/ui/update_keys_selection_test.go` (append)

**Interfaces:**
- Consumes: `selectionActive`, `selectedIssueIDs`, `clearSelection` (Task 1); `clipboard.WriteAll`, `scheduleCopyToastTick` (existing).
- Produces: `App.copiedCount int` — number of beads in the last copy (1 for single).

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/update_keys_selection_test.go`:

```go
func TestBulkCopyJoinsSelectedIDs(t *testing.T) {
	m := newTreeApp()
	m.selectAnchor = 0
	m.cursor = 2 // ab-1, ab-2, ab-3
	m.handleCopyKey()
	if m.copiedCount != 3 {
		t.Fatalf("expected copiedCount 3, got %d", m.copiedCount)
	}
	got, err := clipboard.ReadAll()
	if err != nil {
		t.Skipf("clipboard unavailable in this environment: %v", err)
	}
	if got != "ab-1\nab-2\nab-3" {
		t.Fatalf("expected newline-joined IDs, got %q", got)
	}
}

func TestSingleCopyUnchanged(t *testing.T) {
	m := newTreeApp()
	m.cursor = 1 // no selection
	m.handleCopyKey()
	if m.copiedCount != 1 {
		t.Fatalf("expected copiedCount 1, got %d", m.copiedCount)
	}
	if m.copiedBeadID != "ab-2" {
		t.Fatalf("expected copiedBeadID ab-2, got %s", m.copiedBeadID)
	}
}
```

Add the clipboard import to the test file's import block:

```go
	"github.com/atotto/clipboard"
```

(Confirm the exact clipboard import path matches the one already used in `internal/ui/update_keys.go` — copy it verbatim from there.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui -run 'TestBulkCopy|TestSingleCopy' -v`
Expected: FAIL — `m.copiedCount` undefined.

- [ ] **Step 3: Add the copiedCount field**

In `internal/ui/app.go`, in the copy-toast block (near `copiedBeadID string` ~line 186):

```go
	// Copy toast state
	showCopyToast  bool
	copyToastStart time.Time
	copiedBeadID   string
	copiedCount    int // number of beads copied (>1 => bulk copy)
```

- [ ] **Step 4: Update handleCopyKey for bulk**

In `internal/ui/update_keys.go`, replace `handleCopyKey` body:

```go
// handleCopyKey copies the current bead ID (or the whole selection) to clipboard.
func (m *App) handleCopyKey() (tea.Model, tea.Cmd) {
	if len(m.visibleRows) == 0 {
		return m, nil
	}
	var ids []string
	if m.selectionActive() {
		ids = m.selectedIssueIDs()
	} else {
		ids = []string{m.visibleRows[m.cursor].Node.Issue.ID}
	}
	if err := clipboard.WriteAll(strings.Join(ids, "\n")); err != nil {
		return m, nil
	}
	m.copiedBeadID = ids[0]
	m.copiedCount = len(ids)
	m.showCopyToast = true
	m.copyToastStart = time.Now()
	m.clearSelection()
	return m, scheduleCopyToastTick()
}
```

Ensure `strings` is imported in `update_keys.go` (add to the import block if absent).

- [ ] **Step 5: Update the copy toast message**

In `internal/ui/view_toasts.go`, in `copyToastLayer`, replace the `msgLine` assignment (~line 62):

```go
	msgLine := fmt.Sprintf("Copied '%s' to clipboard.", m.copiedBeadID)
	if m.copiedCount > 1 {
		msgLine = fmt.Sprintf("Copied %d beads to clipboard.", m.copiedCount)
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/ui -run 'TestBulkCopy|TestSingleCopy' -v`
Expected: PASS (bulk asserts count; clipboard content assertion self-skips if clipboard is unavailable in CI).

- [ ] **Step 7: Verify build + full suite**

Run: `make check-test`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/app.go internal/ui/update_keys.go internal/ui/view_toasts.go internal/ui/update_keys_selection_test.go
git commit -m "feat(ui): bulk-copy selected bead IDs to clipboard (ab-qe6x)"
```

---

### Task 4: Bulk fan-out for status / priority / labels

**Files:**
- Modify: `internal/ui/update_overlay.go` (`StatusChangedMsg` ~line 21, `LabelsUpdatedMsg` ~line 62, `PriorityChangedMsg` ~line 300)
- Create: `internal/ui/bulk_edit.go` (fan-out helpers)
- Test: `internal/ui/bulk_edit_test.go`

**Interfaces:**
- Consumes: `selectionActive`, `selectedIssueIDs`, `clearSelection` (Task 1); `executeStatusChangeCmd(issueID, newStatus string) tea.Cmd`, `executeReopenCmd(issueID string) tea.Cmd`, `executePriorityChangeCmd(issueID string, priority int) tea.Cmd`, `executeLabelsUpdate(msg LabelsUpdatedMsg) tea.Cmd` (existing in `update_commands.go`); `beads.MockClient` counters `UpdateStatusCallArgs`, `UpdatePriorityCallArgs`, `AddLabelCallArgs` (existing).
- Produces:
  - `func (m *App) bulkStatusCmds(oldStatus, newStatus string) []tea.Cmd`
  - `func (m *App) bulkPriorityCmds(priority int) []tea.Cmd`
  - `func (m *App) bulkLabelsCmds(msg LabelsUpdatedMsg) []tea.Cmd`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/bulk_edit_test.go`:

```go
package ui

import (
	"testing"

	"abacus/internal/beads"

	tea "github.com/charmbracelet/bubbletea"
)

// drainCmd recursively executes a tea.Cmd tree (handling tea.BatchMsg) and
// returns every leaf message. Leaf closures run their side effects (mock calls).
func drainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, drainCmd(c)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func appWithMockSelection(mock *beads.MockClient, n int) *App {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = string(rune('a'+i)) // "a","b","c",...
	}
	return &App{
		client:       mock,
		selectAnchor: 0,
		cursor:       n - 1,
		visibleRows:  rowsFromIDs(ids...),
	}
}

func TestBulkStatusFansOutOnePerBead(t *testing.T) {
	mock := beads.NewMockClient()
	m := appWithMockSelection(mock, 3)
	cmds := m.bulkStatusCmds("open", "in_progress")
	if len(cmds) != 3 {
		t.Fatalf("expected 3 status cmds, got %d", len(cmds))
	}
	for _, c := range cmds {
		drainCmd(c)
	}
	if mock.UpdateStatusCallCount != 3 {
		t.Fatalf("expected 3 UpdateStatus calls, got %d", mock.UpdateStatusCallCount)
	}
}

func TestBulkStatusReopenUsesReopenPath(t *testing.T) {
	mock := beads.NewMockClient()
	m := appWithMockSelection(mock, 2)
	cmds := m.bulkStatusCmds("closed", "open") // closed->open must reopen
	for _, c := range cmds {
		drainCmd(c)
	}
	if mock.ReopenCallCount != 2 {
		t.Fatalf("expected 2 Reopen calls, got %d", mock.ReopenCallCount)
	}
	if mock.UpdateStatusCallCount != 0 {
		t.Fatalf("expected 0 UpdateStatus calls, got %d", mock.UpdateStatusCallCount)
	}
}

func TestBulkPriorityFansOut(t *testing.T) {
	mock := beads.NewMockClient()
	m := appWithMockSelection(mock, 3)
	cmds := m.bulkPriorityCmds(1)
	for _, c := range cmds {
		drainCmd(c)
	}
	if mock.UpdatePriorityCallCount != 3 {
		t.Fatalf("expected 3 UpdatePriority calls, got %d", mock.UpdatePriorityCallCount)
	}
}

func TestBulkLabelsFansOut(t *testing.T) {
	mock := beads.NewMockClient()
	m := appWithMockSelection(mock, 2)
	cmds := m.bulkLabelsCmds(LabelsUpdatedMsg{Added: []string{"urgent"}})
	for _, c := range cmds {
		drainCmd(c)
	}
	if mock.AddLabelCallCount != 2 {
		t.Fatalf("expected 2 AddLabel calls, got %d", mock.AddLabelCallCount)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui -run 'TestBulk(Status|Priority|Labels)' -v`
Expected: FAIL — `bulkStatusCmds` etc. undefined.

> If `executeLabelsUpdate` reads its issue ID from `msg.IssueID` (not a parameter), the helper must set `msg.IssueID = id` per iteration — confirm by reading `executeLabelsUpdate` in `update_commands.go` before writing Step 3, and adjust the helper accordingly.

- [ ] **Step 3: Create the fan-out helpers**

Create `internal/ui/bulk_edit.go`:

```go
package ui

import tea "github.com/charmbracelet/bubbletea"

// bulkStatusCmds returns one status-change command per selected bead. A
// closed->open transition uses the reopen path, matching the single-target
// handler. oldStatus is the status shown in the overlay before the change.
func (m *App) bulkStatusCmds(oldStatus, newStatus string) []tea.Cmd {
	ids := m.selectedIssueIDs()
	cmds := make([]tea.Cmd, 0, len(ids))
	for _, id := range ids {
		if oldStatus == "closed" && newStatus == "open" {
			cmds = append(cmds, m.executeReopenCmd(id))
		} else {
			cmds = append(cmds, m.executeStatusChangeCmd(id, newStatus))
		}
	}
	return cmds
}

// bulkPriorityCmds returns one priority-change command per selected bead.
func (m *App) bulkPriorityCmds(priority int) []tea.Cmd {
	ids := m.selectedIssueIDs()
	cmds := make([]tea.Cmd, 0, len(ids))
	for _, id := range ids {
		cmds = append(cmds, m.executePriorityChangeCmd(id, priority))
	}
	return cmds
}

// bulkLabelsCmds returns one label-update command per selected bead, applying
// the same Added/Removed sets to each.
func (m *App) bulkLabelsCmds(msg LabelsUpdatedMsg) []tea.Cmd {
	ids := m.selectedIssueIDs()
	cmds := make([]tea.Cmd, 0, len(ids))
	for _, id := range ids {
		perBead := msg
		perBead.IssueID = id
		cmds = append(cmds, m.executeLabelsUpdate(perBead))
	}
	return cmds
}
```

- [ ] **Step 4: Run helper tests to verify they pass**

Run: `go test ./internal/ui -run 'TestBulk(Status|Priority|Labels)' -v`
Expected: PASS (all 4).

- [ ] **Step 5: Wire the helpers into the overlay handlers**

In `internal/ui/update_overlay.go`, `StatusChangedMsg` case — replace the single-target dispatch (the `return m, tea.Batch(...)` lines inside `if msg.NewStatus != ""`) with a selection branch. The block currently reads:

```go
		if msg.NewStatus != "" {
			m.displayStatusToast(msg.IssueID, msg.NewStatus)
			if oldStatus == "closed" && msg.NewStatus == "open" {
				return m, tea.Batch(m.executeReopenCmd(msg.IssueID), scheduleStatusToastTick()), true
			}
			return m, tea.Batch(m.executeStatusChangeCmd(msg.IssueID, msg.NewStatus), scheduleStatusToastTick()), true
		}
```

Replace with:

```go
		if msg.NewStatus != "" {
			if m.selectionActive() {
				cmds := m.bulkStatusCmds(oldStatus, msg.NewStatus)
				m.displayBulkStatusToast(len(cmds), msg.NewStatus)
				m.clearSelection()
				cmds = append(cmds, scheduleStatusToastTick())
				return m, tea.Batch(cmds...), true
			}
			m.displayStatusToast(msg.IssueID, msg.NewStatus)
			if oldStatus == "closed" && msg.NewStatus == "open" {
				return m, tea.Batch(m.executeReopenCmd(msg.IssueID), scheduleStatusToastTick()), true
			}
			return m, tea.Batch(m.executeStatusChangeCmd(msg.IssueID, msg.NewStatus), scheduleStatusToastTick()), true
		}
```

Apply the analogous selection branch to `PriorityChangedMsg` (~line 300, using `m.bulkPriorityCmds(msg.NewPriority)` and `schedulePriorityToastTick()`) and `LabelsUpdatedMsg` (~line 62, using `m.bulkLabelsCmds(msg)` and `scheduleLabelsToastTick()`). For labels, guard the same way it currently guards (`if len(msg.Added) > 0 || len(msg.Removed) > 0`).

Add a bulk toast helper in `internal/ui/update_commands.go` next to `displayStatusToast` (mirror its fields; reuse the existing status-toast state, setting the bead-ID field to a count summary):

```go
// displayBulkStatusToast shows a success toast for a multi-bead status change.
func (m *App) displayBulkStatusToast(count int, newStatus string) {
	m.statusToastVisible = true
	m.statusToastStart = time.Now()
	m.statusToastBeadID = fmt.Sprintf("%d beads", count)
	m.statusToastNewStatus = newStatus
}
```

(If the status toast rendering reads `statusToastBeadID` and formats it as an ID, this yields "N beads → <status>", which is the intended copy. Confirm by reading the status-toast render in `view_toasts.go`; if it hard-prefixes "ab-", add a parallel bulk render branch instead.) Add equivalent `displayBulkPriorityToast(count, priority)` and rely on the existing labels toast for the labels case (labels toast already lists added/removed; passing a "N beads" ID is acceptable).

- [ ] **Step 6: Add a handler-level integration test**

Append to `internal/ui/bulk_edit_test.go`:

```go
func TestStatusChangedMsgFansOutWhenSelectionActive(t *testing.T) {
	mock := beads.NewMockClient()
	m := appWithMockSelection(mock, 3)
	_, cmd, handled := m.handleOverlayMsg(StatusChangedMsg{IssueID: "a", NewStatus: "in_progress"})
	if !handled {
		t.Fatal("expected StatusChangedMsg handled")
	}
	if m.selectionActive() {
		t.Fatal("expected selection cleared after bulk status change")
	}
	for _, msg := range drainCmd(cmd) {
		_ = msg // side effects (mock.UpdateStatus) run during drain
	}
	if mock.UpdateStatusCallCount != 3 {
		t.Fatalf("expected 3 UpdateStatus calls via handler, got %d", mock.UpdateStatusCallCount)
	}
}
```

Note: `handleOverlayMsg` sets `m.statusOverlay = nil`; the test does not set a `statusOverlay`, so `oldStatus` resolves to `""` — fine for the in_progress path. Run:

Run: `go test ./internal/ui -run 'TestStatusChangedMsgFansOut' -v`
Expected: PASS.

- [ ] **Step 7: Verify build + full suite**

Run: `make check-test`
Expected: clean. If a toast render test breaks on the "N beads" ID, adjust the bulk toast render per the Step 5 note.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/bulk_edit.go internal/ui/bulk_edit_test.go internal/ui/update_overlay.go internal/ui/update_commands.go
git commit -m "feat(ui): fan out status/priority/labels edits across selection (ab-qe6x)"
```

---

### Task 5: Render selected rows and footer count

**Files:**
- Modify: `internal/ui/tree.go` (`buildTreeLines` render branch ~line 140; add `buildMultiSelectRow` near `buildCrossHighlightRow` ~line 349)
- Modify: `internal/ui/footer.go` (`renderRefreshStatus` ~line 162 OR `renderFooter` ~line 78)
- Test: golden snapshot `internal/ui/*_golden_test.go`

**Interfaces:**
- Consumes: `rowSelected` (Task 1); `currentThemeWrapper`, `buildSelectedRow`, `buildCrossHighlightRow` (existing).
- Produces: `buildMultiSelectRow(indent, marker, icon string, iconStyle lipgloss.Style, priority, id, title string, textStyle lipgloss.Style, treeWidth, totalWidth int, columns string) string`.

- [ ] **Step 1: Add the multi-select row builder**

In `internal/ui/tree.go`, add after `buildCrossHighlightRow` (~line 383). It mirrors `buildCrossHighlightRow` but uses a distinct background so selected-non-cursor rows read as a group under the (brighter) cursor row:

```go
// buildMultiSelectRow creates a full-width row for a bead inside an active
// multi-selection (but not the cursor row). Uses a dimmer background than the
// cursor so the cursor stays distinguishable within the selected block.
func buildMultiSelectRow(indent, marker, icon string, iconStyle lipgloss.Style, priority, id, title string, textStyle lipgloss.Style, treeWidth, totalWidth int, columns string) string {
	t := currentThemeWrapper()
	bg := t.BorderNormal()

	base := lipgloss.NewStyle().Background(bg)
	prefix := base.Foreground(t.TextMuted())
	iconS := base.Foreground(iconStyle.GetForeground())
	priorityS := base.Foreground(t.TextMuted())
	idS := base.Foreground(t.Accent()).Bold(true)
	textS := base.Foreground(textStyle.GetForeground())

	treeContent := prefix.Render(fmt.Sprintf(" %s%s ", indent, marker)) +
		iconS.Render(icon) + base.Render(" ")
	if priority != "" {
		treeContent += priorityS.Render(priority) + base.Render(" ")
	}
	treeContent += idS.Render(id) + base.Render(" ") + textS.Render(title)

	if columns != "" {
		treeContent = padToWidth(treeContent, treeWidth, base)
		treeContent += columns
	}
	return lipgloss.NewStyle().Background(bg).Width(totalWidth).Render(treeContent)
}
```

> ponytail: reuses `BorderNormal()` (same bg as cross-highlight) rather than adding a new per-theme color. If the visual clash with cross-highlight matters in practice, add a dedicated theme color then.

- [ ] **Step 2: Add the selected-row branch in buildTreeLines**

In `internal/ui/tree.go` `buildTreeLines`, change the render branch (currently `if i == m.cursor {...} else if isCrossHighlight {...} else {...}`) to insert a selected branch AFTER cursor and BEFORE cross-highlight:

```go
		if i == m.cursor {
			// ... unchanged buildSelectedRow block ...
		} else if m.rowSelected(i) {
			line := buildMultiSelectRow(
				indent,
				marker,
				iconStr,
				iconStyle,
				priorityStr,
				idDisplay,
				titleLines[0],
				textStyle,
				treeWidth,
				totalWidth,
				columns.render(node, columnRenderCrossHighlight),
			)
			lines = append(lines, line)
		} else if isCrossHighlight {
			// ... unchanged buildCrossHighlightRow block ...
		} else {
			// ... unchanged normal block ...
		}
```

- [ ] **Step 3: Add the footer "N selected" indicator**

In `internal/ui/footer.go`, in `renderRefreshStatus` (~line 162), prepend a selection count when active. At the top of the function:

```go
func (m *App) renderRefreshStatus() string {
	if m.selectionActive() {
		if ids := m.selectedIssueIDs(); len(ids) > 0 {
			return styleFooterText().Render(fmt.Sprintf("%d selected", len(ids)))
		}
	}
	// ... existing body ...
```

Use whatever footer text style the existing body uses (copy the style call verbatim from the function body; if it uses a local variable, reuse it). Ensure `fmt` is imported.

- [ ] **Step 4: Build and eyeball via the TUI harness**

```bash
make build
./scripts/tui-test.sh start
./scripts/tui-test.sh keys 'JJ'   # extend selection down two rows
./scripts/tui-test.sh view        # confirm 2-3 rows show selection bg + "3 selected" footer
./scripts/tui-test.sh keys 'j'    # plain nav clears selection
./scripts/tui-test.sh view        # confirm selection bg gone
./scripts/tui-test.sh quit
```

Expected: the anchor row + extended rows render with the selection background, cursor row brighter; footer shows the count; plain `j` clears it.

- [ ] **Step 5: Refresh the golden snapshot**

Run: `go test ./internal/ui -run TestOverlayAndToastGoldenSnapshots -update-golden`
Then verify: `go test ./internal/ui -run TestOverlayAndToastGoldenSnapshots -v`
Expected: PASS. Inspect the diff in `testdata/ui/golden/` to confirm the selection rendering is intentional before committing.

- [ ] **Step 6: Verify build + full suite**

Run: `make check-test`
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/tree.go internal/ui/footer.go testdata/ui/golden/
git commit -m "feat(ui): render multi-select rows + footer count (ab-qe6x)"
```

---

### Task 6: Help overlay + docs + close-out

**Files:**
- Modify: `internal/ui/help.go` (add extend + bulk hints to the help listing)
- Modify: `.beads/issues.jsonl` (export after closing the bead)

- [ ] **Step 1: Add help entries**

In `internal/ui/help.go`, add rows describing the new keys near the navigation / actions listing (match the existing help row format in that file):
- `J / ⇧↓  Extend selection down`
- `K / ⇧↑  Extend selection up`
- note that `c` / `s` / `p` / `L` act on the whole selection when one is active.

- [ ] **Step 2: Verify help renders**

```bash
make build
./scripts/tui-test.sh start
./scripts/tui-test.sh keys '?'
./scripts/tui-test.sh view   # confirm the new hints appear
./scripts/tui-test.sh quit
```

- [ ] **Step 3: Full CI gate**

Run: `make ci`
Expected: lint + unit + integration + build all green.

- [ ] **Step 4: Close the bead and export**

```bash
bd update ab-qe6x --remove-label next --remove-label check --remove-label question 2>/dev/null
bd close ab-qe6x
bd sync --flush-only 2>/dev/null || bd export 2>/dev/null
```

- [ ] **Step 5: Commit**

```bash
git add internal/ui/help.go .beads/issues.jsonl
git commit -m "docs(ui): help entries for multi-select; close ab-qe6x"
```

- [ ] **Step 6: Report handoff**

Report changed files, `make ci` result, and the proposed push commands. Do NOT push or `bd dolt push` unless the user authorizes it (conservative profile).

---

## Self-Review

**Spec coverage:**
- Selection state model → Task 1. ✓
- Keys J/K + Shift+arrow, clear on nav/Esc/refresh → Task 2 (+ recalc reset in Task 1). ✓
- Bulk copy → Task 3. ✓
- Bulk status/priority/labels fan-out → Task 4. ✓
- Rendering + footer count → Task 5. ✓
- Dedup multi-parent → Task 1 (`selectedIssueIDs` + test). ✓
- Partial-failure behavior → inherent (independent `tea.Batch` cmds; existing error toast). No dedicated task needed. ✓
- Delete stays single-target → no change made (explicitly untouched). ✓
- Testing (unit, handler, golden) → Tasks 1-5. ✓

**Type consistency:** `selectAnchor`, `selectionActive`, `selectionBounds`, `rowSelected`, `selectedIssueIDs`, `clearSelection`, `handleExtendSelection`, `bulkStatusCmds`/`bulkPriorityCmds`/`bulkLabelsCmds`, `buildMultiSelectRow`, `copiedCount` — used consistently across tasks. `executeReopenCmd`/`executeStatusChangeCmd`/`executePriorityChangeCmd`/`executeLabelsUpdate` names match `update_commands.go`.

**Open verification flagged inline (do before writing that step's code):**
- Task 3: exact clipboard import path (copy from `update_keys.go`).
- Task 4: whether `executeLabelsUpdate` reads `msg.IssueID` (helper sets it per-bead — already handled) and whether the status/priority toast render hard-prefixes "ab-" (add a bulk render branch if so).
- Task 5: exact footer text style call; `columnRenderCrossHighlight` reuse for selected rows.
