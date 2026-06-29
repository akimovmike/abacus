package ui

import (
	"testing"

	"abacus/internal/beads"
	"abacus/internal/graph"
)

func TestCollectCommentState(t *testing.T) {
	// Build a tree with comments loaded
	roots := []*graph.Node{
		{
			Issue: beads.FullIssue{
				ID:    "ab-001",
				Title: "Root Issue",
				Comments: []beads.Comment{
					{ID: "1", Text: "comment 1"},
					{ID: "2", Text: "comment 2"},
				},
			},
			CommentsLoaded: true,
			Children: []*graph.Node{
				{
					Issue: beads.FullIssue{
						ID:    "ab-002",
						Title: "Child Issue",
						Comments: []beads.Comment{
							{ID: "3", Text: "child comment"},
						},
					},
					CommentsLoaded: true,
				},
			},
		},
		{
			Issue: beads.FullIssue{
				ID:    "ab-003",
				Title: "Not loaded",
			},
			CommentsLoaded: false,
		},
	}

	state := collectCommentState(roots)

	// Should have entries for ab-001 and ab-002, but not ab-003
	if len(state) != 2 {
		t.Fatalf("expected 2 entries in state, got %d", len(state))
	}

	s1, ok := state["ab-001"]
	if !ok {
		t.Fatal("expected state for ab-001")
	}
	if len(s1.comments) != 2 {
		t.Fatalf("expected 2 comments for ab-001, got %d", len(s1.comments))
	}
	if !s1.commentsLoaded {
		t.Fatal("expected commentsLoaded=true for ab-001")
	}

	s2, ok := state["ab-002"]
	if !ok {
		t.Fatal("expected state for ab-002")
	}
	if len(s2.comments) != 1 {
		t.Fatalf("expected 1 comment for ab-002, got %d", len(s2.comments))
	}

	if _, ok := state["ab-003"]; ok {
		t.Fatal("should not have state for ab-003 (not loaded)")
	}
}

func TestCollectCommentStateWithError(t *testing.T) {
	roots := []*graph.Node{
		{
			Issue: beads.FullIssue{
				ID:    "ab-err",
				Title: "Error Issue",
			},
			CommentsLoaded: false,
			CommentError:   "failed to load",
		},
	}

	state := collectCommentState(roots)

	// Should capture the error state too
	if len(state) != 1 {
		t.Fatalf("expected 1 entry in state, got %d", len(state))
	}
	s, ok := state["ab-err"]
	if !ok {
		t.Fatal("expected state for ab-err")
	}
	if s.commentError != "failed to load" {
		t.Fatalf("expected error message, got %q", s.commentError)
	}
}

func TestTransferCommentState(t *testing.T) {
	// Old state with comments
	oldState := map[string]commentState{
		"ab-001": {
			comments: []beads.Comment{
				{ID: "1", Text: "preserved comment"},
			},
			commentsLoaded: true,
		},
		"ab-002": {
			commentError: "some error",
		},
	}

	// Fresh nodes without comments
	newRoots := []*graph.Node{
		{
			Issue: beads.FullIssue{ID: "ab-001", Title: "Issue 1"},
			Children: []*graph.Node{
				{Issue: beads.FullIssue{ID: "ab-002", Title: "Issue 2"}},
			},
		},
		{Issue: beads.FullIssue{ID: "ab-003", Title: "New Issue"}},
	}

	transferCommentState(newRoots, oldState)

	// Check ab-001 got its comments back
	if !newRoots[0].CommentsLoaded {
		t.Fatal("expected ab-001 to have CommentsLoaded=true")
	}
	if len(newRoots[0].Issue.Comments) != 1 {
		t.Fatalf("expected 1 comment for ab-001, got %d", len(newRoots[0].Issue.Comments))
	}
	if newRoots[0].Issue.Comments[0].Text != "preserved comment" {
		t.Fatalf("expected preserved comment, got %q", newRoots[0].Issue.Comments[0].Text)
	}

	// Check ab-002 got its error state back
	if newRoots[0].Children[0].CommentError != "some error" {
		t.Fatalf("expected error for ab-002, got %q", newRoots[0].Children[0].CommentError)
	}

	// Check ab-003 was not modified (not in old state)
	if newRoots[1].CommentsLoaded {
		t.Fatal("ab-003 should not have comments loaded")
	}
	if len(newRoots[1].Issue.Comments) != 0 {
		t.Fatal("ab-003 should have no comments")
	}
}

func TestTransferCommentStateKeepsFreshExportedComments(t *testing.T) {
	oldState := map[string]commentState{
		"ab-001": {
			comments:       []beads.Comment{{ID: "1", Text: "old comment"}},
			commentsLoaded: true,
		},
	}
	newRoots := []*graph.Node{
		{
			Issue: beads.FullIssue{
				ID:       "ab-001",
				Title:    "Issue 1",
				Comments: []beads.Comment{{ID: "2", Text: "fresh export comment"}},
			},
			CommentsLoaded: true,
		},
	}

	transferCommentState(newRoots, oldState)

	if len(newRoots[0].Issue.Comments) != 1 {
		t.Fatalf("expected 1 fresh comment, got %d", len(newRoots[0].Issue.Comments))
	}
	if got := newRoots[0].Issue.Comments[0].Text; got != "fresh export comment" {
		t.Fatalf("expected fresh export comment to win, got %q", got)
	}
}

