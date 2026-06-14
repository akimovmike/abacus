package ui

import (
	"sort"
	"strings"

	"abacus/internal/config"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type columnOverlayRowKind int

const (
	columnOverlayRowMaster columnOverlayRowKind = iota
	columnOverlayRowBuiltin
	columnOverlayRowLabel
	columnOverlayRowAdd
)

type columnOverlayRow struct {
	kind        columnOverlayRowKind
	key         string
	label       string
	displayName string
	enabled     bool
	index       int
}

// ColumnsOverlay configures built-in and label columns for the tree view.
type ColumnsOverlay struct {
	showColumns       bool
	builtins          map[string]bool
	labelColumns      []LabelColumnConfig
	allLabels         []string
	cursor            int
	addingLabel       bool
	labelPicker       ChipComboBox
	editingLabel      bool
	editingLabelIndex int
	replaceNameOnEdit bool
	displayNameInput  textinput.Model
}

// ColumnsClosedMsg is sent when the columns overlay is dismissed.
type ColumnsClosedMsg struct{}

type columnsOverlayConfig struct {
	showColumns  bool
	builtins     map[string]bool
	labelColumns []LabelColumnConfig
}

// NewColumnsOverlay creates a columns configuration overlay from current config.
func NewColumnsOverlay(allLabels []string) *ColumnsOverlay {
	labels := append([]string(nil), allLabels...)
	sort.Strings(labels)
	return &ColumnsOverlay{
		showColumns: config.GetBool(config.KeyTreeShowColumns),
		builtins: map[string]bool{
			config.KeyTreeColumnsLastUpdated: config.GetBool(config.KeyTreeColumnsLastUpdated),
			config.KeyTreeColumnsAssignee:    config.GetBool(config.KeyTreeColumnsAssignee),
			config.KeyTreeColumnsComments:    config.GetBool(config.KeyTreeColumnsComments),
		},
		labelColumns: configuredLabelColumns(),
		allLabels:    labels,
	}
}

// Init implements tea.Model.
func (m *ColumnsOverlay) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *ColumnsOverlay) Update(msg tea.Msg) (*ColumnsOverlay, tea.Cmd) {
	if m.editingLabel {
		return m.updateEditingLabel(msg)
	}
	if m.addingLabel {
		return m.updateAddingLabel(msg)
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("esc"))):
		return m, func() tea.Msg { return ColumnsClosedMsg{} }
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
		if m.currentRow().kind == columnOverlayRowAdd {
			m.startAddingLabel()
			return m, nil
		}
		return m, nil
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys(" ", "space"))):
		m.toggleCurrentRow()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("e"))):
		m.startEditingCurrentLabel()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("d"))):
		m.removeCurrentLabelColumn()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
		m.cursor--
		m.clampCursor()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
		m.cursor++
		m.clampCursor()
	}
	return m, nil
}

func (m *ColumnsOverlay) updateEditingLabel(msg tea.Msg) (*ColumnsOverlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.commitDisplayNameEdit()
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			return m, func() tea.Msg { return ColumnsClosedMsg{} }
		}
		if m.replaceNameOnEdit {
			m.prepareSelectedNameForInput(msg)
		}
	}
	var cmd tea.Cmd
	m.displayNameInput, cmd = m.displayNameInput.Update(msg)
	return m, cmd
}

func (m *ColumnsOverlay) updateAddingLabel(msg tea.Msg) (*ColumnsOverlay, tea.Cmd) {
	switch msg := msg.(type) {
	case ComboBoxEnterSelectedMsg:
		m.addLabelColumn(msg.Value)
		return m, nil
	case ComboBoxTabSelectedMsg:
		m.addLabelColumn(msg.Value)
		return m, nil
	case ChipComboBoxChipAddedMsg:
		m.addLabelColumn(msg.Label)
		return m, nil
	case tea.KeyMsg:
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
			return m, func() tea.Msg { return ColumnsClosedMsg{} }
		}
	}
	var cmd tea.Cmd
	m.labelPicker, cmd = m.labelPicker.Update(msg)
	return m, cmd
}

func (m *ColumnsOverlay) toggleCurrentRow() {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return
	}
	row := rows[m.cursor]
	switch row.kind {
	case columnOverlayRowMaster:
		m.showColumns = !m.showColumns
	case columnOverlayRowBuiltin:
		m.builtins[row.key] = !m.builtins[row.key]
	case columnOverlayRowLabel:
		for i := range m.labelColumns {
			if m.labelColumns[i].Label == row.label {
				m.labelColumns[i].Enabled = !m.labelColumns[i].Enabled
				return
			}
		}
	case columnOverlayRowAdd:
	}
}

