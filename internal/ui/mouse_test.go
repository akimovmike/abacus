package ui

import (
	"strings"
	"testing"
	"time"

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

func TestWheelOverDetailsScrollsDetailsWithoutChangingTreeState(t *testing.T) {
	app := pointerTestApp()
	app.focus = FocusTree
	app.cursor = 1
	app.treeTopLine = 1
	app.viewport.Height = 3
	app.viewport.SetContent(strings.Join([]string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
	}, "\n"))

	model, cmd := app.Update(tea.MouseMsg{
		X:      70,
		Y:      2,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
	})
	afterApp := model.(*App)

	if cmd != nil {
		t.Fatal("expected details wheel event to return nil command")
	}
	if afterApp.viewport.YOffset != 1 {
		t.Fatalf("expected details viewport offset 1, got %d", afterApp.viewport.YOffset)
	}
	if afterApp.focus != FocusTree {
		t.Fatalf("expected focus to remain tree, got %v", afterApp.focus)
	}
	if afterApp.cursor != 1 {
		t.Fatalf("expected tree selection to remain 1, got %d", afterApp.cursor)
	}
	if afterApp.treeTopLine != 1 {
		t.Fatalf("expected tree viewport top to remain 1, got %d", afterApp.treeTopLine)
	}
}

func TestPlainClickInsideDetailsFocusesDetailsWithoutChangingSelection(t *testing.T) {
	app := pointerTestApp()
	app.focus = FocusTree
	app.cursor = 1
	app.treeTopLine = 1

	model, cmd := app.Update(tea.MouseMsg{
		X:      70,
		Y:      2,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	afterApp := model.(*App)

	if cmd != nil {
		t.Fatal("expected details click to return nil command")
	}
	if afterApp.focus != FocusDetails {
		t.Fatalf("expected focus to move to details, got %v", afterApp.focus)
	}
	if afterApp.cursor != 1 {
		t.Fatalf("expected tree selection to remain 1, got %d", afterApp.cursor)
	}
	if afterApp.treeTopLine != 1 {
		t.Fatalf("expected tree viewport top to remain 1, got %d", afterApp.treeTopLine)
	}
}

func TestWheelOverTreeScrollsTreeViewportWithoutChangingSelectionOrFocus(t *testing.T) {
	app := pointerTreeScrollTestApp(12)
	app.focus = FocusDetails
	app.cursor = 0
	app.viewport.YOffset = 2

	model, cmd := app.Update(tea.MouseMsg{
		X:      2,
		Y:      2,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
	})
	afterApp := model.(*App)

	if cmd != nil {
		t.Fatal("expected tree wheel event to return nil command")
	}
	if afterApp.treeTopLine != 1 {
		t.Fatalf("expected tree viewport top 1, got %d", afterApp.treeTopLine)
	}
	if afterApp.cursor != 0 {
		t.Fatalf("expected tree selection to remain 0, got %d", afterApp.cursor)
	}
	if afterApp.focus != FocusDetails {
		t.Fatalf("expected focus to remain details, got %v", afterApp.focus)
	}
	if afterApp.viewport.YOffset != 2 {
		t.Fatalf("expected details viewport offset to remain 2, got %d", afterApp.viewport.YOffset)
	}
}

func TestMouseScrolledTreeViewportCanLeaveSelectionOffScreen(t *testing.T) {
	app := pointerTreeScrollTestApp(12)
	app.cursor = 0

	for i := 0; i < 2; i++ {
		model, _ := app.Update(tea.MouseMsg{
			X:      2,
			Y:      2,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		})
		app = model.(*App)
	}

	view := stripANSI(app.renderTreeView())

	if strings.Contains(view, "ab-tree-a") {
		t.Fatalf("expected mouse-scrolled tree viewport to leave selected row off-screen:\n%s", view)
	}
	if app.treeTopLine != 2 {
		t.Fatalf("expected tree viewport top 2 after render, got %d", app.treeTopLine)
	}
	if app.cursor != 0 {
		t.Fatalf("expected tree selection to remain 0, got %d", app.cursor)
	}
}

func TestKeyboardTreeNavigationRecentersOffScreenSelectionBeforeMoving(t *testing.T) {
	app := pointerTreeScrollTestApp(12)
	app.cursor = 0
	app.treeTopLine = 2
	app.treeMouseScrolled = true

	model, cmd := app.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'j'},
	})
	afterApp := model.(*App)

	if cmd != nil {
		t.Fatal("expected tree navigation to return nil command")
	}
	if afterApp.cursor != 1 {
		t.Fatalf("expected tree selection to move to 1, got %d", afterApp.cursor)
	}
	if afterApp.treeTopLine != 0 {
		t.Fatalf("expected tree viewport to recenter before movement, got top %d", afterApp.treeTopLine)
	}
	if afterApp.treeMouseScrolled {
		t.Fatal("expected keyboard tree navigation to clear mouse-scrolled tree state")
	}
}

