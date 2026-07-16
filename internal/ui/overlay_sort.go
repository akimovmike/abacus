package ui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"abacus/internal/graph"
)

// SortOverlay is a modal picker for the tree sort order. Unlike the bead-edit
// overlays it mutates no bead: it emits SortChangedMsg and the App re-sorts the
// in-memory tree + persists the choice.
type SortOverlay struct {
	current  graph.SortSpec
	selected int
	options  []sortOption
}

type sortOption struct {
	spec  graph.SortSpec
	label string
}

// SortChangedMsg is emitted when the user confirms a sort order.
type SortChangedMsg struct {
	Spec graph.SortSpec
}

// SortCancelledMsg is emitted when the user dismisses the overlay.
type SortCancelledMsg struct{}

func sortOptions() []sortOption {
	return []sortOption{
		{graph.SortSpec{Key: graph.SortDefault}, "Default (status)"},
		{graph.SortSpec{Key: graph.SortPriority, Desc: false}, "Priority ↑  urgent first"},
		{graph.SortSpec{Key: graph.SortPriority, Desc: true}, "Priority ↓  backlog first"},
		{graph.SortSpec{Key: graph.SortCreated, Desc: true}, "Created ↓  newest first"},
		{graph.SortSpec{Key: graph.SortCreated, Desc: false}, "Created ↑  oldest first"},
		{graph.SortSpec{Key: graph.SortUpdated, Desc: true}, "Updated ↓  newest first"},
		{graph.SortSpec{Key: graph.SortUpdated, Desc: false}, "Updated ↑  oldest first"},
	}
}

// NewSortOverlay builds the picker, preselecting the current spec.
func NewSortOverlay(current graph.SortSpec) *SortOverlay {
	options := sortOptions()
	selected := 0
	for i, opt := range options {
		if opt.spec == current {
			selected = i
			break
		}
	}
	return &SortOverlay{current: current, selected: selected, options: options}
}

func (m *SortOverlay) Init() tea.Cmd { return nil }

func (m *SortOverlay) Update(msg tea.Msg) (*SortOverlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
			m.selected = (m.selected + 1) % len(m.options)
		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
			m.selected = (m.selected - 1 + len(m.options)) % len(m.options)
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return m, m.confirm()
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			return m, func() tea.Msg { return SortCancelledMsg{} }
		}
	}
	return m, nil
}

func (m *SortOverlay) confirm() tea.Cmd {
	spec := m.options[m.selected].spec
	return func() tea.Msg { return SortChangedMsg{Spec: spec} }
}

func (m *SortOverlay) View() string {
	b := NewOverlayBuilder(OverlaySizeStandard, 0)

	b.Line(styleStatsDim().Render("Sort tree"))
	b.Line(b.Divider())

	for i, opt := range m.options {
		indicator := "○" // ○
		if opt.spec == m.current {
			indicator = "●" // ●
		}
		label := opt.label
		if i == m.selected {
			label += "  ←" // ←
		}
		text := "  " + indicator + " " + label
		if i == m.selected {
			b.Line(styleStatusSelected().Render(text))
		} else {
			b.Line(styleStatusOption().Render(text))
		}
	}

	return b.Build()
}

func (m *SortOverlay) Layer(width, height, topMargin, bottomMargin int) Layer {
	return BaseOverlayLayer(m.View, width, height, topMargin, bottomMargin)
}
