package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"abacus/internal/beads"
	"abacus/internal/debug"
	"abacus/internal/graph"

	tea "github.com/charmbracelet/bubbletea"
)

// refreshTimeout bounds a single auto-refresh `bd list` read. It must clear a
// cold or write-lock-contended read of a large embedded-Dolt store; 10s was
// too tight and got the bd subprocess SIGKILLed, which fed the re-fire storm.
const refreshTimeout = 30 * time.Second

// refreshFailToastThreshold is how many consecutive auto-refresh failures must
// occur before surfacing an error toast. Auto-refresh is best-effort and old
// data stays valid, so a single transient bd failure should stay silent.
const refreshFailToastThreshold = 3

// reconcileEveryTicks bounds the automatic full-reconcile cadence for a
// Delta-capable backend (design fold R01): after this many consecutive
// incremental Delta refreshes, the next auto-refresh tick runs a full Export
// instead. A Delta refresh only sees issues whose own updated_at moved past
// the watermark, so a label/dependency/comment-only external change (no
// issue-row column touched) would otherwise never surface until something
// else bumps that issue's updated_at. The periodic reconcile catches it.
const reconcileEveryTicks = 10

// deltaClient is implemented by Reader backends that can compute an
// incremental refresh from a previously known issue set (currently only the
// dolt reader, see internal/beads/dolt_reader.go). It is deliberately not
// part of beads.Reader/Client: other backends (sqlite, mocks) simply don't
// implement it, and fetchRefreshIssues/applyRefresh fall back to the
// existing full-Export behavior, unchanged, for them.
type deltaClient interface {
	Delta(ctx context.Context, prev []beads.FullIssue) ([]beads.FullIssue, error)
}

// commentFetchTimeout bounds a single background `bd show` comment fetch. It is
// applied per call (not once for the whole batch) so a large or slow load never
// exceeds a shared deadline that would mass-kill every still-pending fetch. The
// budget is generous because a fetch may queue behind a concurrent external bd
// writer holding the embedded Dolt lock; waiting it out beats failing, since the
// load is background and non-blocking.
const commentFetchTimeout = 30 * time.Second

// refreshDataCmd fetches the next issue set (Delta or Export, see
// fetchRefreshIssues) and rebuilds the tree from it. prevIssues is the
// flattened current issue set, used as Delta's merge base; it is ignored
// when reconcile forces a full Export.
func refreshDataCmd(
	client beads.Client, targetModTime time.Time, prevIssues []beads.FullIssue, reconcile bool,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		defer cancel()

		issues, err := fetchRefreshIssues(ctx, client, prevIssues, reconcile)
		if err != nil {
			return refreshCompleteMsg{err: err}
		}

		roots, err := graph.NewBuilder().Build(issues)
		if err != nil {
			return refreshCompleteMsg{err: err}
		}
		markExportedCommentsLoaded(roots)

		return refreshCompleteMsg{
			roots:     roots,
			digest:    buildIssueDigest(roots),
			dbModTime: targetModTime,
			reconcile: reconcile,
		}
	}
}

// fetchRefreshIssues chooses Delta or Export for one refresh tick: reconcile
// forces a full Export (first load / forceRefresh / bounded cadence, fold
// R01); otherwise it prefers an incremental Delta when the client supports
// it, falling back to Export unchanged for any client without Delta
// support (e.g. the sqlite backend).
func fetchRefreshIssues(
	ctx context.Context, client beads.Client, prevIssues []beads.FullIssue, reconcile bool,
) ([]beads.FullIssue, error) {
	if !reconcile {
		if dc, ok := client.(deltaClient); ok {
			return dc.Delta(ctx, prevIssues)
		}
	}
	return client.Export(ctx)
}

// flattenIssues walks the tree and returns one beads.FullIssue per unique
// id, suitable as Delta's prev argument. A node may appear more than once
// when it has multiple parents (multi-parent support, see graph.TreeRow),
// so dedup by id is required to hand Delta exactly the current known issue
// set, not a multiplied one.
func flattenIssues(roots []*graph.Node) []beads.FullIssue {
	seen := make(map[string]beads.FullIssue)
	var walk func([]*graph.Node)
	walk = func(nodes []*graph.Node) {
		for _, n := range nodes {
			if n == nil {
				continue
			}
			if _, ok := seen[n.Issue.ID]; ok {
				// Already visited via another parent edge — its subtree was
				// already walked then too, so skip re-walking it here.
				continue
			}
			seen[n.Issue.ID] = n.Issue
			walk(n.Children)
		}
	}
	walk(roots)

	out := make([]beads.FullIssue, 0, len(seen))
	for _, iss := range seen {
		out = append(out, iss)
	}
	return out
}