func TestRefreshPreservesMouseScrolledTreeViewportByTopRowIdentity(t *testing.T) {
	oldRoots := mouseTreeNodes("ab-a", "ab-b", "ab-c", "ab-d", "ab-e", "ab-f")
	app := pointerTestApp()
	app.roots = oldRoots
	app.height = 10
	app.recalcVisibleRows()
	app.recalcViewportSize()
	app.cursor = 0
	app.treeTopLine = 2
	app.treeMouseScrolled = true

	newRoots := mouseTreeNodes("ab-x", "ab-a", "ab-b", "ab-c", "ab-d", "ab-e", "ab-f")
	app.applyRefresh(newRoots, buildIssueDigest(newRoots), time.Time{})

	if app.treeTopLine != 3 {
		t.Fatalf("expected refresh to keep ab-c at top index 3, got %d", app.treeTopLine)
	}
	if app.visibleRows[app.treeTopLine].Node.Issue.ID != "ab-c" {
		t.Fatalf("expected ab-c to remain top visible row, got %s", app.visibleRows[app.treeTopLine].Node.Issue.ID)
	}
	if app.cursor != 1 {
		t.Fatalf("expected selection ab-a to be restored at index 1, got %d", app.cursor)
	}
}

func TestRefreshPreservesMouseScrolledTreeViewportByApproximateIndexWhenTopRowIsGone(t *testing.T) {
	oldRoots := mouseTreeNodes("ab-a", "ab-b", "ab-c", "ab-d", "ab-e", "ab-f", "ab-g", "ab-h", "ab-i", "ab-j")
	app := pointerTestApp()
	app.roots = oldRoots
	app.height = 10
	app.recalcVisibleRows()
	app.recalcViewportSize()
	app.cursor = 0
	app.treeTopLine = 3
	app.treeMouseScrolled = true

	newRoots := mouseTreeNodes("ab-a", "ab-b", "ab-c", "ab-e", "ab-f", "ab-g", "ab-h", "ab-i", "ab-j")
	app.applyRefresh(newRoots, buildIssueDigest(newRoots), time.Time{})

	if app.treeTopLine != 3 {
		t.Fatalf("expected refresh to fall back to approximate top index 3, got %d", app.treeTopLine)
	}
	if app.visibleRows[app.treeTopLine].Node.Issue.ID != "ab-e" {
		t.Fatalf("expected ab-e at fallback top index, got %s", app.visibleRows[app.treeTopLine].Node.Issue.ID)
	}
}

func TestWheelOverTreeScrollsCurrentFilteredViewModeRows(t *testing.T) {
	app := pointerTestApp()
	app.roots = mouseTreeNodes(
		"ab-match-a", "ab-match-b", "ab-closed", "ab-match-c", "ab-match-d",
		"ab-match-e", "ab-match-f", "ab-match-g", "ab-match-h", "ab-hidden",
	)
	app.roots[2].Issue.Title = "match hidden by active mode"
	app.roots[2].Issue.Status = "closed"
	app.roots[9].Issue.Title = "hidden by text filter"
	app.height = 10
	app.viewMode = ViewModeActive
	app.setFilterText("match")
	app.recalcVisibleRows()
	app.recalcViewportSize()

	for i := 0; i < 4; i++ {
		model, _ := app.Update(tea.MouseMsg{
			X:      2,
			Y:      2,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		})
		app = model.(*App)
	}

	wantMaxTop := len(app.visibleRows) - app.treePaneHeight()
	if app.treeTopLine != wantMaxTop {
		t.Fatalf("expected tree viewport to clamp to filtered max top %d, got %d", wantMaxTop, app.treeTopLine)
	}
	if app.visibleRows[app.treeTopLine].Node.Issue.ID != "ab-match-c" {
		t.Fatalf("expected scrolling to operate over filtered active rows, got %s", app.visibleRows[app.treeTopLine].Node.Issue.ID)
	}
}

func assertPointerTarget(t *testing.T, app *App, x, y int, want pointerTarget) {
	t.Helper()

	if got := app.pointerTargetAt(x, y); got != want {
		t.Fatalf("target at (%d,%d) = %v, want %v", x, y, got, want)
	}
}

func mouseTreeNodes(ids ...string) []*graph.Node {
	nodes := make([]*graph.Node, len(ids))
	for i, id := range ids {
		nodes[i] = &graph.Node{
			Issue: beads.FullIssue{
				ID:     id,
				Title:  "Tree row",
				Status: "open",
			},
		}
	}
	return nodes
}

func pointerTreeScrollTestApp(count int) *App {
	nodes := make([]*graph.Node, count)
	for i := 0; i < count; i++ {
		nodes[i] = &graph.Node{
			Issue: beads.FullIssue{
				ID:     "ab-tree-" + string(rune('a'+i)),
				Title:  "Tree row",
				Status: "open",
			},
		}
	}
	app := pointerTestApp()
	app.roots = nodes
	app.height = 10
	app.recalcVisibleRows()
	app.recalcViewportSize()
	return app
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
