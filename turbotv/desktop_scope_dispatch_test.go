package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Phase-2 dispatch tests: Desktop.handleType consults the desktop's ScopedBindings
// registry at two existing positions — ScopeFocus at the focused-widget stage (after
// the focused widget declines the key) and ScopeFallthrough at the unhandledKeyFn
// stage. The HARD constraint is behavior preservation: with NO scoped bindings
// registered the chain is identical to before. These tests prove both the new wiring
// fires at the right place AND that the unchanged-by-default invariant holds.
// Helpers ev/ctrl (binding_test.go), focusComp (binding_scope_test.go) and
// newTestDesktop (menu_mnemonic_test.go) are reused.

// scopedDesktop builds a desktop with a fullscreen base layer, a focusable inner
// widget nested under a `window` container, and the inner widget focused. The inner
// widget DECLINES every key (so dispatch proceeds past the focused-widget stage),
// recording calls in *typed. It returns the desktop and the window container, which
// is the natural ScopeFocus Target.
func scopedDesktop(t *testing.T, typed *int) (*Desktop, *VisualComponent) {
	t.Helper()
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 11})
	inner := focusComp(false, typed) // declines all keys
	window.AddChild(inner)
	root.AddChild(window)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.setFocus(inner)
	return desktop, window
}

// --- ScopedBindings accessor: lazy, stable, distinct from the menu registry. ---

func TestScopedBindingsIsLazyAndStable(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	a := desktop.ScopedBindings()
	if a == nil {
		t.Fatal("ScopedBindings() must never be nil")
	}
	if b := desktop.ScopedBindings(); b != a {
		t.Fatal("ScopedBindings() must return the same registry on every call")
	}
}

func TestScopedBindingsDistinctFromMenuBindings(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	desktop.SetMenuBar(bar)
	menuReg := desktop.Bindings()
	scopedReg := desktop.ScopedBindings()
	if menuReg == scopedReg {
		t.Fatal("the scoped (Focus/Fallthrough) registry must be distinct from the menu (Global) registry")
	}
	// Registering into the scoped registry must not affect the menu registry.
	scopedReg.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, Scope: ScopeFallthrough}, nil)
	if menuReg.Len() != 1 {
		t.Fatalf("menu registry Len = %d, want 1 (scoped registration must not leak)", menuReg.Len())
	}
}

// --- Behavior preservation: with no scoped bindings the chain is unchanged. ---

func TestNoScopedBindingsDispatchUnchanged(t *testing.T) {
	// Never touching ScopedBindings leaves d.bindings nil, so the two new checks are
	// skipped entirely. Ctrl+C with no handler must still reach the default quit.
	desktop := newTestDesktop(t, 40, 12)
	quit := 0
	desktop.cancel = func() { quit++ }
	desktop.handleType(ctrl('c'))
	if quit != 1 {
		t.Fatalf("with no scoped bindings, default Ctrl+C quit must fire once, got %d", quit)
	}
}

func TestEmptyScopedRegistryDispatchUnchanged(t *testing.T) {
	// Even after ScopedBindings() makes d.bindings non-nil, an EMPTY registry returns
	// false at both checks, so dispatch is still identical. This guards the subtle
	// "the guard is `!= nil`, not `Len() > 0`" path.
	desktop := newTestDesktop(t, 40, 12)
	quit := 0
	desktop.cancel = func() { quit++ }
	_ = desktop.ScopedBindings() // create the empty registry

	desktop.handleType(ctrl('c'))
	if quit != 1 {
		t.Fatalf("an empty scoped registry must not change the Ctrl+C quit path, quit=%d", quit)
	}
}

func TestEmptyScopedRegistryFocusedWidgetUnchanged(t *testing.T) {
	// A focused widget that consumes the key still wins; the empty Focus check after
	// it is never reached because BubbleType already returned.
	typed := 0
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	inner := focusComp(true, &typed) // consumes every key
	root.AddChild(inner)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.setFocus(inner)
	_ = desktop.ScopedBindings()

	desktop.handleType(ev('r'))
	if typed != 1 {
		t.Fatalf("focused widget must still receive the key (typed=%d)", typed)
	}
}

func TestEmptyScopedRegistryFocusNavUnchanged(t *testing.T) {
	// Tab focus navigation still runs after the (empty) Focus check. Two focusable
	// widgets; the focused widget declines Tab, so the desktop moves focus.
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	first := focusComp(false, nil)
	first.Bounds = Rect{X: 0, Y: 0, W: 10, H: 1}
	second := focusComp(false, nil)
	second.Bounds = Rect{X: 0, Y: 2, W: 10, H: 1}
	root.AddChild(first)
	root.AddChild(second)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.setFocus(first)
	_ = desktop.ScopedBindings()

	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
	if !second.Focused() || first.Focused() {
		t.Fatal("Tab must still move focus to the next widget with an empty scoped registry")
	}
}

