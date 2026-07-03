package ui

import (
	"reflect"
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
		ids[i] = string(rune('a' + i)) // "a","b","c",...
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
	cmds := m.bulkStatusCmds("in_progress")
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
	for i := range m.visibleRows {
		m.visibleRows[i].Node.Issue.Status = "closed"
	}
	cmds := m.bulkStatusCmds("open") // closed->open must reopen
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

// TestBulkStatusReopenIsDecidedPerBead reproduces the final-review bug where
// bulkStatusCmds decided the reopen-vs-status-change split once for the whole
// selection (from the cursor bead's status) instead of per bead. A mixed
// range of open/closed beads must only reopen the beads that are
// individually closed.
func TestBulkStatusReopenIsDecidedPerBead(t *testing.T) {
	t.Run("cursor on open bead, range includes a closed bead", func(t *testing.T) {
		mock := beads.NewMockClient()
		rows := rowsFromIDs("a", "b", "c")
		rows[0].Node.Issue.Status = "open"
		rows[1].Node.Issue.Status = "closed"
		rows[2].Node.Issue.Status = "open"
		m := &App{client: mock, selectAnchor: 0, cursor: 2, visibleRows: rows}

		cmds := m.bulkStatusCmds("open")
		for _, c := range cmds {
			drainCmd(c)
		}

		if !reflect.DeepEqual(mock.ReopenCallArgs, []string{"b"}) {
			t.Fatalf("expected Reopen called for [b] only, got %v", mock.ReopenCallArgs)
		}
		wantUpdates := [][]string{{"a", "open"}, {"c", "open"}}
		if !reflect.DeepEqual(mock.UpdateStatusCallArgs, wantUpdates) {
			t.Fatalf("expected UpdateStatus calls %v, got %v", wantUpdates, mock.UpdateStatusCallArgs)
		}
	})

	t.Run("cursor on closed bead, range includes open beads", func(t *testing.T) {
		mock := beads.NewMockClient()
		rows := rowsFromIDs("a", "b", "c")
		rows[0].Node.Issue.Status = "open"
		rows[1].Node.Issue.Status = "closed"
		rows[2].Node.Issue.Status = "open"
		m := &App{client: mock, selectAnchor: 0, cursor: 1, visibleRows: rows}

		cmds := m.bulkStatusCmds("open")
		for _, c := range cmds {
			drainCmd(c)
		}

		if !reflect.DeepEqual(mock.ReopenCallArgs, []string{"b"}) {
			t.Fatalf("expected Reopen called for [b] only, got %v", mock.ReopenCallArgs)
		}
		wantUpdates := [][]string{{"a", "open"}}
		if !reflect.DeepEqual(mock.UpdateStatusCallArgs, wantUpdates) {
			t.Fatalf("expected UpdateStatus calls %v, got %v", wantUpdates, mock.UpdateStatusCallArgs)
		}
	})
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

func TestBeadCountLabelIsSingularForOne(t *testing.T) {
	if got := beadCountLabel(1); got != "1 bead" {
		t.Fatalf("expected singular label, got %q", got)
	}
	if got := beadCountLabel(2); got != "2 beads" {
		t.Fatalf("expected plural label, got %q", got)
	}
}

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
