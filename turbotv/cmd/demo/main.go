package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tui "github.com/hobbestherat/turbotui"
	tv "github.com/hobbestherat/turbotui/turbotv"
)

func main() {
	startTime := time.Now()
	app, err := tui.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize TUI: %v\n", err)
		os.Exit(1)
	}
	desktop := tv.NewDesktop(app)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var cityBox *tv.TextBox

	root := tv.NewComponent(tv.Rect{X: 0, Y: 0, W: app.Width(), H: app.Height()})
	root.DrawFn = func(component *tv.VisualComponent, surface tv.Surface) {
		bounds := component.AbsoluteBounds()
		surface.DrawBox(tv.Rect{X: 0, Y: 1, W: bounds.W, H: bounds.H - 1}, tui.LineSingle, tui.ANSIColor(15), tui.ANSIColor(4))
	}
	menu := tv.NewMenuBar(tv.Rect{X: 0, Y: 0, W: app.Width(), H: 1},
		tv.NewSubMenu("&File",
			tv.NewMenuItem("&Confirm", func() {
				tv.ShowConfirmYesNo(desktop, "Confirm", "Apply values?", func(value bool) {
					if value {
						cityBox.SetText("Saved")
					} else {
						cityBox.SetText("Canceled")
					}
				})
			}).WithShortcut("Ctrl+S", tui.KeyRune, 's', true),
			tv.NewMenuItem("E&xit", func() {
				stop()
			}).WithShortcut("Ctrl+Q", tui.KeyRune, 'q', true),
		),
		tv.NewSubMenu("&Help",
			tv.NewMenuItem("&About", func() {
				tv.ShowConfirmYesNo(desktop, "TurboTV", "TurboTV menu demo running.", nil)
			}),
		),
	)
	desktop.SetMenuBar(menu)
	desktop.AddLayer(tv.NewFullscreenLayer("base", root))

	window := tv.NewWindow("Main Window", tv.Rect{X: 6, Y: 3, W: 80, H: 22}, tui.LineDouble)
	window.OnClose = func(_ *tv.Window) {
		desktop.RemoveTopLayer()
	}
	helpLabel := tv.NewLabel("Type in the fields, or press Alt+<highlighted letter>. Scroll the notes with the wheel.", tv.Rect{X: 1, Y: 0, W: 74, H: 1})
	window.AddContent(helpLabel)

	nameLabel := tv.NewLabel("&Name", tv.Rect{X: 2, Y: 2, W: 12, H: 1})
	cityLabel := tv.NewLabel("&City", tv.Rect{X: 30, Y: 2, W: 12, H: 1})
	notesLabel := tv.NewLabel("Note&s", tv.Rect{X: 56, Y: 2, W: 12, H: 1})
	multiLabel := tv.NewLabel("&Multiline:", tv.Rect{X: 2, Y: 4, W: 16, H: 1})
	nameBox := tv.NewTextBox("Name", tv.Rect{X: 2, Y: 3, W: 24, H: 1})
	cityBox = tv.NewTextBox("City", tv.Rect{X: 30, Y: 3, W: 24, H: 1})
	multi := tv.NewMultiLineInput("Line one\nLine two", tv.Rect{X: 2, Y: 5, W: 52, H: 6})
	view := tv.NewTextView("Scrollable notes:\n- Turbo style colors\n- Layered components\n- Focus manager\n- Mnemonic shortcuts\n- Window shadow\n- Dialog helper\n- Use mouse wheel here\n- More lines...\n- End", tv.Rect{X: 56, Y: 3, W: 20, H: 10})
	nameLabel.SetTarget(nameBox)
	cityLabel.SetTarget(cityBox)
	notesLabel.SetTarget(view)
	multiLabel.SetTarget(multi)
	window.AddContent(nameLabel)
	window.AddContent(cityLabel)
	window.AddContent(notesLabel)
	window.AddContent(multiLabel)
	window.AddContent(nameBox)
	window.AddContent(cityBox)
	window.AddContent(multi)
	window.AddContent(view)

	confirm := tv.NewButton("C&onfirm", tv.Rect{X: 2, Y: 12, W: 14, H: 1}, func() {
		tv.ShowConfirmYesNo(desktop, "Confirm", "Apply values?", func(value bool) {
			if value {
				cityBox.SetText("Saved")
			} else {
				cityBox.SetText("Canceled")
			}
		})
	})
	window.AddContent(confirm)

	regionLabel := tv.NewLabel("&Region", tv.Rect{X: 20, Y: 12, W: 8, H: 1})
	region := tv.NewSelect(desktop, []string{"Europe", "Africa", "Asia", "Americas", "Oceania"}, tv.Rect{X: 28, Y: 12, W: 18, H: 1})
	regionLabel.SetTarget(region)
	window.AddContent(regionLabel)
	window.AddContent(region)

	status := tv.NewComponent(tv.Rect{X: 1, Y: 0, W: 74, H: 1})
	status.DrawFn = func(component *tv.VisualComponent, surface tv.Surface) {
		abs := component.AbsoluteBounds()
		text := fmt.Sprintf("focus:%s  name:%s  city:%s  region:%s", focusName(nameBox, cityBox, multi, view, confirm, region), nameBox.GetText(), cityBox.GetText(), region.Value())
		surface.WriteString(abs.X, abs.Y, text, tui.Cell{FG: tui.ANSIColor(15), BG: tui.ANSIColor(4), Bold: true})
	}
	window.AddBottom(status)
	desktop.AddLayer(tv.NewWindowLayer("window", window))

	// Ctrl+C asks before quitting (in raw mode it arrives as a key event, not a
	// signal). Plain 'q' no longer quits, so it can be typed into the fields.
	quitting := false
	app.OnType(func(event tui.TypeEvent) {
		if event.Key == tui.KeyRune && event.Rune == 'c' && event.Ctrl {
			if quitting {
				return
			}
			quitting = true
			tv.ShowConfirmYesNo(desktop, "Quit", "Quit the demo?", func(yes bool) {
				quitting = false
				if yes {
					stop()
				}
			})
		}
	})

	if err := desktop.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "demo run failed: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime).Round(time.Millisecond)
	summary := strings.Join([]string{
		tui.Styled("TurboTV demo closed.", tui.ANSIColor(15), tui.DefaultColor(), true),
		tui.Styled(fmt.Sprintf("  Ran for %s", elapsed), tui.ANSIColor(10), tui.DefaultColor(), false),
		tui.Styled(fmt.Sprintf("  Name field: %q", nameBox.GetText()), tui.ANSIColor(14), tui.DefaultColor(), false),
		tui.Styled("  Thanks for trying the toolkit!", tui.ANSIColor(11), tui.DefaultColor(), false),
	}, "\n")
	app.CloseWithMessage(summary)
}

func focusName(a *tv.TextBox, b *tv.TextBox, m *tv.MultiLineInput, v *tv.TextView, button *tv.Button, region *tv.Select) string {
	if a.Component.HasFocus {
		return "name"
	}
	if b.Component.HasFocus {
		return "city"
	}
	if m.Component.HasFocus {
		return "notes"
	}
	if v.Component.HasFocus {
		return "textview"
	}
	if button.Component.HasFocus {
		return "confirm"
	}
	if region.Component.HasFocus {
		return "region"
	}
	return "-"
}