// startRefresh dispatches one refresh. reconcile forces a full Export and
// resets the bounded-cadence counter; otherwise it attempts an incremental
// Delta (fetchRefreshIssues falls back to Export for a non-Delta client)
// seeded with the current tree flattened into prevIssues.
func (m *App) startRefresh(targetModTime time.Time, reconcile bool) tea.Cmd {
	if m.refreshInFlight {
		return nil
	}
	m.refreshInFlight = true
	m.lastAttemptedModTime = targetModTime
	// Incremented at dispatch time, not completion, so a failed delta
	// attempt still counts toward the cadence (mirrors lastAttemptedModTime's
	// dispatch-time watermark above) — a stuck/erroring backend can't
	// indefinitely postpone the bounded reconcile that would otherwise fix it.
	if reconcile {
		m.deltaTicksSinceReconcile = 0
		// Stamped at dispatch time (mirrors deltaTicksSinceReconcile above)
		// so the wall-clock fallback (checkDBForChanges/reconcileDue, see
		// refresh_cadence.go) re-arms immediately and can't busy-loop —
		// the interval gate holds again before this reconcile even
		// completes.
		m.lastReconcile = time.Now()
	} else {
		m.deltaTicksSinceReconcile++
	}
	// Observable trail for the reconcile/delta split (a --debug watchdog
	// signal, not just a test): if this ever silently regresses to "always
	// reconcile" (retry-storm's invalidate-all firing every tick) or "never
	// reconcile" (deltaTicksSinceReconcile stuck, staleness never caught),
	// it shows up in ~/.abacus/debug.log without needing a live repro.
	if _, ok := m.client.(deltaClient); ok {
		debug.Logf("refresh dispatch: reconcile=%v deltaTicksSinceReconcile=%d", reconcile, m.deltaTicksSinceReconcile)
	}
	var prevIssues []beads.FullIssue
	if !reconcile {
		prevIssues = flattenIssues(m.roots)
	}
	return tea.Batch(m.spinner.Tick, refreshDataCmd(m.client, targetModTime, prevIssues, reconcile))
}

// forceRefresh always runs a full Export reconcile (user 'r' / a
// write-triggered refresh, fold R01): callers just changed data via a
// write, so a full re-sync — not a Delta guess — is the correct read here.
func (m *App) forceRefresh() tea.Cmd {
	var modTime time.Time
	if m.dbPath != "" {
		if latest, err := m.latestDBModTime(); err == nil {
			modTime = latest
		}
	}
	return m.startRefresh(modTime, true)
}

func (m *App) latestDBModTime() (time.Time, error) {
	if strings.TrimSpace(m.dbPath) == "" {
		return time.Time{}, fmt.Errorf("database path is empty")
	}
	return latestModTimeForDB(m.dbPath)
}

func latestModTimeForDB(dbPath string) (time.Time, error) {
	info, err := os.Stat(dbPath)
	if err != nil {
		return time.Time{}, err
	}
	// A Dolt store is a directory: an external write (create/close/file) lands
	// in a nested noms chunk whose change does NOT bump the store dir's own
	// mtime, so we must walk for the newest entry. Bounded by store size
	// (~17 files / ~10ms for an embedded-dolt repo).
	// ponytail: if a store ever grows huge, watch the noms manifest instead.
	if info.IsDir() {
		return latestModTimeInDir(dbPath, info.ModTime())
	}
	// SQLite: the .db file plus its -wal sibling.
	latest := info.ModTime()
	for _, path := range []string{dbPath + "-wal"} {
		if modTime, err := optionalModTime(path); err != nil {
			return time.Time{}, err
		} else if modTime.After(latest) {
			latest = modTime
		}
	}
	return latest, nil
}

