package tv

import tui "github.com/hobbestherat/turbotui"

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
	MnemonicFG     tui.Color
	SelectionBG    tui.Color
	SelectionFG    tui.Color
}

var DefaultTheme = Theme{
	DesktopBG:      tui.ANSIColor(4),
	DesktopFG:      tui.ANSIColor(15),
	WindowBG:       tui.ANSIColor(4),
	WindowFG:       tui.ANSIColor(15),
	WindowBorderFG: tui.ANSIColor(15),
	WindowBorderBG: tui.ANSIColor(4),
	WindowTitleFG:  tui.ANSIColor(15),
	WindowTitleBG:  tui.ANSIColor(4),
	WindowShadow:   tui.ANSIColor(8),
	ButtonBG:       tui.ANSIColor(2),
	ButtonFG:       tui.ANSIColor(15),
	ButtonFocusBG:  tui.ANSIColor(6),
	ButtonFocusFG:  tui.ANSIColor(0),
	ButtonShadow:   tui.ANSIColor(8),
	DialogBG:       tui.ANSIColor(7),
	DialogFG:       tui.ANSIColor(0),
	DialogBorderFG: tui.ANSIColor(0),
	DialogBorderBG: tui.ANSIColor(7),
	InputBG:        tui.ANSIColor(0),
	InputFG:        tui.ANSIColor(15),
	InputFocusBG:   tui.ANSIColor(6),
	InputFocusFG:   tui.ANSIColor(0),
	CloseButtonBG:  tui.ANSIColor(1),
	CloseButtonFG:  tui.ANSIColor(15),
	MnemonicFG:     tui.ANSIColor(9),
	SelectionBG:    tui.ANSIColor(7),
	SelectionFG:    tui.ANSIColor(0),
}
