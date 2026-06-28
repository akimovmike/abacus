---
name: ui-theme
description: Adds a new color theme to the Abacus TUI in internal/ui/theme/ by implementing the 16-method Theme interface (Primary..BorderDim) as an empty struct, registering it via init()/RegisterTheme, and updating themes_test.go. Use when the user says 'add a theme', 'new color scheme', or names a palette (e.g. 'add Gruvbox', 'add Catppuccin Frappe'). Do NOT use for restyling existing components, changing a single component's color, or editing the theme manager/interface itself.
paths:
  - internal/ui/theme/**
---
# Add a UI Color Theme

Themes are defined one-per-file in `internal/ui/theme/`. Each is an empty struct implementing the 16-method `Theme` interface, self-registering via an `init()` call. New themes need a new file plus updates to `themes_test.go` — nothing else.

## Critical

- **One file per theme**: `internal/ui/theme/<name>.go`, `package theme`. `<name>` is lowercase (e.g. `gruvbox.go`, `tokyonight.go`).
- **Implement ALL 16 methods** of the `Theme` interface in `internal/ui/theme/theme.go`. Missing even one is a compile error (`does not implement Theme`). The 16, in order: `Primary, Secondary, Accent, Error, Warning, Success, Info, Text, TextMuted, TextEmphasized, Background, BackgroundSecondary, BackgroundDarker, BorderNormal, BorderFocused, BorderDim`.
- **Every method returns** `lipgloss.AdaptiveColor{Dark: "#rrggbb", Light: "#rrggbb"}`. Both hex values MUST be exactly 7 chars: `#` + 6 lowercase hex digits. `TestDimmedThemeColorsValid` rejects any other format. Always provide BOTH `Dark` and `Light` (it powers light/dark terminal adaptation).
- **Self-register** in `init()` with `RegisterTheme("<name>", <Name>Theme{})`. The registered key is lowercase `<name>`; the struct is PascalCase `<Name>Theme`.
- **DO NOT** touch `manager.go` (registry/cycling), `theme.go` (interface), or the `dimmedTheme` wrapper. The interface and dimming are auto-derived from your 16 colors. Adding a theme = new `<name>.go` + `themes_test.go` edits only.

## Instructions

1. **Map the palette to the 16 semantic roles.** Read `internal/ui/theme/gruvbox.go` as the canonical reference (it is the smallest, cleanest example). Match the palette's hues to roles using the doc comments in `theme.go`: `Primary` = main accent (focused borders/header bg), `Error`/`Warning`/`Success`/`Info` = status, `Text`/`TextMuted`/`TextEmphasized` = text, `Background`/`BackgroundSecondary`/`BackgroundDarker` = surfaces, `BorderNormal`/`BorderFocused`/`BorderDim` = borders. **Verify before proceeding:** you have a `Dark` AND `Light` 7-char hex for all 16 roles.

2. **Create `internal/ui/theme/<name>.go`** by copying this exact skeleton and substituting the struct name, register key, and hex values from Step 1. Keep every method a single-line return (matches the codebase; well under the 60-line function / 500-line file limits):

   ```go
   package theme

   import "github.com/charmbracelet/lipgloss"

   // <Name>Theme implements the <Name> color scheme.
   type <Name>Theme struct{}

   func (t <Name>Theme) Primary() lipgloss.AdaptiveColor   { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) Secondary() lipgloss.AdaptiveColor { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) Accent() lipgloss.AdaptiveColor    { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) Error() lipgloss.AdaptiveColor     { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) Warning() lipgloss.AdaptiveColor   { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) Success() lipgloss.AdaptiveColor   { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) Info() lipgloss.AdaptiveColor      { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) Text() lipgloss.AdaptiveColor      { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) TextMuted() lipgloss.AdaptiveColor { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) TextEmphasized() lipgloss.AdaptiveColor { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) Background() lipgloss.AdaptiveColor          { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) BackgroundSecondary() lipgloss.AdaptiveColor { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) BackgroundDarker() lipgloss.AdaptiveColor    { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) BorderNormal() lipgloss.AdaptiveColor  { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) BorderFocused() lipgloss.AdaptiveColor { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }
   func (t <Name>Theme) BorderDim() lipgloss.AdaptiveColor     { return lipgloss.AdaptiveColor{Dark: "#......", Light: "#......"} }

   func init() {
   	RegisterTheme("<name>", <Name>Theme{})
   }
   ```

   **Verify before proceeding:** run `make build`. It MUST compile. A compile error here means a missing/misspelled method or a struct-name mismatch.

3. **Update `internal/ui/theme/themes_test.go`** (uses output of Step 2). Two edits:
   - In `TestAllThemesRegistered`, add `"<name>",` to the `expected` slice, keeping it **alphabetical** (the existing list is sorted).
   - Bump the theme-count thresholds. There are three `23` literals (in `TestThemeCount`, `TestCycleTheme`, `TestCyclePreviousTheme`, all comparing `< 23` / `< 23`). Increase each to the new total count. After adding one theme to the current 23, change all three to `24` (and update the `at least 23` comments).

   **Verify before proceeding:** run `make test`. `TestAllThemesRegistered`, `TestThemeCount`, `TestThemeColorsNotEmpty`, and `TestDimmedThemeColorsValid` MUST pass — they iterate every registered theme, so they exercise your new one automatically.

4. **Run the full gate**: `make check-test` (lint + unit tests). Fix any `gofmt`/`golangci-lint` findings (alignment of the single-line returns, unused imports). This is required before the work is done.

5. **Visually confirm in the TUI** (themes are user-facing). Build, launch, and cycle to your theme:
   ```bash
   make build
   ./scripts/tui-test.sh start
   ./scripts/tui-test.sh view   # capture initial state
   ```
   Cycle themes with the in-app theme-cycle key (themes cycle in sorted order via `CycleTheme`), capture `view` again on your theme, and confirm borders/text/backgrounds are legible. `./scripts/tui-test.sh quit` when done.

## Examples

**User says:** "Add a Gruvbox theme."

**Actions taken:**
1. Mapped the Gruvbox palette to the 16 roles (e.g. `Primary` = `#83a598`/`#076678`, `Background` = `#282828`/`#fbf1c7`, `Error` = `#fb4934`/`#9d0006`).
2. Created `internal/ui/theme/gruvbox.go` with `type GruvboxTheme struct{}`, 16 single-line methods, and `func init() { RegisterTheme("gruvbox", GruvboxTheme{}) }`.
3. Added `"gruvbox",` to the `expected` slice in `themes_test.go` (alphabetically between `github` and `kanagawa`) and bumped the `23` count literals to `24`.
4. Ran `make check-test` — green. Ran `./scripts/tui-test.sh` and confirmed Gruvbox renders correctly.

**Result:** `gruvbox` shows up in `theme.Available()` (sorted), is cycle-reachable, and passes all theme tests — identical in structure to the other 22 themes.

## Common Issues

- **`cannot use <Name>Theme{} (...) as Theme value ... missing method BorderDim`** (or any method name) on `make build`: your struct is missing a method or a method has the wrong signature. Every method must be `func (t <Name>Theme) X() lipgloss.AdaptiveColor`. Cross-check against all 16 names in `theme.go`.
- **`TestAllThemesRegistered: expected theme "<name>" to be registered, but it was not found`**: the `init()` `RegisterTheme` key does not match the string you added to `expected`, OR you forgot the `init()` entirely. Both must use the identical lowercase `<name>`.
- **`TestThemeCount: expected at least 23 themes, got 23`** (or cycle tests seeing too few): you added the theme but didn't bump the `23` literals. Raise all three to the new total.
- **`TestDimmedThemeColorsValid: theme "<name>" dimmed: Primary has invalid hex "..."`**: a base hex isn't 7 chars `#`+6. Fix shorthand (`#fff`), uppercase, or missing `#`. The dimming math derives from your hex, so malformed input fails here even though the theme builds.
- **`TestThemeColorsNotEmpty: theme "<name>": X has empty Dark and Light values`**: you left a method returning a zero-value `AdaptiveColor{}`. Fill in both hex strings.
- **`golangci-lint` gofmt/alignment failure in `make check`**: the single-line method returns must be gofmt-aligned. Run `gofmt -w internal/ui/theme/<name>.go` (or `make check` reports the diff) and re-run.
- **`imported and not used: "github.com/charmbracelet/lipgloss"`**: only happens if you removed all method bodies; the skeleton uses it in every return, so keep the import.