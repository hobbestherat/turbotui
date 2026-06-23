package tv

import (
	"testing"
	"time"

	tui "github.com/hobbestherat/turbotui"
)

func focusableComponent(bounds Rect) *VisualComponent {
	component := NewComponent(bounds)
	component.Focusable = true
	return component
}

func TestModalRemoveRestoresPreviousFocus(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	baseRoot := NewComponent(Rect{W: 80, H: 25})
	input := NewTextBox("", Rect{X: 1, Y: 1, W: 20, H: 1})
	baseRoot.AddChild(input)
	desktop.AddLayer(NewFullscreenLayer("base", baseRoot))
	desktop.SetFocus(input)

	modalRoot := NewComponent(Rect{X: 10, Y: 5, W: 20, H: 5})
	button := NewButton("OK", Rect{X: 1, Y: 1, W: 10, H: 1}, nil)
	modalRoot.AddChild(button)
	modalLayer := NewModalLayer("modal", modalRoot)
	desktop.AddLayer(modalLayer)
	desktop.SetFocus(button)

	desktop.RemoveLayer(modalLayer)

	if !input.Component.Focused() {
		t.Fatalf("expected focus to return to pre-modal textbox")
	}
	if button.Component.Focused() {
		t.Fatalf("expected removed modal button to lose focus")
	}
}

func TestModalRemoveClearsFocusWhenPreviousWidgetWasRemoved(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	baseRoot := NewComponent(Rect{W: 80, H: 25})
	input := NewTextBox("", Rect{X: 1, Y: 1, W: 20, H: 1})
	baseRoot.AddChild(input)
	desktop.AddLayer(NewFullscreenLayer("base", baseRoot))
	desktop.SetFocus(input)

	modalRoot := NewComponent(Rect{X: 10, Y: 5, W: 20, H: 5})
	button := NewButton("OK", Rect{X: 1, Y: 1, W: 10, H: 1}, nil)
	modalRoot.AddChild(button)
	modalLayer := NewModalLayer("modal", modalRoot)
	desktop.AddLayer(modalLayer)
	desktop.SetFocus(button)
	baseRoot.RemoveChild(input)

	desktop.RemoveLayer(modalLayer)

	if desktop.focused != nil {
		t.Fatalf("expected focus to clear when pre-modal widget is no longer in the top layer")
	}
	if input.Component.Focused() {
		t.Fatalf("expected removed pre-modal widget not to remain focused")
	}
}

func TestModalRemoveClearsFocusWhenPreviousWidgetIsNotFocusable(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	baseRoot := NewComponent(Rect{W: 80, H: 25})
	input := NewTextBox("", Rect{X: 1, Y: 1, W: 20, H: 1})
	baseRoot.AddChild(input)
	desktop.AddLayer(NewFullscreenLayer("base", baseRoot))
	desktop.SetFocus(input)

	modalRoot := NewComponent(Rect{X: 10, Y: 5, W: 20, H: 5})
	button := NewButton("OK", Rect{X: 1, Y: 1, W: 10, H: 1}, nil)
	modalRoot.AddChild(button)
	modalLayer := NewModalLayer("modal", modalRoot)
	desktop.AddLayer(modalLayer)
	desktop.SetFocus(button)
	input.Component.Focusable = false

	desktop.RemoveLayer(modalLayer)

	if desktop.focused != nil {
		t.Fatalf("expected focus to clear when pre-modal widget is no longer focusable")
	}
}