func latestModTimeInDir(dir string, latest time.Time) (time.Time, error) {
	walkErr := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries; a partial signal still works
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if fi.ModTime().After(latest) {
			latest = fi.ModTime()
		}
		return nil
	})
	return latest, walkErr
}

func optionalModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func (m *App) applyRefresh(newRoots []*graph.Node, newDigest map[string]string, newModTime time.Time, reconcile bool) {
	state := m.captureState()
	oldDigest := buildIssueDigest(m.roots)

	// A Delta-capable client (currently only dolt) hits one of three shapes
	// here: a DELTA tick (Delta's own merge already carries forward each
	// unchanged issue's Comments/DetailLoaded, so only the Node-level error
	// fields need help below); a RECONCILE (full Export always returns
	// skeleton-only rows regardless of whether an issue changed, so the
	// targeted restore below — restoreUnchangedIssues — is what keeps a
	// manual 'r' fast on a large repo instead of force-reloading every
	// already-loaded issue, ab-6irx.6); or a non-Delta client (sqlite,
	// mocks), which keeps its pre-existing full transfer to avoid flicker on
	// every auto-refresh tick (ab-6irx.2 / T11).
	_, deltaCapable := m.client.(deltaClient)
	targetedReconcile := deltaCapable && reconcile
	// Collected unconditionally: the non-Delta path needs the full content
	// for its transfer; a Delta tick needs only the Node-level error fields
	// (CommentError/DetailError can't ride along in beads.FullIssue, so
	// Delta's merge can't carry them — without restoring them here a
	// persistently-failing fetch's retry-sweep exclusion would reset every
	// tick, an unbounded retry storm); and a targeted reconcile needs the
	// full content so restoreUnchangedIssues can selectively keep only
	// issues that are both successfully loaded and content-unchanged.
	oldCommentState := collectCommentState(m.roots)
	oldDetailState := collectDetailState(m.roots)
	var oldChangeSignals map[string]issueChangeSignal
	if targetedReconcile {
		oldChangeSignals = collectChangeSignals(m.roots)
	}
	m.roots = newRoots
	// The UI's chosen sort is authoritative: refreshDataCmd always builds in
	// Default order, so for a custom sort we re-apply the active spec here before
	// rows are recalculated (auto-refresh never clobbers the sort). Default needs
	// no work — the build order already matches.
	if m.sortSpec.Key != graph.SortDefault {
		graph.ApplySort(m.roots, m.sortSpec)
	}
	switch {
	case !deltaCapable:
		transferCommentState(m.roots, oldCommentState)
		transferDetailState(m.roots, oldDetailState)
	case !reconcile:
		// A full reconcile deliberately lets these clear (fresh retry is the
		// intended behavior of a targeted reconcile's invalidated subset);
		// only a delta tick carries them through.
		transferCommentError(m.roots, oldCommentState)
		transferDetailError(m.roots, oldDetailState)
	case targetedReconcile:
		// Restore comments/detail only for issues that were successfully
		// loaded before AND whose content signal is unchanged; everything
		// else (a changed issue, a brand new one, or a previously failed
		// load) is left at the fresh skeleton's blank state. This is the
		// perf fix (ab-6irx.6): a manual 'r' on a large repo no longer
		// force-reloads every already-loaded issue, while an external
		// comment/detail edit on an unchanged-looking issue is still caught
		// (see restoreUnchangedIssues).
		rs := restoreUnchangedIssues(m.roots, oldCommentState, oldDetailState, oldChangeSignals)
		debug.Logf("targeted reconcile: comments kept %d/%d, detail kept %d/%d",
			rs.commentsKept, rs.commentCandidates, rs.detailKept, rs.detailCandidates)
	}
	if !newModTime.IsZero() {
		m.lastDBModTime = newModTime
	}

	m.restoreExpandedState(state.expandedIDs)
	m.expandedInstances = copyBoolMapAll(state.expandedInstances)
	m.setFilterText(state.filterText)
	m.filterCollapsed = copyBoolMap(state.filterCollapsed)
	m.filterForcedExpanded = copyBoolMap(state.filterForcedExpanded)
	m.textInput.SetValue(state.filterText)
	m.viewMode = state.viewMode // Restore view mode across refresh
	m.labelFilter = state.labelFilter
	m.assigneeFilter = state.assigneeFilter
	m.recalcVisibleRows()

	if state.currentID != "" {
		m.restoreCursorToID(state.currentID)
	} else {
		m.cursor = state.cursorIndex
		m.clampCursor()
	}

	if state.currentID != "" {
		m.detailIssueID = state.currentID
	} else {
		m.detailIssueID = ""
	}
	if m.treeMouseScrolled {
		m.restoreTreeViewportTop(state)
	}

	if m.ShowDetails {
		m.focus = state.focus
	} else {
		m.focus = FocusTree
	}

	if m.ShowDetails {
		m.viewport.YOffset = state.viewportYOffset
	}
	m.updateViewportContent()

	m.lastRefreshStats = computeDiffStats(oldDigest, newDigest)
	m.lastRefreshTime = time.Now()
}

