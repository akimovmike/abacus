package ui

import (
	"context"
	"fmt"
	"time"

	"abacus/internal/beads"

	tea "github.com/charmbracelet/bubbletea"
)

// detailFetchTimeout bounds a single lazy detail-load (Client.Show) for the
// currently selected issue. Matches commentFetchTimeout: a Show call may
// queue behind a concurrent external bd/dolt writer holding the embedded
// store lock, so the budget is generous since the load is background and
// non-blocking.
const detailFetchTimeout = 30 * time.Second

// detailLoadedMsg carries the result of a background lazy detail load
// triggered by selecting a skeleton-only issue (Issue.DetailLoaded == false).
type detailLoadedMsg struct {
	issueID string
	issue   beads.FullIssue
	err     error
}

// loadDetailCmd fetches the heavy detail fields + comments for issueID via
// Client.Show, extending the existing lazy-comment pattern (memory
// abacus-comment-add-display-bug) to the dolt skeleton/detail split (fold
// F13/R03): a skeleton-only issue's Description/Design/Notes/
// AcceptanceCriteria/CloseReason/Comments are empty until this runs.
func (m *App) loadDetailCmd(issueID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), detailFetchTimeout)
		defer cancel()
		issues, err := client.Show(ctx, []string{issueID})
		if err != nil {
			return detailLoadedMsg{issueID: issueID, err: err}
		}
		if len(issues) == 0 {
			return detailLoadedMsg{issueID: issueID, err: fmt.Errorf("issue %s not found", issueID)}
		}
		return detailLoadedMsg{issueID: issueID, issue: issues[0]}
	}
}

// maybeTriggerDetailLoad dispatches a background detail load for the
// currently selected issue if it hasn't been loaded (or already errored)
// and no load is already in flight for it. Called from Update on every
// message (via appendDetailLoadCmd) so selecting a skeleton-only issue never
// leaves the detail pane rendering a blank that reads as genuinely empty.
func (m *App) maybeTriggerDetailLoad() tea.Cmd {
	if m.client == nil || !m.ShowDetails || len(m.visibleRows) == 0 || m.cursor < 0 || m.cursor >= len(m.visibleRows) {
		return nil
	}
	node := m.visibleRows[m.cursor].Node
	if node == nil || node.Issue.DetailLoaded || node.DetailError != "" {
		return nil
	}
	id := node.Issue.ID
	if m.detailLoadInFlight == nil {
		m.detailLoadInFlight = make(map[string]bool)
	}
	if m.detailLoadInFlight[id] {
		return nil
	}
	m.detailLoadInFlight[id] = true
	return m.loadDetailCmd(id)
}

// appendDetailLoadCmd batches a pending detail-load command (if any) onto
// cmd. Centralizing this in Update means every message flow — keys, mouse,
// overlays, background ticks — gets a chance to notice the current
// selection needs a lazy load, without threading a return value through
// every updateViewportContent call site.
func (m *App) appendDetailLoadCmd(cmd tea.Cmd) tea.Cmd {
	dc := m.maybeTriggerDetailLoad()
	if dc == nil {
		return cmd
	}
	if cmd == nil {
		return dc
	}
	return tea.Batch(cmd, dc)
}
