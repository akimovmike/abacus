package ui

import (
	"strings"
	"testing"

	"abacus/internal/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
)

func TestColumnsOverlayRendersMasterToggleAsCheckbox(t *testing.T) {
	overlay := NewColumnsOverlay(nil)
	view := stripANSI(overlay.View())

	if !strings.Contains(view, "[x] Show columns") {
		t.Fatalf("expected master toggle to render as a checkbox:\n%s", view)
	}
	if strings.Contains(view, "Show columns: On") || strings.Contains(view, "Show columns: Off") {
		t.Fatalf("master toggle should not render as read-only state text:\n%s", view)
	}
	for _, label := range []string{"[x] Last Updated", "[x] Assignee", "[x] Comments"} {
		if !strings.Contains(view, label) {
			t.Fatalf("expected column checkbox %q, got:\n%s", label, view)
		}
	}
}

func TestColumnsOverlayFooterHintsFollowCurrentRowKind(t *testing.T) {
	overlay := NewColumnsOverlay(nil)
	overlay.labelColumns = []LabelColumnConfig{
		{Label: "feature-ui-redesign", DisplayName: "redesign", Enabled: true},
	}

	overlay.cursor = 0
	assertFooterHints(t, overlay.footerHints(), []footerHint{
		{"↑↓", "Navigate"},
		{"space", "Toggle"},
		{"esc", "Close"},
	})

	overlay.cursor = 1
	assertFooterHints(t, overlay.footerHints(), []footerHint{
		{"↑↓", "Navigate"},
		{"space", "Toggle"},
		{"esc", "Close"},
	})

	overlay.cursor = 4
	assertFooterHints(t, overlay.footerHints(), []footerHint{
		{"↑↓", "Navigate"},
		{"space", "Toggle"},
		{"e", "Rename"},
		{"d", "Remove"},
		{"esc", "Close"},
	})

	overlay.cursor = len(overlay.rows()) - 1
	assertFooterHints(t, overlay.footerHints(), []footerHint{
		{"↑↓", "Navigate"},
		{"enter", "Add"},
		{"esc", "Close"},
	})
}

func TestColumnsOverlaySeparatesMasterToggleAndColumnRows(t *testing.T) {
	overlay := NewColumnsOverlay(nil)
	lines := strings.Split(stripANSI(overlay.View()), "\n")

	masterLine := -1
	firstColumnLine := -1
	for i, line := range lines {
		if strings.Contains(line, "[x] Show columns") || strings.Contains(line, "[ ] Show columns") {
			masterLine = i
		}
		if strings.Contains(line, "[x] Last Updated") || strings.Contains(line, "[ ] Last Updated") {
			firstColumnLine = i
		}
	}
	if masterLine < 0 || firstColumnLine < 0 || firstColumnLine <= masterLine {
		t.Fatalf("expected master toggle before column rows, got:\n%s", stripANSI(overlay.View()))
	}
	for _, line := range lines[masterLine+1 : firstColumnLine] {
		if strings.Trim(line, " │") == "" {
			return
		}
	}
	t.Fatalf("expected blank visual separator between master toggle and column rows:\n%s", stripANSI(overlay.View()))
}

func TestColumnsOverlaySeparatesBuiltinAndLabelColumns(t *testing.T) {
	overlay := NewColumnsOverlay(nil)
	overlay.labelColumns = []LabelColumnConfig{
		{Label: "feature-ui-redesign", DisplayName: "redesign", Enabled: true},
	}
	lines := strings.Split(stripANSI(overlay.View()), "\n")

	commentsLine := -1
	labelLine := -1
	for i, line := range lines {
		if strings.Contains(line, "[x] Comments") {
			commentsLine = i
		}
		if strings.Contains(line, "feature-ui-redesign") && strings.Contains(line, "[redesign]") {
			labelLine = i
		}
	}
	if commentsLine < 0 || labelLine < 0 || labelLine <= commentsLine {
		t.Fatalf("expected comments row before label row, got:\n%s", stripANSI(overlay.View()))
	}
	for _, line := range lines[commentsLine+1 : labelLine] {
		if strings.Trim(line, " │") == "" {
			return
		}
	}
	t.Fatalf("expected blank visual separator between built-in and label columns:\n%s", stripANSI(overlay.View()))
}

func assertFooterHints(t *testing.T, got, want []footerHint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d footer hints, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("footer hint %d = %#v, want %#v (all hints: %#v)", i, got[i], want[i], got)
		}
	}
}