func (m *App) restoreTreeViewportTop(state viewState) {
	if state.treeTopRowKey != "" {
		for idx, row := range m.visibleRows {
			if treeRowIdentity(row) == state.treeTopRowKey {
				m.treeTopLine = idx
				return
			}
		}
	}
	m.treeTopLine = m.clampedTreeViewportTop(state.treeTopLine, m.treePaneHeight(), len(m.visibleRows))
}

// eventualRefreshMsg is sent after a delay to trigger a background consistency refresh.
type eventualRefreshMsg struct{}

// scheduleEventualRefresh schedules a delayed consistency refresh after fast injection.
// This ensures the tree stays consistent with the database without blocking the UI.
func (m *App) scheduleEventualRefresh() tea.Cmd {
	// Wait 2 seconds before triggering consistency refresh
	// This gives user time to interact with the new node
	return tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
		return eventualRefreshMsg{}
	})
}

// loadCommentsInBackground loads comments for all issues without blocking the UI (ab-fkyz).
// This is called after the TUI is displayed to avoid startup delay.
func (m *App) loadCommentsInBackground() tea.Cmd {
	if m.client == nil || len(m.roots) == 0 {
		return func() tea.Msg { return commentBatchLoadedMsg{} }
	}

	client := m.client
	// Prioritize the currently focused issue so the detail pane updates quickly.
	var priorityIDs []string
	if len(m.visibleRows) > 0 && m.cursor >= 0 && m.cursor < len(m.visibleRows) {
		priorityIDs = append(priorityIDs, m.visibleRows[m.cursor].Node.Issue.ID)
	}
	nodes := collectCommentNodes(m.roots, priorityIDs)
	if len(nodes) == 0 {
		return func() tea.Msg { return commentBatchLoadedMsg{} }
	}

	workerLimit := maxConcurrentCommentFetches
	if len(nodes) < workerLimit {
		workerLimit = len(nodes)
	}
	if workerLimit <= 0 {
		workerLimit = 1
	}

	return func() tea.Msg {
		type result struct {
			id       string
			comments []beads.Comment
			err      error
		}

		results := make([]result, 0, len(nodes))
		var mu sync.Mutex

		jobs := make(chan *graph.Node, workerLimit)
		var wg sync.WaitGroup

		for i := 0; i < workerLimit; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for node := range jobs {
					// Per-call timeout: a single shared batch deadline would
					// guillotine every still-pending bd show at once once a
					// large load runs long. Each fetch gets its own budget.
					ctx, cancel := context.WithTimeout(context.Background(), commentFetchTimeout)
					comments, err := client.Comments(ctx, node.Issue.ID)
					cancel()
					if err == nil && comments == nil {
						comments = []beads.Comment{}
					}
					mu.Lock()
					results = append(results, result{
						id:       node.Issue.ID,
						comments: comments,
						err:      err,
					})
					mu.Unlock()
				}
			}()
		}

		for _, n := range nodes {
			jobs <- n
		}
		close(jobs)
		wg.Wait()

		batch := commentBatchLoadedMsg{
			results: make([]commentLoadedMsg, 0, len(results)),
		}
		for _, r := range results {
			batch.results = append(batch.results, commentLoadedMsg{
				issueID:  r.id,
				comments: r.comments,
				err:      r.err,
			})
		}

		return batch
	}
}
