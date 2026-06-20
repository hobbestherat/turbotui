package tui_test

import (
	"fmt"
	"io"
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

// Example draws a framed greeting into an off-screen buffer and reads it back.
// A real program would use tui.New() and app.Run(ctx) instead of NewWithSize +
// ReadCell; the buffer-backed app is what makes the output reproducible here.
func Example() {
	app := tui.NewWithSize(20, 3, io.Discard)

	white := tui.ANSIColor(15)
	blue := tui.ANSIColor(4)

	app.Clear(tui.Cell{Ch: ' ', BG: blue})
	app.DrawBox(0, 0, app.Width(), app.Height(), tui.LineDouble, white, blue)
	app.WriteString(2, 1, "Hi there!", tui.Cell{FG: white, BG: blue})

	// Read the text row back out of the buffer.
	var row strings.Builder
	for x := 2; x < 2+len("Hi there!"); x++ {
		row.WriteRune(app.ReadCell(x, 1).Ch)
	}
	fmt.Println(row.String())
	// Output: Hi there!
}

// ExampleApp_OnType wires a key handler the way the Run loop would invoke it.
// Run normally blocks on real terminal input; here the registered handler is
// called directly so the example stays self-contained.
func ExampleApp_OnType() {
	app := tui.NewWithSize(20, 1, io.Discard)

	quit := false
	onKey := func(e tui.TypeEvent) {
		if e.Key == tui.KeyRune && e.Rune == 'q' {
			quit = true
		}
	}
	app.OnType(onKey) // Run delivers parsed key events to this handler

	// Show what the handler does when the user presses 'q' (Run would call it).
	onKey(tui.TypeEvent{Key: tui.KeyRune, Rune: 'q'})
	fmt.Println(quit)
	// Output: true
}
