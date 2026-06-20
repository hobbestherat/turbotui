package tv

import (
	"bytes"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestTabFocusStaysInTopLayer(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(40, 10, &output)
	desktop := NewDesktop(app)

	baseRoot := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 10})
	baseA := NewComponent(Rect{X: 1, Y: 1, W: 5, H: 1})
	baseA.Focusable = true
	baseRoot.AddChild(baseA)
	desktop.AddLayer(NewLayer("base", baseRoot, true, false))

	topRoot := NewComponent(Rect{X: 2, Y: 2, W: 30, H: 6})
	topA := NewComponent(Rect{X: 1, Y: 1, W: 5, H: 1})
	topA.Focusable = true
	topB := NewComponent(Rect{X: 1, Y: 3, W: 5, H: 1})
	topB.Focusable = true
	topRoot.AddChild(topA)
	topRoot.AddChild(topB)
	desktop.AddLayer(NewLayer("top", topRoot, true, false))

	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
	if !topA.Focused() {
		t.Fatalf("expected top layer first focusable to receive focus")
	}
	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
	if !topB.Focused() {
		t.Fatalf("expected top layer second focusable to receive focus")
	}
	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
	if !topA.Focused() {
		t.Fatalf("expected focus cycle to stay in top layer")
	}
	if baseA.Focused() {
		t.Fatalf("expected base layer focusable to stay unfocused")
	}
}

func TestWindowDragMovesComponent(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(80, 25, &output)
	desktop := NewDesktop(app)

	window := NewWindow("drag", Rect{X: 10, Y: 6, W: 20, H: 8}, tui.LineSingle)
	desktop.AddLayer(NewLayer("window", window.Component, true, false))

	desktop.handleClick(tui.ClickEvent{X: 12, Y: 6, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: 20, Y: 10, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: 20, Y: 10, Button: tui.MouseLeft, Down: false})

	if window.Component.Bounds.X != 18 || window.Component.Bounds.Y != 10 {
		t.Fatalf("expected dragged window at (18,10), got (%d,%d)", window.Component.Bounds.X, window.Component.Bounds.Y)
	}
}

func TestWindowDragKeepsCaptureOutsideBounds(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(80, 25, &output)
	desktop := NewDesktop(app)

	window := NewWindow("drag", Rect{X: 10, Y: 6, W: 20, H: 8}, tui.LineSingle)
	desktop.AddLayer(NewLayer("window", window.Component, true, false))

	desktop.handleClick(tui.ClickEvent{X: 12, Y: 6, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: 2, Y: 20, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: 2, Y: 20, Button: tui.MouseLeft, Down: false})

	if window.Component.Bounds.X != 0 || window.Component.Bounds.Y != 20 {
		t.Fatalf("expected drag to continue with capture, got (%d,%d)", window.Component.Bounds.X, window.Component.Bounds.Y)
	}
}

func TestClipRectPreventsChildOverdraw(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(20, 6, &output)
	desktop := NewDesktop(app)
	desktop.SetBackground(tui.Cell{Ch: '.', FG: tui.ANSIColor(7), BG: tui.ANSIColor(0)})

	root := NewComponent(Rect{X: 0, Y: 0, W: 20, H: 6})
	parent := NewComponent(Rect{X: 2, Y: 2, W: 3, H: 1})
	child := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	child.DrawFn = func(component *VisualComponent, surface Surface) {
		abs := component.AbsoluteBounds()
		surface.WriteString(abs.X, abs.Y, "ABCDE", tui.Cell{FG: tui.ANSIColor(15), BG: tui.ANSIColor(0)})
	}
	parent.AddChild(child)
	root.AddChild(parent)
	desktop.AddLayer(NewLayer("root", root, false, false))
	desktop.Redraw()

	rendered := output.String()
	if !strings.Contains(rendered, "A") || !strings.Contains(rendered, "B") || !strings.Contains(rendered, "C") {
		t.Fatalf("expected clipped region to include ABC")
	}
	if strings.Contains(rendered, "D") || strings.Contains(rendered, "E") {
		t.Fatalf("expected clipping to prevent drawing D/E outside parent bounds")
	}
}

func TestFocusedTextBoxConsumesArrowKeys(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(40, 10, &output)
	desktop := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 10})
	first := NewTextBox("AB", Rect{X: 1, Y: 1, W: 10, H: 1})
	second := NewTextBox("CD", Rect{X: 1, Y: 3, W: 10, H: 1})
	root.AddChild(first.Component)
	root.AddChild(second.Component)
	desktop.AddLayer(NewLayer("top", root, true, false))

	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
	if !first.Component.Focused() {
		t.Fatalf("expected first textbox to be focused")
	}
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRight})
	if !first.Component.Focused() {
		t.Fatalf("expected focus to stay in first textbox for cursor move")
	}
}

func TestClickRaisesLowerWindowAndFocusesIt(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(80, 25, &output)
	desktop := NewDesktop(app)

	// Window A on the left, window B on the right; B is added last so it starts
	// on top with the keyboard focus.
	windowA := NewWindow("A", Rect{X: 0, Y: 0, W: 20, H: 8}, tui.LineSingle)
	inputA := NewComponent(Rect{X: 0, Y: 0, W: 5, H: 1})
	inputA.Focusable = true
	windowA.Content.AddChild(inputA)
	layerA := NewWindowLayer("A", windowA)
	desktop.AddLayer(layerA)

	windowB := NewWindow("B", Rect{X: 40, Y: 0, W: 20, H: 8}, tui.LineSingle)
	inputB := NewComponent(Rect{X: 0, Y: 0, W: 5, H: 1})
	inputB.Focusable = true
	windowB.Content.AddChild(inputB)
	layerB := NewWindowLayer("B", windowB)
	desktop.AddLayer(layerB)
	desktop.SetFocus(inputB)

	// Click window A's title bar (a non-focusable region).
	desktop.handleClick(tui.ClickEvent{X: 2, Y: 0, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: 2, Y: 0, Button: tui.MouseLeft, Down: false})

	if desktop.TopLayer() != layerA {
		t.Fatalf("expected clicked window A to be raised to the top of the stack")
	}
	if !inputA.Focused() {
		t.Fatalf("expected clicking window A to move keyboard focus into it")
	}
	if inputB.Focused() {
		t.Fatalf("expected window B to lose focus when A is clicked")
	}
}

func TestRaiseLayerKeepsModalOnTop(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(80, 25, &output)
	desktop := NewDesktop(app)

	winRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layer := NewWindowLayer("win", winRoot)
	other := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	otherLayer := NewWindowLayer("other", other)
	modalRoot := NewComponent(Rect{X: 5, Y: 5, W: 10, H: 4})
	modalLayer := NewModalLayer("modal", modalRoot)

	desktop.AddLayer(layer)
	desktop.AddLayer(otherLayer)
	desktop.AddLayer(modalLayer)

	desktop.RaiseLayer(layer)

	if desktop.TopLayer() != modalLayer {
		t.Fatalf("expected modal layer to stay on top after raising a window")
	}
}

func TestHitTestFallsThroughLowerLayer(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(30, 8, &output)
	desktop := NewDesktop(app)

	bottom := NewComponent(Rect{X: 0, Y: 0, W: 30, H: 1})
	top := NewComponent(Rect{X: 0, Y: 2, W: 30, H: 2})
	desktop.AddLayer(NewLayer("bottom", bottom, true, false))
	desktop.AddLayer(NewLayer("top", top, true, false))

	target := desktop.hitTestTopLayer(1, 0)
	if target != bottom {
		t.Fatalf("expected lower input layer target, got %#v", target)
	}
}
