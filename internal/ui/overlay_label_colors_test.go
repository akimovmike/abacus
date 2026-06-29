package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keyRunes(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestLabelColorsOverlaySortsLabelsAndStartsAtTop(t *testing.T) {
	o := NewLabelColorsOverlay([]string{"ui", "bug", "api"}, nil)
	if got := o.labels; len(got) != 3 || got[0] != "api" || got[1] != "bug" || got[2] != "ui" {
		t.Fatalf("expected sorted [api bug ui], got %v", got)
	}
	if o.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", o.cursor)
	}
}

func TestLabelColorsOverlayCyclesColorForCurrentLabel(t *testing.T) {
	o := NewLabelColorsOverlay([]string{"bug"}, nil)

	// First cycle assigns the first palette color.
	o, _ = o.Update(keyRunes(' '))
	if got := o.Colors()["bug"]; got != labelColorPalette[0] {
		t.Fatalf("after first cycle expected %q, got %q", labelColorPalette[0], got)
	}

	// Second cycle advances to the next palette color.
	o, _ = o.Update(keyRunes(' '))
	if got := o.Colors()["bug"]; got != labelColorPalette[1] {
		t.Fatalf("after second cycle expected %q, got %q", labelColorPalette[1], got)
	}
}

func TestLabelColorsOverlayClearResetsToDefault(t *testing.T) {
	o := NewLabelColorsOverlay([]string{"bug"}, map[string]string{"bug": labelColorPalette[2]})

	o, _ = o.Update(keyRunes('d'))
	if _, ok := o.Colors()["bug"]; ok {
		t.Fatalf("expected bug override cleared, got %v", o.Colors())
	}
}

func TestLabelColorsOverlayEscWithoutChangesCancels(t *testing.T) {
	o := NewLabelColorsOverlay([]string{"bug"}, map[string]string{"bug": labelColorPalette[0]})

	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command from esc")
	}
	if _, ok := cmd().(LabelColorsCancelledMsg); !ok {
		t.Fatalf("expected LabelColorsCancelledMsg, got %T", cmd())
	}
}

func TestLabelColorsOverlayEscWithChangesEmitsChanged(t *testing.T) {
	o := NewLabelColorsOverlay([]string{"bug"}, nil)
	o, _ = o.Update(keyRunes(' ')) // assign a color → dirty

	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command from esc")
	}
	msg, ok := cmd().(LabelColorsChangedMsg)
	if !ok {
		t.Fatalf("expected LabelColorsChangedMsg, got %T", cmd())
	}
	if msg.Colors["bug"] != labelColorPalette[0] {
		t.Fatalf("expected changed colors to include bug=%q, got %v", labelColorPalette[0], msg.Colors)
	}
}

func TestLabelColorsOverlayNoLabelsIsSafe(t *testing.T) {
	o := NewLabelColorsOverlay(nil, nil)
	o, _ = o.Update(keyRunes(' '))
	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyDown})
	if len(o.Colors()) != 0 {
		t.Fatalf("expected no colors with no labels, got %v", o.Colors())
	}
}