func (m *ColumnsOverlay) startEditingCurrentLabel() {
	row := m.currentRow()
	if row.kind != columnOverlayRowLabel {
		return
	}
	input := textinput.New()
	input.Width = OverlayContentWidth(OverlayWidthStandard) / 2
	input.SetValue(row.displayName)
	input.SetCursor(len(row.displayName))
	m.displayNameInput = input
	m.editingLabel = true
	m.editingLabelIndex = row.index
	m.replaceNameOnEdit = false
	_ = m.displayNameInput.Focus()
}

func (m *ColumnsOverlay) prepareSelectedNameForInput(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyRunes:
		m.displayNameInput.SetValue("")
		m.displayNameInput.SetCursor(0)
		m.replaceNameOnEdit = false
	case tea.KeyBackspace, tea.KeyDelete:
		m.displayNameInput.SetValue("")
		m.displayNameInput.SetCursor(0)
		m.replaceNameOnEdit = false
	}
}

func (m *ColumnsOverlay) commitDisplayNameEdit() {
	if m.editingLabelIndex >= 0 && m.editingLabelIndex < len(m.labelColumns) {
		value := strings.TrimSpace(m.displayNameInput.Value())
		if value == "" {
			value = defaultLabelColumnDisplayName(m.labelColumns[m.editingLabelIndex].Label)
		}
		m.labelColumns[m.editingLabelIndex].DisplayName = value
	}
	m.editingLabel = false
	m.replaceNameOnEdit = false
}

func (m *ColumnsOverlay) removeCurrentLabelColumn() {
	row := m.currentRow()
	if row.kind != columnOverlayRowLabel || row.index < 0 || row.index >= len(m.labelColumns) {
		return
	}
	m.labelColumns = append(m.labelColumns[:row.index], m.labelColumns[row.index+1:]...)
	m.clampCursor()
}

func (m *ColumnsOverlay) startAddingLabel() {
	m.addingLabel = true
	m.labelPicker = NewChipComboBox(m.availableLabelColumnOptions()).
		WithWidth(OverlayContentWidth(OverlayWidthStandard)).
		WithMaxVisible(8).
		WithPlaceholder("type label...").
		WithAllowNew(false, "")
	_ = m.labelPicker.Focus()
}

func (m *ColumnsOverlay) addLabelColumn(label string) {
	if label == "" {
		return
	}
	for _, col := range m.labelColumns {
		if col.Label == label {
			m.addingLabel = false
			return
		}
	}
	m.labelColumns = append(m.labelColumns, LabelColumnConfig{
		Label:       label,
		DisplayName: defaultLabelColumnDisplayName(label),
		Enabled:     true,
	})
	m.cursor = len(m.rows()) - 2
	m.addingLabel = false
	m.startEditingCurrentLabel()
	m.replaceNameOnEdit = true
}

func (m *ColumnsOverlay) availableLabelColumnOptions() []string {
	configured := make(map[string]struct{}, len(m.labelColumns))
	for _, col := range m.labelColumns {
		configured[col.Label] = struct{}{}
	}
	options := make([]string, 0, len(m.allLabels))
	for _, label := range m.allLabels {
		if _, ok := configured[label]; ok {
			continue
		}
		options = append(options, label)
	}
	return options
}

func (m *ColumnsOverlay) currentRow() columnOverlayRow {
	rows := m.rows()
	if len(rows) == 0 {
		return columnOverlayRow{}
	}
	if m.cursor < 0 {
		m.cursor = 0
	} else if m.cursor >= len(rows) {
		m.cursor = len(rows) - 1
	}
	return rows[m.cursor]
}

func (m *ColumnsOverlay) configSnapshot() columnsOverlayConfig {
	builtins := make(map[string]bool, len(m.builtins))
	for k, v := range m.builtins {
		builtins[k] = v
	}
	labelColumns := append([]LabelColumnConfig(nil), m.labelColumns...)
	return columnsOverlayConfig{
		showColumns:  m.showColumns,
		builtins:     builtins,
		labelColumns: labelColumns,
	}
}

func (c columnsOverlayConfig) equal(other columnsOverlayConfig) bool {
	if c.showColumns != other.showColumns {
		return false
	}
	if len(c.builtins) != len(other.builtins) || len(c.labelColumns) != len(other.labelColumns) {
		return false
	}
	for key, enabled := range c.builtins {
		if other.builtins[key] != enabled {
			return false
		}
	}
	for i, col := range c.labelColumns {
		if other.labelColumns[i] != col {
			return false
		}
	}
	return true
}

