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
			Node: &graph.Node{Issue: beads.FullIssue{ID: id}},
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
