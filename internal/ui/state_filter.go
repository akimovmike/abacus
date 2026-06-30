package ui

import (
	"strings"

	"abacus/internal/domain"
	"abacus/internal/graph"
)

func nodeMatchesFilter(filterLower string, node *graph.Node) bool {
	if filterLower == "" {
		return true
	}

	titleLower := strings.ToLower(node.Issue.Title)
	if strings.Contains(titleLower, filterLower) {
		return true
	}

	idLower := strings.ToLower(node.Issue.ID)
	if strings.Contains(idLower, filterLower) {
		return true
	}

	trimmed := strings.TrimPrefix(idLower, "ab-")
	return strings.Contains(trimmed, filterLower)
}

// nodeMatchesLabelFilter reports whether the node carries the given label.
// An empty filter matches everything. Matching is exact (not substring) so
// "back" does not match "backend".
func nodeMatchesLabelFilter(labelFilter string, node *graph.Node) bool {
	if labelFilter == "" {
		return true
	}
	for _, l := range node.Issue.Labels {
		if l == labelFilter {
			return true
		}
	}
	return false
}

// nodeMatchesAssigneeFilter reports whether the node's assignee equals the
// given value. An empty filter matches everything.
func nodeMatchesAssigneeFilter(assigneeFilter string, node *graph.Node) bool {
	if assigneeFilter == "" {
		return true
	}
	return node.Issue.Assignee == assigneeFilter
}

// viewModeDef is one entry in the view-mode table: a display name and a
// predicate reporting whether an issue is shown (true = keep).
type viewModeDef struct {
	name string
	keep func(status string, isBlocked bool) bool
}

// viewModeDefs is the single source of truth for view modes. It is keyed by
// the ViewMode consts (app.go) so each name+predicate is bound to its enum
// value and cannot drift. len(viewModeDefs) drives the Next/Prev cycle.
//
// The status checks are plain string comparisons against domain status values;
// "Ready" (open && !blocked) is exactly domain.Issue.IsReady(), so this needs
// no domain.NewIssueFromFull conversion. Unknown statuses (e.g. "reviewing")
// fall through every "not X" check and stay visible except in Ready.
var viewModeDefs = [...]viewModeDef{
	ViewModeAll: {"All", func(string, bool) bool { return true }},
	ViewModeNotClosed: {"Not Closed", func(s string, _ bool) bool {
		return s != string(domain.StatusClosed)
	}},
	ViewModeActive: {"Active", func(s string, _ bool) bool {
		return s != string(domain.StatusClosed) &&
			s != string(domain.StatusBlocked) &&
			s != string(domain.StatusDeferred)
	}},
	ViewModeReady: {"Ready", func(s string, isBlocked bool) bool {
		return s == string(domain.StatusOpen) && !isBlocked
	}},
}

// nodeMatchesViewMode checks if a node matches the current view mode filter.
func nodeMatchesViewMode(mode ViewMode, node *graph.Node) bool {
	if !mode.valid() || viewModeDefs[mode].keep == nil {
		return true // guard: an unknown/incomplete mode never hides on a bug
	}
	return viewModeDefs[mode].keep(node.Issue.Status, node.IsBlocked)
}

func (m *App) computeFilterEval(filterLower string) map[string]filterEvaluation {
	evals := make(map[string]filterEvaluation)
	var walk func(node *graph.Node) bool
	walk = func(node *graph.Node) bool {
		// Node matches only if it satisfies every active dimension:
		// view mode, text, label, and assignee.
		viewModeMatch := nodeMatchesViewMode(m.viewMode, node)
		textMatch := nodeMatchesFilter(filterLower, node)
		labelMatch := nodeMatchesLabelFilter(m.labelFilter, node)
		assigneeMatch := nodeMatchesAssigneeFilter(m.assigneeFilter, node)
		directMatch := viewModeMatch && textMatch && labelMatch && assigneeMatch

		hasChildMatch := false
		for _, child := range node.Children {
			if walk(child) {
				hasChildMatch = true
			}
		}
		evals[node.Issue.ID] = filterEvaluation{
			matches:          directMatch,
			hasMatchingChild: hasChildMatch,
		}
		return directMatch || hasChildMatch
	}
	for _, root := range m.roots {
		walk(root)
	}
	return evals
}

func (m *App) shouldExpandFilteredRow(row graph.TreeRow, hasMatchingChild bool) bool {
	node := row.Node
	if len(node.Children) == 0 {
		return false
	}
	key := treeRowStateKey(row)
	if m.filterCollapsed != nil && m.filterCollapsed[key] {
		return false
	}
	if m.filterForcedExpanded != nil && m.filterForcedExpanded[key] {
		return true
	}
	if hasMatchingChild {
		return true
	}
	return m.isRowExpandedForTraversal(row)
}
