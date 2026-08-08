package ui

import (
	"context"
	"testing"
	"time"

	"abacus/internal/beads"
	"abacus/internal/graph"
)

// deltaStubClient is a Delta-capable test double: it embeds MockClient (so
// Export/Comments/etc. behave exactly as they do in every other test) and
// adds a Delta method, exercising the incremental-refresh code path a plain
// MockClient can never reach — real MockClient, like the sqlite backend,
// does not implement Delta at all.
type deltaStubClient struct {
	*beads.MockClient
	deltaCallCount int
	deltaFn        func(ctx context.Context, prev []beads.FullIssue) ([]beads.FullIssue, error)
}

func newDeltaStubClient() *deltaStubClient {
	return &deltaStubClient{MockClient: beads.NewMockClient()}
}

func (d *deltaStubClient) Delta(ctx context.Context, prev []beads.FullIssue) ([]beads.FullIssue, error) {
	d.deltaCallCount++
	if d.deltaFn != nil {
		return d.deltaFn(ctx, prev)
	}
	return prev, nil
}

var _ beads.Client = (*deltaStubClient)(nil)
var _ deltaClient = (*deltaStubClient)(nil)

func TestFlattenIssuesDedupsMultiParentNodes(t *testing.T) {
	shared := &graph.Node{Issue: beads.FullIssue{ID: "ab-shared", Title: "Shared"}}
	parentA := &graph.Node{Issue: beads.FullIssue{ID: "ab-a"}, Children: []*graph.Node{shared}}
	parentB := &graph.Node{Issue: beads.FullIssue{ID: "ab-b"}, Children: []*graph.Node{shared}}
	shared.Parents = []*graph.Node{parentA, parentB}

	got := flattenIssues([]*graph.Node{parentA, parentB})

	counts := map[string]int{}
	for _, iss := range got {
		counts[iss.ID]++
	}
	if counts["ab-shared"] != 1 {
		t.Fatalf("expected ab-shared to appear exactly once, got %d", counts["ab-shared"])
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 unique issues, got %d: %+v", len(got), got)
	}
}

func TestFlattenIssuesEmptyRootsReturnsEmpty(t *testing.T) {
	got := flattenIssues(nil)
	if len(got) != 0 {
		t.Fatalf("expected no issues, got %d", len(got))
	}
}

func TestReconcileDueNoDeltaClientAlwaysTrue(t *testing.T) {
	app := &App{client: beads.NewMockClient()}
	for i := 0; i < 3; i++ {
		if !app.reconcileDue() {
			t.Fatalf("expected reconcileDue=true for a non-Delta client (iteration %d)", i)
		}
		app.deltaTicksSinceReconcile++
	}
}

func TestReconcileDueDeltaClientBoundedCadence(t *testing.T) {
	app := &App{client: newDeltaStubClient()}
	for i := 0; i < reconcileEveryTicks; i++ {
		if app.reconcileDue() {
			t.Fatalf("expected reconcileDue=false before the cadence threshold (tick %d)", i)
		}
		app.deltaTicksSinceReconcile++
	}
	if !app.reconcileDue() {
		t.Fatal("expected reconcileDue=true once deltaTicksSinceReconcile reaches reconcileEveryTicks")
	}
}

func TestFetchRefreshIssuesUsesDeltaWhenNotReconciling(t *testing.T) {
	client := newDeltaStubClient()
	prev := []beads.FullIssue{{ID: "ab-1"}}

	if _, err := fetchRefreshIssues(context.Background(), client, prev, false); err != nil {
		t.Fatalf("fetchRefreshIssues: %v", err)
	}
	if client.deltaCallCount != 1 {
		t.Fatalf("expected Delta to be called once, got %d", client.deltaCallCount)
	}
	if client.ExportCallCount != 0 {
		t.Fatalf("expected Export not to be called, got %d", client.ExportCallCount)
	}
}

func TestFetchRefreshIssuesUsesExportWhenReconciling(t *testing.T) {
	client := newDeltaStubClient()
	client.ExportFn = func(context.Context) ([]beads.FullIssue, error) { return nil, nil }

	if _, err := fetchRefreshIssues(context.Background(), client, nil, true); err != nil {
		t.Fatalf("fetchRefreshIssues: %v", err)
	}
	if client.deltaCallCount != 0 {
		t.Fatalf("expected Delta not to be called on a reconcile, got %d", client.deltaCallCount)
	}
	if client.ExportCallCount != 1 {
		t.Fatalf("expected Export to be called once, got %d", client.ExportCallCount)
	}
}

