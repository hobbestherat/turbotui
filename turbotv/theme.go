package tv

import tui "github.com/hobbestherat/turbotui"

// Theme is the palette every widget seeds its colours from. Construct a copy of
// DefaultTheme (or HighContrastTheme), override the fields you want, then call
// SetTheme (or desktop.SetTheme) before building the UI so newly created widgets
// pick it up. Chrome that resolves at draw time (desktop background, menus,
// dropdown popups, selections) reflects a swap immediately.
type Theme struct {
	DesktopBG      tui.Color
	DesktopFG      tui.Color
	WindowBG       tui.Color
	WindowFG       tui.Color
	WindowBorderFG tui.Color
	WindowBorderBG tui.Color
	WindowTitleFG  tui.Color
	WindowTitleBG  tui.Color
	WindowShadow   tui.Color
	ButtonBG       tui.Color
	ButtonFG       tui.Color
	ButtonFocusBG  tui.Color
	ButtonFocusFG  tui.Color
	ButtonShadow   tui.Color
	DialogBG       tui.Color
	DialogFG       tui.Color
	DialogBorderFG tui.Color
	DialogBorderBG tui.Color
	InputBG        tui.Color
	InputFG        tui.Color
	InputFocusBG   tui.Color
	InputFocusFG   tui.Color
	CloseButtonBG  tui.Color
	CloseButtonFG  tui.Color
	// MnemonicFG is the hot-key highlight on window/desktop backgrounds;
	// DialogMnemonicFG is the equivalent on the (light) dialog background, kept
	// separate so neither lands as a low-contrast pair.
	MnemonicFG       tui.Color
	DialogMnemonicFG tui.Color
	SelectionBG      tui.Color
	SelectionFG      tui.Color
	// Menu chrome. The menubar is always-on-top so it gets first-class fields
	// rather than literal constants.
	MenuBarFG    tui.Color
	MenuBarBG    tui.Color
	MenuHotFG    tui.Color
	MenuHotBG    tui.Color
	MenuSelectFG tui.Color
	MenuSelectBG tui.Color
	MenuShadow   tui.Color
}

// DefaultTheme is the classic blue palette. Contrast notes: the hot-key colour
// is yellow on the blue window background (was bright red, a poor pair on blue
// and for red/green colour blindness) and dark red on the light dialog
// background, so it never renders as yellow-on-light-grey. Every default pair is
// chosen to keep a clear luminance difference.
var DefaultTheme = Theme{
	DesktopBG:        tui.ANSIColor(4),
	DesktopFG:        tui.ANSIColor(15),
	WindowBG:         tui.ANSIColor(4),
	WindowFG:         tui.ANSIColor(15),
	WindowBorderFG:   tui.ANSIColor(15),
	WindowBorderBG:   tui.ANSIColor(4),
	WindowTitleFG:    tui.ANSIColor(15),
	WindowTitleBG:    tui.ANSIColor(4),
	WindowShadow:     tui.ANSIColor(8),
	ButtonBG:         tui.ANSIColor(2),
	ButtonFG:         tui.ANSIColor(15),
	ButtonFocusBG:    tui.ANSIColor(6),
	ButtonFocusFG:    tui.ANSIColor(0),
	ButtonShadow:     tui.ANSIColor(8),
	DialogBG:         tui.ANSIColor(7),
	DialogFG:         tui.ANSIColor(0),
	DialogBorderFG:   tui.ANSIColor(0),
	DialogBorderBG:   tui.ANSIColor(7),
	InputBG:          tui.ANSIColor(0),
	InputFG:          tui.ANSIColor(15),
	InputFocusBG:     tui.ANSIColor(6),
	InputFocusFG:     tui.ANSIColor(0),
	CloseButtonBG:    tui.ANSIColor(1),
	CloseButtonFG:    tui.ANSIColor(15),
	MnemonicFG:       tui.ANSIColor(11), // bright yellow: high contrast on blue
	DialogMnemonicFG: tui.ANSIColor(1),  // dark red: high contrast on light grey
	SelectionBG:      tui.ANSIColor(7),
	SelectionFG:      tui.ANSIColor(0),
	MenuBarFG:        tui.ANSIColor(0),
	MenuBarBG:        tui.ANSIColor(7),
	MenuHotFG:        tui.ANSIColor(1), // dark red on light grey
	MenuHotBG:        tui.ANSIColor(7),
	MenuSelectFG:     tui.ANSIColor(15),
	MenuSelectBG:     tui.ANSIColor(4),
	MenuShadow:       tui.ANSIColor(8),
}

// HighContrastTheme is a black/white, colour-blind-safe preset: chrome is pure
// black-on-white (or its inverse), and the only accent is bright yellow — the
// luminance gap, not hue, carries the meaning, so it survives any colour-vision
// deficiency (cf. the Okabe–Ito guidance, https://jfly.uni-koeln.de/color/).
var HighContrastTheme = Theme{
	DesktopBG:        tui.ANSIColor(0),
	DesktopFG:        tui.ANSIColor(15),
	WindowBG:         tui.ANSIColor(0),
	WindowFG:         tui.ANSIColor(15),
	WindowBorderFG:   tui.ANSIColor(15),
	WindowBorderBG:   tui.ANSIColor(0),
	WindowTitleFG:    tui.ANSIColor(0),
	WindowTitleBG:    tui.ANSIColor(15),
	WindowShadow:     tui.ANSIColor(0),
	ButtonBG:         tui.ANSIColor(15),
	ButtonFG:         tui.ANSIColor(0),
	ButtonFocusBG:    tui.ANSIColor(11),
	ButtonFocusFG:    tui.ANSIColor(0),
	ButtonShadow:     tui.ANSIColor(0),
	DialogBG:         tui.ANSIColor(0),
	DialogFG:         tui.ANSIColor(15),
	DialogBorderFG:   tui.ANSIColor(15),
	DialogBorderBG:   tui.ANSIColor(0),
	InputBG:          tui.ANSIColor(0),
	InputFG:          tui.ANSIColor(15),
	InputFocusBG:     tui.ANSIColor(15),
	InputFocusFG:     tui.ANSIColor(0),
	CloseButtonBG:    tui.ANSIColor(0),
	CloseButtonFG:    tui.ANSIColor(11),
	MnemonicFG:       tui.ANSIColor(11),
	DialogMnemonicFG: tui.ANSIColor(11),
	SelectionBG:      tui.ANSIColor(15),
	SelectionFG:      tui.ANSIColor(0),
	MenuBarFG:        tui.ANSIColor(15),
	MenuBarBG:        tui.ANSIColor(0),
	MenuHotFG:        tui.ANSIColor(11),
	MenuHotBG:        tui.ANSIColor(0),
	MenuSelectFG:     tui.ANSIColor(0),
	MenuSelectBG:     tui.ANSIColor(15),
	MenuShadow:       tui.ANSIColor(0),
}

// activeTheme is the palette widgets seed from at construction and that
// draw-time chrome resolves against. SetTheme replaces it.
var activeTheme = DefaultTheme

// SetTheme makes t the active theme. Widgets created afterwards seed their
// colours from it, and chrome resolved at draw time (desktop background, menus,
// popups, selections) reflects it on the next redraw. Call it before building
// the UI for a full re-skin.
func SetTheme(t Theme) {
	activeTheme = t
}

// ActiveTheme returns the active theme.
func ActiveTheme() Theme {
	return activeTheme
}
