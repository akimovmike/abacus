package ui

import (
	"fmt"
	"strings"

	"abacus/internal/config"
	"abacus/internal/domain"

	"github.com/charmbracelet/lipgloss"
)

const treeScrollMargin = 1

// treeRowRenderCount counts individual tree rows styled since the last reset.
// Incremented by renderRow (prod); read only by tests, which assert
// renderTreeView styles the visible viewport rather than every row (guards the
// large-project scroll perf fix, ab-228x). Lint runs with tests:false, so the
// test-only reads are invisible to it.
//
//nolint:unused
var treeRowRenderCount int

// renderTreeView renders the tree list. Theme is managed by the caller (view.go)
// which sets dimmed theme when an overlay is active.
func (m *App) renderTreeView() string {
	listHeight := m.treePaneHeight()
	if len(m.visibleRows) == 0 {
		m.treeTopLine = 0
		// Show empty state message with hint to create first bead
		emptyMsg := styleStatsDim().Render("No beads yet. Press ") +
			styleID().Render("n") +
			styleStatsDim().Render(" to add your first bead.")
		return emptyMsg
	}

	totalWidth := m.width - 2
	if m.ShowDetails && m.layout != LayoutTall {
		totalWidth = m.width - m.viewport.Width - 4
	}
	totalWidth = clampDimension(totalWidth, minTreeWidth, m.width-2)

	// Each visible row renders to exactly one line, so the viewport window can
	// be computed from counts alone. We then style ONLY the windowed rows,
	// keeping a scroll keystroke O(viewport) instead of O(total rows) (ab-228x).
	totalLines := len(m.visibleRows)
	cursorStart, cursorEnd := -1, -1
	if m.cursor >= 0 && m.cursor < totalLines {
		cursorStart, cursorEnd = m.cursor, m.cursor+1
	}

	if m.treeMouseScrolled {
		m.clampTreeViewportTop(listHeight, totalLines)
	} else {
		m.ensureTreeSelectionVisible(listHeight, totalLines, cursorStart, cursorEnd)
	}

	maxTop := totalLines - listHeight
	if maxTop < 0 {
		maxTop = 0
	}
	start := m.treeTopLine
	if start < 0 {
		start = 0
	} else if start > maxTop {
		start = maxTop
	}
	end := start + listHeight
	if end > totalLines {
		end = totalLines
	}

	r := m.newTreeRowRenderer(totalWidth)
	visible := make([]string, 0, listHeight)
	for i := start; i < end; i++ {
		visible = append(visible, r.renderRow(i))
	}
	for len(visible) < listHeight {
		visible = append(visible, "")
	}

	return strings.Join(visible, "\n")
}

// treeRowRenderer holds the shared state for one View pass so an individual row
// can be styled on demand. This lets renderTreeView style only the rows inside
// the viewport window instead of every visible row (ab-228x).
type treeRowRenderer struct {
	m            *App
	totalWidth   int
	treeWidth    int
	columns      columnState
	showColumns  bool
	showPriority bool
	selectedID   string
}

func (m *App) newTreeRowRenderer(totalWidth int) treeRowRenderer {
	columns, treeWidth := prepareColumnState(totalWidth)
	// Track which node is selected for cross-highlighting duplicate instances.
	var selectedID string
	if m.cursor >= 0 && m.cursor < len(m.visibleRows) {
		selectedID = m.visibleRows[m.cursor].Node.Issue.ID
	}
	return treeRowRenderer{
		m:            m,
		totalWidth:   totalWidth,
		treeWidth:    treeWidth,
		columns:      columns,
		showColumns:  columns.enabled(),
		showPriority: config.GetBool(config.KeyTreeShowPriority),
		selectedID:   selectedID,
	}
}