func TestFetchRefreshIssuesFallsBackToExportForNonDeltaClient(t *testing.T) {
	client := beads.NewMockClient()
	client.ExportFn = func(context.Context) ([]beads.FullIssue, error) { return nil, nil }

	// reconcile=false: a non-Delta client has no incremental path at all, so
	// this must still fall back to Export unchanged (fold "keep the existing
	// Export path unchanged" for backends without Delta support).
	if _, err := fetchRefreshIssues(context.Background(), client, nil, false); err != nil {
		t.Fatalf("fetchRefreshIssues: %v", err)
	}
	if client.ExportCallCount != 1 {
		t.Fatalf("expected Export to be called once, got %d", client.ExportCallCount)
	}
}

// TestStartRefreshDeltaTickCallsDeltaNotExport drives the real startRefresh
// entry point (not fetchRefreshIssues directly) for a non-reconcile tick and
// confirms it calls Delta, not Export, and seeds Delta's prev argument from
// the live tree.
func TestStartRefreshDeltaTickCallsDeltaNotExport(t *testing.T) {
	client := newDeltaStubClient()
	var gotPrev []beads.FullIssue
	client.deltaFn = func(_ context.Context, prev []beads.FullIssue) ([]beads.FullIssue, error) {
		gotPrev = prev
		return prev, nil
	}
	app := &App{
		client: client,
		roots:  []*graph.Node{{Issue: beads.FullIssue{ID: "ab-1", Title: "T", Status: "open", IssueType: "task"}}},
	}

	msg := extractRefreshMsg(t, app.startRefresh(time.Now(), false))
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if client.deltaCallCount != 1 {
		t.Fatalf("expected Delta to be called once, got %d", client.deltaCallCount)
	}
	if client.ExportCallCount != 0 {
		t.Fatalf("expected Export not to be called, got %d", client.ExportCallCount)
	}
	if len(gotPrev) != 1 || gotPrev[0].ID != "ab-1" {
		t.Fatalf("expected Delta to receive the flattened live tree as prev, got %+v", gotPrev)
	}
	if app.deltaTicksSinceReconcile != 1 {
		t.Fatalf("expected deltaTicksSinceReconcile=1 after a delta tick, got %d", app.deltaTicksSinceReconcile)
	}
}

// TestStartRefreshReconcileCallsExportAndResetsCadence drives startRefresh
// with reconcile=true and confirms it calls Export, not Delta, and resets
// the bounded-cadence counter (fold R01).
func TestStartRefreshReconcileCallsExportAndResetsCadence(t *testing.T) {
	client := newDeltaStubClient()
	client.ExportFn = func(context.Context) ([]beads.FullIssue, error) {
		return []beads.FullIssue{{ID: "ab-1", Title: "T", Status: "open", IssueType: "task"}}, nil
	}
	app := &App{client: client, deltaTicksSinceReconcile: 7}

	msg := extractRefreshMsg(t, app.startRefresh(time.Now(), true))
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if client.deltaCallCount != 0 {
		t.Fatalf("expected Delta not to be called on a reconcile, got %d", client.deltaCallCount)
	}
	if client.ExportCallCount != 1 {
		t.Fatalf("expected Export to be called once, got %d", client.ExportCallCount)
	}
	if app.deltaTicksSinceReconcile != 0 {
		t.Fatalf("expected deltaTicksSinceReconcile reset to 0 after a reconcile, got %d", app.deltaTicksSinceReconcile)
	}
}

func TestForceRefreshAlwaysReconciles(t *testing.T) {
	client := newDeltaStubClient()
	client.ExportFn = func(context.Context) ([]beads.FullIssue, error) { return nil, nil }
	app := &App{client: client, deltaTicksSinceReconcile: 2}

	msg := extractRefreshMsg(t, app.forceRefresh())
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if client.deltaCallCount != 0 {
		t.Fatalf("expected forceRefresh never to call Delta, got %d calls", client.deltaCallCount)
	}
	if client.ExportCallCount != 1 {
		t.Fatalf("expected forceRefresh to call Export once, got %d", client.ExportCallCount)
	}
	if app.deltaTicksSinceReconcile != 0 {
		t.Fatalf("expected forceRefresh to reset the cadence counter, got %d", app.deltaTicksSinceReconcile)
	}
}

