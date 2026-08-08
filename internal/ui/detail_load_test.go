package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"abacus/internal/beads"
	"abacus/internal/graph"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// --- MUST: selecting a skeleton-only issue triggers a lazy detail load,
// and the detail pane shows a "loading" affordance rather than a blank. ---

func TestSelectingSkeletonIssueTriggersDetailLoad(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{ID: "ab-1", Title: "Skeleton only"}}
	app := &App{
		ShowDetails: true,
		visibleRows: nodesToRows(node),
		client:      beads.NewMockClient(),
	}

	cmd := app.maybeTriggerDetailLoad()

	if cmd == nil {
		t.Fatal("expected a detail-load command for a skeleton-only selected issue")
	}
	if !app.detailLoadInFlight["ab-1"] {
		t.Fatal("expected detailLoadInFlight to be set for the selected issue")
	}
}

func TestDetailLoadSkippedWhenAlreadyLoaded(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{ID: "ab-1", DetailLoaded: true}}
	app := &App{
		ShowDetails: true,
		visibleRows: nodesToRows(node),
		client:      beads.NewMockClient(),
	}

	if cmd := app.maybeTriggerDetailLoad(); cmd != nil {
		t.Fatal("expected no detail-load command once DetailLoaded is true")
	}
}

func TestDetailLoadSkippedWhileInFlight(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{ID: "ab-1"}}
	app := &App{
		ShowDetails:        true,
		visibleRows:        nodesToRows(node),
		client:             beads.NewMockClient(),
		detailLoadInFlight: map[string]bool{"ab-1": true},
	}

	if cmd := app.maybeTriggerDetailLoad(); cmd != nil {
		t.Fatal("expected no second detail-load command while one is already in flight")
	}
}

func TestDetailLoadSkippedWhenNoClient(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{ID: "ab-1"}}
	app := &App{ShowDetails: true, visibleRows: nodesToRows(node)}

	if cmd := app.maybeTriggerDetailLoad(); cmd != nil {
		t.Fatal("expected no detail-load command when the app has no client")
	}
}

func TestDetailLoadSkippedAfterPriorError(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{ID: "ab-1"}, DetailError: "failed: boom"}
	app := &App{
		ShowDetails: true,
		visibleRows: nodesToRows(node),
		client:      beads.NewMockClient(),
	}

	if cmd := app.maybeTriggerDetailLoad(); cmd != nil {
		t.Fatal("expected no automatic retry for an already-errored detail load")
	}
}

// TestUpdateViewportContentShowsLoadingAffordanceForSkeletonIssue is the
// scenario from the task-11 brief: a skeleton issue (DetailLoaded == false)
// must render a non-blank "loading" placeholder, not an empty Description.
func TestUpdateViewportContentShowsLoadingAffordanceForSkeletonIssue(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{ID: "ab-1", Title: "Skeleton only", Status: "open"}}
	app := &App{
		ShowDetails:  true,
		visibleRows:  nodesToRows(node),
		viewport:     viewport.New(80, 30),
		outputFormat: "plain",
	}

	app.updateViewportContent()
	content := stripANSI(app.viewport.View())

	if !strings.Contains(content, "Loading details") {
		t.Fatalf("expected a loading affordance for a skeleton-only issue:\n%s", content)
	}
}

// TestDetailLoadedMsgPopulatesDescription completes the round trip: once
// Client.Show returns, the node's Description/Comments populate, DetailLoaded
// flips true, the in-flight guard clears, and the detail pane shows the text.
func TestDetailLoadedMsgPopulatesDescription(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{ID: "ab-1", Title: "Skeleton only", Status: "open"}}
	app := &App{
		ShowDetails:        true,
		visibleRows:        nodesToRows(node),
		roots:              []*graph.Node{node},
		viewport:           viewport.New(80, 30),
		outputFormat:       "plain",
		detailLoadInFlight: map[string]bool{"ab-1": true},
	}

	loaded := beads.FullIssue{
		ID:           "ab-1",
		Description:  "The real description.",
		Comments:     []beads.Comment{{Author: "qa", Text: "looks good"}},
		DetailLoaded: true,
	}
	model, _ := app.Update(detailLoadedMsg{issueID: "ab-1", issue: loaded})
	app = model.(*App)

	if app.detailLoadInFlight["ab-1"] {
		t.Fatal("expected detailLoadInFlight to be cleared once the load completes")
	}
	if !node.Issue.DetailLoaded {
		t.Fatal("expected DetailLoaded to be true after a successful load")
	}
	content := stripANSI(app.viewport.View())
	if !strings.Contains(content, "The real description.") {
		t.Fatalf("expected the loaded description to render:\n%s", content)
	}
	if !node.CommentsLoaded {
		t.Fatal("expected CommentsLoaded to be set since Show loads comments too")
	}
}

