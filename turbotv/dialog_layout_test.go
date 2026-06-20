package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// TestButtonLabelWidth checks the natural button width is derived from the label
// (mnemonic marker stripped) and floored at minButtonWidth.
func TestButtonLabelWidth(t *testing.T) {
	tests := []struct {
		label string
		want  int
	}{
		{"OK", minButtonWidth},                    // short caption floors at the minimum
		{"&Yes", minButtonWidth},                  // '&' is not counted
		{"Cancel", minButtonWidth},                // 6+4 = 10 == minimum
		{"Save changes", len("Save changes") + 4}, // long caption sizes to content
	}
	for _, tc := range tests {
		if got := buttonLabelWidth(tc.label); got != tc.want {
			t.Errorf("buttonLabelWidth(%q) = %d, want %d", tc.label, got, tc.want)
		}
	}
}

// TestNewButtonRowAlignsAndDoesNotOverlap lays a row out at several alignments and
// asserts the buttons are content-sized, disjoint, in-bounds, and grouped per the
// requested alignment (issue #7).
func TestNewButtonRowAlignsAndDoesNotOverlap(t *testing.T) {
	const interiorW = 40
	tests := []struct {
		name     string
		align    Align
		wantBoxX int
	}{
		{"center", AlignCenter, (interiorW - (2*minButtonWidth + DefaultButtonGap)) / 2},
		{"end", AlignEnd, interiorW - (2*minButtonWidth + DefaultButtonGap)},
		{"start", AlignStart, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewButton("OK", Rect{}, nil)
			b := NewButton("No", Rect{}, nil)
			row := NewButtonRow(2, interiorW, tc.align, DefaultButtonGap, a, b)
			if row.Component.Bounds.X != tc.wantBoxX {
				t.Fatalf("box X = %d, want %d", row.Component.Bounds.X, tc.wantBoxX)
			}
			if row.Component.Bounds.Y != 2 {
				t.Fatalf("box Y = %d, want 2", row.Component.Bounds.Y)
			}
			// Lay the children out (HBox lays out in its LayoutFn).
			row.Component.LayoutFn(row.Component)
			ab := a.Component.Bounds
			bb := b.Component.Bounds
			if ab.W != minButtonWidth || bb.W != minButtonWidth {
				t.Fatalf("buttons not content-sized: a=%d b=%d", ab.W, bb.W)
			}
			// Disjoint with exactly the gap between them.
			if bb.X != ab.Right()+1+DefaultButtonGap {
				t.Fatalf("buttons not separated by the gap: a=%+v b=%+v", ab, bb)
			}
			// In-bounds within the interior (absolute = box X + child X).
			if row.Component.Bounds.X+bb.Right() >= interiorW {
				t.Fatalf("row overflows the interior: boxX=%d b=%+v interiorW=%d",
					row.Component.Bounds.X, bb, interiorW)
			}
		})
	}
}

// TestShowConfirmYesNoSizesToContent verifies a short-message dialog is narrower
// than a long-message one (the width tracks the message), both stay on-screen, and
// the buttons render in-bounds — replacing the old fixed width=54 (issues #7, #48).
func TestShowConfirmYesNoSizesToContent(t *testing.T) {
	app := tui.NewWithSize(80, 25, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 80, H: 25})))

	short := ShowConfirmYesNo(desktop, "Confirm", "OK?", nil)
	shortW := short.window.Component.Bounds.W
	desktop.RemoveLayer(short)

	long := ShowConfirmYesNo(desktop, "Confirm",
		"This is a considerably longer confirmation message that should widen the dialog.", nil)
	longW := long.window.Component.Bounds.W

	if longW <= shortW {
		t.Fatalf("expected a longer message to widen the dialog: short=%d long=%d", shortW, longW)
	}
	if longW > 80 {
		t.Fatalf("dialog width %d exceeds the 80-col screen", longW)
	}
	b := long.window.Component.Bounds
	if b.X < 0 || b.Right() > 79 {
		t.Fatalf("dialog is off-screen: %+v", b)
	}
}

// TestShowConfirmYesNoClampsToNarrowScreen builds the dialog on a tiny terminal and
// asserts it never extends past the screen edges — the old code only floored x/y at
// 0 and clipped the fixed 54-wide frame (issue #48).
func TestShowConfirmYesNoClampsToNarrowScreen(t *testing.T) {
	app := tui.NewWithSize(24, 8, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 24, H: 8})))

	layer := ShowConfirmYesNo(desktop, "Confirm", "Delete everything in this folder?", nil)
	b := layer.window.Component.Bounds
	if b.X < 0 || b.Y < 0 || b.Right() > 23 || b.Bottom() > 7 {
		t.Fatalf("dialog not clamped to the 24x8 screen: %+v", b)
	}
}

