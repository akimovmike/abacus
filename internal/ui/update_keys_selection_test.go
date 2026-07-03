package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTreeApp() *App {
	m := &App{
		selectAnchor: -1,
		cursor:       0,
		keys:         DefaultKeyMap(),
		visibleRows:  rowsFromIDs("ab-1", "ab-2", "ab-3", "ab-4"),
	}
	return m
}

func TestExtendDownStartsAndGrowsSelection(t *testing.T) {
	m := newTreeApp()
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	if !m.selectionActive() {
		t.Fatal("expected selection active after J")
	}
	if m.selectAnchor != 0 || m.cursor != 1 {
		t.Fatalf("expected anchor=0 cursor=1, got anchor=%d cursor=%d", m.selectAnchor, m.cursor)
	}
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	if m.selectAnchor != 0 || m.cursor != 2 {
		t.Fatalf("expected anchor=0 cursor=2, got anchor=%d cursor=%d", m.selectAnchor, m.cursor)
	}
}

func TestExtendUpStartsSelection(t *testing.T) {
	m := newTreeApp()
	m.cursor = 3
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")})
	if m.selectAnchor != 3 || m.cursor != 2 {
		t.Fatalf("expected anchor=3 cursor=2, got anchor=%d cursor=%d", m.selectAnchor, m.cursor)
	}
}

func TestPlainNavigationClearsSelection(t *testing.T) {
	m := newTreeApp()
	m.selectAnchor = 0
	m.cursor = 2
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.selectionActive() {
		t.Fatal("expected plain 'j' to clear selection")
	}
	if m.cursor != 3 {
		t.Fatalf("expected cursor to advance to 3, got %d", m.cursor)
	}
}

func TestExtendDoesNotOverrunBottom(t *testing.T) {
	m := newTreeApp()
	m.cursor = 3 // last row
	m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	if m.cursor != 3 {
		t.Fatalf("expected cursor clamped at 3, got %d", m.cursor)
	}
}
