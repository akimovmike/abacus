package ui

import (
	"strings"
	"testing"

	"abacus/internal/beads"
	"abacus/internal/graph"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBackdropClickDismissesStatusOverlayLikeEscape(t *testing.T) {
	app := pointerOverlayTestApp()
	app.activeOverlay = OverlayStatus
	app.statusOverlay = NewStatusOverlay("ab-1", "First", "open")

	model, cmd := app.Update(plainMouseClick(0, 0))
	afterApp := model.(*App)

	if cmd == nil {
		t.Fatal("expected backdrop click to return status cancel command")
	}
	afterApp = runPointerCommand(t, afterApp, cmd)

	if afterApp.activeOverlay != OverlayNone {
		t.Fatalf("expected status overlay to close, got %v", afterApp.activeOverlay)
	}
	if afterApp.statusOverlay != nil {
		t.Fatal("expected status overlay model to be cleared")
	}
}

func TestClickEnabledStatusOptionActivatesLikeHotkey(t *testing.T) {
	app := pointerOverlayTestApp()
	app.activeOverlay = OverlayStatus
	app.statusOverlay = NewStatusOverlay("ab-1", "First", "open")

	model, cmd := app.Update(clickOverlayText(t, app, "In Progress"))
	afterApp := model.(*App)

	if cmd == nil {
		t.Fatal("expected status option click to return status change command")
	}
	afterApp = runPointerCommand(t, afterApp, cmd)

	if afterApp.activeOverlay != OverlayNone {
		t.Fatalf("expected status overlay to close, got %v", afterApp.activeOverlay)
	}
	if !afterApp.statusToastVisible {
		t.Fatal("expected status toast to be visible")
	}
	if afterApp.statusToastNewStatus != "in_progress" {
		t.Fatalf("expected status toast for in_progress, got %s", afterApp.statusToastNewStatus)
	}
}

func TestClickPriorityOptionActivatesLikeHotkey(t *testing.T) {
	app := pointerOverlayTestApp()
	app.activeOverlay = OverlayPriority
	app.priorityOverlay = NewPriorityOverlay("ab-1", "First", 2)

	model, cmd := app.Update(clickOverlayText(t, app, "P1"))
	afterApp := model.(*App)

	if cmd == nil {
		t.Fatal("expected priority option click to return priority change command")
	}
	afterApp = runPointerCommand(t, afterApp, cmd)

	if afterApp.activeOverlay != OverlayNone {
		t.Fatalf("expected priority overlay to close, got %v", afterApp.activeOverlay)
	}
	if !afterApp.priorityToastVisible {
		t.Fatal("expected priority toast to be visible")
	}
	if afterApp.priorityToastNewPriority != 1 {
		t.Fatalf("expected priority toast for P1, got P%d", afterApp.priorityToastNewPriority)
	}
}

func TestClickDisabledStatusOptionIsInert(t *testing.T) {
	app := pointerOverlayTestApp()
	app.activeOverlay = OverlayStatus
	app.statusOverlay = NewStatusOverlay("ab-1", "First", "closed")
	app.cursor = 1
	app.focus = FocusDetails

	model, cmd := app.Update(clickOverlayText(t, app, "In Progress"))
	afterApp := model.(*App)

	if cmd != nil {
		t.Fatal("expected disabled status option click to return nil command")
	}
	if afterApp.activeOverlay != OverlayStatus {
		t.Fatalf("expected status overlay to remain open, got %v", afterApp.activeOverlay)
	}
	if afterApp.cursor != 1 {
		t.Fatalf("expected tree selection to remain 1, got %d", afterApp.cursor)
	}
	if afterApp.focus != FocusDetails {
		t.Fatalf("expected focus to remain details, got %v", afterApp.focus)
	}
	if afterApp.statusToastVisible {
		t.Fatal("expected no status toast for disabled option")
	}
}

func TestClickInsideOverlayWithoutActionIsConsumed(t *testing.T) {
	app := pointerOverlayTestApp()
	app.activeOverlay = OverlayStatus
	app.statusOverlay = NewStatusOverlay("ab-1", "First", "open")
	app.cursor = 1
	app.focus = FocusDetails
	app.viewport.YOffset = 2

	layout := app.pointerLayout()
	model, cmd := app.Update(plainMouseClick(layout.overlaySurface.x+1, layout.overlaySurface.y+1))
	afterApp := model.(*App)

	if cmd != nil {
		t.Fatal("expected inert overlay click to return nil command")
	}
	if afterApp.activeOverlay != OverlayStatus {
		t.Fatalf("expected status overlay to remain open, got %v", afterApp.activeOverlay)
	}
	if afterApp.cursor != 1 {
		t.Fatalf("expected tree selection to remain 1, got %d", afterApp.cursor)
	}
	if afterApp.focus != FocusDetails {
		t.Fatalf("expected focus to remain details, got %v", afterApp.focus)
	}
	if afterApp.viewport.YOffset != 2 {
		t.Fatalf("expected details scroll to remain 2, got %d", afterApp.viewport.YOffset)
	}
}

func TestClickOnOverlayBorderBesideOptionIsInert(t *testing.T) {
	app := pointerOverlayTestApp()
	app.activeOverlay = OverlayStatus
	app.statusOverlay = NewStatusOverlay("ab-1", "First", "open")
	optionClick := clickOverlayText(t, app, "In Progress")
	layout := app.pointerLayout()

	_, cmd := app.Update(plainMouseClick(layout.overlaySurface.x, optionClick.Y))

	if cmd != nil {
		t.Fatal("expected border click beside option row to be inert")
	}
}

func TestBackdropClickDismissesSimpleOverlaysLikeEscape(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*App)
		check func(*testing.T, *App)
	}{
		{
			name: "priority",
			setup: func(app *App) {
				app.activeOverlay = OverlayPriority
				app.priorityOverlay = NewPriorityOverlay("ab-1", "First", 2)
			},
			check: func(t *testing.T, app *App) {
				t.Helper()
				if app.activeOverlay != OverlayNone || app.priorityOverlay != nil {
					t.Fatalf("expected priority overlay closed, got overlay=%v model=%v", app.activeOverlay, app.priorityOverlay)
				}
			},
		},
		{
			name: "delete",
			setup: func(app *App) {
				app.activeOverlay = OverlayDelete
				app.deleteOverlay = NewDeleteOverlay("ab-1", "First", nil, nil)
			},
			check: func(t *testing.T, app *App) {
				t.Helper()
				if app.activeOverlay != OverlayNone || app.deleteOverlay != nil {
					t.Fatalf("expected delete overlay closed, got overlay=%v model=%v", app.activeOverlay, app.deleteOverlay)
				}
			},
		},
		{
			name: "labels",
			setup: func(app *App) {
				app.activeOverlay = OverlayLabels
				app.labelsOverlay = NewLabelsOverlay("ab-1", "First", []string{"ui"}, []string{"ui"})
			},
			check: func(t *testing.T, app *App) {
				t.Helper()
				if app.activeOverlay != OverlayNone || app.labelsOverlay != nil {
					t.Fatalf("expected labels overlay closed, got overlay=%v model=%v", app.activeOverlay, app.labelsOverlay)
				}
			},
		},
		{
			name: "help",
			setup: func(app *App) {
				app.showHelp = true
			},
			check: func(t *testing.T, app *App) {
				t.Helper()
				if app.showHelp {
					t.Fatal("expected help overlay closed")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := pointerOverlayTestApp()
			tt.setup(app)

			model, cmd := app.Update(plainMouseClick(0, 0))
			afterApp := model.(*App)
			afterApp = runPointerCommand(t, afterApp, cmd)

			tt.check(t, afterApp)
		})
	}
}

