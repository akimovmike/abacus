package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestPlainClickOnTreeRowClearsActiveSelection reproduces the final-review
// bug where a mouse click moved the cursor without clearing an active
// keyboard-driven multi-selection, silently growing the selection range to
// whatever row was clicked (fixed in selectTreeRowAtPointer).
func TestPlainClickOnTreeRowClearsActiveSelection(t *testing.T) {
	app := pointerTreeScrollTestApp(6)
	app.selectAnchor = 1
	app.cursor = 3

	if !app.selectionActive() {
		t.Fatal("expected selection to be active before click")
	}

	model, cmd := app.Update(tea.MouseMsg{
		X:      3,
		Y:      7, // row index 5
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	afterApp := model.(*App)

	if cmd != nil {
		t.Fatal("expected tree click to return nil command")
	}
	if afterApp.selectionActive() {
		t.Fatal("expected mouse click to clear the active selection")
	}
	if afterApp.cursor != 5 {
		t.Fatalf("expected cursor to move to clicked row 5, got %d", afterApp.cursor)
	}
}
