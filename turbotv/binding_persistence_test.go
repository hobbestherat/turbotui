package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func ctrlN() tui.TypeEvent { return tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true} }
func ctrlM() tui.TypeEvent { return tui.TypeEvent{Key: tui.KeyRune, Rune: 'm', Ctrl: true} }

func TestDesktopBindingsAndScopedBindingsAreOneRegistry(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)

	bindings := desktop.Bindings()
	scoped := desktop.ScopedBindings()
	if bindings == nil {
		t.Fatal("Desktop.Bindings() must create and return the unified registry")
	}
	if bindings != scoped {
		t.Fatal("Desktop.Bindings() and Desktop.ScopedBindings() must return the same registry instance")
	}

	bindings.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}, ActionID: "file.new"}, nil)
	if got, ok := scoped.Match(ctrlN()); !ok || got.ActionID != "file.new" {
		t.Fatalf("registration through Bindings must be visible through ScopedBindings: got %q ok=%v", got.ActionID, ok)
	}
}

func TestMenuShortcutIsDisplayOnlyAndDoesNotRegisterAccelerator(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	fired := 0
	item := NewMenuItem("New", func() { fired++ }).
		WithShortcut("Ctrl+N", tui.KeyRune, 'n', true).
		WithActionID("file.new")
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File", item))
	desktop.SetMenuBar(bar)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))

	if item.Shortcut == nil || item.Shortcut.Display != "Ctrl+N" || item.ActionID != "file.new" {
		t.Fatal("menu item should retain display metadata")
	}
	desktop.handleType(ctrlN())
	if fired != 0 {
		t.Fatalf("menu Shortcut must be display-only; OnSelect fired %d times without a desktop binding", fired)
	}
	if _, ok := desktop.Bindings().Match(ctrlN()); ok {
		t.Fatal("setting a menu Shortcut must not register a desktop binding")
	}
}

func TestDesktopGlobalBindingDispatchesThroughUnifiedRegistry(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File")))
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))

	fired := 0
	desktop.Bindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}, ActionID: "file.new", Scope: ScopeGlobal},
		func() bool { fired++; return true },
	)

	desktop.handleType(ctrlN())
	if fired != 1 {
		t.Fatalf("global binding should fire through Desktop.handleType once, got %d", fired)
	}
}

func TestDesktopGlobalBindingSurvivesMenuRebuildReplacement(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", nil).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	))
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))

	fired := 0
	reg := desktop.Bindings()
	reg.Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'm', Ctrl: true}, ActionID: "app.commandPalette", Scope: ScopeGlobal},
		func() bool { fired++; return true },
	)

	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File",
			NewMenuItem("Open", nil).WithShortcut("Ctrl+O", tui.KeyRune, 'o', true),
			NewMenuItem("Command Palette", nil).WithShortcut("Ctrl+M", tui.KeyRune, 'm', true),
		),
	))

	if desktop.Bindings() != reg {
		t.Fatal("replacing/rebuilding the menu must not replace the desktop binding registry")
	}
	desktop.handleType(ctrlM())
	if fired != 1 {
		t.Fatalf("desktop-registered global must survive a menu rebuild/replacement, fired=%d", fired)
	}
}

func TestDesktopModalLayerBlocksUnifiedGlobalBinding(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File")))
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))

	fired := 0
	desktop.Bindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}, ActionID: "file.new", Scope: ScopeGlobal},
		func() bool { fired++; return true },
	)

	desktop.handleType(ctrlN())
	if fired != 1 {
		t.Fatalf("precondition: global should fire with no modal, got %d", fired)
	}

	desktop.AddLayer(NewModalLayer("modal", NewComponent(Rect{X: 5, Y: 5, W: 20, H: 5})))
	desktop.handleType(ctrlN())
	if fired != 1 {
		t.Fatalf("modal layer must gate unified global dispatch, fired=%d", fired)
	}
}
