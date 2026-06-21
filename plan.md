# Plan: gate the Select popup drop shadow (gogent #231)

## Problem
gogent's "Disable shadows" (#215) toggles `Shadow bool` on Window, Button and
MenuBar, but turbotui's `Select` draws its dropdown-popup shadow
unconditionally in `drawPopup` (`turbotv/widget_select.go:285`) and has no
`Shadow` field, so the dropdown list always shows a shadow.

## Fix (match Window/Button/MenuBar exactly)
1. Add a public `Shadow bool` field to the `Select` struct, documented.
2. Default it to `true` in `NewSelect` so existing behaviour is preserved.
3. In `drawPopup`, gate the `surface.DrawShadow(...)` call behind `if s.Shadow`.

This lets gogent add `applySelectShadow(sel){ sel.Shadow = shadowsEnabled }`,
mirroring `applyWindowShadow` / `applyButtonShadow` / `applyMenuBarShadow`.

## Constraints
- No new deps. gofmt. Match package style.
- Keep the popup's shadow colour/style as-is (`activeTheme.WindowShadow`,
  `DefaultShadowStyle`) — only the visibility is now conditional.

## Test surface (GLM writes tests)
- `Select.Shadow` defaults to `true` after `NewSelect`.
- Opened popup with `Shadow == true` draws a shadow.
- Opened popup with `Shadow == false` draws no shadow.
