package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestMenuShortcutChordBridgesAllFields(t *testing.T) {
	sc := &MenuShortcut{Display: "Ctrl+Shift+Alt+S", Key: tui.KeyRune, Rune: 's', Ctrl: true, Shift: true, Alt: true}
	c := sc.Chord()
	want := Chord{Key: tui.KeyRune, Rune: 's', Ctrl: true, Shift: true, Alt: true}
	if c != want {
		t.Fatalf("Chord() = %+v, want %+v", c, want)
	}
	if sc.Display != "Ctrl+Shift+Alt+S" {
		t.Fatal("Chord() must not disturb the Display hint")
	}
}

func TestNilMenuShortcutChordIsZero(t *testing.T) {
	var sc *MenuShortcut
	if sc.Chord() != (Chord{}) {
		t.Fatal("a nil MenuShortcut must yield the zero Chord")
	}
}

func TestMatchShortcutDelegatesToChord(t *testing.T) {
	sc := &MenuShortcut{Key: tui.KeyRune, Rune: 'n', Ctrl: true}
	cases := []tui.TypeEvent{
		{Key: tui.KeyRune, Rune: 'n', Ctrl: true},
		{Key: tui.KeyRune, Rune: 'N', Ctrl: true},
		{Key: tui.KeyRune, Rune: 'n'},
		{Key: tui.KeyRune, Rune: 'w', Ctrl: true},
		{Key: tui.KeyF1},
	}
	for _, e := range cases {
		if matchShortcut(e, sc) != sc.Chord().Matches(e) {
			t.Fatalf("matchShortcut and Chord.Matches disagree for %+v", e)
		}
	}
	if matchShortcut(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'}, nil) {
		t.Fatal("matchShortcut(nil) must be false")
	}
}

func TestWithActionIDTagsItemAndDefaultsEmpty(t *testing.T) {
	plain := NewMenuItem("New", nil)
	if plain.ActionID != "" {
		t.Fatalf("default ActionID = %q, want empty", plain.ActionID)
	}
	tagged := NewMenuItem("New", nil).WithActionID("file.new")
	if tagged.ActionID != "file.new" {
		t.Fatalf("WithActionID set %q, want %q", tagged.ActionID, "file.new")
	}
	chained := NewMenuItem("New", nil).
		WithShortcut("Ctrl+N", tui.KeyRune, 'n', true).
		WithActionID("file.new")
	if chained.Shortcut == nil || chained.Shortcut.Display != "Ctrl+N" || chained.ActionID != "file.new" {
		t.Fatalf("WithActionID must compose with WithShortcut: %#v / %q", chained.Shortcut, chained.ActionID)
	}
}

func TestMenuBarDoesNotRegisterShortcutsFromMenuTree(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	fired := 0
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File",
			NewMenuItem("New", func() { fired++ }).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true),
			NewSubMenu("More",
				NewMenuItem("Nested", func() { fired++ }).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true),
			),
		),
	)
	desktop.SetMenuBar(bar)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))

	desktop.handleType(ctrlN())
	if fired != 0 {
		t.Fatalf("menu-owned shortcuts must not self-register into accelerator dispatch, fired=%d", fired)
	}
	if got := desktop.Bindings().Len(); got != 0 {
		t.Fatalf("menu tree should not add registry entries, Len=%d", got)
	}
}

func TestDesktopUnifiedRegistryResolvesAcceleratorAndKeepsMenuDisplay(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	item := NewMenuItem("New", nil).
		WithShortcut("Ctrl+N", tui.KeyRune, 'n', true).
		WithActionID("file.new")
	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File", item)))
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))

	fired := 0
	desktop.Bindings().Register(
		KeyBinding{Chord: item.Shortcut.Chord(), ActionID: item.ActionID, Scope: ScopeGlobal},
		func() bool { fired++; return true },
	)

	got, ok := desktop.Bindings().Match(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true})
	if !ok || got.ActionID != "file.new" {
		t.Fatalf("desktop registry Match = %q ok=%v, want file.new", got.ActionID, ok)
	}
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true})
	if fired != 1 {
		t.Fatalf("desktop registry accelerator fired %d times, want 1", fired)
	}
	if item.Shortcut == nil || item.Shortcut.Display != "Ctrl+N" {
		t.Fatal("registry dispatch must not strip the menu display shortcut")
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'x', Ctrl: true})
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n'})
	if fired != 1 {
		t.Fatalf("only the registered Ctrl+N should fire; count=%d", fired)
	}
}

func TestDesktopDispatchClosesOpenMenuAfterGlobalBinding(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File"))
	desktop.SetMenuBar(bar)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))
	desktop.Bindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}, Scope: ScopeGlobal},
		func() bool { return true },
	)

	bar.openPath = []int{0}
	bar.hoverPath = []int{0, 0}
	desktop.handleType(ctrlN())
	if len(bar.openPath) != 0 || len(bar.hoverPath) != 0 {
		t.Fatalf("desktop global dispatch must reset openPath/hoverPath, got %v / %v", bar.openPath, bar.hoverPath)
	}
}

func TestDisabledGlobalBindingIsSkippedNotSwallowed(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File")))
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))

	enabledFired := 0
	desktop.Bindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}, ActionID: "disabled", Scope: ScopeGlobal},
		func() bool { return false },
	)
	desktop.Bindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}, ActionID: "enabled", Scope: ScopeGlobal},
		func() bool { enabledFired++; return true },
	)

	desktop.handleType(ctrlN())
	if enabledFired != 1 {
		t.Fatalf("later live duplicate should fire after a disabled global binding, fired=%d", enabledFired)
	}
}

func TestLoneDisabledGlobalBindingFallsThrough(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File")))
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))

	unhandled := 0
	desktop.Bindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}, ActionID: "disabled", Scope: ScopeGlobal},
		func() bool { return false },
	)
	desktop.SetUnhandledKeyFn(func(event tui.TypeEvent) {
		if event.Key == tui.KeyRune && event.Rune == 'n' && event.Ctrl {
			unhandled++
		}
	})

	desktop.handleType(ctrlN())
	if unhandled != 1 {
		t.Fatalf("disabled global binding should not swallow the chord; unhandled=%d", unhandled)
	}
}

func TestFunctionKeyAcceleratorThroughDesktopRegistry(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&Help", NewMenuItem("Help", nil).WithShortcutMod("F1", tui.KeyF1, 0, false, false, false)),
	))
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})))

	fired := 0
	desktop.Bindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyF1}, ActionID: "help", Scope: ScopeGlobal},
		func() bool { fired++; return true },
	)

	if got, ok := desktop.Bindings().Match(tui.TypeEvent{Key: tui.KeyF1}); !ok || got.Chord.Key != tui.KeyF1 {
		t.Fatalf("F1 must resolve in the desktop registry: %+v ok=%v", got, ok)
	}
	desktop.handleType(tui.TypeEvent{Key: tui.KeyF1})
	if fired != 1 {
		t.Fatalf("F1 accelerator must fire once through desktop registry, fired=%d", fired)
	}
}
