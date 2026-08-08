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

// issueChangeSignal snapshots the two per-issue fields a targeted reconcile
// compares against a freshly read tree to decide whether a previously
// loaded issue's cache is still safe to keep (ab-6irx.6): UpdatedAt (bumped
// by any edit to the issue row itself -- description/notes/acceptance/close
// reason all live there) and CommentFingerprint (comments live in a
// separate table and don't bump UpdatedAt, so they need their own signal;
// see beads.FullIssue.CommentFingerprint).
type issueChangeSignal struct {
	updatedAt          string
	commentFingerprint string
}

// collectChangeSignals snapshots every node's UpdatedAt/CommentFingerprint
// from the tree, keyed by issue ID, for restoreUnchangedIssues to compare a
// freshly read tree against.
func collectChangeSignals(roots []*graph.Node) map[string]issueChangeSignal {
	signals := make(map[string]issueChangeSignal)
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			signals[n.Issue.ID] = issueChangeSignal{
				updatedAt:          n.Issue.UpdatedAt,
				commentFingerprint: n.Issue.CommentFingerprint,
			}
			walk(n.Children)
		}
	}
	walk(roots)
	return signals
}

// restoreUnchangedIssues is a Delta-capable client's targeted reconcile
// (ab-6irx.6): roots is a fresh full-Export skeleton (every node blank --
// Comments=nil, DetailLoaded=false, per validateSkeletonNotPreloaded), and
// this restores a node's cached comments/detail from old/oldSignals ONLY
// when BOTH (a) the prior load actually succeeded (commentsLoaded /
// detailLoaded true) and (b) the issue's content signal is unchanged since
// then (CommentFingerprint / UpdatedAt still match). Every other node is
// left at the fresh skeleton's blank state, which covers three cases at
// once without special-casing any of them:
//   - a changed issue (signal differs) -- forces a real reload of its new
//     content, exactly the freshness guarantee a manual 'r' promises;
//   - a brand new issue (absent from old/oldSignals) -- nothing to
//     restore, so it naturally starts blank like any other never-seen row;
//   - an issue whose last load attempt FAILED (CommentError/DetailError
//     set, loaded=false) -- also left blank, clearing the error and making
//     it eligible for retry again, matching the pre-existing "a full
//     reconcile clears errors" behavior (ab-6irx.1) rather than leaving a
//     transient failure stuck forever just because content never changed.
//
// This is what makes a manual 'r' fast on a large repo: only issues that
// actually changed (or previously failed) pay the reload cost again: every
// other already-loaded issue's cache survives untouched.
func restoreUnchangedIssues(
	roots []*graph.Node,
	oldComments map[string]commentState,
	oldDetails map[string]detailState,
	oldSignals map[string]issueChangeSignal,
) {
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			sig, hadSig := oldSignals[n.Issue.ID]
			if cs, ok := oldComments[n.Issue.ID]; ok && cs.commentsLoaded &&
				hadSig && sig.commentFingerprint == n.Issue.CommentFingerprint {
				n.Issue.Comments = cs.comments
				n.CommentsLoaded = true
			}
			if ds, ok := oldDetails[n.Issue.ID]; ok && ds.detailLoaded &&
				hadSig && sig.updatedAt == n.Issue.UpdatedAt {
				n.Issue.Description = ds.description
				n.Issue.Design = ds.design
				n.Issue.Notes = ds.notes
				n.Issue.AcceptanceCriteria = ds.acceptanceCriteria
				n.Issue.CloseReason = ds.closeReason
				n.Issue.ExternalRef = ds.externalRef
				n.Issue.DetailLoaded = true
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