// renderRow styles the visible row at index i into a single line.
func (r treeRowRenderer) renderRow(i int) string {
	treeRowRenderCount++
	m := r.m
	row := m.visibleRows[i]
	node := row.Node
	indent := strings.Repeat("  ", row.Depth)
	marker := " •"
	if len(node.Children) > 0 {
		if m.isNodeExpandedInView(row) {
			marker = " ▼"
		} else {
			marker = " ▶"
		}
	}

	iconStr, iconStyle, textStyle := "○", styleNormalText(), styleNormalText()
	domainIssue, err := domain.NewIssueFromFull(node.Issue, node.IsBlocked)
	status := node.Issue.Status
	if err == nil {
		status = string(domainIssue.Status())
	}
	switch status {
	case "in_progress":
		iconStr, iconStyle, textStyle = "◐", styleIconInProgress(), styleInProgressText()
	case "closed":
		iconStr, iconStyle, textStyle = "✔", styleIconDone(), styleDoneText()
	case "blocked":
		// Explicit blocked status - same visual as dependency-blocked
		iconStr, iconStyle, textStyle = "⛔", styleIconBlocked(), styleBlockedText()
	case "deferred":
		// Deferred (on ice) - snowflake icon with muted styling
		iconStr, iconStyle, textStyle = "❄", styleIconDeferred(), styleDeferredText()
	default:
		// Open status - check if blocked by dependencies
		if node.IsBlocked {
			iconStr, iconStyle, textStyle = "⛔", styleIconBlocked(), styleBlockedText()
		}
	}

	// Add * indicator for multi-parent items
	idDisplay := node.Issue.ID
	if row.HasMultipleParents() {
		idDisplay = node.Issue.ID + "*"
	}

	// Format priority (e.g., "P2") or empty string if not shown
	priorityStr := formatPriority(node.Issue.Priority, r.showPriority)

	totalPrefixWidth := treePrefixWidth(indent, marker, iconStr, priorityStr, idDisplay)
	availableWidth := r.treeWidth - totalPrefixWidth
	if availableWidth < 1 {
		availableWidth = 1
	}
	title := truncateWithEllipsis(node.Issue.Title, availableWidth)

	// Cross-highlighting: same node appears under multiple parents
	isCrossHighlight := i != m.cursor && node.Issue.ID == r.selectedID

	switch {
	case i == m.cursor:
		return buildSelectedRow(indent, marker, iconStr, iconStyle, priorityStr, idDisplay,
			title, textStyle, r.treeWidth, r.totalWidth, r.columns.render(node, columnRenderSelected))
	case m.rowSelected(i):
		// Row is part of an active multi-selection (but not the cursor)
		return buildMultiSelectRow(indent, marker, iconStr, iconStyle, priorityStr, idDisplay,
			title, textStyle, r.treeWidth, r.totalWidth, r.columns.render(node, columnRenderCrossHighlight))
	case isCrossHighlight:
		// Cross-highlight style for duplicate instances
		return buildCrossHighlightRow(indent, marker, iconStr, iconStyle, priorityStr, idDisplay,
			title, textStyle, r.treeWidth, r.totalWidth, r.columns.render(node, columnRenderCrossHighlight))
	default:
		// Style the indent and all spacing with background
		sp := styleNormalText().Render(" ")
		styledIndent := styleNormalText().Render(" " + indent)
		line := styledIndent + iconStyle.Render(marker) + sp + iconStyle.Render(iconStr) + sp
		if priorityStr != "" {
			line += stylePriority().Render(priorityStr) + sp
		}
		line += styleID().Render(idDisplay) + sp + textStyle.Render(title)
		if r.showColumns {
			// Pad tree content to treeWidth so columns align vertically
			line = padToWidth(line, r.treeWidth, styleNormalText())
			line += r.columns.render(node, columnRenderNormal)
		}
		return line
	}
}

