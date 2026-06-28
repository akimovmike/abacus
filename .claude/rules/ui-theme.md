---
paths:
  - internal/ui/theme/*.go
---

# Theme conventions

- Implement every Theme method (Primary, Secondary, Accent, Error/Warning/Success/Info, Text*, Background*, Border*) returning `lipgloss.AdaptiveColor` with Light + Dark set.
- Register in `init()` via `RegisterTheme(name, T{})`.
- Add the name to `themes_test.go` expectations; keep all colors non-empty.
