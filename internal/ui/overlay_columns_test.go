package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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
