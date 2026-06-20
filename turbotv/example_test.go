package tv_test

import (
	"fmt"
	"io"
	"strings"

	tui "github.com/hobbestherat/turbotui"
	tv "github.com/hobbestherat/turbotui/turbotv"
)

// screenContains reports whether the rune sequence of any single row contains
// sub, i.e. whether sub was composited somewhere onto the screen.
func screenContains(app *tui.App, sub string) bool {
	for y := 0; y < app.Height(); y++ {
		row := make([]rune, 0, app.Width())
		for x := 0; x < app.Width(); x++ {
			row = append(row, app.ReadCell(x, y).Ch)
		}
		if strings.Contains(string(row), sub) {
			return true
		}
	}
	return false
}

// Example builds a desktop with one window and a button, then composites a
// frame into an off-screen buffer. A real program would use tui.New() and
// desktop.Run(ctx); the buffer-backed app (NewWithSize) plus Redraw is what
// makes the rendered output checkable here.
func Example() {
	app := tui.NewWithSize(40, 12, io.Discard)
	desktop := tv.NewDesktop(app)

	window := tv.NewWindow("Hello", tv.Rect{X: 2, Y: 1, W: 30, H: 8}, tui.LineDouble)
	window.AddContent(tv.NewButton("&Quit", tv.Rect{X: 2, Y: 2, W: 12, H: 1}, func() {}))
	desktop.AddLayer(tv.NewWindowLayer("main", window))

	desktop.Redraw() // compose every layer into the buffer

	fmt.Println("title drawn:", screenContains(app, "Hello"))
	fmt.Println("button drawn:", screenContains(app, "Quit"))
	// Output:
	// title drawn: true
	// button drawn: true
}

// Example_customWidget builds a minimal widget directly from a VisualComponent
// and a pair of closures — no new type required. The widget fills its bounds
// with a glyph that toggles when it is focused and the spacebar is pressed,
// showing how DrawFn paints and how an input callback consumes a key (returning
// true) instead of letting it bubble (returning false).
func Example_customWidget() {
	on := false

	gauge := tv.NewComponent(tv.Rect{X: 1, Y: 1, W: 4, H: 1})
	gauge.Focusable = true

	// DrawFn paints the component's own content; its children draw afterwards.
	gauge.DrawFn = func(c *tv.VisualComponent, s tv.Surface) {
		glyph := '-'
		if on {
			glyph = '#'
		}
		s.Fill(c.AbsoluteBounds(), tui.Cell{Ch: glyph})
	}
	// OnTypeFn is offered each key while the widget is focused. Returning true
	// consumes the key; returning false lets it bubble up to the parent.
	gauge.OnTypeFn = func(_ *tv.VisualComponent, e tui.TypeEvent) bool {
		if e.Key == tui.KeyRune && e.Rune == ' ' {
			on = !on
			return true
		}
		return false
	}

	app := tui.NewWithSize(8, 4, io.Discard)
	desktop := tv.NewDesktop(app)
	desktop.AddLayer(tv.NewWindowLayer("main", gauge))
	desktop.SetFocus(gauge)

	desktop.Redraw()
	fmt.Printf("before: %c\n", app.ReadCell(1, 1).Ch)

	// The desktop calls OnTypeFn on the focused widget; simulate a spacebar.
	gauge.OnTypeFn(gauge, tui.TypeEvent{Key: tui.KeyRune, Rune: ' '})
	desktop.Redraw()
	fmt.Printf("after:  %c\n", app.ReadCell(1, 1).Ch)
	// Output:
	// before: -
	// after:  #
}
