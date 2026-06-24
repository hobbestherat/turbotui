// Command tabs demos the tv.Tabs widget: a window holding a Tabs with three pages
// of mixed content — a multi-select checkbox group, a single-choice RadioGroup, and
// a couple of text fields. Switch tabs with Alt+Left/Alt+Right (or Ctrl+Tab), Tab
// moves focus within the active tab, and clicking a label jumps to it. Run it with
// `go run ./turbotv/cmd/tabs` and press Ctrl+C to quit.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tui "github.com/hobbestherat/turbotui"
	tv "github.com/hobbestherat/turbotui/turbotv"
)

func main() {
	app := tui.New()
	if err := app.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize TUI: %v\n", err)
		os.Exit(1)
	}
	desktop := tv.NewDesktop(app)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := tv.NewComponent(tv.Rect{X: 0, Y: 0, W: app.Width(), H: app.Height()})
	root.DrawFn = func(component *tv.VisualComponent, surface tv.Surface) {
		b := component.AbsoluteBounds()
		surface.Fill(b, tui.Cell{Ch: ' ', FG: tui.ANSIColor(15), BG: tui.ANSIColor(4)})
	}
	desktop.AddLayer(tv.NewFullscreenLayer("base", root))

	window := tv.NewWindow("Tabs demo — Alt+←/→ to switch, Tab within", tv.Rect{X: 4, Y: 2, W: 64, H: 20}, tui.LineDouble)
	window.OnClose = func(_ *tv.Window) { stop() }

	status := tv.NewLabel("Active tab: Scope", tv.Rect{X: 1, Y: 16, W: 60, H: 1})

	tabs := tv.NewTabs(desktop, tv.Rect{X: 1, Y: 1, W: 60, H: 14})
	tabs.AddTab("Scope", scopePage())
	tabs.AddTab("Constraints", constraintsPage())
	tabs.AddTab("Notes", notesPage())
	titles := []string{"Scope", "Constraints", "Notes"}
	tabs.OnTabChange = func(index int) {
		status.SetText("Active tab: " + titles[index])
	}

	window.AddContent(tabs)
	window.AddContent(status)
	desktop.AddLayer(tv.NewWindowLayer("window", window))

	if err := desktop.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "tabs demo run failed: %v\n", err)
		os.Exit(1)
	}
	app.CloseWithMessage("Tabs demo closed.")
}

// scopePage builds a multi-select group of checkboxes.
func scopePage() *tv.Box {
	page := tv.NewVBox(tv.Rect{X: 0, Y: 0, W: 0, H: 0})
	page.Spacing = 0
	page.Add(tv.NewLabel("Which areas should I touch? (pick any)", tv.Rect{X: 0, Y: 0, W: 50, H: 1}))
	ms := tv.NewMultiSelect()
	for _, name := range []string{"&Backend API", "&Frontend UI", "&Tests", "&Docs"} {
		cb := tv.NewCheckbox(name, tv.Rect{X: 0, Y: 0, W: 30, H: 1}, nil)
		ms.Add(cb)
		page.Add(cb)
	}
	return page
}

// constraintsPage builds a single-choice RadioGroup.
func constraintsPage() *tv.Box {
	page := tv.NewVBox(tv.Rect{X: 0, Y: 0, W: 0, H: 0})
	page.Add(tv.NewLabel("Priority (single choice):", tv.Rect{X: 0, Y: 0, W: 50, H: 1}))
	rg := tv.NewRadioGroup()
	for _, name := range []string{"&Low", "&Normal", "&High", "&Critical"} {
		cb := tv.NewCheckbox(name, tv.Rect{X: 0, Y: 0, W: 30, H: 1}, nil)
		rg.Add(cb)
		page.Add(cb)
	}
	rg.SetSelected(1)
	return page
}

// notesPage builds a couple of text fields.
func notesPage() *tv.Box {
	page := tv.NewVBox(tv.Rect{X: 0, Y: 0, W: 0, H: 0})
	page.Add(tv.NewLabel("One-line goal:", tv.Rect{X: 0, Y: 0, W: 50, H: 1}))
	page.Add(tv.NewTextBox("goal", tv.Rect{X: 0, Y: 0, W: 50, H: 1}))
	page.Add(tv.NewLabel("Notes:", tv.Rect{X: 0, Y: 0, W: 50, H: 1}))
	page.Add(tv.NewMultiLineInput("", tv.Rect{X: 0, Y: 0, W: 50, H: 5}))
	return page
}
