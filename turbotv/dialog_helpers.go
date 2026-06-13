package tv

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
	yes := NewButton("Yes", Rect{X: 12, Y: 4, W: 10, H: 1}, func() {
		closeDialog(true)
	})
	no := NewButton("No", Rect{X: 28, Y: 4, W: 10, H: 1}, func() {
		closeDialog(false)
	})
	dialog.Window.AddContent(yes)
	dialog.Window.AddContent(no)

	layer = NewModalLayer("confirm-dialog", dialog)
	desktop.AddLayer(layer)
	return layer
}
