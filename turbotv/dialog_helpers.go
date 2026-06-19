package tv

import tui "github.com/hobbestherat/turbotui"

func ShowConfirmYesNo(desktop *Desktop, title string, message string, onResult func(bool)) *Layer {
	if desktop == nil {
		return nil
	}
	const width = 54
	const height = 10
	x := (desktop.App().Width() - width) / 2
	y := (desktop.App().Height() - height) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	dialog := NewDialog(title, x, y, width, height)
	dialog.Window.ShowClose = false

	label := NewLabel(message, Rect{X: 2, Y: 1, W: width - 4, H: 2})
	label.FG = DefaultTheme.DialogFG
	label.BG = DefaultTheme.DialogBG
	dialog.Window.AddContent(label)

	var layer *Layer
	closeDialog := func(value bool) {
		desktop.RemoveLayer(layer)
		if onResult != nil {
			onResult(value)
		}
	}
	// Lay the buttons out in an HBox centred in the dialog, so their positions
	// adapt to the dialog width instead of being hard-coded column numbers.
	const buttonWidth = 10
	buttonSpacing := 4
	rowWidth := buttonWidth*2 + buttonSpacing
	rowX := (width - 2 - rowWidth) / 2
	if rowX < 0 {
		rowX = 0
	}
	buttons := NewHBox(Rect{X: rowX, Y: 4, W: rowWidth, H: 1})
	buttons.Spacing = buttonSpacing
	yes := NewButton("Yes", Rect{X: 0, Y: 0, W: buttonWidth, H: 1}, func() {
		closeDialog(true)
	})
	no := NewButton("No", Rect{X: 0, Y: 0, W: buttonWidth, H: 1}, func() {
		closeDialog(false)
	})
	buttons.Add(yes)
	buttons.Add(no)
	dialog.Window.AddContent(buttons)

	// Escape anywhere in the dialog cancels it (counts as "No").
	dialog.Root().OnTypeFn = func(_ *VisualComponent, event tui.TypeEvent) bool {
		if event.Key == tui.KeyEscape {
			closeDialog(false)
			return true
		}
		return false
	}

	layer = NewModalLayer("confirm-dialog", dialog)
	desktop.AddLayer(layer)
	desktop.SetFocus(no)
	return layer
}