func (m *App) ensureTreeSelectionVisible(listHeight, totalLines, cursorStart, cursorEnd int) {
	m.clampTreeViewportTop(listHeight, totalLines)
	if cursorStart < 0 {
		return
	}

	margin := treeScrollMargin
	if margin > listHeight/2 {
		margin = listHeight / 2
	}
	top := m.treeTopLine
	if cursorStart < top+margin {
		top = cursorStart - margin
	}

	cursorBottom := cursorEnd - 1
	if cursorBottom < cursorStart {
		cursorBottom = cursorStart
	}
	maxVisible := top + listHeight - 1 - margin
	if cursorBottom > maxVisible {
		top = cursorBottom - (listHeight - 1 - margin)
	}

	m.treeTopLine = m.clampedTreeViewportTop(top, listHeight, totalLines)
}

func (m *App) clampTreeViewportTop(listHeight, totalLines int) {
	m.treeTopLine = m.clampedTreeViewportTop(m.treeTopLine, listHeight, totalLines)
}

func (m *App) prepareTreeKeyboardNavigation() {
	if !m.treeMouseScrolled {
		return
	}
	m.treeMouseScrolled = false
	m.ensureTreeSelectionVisible(m.treePaneHeight(), len(m.visibleRows), m.cursor, m.cursor+1)
}

func (m *App) clampedTreeViewportTop(top, listHeight, totalLines int) int {
	if listHeight < 1 {
		listHeight = 1
	}
	maxTop := totalLines - listHeight
	if maxTop < 0 {
		maxTop = 0
	}
	if top < 0 {
		return 0
	}
	if top > maxTop {
		return maxTop
	}
	return top
}

func truncateWithEllipsis(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	if lipgloss.Width(text) <= maxWidth {
		return text
	}

	ellipsis := "…"
	ellipsisWidth := lipgloss.Width(ellipsis)
	if maxWidth <= ellipsisWidth {
		return strings.Repeat(".", maxWidth)
	}

	runes := []rune(text)
	for i := len(runes); i >= 0; i-- {
		candidate := string(runes[:i])
		if lipgloss.Width(candidate)+ellipsisWidth <= maxWidth {
			return candidate + ellipsis
		}
	}

	return strings.Repeat(".", maxWidth)
}

// formatPriority returns a compact priority string (e.g., "P2") or empty if priority is not shown.
func formatPriority(priority int, showPriority bool) string {
	if !showPriority {
		return ""
	}
	return fmt.Sprintf("P%d", priority)
}

func treePrefixWidth(indent, marker, icon, priority, id string) int {
	var raw string
	if priority == "" {
		raw = fmt.Sprintf(" %s%s %s %s ", indent, marker, icon, id)
	} else {
		raw = fmt.Sprintf(" %s%s %s %s %s ", indent, marker, icon, priority, id)
	}
	width := lipgloss.Width(raw)
	if width < 0 {
		return 0
	}
	return width
}

// padToWidth pads a string to exactly the target width using spaces styled with the given style.
// If the string is already wider than target, it's returned unchanged.
func padToWidth(s string, targetWidth int, style lipgloss.Style) string {
	currentWidth := lipgloss.Width(s)
	if currentWidth >= targetWidth {
		return s
	}
	padding := strings.Repeat(" ", targetWidth-currentWidth)
	return s + style.Render(padding)
}

// buildSelectedRow creates a full-width row with selection background.
// It preserves the icon's status color while applying selection background to all elements.
// treeWidth is the width for the tree portion (before columns), totalWidth is the full row width.
func buildSelectedRow(indent, marker, icon string, iconStyle lipgloss.Style, priority, id, title string, textStyle lipgloss.Style, treeWidth, totalWidth int, columns string) string {
	t := currentThemeWrapper()
	bg := t.BackgroundSecondary()

	// Create styles with selection background
	selectedBase := lipgloss.NewStyle().Background(bg)
	selectedPrefix := selectedBase.Bold(true).Foreground(t.Primary())
	selectedIcon := selectedBase.Foreground(iconStyle.GetForeground())
	selectedPriority := selectedBase.Foreground(t.TextMuted())
	selectedID := selectedBase.Foreground(t.Accent()).Bold(true)
	selectedText := selectedBase.Bold(true).Foreground(textStyle.GetForeground())

	// Build the tree content (without columns)
	treeContent := selectedPrefix.Render(fmt.Sprintf(" %s%s ", indent, marker)) +
		selectedIcon.Render(icon) + selectedBase.Render(" ")

	if priority != "" {
		treeContent += selectedPriority.Render(priority) + selectedBase.Render(" ")
	}

	treeContent += selectedID.Render(id) + selectedBase.Render(" ") +
		selectedText.Render(title)

	// Pad tree content to treeWidth so columns align vertically
	if columns != "" {
		treeContent = padToWidth(treeContent, treeWidth, selectedBase)
		treeContent += columns
	}

	// Pad to full width with selection background
	return lipgloss.NewStyle().
		Background(bg).
		Width(totalWidth).
		Render(treeContent)
}

