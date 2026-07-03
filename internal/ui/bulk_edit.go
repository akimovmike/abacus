package ui

import tea "github.com/charmbracelet/bubbletea"

// bulkStatusCmds returns one status-change command per selected bead. A
// closed->open transition uses the reopen path, matching the single-target
// handler. oldStatus is the status shown in the overlay before the change.
func (m *App) bulkStatusCmds(oldStatus, newStatus string) []tea.Cmd {
	ids := m.selectedIssueIDs()
	cmds := make([]tea.Cmd, 0, len(ids))
	for _, id := range ids {
		if oldStatus == "closed" && newStatus == "open" {
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
