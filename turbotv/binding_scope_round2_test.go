package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestMatchIgnoresScopedBindings(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "focus", Scope: ScopeFocus, Target: window}, nil)
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "fall", Scope: ScopeFallthrough}, nil)

	if got, ok := r.Match(ev('r')); ok {
		t.Fatalf("Match must not surface a scoped binding; got %q", got.ActionID)
	}
}

func TestDispatchIgnoresScopedBindings(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	focusFired, fallFired := 0, 0
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window},
		func() bool { focusFired++; return true })
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFallthrough},
		func() bool { fallFired++; return true })

	if r.Dispatch(ev('r')) {
		t.Fatal("Dispatch (Global-only) must not consume a chord that only scoped bindings hold")
	}
	if focusFired != 0 || fallFired != 0 {
		t.Fatalf("Dispatch must not fire scoped handlers (focus=%d fall=%d)", focusFired, fallFired)
	}
}

func TestDispatchNotLiveGlobalDoesNotFallToScoped(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	globalTried, focusFired := 0, 0
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "g", Scope: ScopeGlobal},
		func() bool { globalTried++; return false })
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "f", Scope: ScopeFocus, Target: window},
		func() bool { focusFired++; return true })

	if r.Dispatch(ev('r')) {
		t.Fatal("Dispatch must return false: the only live binding is Focus-scoped, not Global")
	}
	if globalTried != 1 {
		t.Fatalf("the not-live Global binding should have been tried once, got %d", globalTried)
	}
	if focusFired != 0 {
		t.Fatalf("Dispatch must not fall through a not-live Global to a live Focus binding (fired=%d)", focusFired)
	}
}

func TestSingleRegistryDispatchesGlobalFocusAndFallthroughAtCorrectStages(t *testing.T) {
	r := NewBindingRegistry()
	target := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	focused := NewComponent(Rect{X: 1, Y: 1, W: 10, H: 1})
	outside := NewComponent(Rect{X: 1, Y: 2, W: 10, H: 1})
	target.AddChild(focused)

	counts := map[string]int{}
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'g', Ctrl: true}, ActionID: "global", Scope: ScopeGlobal},
		func() bool { counts["global"]++; return true })
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'f'}, ActionID: "focus", Scope: ScopeFocus, Target: target},
		func() bool { counts["focus"]++; return true })
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 't'}, ActionID: "fallthrough", Scope: ScopeFallthrough},
		func() bool { counts["fallthrough"]++; return true })

	if !r.Dispatch(ctrl('g')) {
		t.Fatal("Dispatch should consume the Global binding")
	}
	if r.Dispatch(ev('f')) || r.Dispatch(ev('t')) {
		t.Fatal("Dispatch must ignore Focus and Fallthrough bindings in the same registry")
	}
	if !r.DispatchFocus(ev('f'), focused) {
		t.Fatal("DispatchFocus should consume when focused is within Target")
	}
	if r.DispatchFocus(ev('f'), outside) {
		t.Fatal("DispatchFocus must ignore Focus bindings when focus is outside Target")
	}
	if r.DispatchFocus(ctrl('g'), focused) || r.DispatchFocus(ev('t'), focused) {
		t.Fatal("DispatchFocus must ignore Global and Fallthrough bindings in the same registry")
	}
	if !r.DispatchFallthrough(ev('t')) {
		t.Fatal("DispatchFallthrough should consume the Fallthrough binding")
	}
	if r.DispatchFallthrough(ctrl('g')) || r.DispatchFallthrough(ev('f')) {
		t.Fatal("DispatchFallthrough must ignore Global and Focus bindings in the same registry")
	}

	if counts["global"] != 1 || counts["focus"] != 1 || counts["fallthrough"] != 1 {
		t.Fatalf("unexpected dispatch counts: %#v", counts)
	}
}

func TestDesktopAcceleratorIgnoresScopedBindingInUnifiedRegistry(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File")))
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	desktop.AddLayer(NewFullscreenLayer("base", root))

	fired := 0
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	desktop.Bindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'q', Ctrl: true}, Scope: ScopeFocus, Target: window},
		func() bool { fired++; return true },
	)
	desktop.handleType(ctrl('q'))
	if fired != 0 {
		t.Fatalf("a Focus binding in the unified registry must not fire at the accelerator stage (fired=%d)", fired)
	}
}

func TestFocusBindingInUnifiedRegistrySurvivesMenuReplacement(t *testing.T) {
	typed := 0
	desktop := newTestDesktop(t, 40, 12)
	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File")))
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 11})
	inner := focusComp(false, &typed)
	window.AddChild(inner)
	root.AddChild(window)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.setFocus(inner)

	fired := 0
	reg := desktop.ScopedBindings()
	reg.Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window},
		func() bool { fired++; return true },
	)

	desktop.SetMenuBar(NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("Replacement", nil).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	))

	if desktop.Bindings() != reg {
		t.Fatal("menu replacement must not replace the unified registry")
	}
	desktop.handleType(ev('r'))
	if fired != 1 {
		t.Fatalf("Focus binding should survive menu replacement, fired=%d", fired)
	}
	if typed != 1 {
		t.Fatalf("focused widget should see the key before Focus binding dispatch, typed=%d", typed)
	}
}