// --- ScopeFocus dispatch wiring. ---

func TestFocusBindingFiresWhenTargetFocused(t *testing.T) {
	typed := 0
	desktop, window := scopedDesktop(t, &typed)
	fired := 0
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "focus.r", Scope: ScopeFocus, Target: window},
		func() bool { fired++; return true },
	)

	desktop.handleType(ev('r'))
	if typed != 1 {
		t.Fatalf("the focused widget must be offered the key first (typed=%d)", typed)
	}
	if fired != 1 {
		t.Fatalf("Focus binding must fire after the focused widget declines (fired=%d)", fired)
	}
}

func TestFocusBindingSkippedWhenFocusOutsideTarget(t *testing.T) {
	// The scoping guarantee at the dispatch level: a Focus binding for `window` must
	// NOT fire when focus is on a widget outside that window.
	typed := 0
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	window := NewComponent(Rect{X: 0, Y: 0, W: 20, H: 11})
	outside := focusComp(false, &typed) // focused, but NOT under window
	root.AddChild(window)
	root.AddChild(outside)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.setFocus(outside)

	fired := 0
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window},
		func() bool { fired++; return true },
	)

	desktop.handleType(ev('r'))
	if fired != 0 {
		t.Fatalf("Focus binding must NOT fire when focus is outside its Target (fired=%d)", fired)
	}
}

func TestFocusBindingNotReachedWhenWidgetConsumes(t *testing.T) {
	// Ordering: the focused widget runs BEFORE the Focus binding. A widget that
	// consumes 'r' preempts the Focus binding entirely.
	typed := 0
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 11})
	inner := focusComp(true, &typed) // CONSUMES every key
	window.AddChild(inner)
	root.AddChild(window)
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.setFocus(inner)

	fired := 0
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window},
		func() bool { fired++; return true },
	)

	desktop.handleType(ev('r'))
	if typed != 1 {
		t.Fatalf("focused widget should have consumed the key (typed=%d)", typed)
	}
	if fired != 0 {
		t.Fatalf("Focus binding must not fire once the focused widget consumes the key (fired=%d)", fired)
	}
}

func TestFocusBindingNotLiveFallsThrough(t *testing.T) {
	// A not-live Focus binding (handler returns false) must not consume the key: the
	// chain continues. With 'r' otherwise unhandled and no quit meaning, it simply
	// ends — but a Fallthrough binding on the same chord should then get it, proving
	// the Focus stage did not swallow it.
	typed := 0
	desktop, window := scopedDesktop(t, &typed)
	focusTried, fallFired := 0, 0
	reg := desktop.ScopedBindings()
	reg.Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window},
		func() bool { focusTried++; return false },
	)
	reg.Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFallthrough},
		func() bool { fallFired++; return true },
	)

	desktop.handleType(ev('r'))
	if focusTried != 1 {
		t.Fatalf("the not-live Focus binding should have been tried once (got %d)", focusTried)
	}
	if fallFired != 1 {
		t.Fatalf("a declined Focus binding must let the key reach the Fallthrough stage (fall=%d)", fallFired)
	}
}

// --- ScopeFallthrough dispatch wiring. ---

func TestFallthroughBindingFiresAtUnhandledStage(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	desktop.AddLayer(NewFullscreenLayer("base", root))
	fired := 0
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, ActionID: "help", Scope: ScopeFallthrough},
		func() bool { fired++; return true },
	)

	desktop.handleType(ev('?'))
	if fired != 1 {
		t.Fatalf("Fallthrough binding must fire at the unhandled-key stage (fired=%d)", fired)
	}
}

func TestFallthroughBindingRunsBeforeUnhandledKeyFn(t *testing.T) {
	// Ordering: the Fallthrough registry runs BEFORE the app's unhandledKeyFn. A
	// matched Fallthrough binding consumes the key, so unhandledKeyFn never sees it;
	// an unmatched key still reaches unhandledKeyFn.
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	desktop.AddLayer(NewFullscreenLayer("base", root))
	unhandled := 0
	desktop.SetUnhandledKeyFn(func(tui.TypeEvent) { unhandled++ })
	fallFired := 0
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, Scope: ScopeFallthrough},
		func() bool { fallFired++; return true },
	)

	desktop.handleType(ev('?'))
	if fallFired != 1 || unhandled != 0 {
		t.Fatalf("Fallthrough must claim '?' before unhandledKeyFn (fall=%d unhandled=%d)", fallFired, unhandled)
	}
	// A key with no Fallthrough binding still reaches the app handler.
	desktop.handleType(ev('x'))
	if unhandled != 1 {
		t.Fatalf("an unmatched key must still reach unhandledKeyFn (unhandled=%d)", unhandled)
	}
}