func TestBackdropClickFollowsCreateAndCommentStagedEscape(t *testing.T) {
	t.Run("create backend error stage", func(t *testing.T) {
		app := pointerOverlayTestApp()
		app.activeOverlay = OverlayCreate
		app.createOverlay = NewCreateOverlay(CreateOverlayOptions{})
		app.createOverlay.hasBackendError = true
		app.showErrorToast = true

		model, cmd := app.Update(plainMouseClick(0, 0))
		afterApp := model.(*App)

		if cmd == nil {
			t.Fatal("expected create backdrop click to return dismiss-error command")
		}
		afterApp = runPointerCommand(t, afterApp, cmd)

		if afterApp.activeOverlay != OverlayCreate {
			t.Fatalf("expected create overlay to remain open, got %v", afterApp.activeOverlay)
		}
		if afterApp.createOverlay == nil || afterApp.createOverlay.hasBackendError {
			t.Fatal("expected create backend error stage to clear without cancelling overlay")
		}
		if afterApp.showErrorToast {
			t.Fatal("expected global error toast to be dismissed")
		}
	})

	t.Run("comment text clear stage", func(t *testing.T) {
		app := pointerOverlayTestApp()
		app.activeOverlay = OverlayComment
		app.commentOverlay = NewCommentOverlay("ab-1", "First")
		app.commentOverlay.textarea.SetValue("do not discard")

		model, cmd := app.Update(plainMouseClick(0, 0))
		afterApp := model.(*App)

		if cmd != nil {
			t.Fatal("expected comment text-clear stage to return nil command")
		}
		if afterApp.activeOverlay != OverlayComment {
			t.Fatalf("expected comment overlay to remain open, got %v", afterApp.activeOverlay)
		}
		if afterApp.commentOverlay == nil || afterApp.commentOverlay.textarea.Value() != "" {
			t.Fatal("expected comment backdrop click to clear text without cancelling overlay")
		}
	})
}

