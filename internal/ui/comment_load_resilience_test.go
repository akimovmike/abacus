package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"abacus/internal/beads"
	"abacus/internal/graph"
)

func commentNodeIDs(nodes []*graph.Node) []string {
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.Issue.ID)
	}
	return ids
}

func containsCommentNodeID(nodes []*graph.Node, id string) bool {
	for _, n := range nodes {
		if n.Issue.ID == id {
			return true
		}
	}
	return false
}

// --- Defect #1: in-flight guard for background comment loads ---

func TestBackgroundCommentLoadSkippedWhileInFlight(t *testing.T) {
	app := &App{
		commentLoadInFlight: true,
		roots:               []*graph.Node{{Issue: beads.FullIssue{ID: "a"}}},
		client:              beads.NewMockClient(),
	}

	_, cmd := app.Update(startBackgroundCommentLoadMsg{})

	if cmd != nil {
		t.Fatal("expected no command when a comment load is already in flight")
	}
	if !app.commentLoadInFlight {
		t.Fatal("expected commentLoadInFlight to remain true")
	}
}

func TestStartBackgroundCommentLoadSetsInFlight(t *testing.T) {
	app := &App{
		commentLoadInFlight: false,
		roots:               []*graph.Node{{Issue: beads.FullIssue{ID: "a"}}},
		visibleRows:         nodesToRows(&graph.Node{Issue: beads.FullIssue{ID: "a"}}),
		client:              beads.NewMockClient(),
	}

	model, cmd := app.Update(startBackgroundCommentLoadMsg{})
	app = model.(*App)

	if cmd == nil {
		t.Fatal("expected a load command when no load is in flight")
	}
	if !app.commentLoadInFlight {
		t.Fatal("expected commentLoadInFlight to be set true when a load starts")
	}
}

func TestCommentBatchLoadedClearsInFlight(t *testing.T) {
	app := &App{commentLoadInFlight: true}

	model, _ := app.Update(commentBatchLoadedMsg{})
	app = model.(*App)

	if app.commentLoadInFlight {
		t.Fatal("expected commentLoadInFlight to be cleared after a batch completes")
	}
}

// --- Defect #2: stop re-fetching errored comment nodes on every sweep ---

func TestCollectCommentNodesSkipsErroredNodesInBulk(t *testing.T) {
	ok := &graph.Node{Issue: beads.FullIssue{ID: "ehr-ok"}}
	errored := &graph.Node{Issue: beads.FullIssue{ID: "ehr-err"}, CommentError: "failed: killed"}

	got := collectCommentNodes([]*graph.Node{ok, errored}, nil)

	if containsCommentNodeID(got, "ehr-err") {
		t.Fatalf("errored node must be skipped in the bulk sweep; got %v", commentNodeIDs(got))
	}
	if !containsCommentNodeID(got, "ehr-ok") {
		t.Fatalf("un-loaded node must still be fetched; got %v", commentNodeIDs(got))
	}
}

func TestCollectCommentNodesRetriesErroredNodeWhenPrioritized(t *testing.T) {
	errored := &graph.Node{Issue: beads.FullIssue{ID: "ehr-err"}, CommentError: "failed: killed"}

	got := collectCommentNodes([]*graph.Node{errored}, []string{"ehr-err"})

	if !containsCommentNodeID(got, "ehr-err") {
		t.Fatalf("focused errored node must be retried on demand; got %v", commentNodeIDs(got))
	}
}

// --- Primary root cause: a single batch-wide ctx deadline guillotines every
// pending bd show at once. Each fetch must get its own timeout. ---

func TestBackgroundCommentLoadUsesPerCallTimeout(t *testing.T) {
	client := beads.NewMockClient()
	var mu sync.Mutex
	var deadlines []time.Time
	client.CommentsFn = func(ctx context.Context, id string) ([]beads.Comment, error) {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Error("expected a per-call deadline on the comment fetch context")
		}
		mu.Lock()
		deadlines = append(deadlines, dl)
		mu.Unlock()
		time.Sleep(3 * time.Millisecond) // measurable gap between sequential calls
		return []beads.Comment{}, nil
	}

	// More nodes than workers, so each worker fetches several sequentially.
	var nodes []*graph.Node
	for i := 0; i < 16; i++ {
		nodes = append(nodes, &graph.Node{Issue: beads.FullIssue{ID: string(rune('a' + i))}})
	}
	app := &App{roots: nodes, client: client, visibleRows: nodesToRows(nodes...)}

	app.loadCommentsInBackground()()

	mu.Lock()
	defer mu.Unlock()
	if len(deadlines) != len(nodes) {
		t.Fatalf("expected %d fetches, got %d", len(nodes), len(deadlines))
	}
	allSame := true
	for _, d := range deadlines {
		if !d.Equal(deadlines[0]) {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("all fetch deadlines identical -> one shared batch ctx; expected a per-call timeout")
	}
}

// --- Defect #3: refresh bd-list failures must not flash a toast on the first
// transient hiccup, and a success must reset the streak. ---

func TestRefreshErrorBelowThresholdSuppressesToast(t *testing.T) {
	app := &App{ready: true}

	model, _ := app.Update(refreshCompleteMsg{err: errors.New("run bd list: boom")})
	app = model.(*App)

	if app.showErrorToast {
		t.Fatal("a single transient refresh failure must not show a toast")
	}
	if app.refreshFailCount != 1 {
		t.Fatalf("expected refreshFailCount=1, got %d", app.refreshFailCount)
	}
}

func TestRefreshErrorReachingThresholdShowsToast(t *testing.T) {
	app := &App{ready: true, refreshFailCount: refreshFailToastThreshold - 1}

	model, _ := app.Update(refreshCompleteMsg{err: errors.New("run bd list: boom")})
	app = model.(*App)

	if !app.showErrorToast {
		t.Fatalf("expected toast once failures reach %d", refreshFailToastThreshold)
	}
}

func TestRefreshSuccessResetsFailCount(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "beads.db")
	if f, err := os.Create(db); err != nil {
		t.Fatalf("create db: %v", err)
	} else {
		_ = f.Close()
	}

	root := &graph.Node{Issue: beads.FullIssue{ID: "ab-1", Title: "Root"}}
	app := &App{
		roots:            []*graph.Node{root},
		visibleRows:      nodesToRows(root),
		dbPath:           db,
		refreshFailCount: 2,
		refreshInFlight:  true,
	}

	msg := refreshCompleteMsg{
		roots:     []*graph.Node{root},
		digest:    map[string]string{"ab-1": "x"},
		dbModTime: time.Now(),
	}
	model, _ := app.Update(msg)
	app = model.(*App)

	if app.refreshFailCount != 0 {
		t.Fatalf("expected refreshFailCount reset to 0 on success, got %d", app.refreshFailCount)
	}
}