func TestFallthroughNotLiveReachesUnhandledKeyFn(t *testing.T) {
	// A not-live Fallthrough binding must not swallow the key; it falls through to the
	// app's unhandledKeyFn.
	desktop := newTestDesktop(t, 40, 12)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	desktop.AddLayer(NewFullscreenLayer("base", root))
	unhandled := 0
	desktop.SetUnhandledKeyFn(func(tui.TypeEvent) { unhandled++ })
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, Scope: ScopeFallthrough},
		func() bool { return false }, // not live
	)

	desktop.handleType(ev('?'))
	if unhandled != 1 {
		t.Fatalf("a declined Fallthrough binding must let the key reach unhandledKeyFn (unhandled=%d)", unhandled)
	}
}

// --- Ctrl+C invariant preserved alongside Fallthrough bindings. ---

func TestCtrlCStillQuitsWithUnrelatedFallthroughBinding(t *testing.T) {
	// A Fallthrough binding that does NOT match Ctrl+C must not disturb the default
	// quit (the Ctrl+C-reaches-quit-only-when-unconsumed rule).
	desktop := newTestDesktop(t, 40, 12)
	quit := 0
	desktop.cancel = func() { quit++ }
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, Scope: ScopeFallthrough},
		func() bool { return true },
	)
	desktop.handleType(ctrl('c'))
	if quit != 1 {
		t.Fatalf("an unrelated Fallthrough binding must not block the default Ctrl+C quit (quit=%d)", quit)
	}
}

func TestFallthroughBindingOnCtrlCPreemptsDefaultQuit(t *testing.T) {
	// Documented consequence of the dispatch order: a Fallthrough binding ON Ctrl+C
	// runs before the built-in quit default, so it consumes the chord and the default
	// quit does not fire. (Existing apps register no such binding, so this is opt-in.)
	desktop := newTestDesktop(t, 40, 12)
	quit := 0
	desktop.cancel = func() { quit++ }
	fired := 0
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'c', Ctrl: true}, Scope: ScopeFallthrough},
		func() bool { fired++; return true },
	)
	desktop.handleType(ctrl('c'))
	if fired != 1 {
		t.Fatalf("the Ctrl+C Fallthrough binding should fire (fired=%d)", fired)
	}
	if quit != 0 {
		t.Fatalf("the consumed Ctrl+C must not also trigger the default quit (quit=%d)", quit)
	}
}

func TestCtrlCConsumedByCopyNeverReachesFallthrough(t *testing.T) {
	// Copy still wins over the Fallthrough registry: a focused widget with something
	// to copy consumes Ctrl+C at the copy stage, long before the Fallthrough check.
	desktop := newTestDesktop(t, 40, 12)
	quit := 0
	desktop.cancel = func() { quit++ }
	root := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	root.Focusable = true
	root.CopyFn = func(*VisualComponent) (string, bool) { return "selection", true }
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.setFocus(root)

	fallFired := 0
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'c', Ctrl: true}, Scope: ScopeFallthrough},
		func() bool { fallFired++; return true },
	)
	desktop.handleType(ctrl('c'))
	if fallFired != 0 {
		t.Fatalf("copy must consume Ctrl+C before the Fallthrough stage (fall=%d)", fallFired)
	}
	if quit != 0 {
		t.Fatalf("Ctrl+C consumed by copy must not quit (quit=%d)", quit)
	}
}

// --- Full-chain ordering with all three scopes live simultaneously. ---

func TestAllThreeScopesFireAtTheirOwnPositions(t *testing.T) {
	typed := 0
	desktop, window := scopedDesktop(t, &typed)

	// Global menu accelerator (Ctrl+N) via the menu registry.
	menuFired := 0
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() { menuFired++ }).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	desktop.SetMenuBar(bar)

	// Focus 'r' (scoped to the focused window) and Fallthrough '?'.
	focusFired, fallFired := 0, 0
	reg := desktop.ScopedBindings()
	reg.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window},
		func() bool { focusFired++; return true })
	reg.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, Scope: ScopeFallthrough},
		func() bool { fallFired++; return true })

	// Ctrl+N -> menu only.
	desktop.handleType(ctrl('n'))
	if menuFired != 1 || focusFired != 0 || fallFired != 0 {
		t.Fatalf("Ctrl+N must fire only the menu (menu=%d focus=%d fall=%d)", menuFired, focusFired, fallFired)
	}
	// 'r' (focus within window, widget declines) -> focus only.
	desktop.handleType(ev('r'))
	if focusFired != 1 || menuFired != 1 || fallFired != 0 {
		t.Fatalf("'r' must fire only the Focus binding (menu=%d focus=%d fall=%d)", menuFired, focusFired, fallFired)
	}
	// '?' -> fallthrough only.
	desktop.handleType(ev('?'))
	if fallFired != 1 || focusFired != 1 || menuFired != 1 {
		t.Fatalf("'?' must fire only the Fallthrough binding (menu=%d focus=%d fall=%d)", menuFired, focusFired, fallFired)
	}
}
