package ui

import (
	"context"
	"testing"
	"time"

	"abacus/internal/beads"
	"abacus/internal/graph"
)

// TestReconcileIntervalElapsed exercises the raw wall-clock gate in
// isolation from checkDBForChanges/reconcileDue.
func TestReconcileIntervalElapsed(t *testing.T) {
	app := &App{lastReconcile: time.Now()}
	if app.reconcileIntervalElapsed() {
		t.Fatal("expected false immediately after lastReconcile is set")
	}

	app.lastReconcile = time.Now().Add(-reconcileMaxAge - time.Second)
	if !app.reconcileIntervalElapsed() {
		t.Fatal("expected true once reconcileMaxAge has elapsed")
	}
}

// TestCheckDBForChangesFiresWallClockReconcileWhenDBQuiet is the core
// ab-6irx.7 case: mtime hasn't moved (the DB is quiet) but reconcileMaxAge
// has elapsed since the last reconcile, so checkDBForChanges must still
// dispatch a full reconcile — driven purely by the periodic tickMsg, not by
// any DB activity.
func TestCheckDBForChangesFiresWallClockReconcileWhenDBQuiet(t *testing.T) {
	dbFile := createTempDBFile(t)
	client := newDeltaStubClient()
	client.ExportFn = func(context.Context) ([]beads.FullIssue, error) {
		return []beads.FullIssue{{ID: "ab-1", Title: "T", Status: "open", IssueType: "task"}}, nil
	}
	app := &App{
		client:        client,
		dbPath:        dbFile,
		lastDBModTime: fileModTime(t, dbFile),
		lastReconcile: time.Now().Add(-reconcileMaxAge - time.Second),
	}

	cmd := app.checkDBForChanges()
	if cmd == nil {
		t.Fatal("expected a wall-clock reconcile once reconcileMaxAge elapsed with mtime unchanged")
	}
	msg := extractRefreshMsg(t, cmd)
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if !msg.reconcile {
		t.Fatal("expected the wall-clock fallback to dispatch a full reconcile, not a delta tick")
	}
	if client.deltaCallCount != 0 {
		t.Fatalf("expected Delta not to be called by a wall-clock reconcile, got %d calls", client.deltaCallCount)
	}
	if client.ExportCallCount != 1 {
		t.Fatalf("expected Export to be called once by the wall-clock reconcile, got %d", client.ExportCallCount)
	}
}

// TestCheckDBForChangesNoReconcileBeforeIntervalElapses is the negative case:
// mtime unchanged AND reconcileMaxAge not yet elapsed must stay quiet, or the
// fallback would degrade into a tight poll.
func TestCheckDBForChangesNoReconcileBeforeIntervalElapses(t *testing.T) {
	dbFile := createTempDBFile(t)
	app := &App{
		client:        newDeltaStubClient(),
		dbPath:        dbFile,
		lastDBModTime: fileModTime(t, dbFile),
		lastReconcile: time.Now().Add(-reconcileMaxAge / 2),
	}

	if cmd := app.checkDBForChanges(); cmd != nil {
		t.Fatal("expected no refresh: mtime unchanged and reconcileMaxAge not yet elapsed")
	}
}

// TestCheckDBForChangesNoWallClockReconcileForNonDeltaClient is the
// regression test for the plan-review fix: the wall-clock fallback must be
// gated to a Delta-capable client. A non-Delta client (MockClient, matching
// the sqlite backend's own lack of a Delta method) already fully reconciles
// on every mtime-triggered refresh via reconcileDue's unconditional true, so
// once mtime stops advancing it has nothing analogous to
// deltaTicksSinceReconcile getting stuck below threshold to recover from.
// Firing this fallback for it too would add a periodic full re-Export
// forever to a session that previously did zero background work once idle
// -- an undisclosed scope expansion this test locks out.
func TestCheckDBForChangesNoWallClockReconcileForNonDeltaClient(t *testing.T) {
	dbFile := createTempDBFile(t)
	app := &App{
		client:        beads.NewMockClient(),
		dbPath:        dbFile,
		lastDBModTime: fileModTime(t, dbFile),
		lastReconcile: time.Now().Add(-reconcileMaxAge - time.Second),
	}

	if cmd := app.checkDBForChanges(); cmd != nil {
		t.Fatal("expected no wall-clock reconcile for a non-Delta client, even past reconcileMaxAge")
	}
}

