package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func altRune(r rune) tui.TypeEvent {
	return tui.TypeEvent{Key: tui.KeyRune, Rune: r, Alt: true}
}

func newTestDesktop(t *testing.T, w int, h int) *Desktop {
	t.Helper()
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	return NewDesktop(app)
}

func TestAltMnemonicPressesButton(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	pressed := 0
	button := NewButton("Canc&el", Rect{X: 2, Y: 2, W: 12, H: 1}, func() { pressed++ })
	root.AddChild(button)
	desktop.AddLayer(NewWindowLayer("win", root))
	desktop.compose() // populate mnemonicActive

	desktop.handleType(altRune('e'))
	if pressed != 1 {
		t.Fatalf("expected Alt+e to press the button once, got %d", pressed)
	}
}

func TestAltMnemonicFocusesLabelTarget(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	label := NewLabel("&Name", Rect{X: 2, Y: 2, W: 10, H: 1})
	field := NewTextBox("", Rect{X: 2, Y: 3, W: 20, H: 1})
	label.SetTarget(field)
	root.AddChild(label)
	root.AddChild(field)
	desktop.AddLayer(NewWindowLayer("win", root))
	desktop.compose()

	desktop.handleType(altRune('n'))
	if !field.Component.Focused() {
		t.Fatalf("expected Alt+n to focus the labelled field")
	}
}

func TestMnemonicClashFirstWins(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	firstHits := 0
	secondHits := 0
	first := NewButton("&Save", Rect{X: 2, Y: 2, W: 12, H: 1}, func() { firstHits++ })
	second := NewButton("&Send", Rect{X: 2, Y: 4, W: 12, H: 1}, func() { secondHits++ })
	root.AddChild(first)
	root.AddChild(second)
	desktop.AddLayer(NewWindowLayer("win", root))
	desktop.compose()

	if !first.Component.MnemonicActive() || second.Component.MnemonicActive() {
		t.Fatalf("expected only the first 's' mnemonic to be active")
	}
	desktop.handleType(altRune('s'))
	if firstHits != 1 || secondHits != 0 {
		t.Fatalf("expected only the first button to fire, got first=%d second=%d", firstHits, secondHits)
	}
}

func TestModalLayerBlocksMenuMnemonic(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	opened := 0
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", func() { opened++ })),
	)
	desktop.SetMenuBar(menu)
	base := NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	// With only the base layer, Alt+f opens the File menu.
	desktop.handleType(altRune('f'))
	if !menu.IsOpen() {
		t.Fatalf("expected Alt+f to open the File menu in base layer")
	}
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEscape})

	// A modal dialog on top must block the menubar's mnemonics.
	dialog := NewComponent(Rect{X: 5, Y: 5, W: 20, H: 5})
	desktop.AddLayer(NewModalLayer("modal", dialog))
	desktop.handleType(altRune('f'))
	if menu.IsOpen() {
		t.Fatalf("expected modal layer to block the menubar Alt+f")
	}
}

func TestNonModalWindowKeepsMenuMnemonic(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	opened := 0
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", func() { opened++ })),
	)
	desktop.SetMenuBar(menu)
	base := NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	// A non-modal window on top must NOT block the menubar's mnemonics.
	window := NewComponent(Rect{X: 4, Y: 3, W: 30, H: 8})
	desktop.AddLayer(NewWindowLayer("window", window))
	desktop.handleType(altRune('f'))
	if !menu.IsOpen() {
		t.Fatalf("expected Alt+f to open the File menu through a non-modal window")
	}
}

func TestOpenMenuLetterSelectsItem(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	found := 0
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File", NewMenuItem("&Find", func() { found++ })),
	)
	desktop.SetMenuBar(menu)
	base := NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	desktop.handleType(altRune('f')) // open File
	if !menu.IsOpen() {
		t.Fatalf("expected File menu to open")
	}
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'f'}) // select Find by letter
	if found != 1 {
		t.Fatalf("expected plain 'f' to select Find once, got %d", found)
	}
	if menu.IsOpen() {
		t.Fatalf("expected menu to close after selecting a leaf item")
	}
}
