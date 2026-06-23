package tv

import tui "github.com/hobbestherat/turbotui"

// ShowConfirmYesNo pushes a modal Yes/No confirmation dialog and returns its
// layer. The dialog is sized to its message instead of a fixed magic width
// (issues #7, #48): the width fits the longer of the wrapped message and the
// button row, the height fits the wrapped line count, and both are capped to the
// screen. The message label word-wraps across its rows, the Yes/No buttons are
// laid out by NewButtonRow from their label widths, and the buttons carry &Yes /
// &No mnemonics with Enter→Yes (default) / Escape→No (cancel) semantics (#63).
func ShowConfirmYesNo(desktop *Desktop, title string, message string, onResult func(bool)) *Layer {
	if desktop == nil {
		return nil
	}
	screenW := desktop.App().Width()
	screenH := desktop.App().Height()

	const (
		yesLabel = "&Yes"
		noLabel  = "&No"
		hPad     = 1 // blank columns between the dialog border and its content
	)
	rowWidth := buttonLabelWidth(yesLabel) + DefaultButtonGap + buttonLabelWidth(noLabel)

	// Width: fit the wider of the message and the button row, plus border + pad,
	// then make room for the title bar, all clamped to the screen.
	textWidth := longestLineWidth(message)
	if rowWidth > textWidth {
		textWidth = rowWidth
	}
	maxText := screenW - 2 - 2*hPad
	if maxText < 1 {
		maxText = 1
	}
	if textWidth > maxText {
		textWidth = maxText
	}
	width := textWidth + 2 + 2*hPad
	if titleWidth := tui.StringWidth(title) + 6; width < titleWidth {
		width = titleWidth
	}
	if width < 16 {
		width = 16
	}
	if width > screenW {
		width = screenW
	}

	// Height: one row per wrapped message line, a blank row, and the button row,
	// inside the border. WrapLabelRunes mirrors how the Label itself wraps.
	lineCount := len(WrapLabelRunes([]rune(message), width-2-2*hPad))
	if lineCount < 1 {
		lineCount = 1
	}
	height := lineCount + 4 // border(2) + blank(1) + button row(1)
	if height < 7 {
		height = 7
	}
	if height > screenH {
		height = screenH
	}

	x := (screenW - width) / 2
	y := (screenH - height) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	dialog := NewDialog(title, x, y, width, height)
	dialog.Window.ShowClose = false

	contentW := width - 2
	contentH := height - 2
	buttonRowY := contentH - 1
	if buttonRowY < 1 {
		buttonRowY = contentH
	}
	labelHeight := buttonRowY - 1
	if labelHeight < 1 {
		labelHeight = 1
	}
	label := NewLabel(message, Rect{X: hPad, Y: 0, W: contentW - 2*hPad, H: labelHeight})
	label.FG = activeTheme.DialogFG
	label.BG = activeTheme.DialogBG
	label.HotFG = activeTheme.DialogMnemonicFG
	dialog.Window.AddContent(label)

	var layer *Layer
	closeDialog := func(value bool) {
		desktop.RemoveLayer(layer)
		if onResult != nil {
			onResult(value)
		}
	}
	yes := NewButton(yesLabel, Rect{}, func() { closeDialog(true) })
	yes.Default = true
	no := NewButton(noLabel, Rect{}, func() { closeDialog(false) })
	no.Cancel = true
	row := NewButtonRow(buttonRowY, contentW, AlignCenter, DefaultButtonGap, yes, no)
	dialog.Window.AddContent(row)

	// Enter activates the default (Yes), Escape the cancel (No); a focused button
	// still handles its own Enter/Space.
	dialog.SetDefaultCancelButtons(yes, no)

	layer = NewModalLayer("confirm-dialog", dialog)
	// A confirm dialog is user-initiated — it appears in direct response to a user
	// action and cannot interrupt mid-keystroke — so it opts out of the modal
	// Enter-grace (gogent#347, which targets background-triggered modals): Enter on
	// the focused button works immediately.
	layer.NoEnterGrace = true
	desktop.AddLayer(layer)
	desktop.SetFocus(no)
	return layer
}