func TestColumnsOverlayAddsLabelColumnWithDefaultDisplayName(t *testing.T) {
	overlay := NewColumnsOverlay([]string{"backend", "feature-ui-redesign"})
	overlay.cursor = len(overlay.rows()) - 1

	updated, cmd := overlay.Update(tea.KeyMsg{Type: tea.KeyEnter})
	overlay = updated
	if cmd != nil {
		t.Fatal("expected entering add mode to be synchronous")
	}
	if !overlay.addingLabel {
		t.Fatal("expected add label picker mode")
	}

	updated, cmd = overlay.Update(ComboBoxEnterSelectedMsg{Value: "feature-ui-redesign"})
	overlay = updated
	if cmd != nil {
		t.Fatal("expected label selection to update overlay without app command")
	}

	if overlay.addingLabel {
		t.Fatal("expected add label picker to close after selection")
	}
	if !overlay.editingLabel {
		t.Fatal("expected display-name edit mode after selecting label")
	}
	if overlay.displayNameInput.Value() != "redesign" {
		t.Fatalf("expected display-name input to be prefilled with redesign, got %q", overlay.displayNameInput.Value())
	}
	if len(overlay.labelColumns) != 1 {
		t.Fatalf("expected one label column, got %d", len(overlay.labelColumns))
	}
	got := overlay.labelColumns[0]
	if got.Label != "feature-ui-redesign" {
		t.Fatalf("expected feature-ui-redesign label, got %q", got.Label)
	}
	if got.DisplayName != "redesign" {
		t.Fatalf("expected default display name redesign, got %q", got.DisplayName)
	}
	if !got.Enabled {
		t.Fatal("expected added label column to be enabled")
	}
}

func TestColumnsOverlayTypingAfterAddReplacesSuggestedDisplayName(t *testing.T) {
	overlay := NewColumnsOverlay([]string{"feature-ui-redesign"})
	overlay.cursor = len(overlay.rows()) - 1

	updated, _ := overlay.Update(tea.KeyMsg{Type: tea.KeyEnter})
	overlay = updated
	updated, _ = overlay.Update(ComboBoxEnterSelectedMsg{Value: "feature-ui-redesign"})
	overlay = updated
	updated, _ = overlay.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("UI")})
	overlay = updated
	if overlay.displayNameInput.Value() != "UI" {
		t.Fatalf("expected typed name to replace suggested default, got %q", overlay.displayNameInput.Value())
	}

	updated, _ = overlay.Update(tea.KeyMsg{Type: tea.KeyEnter})
	overlay = updated
	if overlay.labelColumns[0].DisplayName != "UI" {
		t.Fatalf("expected committed display name UI, got %q", overlay.labelColumns[0].DisplayName)
	}
}

func TestColumnsOverlayEditsAndRemovesLabelColumn(t *testing.T) {
	overlay := NewColumnsOverlay(nil)
	overlay.labelColumns = []LabelColumnConfig{
		{Label: "ui-redesign", DisplayName: "redesign", Enabled: true},
	}
	overlay.cursor = 4

	updated, cmd := overlay.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	overlay = updated
	if cmd != nil {
		t.Fatal("expected edit mode to start synchronously")
	}
	if !overlay.editingLabel {
		t.Fatal("expected inline display-name edit mode")
	}

	overlay.displayNameInput.SetValue("UI")
	updated, cmd = overlay.Update(tea.KeyMsg{Type: tea.KeyEnter})
	overlay = updated
	if cmd != nil {
		t.Fatal("expected committing display name to return no command")
	}
	if overlay.editingLabel {
		t.Fatal("expected edit mode to close after Enter")
	}
	if overlay.labelColumns[0].DisplayName != "UI" {
		t.Fatalf("expected edited display name UI, got %q", overlay.labelColumns[0].DisplayName)
	}

	updated, cmd = overlay.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	overlay = updated
	if cmd != nil {
		t.Fatal("expected removing label column to return no command")
	}
	if len(overlay.labelColumns) != 0 {
		t.Fatalf("expected label column to be removed, got %v", overlay.labelColumns)
	}
}

func TestColumnsOverlayEscFromEditingReturnsToMainView(t *testing.T) {
	overlay := NewColumnsOverlay(nil)
	overlay.labelColumns = []LabelColumnConfig{
		{Label: "ui-redesign", DisplayName: "redesign", Enabled: true},
	}
	overlay.cursor = 4

	updated, _ := overlay.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	overlay = updated
	if !overlay.editingLabel {
		t.Fatal("expected edit mode")
	}

	updated, cmd := overlay.Update(tea.KeyMsg{Type: tea.KeyEscape})
	overlay = updated
	if cmd != nil {
		t.Fatal("expected Esc from editing to return no command")
	}
	if overlay.editingLabel {
		t.Fatal("expected Esc to exit edit mode")
	}
	if overlay.closed {
		t.Fatal("expected Esc from editing to stay in overlay, not close it")
	}
}

func TestColumnsOverlayEscFromAddingReturnsToMainView(t *testing.T) {
	overlay := NewColumnsOverlay([]string{"backend"})
	overlay.cursor = len(overlay.rows()) - 1

	updated, _ := overlay.Update(tea.KeyMsg{Type: tea.KeyEnter})
	overlay = updated
	if !overlay.addingLabel {
		t.Fatal("expected adding mode")
	}

	updated, cmd := overlay.Update(tea.KeyMsg{Type: tea.KeyEscape})
	overlay = updated
	if cmd != nil {
		t.Fatal("expected Esc from adding to return no command")
	}
	if overlay.addingLabel {
		t.Fatal("expected Esc to exit adding mode")
	}
	if overlay.closed {
		t.Fatal("expected Esc from adding to stay in overlay, not close it")
	}
}

