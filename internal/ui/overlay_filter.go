package ui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// FilterOverlay lets the user filter the tree by a single label and/or a
// single assignee. Both are transient (session-only) and combine with the
// view mode and text filter in computeFilterEval.
//
// ponytail: values are chosen by left/right cycling through "(any)" + the
// sorted list, mirroring overlay_label_colors. Swap in the ComboBox widget for
// type-ahead if a project accumulates more labels than is comfortable to cycle.
type FilterOverlay struct {
	labels    []string // sorted distinct labels (no "(any)" sentinel)
	assignees []string // sorted distinct assignees
	label     string   // current label selection; "" = (any)
	assignee  string   // current assignee selection; "" = (any)
	row       int      // 0 = label row, 1 = assignee row
}

// FilterChangedMsg is emitted when the filter selection is applied.
type FilterChangedMsg struct {
	Label    string
	Assignee string
}

// FilterCancelledMsg is emitted when the overlay closes without applying.
type FilterCancelledMsg struct{}

// NewFilterOverlay builds the overlay from the project's labels/assignees and
// the currently active filter selections.
func NewFilterOverlay(labels, assignees []string, curLabel, curAssignee string) *FilterOverlay {
	return &FilterOverlay{
		labels:    labels,
		assignees: assignees,
		label:     curLabel,
		assignee:  curAssignee,
	}
}

func (o *FilterOverlay) Init() tea.Cmd { return nil }

// Update implements the overlay message loop.
func (o *FilterOverlay) Update(msg tea.Msg) (*FilterOverlay, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil
	}

	switch {
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
		o.row = 1 - o.row
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
		o.row = 1 - o.row
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("right", "l", " "))):
		o.cycle(1)
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("left", "h"))):
		o.cycle(-1)
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("d", "backspace", "delete"))):
		o.clear()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
		return o, o.apply()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("esc"))):
		return o, func() tea.Msg { return FilterCancelledMsg{} }
	}
	return o, nil
}

// cycle advances the active row's value through ["" (any), options...]. delta>0
// steps forward, delta<0 backward, wrapping at the ends.
func (o *FilterOverlay) cycle(delta int) {
	if o.row == 0 {
		o.label = cycleValue(o.label, o.labels, delta)
	} else {
		o.assignee = cycleValue(o.assignee, o.assignees, delta)
	}
}

func (o *FilterOverlay) clear() {
	if o.row == 0 {
		o.label = ""
	} else {
		o.assignee = ""
	}
}

func (o *FilterOverlay) apply() tea.Cmd {
	label, assignee := o.label, o.assignee
	return func() tea.Msg { return FilterChangedMsg{Label: label, Assignee: assignee} }
}

// cycleValue returns the next value after current in ["", options...], stepping
// by delta with wraparound. An empty options slice always stays "".
func cycleValue(current string, options []string, delta int) string {
	all := make([]string, 0, len(options)+1)
	all = append(all, "") // "(any)" sentinel at index 0
	all = append(all, options...)

	idx := 0
	for i, v := range all {
		if v == current {
			idx = i
			break
		}
	}
	n := len(all)
	next := ((idx+delta)%n + n) % n
	return all[next]
}

// View renders the filter picker.
func (o *FilterOverlay) View() string {
	b := NewOverlayBuilder(OverlaySizeStandard, 0)
	b.Line(styleOverlaySectionLabel().Render("Filter"))
	b.Line(b.Divider())
	b.BlankLine()

	b.Line(o.renderRow("Label:    ", o.label, 0))
	b.Line(o.renderRow("Assignee: ", o.assignee, 1))

	b.BlankLine()
	b.Footer([]footerHint{
		{"←→", "Value"},
		{"↑↓", "Row"},
		{"d", "Clear"},
		{"⏎", "Apply"},
		{"esc", "Cancel"},
	})
	return b.Build()
}

func (o *FilterOverlay) renderRow(label, value string, row int) string {
	cursor := "  "
	if o.row == row {
		cursor = styleStatusSelected().Render("→ ")
	}
	display := value
	if display == "" {
		display = "(any)"
		return cursor + label + styleStatsDim().Render(display)
	}
	return cursor + label + styleNormalText().Render(display)
}

// Layer returns a centered layer for the overlay.
func (o *FilterOverlay) Layer(width, height, topMargin, bottomMargin int) Layer {
	return BaseOverlayLayer(o.View, width, height, topMargin, bottomMargin)
}
