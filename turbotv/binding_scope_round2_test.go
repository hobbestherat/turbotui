package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Phase-2 round-2 tests pinning the "Match/Dispatch are Global-only" hardening.
// The menu-accelerator path (HandleAccelerator -> Registry().Dispatch) and the pure
// Match lookup must surface ONLY ScopeGlobal bindings, so a Focus/Fallthrough binding
// accidentally registered into the menu (Global) registry is inert there instead of
// firing as a global accelerator — which would bypass focus-scoping and modal
// blocking. These tests fail if the filter is reverted to scope-agnostic.
// Helpers ev/ctrl (binding_test.go), focusComp (binding_scope_test.go),
// newTestDesktop (menu_mnemonic_test.go) are reused.

// --- Match is Global-only. ---

func TestMatchIgnoresScopedBindings(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	// Only Focus and Fallthrough bindings exist on this chord — no Global.
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "focus", Scope: ScopeFocus, Target: window}, nil)
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "fall", Scope: ScopeFallthrough}, nil)

	if got, ok := r.Match(ev('r')); ok {
		t.Fatalf("Match must not surface a scoped binding; got %q", got.ActionID)
	}
}

// --- Dispatch is Global-only, including across the liveness fall-through. ---

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
	// Adversarial: a not-live Global binding registered before a live Focus binding on
	// the same chord. Under the OLD scope-agnostic Dispatch the not-live Global would
	// be skipped and the live Focus binding would fire. Under the fixed Global-only
	// Dispatch the Focus binding is never even a candidate, so Dispatch returns false
	// and the Focus handler does NOT fire.
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	globalTried, focusFired := 0, 0
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "g", Scope: ScopeGlobal},
		func() bool { globalTried++; return false }) // not live
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "f", Scope: ScopeFocus, Target: window},
		func() bool { focusFired++; return true })

	if r.Dispatch(ev('r')) {
		t.Fatal("Dispatch must return false: the only live binding is Focus-scoped, not Global")
	}
	if globalTried != 1 {
		t.Fatalf("the not-live Global binding should have been tried once, got %d", globalTried)
	}
	if focusFired != 0 {
		t.Fatalf("Dispatch must NOT fall through a not-live Global to a live Focus binding (fired=%d)", focusFired)
	}
}

// --- Integration: a scoped binding in the MENU registry can't fire as accelerator. ---

func TestHandleAcceleratorIgnoresScopedBindingInMenuRegistry(t *testing.T) {
	// Register a Fallthrough binding directly into the menu's (Global) registry. The
	// accelerator path must NOT fire it — otherwise a Fallthrough/Focus binding placed
	// in the wrong registry would run before the focused widget and bypass modal
	// blocking (the very footgun the Global-only filter closes).
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	fired := 0
	bar.Registry().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'z', Ctrl: true}, ActionID: "misplaced", Scope: ScopeFallthrough},
		func() bool { fired++; return true },
	)
	// The real Global accelerator still works...
	if !bar.HandleAccelerator(ctrl('n')) {
		t.Fatal("the genuine Global Ctrl+N accelerator must still fire")
	}
	// ...but the misplaced Fallthrough binding is inert through the accelerator path.
	if bar.HandleAccelerator(ctrl('z')) {
		t.Fatal("a Fallthrough binding in the menu registry must NOT fire as a global accelerator")
	}
	if fired != 0 {
		t.Fatalf("the misplaced scoped handler must never run via HandleAccelerator (fired=%d)", fired)
	}
}

func TestDesktopAcceleratorIgnoresScopedBindingInMenuRegistry(t *testing.T) {
	// Same hardening through the full Desktop dispatch: a Focus binding wrongly placed
	// in the menu registry does not fire at the menu-accelerator stage.
	desktop := newTestDesktop(t, 40, 12)
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	desktop.SetMenuBar(bar)
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
		t.Fatalf("a Focus binding in the menu registry must not fire at the accelerator stage (fired=%d)", fired)
	}
}

// --- ScopedBindings durability across a menu rebuild (critique #3). ---

func TestScopedBindingsSurviveMenuRebuild(t *testing.T) {
	// The desktop's ScopedBindings registry is independent of the menu registry, so a
	// menu RebuildBindings (which Clear()s and repopulates the menu registry) must NOT
	// drop a Focus/Fallthrough binding registered on the desktop. This pins the doc'd
	// "ScopedBindings survives a menu rebuild" promise — currently true only because
	// nothing touches d.bindings, so a future regression that reset it is caught here.
	typed := 0
	desktop := newTestDesktop(t, 40, 12)
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	desktop.SetMenuBar(bar)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 11})
	inner := focusComp(false, &typed) // declines keys
	window.AddChild(inner)
	root.AddChild(window)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.setFocus(inner)

	fired := 0
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window},
		func() bool { fired++; return true },
	)

	// Structurally rebuild the menu bindings (drops extras in the MENU registry only).
	bar.RebuildBindings()

	desktop.handleType(ev('r'))
	if fired != 1 {
		t.Fatalf("a desktop ScopedBindings Focus binding must survive a menu RebuildBindings (fired=%d)", fired)
	}
}

func TestMenuRegistryExtraBindingDroppedByRebuild(t *testing.T) {
	// The guardrail contrast: an extra binding Registered directly into the MENU
	// registry IS dropped by RebuildBindings (documented). This pins the boundary so
	// the durable-vs-ephemeral distinction between ScopedBindings and the menu
	// registry stays a conscious contract.
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	bar.Registry().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'b', Ctrl: true}, ActionID: "extra"}, // Global
		func() bool { return true },
	)
	if _, ok := bar.Registry().Match(ctrl('b')); !ok {
		t.Fatal("precondition: the extra Global binding should resolve before a rebuild")
	}
	bar.RebuildBindings()
	if _, ok := bar.Registry().Match(ctrl('b')); ok {
		t.Fatal("RebuildBindings must drop extra bindings registered into the menu registry")
	}
	// The genuine menu accelerator survives the rebuild.
	if _, ok := bar.Registry().Match(ctrl('n')); !ok {
		t.Fatal("the menu's own Ctrl+N binding must be present after a rebuild")
	}
}
