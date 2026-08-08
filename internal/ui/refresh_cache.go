package ui

import (
	"abacus/internal/beads"
	"abacus/internal/graph"
)

// commentState holds the comment data for a single issue.
type commentState struct {
	comments       []beads.Comment
	commentsLoaded bool
	commentError   string
}

// collectCommentState builds a map of issue ID -> comment state from the tree.
func collectCommentState(roots []*graph.Node) map[string]commentState {
	state := make(map[string]commentState)
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if n.CommentsLoaded || n.CommentError != "" {
				state[n.Issue.ID] = commentState{
					comments:       n.Issue.Comments,
					commentsLoaded: n.CommentsLoaded,
					commentError:   n.CommentError,
				}
			}
			walk(n.Children)
		}
	}
	walk(roots)
	return state
}

// transferCommentState applies previously collected comment state to new nodes.
func transferCommentState(roots []*graph.Node, state map[string]commentState) {
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if n.CommentsLoaded && n.Issue.Comments != nil {
				walk(n.Children)
				continue
			}
			if cs, ok := state[n.Issue.ID]; ok {
				n.Issue.Comments = cs.comments
				n.CommentsLoaded = cs.commentsLoaded
				n.CommentError = cs.commentError
			}
			walk(n.Children)
		}
	}
	walk(roots)
}

// transferCommentError carries forward only CommentError from state (not
// Comments/CommentsLoaded — Delta's own merge already handles that content
// correctly per-issue). Used on a Delta-capable client's incremental tick:
// CommentError is a Node-level field Delta cannot merge (it isn't part of
// beads.FullIssue), so without this a persistently-failing comment fetch's
// exclusion from loadCommentsInBackground's retry sweep (needsCommentFetch
// skips CommentError != "") would reset on every single tick.
func transferCommentError(roots []*graph.Node, state map[string]commentState) {
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if cs, ok := state[n.Issue.ID]; ok && cs.commentError != "" {
				n.CommentError = cs.commentError
			}
			walk(n.Children)
		}
	}
	walk(roots)
}

// invalidateCommentCache resets the comment cache for the given issue ID across
// the tree so the background loader re-fetches it on the next refresh. Called
// after adding a comment: without it, collectCommentState/transferCommentState
// copy the stale (often empty) comment list back onto the refreshed node and the
// just-added comment never appears (ab-udk6).
func invalidateCommentCache(roots []*graph.Node, issueID string) {
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if n.Issue.ID == issueID {
				n.Issue.Comments = nil
				n.CommentsLoaded = false
				n.CommentError = ""
			}
			walk(n.Children)
		}
	}
	walk(roots)
}

// detailState holds the lazily-loaded detail data for a single issue,
// mirroring commentState so applyRefresh can preserve it across the
// Export-driven tree rebuild the same way loaded comments are preserved
// (dolt skeleton/detail split, fold R03/F13).
type detailState struct {
	description        string
	design             string
	notes              string
	acceptanceCriteria string
	closeReason        string
	externalRef        string
	detailLoaded       bool
	detailError        string
}

// collectDetailState builds a map of issue ID -> detail state from the tree.
func collectDetailState(roots []*graph.Node) map[string]detailState {
	state := make(map[string]detailState)
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if n.Issue.DetailLoaded || n.DetailError != "" {
				state[n.Issue.ID] = detailState{
					description:        n.Issue.Description,
					design:             n.Issue.Design,
					notes:              n.Issue.Notes,
					acceptanceCriteria: n.Issue.AcceptanceCriteria,
					closeReason:        n.Issue.CloseReason,
					externalRef:        n.Issue.ExternalRef,
					detailLoaded:       n.Issue.DetailLoaded,
					detailError:        n.DetailError,
				}
			}
			walk(n.Children)
		}
	}
	walk(roots)
	return state
}

// transferDetailState applies previously collected detail state to new
// nodes, unless the fresh Export/Show already returned DetailLoaded=true for
// that node (Client.Show sets it directly; a future Reader could return it
// pre-loaded from Export too) — that fresh data wins over the cached copy.
func transferDetailState(roots []*graph.Node, state map[string]detailState) {
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if n.Issue.DetailLoaded {
				walk(n.Children)
				continue
			}
			if ds, ok := state[n.Issue.ID]; ok {
				n.Issue.Description = ds.description
				n.Issue.Design = ds.design
				n.Issue.Notes = ds.notes
				n.Issue.AcceptanceCriteria = ds.acceptanceCriteria
				n.Issue.CloseReason = ds.closeReason
				n.Issue.ExternalRef = ds.externalRef
				n.Issue.DetailLoaded = ds.detailLoaded
				n.DetailError = ds.detailError
			}
			walk(n.Children)
		}
	}
	walk(roots)
}

// transferDetailError carries forward only DetailError from state (not the
// detail content fields — Delta's own merge already handles those correctly
// per-issue via beads.FullIssue.DetailLoaded). Mirrors transferCommentError
// for the lazy detail load's Node-level error field.
func transferDetailError(roots []*graph.Node, state map[string]detailState) {
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if ds, ok := state[n.Issue.ID]; ok && ds.detailError != "" {
				n.DetailError = ds.detailError
			}
			walk(n.Children)
		}
	}
	walk(roots)
}

// invalidateDetailCache resets the detail cache for the given issue ID
// across the tree so the lazy loader re-fetches it on the next selection.
// Mirrors invalidateCommentCache; called after a successful write to the
// acted-on issue so a stale cached Description/Design/etc. never survives
// the write (design spec fold R02).
func invalidateDetailCache(roots []*graph.Node, issueID string) {
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if n.Issue.ID == issueID {
				n.Issue.Description = ""
				n.Issue.Design = ""
				n.Issue.Notes = ""
				n.Issue.AcceptanceCriteria = ""
				n.Issue.CloseReason = ""
				n.Issue.ExternalRef = ""
				n.Issue.DetailLoaded = false
				n.DetailError = ""
			}
			walk(n.Children)
		}
	}
	walk(roots)
}

// applyCommentsToNode sets freshly fetched comments on the matching node(s) and
// marks them loaded, so a just-added comment shows immediately and is preserved
// by collectCommentState/transferCommentState across the next refresh — without
// waiting on the background bulk loader (ab-j4pi.2).
func applyCommentsToNode(roots []*graph.Node, issueID string, comments []beads.Comment) {
	if comments == nil {
		comments = []beads.Comment{}
	}
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if n.Issue.ID == issueID {
				n.Issue.Comments = comments
				n.CommentsLoaded = true
				n.CommentError = ""
			}
			walk(n.Children)
		}
	}
	walk(roots)
}
