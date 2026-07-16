package graph

import (
	"testing"

	"abacus/internal/beads"
)

func sortIDs(nodes []*Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Issue.ID
	}
	return out
}

func eqIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func containsID(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestApplySortPriority(t *testing.T) {
	roots := []*Node{
		{Issue: beads.FullIssue{ID: "b", Priority: 2}},
		{Issue: beads.FullIssue{ID: "a", Priority: 0}},
		{Issue: beads.FullIssue{ID: "c", Priority: 4}},
	}
	ApplySort(roots, SortSpec{Key: SortPriority, Desc: false})
	eqIDs(t, sortIDs(roots), []string{"a", "b", "c"}) // P0,P2,P4 — urgent first

	ApplySort(roots, SortSpec{Key: SortPriority, Desc: true})
	eqIDs(t, sortIDs(roots), []string{"c", "b", "a"}) // P4,P2,P0
}

func TestApplySortCreated(t *testing.T) {
	roots := []*Node{
		{Issue: beads.FullIssue{ID: "mid", CreatedAt: "2024-02-01T00:00:00Z"}},
		{Issue: beads.FullIssue{ID: "old", CreatedAt: "2024-01-01T00:00:00Z"}},
		{Issue: beads.FullIssue{ID: "new", CreatedAt: "2024-03-01T00:00:00Z"}},
	}
	ApplySort(roots, SortSpec{Key: SortCreated, Desc: false}) // oldest first
	eqIDs(t, sortIDs(roots), []string{"old", "mid", "new"})
	ApplySort(roots, SortSpec{Key: SortCreated, Desc: true}) // newest first
	eqIDs(t, sortIDs(roots), []string{"new", "mid", "old"})
}

func TestApplySortUpdated(t *testing.T) {
	roots := []*Node{
		{Issue: beads.FullIssue{ID: "a", UpdatedAt: "2024-01-01T00:00:00Z"}},
		{Issue: beads.FullIssue{ID: "b", UpdatedAt: "2024-03-01T00:00:00Z"}},
	}
	ApplySort(roots, SortSpec{Key: SortUpdated, Desc: true})
	eqIDs(t, sortIDs(roots), []string{"b", "a"})
	ApplySort(roots, SortSpec{Key: SortUpdated, Desc: false})
	eqIDs(t, sortIDs(roots), []string{"a", "b"})
}

func TestApplySortMissingDatesLast(t *testing.T) {
	// empty + malformed timestamps sort LAST in BOTH directions.
	for _, desc := range []bool{false, true} {
		roots := []*Node{
			{Issue: beads.FullIssue{ID: "empty", CreatedAt: ""}},
			{Issue: beads.FullIssue{ID: "good", CreatedAt: "2024-01-01T00:00:00Z"}},
			{Issue: beads.FullIssue{ID: "bad", CreatedAt: "not-a-date"}},
		}
		ApplySort(roots, SortSpec{Key: SortCreated, Desc: desc})
		if roots[0].Issue.ID != "good" {
			t.Fatalf("desc=%v: expected good first, got %v", desc, sortIDs(roots))
		}
		last := sortIDs(roots)[1:]
		if !containsID(last, "empty") || !containsID(last, "bad") {
			t.Fatalf("desc=%v: expected empty+bad last, got %v", desc, sortIDs(roots))
		}
	}
}

func TestApplySortHierarchical(t *testing.T) {
	// children must be re-sorted, not just roots.
	root := &Node{Issue: beads.FullIssue{ID: "epic", Priority: 2}, Children: []*Node{
		{Issue: beads.FullIssue{ID: "c2", Priority: 3}},
		{Issue: beads.FullIssue{ID: "c0", Priority: 0}},
		{Issue: beads.FullIssue{ID: "c1", Priority: 1}},
	}}
	ApplySort([]*Node{root}, SortSpec{Key: SortPriority})
	eqIDs(t, sortIDs(root.Children), []string{"c0", "c1", "c2"})
}

func TestApplySortEpicOwnField(t *testing.T) {
	// An epic ranks by its OWN priority, not its most-urgent descendant.
	epicHigh := &Node{Issue: beads.FullIssue{ID: "epicA", Priority: 1}, Children: []*Node{
		{Issue: beads.FullIssue{ID: "a-child", Priority: 4}},
	}}
	epicLowUrgentChild := &Node{Issue: beads.FullIssue{ID: "epicB", Priority: 3}, Children: []*Node{
		{Issue: beads.FullIssue{ID: "b-child", Priority: 0}},
	}}
	roots := []*Node{epicLowUrgentChild, epicHigh}
	ApplySort(roots, SortSpec{Key: SortPriority})
	eqIDs(t, sortIDs(roots), []string{"epicA", "epicB"}) // P1 epic before P3 epic
}

func TestApplySortDefaultUnchanged(t *testing.T) {
	// SortDefault must reproduce the legacy computeSortMetrics + sortNodes path.
	build := func() []*Node {
		return []*Node{
			{Issue: beads.FullIssue{ID: "ip", Status: "in_progress", UpdatedAt: "2024-01-02T00:00:00Z"}},
			{Issue: beads.FullIssue{ID: "closed", Status: "closed", ClosedAt: "2024-01-03T00:00:00Z"}},
			{Issue: beads.FullIssue{ID: "ready", Status: "open", CreatedAt: "2024-01-01T00:00:00Z"}},
		}
	}
	legacy := build()
	for _, r := range legacy {
		computeSortMetrics(r)
	}
	sortNodes(legacy)

	viaApply := build()
	ApplySort(viaApply, SortSpec{}) // zero value = Default
	eqIDs(t, sortIDs(viaApply), sortIDs(legacy))
}

func TestApplySortEqualAndTZEquivalentTimes(t *testing.T) {
	// Same instant in different zones must compare equal -> ID tiebreak, both dirs.
	for _, desc := range []bool{false, true} {
		roots := []*Node{
			{Issue: beads.FullIssue{ID: "z", CreatedAt: "2024-01-01T01:00:00+01:00"}},
			{Issue: beads.FullIssue{ID: "a", CreatedAt: "2024-01-01T00:00:00Z"}}, // same instant as z
		}
		ApplySort(roots, SortSpec{Key: SortCreated, Desc: desc})
		eqIDs(t, sortIDs(roots), []string{"a", "z"})
	}
}

func TestApplySortDefaultUnchangedNested(t *testing.T) {
	// Default must reproduce the legacy order for nested children, not just roots.
	build := func() []*Node {
		return []*Node{
			{Issue: beads.FullIssue{ID: "epic", Status: "open", CreatedAt: "2024-01-01T00:00:00Z"}, Children: []*Node{
				{Issue: beads.FullIssue{ID: "c-ip", Status: "in_progress", UpdatedAt: "2024-02-01T00:00:00Z"}},
				{Issue: beads.FullIssue{ID: "c-closed", Status: "closed", ClosedAt: "2024-03-01T00:00:00Z"}},
				{Issue: beads.FullIssue{ID: "c-ready", Status: "open", CreatedAt: "2024-01-15T00:00:00Z"}},
			}},
		}
	}
	legacy := build()
	for _, r := range legacy {
		computeSortMetrics(r)
	}
	sortNodes(legacy)

	viaApply := build()
	ApplySort(viaApply, SortSpec{})
	eqIDs(t, sortIDs(viaApply[0].Children), sortIDs(legacy[0].Children))
}

func TestSortSpecValid(t *testing.T) {
	if !(SortSpec{Key: SortPriority}).Valid() {
		t.Fatal("SortPriority should be valid")
	}
	if (SortSpec{Key: SortKey(99)}).Valid() {
		t.Fatal("out-of-range key should be invalid")
	}
}

func TestSortSpecStringRoundTrip(t *testing.T) {
	specs := []SortSpec{
		{SortDefault, false},
		{SortPriority, false}, {SortPriority, true},
		{SortCreated, true}, {SortCreated, false},
		{SortUpdated, true}, {SortUpdated, false},
	}
	for _, s := range specs {
		got, ok := ParseSortSpec(s.String())
		if !ok || got != s {
			t.Fatalf("round-trip %v -> %q -> %v (ok=%v)", s, s.String(), got, ok)
		}
	}
	if _, ok := ParseSortSpec("bogus"); ok {
		t.Fatal("bogus token should not parse")
	}
}