// TestCheckDBForChangesRespectsSingleFlightForWallClockReconcile proves the
// wall-clock path is gated by the same single-flight guard as every other
// refresh: a reconcile already in flight must never be stacked with another
// one dispatched by the timer.
func TestCheckDBForChangesRespectsSingleFlightForWallClockReconcile(t *testing.T) {
	dbFile := createTempDBFile(t)
	app := &App{
		client:          newDeltaStubClient(),
		dbPath:          dbFile,
		lastDBModTime:   fileModTime(t, dbFile),
		lastReconcile:   time.Now().Add(-reconcileMaxAge - time.Second),
		refreshInFlight: true,
	}

	if cmd := app.checkDBForChanges(); cmd != nil {
		t.Fatal("expected refreshInFlight to block a wall-clock reconcile from stacking")
	}
}

// TestWallClockReconcileDoesNotBusyLoop proves the no-busy-loop invariant:
// once a wall-clock reconcile is dispatched, lastReconcile is stamped
// immediately (at dispatch, not completion — see startRefresh), so the very
// next tick does not re-fire even though the DB is still quiet.
func TestWallClockReconcileDoesNotBusyLoop(t *testing.T) {
	dbFile := createTempDBFile(t)
	app := &App{
		client:        newDeltaStubClient(),
		dbPath:        dbFile,
		lastDBModTime: fileModTime(t, dbFile),
		lastReconcile: time.Now().Add(-reconcileMaxAge - time.Second),
	}

	if cmd := app.checkDBForChanges(); cmd == nil {
		t.Fatal("expected the first idle tick past reconcileMaxAge to fire a reconcile")
	}
	if app.reconcileIntervalElapsed() {
		t.Fatal("expected lastReconcile to be stamped at dispatch time so the interval gate re-arms immediately")
	}

	// Simulate the dispatched refresh completing (mirrors update.go's
	// refreshCompleteMsg handler clearing refreshInFlight); the very next
	// tick, still well within reconcileMaxAge, must not reconcile again.
	app.refreshInFlight = false
	if cmd := app.checkDBForChanges(); cmd != nil {
		t.Fatal("expected no immediate re-reconcile: lastReconcile was just stamped")
	}
}

// TestReconcileDueUpgradesOnWallClockIntervalEvenBelowCountThreshold covers
// reconcileDue's OR condition: a Delta-capable client with a low tick count
// (well under reconcileEveryTicks) must still report reconcileDue=true once
// reconcileMaxAge has elapsed, so an mtime-triggered refresh landing after
// the wall-clock interval upgrades to a full reconcile too.
func TestReconcileDueUpgradesOnWallClockIntervalEvenBelowCountThreshold(t *testing.T) {
	app := &App{
		client:                   newDeltaStubClient(),
		deltaTicksSinceReconcile: 1,
		lastReconcile:            time.Now().Add(-reconcileMaxAge - time.Second),
	}

	if !app.reconcileDue() {
		t.Fatal("expected reconcileDue=true once reconcileMaxAge elapses, even below the count threshold")
	}
}

// TestStartRefreshStampsLastReconcileOnlyOnReconcile proves the lastReconcile
// lifecycle: only a reconcile dispatch stamps it (mirroring the existing
// deltaTicksSinceReconcile reset), never a plain delta tick.
func TestStartRefreshStampsLastReconcileOnlyOnReconcile(t *testing.T) {
	client := newDeltaStubClient()
	app := &App{
		client: client,
		roots:  []*graph.Node{{Issue: beads.FullIssue{ID: "ab-1", Title: "T", Status: "open", IssueType: "task"}}},
	}

	_ = app.startRefresh(time.Now(), false) // delta tick, not a reconcile
	if !app.lastReconcile.IsZero() {
		t.Fatalf("expected a delta tick not to stamp lastReconcile, got %v", app.lastReconcile)
	}

	app.refreshInFlight = false
	before := time.Now()
	_ = app.startRefresh(time.Now(), true) // full reconcile
	if app.lastReconcile.Before(before) {
		t.Fatalf("expected lastReconcile to be stamped at reconcile dispatch time, got %v (before %v)",
			app.lastReconcile, before)
	}
}
