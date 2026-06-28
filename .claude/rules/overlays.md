---
paths:
  - internal/ui/overlay_*.go
---

# Overlay conventions

- Each overlay gets its own `overlay_<name>.go` + `_test.go`; struct exposes `Init`/`Update`/`View`/`Layer(width,height,topMargin,bottomMargin)`.
- Emit `<Name>ChangedMsg` / `<Name>CancelledMsg`; never mutate `App` fields directly.
- Register in the `OverlayType` enum (`app.go`) and wire open/route in `update_keys.go` + `update_overlay.go`.
- Build chrome via `NewOverlayBuilder` (`overlay_base.go`); reuse `footerHint`s; staged Esc clears input before closing.
- Draw through Surface/Canvas (`surface.go`, `cell_canvas.go`), not manual background fills. See `docs/UI_PRINCIPLES.md`.
