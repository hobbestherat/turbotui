package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tui "github.com/hobbestherat/turbotui"
)

func main() {
	app, err := tui.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize TUI: %v\n", err)
		os.Exit(1)
	}

	lastX := 2
	lastY := 3
	scrollCount := 0

	white := tui.ANSIColor(15)
	blue := tui.ANSIColor(4)
	green := tui.ANSIColor(2)
	black := tui.ANSIColor(0)
	yellow := tui.ANSIColor(11)

	drawStatic := func() {
		app.Clear(tui.Cell{Ch: ' ', FG: white, BG: black})
		app.DrawBox(0, 0, app.Width(), app.Height(), tui.LineDouble, blue, black)
		app.WriteString(2, 1, "Go TUI demo - click, type, scroll, resize. Press q to quit.", tui.Cell{FG: yellow, BG: black})
		app.WriteString(2, 2, "Last mouse position:", tui.Cell{FG: green, BG: black})
		app.WriteString(24, 2, fmt.Sprintf("(%d, %d)  ", lastX, lastY), tui.Cell{FG: white, BG: black, Bold: true})
		app.WriteString(2, 4, "Scroll:", tui.Cell{FG: green, BG: black})
		app.WriteString(10, 4, fmt.Sprintf("%d  ", scrollCount), tui.Cell{FG: white, BG: black, Bold: true})
		centerX := app.Width() / 2
		app.WriteString(centerX-2, 4, "▲", tui.Cell{FG: white, BG: black})
		app.WriteString(centerX-2, 6, "▼", tui.Cell{FG: white, BG: black})
	}

	drawStatic()
	if err := app.Apply(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to render initial frame: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app.OnClick(func(event tui.ClickEvent) {
		if !event.Down {
			return
		}
		lastX = event.X
		lastY = event.Y
		drawStatic()
		_ = app.Apply()
	})

	app.OnScroll(func(event tui.ScrollEvent) {
		scrollCount += event.Delta
		drawStatic()
		if event.Delta > 0 {
			app.WriteString(app.Width()/2, 4, "↑", tui.Cell{FG: yellow, BG: black, Bold: true})
		} else {
			app.WriteString(app.Width()/2, 6, "↓", tui.Cell{FG: yellow, BG: black, Bold: true})
		}
		_ = app.Apply()
	})

	app.OnType(func(event tui.TypeEvent) {
		if event.Key == tui.KeyRune && event.Rune == 'q' && !event.Ctrl {
			stop()
			return
		}
		label := keyLabel(event)
		app.WriteString(lastX, lastY, label, tui.Cell{FG: yellow, BG: black, Bold: true})
		_ = app.Apply()
	})

	app.OnResize(func(_ tui.ResizeEvent) {
		drawStatic()
	})

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "TUI run error: %v\n", err)
		os.Exit(1)
	}
}

func keyLabel(event tui.TypeEvent) string {
	if event.Key == tui.KeyRune {
		if event.Ctrl {
			return fmt.Sprintf("Ctrl-%c", event.Rune)
		}
		return fmt.Sprintf("Key %q", event.Rune)
	}
	switch event.Key {
	case tui.KeyEnter:
		return "Enter"
	case tui.KeyTab:
		return "Tab"
	case tui.KeyBackspace:
		return "Backspace"
	case tui.KeyEscape:
		return "Escape"
	case tui.KeyUp:
		return "Arrow Up"
	case tui.KeyDown:
		return "Arrow Down"
	case tui.KeyLeft:
		return "Arrow Left"
	case tui.KeyRight:
		return "Arrow Right"
	case tui.KeyHome:
		return "Home"
	case tui.KeyEnd:
		return "End"
	case tui.KeyPageUp:
		return "PageUp"
	case tui.KeyPageDown:
		return "PageDown"
	case tui.KeyInsert:
		return "Insert"
	case tui.KeyDelete:
		return "Delete"
	default:
		return "Unknown"
	}
}
