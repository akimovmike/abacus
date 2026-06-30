package ui

import (
	"testing"

	"abacus/internal/beads"
	"abacus/internal/graph"
)

func TestNodeMatchesLabelFilter(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{Labels: []string{"backend", "urgent"}}}
	tests := []struct {
		name   string
		filter string
		want   bool
	}{
		{"empty filter matches all", "", true},
		{"exact label present", "backend", true},
		{"other label present", "urgent", true},
		{"label absent", "frontend", false},
		{"substring is not a match", "back", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeMatchesLabelFilter(tt.filter, node); got != tt.want {
				t.Errorf("nodeMatchesLabelFilter(%q) = %v, want %v", tt.filter, got, tt.want)
			}
		})
	}
}

func TestNodeMatchesAssigneeFilter(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{Assignee: "Claude"}}
	tests := []struct {
		name   string
		filter string
		want   bool
	}{
		{"empty filter matches all", "", true},
		{"exact assignee", "Claude", true},
		{"different assignee", "Mikhail", false},
		{"substring is not a match", "Cla", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeMatchesAssigneeFilter(tt.filter, node); got != tt.want {
				t.Errorf("nodeMatchesAssigneeFilter(%q) = %v, want %v", tt.filter, got, tt.want)
			}
		})
	}
}

func TestComputeFilterEvalCombinesLabelAndAssignee(t *testing.T) {
	a := &graph.Node{Issue: beads.FullIssue{ID: "ab-1", Title: "alpha", Status: "open", Labels: []string{"backend"}, Assignee: "Claude"}}
	b := &graph.Node{Issue: beads.FullIssue{ID: "ab-2", Title: "beta", Status: "open", Labels: []string{"backend"}, Assignee: "Mikhail"}}
	c := &graph.Node{Issue: beads.FullIssue{ID: "ab-3", Title: "gamma", Status: "open", Labels: []string{"frontend"}, Assignee: "Claude"}}

	app := &App{
		roots:          []*graph.Node{a, b, c},
		viewMode:       ViewModeAll,
		labelFilter:    "backend",
		assigneeFilter: "Claude",
		keys:           DefaultKeyMap(),
	}
	app.recalcVisibleRows()

	// Only ab-1 satisfies both label=backend AND assignee=Claude.
	if len(app.visibleRows) != 1 {
		t.Fatalf("expected 1 visible row, got %d", len(app.visibleRows))
	}
	if app.visibleRows[0].Node.Issue.ID != "ab-1" {
		t.Errorf("expected ab-1, got %s", app.visibleRows[0].Node.Issue.ID)
	}
}

func TestComputeFilterEvalKeepsAncestorOfLabelMatch(t *testing.T) {
	child := &graph.Node{Issue: beads.FullIssue{ID: "ab-c", Title: "child", Status: "open", Labels: []string{"backend"}}}
	parent := &graph.Node{Issue: beads.FullIssue{ID: "ab-p", Title: "parent", Status: "open", Labels: []string{"frontend"}}, Children: []*graph.Node{child}}

	app := &App{
		roots:       []*graph.Node{parent},
		viewMode:    ViewModeAll,
		labelFilter: "backend",
		keys:        DefaultKeyMap(),
	}
	app.recalcVisibleRows()

	// Parent does not match but is kept as ancestor of the matching child.
	ids := map[string]bool{}
	for _, r := range app.visibleRows {
		ids[r.Node.Issue.ID] = true
	}
	if !ids["ab-p"] || !ids["ab-c"] {
		t.Errorf("expected parent and child visible, got rows %v", ids)
	}
}
