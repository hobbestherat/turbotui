package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// --- Issue #61: menu shortcuts can carry Shift/Alt and function keys. ---

func TestMatchShortcutModifiers(t *testing.T) {
	shiftF1 := &MenuShortcut{Key: tui.KeyF1, Shift: true}
	if !matchShortcut(tui.TypeEvent{Key: tui.KeyF1, Shift: true}, shiftF1) {
		t.Fatal("Shift+F1 should match a Shift+F1 shortcut")
	}
	if matchShortcut(tui.TypeEvent{Key: tui.KeyF1}, shiftF1) {
		t.Fatal("bare F1 must not match a Shift+F1 shortcut")
	}

	ctrlShiftS := &MenuShortcut{Key: tui.KeyRune, Rune: 's', Ctrl: true, Shift: true}
	if !matchShortcut(tui.TypeEvent{Key: tui.KeyRune, Rune: 's', Ctrl: true, Shift: true}, ctrlShiftS) {
		t.Fatal("Ctrl+Shift+S should match")
	}
	if matchShortcut(tui.TypeEvent{Key: tui.KeyRune, Rune: 's', Ctrl: true}, ctrlShiftS) {
		t.Fatal("Ctrl+S must not match a Ctrl+Shift+S shortcut")
	}
}

func TestWithShortcutModBuildsShortcut(t *testing.T) {
	item := NewMenuItem("Save", nil).WithShortcutMod("Ctrl+Shift+S", tui.KeyRune, 's', true, true, false)
	sc := item.Shortcut
	if sc == nil || !sc.Ctrl || !sc.Shift || sc.Alt || sc.Rune != 's' {
		t.Fatalf("WithShortcutMod built unexpected shortcut: %#v", sc)
	}
}

func TestFunctionKeyAcceleratorFires(t *testing.T) {
	fired := 0
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 20, H: 1},
		NewSubMenu("&File",
			NewMenuItem("Help", func() { fired++ }).WithShortcutMod("F1", tui.KeyF1, 0, false, false, false),
		),
	)
	if !bar.HandleAccelerator(tui.TypeEvent{Key: tui.KeyF1}) {
		t.Fatal("F1 accelerator should be handled")
	}
	if fired != 1 {
		t.Fatalf("expected F1 accelerator to fire once, got %d", fired)
	}
}

// --- Issue #75: Ctrl+C quits by default when nothing consumes it. ---

func TestDefaultCtrlCQuits(t *testing.T) {
	app := tui.NewWithSize(20, 5, &bytes.Buffer{})
	desktop := NewDesktop(app)
	quit := 0
	desktop.cancel = func() { quit++ }

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'c', Ctrl: true})
	if quit != 1 {
		t.Fatalf("expected default Ctrl+C to quit once, got %d", quit)
	}
}

func TestCtrlCConsumedByCopyDoesNotQuit(t *testing.T) {
	app := tui.NewWithSize(20, 5, &bytes.Buffer{})
	desktop := NewDesktop(app)
	quit := 0
	desktop.cancel = func() { quit++ }

	// A focused widget that has something to copy consumes Ctrl+C, so it must not
	// fall through to the default quit.
	root := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	root.Focusable = true
	root.CopyFn = func(*VisualComponent) (string, bool) { return "selection", true }
	desktop.AddLayer(NewLayer("base", root, true, false))
	desktop.setFocus(root)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'c', Ctrl: true})
	if quit != 0 {
		t.Fatalf("Ctrl+C should have been consumed by copy, not quit (quit=%d)", quit)
	}
}

func TestUnhandledKeyFnOverridesDefaultQuit(t *testing.T) {
	app := tui.NewWithSize(20, 5, &bytes.Buffer{})
	desktop := NewDesktop(app)
	quit := 0
	desktop.cancel = func() { quit++ }
	seen := 0
	desktop.SetUnhandledKeyFn(func(tui.TypeEvent) { seen++ })

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'c', Ctrl: true})
	if quit != 0 || seen != 1 {
		t.Fatalf("custom handler should replace default quit: quit=%d seen=%d", quit, seen)
	}
}

// --- Issue #17: Desktop.Post defers the redraw instead of flushing inline. ---

func TestDesktopPostDefersRedraw(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(20, 5, &output)
	desktop := NewDesktop(app)
	root := NewComponent(Rect{X: 0, Y: 0, W: 20, H: 5})
	desktop.AddLayer(NewLayer("base", root, true, false)) // composes + flushes once

	output.Reset()
	// Posting must only enqueue work; the actual repaint is coalesced and happens
	// when the event loop drains the mailbox, so nothing is written here.
	desktop.Post(func() {})
	desktop.Post(func() {})
	if output.Len() != 0 {
		t.Fatalf("Desktop.Post flushed inline (%q); the redraw should be deferred", output.String())
	}
}