// View implements tea.Model using the unified overlay framework.
func (m *ColumnsOverlay) View() string {
	b := NewOverlayBuilder(OverlaySizeStandard, 0)
	b.Line(styleOverlaySectionLabel().Render("Columns"))
	b.Line(b.Divider())
	b.BlankLine()
	if m.addingLabel {
		b.Line(m.labelPicker.View())
		b.BlankLine()
		b.Footer(m.footerHints())
		return b.Build()
	}

	rows := m.rows()
	for i, row := range rows {
		if row.kind == columnOverlayRowLabel && i > 0 && rows[i-1].kind == columnOverlayRowBuiltin {
			b.BlankLine()
		}
		prefix := "  "
		if i == m.cursor {
			prefix = "› "
		}
		b.Line(prefix + m.renderRow(row))
	}
	b.BlankLine()
	b.Footer(m.footerHints())
	return b.Build()
}

func (m *ColumnsOverlay) renderRow(row columnOverlayRow) string {
	if row.kind == columnOverlayRowAdd {
		return styleHelpKey().Render("+") + styleHelpDesc().Render(" Add label column...")
	}
	if m.editingLabel && row.kind == columnOverlayRowLabel && row.index == m.editingLabelIndex {
		return styleHelpKey().Render("[x]") + " " + m.displayNameInput.View()
	}
	checked := "[ ]"
	if row.enabled {
		checked = "[x]"
	}
	label := row.label
	if row.kind == columnOverlayRowLabel && row.displayName != "" {
		label = row.displayName + "  " + styleStatsDim().Render(row.label)
	}
	style := styleHelpDesc()
	if !m.showColumns && row.kind != columnOverlayRowMaster {
		style = styleStatsDim()
	}
	return styleHelpKey().Render(checked) + style.Render(" "+label)
}

func (m *ColumnsOverlay) rows() []columnOverlayRow {
	rows := []columnOverlayRow{
		{kind: columnOverlayRowMaster, label: "Show columns", enabled: m.showColumns},
		{kind: columnOverlayRowBuiltin, key: config.KeyTreeColumnsLastUpdated, label: "Last Updated", enabled: m.builtins[config.KeyTreeColumnsLastUpdated]},
		{kind: columnOverlayRowBuiltin, key: config.KeyTreeColumnsAssignee, label: "Assignee", enabled: m.builtins[config.KeyTreeColumnsAssignee]},
		{kind: columnOverlayRowBuiltin, key: config.KeyTreeColumnsComments, label: "Comments", enabled: m.builtins[config.KeyTreeColumnsComments]},
	}
	for i, col := range m.labelColumns {
		rows = append(rows, columnOverlayRow{
			kind:        columnOverlayRowLabel,
			label:       col.Label,
			displayName: labelColumnDisplayName(col),
			enabled:     col.Enabled,
			index:       i,
		})
	}
	rows = append(rows, columnOverlayRow{kind: columnOverlayRowAdd})
	return rows
}

func (m *ColumnsOverlay) clampCursor() {
	rowCount := len(m.rows())
	if rowCount <= 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = rowCount - 1
	} else if m.cursor >= rowCount {
		m.cursor = 0
	}
}

func (m *ColumnsOverlay) footerHints() []footerHint {
	if m.addingLabel {
		return []footerHint{
			{"↑↓", "Navigate"},
			{"enter", "Add"},
			{"esc", "Close"},
		}
	}
	if m.editingLabel {
		return []footerHint{
			{"enter", "Apply"},
			{"esc", "Close"},
		}
	}
	hints := []footerHint{
		{"↑↓", "Navigate"},
	}
	switch m.currentRow().kind {
	case columnOverlayRowMaster, columnOverlayRowBuiltin:
		hints = append(hints,
			footerHint{"space", "Toggle"},
			footerHint{"esc", "Close"},
		)
	case columnOverlayRowLabel:
		hints = append(hints,
			footerHint{"space", "Toggle"},
			footerHint{"e", "Rename"},
			footerHint{"d", "Remove"},
			footerHint{"esc", "Close"},
		)
	case columnOverlayRowAdd:
		hints = append(hints,
			footerHint{"enter", "Add"},
			footerHint{"esc", "Close"},
		)
	}
	return hints
}

// Layer returns a centered layer for the columns overlay.
func (m *ColumnsOverlay) Layer(width, height, topMargin, bottomMargin int) Layer {
	return BaseOverlayLayer(m.View, width, height, topMargin, bottomMargin)
}
