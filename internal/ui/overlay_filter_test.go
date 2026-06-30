package ui

import (
	"testing"

	"abacus/internal/beads"
	"abacus/internal/graph"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilterKeyOpensOverlay(t *testing.T) {
	app := &App{
		ready:    true,
		viewMode: ViewModeAll,
		keys:     DefaultKeyMap(),
		roots:    []*graph.Node{{Issue: beads.FullIssue{ID: "ab-1", Labels: []string{"backend"}}}},
	}
	result, _ := app.Update(keyRunes('f'))
	app = result.(*App)
	if app.activeOverlay != OverlayFilter || app.filterOverlay == nil {
		t.Fatalf("pressing f should open the filter overlay, got overlay=%v ptr=%v", app.activeOverlay, app.filterOverlay)
	}
}

func TestFilterChangedMsgFiltersTree(t *testing.T) {
	a := &graph.Node{Issue: beads.FullIssue{ID: "ab-1", Title: "a", Status: "open", Labels: []string{"backend"}}}
	b := &graph.Node{Issue: beads.FullIssue{ID: "ab-2", Title: "b", Status: "open", Labels: []string{"frontend"}}}
	app := &App{
		ready:    true,
		viewMode: ViewModeAll,
		keys:     DefaultKeyMap(),
		roots:    []*graph.Node{a, b},
	}
	app.recalcVisibleRows()

	result, _ := app.Update(FilterChangedMsg{Label: "backend"})
	app = result.(*App)

	if app.activeOverlay != OverlayNone {
		t.Errorf("overlay should close after applying, got %v", app.activeOverlay)
	}
	if app.labelFilter != "backend" {
		t.Errorf("labelFilter not set, got %q", app.labelFilter)
	}
	if len(app.visibleRows) != 1 || app.visibleRows[0].Node.Issue.ID != "ab-1" {
		t.Errorf("expected only ab-1 visible after filter, got %d rows", len(app.visibleRows))
	}
}

func TestNewFilterOverlaySeedsCurrentSelection(t *testing.T) {
	o := NewFilterOverlay([]string{"backend", "ui"}, []string{"Claude", "Mikhail"}, "ui", "Claude")
	if o.label != "ui" || o.assignee != "Claude" {
		t.Errorf("seed: got label=%q assignee=%q, want ui/Claude", o.label, o.assignee)
	}
}

func TestFilterOverlayCycleLabelForward(t *testing.T) {
	o := NewFilterOverlay([]string{"backend", "ui"}, nil, "", "")
	// On the label row (default), right cycles (any) -> backend -> ui -> (any).
	o, _ = o.Update(keyRunes('l'))
	if o.label != "backend" {
		t.Fatalf("after 1 right: got %q, want backend", o.label)
	}
	o, _ = o.Update(keyRunes('l'))
	if o.label != "ui" {
		t.Fatalf("after 2 right: got %q, want ui", o.label)
	}
	o, _ = o.Update(keyRunes('l'))
	if o.label != "" {
		t.Fatalf("after 3 right: got %q, want (any)/empty", o.label)
	}
}

func TestFilterOverlayCycleAssigneeOnSecondRow(t *testing.T) {
	o := NewFilterOverlay(nil, []string{"Claude", "Mikhail"}, "", "")
	o, _ = o.Update(keyRunes('j')) // move to assignee row
	o, _ = o.Update(keyRunes('l')) // cycle assignee forward
	if o.assignee != "Claude" {
		t.Errorf("assignee after right on row 2: got %q, want Claude", o.assignee)
	}
	if o.label != "" {
		t.Errorf("label row must be untouched, got %q", o.label)
	}
}

func TestFilterOverlayClearActiveRow(t *testing.T) {
	o := NewFilterOverlay([]string{"backend"}, nil, "backend", "")
	o, _ = o.Update(keyRunes('d'))
	if o.label != "" {
		t.Errorf("after clear: got %q, want empty", o.label)
	}
}

func TestFilterOverlayEnterEmitsChanged(t *testing.T) {
	o := NewFilterOverlay([]string{"backend"}, []string{"Claude"}, "backend", "Claude")
	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should return a command")
	}
	msg := cmd()
	changed, ok := msg.(FilterChangedMsg)
	if !ok {
		t.Fatalf("expected FilterChangedMsg, got %T", msg)
	}
	if changed.Label != "backend" || changed.Assignee != "Claude" {
		t.Errorf("changed = %+v, want backend/Claude", changed)
	}
}

func TestFilterOverlayEscEmitsCancelled(t *testing.T) {
	o := NewFilterOverlay(nil, nil, "", "")
	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should return a command")
	}
	if _, ok := cmd().(FilterCancelledMsg); !ok {
		t.Errorf("expected FilterCancelledMsg, got %T", cmd())
	}
}

func TestFilterOverlayEmptyListsStaySafe(t *testing.T) {
	o := NewFilterOverlay(nil, nil, "", "")
	o, _ = o.Update(keyRunes('l')) // cycle with no options
	if o.label != "" {
		t.Errorf("cycling empty label list should stay empty, got %q", o.label)
	}
	o, _ = o.Update(keyRunes('j'))
	o, _ = o.Update(keyRunes('l'))
	if o.assignee != "" {
		t.Errorf("cycling empty assignee list should stay empty, got %q", o.assignee)
	}
}
