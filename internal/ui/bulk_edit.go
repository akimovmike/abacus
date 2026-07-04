package ui

import tea "github.com/charmbracelet/bubbletea"

// bulkStatusCmds returns one status-change command per selected bead. Each
// bead's own current status (not a single snapshot for the whole selection)
// decides whether it uses the reopen path: a bead only reopens if it is
// individually closed and newStatus is "open", matching the single-target
// handler's per-bead behavior.
func (m *App) bulkStatusCmds(newStatus string) []tea.Cmd {
	lo, hi := m.selectionBounds()
	if lo < 0 {
		return nil
	}
	seen := make(map[string]bool, hi-lo+1)
	cmds := make([]tea.Cmd, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		node := m.visibleRows[i].Node
		id := node.Issue.ID
		if seen[id] {
			continue
		}
		seen[id] = true
		if node.Issue.Status == "closed" && newStatus == "open" {
			cmds = append(cmds, m.executeReopenCmd(id))
		} else {
			cmds = append(cmds, m.executeStatusChangeCmd(id, newStatus))
		}
	}
	return cmds
}

// bulkPriorityCmds returns one priority-change command per selected bead.
func (m *App) bulkPriorityCmds(priority int) []tea.Cmd {
	ids := m.selectedIssueIDs()
	cmds := make([]tea.Cmd, 0, len(ids))
	for _, id := range ids {
		cmds = append(cmds, m.executePriorityChangeCmd(id, priority))
	}
	return cmds
}

// bulkLabelsCmds returns one label-update command per selected bead, applying
// the same Added/Removed sets to each.
func (m *App) bulkLabelsCmds(msg LabelsUpdatedMsg) []tea.Cmd {
	ids := m.selectedIssueIDs()
	cmds := make([]tea.Cmd, 0, len(ids))
	for _, id := range ids {
		perBead := msg
		perBead.IssueID = id
		cmds = append(cmds, m.executeLabelsUpdate(perBead))
	}
	return cmds
}
