package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"abacus/internal/graph"
)

func TestSortOverlayEnterEmitsChanged(t *testing.T) {
	o := NewSortOverlay(graph.SortSpec{Key: graph.SortDefault})
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyDown}) // -> Priority asc (index 1)
	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit a cmd")
	}
	changed, ok := cmd().(SortChangedMsg)
	if !ok {
		t.Fatalf("expected SortChangedMsg, got %T", cmd())
	}
	if changed.Spec != (graph.SortSpec{Key: graph.SortPriority, Desc: false}) {
		t.Fatalf("expected Priority asc, got %v", changed.Spec)
	}
}

func TestSortOverlayEscCancels(t *testing.T) {
	o := NewSortOverlay(graph.SortSpec{})
	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should emit a cmd")
	}
	if _, ok := cmd().(SortCancelledMsg); !ok {
		t.Fatalf("expected SortCancelledMsg, got %T", cmd())
	}
}

func TestSortOverlayHighlightsCurrent(t *testing.T) {
	o := NewSortOverlay(graph.SortSpec{Key: graph.SortUpdated, Desc: true})
	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEnter}) // enter without moving
	changed := cmd().(SortChangedMsg)
	if changed.Spec != (graph.SortSpec{Key: graph.SortUpdated, Desc: true}) {
		t.Fatalf("expected current preselected, got %v", changed.Spec)
	}
}

func TestSortOverlayViewContainsLabels(t *testing.T) {
	v := NewSortOverlay(graph.SortSpec{}).View()
	for _, want := range []string{"Sort", "Priority", "Created", "Updated"} {
		if !strings.Contains(v, want) {
			t.Fatalf("View missing %q:\n%s", want, v)
		}
	}
}
