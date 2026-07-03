package ui

import (
	"abacus/internal/graph"

	tea "github.com/charmbracelet/bubbletea"
)

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

func (m *App) handleMouseMsg(msg tea.MouseMsg) tea.Cmd {
	event, ok := m.pointerEventFromMouseMsg(msg)
	if !ok {
		return nil
	}

	if event.target == pointerTargetBackdrop {
		return m.handleBackdropPointerEvent(event)
	}
	if event.target == pointerTargetOverlay {
		return m.handleOverlayPointerEvent(event)
	}

	if event.target == pointerTargetDetails {
		m.handleDetailsPointerEvent(event)
		return nil
	}
	if event.target == pointerTargetTree {
		m.handleTreePointerEvent(event)
	}
	return nil
}

func (m *App) handleBackdropPointerEvent(event pointerEvent) tea.Cmd {
	if event.action != pointerActionPlainClick {
		return nil
	}
	if m.showHelp {
		m.showHelp = false
		return nil
	}
	cmd, _ := m.delegateToOverlay(tea.KeyMsg{Type: tea.KeyEsc})
	return cmd
}

func (m *App) handleOverlayPointerEvent(event pointerEvent) tea.Cmd {
	if event.action != pointerActionPlainClick {
		return nil
	}
	if m.activeOverlay == OverlayStatus && m.statusOverlay != nil {
		optionIndex, ok := m.overlayChoiceOptionIndexAtPointer(event, len(m.statusOverlay.options))
		if !ok {
			return nil
		}
		return m.statusOverlay.selectByValue(m.statusOverlay.options[optionIndex].value)
	}
	if m.activeOverlay == OverlayPriority && m.priorityOverlay != nil {
		optionIndex, ok := m.overlayChoiceOptionIndexAtPointer(event, len(m.priorityOverlay.options))
		if !ok {
			return nil
		}
		return m.priorityOverlay.selectByValue(m.priorityOverlay.options[optionIndex].value)
	}
	return nil
}

func (m *App) overlayChoiceOptionIndexAtPointer(event pointerEvent, optionCount int) (int, bool) {
	const optionStartY = 4

	surface := m.pointerLayout().overlaySurface
	relativeX := event.x - surface.x
	if relativeX <= 0 || relativeX >= surface.width-1 {
		return 0, false
	}
	relativeY := event.y - surface.y
	optionIndex := relativeY - optionStartY
	if optionIndex < 0 || optionIndex >= optionCount {
		return 0, false
	}
	return optionIndex, true
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
	case pointerActionPlainClick:
		m.selectTreeRowAtPointer(event)
	case pointerActionWheelUp:
		m.scrollTreeViewportBy(-1)
	case pointerActionWheelDown:
		m.scrollTreeViewportBy(1)
	}
}

func (m *App) selectTreeRowAtPointer(event pointerEvent) {
	rowIndex, ok := m.treeRowIndexAtPointer(event)
	if !ok {
		return
	}
	m.clearSelection()
	row := m.visibleRows[rowIndex]
	m.cursor = rowIndex
	m.focus = FocusTree
	m.treeMouseScrolled = false

	if m.treeExpansionHitAreaContains(event, row) && len(row.Node.Children) > 0 {
		if m.isNodeExpandedInView(row) {
			m.collapseNodeForView(row)
		} else {
			m.expandNodeForView(row)
		}
		m.recalcVisibleRows()
	}

	m.updateViewportContent()
}

func (m *App) treeRowIndexAtPointer(event pointerEvent) (int, bool) {
	treeBounds := m.pointerLayout().tree
	contentY := event.y - treeBounds.y - 1
	if contentY < 0 || contentY >= m.treePaneHeight() {
		return 0, false
	}
	rowIndex := m.treeTopLine + contentY
	if rowIndex < 0 || rowIndex >= len(m.visibleRows) {
		return 0, false
	}
	return rowIndex, true
}

func (m *App) treeExpansionHitAreaContains(event pointerEvent, row graph.TreeRow) bool {
	treeBounds := m.pointerLayout().tree
	contentX := event.x - treeBounds.x - 1
	if contentX < 0 {
		return false
	}
	return contentX < treeExpansionHitAreaWidth(row)
}

func treeExpansionHitAreaWidth(row graph.TreeRow) int {
	return 1 + row.Depth*2 + 2 + 1
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
	case m.activeOverlay == OverlayColumns && m.columnsOverlay != nil:
		layer = m.columnsOverlay.Layer(m.width, m.height, topMargin, bottomMargin)
	case m.showHelp:
		layer = newHelpOverlayLayer(m.keys, m.width, m.height, topMargin, bottomMargin)
	}
	return boundsFromLayer(layer)
}

func (m *App) toastSurfaceBounds(mainBodyStart, mainBodyHeight int) pointerBounds {
	toastFactories := []func(int, int, int, int) Layer{
		m.themeToastLayer,
		m.layoutToastLayer,
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