func TestNestedModalRemoveRestoresFocusStackInOrder(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	baseRoot := NewComponent(Rect{W: 80, H: 25})
	input := NewTextBox("", Rect{X: 1, Y: 1, W: 20, H: 1})
	baseRoot.AddChild(input)
	desktop.AddLayer(NewFullscreenLayer("base", baseRoot))
	desktop.SetFocus(input)

	outerRoot := NewComponent(Rect{X: 5, Y: 3, W: 30, H: 8})
	outerButton := NewButton("Outer", Rect{X: 1, Y: 1, W: 10, H: 1}, nil)
	outerRoot.AddChild(outerButton)
	outerLayer := NewModalLayer("outer", outerRoot)
	desktop.AddLayer(outerLayer)
	desktop.SetFocus(outerButton)

	innerRoot := NewComponent(Rect{X: 8, Y: 5, W: 30, H: 8})
	innerButton := NewButton("Inner", Rect{X: 1, Y: 1, W: 10, H: 1}, nil)
	innerRoot.AddChild(innerButton)
	innerLayer := NewModalLayer("inner", innerRoot)
	desktop.AddLayer(innerLayer)
	desktop.SetFocus(innerButton)

	desktop.RemoveLayer(innerLayer)
	if !outerButton.Component.Focused() {
		t.Fatalf("expected closing inner modal to restore focus to outer modal button")
	}
	if innerButton.Component.Focused() {
		t.Fatalf("expected inner modal button to lose focus after close")
	}

	desktop.RemoveLayer(outerLayer)
	if !input.Component.Focused() {
		t.Fatalf("expected closing outer modal to restore focus to base input")
	}
}

func TestNonModalRemoveDoesNotRestorePreviousLayerFocus(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	baseRoot := NewComponent(Rect{W: 80, H: 25})
	baseInput := focusableComponent(Rect{X: 1, Y: 1, W: 10, H: 1})
	baseRoot.AddChild(baseInput)
	desktop.AddLayer(NewFullscreenLayer("base", baseRoot))
	desktop.SetFocus(baseInput)

	topRoot := NewComponent(Rect{X: 10, Y: 5, W: 20, H: 5})
	topInput := focusableComponent(Rect{X: 1, Y: 1, W: 10, H: 1})
	topRoot.AddChild(topInput)
	topLayer := NewWindowLayer("top", topRoot)
	desktop.AddLayer(topLayer)
	desktop.SetFocus(topInput)

	desktop.RemoveLayer(topLayer)

	if desktop.focused != nil {
		t.Fatalf("expected non-modal removal to preserve clear-to-nil behavior, got focus on %#v", desktop.focused)
	}
	if baseInput.Focused() {
		t.Fatalf("expected focus not to be restored across non-modal layer removal")
	}
}

func TestModalEnterGraceSuppressesEnterButNotOtherActivation(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	desktop.SetClock(func() time.Time { return now })
	desktop.SetEnterGrace(300 * time.Millisecond)

	pressed := 0
	root := NewComponent(Rect{X: 10, Y: 5, W: 20, H: 5})
	button := NewButton("OK", Rect{X: 1, Y: 1, W: 10, H: 1}, func() { pressed++ })
	root.AddChild(button)
	layer := NewModalLayer("modal", root)
	desktop.AddLayer(layer)
	desktop.SetFocus(button)

	now = now.Add(299 * time.Millisecond)
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter})
	if pressed != 0 {
		t.Fatalf("expected Enter inside grace window not to press button, got %d presses", pressed)
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: ' '})
	if pressed != 1 {
		t.Fatalf("expected Space activation to work during Enter grace, got %d presses", pressed)
	}

	desktop.handleClick(tui.ClickEvent{X: 11, Y: 6, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: 11, Y: 6, Button: tui.MouseLeft, Down: false})
	if pressed != 2 {
		t.Fatalf("expected mouse click activation to work during Enter grace, got %d presses", pressed)
	}

	now = layer.ArmedAt().Add(300 * time.Millisecond)
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter})
	if pressed != 3 {
		t.Fatalf("expected Enter to press button after grace elapses, got %d presses", pressed)
	}
}

func TestModalEnterGraceAppliesToFreshModalByDefault(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	desktop.SetClock(func() time.Time { return now })

	pressed := 0
	root := NewComponent(Rect{X: 10, Y: 5, W: 20, H: 5})
	button := NewButton("OK", Rect{X: 1, Y: 1, W: 10, H: 1}, func() { pressed++ })
	root.AddChild(button)
	desktop.AddLayer(NewModalLayer("modal", root))
	desktop.SetFocus(button)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter})
	if pressed != 0 {
		t.Fatalf("expected a freshly shown modal button to ignore Enter during the default grace window, got %d presses", pressed)
	}
}

