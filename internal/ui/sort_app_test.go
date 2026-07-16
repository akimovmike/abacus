package ui

import (
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	"abacus/internal/beads"
	"abacus/internal/graph"
)

func rowIDs(app *App) []string {
	out := make([]string, len(app.visibleRows))
	for i, r := range app.visibleRows {
		out[i] = r.Node.Issue.ID
	}
	return out
}

// TestSortChangedReordersAndPreservesCursor drives the SortChangedMsg handler
// end to end: the tree must re-sort under the new spec and the cursor must stay
// on the same bead (not the same index).
func TestSortChangedReordersAndPreservesCursor(t *testing.T) {
	// chdir away from the real repo so config.SaveSort can't find a .beads dir
	// and mutate the project's .abacus/config.yaml (findBeadsDir walks upward).
	oldWd, _ := os.Getwd()
	_ = os.Chdir(t.TempDir())
	defer func() { _ = os.Chdir(oldWd) }()

	mkRoot := func(id string, prio int) *graph.Node {
		return &graph.Node{Issue: beads.FullIssue{ID: id, Priority: prio, Status: "open"}}
	}
	app := &App{
		roots:        []*graph.Node{mkRoot("b", 2), mkRoot("a", 0), mkRoot("c", 4)},
		selectAnchor: -1,
		sortSpec:     graph.SortSpec{}, // Default
	}
	app.recalcVisibleRows()
	if !app.restoreCursorToRow("a", "") {
		t.Fatal("could not place cursor on bead a")
	}

	_, _, handled := app.handleOverlayMsg(SortChangedMsg{Spec: graph.SortSpec{Key: graph.SortPriority, Desc: false}})
	if !handled {
		t.Fatal("SortChangedMsg was not handled")
	}

	got := make([]string, len(app.visibleRows))
	for i, r := range app.visibleRows {
		got[i] = r.Node.Issue.ID
	}
	want := []string{"a", "b", "c"} // P0, P2, P4
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if app.sortSpec != (graph.SortSpec{Key: graph.SortPriority}) {
		t.Fatalf("sortSpec = %v, want Priority asc", app.sortSpec)
	}
	if app.visibleRows[app.cursor].Node.Issue.ID != "a" {
		t.Fatalf("cursor on %q, want a", app.visibleRows[app.cursor].Node.Issue.ID)
	}
	if app.activeOverlay != OverlayNone || app.sortOverlay != nil {
		t.Fatal("overlay should be closed after apply")
	}
}

// TestApplyRefreshReSortsToActiveSpec proves the async-refresh race is closed:
// a refresh delivers roots in build/default order, and applyRefresh must re-sort
// them to the UI's active custom spec before rows are recalculated.
func TestApplyRefreshReSortsToActiveSpec(t *testing.T) {
	mk := func(id string, prio int) *graph.Node {
		return &graph.Node{Issue: beads.FullIssue{ID: id, Priority: prio, Status: "open"}}
	}
	app := &App{
		roots:        []*graph.Node{mk("a", 0)},
		selectAnchor: -1,
		sortSpec:     graph.SortSpec{Key: graph.SortPriority, Desc: false},
		textInput:    textinput.New(),
	}
	app.recalcVisibleRows()

	newRoots := []*graph.Node{mk("b", 2), mk("a", 0), mk("c", 4)} // not priority-ordered
	app.applyRefresh(newRoots, buildIssueDigest(newRoots), time.Now())

	if got := rowIDs(app); !equalStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("post-refresh order = %v, want [a b c]", got)
	}
}

// TestFastInjectRespectsActiveSort proves an optimistically-injected bead lands
// in its sorted slot under a non-default sort (not the default insertion slot).
func TestFastInjectRespectsActiveSort(t *testing.T) {
	mk := func(id string, prio int) *graph.Node {
		return &graph.Node{Issue: beads.FullIssue{ID: id, Priority: prio, Status: "open"}}
	}
	app := &App{
		roots:        []*graph.Node{mk("b", 2), mk("c", 4)},
		selectAnchor: -1,
		sortSpec:     graph.SortSpec{Key: graph.SortPriority, Desc: false},
		textInput:    textinput.New(),
	}
	app.recalcVisibleRows()

	if err := app.fastInjectBead(beads.FullIssue{ID: "a", Priority: 0, Status: "open"}, ""); err != nil {
		t.Fatalf("fastInjectBead: %v", err)
	}
	if got := rowIDs(app); got[0] != "a" {
		t.Fatalf("expected P0 bead 'a' first under Priority asc, got %v", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