// TestCommentsSurviveDeltaTickButReloadAfterReconcile is the central
// correctness test for this fold: a Delta tick must preserve an
// already-loaded comment (via Delta's own merge, since the issue itself did
// not change), while a subsequent full reconcile must invalidate it so it
// re-loads fresh (design fold R02 / the #7 finding) rather than staying
// stale forever.
func TestCommentsSurviveDeltaTickButReloadAfterReconcile(t *testing.T) {
	client := newDeltaStubClient()
	loadedComment := []beads.Comment{{ID: "1", Text: "real comment"}}

	app := &App{
		client: client,
		roots: []*graph.Node{{
			Issue: beads.FullIssue{
				ID: "ab-1", Title: "T", Status: "open", IssueType: "task",
				Comments: loadedComment,
			},
			CommentsLoaded: true,
		}},
	}

	// Delta tick: nothing in the DB changed, so Delta's merge just echoes
	// prev back — the comment must survive untouched.
	client.deltaFn = func(_ context.Context, prev []beads.FullIssue) ([]beads.FullIssue, error) {
		return prev, nil
	}
	msg := extractRefreshMsg(t, app.startRefresh(time.Now(), app.reconcileDue()))
	if msg.err != nil {
		t.Fatalf("delta tick: unexpected error: %v", msg.err)
	}
	app.refreshInFlight = false // mirrors update.go's refreshCompleteMsg handler
	app.applyRefresh(msg.roots, msg.digest, msg.dbModTime, msg.reconcile)

	node := app.findNodeByID("ab-1")
	if node == nil {
		t.Fatal("expected ab-1 to survive the delta tick")
	}
	if !node.CommentsLoaded || len(node.Issue.Comments) != 1 || node.Issue.Comments[0].Text != "real comment" {
		t.Fatalf("expected the loaded comment to survive the delta tick, got %+v (loaded=%v)",
			node.Issue.Comments, node.CommentsLoaded)
	}

	// Force the bounded cadence: the next refresh must be a full reconcile.
	app.deltaTicksSinceReconcile = reconcileEveryTicks
	client.ExportFn = func(context.Context) ([]beads.FullIssue, error) {
		// A real Export always returns skeleton-only rows: Comments nil.
		return []beads.FullIssue{{ID: "ab-1", Title: "T", Status: "open", IssueType: "task"}}, nil
	}
	if !app.reconcileDue() {
		t.Fatal("expected reconcileDue=true once the cadence threshold is hit")
	}
	msg = extractRefreshMsg(t, app.startRefresh(time.Now(), true))
	if msg.err != nil {
		t.Fatalf("reconcile: unexpected error: %v", msg.err)
	}
	if client.deltaCallCount != 1 {
		t.Fatalf("expected Delta to have been called exactly once (the earlier tick), got %d", client.deltaCallCount)
	}
	if client.ExportCallCount != 1 {
		t.Fatalf("expected the reconcile to call Export once, got %d", client.ExportCallCount)
	}
	app.applyRefresh(msg.roots, msg.digest, msg.dbModTime, msg.reconcile)

	node = app.findNodeByID("ab-1")
	if node == nil {
		t.Fatal("expected ab-1 to survive the reconcile")
	}
	if node.CommentsLoaded || node.Issue.Comments != nil {
		t.Fatalf("expected the reconcile to invalidate the cached comment (not a permanent wipe — "+
			"the background loader re-fetches it), got Comments=%+v CommentsLoaded=%v",
			node.Issue.Comments, node.CommentsLoaded)
	}
}

// TestDetailSurvivesDeltaTick mirrors
// TestCommentsSurviveDeltaTickButReloadAfterReconcile for the lazy
// detail-load fields: a Delta tick must preserve an already-loaded issue's
// Description/DetailLoaded via Delta's own merge (the issue itself did not
// change), closing the coverage gap the comments-only test left on the
// detail side of the same mechanism.
func TestDetailSurvivesDeltaTick(t *testing.T) {
	client := newDeltaStubClient()
	client.deltaFn = func(_ context.Context, prev []beads.FullIssue) ([]beads.FullIssue, error) {
		return prev, nil
	}

	app := &App{
		client: client,
		roots: []*graph.Node{{
			Issue: beads.FullIssue{
				ID: "ab-1", Title: "T", Status: "open", IssueType: "task",
				Description:  "real description",
				DetailLoaded: true,
			},
		}},
	}

	msg := extractRefreshMsg(t, app.startRefresh(time.Now(), app.reconcileDue()))
	if msg.err != nil {
		t.Fatalf("delta tick: unexpected error: %v", msg.err)
	}
	app.refreshInFlight = false
	app.applyRefresh(msg.roots, msg.digest, msg.dbModTime, msg.reconcile)

	node := app.findNodeByID("ab-1")
	if node == nil {
		t.Fatal("expected ab-1 to survive the delta tick")
	}
	if !node.Issue.DetailLoaded || node.Issue.Description != "real description" {
		t.Fatalf("expected the loaded detail to survive the delta tick, got description=%q detailLoaded=%v",
			node.Issue.Description, node.Issue.DetailLoaded)
	}
}