func TestModalEnterGraceDoesNotSuppressEnterForFocusedInput(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	desktop.SetClock(func() time.Time { return now })
	desktop.SetEnterGrace(DefaultModalEnterGrace)

	submits := 0
	root := NewComponent(Rect{X: 10, Y: 5, W: 30, H: 8})
	input := NewTextBox("", Rect{X: 1, Y: 1, W: 20, H: 1})
	input.OnSubmit = func() { submits++ }
	root.AddChild(input)
	desktop.AddLayer(NewModalLayer("modal", root))
	desktop.SetFocus(input)

	now = now.Add(10 * time.Millisecond)
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter})
	if submits != 1 {
		t.Fatalf("expected Enter grace to target button activation only; focused modal input got %d submits", submits)
	}
}

func TestModalEnterGraceLeavesEscapeDeliverable(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	desktop.SetClock(func() time.Time { return now })
	desktop.SetEnterGrace(DefaultModalEnterGrace)

	dialog := NewDialog("Confirm", 10, 5, 30, 8)
	result := ""
	ok := NewButton("&OK", Rect{}, func() { result = "ok" })
	ok.Default = true
	cancel := NewButton("&Cancel", Rect{}, func() { result = "cancel" })
	cancel.Cancel = true
	dialog.Window.AddContent(NewButtonRow(4, 28, AlignCenter, DefaultButtonGap, ok, cancel))
	dialog.SetDefaultCancelButtons(ok, cancel)
	layer := NewModalLayer("dialog", dialog)
	desktop.AddLayer(layer)
	desktop.SetFocus(cancel)

	now = now.Add(10 * time.Millisecond)
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEscape})
	if result != "cancel" {
		t.Fatalf("expected Escape to remain deliverable during Enter grace, got %q", result)
	}
}

func TestModalEnterGraceDisabledByDefault(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	desktop.SetClock(func() time.Time { return now })

	pressed := 0
	root := NewComponent(Rect{X: 10, Y: 5, W: 20, H: 5})
	button := NewButton("OK", Rect{X: 1, Y: 1, W: 10, H: 1}, func() { pressed++ })
	root.AddChild(button)
	desktop.AddLayer(NewModalLayer("modal", root))
	desktop.SetFocus(button)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter})
	if pressed != 1 {
		t.Fatalf("expected Enter to activate button when grace is disabled, got %d presses", pressed)
	}
}

func TestRemovingBuriedModalDoesNotCorruptFocusHistoryForTopModal(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	baseRoot := NewComponent(Rect{W: 80, H: 25})
	baseInput := NewTextBox("", Rect{X: 1, Y: 1, W: 20, H: 1})
	baseRoot.AddChild(baseInput)
	desktop.AddLayer(NewFullscreenLayer("base", baseRoot))
	desktop.SetFocus(baseInput)

	outerRoot := NewComponent(Rect{X: 5, Y: 3, W: 30, H: 8})
	outerButton := NewButton("Outer", Rect{X: 1, Y: 1, W: 10, H: 1}, nil)
	outerRoot.AddChild(outerButton)
	outerLayer := NewModalLayer("outer", outerRoot)
	desktop.AddLayer(outerLayer)
	desktop.SetFocus(outerButton)

	middleRoot := NewComponent(Rect{X: 8, Y: 5, W: 30, H: 8})
	middleButton := NewButton("Middle", Rect{X: 1, Y: 1, W: 10, H: 1}, nil)
	middleRoot.AddChild(middleButton)
	middleLayer := NewModalLayer("middle", middleRoot)
	desktop.AddLayer(middleLayer)
	desktop.SetFocus(middleButton)

	innerRoot := NewComponent(Rect{X: 11, Y: 7, W: 30, H: 8})
	innerButton := NewButton("Inner", Rect{X: 1, Y: 1, W: 10, H: 1}, nil)
	innerRoot.AddChild(innerButton)
	innerLayer := NewModalLayer("inner", innerRoot)
	desktop.AddLayer(innerLayer)
	desktop.SetFocus(innerButton)

	desktop.RemoveLayer(outerLayer)
	if !innerButton.Component.Focused() {
		t.Fatalf("expected removing a buried modal to leave top modal focus intact")
	}

	desktop.RemoveLayer(innerLayer)
	if !middleButton.Component.Focused() {
		t.Fatalf("expected closing top modal to restore focus to the still-present middle modal button")
	}
}

