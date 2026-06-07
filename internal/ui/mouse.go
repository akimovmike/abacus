package ui

import tea "github.com/charmbracelet/bubbletea"

type pointerAction int

const (
	pointerActionNone pointerAction = iota
	pointerActionPlainClick
	pointerActionWheelUp
	pointerActionWheelDown
)

type pointerTarget int

const (
	pointerTargetNone pointerTarget = iota
	pointerTargetTree
	pointerTargetDetails
	pointerTargetOverlay
	pointerTargetBackdrop
	pointerTargetInertChrome
)

type pointerEvent struct {
	action pointerAction
	target pointerTarget
	x      int
	y      int
}

type pointerBounds struct {
	x      int
	y      int
	width  int
	height int
}

func (b pointerBounds) contains(x, y int) bool {
	return !b.empty() &&
		x >= b.x &&
		y >= b.y &&
		x < b.x+b.width &&
		y < b.y+b.height
}

func (b pointerBounds) empty() bool {
	return b.width <= 0 || b.height <= 0
}

type pointerLayout struct {
	tree           pointerBounds
	details        pointerBounds
	header         pointerBounds
	footer         pointerBounds
	toast          pointerBounds
	overlaySurface pointerBounds
	overlayActive  bool
}

func (m *App) handleMouseMsg(msg tea.MouseMsg) {
	event, ok := m.pointerEventFromMouseMsg(msg)
	if !ok {
		return
	}

	if event.target == pointerTargetDetails {
		m.handleDetailsPointerEvent(event)
		return
	}
	if event.target == pointerTargetTree {
		m.handleTreePointerEvent(event)
	}
}

func (m *App) handleDetailsPointerEvent(event pointerEvent) {
	switch event.action {
	case pointerActionPlainClick:
		m.focus = FocusDetails
	case pointerActionWheelUp:
		m.viewport.ScrollUp(1)
	case pointerActionWheelDown:
		m.viewport.ScrollDown(1)
	}
}

func (m *App) handleTreePointerEvent(event pointerEvent) {
	switch event.action {
	case pointerActionWheelUp:
		m.scrollTreeViewportBy(-1)
	case pointerActionWheelDown:
		m.scrollTreeViewportBy(1)
	}
}

func (m *App) scrollTreeViewportBy(delta int) {
	maxTop := len(m.visibleRows) - m.treePaneHeight()
	if maxTop < 0 {
		maxTop = 0
	}
	m.treeTopLine += delta
	if m.treeTopLine < 0 {
		m.treeTopLine = 0
	} else if m.treeTopLine > maxTop {
		m.treeTopLine = maxTop
	}
	m.treeMouseScrolled = true
}

func (m *App) pointerEventFromMouseMsg(msg tea.MouseMsg) (pointerEvent, bool) {
	if msg.Action != tea.MouseActionPress {
		return pointerEvent{}, false
	}

	var action pointerAction
	switch msg.Button {
	case tea.MouseButtonLeft:
		action = pointerActionPlainClick
	case tea.MouseButtonWheelUp:
		action = pointerActionWheelUp
	case tea.MouseButtonWheelDown:
		action = pointerActionWheelDown
	default:
		return pointerEvent{}, false
	}

	return pointerEvent{
		action: action,
		target: m.pointerTargetAt(msg.X, msg.Y),
		x:      msg.X,
		y:      msg.Y,
	}, true
}

func (m *App) pointerTargetAt(x, y int) pointerTarget {
	return m.pointerLayout().targetAt(x, y)
}

func (l pointerLayout) targetAt(x, y int) pointerTarget {
	if l.overlayActive {
		if l.overlaySurface.contains(x, y) {
			return pointerTargetOverlay
		}
		return pointerTargetBackdrop
	}
	if l.toast.contains(x, y) ||
		l.header.contains(x, y) ||
		l.footer.contains(x, y) {
		return pointerTargetInertChrome
	}
	if l.tree.contains(x, y) {
		return pointerTargetTree
	}
	if l.details.contains(x, y) {
		return pointerTargetDetails
	}
	return pointerTargetInertChrome
}