// TestCommentAndDetailErrorExcludedFromRetryAfterDeltaTickButClearOnReconcile
// is the regression test for the retry-storm fix: CommentError/DetailError
// are Node-level fields (not part of beads.FullIssue), so Delta's merge
// cannot carry them forward the way it carries Comments/DetailLoaded.
// Without transferCommentError/transferDetailError, a persistently-failing
// fetch's exclusion from the retry sweep would silently reset to "" on
// every single delta tick, re-queuing it with no backoff. A delta tick must
// carry the error through (still excluded); a full reconcile must let it
// clear (fresh retry is the intended behavior of invalidate-all).
func TestCommentAndDetailErrorExcludedFromRetryAfterDeltaTickButClearOnReconcile(t *testing.T) {
	client := newDeltaStubClient()
	client.deltaFn = func(_ context.Context, prev []beads.FullIssue) ([]beads.FullIssue, error) {
		return prev, nil
	}

	app := &App{
		client: client,
		roots: []*graph.Node{{
			Issue:        beads.FullIssue{ID: "ab-1", Title: "T", Status: "open", IssueType: "task"},
			CommentError: "boom: bd show failed",
			DetailError:  "boom: bd show failed",
		}},
	}

	// Delta tick: the failing issue is untouched, so Delta's merge just
	// echoes prev back. Both error fields must survive so the node stays
	// excluded from the retry sweep.
	msg := extractRefreshMsg(t, app.startRefresh(time.Now(), app.reconcileDue()))
	if msg.err != nil {
		t.Fatalf("delta tick: unexpected error: %v", msg.err)
	}
	app.refreshInFlight = false
	app.applyRefresh(msg.roots, msg.digest, msg.dbModTime, msg.reconcile)

	node := app.findNodeByID("ab-1")
	if node == nil {
		t.Fatal("expected ab-1 to survive the delta tick")
	}
	if node.CommentError == "" {
		t.Fatal("expected CommentError to survive the delta tick (excluded from the retry sweep)")
	}
	if node.DetailError == "" {
		t.Fatal("expected DetailError to survive the delta tick")
	}
	if needsCommentFetch(node) {
		t.Fatal("expected the failing node to stay excluded from the bulk comment retry sweep after a delta tick")
	}

	// Force the bounded cadence: the next refresh is a full reconcile, which
	// must let both error fields clear (fresh retry is the intended
	// behavior of invalidate-all).
	app.deltaTicksSinceReconcile = reconcileEveryTicks
	client.ExportFn = func(context.Context) ([]beads.FullIssue, error) {
		return []beads.FullIssue{{ID: "ab-1", Title: "T", Status: "open", IssueType: "task"}}, nil
	}
	if !app.reconcileDue() {
		t.Fatal("expected reconcileDue=true once the cadence threshold is hit")
	}
	msg = extractRefreshMsg(t, app.startRefresh(time.Now(), true))
	if msg.err != nil {
		t.Fatalf("reconcile: unexpected error: %v", msg.err)
	}
	app.applyRefresh(msg.roots, msg.digest, msg.dbModTime, msg.reconcile)

	node = app.findNodeByID("ab-1")
	if node == nil {
		t.Fatal("expected ab-1 to survive the reconcile")
	}
	if node.CommentError != "" {
		t.Fatalf("expected CommentError to clear after a full reconcile, got %q", node.CommentError)
	}
	if node.DetailError != "" {
		t.Fatalf("expected DetailError to clear after a full reconcile, got %q", node.DetailError)
	}
	if !needsCommentFetch(node) {
		t.Fatal("expected the node to be eligible for retry again after a reconcile clears the error")
	}
}
