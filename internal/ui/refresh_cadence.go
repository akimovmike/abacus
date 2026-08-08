package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// reconcileMaxAge bounds how long a Delta-capable client's DB can stay quiet
// before an idle auto-refresh tick still runs a full Export reconcile, even
// when no mtime change was observed (ab-6irx.7). This fallback applies only
// to a Delta-capable client (see checkDBForChanges' deltaClient gate below):
// a non-Delta client (sqlite, mocks) has no incremental path at all, so
// reconcileDue already reports true unconditionally for it — it fully
// reconciles on every mtime-triggered refresh already, and once mtime stops
// advancing it has nothing analogous to deltaTicksSinceReconcile getting
// stuck below threshold to recover from. Applying this fallback to it too
// would add periodic background work (a full re-Export + rebuild, a visible
// spinner blip) to an idle non-Delta session that previously did nothing
// once quiet — an undisclosed scope expansion, not this fix's target.
//
// For a Delta-capable client, a metadata-only external edit
// (label/dependency/comment) does bump the store's mtime and fires a Delta
// tick on its own turn, but a Delta tick only sees issues whose own
// updated_at moved past the watermark — content-only fingerprint
// invalidation (ab-6irx.6) runs solely on the reconcile path. Without this
// fallback, that one edit would advance deltaTicksSinceReconcile by exactly
// one and then wait indefinitely (up to reconcileEveryTicks more DB-change
// ticks, or a manual 'r') if the DB then goes quiet. The periodic tickMsg
// (internal/ui/update.go, case tickMsg) fires every refreshInterval
// regardless of DB activity, so it is the driver here: once reconcileMaxAge
// has elapsed since the last reconcile, the next tick upgrades to a full
// reconcile. Not sub-10s: a reconcile is a real (if cheap, post-ab-6irx.6
// targeted-invalidation) bd read, so this stays a background cadence, not a
// tight poll.
const reconcileMaxAge = 60 * time.Second

// reconcileIntervalElapsed reports whether reconcileMaxAge has passed since
// the last full Export reconcile was dispatched (see startRefresh, which
// stamps lastReconcile at dispatch time — mirroring deltaTicksSinceReconcile
// so a slow or erroring reconcile still re-arms the gate immediately instead
// of leaving it open for another tick to double-fire).
func (m *App) reconcileIntervalElapsed() bool {
	return time.Since(m.lastReconcile) >= reconcileMaxAge
}

// checkDBForChanges is invoked on every auto-refresh tick (case tickMsg,
// internal/ui/update.go) to decide whether a refresh should run.
func (m *App) checkDBForChanges() tea.Cmd {
	if m.refreshInFlight || m.dbPath == "" {
		return nil
	}

	modTime, err := m.latestDBModTime()
	if err != nil {
		m.lastError = fmt.Sprintf("refresh check failed: %v", err)
		m.lastErrorSource = errorSourceRefresh
		m.lastRefreshStats = "refresh error"
		return nil // Try again next tick
	}

	if !modTime.After(m.lastDBModTime) {
		// Gated to a Delta-capable client (see reconcileMaxAge doc): a
		// non-Delta client already fully reconciles on every mtime-triggered
		// refresh and has nothing analogous to a Delta tick's watermark miss
		// to recover from, so with mtime unchanged it correctly stays fully
		// idle here, exactly as it did before this fallback existed.
		//
		// For a Delta-capable client, the DB itself hasn't changed since the
		// last successful refresh, but a lone external metadata-only edit
		// may already be sitting unseen: on the tick it landed, it bumped
		// mtime and advanced deltaTicksSinceReconcile by one via a Delta
		// tick that can't see its content. If the DB then goes quiet,
		// nothing else would ever push the bounded tick-count cadence to
		// fire. Fire a full reconcile here, driven purely by the wall clock,
		// so that edit surfaces within reconcileMaxAge regardless of further
		// DB activity.
		if _, ok := m.client.(deltaClient); ok && m.reconcileIntervalElapsed() {
			return m.startRefresh(modTime, true)
		}
		return nil
	}

	// A prior auto-refresh for this DB state already failed; don't hammer bd
	// with the same doomed read every tick. Wait for a newer write (mtime
	// advances past the last attempt) or a manual refresh.
	if !modTime.After(m.lastAttemptedModTime) {
		return nil
	}

	return m.startRefresh(modTime, m.reconcileDue())
}

// reconcileDue reports whether the upcoming auto-refresh tick should run a
// full Export reconcile instead of an incremental Delta. A client with no
// Delta support has no incremental path at all, so it always "reconciles"
// via Export — exactly its pre-existing behavior, unchanged. A Delta-capable
// client reconciles once either the count-based cadence (reconcileEveryTicks
// delta refreshes) or the wall-clock cadence (reconcileMaxAge since the last
// reconcile, ab-6irx.7) is due — so an mtime-triggered refresh that lands
// after the wall-clock interval also upgrades to a full reconcile instead of
// another Delta tick, rather than waiting for the count-based threshold too.
func (m *App) reconcileDue() bool {
	if _, ok := m.client.(deltaClient); !ok {
		return true
	}
	if m.deltaTicksSinceReconcile >= reconcileEveryTicks {
		return true
	}
	return m.reconcileIntervalElapsed()
}