func TestCommentStatePreservedAcrossRefresh(t *testing.T) {
	// Simulate a full refresh cycle - this is an integration test of the
	// collect -> transfer flow that happens in applyRefresh

	// Start with nodes that have comments loaded
	originalRoots := []*graph.Node{
		{
			Issue: beads.FullIssue{
				ID:    "ab-001",
				Title: "Original Title",
				Comments: []beads.Comment{
					{ID: "1", Text: "comment A"},
					{ID: "2", Text: "comment B"},
				},
			},
			CommentsLoaded: true,
		},
	}

	// Collect comment state (this happens before replacing roots)
	savedState := collectCommentState(originalRoots)

	// Simulate a refresh - new nodes come in without comments
	refreshedRoots := []*graph.Node{
		{
			Issue: beads.FullIssue{
				ID:       "ab-001",
				Title:    "Updated Title", // Title changed but ID same
				Comments: nil,             // No comments on fresh node
			},
			CommentsLoaded: false,
		},
	}

	// Transfer state to new nodes (this happens after replacing roots)
	transferCommentState(refreshedRoots, savedState)

	// Verify comments were preserved
	if !refreshedRoots[0].CommentsLoaded {
		t.Fatal("CommentsLoaded should be true after transfer")
	}
	if len(refreshedRoots[0].Issue.Comments) != 2 {
		t.Fatalf("expected 2 comments preserved, got %d", len(refreshedRoots[0].Issue.Comments))
	}
	if refreshedRoots[0].Issue.Comments[0].Text != "comment A" {
		t.Fatalf("expected 'comment A', got %q", refreshedRoots[0].Issue.Comments[0].Text)
	}
}

// TestInvalidateCommentCache verifies that invalidating an issue's comment
// cache resets the node so the background loader re-fetches it, and (critically)
// that the invalidated node is no longer captured by collectCommentState — which
// is what previously re-masked a freshly added comment across a refresh (ab-udk6).
func TestInvalidateCommentCache(t *testing.T) {
	roots := []*graph.Node{
		{
			Issue:          beads.FullIssue{ID: "ab-001", Comments: []beads.Comment{{ID: "1", Text: "old"}}},
			CommentsLoaded: true,
			Children: []*graph.Node{
				{
					Issue:          beads.FullIssue{ID: "ab-002"},
					CommentsLoaded: true,
					CommentError:   "boom",
				},
			},
		},
	}

	invalidateCommentCache(roots, "ab-002")

	child := roots[0].Children[0]
	if child.CommentsLoaded {
		t.Fatal("ab-002 CommentsLoaded should be false after invalidate")
	}
	if child.CommentError != "" {
		t.Fatalf("ab-002 CommentError should be cleared, got %q", child.CommentError)
	}
	if child.Issue.Comments != nil {
		t.Fatal("ab-002 Comments should be nil after invalidate")
	}

	if !roots[0].CommentsLoaded {
		t.Fatal("ab-001 should be untouched by invalidate of ab-002")
	}

	// Regression: an invalidated node must not be captured here, otherwise
	// transferCommentState would copy the stale empty list back onto the
	// refreshed node and hide the just-added comment.
	state := collectCommentState(roots)
	if _, ok := state["ab-002"]; ok {
		t.Fatal("invalidated ab-002 must not be captured by collectCommentState")
	}
}

// TestApplyCommentsToNode verifies a freshly fetched comment set is applied to
// the matching node and marked loaded, so it survives the next refresh (ab-j4pi.2).
func TestApplyCommentsToNode(t *testing.T) {
	roots := []*graph.Node{
		{Issue: beads.FullIssue{ID: "ab-001"}},
	}

	applyCommentsToNode(roots, "ab-001", []beads.Comment{{ID: "1", Text: "fresh"}})

	n := roots[0]
	if !n.CommentsLoaded {
		t.Fatal("expected CommentsLoaded=true after apply")
	}
	if len(n.Issue.Comments) != 1 || n.Issue.Comments[0].Text != "fresh" {
		t.Fatalf("expected the fetched comment applied, got %+v", n.Issue.Comments)
	}
	if n.CommentError != "" {
		t.Fatalf("expected CommentError cleared, got %q", n.CommentError)
	}

	// Loaded + non-empty must be captured so transfer preserves it across refresh.
	state := collectCommentState(roots)
	if _, ok := state["ab-001"]; !ok {
		t.Fatal("expected applied comments to be captured by collectCommentState")
	}
}