func TestDetailLoadedMsgErrorSetsDetailError(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{ID: "ab-1"}}
	app := &App{
		roots:              []*graph.Node{node},
		detailLoadInFlight: map[string]bool{"ab-1": true},
	}

	model, _ := app.Update(detailLoadedMsg{issueID: "ab-1", err: errors.New("boom")})
	app = model.(*App)

	if app.detailLoadInFlight["ab-1"] {
		t.Fatal("expected detailLoadInFlight to be cleared even on error")
	}
	if node.DetailError == "" {
		t.Fatal("expected DetailError to be set after a failed load")
	}
}

func TestLoadDetailCmdReturnsShowResult(t *testing.T) {
	client := beads.NewMockClient()
	client.ShowFn = func(_ context.Context, ids []string) ([]beads.FullIssue, error) {
		if len(ids) != 1 || ids[0] != "ab-1" {
			t.Fatalf("expected Show to be called with [ab-1], got %v", ids)
		}
		return []beads.FullIssue{{ID: "ab-1", Description: "loaded", DetailLoaded: true}}, nil
	}
	app := &App{client: client}

	msg := app.loadDetailCmd("ab-1")()
	dl, ok := msg.(detailLoadedMsg)
	if !ok {
		t.Fatalf("expected detailLoadedMsg, got %T", msg)
	}
	if dl.err != nil || dl.issue.Description != "loaded" {
		t.Fatalf("unexpected result: %+v", dl)
	}
}

// --- Cache invalidation (item 2): a successful write must invalidate the
// acted-on issue's detail+comment cache so the next view re-loads fresh. ---

func TestInvalidateDetailCacheResetsLoadedState(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{
		ID:           "ab-1",
		Description:  "stale",
		DetailLoaded: true,
	}}

	invalidateDetailCache([]*graph.Node{node}, "ab-1")

	if node.Issue.DetailLoaded {
		t.Fatal("expected DetailLoaded to be reset to false")
	}
	if node.Issue.Description != "" {
		t.Fatalf("expected Description to be cleared, got %q", node.Issue.Description)
	}
}

func TestStatusUpdateCompleteInvalidatesDetailCache(t *testing.T) {
	node := &graph.Node{Issue: beads.FullIssue{
		ID:           "ab-1",
		Description:  "stale",
		DetailLoaded: true,
	}}
	app := &App{roots: []*graph.Node{node}, client: beads.NewMockClient()}

	model, cmd := app.Update(statusUpdateCompleteMsg{issueID: "ab-1"})
	app = model.(*App)

	if cmd == nil {
		t.Fatal("expected a forceRefresh command after a successful status update")
	}
	if node.Issue.DetailLoaded {
		t.Fatal("expected the acted-on issue's detail cache to be invalidated")
	}
}

// --- SHOULD: bd 1.1.x migrate-gate errors surface an actionable toast. ---

func TestIsMigrateGateErrorDetectsMarkers(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"refusing-to-auto-apply", errors.New("bd update: refusing to auto-apply migration"), true},
		{"env-var-marker", errors.New("set BD_ALLOW_REMOTE_MIGRATE=1 to proceed"), true},
		{"warning-marker", errors.New("Warning: schema drift detected"), true},
		{"unrelated-error", errors.New("bd update: issue not found"), false},
		{"nil-error", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMigrateGateError(tc.err); got != tc.want {
				t.Errorf("isMigrateGateError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestShowOperationErrorUsesActionableMessageForMigrateGate(t *testing.T) {
	app := &App{}
	cmd := app.showOperationError(errors.New("bd update: refusing to auto-apply migration"))

	if cmd == nil {
		t.Fatal("expected a toast-tick command")
	}
	if app.lastError != migrateGateToastMessage {
		t.Fatalf("expected the actionable migrate-gate message, got %q", app.lastError)
	}
}

func TestShowOperationErrorPassesThroughOtherErrors(t *testing.T) {
	app := &App{}
	_ = app.showOperationError(errors.New("bd update: issue not found"))

	if app.lastError != "bd update: issue not found" {
		t.Fatalf("expected the raw error message, got %q", app.lastError)
	}
}

var _ tea.Msg = detailLoadedMsg{}