func TestRecentlyTypedTracksConsumedTextInputAndDecays(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	desktop.SetClock(func() time.Time { return now })

	root := NewComponent(Rect{W: 80, H: 25})
	input := NewTextBox("", Rect{X: 1, Y: 1, W: 20, H: 1})
	root.AddChild(input)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.SetFocus(input)

	if desktop.RecentlyTyped(time.Second) {
		t.Fatalf("expected RecentlyTyped to be false before any text input")
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'})
	if !desktop.RecentlyTyped(time.Second) {
		t.Fatalf("expected rune input consumed by focused textbox to count as recent typing")
	}

	now = now.Add(999 * time.Millisecond)
	if !desktop.RecentlyTyped(time.Second) {
		t.Fatalf("expected typing signal to remain true inside query window")
	}

	now = now.Add(time.Millisecond)
	if desktop.RecentlyTyped(time.Second) {
		t.Fatalf("expected typing signal to decay once query window elapses")
	}
}

func TestRecentlyTypedIgnoresNavigationShortcutsEnterAndUnhandledKeys(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	desktop.SetClock(func() time.Time { return now })

	root := NewComponent(Rect{W: 80, H: 25})
	input := NewTextBox("", Rect{X: 1, Y: 1, W: 20, H: 1})
	submits := 0
	input.OnSubmit = func() { submits++ }
	root.AddChild(input)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.SetFocus(input)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyLeft})
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a', Ctrl: true})
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter})
	desktop.handleType(tui.TypeEvent{Key: tui.KeyF1})

	if submits != 1 {
		t.Fatalf("expected Enter to submit once, got %d submits", submits)
	}
	if desktop.RecentlyTyped(time.Second) {
		t.Fatalf("expected navigation, shortcuts, Enter, and unhandled keys not to count as recent typing")
	}
}

func TestRecentlyTypedTracksPasteAndDelete(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	desktop.SetClock(func() time.Time { return now })

	root := NewComponent(Rect{W: 80, H: 25})
	input := NewTextBox("abc", Rect{X: 1, Y: 1, W: 20, H: 1})
	root.AddChild(input)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.SetFocus(input)

	desktop.handlePaste(tui.PasteEvent{Text: "x"})
	if !desktop.RecentlyTyped(time.Second) {
		t.Fatalf("expected consumed paste to count as recent typing")
	}

	now = now.Add(2 * time.Second)
	if desktop.RecentlyTyped(time.Second) {
		t.Fatalf("expected paste typing signal to decay")
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyBackspace})
	if !desktop.RecentlyTyped(time.Second) {
		t.Fatalf("expected consumed backspace/delete editing key to count as recent typing")
	}
}

func TestRecentlyTypedIgnoresButtonActivationSpace(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	desktop.SetClock(func() time.Time { return now })

	root := NewComponent(Rect{W: 80, H: 25})
	pressed := 0
	button := NewButton("OK", Rect{X: 1, Y: 1, W: 10, H: 1}, func() { pressed++ })
	root.AddChild(button)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.SetFocus(button)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: ' '})
	if pressed != 1 {
		t.Fatalf("expected Space to activate focused button once, got %d presses", pressed)
	}
	if desktop.RecentlyTyped(time.Second) {
		t.Fatalf("expected button Space activation not to be reported as recent typing into an input")
	}
}