// TestShowConfirmYesNoMnemonics confirms the buttons carry &Yes / &No mnemonics so
// Alt+Y / Alt+N activate them regardless of focus (issue #63).
func TestShowConfirmYesNoMnemonics(t *testing.T) {
	for _, tc := range []struct {
		key  rune
		want string
	}{
		{'y', "yes"},
		{'n', "no"},
	} {
		app := tui.NewWithSize(80, 25, &bytes.Buffer{})
		desktop := NewDesktop(app)
		desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 80, H: 25})))

		result := "none"
		ShowConfirmYesNo(desktop, "Confirm", "Apply?", func(yes bool) {
			if yes {
				result = "yes"
			} else {
				result = "no"
			}
		})
		desktop.Redraw() // populate mnemonicActive

		desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: tc.key, Alt: true})
		if result != tc.want {
			t.Fatalf("Alt+%c expected %q, got %q", tc.key, tc.want, result)
		}
	}
}

// TestDialogCloseButtonSelfDismisses checks a generic NewDialog (which inherits a
// live [■]) removes its own layer when clicked and no OnClose is set, instead of
// being a silent no-op (issue #49).
func TestDialogCloseButtonSelfDismisses(t *testing.T) {
	app := tui.NewWithSize(60, 20, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 60, H: 20})))

	dialog := NewDialog("Stats", 10, 5, 30, 8)
	layer := NewModalLayer("stats", dialog)
	desktop.AddLayer(layer)
	if got := len(desktop.layerSnapshot()); got != 2 {
		t.Fatalf("expected 2 layers after opening the dialog, got %d", got)
	}

	abs := dialog.Window.Component.AbsoluteBounds()
	buttons := dialog.Window.titleButtons(abs)
	desktop.handleClick(tui.ClickEvent{X: buttons.closeRect.X, Y: buttons.closeRect.Y, Button: tui.MouseLeft, Down: true})

	if got := len(desktop.layerSnapshot()); got != 1 {
		t.Fatalf("clicking [■] should remove the dialog layer; got %d layers", got)
	}
}

// TestDialogCloseButtonRoutesThroughOnClose checks that when an app does wire
// OnClose, the close button still routes through it only (so a confirmation step
// keeps control) and the framework does not auto-remove the layer (issue #49).
func TestDialogCloseButtonRoutesThroughOnClose(t *testing.T) {
	app := tui.NewWithSize(60, 20, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 60, H: 20})))

	dialog := NewDialog("Stats", 10, 5, 30, 8)
	closed := 0
	dialog.Window.OnClose = func(*Window) { closed++ } // app handles teardown itself
	layer := NewModalLayer("stats", dialog)
	desktop.AddLayer(layer)

	abs := dialog.Window.Component.AbsoluteBounds()
	buttons := dialog.Window.titleButtons(abs)
	desktop.handleClick(tui.ClickEvent{X: buttons.closeRect.X, Y: buttons.closeRect.Y, Button: tui.MouseLeft, Down: true})

	if closed != 1 {
		t.Fatalf("expected OnClose to fire once, got %d", closed)
	}
	if got := len(desktop.layerSnapshot()); got != 2 {
		t.Fatalf("with OnClose set the app owns teardown; framework should not remove the layer (got %d)", got)
	}
}

// TestDialogDefaultCancelButtons exercises Enter→default / Escape→cancel when focus
// is on a non-button field, so the keystroke reaches the dialog root (issue #63).
func TestDialogDefaultCancelButtons(t *testing.T) {
	app := tui.NewWithSize(60, 20, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 60, H: 20})))

	dialog := NewDialog("Form", 10, 5, 30, 8)
	result := ""
	ok := NewButton("&OK", Rect{}, func() { result = "ok" })
	ok.Default = true
	cancel := NewButton("&Cancel", Rect{}, func() { result = "cancel" })
	cancel.Cancel = true
	dialog.Window.AddContent(NewButtonRow(4, 28, AlignCenter, DefaultButtonGap, ok, cancel))

	// A plain focusable field that does not consume Enter/Escape, so the keys bubble
	// to the dialog root where the default/cancel handler lives.
	field := NewComponent(Rect{X: 1, Y: 1, W: 10, H: 1})
	field.Focusable = true
	dialog.Window.AddContent(field)
	dialog.SetDefaultCancelButtons(ok, cancel)

	desktop.AddLayer(NewModalLayer("form", dialog))
	desktop.SetFocus(field)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter})
	if result != "ok" {
		t.Fatalf("Enter on a non-button field should activate the default button, got %q", result)
	}

	result = ""
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEscape})
	if result != "cancel" {
		t.Fatalf("Escape should activate the cancel button, got %q", result)
	}
}