func (m *App) pointerLayout() pointerLayout {
	if m.width <= 0 || m.height <= 0 {
		return pointerLayout{}
	}

	const headerHeight = 1
	const bottomMargin = 1

	layout := pointerLayout{
		header: pointerBounds{x: 0, y: 0, width: m.width, height: headerHeight},
		footer: pointerBounds{
			x:      0,
			y:      maxInt(0, m.height-bottomMargin),
			width:  m.width,
			height: bottomMargin,
		},
	}

	m.assignPaneBounds(&layout, headerHeight)
	mainBodyHeight := maxInt(0, layout.footer.y-headerHeight)
	layout.toast = m.toastSurfaceBounds(headerHeight, mainBodyHeight)
	layout.overlayActive = m.activeOverlay != OverlayNone || m.showHelp
	if layout.overlayActive {
		layout.overlaySurface = m.overlaySurfaceBounds(headerHeight, bottomMargin)
	}
	return layout
}

func (m *App) assignPaneBounds(layout *pointerLayout, y int) {
	listHeight := clampDimension(m.height-4, minListHeight, m.height-2)
	paneHeight := listHeight + 2

	if !m.ShowDetails {
		layout.tree = pointerBounds{x: 0, y: y, width: m.width, height: paneHeight}
		return
	}

	if m.layout == LayoutTall {
		treeH, detailH := m.tallLayoutSplit()
		treeHeight := treeH + 2
		layout.tree = pointerBounds{x: 0, y: y, width: m.width, height: treeHeight}
		layout.details = pointerBounds{x: 0, y: y + treeHeight, width: m.width, height: detailH + 2}
		return
	}

	leftWidth := m.width - m.viewport.Width - 4
	if leftWidth < 1 {
		leftWidth = 1
	}
	rightWidth := m.viewport.Width
	if rightWidth < 1 {
		rightWidth = 1
	}
	treeWidth := leftWidth + 2
	layout.tree = pointerBounds{x: 0, y: y, width: treeWidth, height: paneHeight}
	layout.details = pointerBounds{x: treeWidth, y: y, width: rightWidth + 2, height: paneHeight}
}

func (m *App) overlaySurfaceBounds(topMargin, bottomMargin int) pointerBounds {
	var layer Layer
	switch {
	case m.activeOverlay == OverlayStatus && m.statusOverlay != nil:
		layer = m.statusOverlay.Layer(m.width, m.height, topMargin, bottomMargin)
	case m.activeOverlay == OverlayLabels && m.labelsOverlay != nil:
		layer = m.labelsOverlay.Layer(m.width, m.height, topMargin, bottomMargin)
	case m.activeOverlay == OverlayCreate && m.createOverlay != nil:
		layer = m.createOverlay.Layer(m.width, m.height, topMargin, bottomMargin)
	case m.activeOverlay == OverlayDelete && m.deleteOverlay != nil:
		layer = m.deleteOverlay.Layer(m.width, m.height, topMargin, bottomMargin)
	case m.activeOverlay == OverlayComment && m.commentOverlay != nil:
		layer = m.commentOverlay.Layer(m.width, m.height, topMargin, bottomMargin)
	case m.activeOverlay == OverlayPriority && m.priorityOverlay != nil:
		layer = m.priorityOverlay.Layer(m.width, m.height, topMargin, bottomMargin)
	case m.showHelp:
		layer = newHelpOverlayLayer(m.keys, m.width, m.height, topMargin, bottomMargin)
	}
	return boundsFromLayer(layer)
}

func (m *App) toastSurfaceBounds(mainBodyStart, mainBodyHeight int) pointerBounds {
	toastFactories := []func(int, int, int, int) Layer{
		m.themeToastLayer,
		m.layoutToastLayer,
		m.columnsToastLayer,
		m.updateSuccessToastLayer,
		m.updateFailureToastLayer,
		m.updateToastLayer,
		m.deleteToastLayer,
		m.createToastLayer,
		m.commentToastLayer,
		m.priorityToastLayer,
		m.newAssigneeToastLayer,
		m.newLabelToastLayer,
		m.labelsToastLayer,
		m.statusToastLayer,
		m.copyToastLayer,
		m.errorToastLayer,
	}

	for _, factory := range toastFactories {
		if bounds := boundsFromLayer(factory(m.width, m.height, mainBodyStart, mainBodyHeight)); !bounds.empty() {
			return bounds
		}
	}
	return pointerBounds{}
}

func boundsFromLayer(layer Layer) pointerBounds {
	if layer == nil {
		return pointerBounds{}
	}
	canvas := layer.Render()
	if canvas == nil {
		return pointerBounds{}
	}
	x, y := canvas.Offset()
	return pointerBounds{x: x, y: y, width: canvas.Width(), height: canvas.Height()}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