func TestCreateAndEditPropertyPillsAreMouseInert(t *testing.T) {
	tests := []struct {
		name    string
		overlay *CreateOverlay
		text    string
	}{
		{
			name:    "create priority pill",
			overlay: NewCreateOverlay(CreateOverlayOptions{}),
			text:    "High",
		},
		{
			name: "edit priority pill",
			overlay: NewEditOverlay(&beads.FullIssue{
				ID:        "ab-1",
				Title:     "First",
				IssueType: "task",
				Priority:  2,
			}, CreateOverlayOptions{}),
			text: "High",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := pointerOverlayTestApp()
			app.activeOverlay = OverlayCreate
			app.createOverlay = tt.overlay
			beforePriority := tt.overlay.Priority()
			beforeFocus := tt.overlay.Focus()

			model, cmd := app.Update(clickOverlayText(t, app, tt.text))
			afterApp := model.(*App)

			if cmd != nil {
				t.Fatal("expected create/edit property pill click to return nil command")
			}
			if afterApp.activeOverlay != OverlayCreate {
				t.Fatalf("expected create overlay to remain open, got %v", afterApp.activeOverlay)
			}
			if afterApp.createOverlay.Priority() != beforePriority {
				t.Fatalf("expected priority to remain P%d, got P%d", beforePriority, afterApp.createOverlay.Priority())
			}
			if afterApp.createOverlay.Focus() != beforeFocus {
				t.Fatalf("expected focus to remain %v, got %v", beforeFocus, afterApp.createOverlay.Focus())
			}
		})
	}
}

func pointerOverlayTestApp() *App {
	nodes := []*graph.Node{
		{Issue: beads.FullIssue{ID: "ab-1", Title: "First", Status: "open", Priority: 2}},
		{Issue: beads.FullIssue{ID: "ab-2", Title: "Second", Status: "open", Priority: 3}},
	}
	app := pointerTestApp()
	app.roots = nodes
	app.visibleRows = nodesToRows(nodes...)
	app.cursor = 0
	app.detailIssueID = "ab-1"
	return app
}

func plainMouseClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}
}

func runPointerCommand(t *testing.T, app *App, cmd tea.Cmd) *App {
	t.Helper()
	if cmd == nil {
		return app
	}
	model, _ := app.Update(cmd())
	return model.(*App)
}

func clickOverlayText(t *testing.T, app *App, text string) tea.MouseMsg {
	t.Helper()
	layout := app.pointerLayout()
	if layout.overlaySurface.empty() {
		t.Fatal("expected overlay surface bounds")
	}
	layer := app.overlayLayerForPointerTest()
	if layer == nil {
		t.Fatal("expected overlay layer")
	}
	canvas := layer.Render()
	if canvas == nil {
		t.Fatal("expected overlay canvas")
	}
	for y, line := range strings.Split(stripANSI(canvas.Render()), "\n") {
		if x := strings.Index(line, text); x >= 0 {
			return plainMouseClick(layout.overlaySurface.x+x, layout.overlaySurface.y+y)
		}
	}
	t.Fatalf("overlay text %q not found in:\n%s", text, stripANSI(canvas.Render()))
	return tea.MouseMsg{}
}

func (m *App) overlayLayerForPointerTest() Layer {
	const headerHeight = 1
	const bottomMargin = 1

	switch {
	case m.activeOverlay == OverlayStatus && m.statusOverlay != nil:
		return m.statusOverlay.Layer(m.width, m.height, headerHeight, bottomMargin)
	case m.activeOverlay == OverlayPriority && m.priorityOverlay != nil:
		return m.priorityOverlay.Layer(m.width, m.height, headerHeight, bottomMargin)
	case m.activeOverlay == OverlayLabels && m.labelsOverlay != nil:
		return m.labelsOverlay.Layer(m.width, m.height, headerHeight, bottomMargin)
	case m.activeOverlay == OverlayCreate && m.createOverlay != nil:
		return m.createOverlay.Layer(m.width, m.height, headerHeight, bottomMargin)
	case m.activeOverlay == OverlayDelete && m.deleteOverlay != nil:
		return m.deleteOverlay.Layer(m.width, m.height, headerHeight, bottomMargin)
	case m.activeOverlay == OverlayComment && m.commentOverlay != nil:
		return m.commentOverlay.Layer(m.width, m.height, headerHeight, bottomMargin)
	case m.showHelp:
		return newHelpOverlayLayer(m.keys, m.width, m.height, headerHeight, bottomMargin)
	default:
		return nil
	}
}
