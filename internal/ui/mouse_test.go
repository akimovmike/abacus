package ui

import (
	"testing"

	"abacus/internal/beads"
	"abacus/internal/graph"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPointerEventFromMouseMsgRoutesOnlyFirstPassEvents(t *testing.T) {
	app := pointerTestApp()

	tests := []struct {
		name       string
		msg        tea.MouseMsg
		wantAction pointerAction
		wantTarget pointerTarget
		wantOK     bool
	}{
		{
			name: "left press is plain click",
			msg: tea.MouseMsg{
				X:      2,
				Y:      2,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonLeft,
			},
			wantAction: pointerActionPlainClick,
			wantTarget: pointerTargetTree,
			wantOK:     true,
		},
		{
			name: "wheel up press is routed",
			msg: tea.MouseMsg{
				X:      70,
				Y:      2,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonWheelUp,
			},
			wantAction: pointerActionWheelUp,
			wantTarget: pointerTargetDetails,
			wantOK:     true,
		},
		{
			name: "wheel down press is routed",
			msg: tea.MouseMsg{
				X:      70,
				Y:      2,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonWheelDown,
			},
			wantAction: pointerActionWheelDown,
			wantTarget: pointerTargetDetails,
			wantOK:     true,
		},
		{
			name: "right click is unsupported",
			msg: tea.MouseMsg{
				X:      2,
				Y:      2,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonRight,
			},
		},
		{
			name: "middle click is unsupported",
			msg: tea.MouseMsg{
				X:      2,
				Y:      2,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonMiddle,
			},
		},
		{
			name: "drag motion is unsupported",
			msg: tea.MouseMsg{
				X:      2,
				Y:      2,
				Action: tea.MouseActionMotion,
				Button: tea.MouseButtonLeft,
			},
		},
		{
			name: "release is unsupported",
			msg: tea.MouseMsg{
				X:      2,
				Y:      2,
				Action: tea.MouseActionRelease,
				Button: tea.MouseButtonLeft,
			},
		},
		{
			name: "horizontal wheel is unsupported",
			msg: tea.MouseMsg{
				X:      2,
				Y:      2,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonWheelLeft,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := app.pointerEventFromMouseMsg(tt.msg)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.action != tt.wantAction {
				t.Fatalf("action = %v, want %v", got.action, tt.wantAction)
			}
			if got.target != tt.wantTarget {
				t.Fatalf("target = %v, want %v", got.target, tt.wantTarget)
			}
		})
	}
}

func TestPointerTargetAtDistinguishesLayoutAreas(t *testing.T) {
	t.Run("wide layout with details", func(t *testing.T) {
		app := pointerTestApp()

		assertPointerTarget(t, app, 2, 0, pointerTargetInertChrome)
		assertPointerTarget(t, app, 2, 2, pointerTargetTree)
		assertPointerTarget(t, app, 70, 2, pointerTargetDetails)
		assertPointerTarget(t, app, 2, 23, pointerTargetInertChrome)
	})

	t.Run("tall layout with details", func(t *testing.T) {
		app := pointerTestApp()
		app.layout = LayoutTall
		app.recalcViewportSize()

		assertPointerTarget(t, app, 2, 2, pointerTargetTree)
		assertPointerTarget(t, app, 2, 17, pointerTargetDetails)
	})

	t.Run("detail-hidden layout", func(t *testing.T) {
		app := pointerTestApp()
		app.ShowDetails = false

		assertPointerTarget(t, app, 2, 2, pointerTargetTree)
		assertPointerTarget(t, app, 70, 2, pointerTargetTree)
	})

	t.Run("active overlay separates surface from backdrop", func(t *testing.T) {
		app := pointerTestApp()
		app.activeOverlay = OverlayStatus
		app.statusOverlay = NewStatusOverlay("ab-1", "First", "open")

		layout := app.pointerLayout()
		if layout.overlaySurface.empty() {
			t.Fatal("expected overlay surface bounds")
		}

		assertPointerTarget(t, app, layout.overlaySurface.x, layout.overlaySurface.y, pointerTargetOverlay)
		assertPointerTarget(t, app, 0, 0, pointerTargetBackdrop)
	})
}

func TestUnsupportedMouseMsgIsConsumedWithoutChangingAppState(t *testing.T) {
	app := pointerTestApp()
	app.cursor = 1
	app.focus = FocusDetails
	app.treeTopLine = 3
	app.viewport.YOffset = 4
	app.activeOverlay = OverlayStatus
	app.statusOverlay = NewStatusOverlay("ab-2", "Second", "open")

	before := app.capturePointerInvariant()
	model, cmd := app.Update(tea.MouseMsg{
		X:      2,
		Y:      2,
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
	})
	afterApp := model.(*App)

	if cmd != nil {
		t.Fatal("expected unsupported mouse event to return nil command")
	}
	if got := afterApp.capturePointerInvariant(); got != before {
		t.Fatalf("unsupported mouse event changed state: got %+v, want %+v", got, before)
	}
}

func assertPointerTarget(t *testing.T, app *App, x, y int, want pointerTarget) {
	t.Helper()

	if got := app.pointerTargetAt(x, y); got != want {
		t.Fatalf("target at (%d,%d) = %v, want %v", x, y, got, want)
	}
}

func pointerTestApp() *App {
	nodes := []*graph.Node{
		{Issue: beads.FullIssue{ID: "ab-1", Title: "First", Status: "open"}},
		{Issue: beads.FullIssue{ID: "ab-2", Title: "Second", Status: "open"}},
	}
	app := &App{
		roots:       nodes,
		width:       100,
		height:      24,
		ready:       true,
		ShowDetails: true,
		focus:       FocusTree,
		layout:      LayoutWide,
		viewport:    viewport.New(30, 19),
		keys:        DefaultKeyMap(),
	}
	app.recalcVisibleRows()
	app.recalcViewportSize()
	return app
}

type pointerInvariant struct {
	cursor          int
	focus           FocusArea
	treeTopLine     int
	viewportYOffset int
	activeOverlay   OverlayType
	showHelp        bool
}

func (m *App) capturePointerInvariant() pointerInvariant {
	return pointerInvariant{
		cursor:          m.cursor,
		focus:           m.focus,
		treeTopLine:     m.treeTopLine,
		viewportYOffset: m.viewport.YOffset,
		activeOverlay:   m.activeOverlay,
		showHelp:        m.showHelp,
	}
}
