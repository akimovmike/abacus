package ui

import (
	"fmt"
	"strings"
	"time"

	"abacus/internal/config"
	"abacus/internal/graph"

	"github.com/charmbracelet/lipgloss"
)

const columnSeparator = " │"

var columnSeparatorWidth = lipgloss.Width(columnSeparator)

type treeColumn struct {
	ConfigKey string
	Label     string
	Width     int
	// Render draws the cell value. bg is the current row background (theme bg,
	// or the selection/cross-highlight color) so the labels column can paint
	// chips that track the row; plain-text columns ignore it since the outer
	// column style already fills their background.
	Render func(node *graph.Node, bg lipgloss.TerminalColor) string
}

// LabelColumnConfig stores a user-configured label column.
type LabelColumnConfig struct {
	Label       string `mapstructure:"label" json:"label" yaml:"label"`
	DisplayName string `mapstructure:"displayName" json:"displayName" yaml:"displayName"`
	Enabled     bool   `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
}

type storedLabelColumnConfig struct {
	Label       string `mapstructure:"label"`
	DisplayName string `mapstructure:"displayName"`
	Enabled     *bool  `mapstructure:"enabled"`
}

var defaultTreeColumns = []treeColumn{
	{
		ConfigKey: config.KeyTreeColumnsLastUpdated,
		Width:     8,
		Render:    renderLastUpdatedColumn,
	},
	{
		ConfigKey: config.KeyTreeColumnsAssignee,
		Width:     10,
		Render:    renderAssigneeColumn,
	},
	{
		ConfigKey: config.KeyTreeColumnsComments,
		Width:     5,
		Render:    renderCommentsColumn,
	},
	{
		ConfigKey: config.KeyTreeColumnsLabels,
		Label:     "Labels",
		Width:     labelsColumnWidth,
		Render:    renderLabelsColumn(labelsColumnWidth),
	},
}

type columnState struct {
	columns    []treeColumn
	totalWidth int
}

func (c columnState) enabled() bool {
	return len(c.columns) > 0
}

func (c columnState) render(node *graph.Node, mode columnRenderMode) string {
	if !c.enabled() {
		return ""
	}
	bg := columnValueBackground(mode)
	return c.renderWithProvider(mode, func(col treeColumn) string {
		return col.Render(node, bg)
	})
}

// columnValueBackground returns the background the value cells are painted with
// for the given render mode, mirroring columnStyles' value style. The labels
// column uses it so its chips track the row highlight.
func columnValueBackground(mode columnRenderMode) lipgloss.TerminalColor {
	t := currentThemeWrapper()
	switch mode {
	case columnRenderSelected:
		return t.BackgroundSecondary()
	case columnRenderCrossHighlight:
		return t.BorderNormal()
	default:
		return t.Background()
	}
}

func (c columnState) renderWithProvider(mode columnRenderMode, valueProvider func(treeColumn) string) string {
	if !c.enabled() {
		return ""
	}

	sepStyle, valueStyle := columnStyles(mode)
	var builder strings.Builder
	builder.WriteString(sepStyle.Render(columnSeparator))

	for i, col := range c.columns {
		if i > 0 {
			builder.WriteString(valueStyle.Render(" "))
		}
		cellValue := valueProvider(col)
		cell := valueStyle.
			Width(col.Width).
			Align(lipgloss.Right).
			Render(cellValue)
		builder.WriteString(cell)
	}
	return builder.String()
}

type columnRenderMode int

const (
	columnRenderNormal columnRenderMode = iota
	columnRenderSelected
	columnRenderCrossHighlight
)

func columnStyles(mode columnRenderMode) (lipgloss.Style, lipgloss.Style) {
	t := currentThemeWrapper()
	switch mode {
	case columnRenderSelected:
		base := lipgloss.NewStyle().Background(t.BackgroundSecondary())
		sep := base.Foreground(t.BorderNormal())
		val := base.Foreground(t.Text())
		return sep, val
	case columnRenderCrossHighlight:
		base := lipgloss.NewStyle().Background(t.BorderNormal())
		sep := base.Foreground(t.TextMuted())
		val := base.Foreground(t.Text())
		return sep, val
	default:
		return styleColumnSeparator(), styleColumnText()
	}
}

func prepareColumnState(totalWidth int) (columnState, int) {
	if !config.GetBool(config.KeyTreeShowColumns) {
		return columnState{}, totalWidth
	}

	// Gather all enabled columns.
	enabledCols := make([]treeColumn, 0, len(defaultTreeColumns))
	for _, col := range defaultTreeColumns {
		if config.GetBool(col.ConfigKey) {
			enabledCols = append(enabledCols, col)
		}
	}
	for _, labelCol := range configuredLabelColumns() {
		if !labelCol.Enabled {
			continue
		}
		displayName := labelColumnDisplayName(labelCol)
		enabledCols = append(enabledCols, treeColumn{
			Label:  labelCol.Label,
			Width:  lipgloss.Width(displayName),
			Render: renderLabelColumn(labelCol.Label, displayName),
		})
	}
	if len(enabledCols) == 0 {
		return columnState{}, totalWidth
	}

	// Progressive hiding: remove columns from right to left until they fit
	// Columns are ordered left-to-right by priority (leftmost = highest priority)
	// so we remove from the end (rightmost = lowest priority = hides first)
	for len(enabledCols) > 0 {
		// Width = separator + each column + 1-space gap between each column
		width := columnSeparatorWidth
		for i, col := range enabledCols {
			width += col.Width
			if i > 0 {
				width++ // space between adjacent columns
			}
		}

		treeWidth := totalWidth - width
		if treeWidth >= minTreeWidthForColumns {
			// Columns fit while respecting minimum tree width
			return columnState{
				columns:    enabledCols,
				totalWidth: width,
			}, treeWidth
		}

		// Remove rightmost column (lowest priority) and try again
		enabledCols = enabledCols[:len(enabledCols)-1]
	}

	// No columns fit - return empty state
	return columnState{}, totalWidth
}

func renderLastUpdatedColumn(node *graph.Node, _ lipgloss.TerminalColor) string {
	if node == nil || node.Issue.UpdatedAt == "" {
		return ""
	}
	ts, err := time.Parse(time.RFC3339, node.Issue.UpdatedAt)
	if err != nil {
		return ""
	}
	return FormatRelativeTime(ts)
}

func renderCommentsColumn(node *graph.Node, _ lipgloss.TerminalColor) string {
	if node == nil || !node.CommentsLoaded {
		return ""
	}
	count := len(node.Issue.Comments)
	if count <= 0 {
		return ""
	}
	switch {
	case count > 99:
		return "💬99+"
	default:
		return fmt.Sprintf("💬%d", count)
	}
}

func renderAssigneeColumn(node *graph.Node, _ lipgloss.TerminalColor) string {
	if node == nil || node.Issue.Assignee == "" {
		return ""
	}
	const columnWidth = 10
	return truncateWithEllipsis(node.Issue.Assignee, columnWidth)
}

func configuredLabelColumns() []LabelColumnConfig {
	var cols []storedLabelColumnConfig
	if err := config.UnmarshalKey(config.KeyTreeLabelColumns, &cols); err != nil {
		return nil
	}

	normalized := make([]LabelColumnConfig, 0, len(cols))
	for _, col := range cols {
		label := strings.TrimSpace(col.Label)
		displayName := strings.TrimSpace(col.DisplayName)
		if label == "" {
			continue
		}
		if displayName == "" {
			displayName = defaultLabelColumnDisplayName(label)
		}
		if lipgloss.Width(displayName) == 0 {
			displayName = label
		}
		enabled := true
		if col.Enabled != nil {
			enabled = *col.Enabled
		}
		normalized = append(normalized, LabelColumnConfig{
			Label:       label,
			DisplayName: displayName,
			Enabled:     enabled,
		})
	}
	return normalized
}

func setConfiguredLabelColumns(cols []LabelColumnConfig) error {
	normalized := make([]LabelColumnConfig, 0, len(cols))
	seen := make(map[string]struct{}, len(cols))
	for _, col := range cols {
		col.Label = strings.TrimSpace(col.Label)
		col.DisplayName = strings.TrimSpace(col.DisplayName)
		if col.Label == "" {
			continue
		}
		if _, ok := seen[col.Label]; ok {
			continue
		}
		seen[col.Label] = struct{}{}
		if col.DisplayName == "" {
			col.DisplayName = defaultLabelColumnDisplayName(col.Label)
		}
		normalized = append(normalized, col)
	}
	return config.Set(config.KeyTreeLabelColumns, normalized)
}

func renderLabelColumn(label, displayName string) func(*graph.Node, lipgloss.TerminalColor) string {
	return func(node *graph.Node, _ lipgloss.TerminalColor) string {
		if node == nil {
			return ""
		}
		for _, issueLabel := range node.Issue.Labels {
			if issueLabel == label {
				return displayName
			}
		}
		return ""
	}
}

// labelsColumnWidth is the fixed display width of the all-labels column.
const labelsColumnWidth = 24

// renderLabelsColumn returns a renderer for the all-labels column: every label
// on the issue shown as a colored chip within the given fixed width (ab-chpa).
func renderLabelsColumn(width int) func(*graph.Node, lipgloss.TerminalColor) string {
	return func(node *graph.Node, bg lipgloss.TerminalColor) string {
		if node == nil || len(node.Issue.Labels) == 0 {
			return ""
		}
		return renderLabelChips(node.Issue.Labels, width, bg)
	}
}

// renderLabelChips renders labels as space-separated pill chips, fitting as many
// as possible within maxWidth and collapsing the remainder into a "+N" marker.
// The marker width is reserved up front so the result never exceeds maxWidth.
func renderLabelChips(labels []string, maxWidth int, bg lipgloss.TerminalColor) string {
	if len(labels) == 0 || maxWidth <= 0 {
		return ""
	}
	const sepWidth = 1 // single space between chips and before the marker
	// Separators and the marker carry the row background so they don't fall to
	// the terminal-default background after each chip's reset (ab-uyts).
	bgStyle := lipgloss.NewStyle().Background(bg)
	sep := bgStyle.Render(" ")
	var b strings.Builder
	used := 0
	shown := 0
	for i, label := range labels {
		chip := renderLabelTagBg(label, customLabelColorHex(label), bg)
		w := lipgloss.Width(chip)
		sepW := 0
		if shown > 0 {
			sepW = sepWidth
		}
		// Reserve room for a "+N" marker if any labels remain after this one.
		reserve := 0
		if remaining := len(labels) - i - 1; remaining > 0 {
			reserve = sepWidth + lipgloss.Width(labelOverflowMarker(remaining))
		}
		if used+sepW+w+reserve > maxWidth {
			break
		}
		if shown > 0 {
			b.WriteString(sep)
			used += sepWidth
		}
		b.WriteString(chip)
		used += w
		shown++
	}
	if shown < len(labels) {
		if shown > 0 {
			b.WriteString(sep)
		}
		b.WriteString(bgStyle.Render(labelOverflowMarker(len(labels) - shown)))
	}
	return b.String()
}

func labelOverflowMarker(n int) string {
	return fmt.Sprintf("+%d", n)
}

func labelColumnDisplayName(col LabelColumnConfig) string {
	if strings.TrimSpace(col.DisplayName) != "" {
		return strings.TrimSpace(col.DisplayName)
	}
	return defaultLabelColumnDisplayName(col.Label)
}

func defaultLabelColumnDisplayName(label string) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "-")
	return parts[len(parts)-1]
}