func TestColumnsOverlayEscFromMainViewClosesOverlay(t *testing.T) {
	overlay := NewColumnsOverlay(nil)

	updated, cmd := overlay.Update(tea.KeyMsg{Type: tea.KeyEscape})
	overlay = updated
	if cmd != nil {
		t.Fatal("expected Esc from main view to return no command")
	}
	if !overlay.closed {
		t.Fatal("expected Esc from main view to close the overlay")
	}
}

func TestColumnsOverlayFooterHintsInEditingAndAddingModes(t *testing.T) {
	overlay := NewColumnsOverlay([]string{"backend"})
	overlay.labelColumns = []LabelColumnConfig{
		{Label: "ui-redesign", DisplayName: "redesign", Enabled: true},
	}

	overlay.editingLabel = true
	assertFooterHints(t, overlay.footerHints(), []footerHint{
		{"enter", "Apply"},
		{"esc", "Back"},
	})

	overlay.editingLabel = false
	overlay.addingLabel = true
	assertFooterHints(t, overlay.footerHints(), []footerHint{
		{"↑↓", "Navigate"},
		{"enter", "Add"},
		{"esc", "Back"},
	})
}

func TestColumnsOverlayLabelRowRendersPillAndBracketedDisplayName(t *testing.T) {
	overlay := NewColumnsOverlay(nil)
	overlay.labelColumns = []LabelColumnConfig{
		{Label: "ready-for-agent", DisplayName: "agent", Enabled: true},
	}
	view := stripANSI(overlay.View())

	if !strings.Contains(view, "ready-for-agent") {
		t.Fatalf("expected label name in view:\n%s", view)
	}
	if !strings.Contains(view, "→") {
		t.Fatalf("expected arrow separator in label row:\n%s", view)
	}
	if !strings.Contains(view, "[agent]") {
		t.Fatalf("expected bracketed display name [agent] in view:\n%s", view)
	}
}

func TestColumnsOverlayLabelSeparatorKeepsOverlayBackground(t *testing.T) {
	prevTheme := theme.CurrentName()
	t.Cleanup(func() { _ = theme.SetTheme(prevTheme) })
	if !theme.SetTheme("dracula") {
		t.Fatal("expected dracula theme to be registered")
	}

	overlay := NewColumnsOverlay(nil)
	overlay.labelColumns = []LabelColumnConfig{
		{Label: "ready-for-agent", DisplayName: "agent", Enabled: true},
	}
	layer := overlay.Layer(80, 24, 1, 1)
	if layer == nil {
		t.Fatal("expected columns overlay layer")
	}
	canvas := layer.Render()
	if canvas == nil {
		t.Fatal("expected columns overlay canvas")
	}

	x, y, ok := findCanvasCell(canvas, "→")
	if !ok {
		t.Fatalf("expected label separator arrow in overlay:\n%s", stripANSI(canvas.Render()))
	}
	for _, cellX := range []int{x - 1, x, x + 1} {
		assertCellHasSecondaryBackground(t, canvas, cellX, y)
	}
}

func TestColumnsOverlayViewOmitsDefaultResetGapAfterPill(t *testing.T) {
	app := &App{
		ready:         true,
		width:         100,
		height:        30,
		repoName:      "abacus",
		activeOverlay: OverlayColumns,
		columnsOverlay: &ColumnsOverlay{
			showColumns: true,
			builtins:    map[string]bool{},
			labelColumns: []LabelColumnConfig{
				{Label: "ready-for-agent", DisplayName: "agent", Enabled: true},
			},
		},
	}

	view := app.View()
	if strings.Contains(view, "\x1b[0m ") {
		t.Fatalf("columns overlay contains default reset gap after styled content: %q", view)
	}
}

func findCanvasCell(canvas *Canvas, value string) (int, int, bool) {
	for y := 0; y < canvas.Height(); y++ {
		for x := 0; x < canvas.Width(); x++ {
			cell := canvas.Cell(x, y)
			if cell != nil && cell.String() == value {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func assertCellHasSecondaryBackground(t *testing.T, canvas *Canvas, x, y int) {
	t.Helper()
	cell := canvas.Cell(x, y)
	if cell == nil {
		t.Fatalf("missing cell at %d,%d", x, y)
	}
	expected, ok := lipglossColorToANSI(theme.Current().BackgroundSecondary())
	if !ok {
		t.Fatal("expected secondary background color to convert to ANSI")
	}
	if cell.Style.Bg == nil {
		t.Fatalf("cell (%d,%d) missing background", x, y)
	}
	gr, gg, gb, _ := cell.Style.Bg.RGBA()
	er, eg, eb, _ := expected.RGBA()
	if !closeUint32(gr, er) || !closeUint32(gg, eg) || !closeUint32(gb, eb) {
		t.Fatalf("cell (%d,%d) background = RGB(%d,%d,%d), want RGB(%d,%d,%d)", x, y, gr, gg, gb, er, eg, eb)
	}
}

func closeUint32(a, b uint32) bool {
	const tolerance = 0x101
	if a > b {
		return a-b <= tolerance
	}
	return b-a <= tolerance
}