// buildCrossHighlightRow creates a full-width row with cross-highlight background.
// treeWidth is the width for the tree portion (before columns), totalWidth is the full row width.
func buildCrossHighlightRow(indent, marker, icon string, iconStyle lipgloss.Style, priority, id, title string, textStyle lipgloss.Style, treeWidth, totalWidth int, columns string) string {
	t := currentThemeWrapper()
	bg := t.BorderNormal()

	// Create styles with cross-highlight background
	crossBase := lipgloss.NewStyle().Background(bg)
	crossPrefix := crossBase.Foreground(t.TextMuted())
	crossIcon := crossBase.Foreground(iconStyle.GetForeground())
	crossPriority := crossBase.Foreground(t.TextMuted())
	crossID := crossBase.Foreground(t.Accent()).Bold(true)
	crossText := crossBase.Foreground(textStyle.GetForeground())

	// Build the tree content (without columns)
	treeContent := crossPrefix.Render(fmt.Sprintf(" %s%s ", indent, marker)) +
		crossIcon.Render(icon) + crossBase.Render(" ")

	if priority != "" {
		treeContent += crossPriority.Render(priority) + crossBase.Render(" ")
	}

	treeContent += crossID.Render(id) + crossBase.Render(" ") +
		crossText.Render(title)

	// Pad tree content to treeWidth so columns align vertically
	if columns != "" {
		treeContent = padToWidth(treeContent, treeWidth, crossBase)
		treeContent += columns
	}

	// Pad to full width with cross-highlight background
	return lipgloss.NewStyle().
		Background(bg).
		Width(totalWidth).
		Render(treeContent)
}

// buildMultiSelectRow creates a full-width row for a bead inside an active
// multi-selection (but not the cursor row). Uses a dimmer background than the
// cursor so the cursor stays distinguishable within the selected block.
//
// ponytail: reuses BorderNormal() (same bg as cross-highlight) rather than
// adding a new per-theme color. If the visual clash with cross-highlight
// matters in practice, add a dedicated theme color then.
func buildMultiSelectRow(indent, marker, icon string, iconStyle lipgloss.Style, priority, id, title string, textStyle lipgloss.Style, treeWidth, totalWidth int, columns string) string {
	t := currentThemeWrapper()
	bg := t.BorderNormal()

	base := lipgloss.NewStyle().Background(bg)
	prefix := base.Foreground(t.TextMuted())
	iconS := base.Foreground(iconStyle.GetForeground())
	priorityS := base.Foreground(t.TextMuted())
	idS := base.Foreground(t.Accent()).Bold(true)
	textS := base.Foreground(textStyle.GetForeground())

	treeContent := prefix.Render(fmt.Sprintf(" %s%s ", indent, marker)) +
		iconS.Render(icon) + base.Render(" ")
	if priority != "" {
		treeContent += priorityS.Render(priority) + base.Render(" ")
	}
	treeContent += idS.Render(id) + base.Render(" ") + textS.Render(title)

	if columns != "" {
		treeContent = padToWidth(treeContent, treeWidth, base)
		treeContent += columns
	}
	return lipgloss.NewStyle().Background(bg).Width(totalWidth).Render(treeContent)
}
