# Abacus

Abacus is a terminal UI for working with Beads projects. This glossary captures product language for the UI so interaction decisions use consistent terms.

## Language

**Keyboard Focus**:
The pane or overlay that receives keyboard navigation and commands.
_Avoid_: Active pane, selected pane

**Pointer Target**:
The UI area underneath the mouse pointer for a specific mouse event.
_Avoid_: Hover focus, mouse focus

**Pointer-Directed Scrolling**:
Mouse wheel scrolling that affects the pane under the pointer without changing keyboard focus.
_Avoid_: Focused scrolling, hover focus

**Details Pane**:
The read-only pane that shows information for the tree selection.
_Avoid_: Detail view, preview pane

**Pane Bounds**:
The rendered area occupied by a pane in the current layout.
_Avoid_: Hard-coded pane position, layout guess

**Plain Click**:
A mouse click interpreted as a direct selection or focus action, without extra behavior for double-click timing.
_Avoid_: Double-click command, hidden gesture

**Overlay Backdrop**:
The dimmed area outside an active overlay.
_Avoid_: Outside modal, background area

**Light Dismiss**:
Dismissing an overlay with a low-friction action such as pressing Escape or clicking the overlay backdrop.
_Avoid_: Outside click, click-away close

**Inert Click**:
A click inside an active surface that has no assigned action and does not fall through to lower layers.
_Avoid_: Dead click, ignored click

**Option Activation**:
Choosing an actionable option with the mouse exactly as if that option's keyboard shortcut had been pressed in the same context.
_Avoid_: Click applies, global option click

**Tree Row**:
A visible bead entry in the tree pane, including its indentation, status marker, bead ID, title, and optional columns.
_Avoid_: List item, tree item

**Tree Selection**:
The bead currently selected in the tree pane and represented in the detail pane.
_Avoid_: Current row, highlighted item

**Tree Viewport**:
The visible window onto the tree pane's rows, independent of which bead is selected.
_Avoid_: Tree selection, tree focus

**Off-Screen Selection**:
A tree selection that remains selected while the tree viewport is scrolled away from it.
_Avoid_: Lost selection, hidden focus

**Expansion Toggle**:
The tree row control that shows and changes whether a bead's children are visible.
_Avoid_: Disclosure marker, arrow, caret, expander

**Expansion Toggle Hit Area**:
The indentation, expansion toggle, and following separator space where a plain click selects the row and toggles child visibility.
_Avoid_: Exact glyph click, whole-row toggle
