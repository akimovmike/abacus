package ui

// selectionActive reports whether a contiguous multi-selection is in progress.
//
//nolint:unused // used in tests, kept for future multi-select keybinding/render wiring
func (m *App) selectionActive() bool {
	return m.selectAnchor >= 0
}

// clearSelection ends any active multi-selection.
//
//nolint:unused // used in tests, kept for future multi-select keybinding/render wiring
func (m *App) clearSelection() {
	m.selectAnchor = -1
}

// selectionBounds returns the inclusive [lo, hi] range of selected visibleRows
// indices, or (-1, -1) when no selection is active or the tree is empty.
//
//nolint:unused // used in tests, kept for future multi-select keybinding/render wiring
func (m *App) selectionBounds() (int, int) {
	if !m.selectionActive() || len(m.visibleRows) == 0 {
		return -1, -1
	}
	lo, hi := m.selectAnchor, m.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi > len(m.visibleRows)-1 {
		hi = len(m.visibleRows) - 1
	}
	return lo, hi
}

// rowSelected reports whether the visibleRows index i is within the selection.
//
//nolint:unused // used in tests, kept for future multi-select keybinding/render wiring
func (m *App) rowSelected(i int) bool {
	lo, hi := m.selectionBounds()
	return lo >= 0 && i >= lo && i <= hi
}

// selectedIssueIDs returns the deduped, in-order issue IDs of the selected rows.
// A multi-parent node appearing more than once in the range yields one ID.
//
//nolint:unused // used in tests, kept for future multi-select keybinding/render wiring
func (m *App) selectedIssueIDs() []string {
	lo, hi := m.selectionBounds()
	if lo < 0 {
		return nil
	}
	seen := make(map[string]bool, hi-lo+1)
	ids := make([]string, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		id := m.visibleRows[i].Node.Issue.ID
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
