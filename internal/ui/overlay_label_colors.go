package ui

import (
	"sort"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"abacus/internal/ui/theme"
)

// labelColorPalette is the curated set of selectable label colors. Cycling a
// label steps through these; "default" (no entry) falls back to the theme.
// ponytail: a fixed palette instead of a free-form hex input — add hex entry
// later if users ask for arbitrary colors.
var labelColorPalette = []string{
	"#e06c75", // red
	"#d19a66", // orange
	"#e5c07b", // yellow
	"#98c379", // green
	"#56b6c2", // cyan
	"#61afef", // blue
	"#c678dd", // purple
	"#abb2bf", // grey
}

// LabelColorsOverlay lets the user assign a custom color to each project label.
type LabelColorsOverlay struct {
	labels []string          // sorted distinct labels
	colors map[string]string // working label -> hex override
	cursor int
	dirty  bool
}

// LabelColorsChangedMsg is emitted when color assignments are confirmed.
type LabelColorsChangedMsg struct {
	Colors map[string]string
}

// LabelColorsCancelledMsg is emitted when the overlay closes without changes.
type LabelColorsCancelledMsg struct{}

// NewLabelColorsOverlay builds the overlay from the project's labels and the
// currently configured color overrides.
func NewLabelColorsOverlay(labels []string, existing map[string]string) *LabelColorsOverlay {
	sorted := append([]string(nil), labels...)
	sort.Strings(sorted)

	colors := make(map[string]string, len(existing))
	for k, v := range existing {
		colors[k] = v
	}

	return &LabelColorsOverlay{
		labels: sorted,
		colors: colors,
	}
}

func (m *LabelColorsOverlay) Init() tea.Cmd { return nil }

// Update implements the overlay message loop.
func (m *LabelColorsOverlay) Update(msg tea.Msg) (*LabelColorsOverlay, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
		m.moveCursor(1)
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
		m.moveCursor(-1)
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("right", "l", " "))):
		m.cycle(1)
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("left", "h"))):
		m.cycle(-1)
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("d", "backspace", "delete"))):
		m.clear()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
		return m, m.confirm()
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("esc"))):
		if m.dirty {
			return m, m.confirm()
		}
		return m, func() tea.Msg { return LabelColorsCancelledMsg{} }
	}
	return m, nil
}

func (m *LabelColorsOverlay) moveCursor(delta int) {
	if len(m.labels) == 0 {
		return
	}
	n := len(m.labels)
	m.cursor = ((m.cursor+delta)%n + n) % n
}

// cycle advances the current label's color through the palette. delta>0 moves
// forward, delta<0 backward. An unset label starts at the first/last entry.
func (m *LabelColorsOverlay) cycle(delta int) {
	if len(m.labels) == 0 {
		return
	}
	label := m.labels[m.cursor]
	n := len(labelColorPalette)
	idx := paletteIndex(m.colors[label])

	var next int
	switch {
	case idx == -1 && delta > 0:
		next = 0
	case idx == -1:
		next = n - 1
	default:
		next = ((idx+delta)%n + n) % n
	}
	m.colors[label] = labelColorPalette[next]
	m.dirty = true
}

func (m *LabelColorsOverlay) clear() {
	if len(m.labels) == 0 {
		return
	}
	label := m.labels[m.cursor]
	if _, ok := m.colors[label]; ok {
		delete(m.colors, label)
		m.dirty = true
	}
}

func (m *LabelColorsOverlay) confirm() tea.Cmd {
	colors := make(map[string]string, len(m.colors))
	for k, v := range m.colors {
		colors[k] = v
	}
	return func() tea.Msg { return LabelColorsChangedMsg{Colors: colors} }
}

// Colors returns the working color map (for testing).
func (m *LabelColorsOverlay) Colors() map[string]string { return m.colors }

func paletteIndex(hex string) int {
	for i, c := range labelColorPalette {
		if c == hex {
			return i
		}
	}
	return -1
}

// View renders the label color picker.
func (m *LabelColorsOverlay) View() string {
	b := NewOverlayBuilder(OverlaySizeStandard, 0)
	b.Line(styleOverlaySectionLabel().Render("Label Colors"))
	b.Line(b.Divider())

	if len(m.labels) == 0 {
		b.BlankLine()
		b.Line(styleStatsDim().Render("No labels in this project yet."))
		b.BlankLine()
		b.Footer([]footerHint{{"esc", "Close"}})
		return b.Build()
	}

	bg := theme.Current().Background()
	for i, label := range m.labels {
		hex, custom := m.colors[label]
		var color lipgloss.TerminalColor = theme.Current().Info()
		if custom && hex != "" {
			color = lipgloss.Color(hex)
		}
		chip := renderChipWithColors(label, color, bg, false)

		cursor := "  "
		if i == m.cursor {
			cursor = styleStatusSelected().Render("→ ")
		}
		swatch := styleStatsDim().Render("default")
		if custom {
			swatch = styleStatsDim().Render(hex)
		}
		b.Line(cursor + chip + "  " + swatch)
	}

	b.BlankLine()
	b.Footer([]footerHint{
		{"←→/space", "Color"},
		{"↑↓", "Navigate"},
		{"d", "Clear"},
		{"⏎", "Save"},
		{"esc", "Close"},
	})
	return b.Build()
}

// Layer returns a centered layer for the overlay.
func (m *LabelColorsOverlay) Layer(width, height, topMargin, bottomMargin int) Layer {
	return BaseOverlayLayer(m.View, width, height, topMargin, bottomMargin)
}
