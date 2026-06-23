package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Phase-2 unit tests for the scope-aware BindingRegistry lookups (binding.go):
// Scope, KeyBinding.Target, focusWithin, and the Focus/Fallthrough Match*/Dispatch*
// methods. These pin the contract the Desktop dispatch chain relies on, so a
// regression in the scope filter trips here rather than only in the higher-level
// dispatch test. Helpers ev/ctrl live in binding_test.go (same package).

// focusComp builds a focusable leaf component whose OnTypeFn records each event and
// returns `consume` (so a test can make the focused widget claim or decline a key).
func focusComp(consume bool, seen *int) *VisualComponent {
	c := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	c.Focusable = true
	c.OnTypeFn = func(_ *VisualComponent, _ tui.TypeEvent) bool {
		if seen != nil {
			*seen++
		}
		return consume
	}
	return c
}

// --- Scope is an int whose zero value is ScopeGlobal (Phase-1 default). ---

func TestScopeZeroValueIsGlobal(t *testing.T) {
	var s Scope
	if s != ScopeGlobal {
		t.Fatalf("zero Scope = %d, want ScopeGlobal (%d)", s, ScopeGlobal)
	}
	// A zero-valued KeyBinding (the Phase-1 shape) is therefore Global with no Target.
	var kb KeyBinding
	if kb.Scope != ScopeGlobal {
		t.Fatalf("zero KeyBinding.Scope = %d, want ScopeGlobal", kb.Scope)
	}
	if kb.Target != nil {
		t.Fatal("zero KeyBinding.Target must be nil")
	}
	// The three scopes are distinct.
	if ScopeGlobal == ScopeFocus || ScopeFocus == ScopeFallthrough || ScopeGlobal == ScopeFallthrough {
		t.Fatal("the three scopes must be distinct constants")
	}
}

// --- focusWithin: the focus-within walk a ScopeFocus binding uses. ---

func TestFocusWithin(t *testing.T) {
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	mid := NewComponent(Rect{X: 0, Y: 0, W: 20, H: 6})
	leaf := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	root.AddChild(mid)
	mid.AddChild(leaf)
	other := NewComponent(Rect{X: 0, Y: 0, W: 5, H: 1}) // unrelated tree

	// focused == target.
	if !focusWithin(leaf, leaf) {
		t.Fatal("a component is within itself")
	}
	// focused is a direct child of target.
	if !focusWithin(leaf, mid) {
		t.Fatal("leaf is within its parent mid")
	}
	// focused is a deep descendant of target.
	if !focusWithin(leaf, root) {
		t.Fatal("leaf is within its grandparent root")
	}
	// target is a descendant of focused (wrong direction) -> not within.
	if focusWithin(root, leaf) {
		t.Fatal("root is NOT within its descendant leaf (walk is upward only)")
	}
	// unrelated trees.
	if focusWithin(leaf, other) {
		t.Fatal("leaf is not within an unrelated component")
	}
	// nil target -> never within (the unscoped-Focus guard).
	if focusWithin(leaf, nil) {
		t.Fatal("nil target must never be within")
	}
	// nil focused -> never within.
	if focusWithin(nil, root) {
		t.Fatal("nil focused must never be within")
	}
	if focusWithin(nil, nil) {
		t.Fatal("nil/nil must be false")
	}
}

// --- MatchFocus: scope-filtered + focus-within + chord, liveness-agnostic. ---

func TestMatchFocusFindsOnlyForTargetInFocus(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	inner := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	window.AddChild(inner)
	other := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})

	r.Register(KeyBinding{
		Chord:    Chord{Key: tui.KeyRune, Rune: 'r'},
		ActionID: "transcript.reasoning",
		Scope:    ScopeFocus,
		Target:   window,
	}, nil)

	// Focus inside the target (a descendant) -> found.
	got, ok := r.MatchFocus(ev('r'), inner)
	if !ok || got.ActionID != "transcript.reasoning" {
		t.Fatalf("Focus binding must be found when focus is within Target: %q ok=%v", got.ActionID, ok)
	}
	// Focus is the target itself -> found.
	if _, ok := r.MatchFocus(ev('r'), window); !ok {
		t.Fatal("Focus binding must be found when the target itself is focused")
	}
	// Focus elsewhere -> NOT found (the scoping guarantee).
	if _, ok := r.MatchFocus(ev('r'), other); ok {
		t.Fatal("Focus binding must NOT be found when focus is outside Target")
	}
	// No focus -> NOT found.
	if _, ok := r.MatchFocus(ev('r'), nil); ok {
		t.Fatal("Focus binding must NOT be found when nothing is focused")
	}
	// Right target, wrong chord -> NOT found (chord still checked).
	if _, ok := r.MatchFocus(ev('x'), inner); ok {
		t.Fatal("a non-matching chord must not be found even within Target")
	}
}

func TestMatchFocusNilTargetNeverMatches(t *testing.T) {
	// A ScopeFocus binding with a nil Target must never match — it would otherwise
	// steal the key from every focused widget.
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: nil}, nil)
	inner := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	if _, ok := r.MatchFocus(ev('r'), inner); ok {
		t.Fatal("a nil-Target Focus binding must never match")
	}
}

func TestMatchFocusModifierSensitivity(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	// A Ctrl+R focus binding must not match a plain r within the same target.
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r', Ctrl: true}, Scope: ScopeFocus, Target: window}, nil)
	if _, ok := r.MatchFocus(ev('r'), window); ok {
		t.Fatal("Ctrl+R focus binding must not match plain r")
	}
	if _, ok := r.MatchFocus(ctrl('r'), window); !ok {
		t.Fatal("Ctrl+R focus binding must match a Ctrl+R event within target")
	}
}

// --- MatchFallthrough: scope-filtered + chord, focus-agnostic. ---

func TestMatchFallthroughIgnoresFocus(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, ActionID: "help", Scope: ScopeFallthrough}, nil)
	got, ok := r.MatchFallthrough(ev('?'))
	if !ok || got.ActionID != "help" {
		t.Fatalf("Fallthrough binding must match regardless of focus: %q ok=%v", got.ActionID, ok)
	}
	// Wrong chord -> not found.
	if _, ok := r.MatchFallthrough(ev('!')); ok {
		t.Fatal("a non-matching chord must not match the Fallthrough binding")
	}
}

// --- Scope isolation: each lookup sees only its own scope. ---

func TestScopeLookupsAreMutuallyIsolated(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	gChord := Chord{Key: tui.KeyRune, Rune: 'g'}
	fChord := Chord{Key: tui.KeyRune, Rune: 'g'}
	tChord := Chord{Key: tui.KeyRune, Rune: 'g'}
	r.Register(KeyBinding{Chord: gChord, ActionID: "global", Scope: ScopeGlobal}, nil)
	r.Register(KeyBinding{Chord: fChord, ActionID: "focus", Scope: ScopeFocus, Target: window}, nil)
	r.Register(KeyBinding{Chord: tChord, ActionID: "fall", Scope: ScopeFallthrough}, nil)

	// MatchFocus must skip the Global and Fallthrough bindings on the same chord.
	if got, ok := r.MatchFocus(ev('g'), window); !ok || got.ActionID != "focus" {
		t.Fatalf("MatchFocus picked %q ok=%v, want the Focus-scope binding", got.ActionID, ok)
	}
	// MatchFallthrough must skip Global and Focus.
	if got, ok := r.MatchFallthrough(ev('g')); !ok || got.ActionID != "fall" {
		t.Fatalf("MatchFallthrough picked %q ok=%v, want the Fallthrough-scope binding", got.ActionID, ok)
	}
	// The scope-agnostic Match returns the FIRST registered regardless of scope.
	if got, ok := r.Match(ev('g')); !ok || got.ActionID != "global" {
		t.Fatalf("scope-agnostic Match picked %q ok=%v, want the first-registered 'global'", got.ActionID, ok)
	}
}

func TestBenignCrossScopeOverlap(t *testing.T) {
	// The issue's example: a Global Ctrl+R and a Focus-scope plain r coexist without
	// conflict. Each chord resolves to its own scope's binding; they never cross.
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r', Ctrl: true}, ActionID: "global.refresh", Scope: ScopeGlobal}, nil)
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "focus.reasoning", Scope: ScopeFocus, Target: window}, nil)

	// Ctrl+R resolves Global; plain r within target resolves Focus.
	if got, ok := r.Match(ctrl('r')); !ok || got.ActionID != "global.refresh" {
		t.Fatalf("Ctrl+R must resolve Global: %q ok=%v", got.ActionID, ok)
	}
	if got, ok := r.MatchFocus(ev('r'), window); !ok || got.ActionID != "focus.reasoning" {
		t.Fatalf("plain r within target must resolve Focus: %q ok=%v", got.ActionID, ok)
	}
	// Cross-checks: the Global Ctrl+R is NOT a Focus match, the Focus r is NOT a
	// Global Ctrl+R match.
	if _, ok := r.MatchFocus(ctrl('r'), window); ok {
		t.Fatal("the Global Ctrl+R must not surface as a Focus-scope match")
	}
}

// --- DispatchFocus: liveness + focus-within, mirroring Dispatch semantics. ---

func TestDispatchFocusFiresWithinTargetOnly(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	inner := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	window.AddChild(inner)
	other := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})

	fired := 0
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window},
		func() bool { fired++; return true })

	if !r.DispatchFocus(ev('r'), inner) {
		t.Fatal("DispatchFocus must fire when focus is within Target")
	}
	if fired != 1 {
		t.Fatalf("handler fired %d times, want 1", fired)
	}
	// Focus outside the target: nothing fires, event unconsumed.
	if r.DispatchFocus(ev('r'), other) {
		t.Fatal("DispatchFocus must NOT consume when focus is outside Target")
	}
	if fired != 1 {
		t.Fatalf("handler fired again outside target (count=%d)", fired)
	}
	// Nil focus: nothing fires.
	if r.DispatchFocus(ev('r'), nil) {
		t.Fatal("DispatchFocus must NOT consume when nothing is focused")
	}
}

func TestDispatchFocusNilHandlerConsumes(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window}, nil)
	if !r.DispatchFocus(ev('r'), window) {
		t.Fatal("a matched Focus binding with a nil handler must consume the event")
	}
}

func TestDispatchFocusSkipsNotLiveThenFires(t *testing.T) {
	// A not-live Focus handler (returns false) is skipped; a later live Focus binding
	// (also within target) fires — the same "disabled is skipped, not swallowed" rule.
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	chord := Chord{Key: tui.KeyRune, Rune: 'r'}
	dead, live := 0, 0
	r.Register(KeyBinding{Chord: chord, ActionID: "dead", Scope: ScopeFocus, Target: window},
		func() bool { dead++; return false })
	r.Register(KeyBinding{Chord: chord, ActionID: "live", Scope: ScopeFocus, Target: window},
		func() bool { live++; return true })
	if !r.DispatchFocus(ev('r'), window) {
		t.Fatal("DispatchFocus must fall through the not-live binding to the live one")
	}
	if dead != 1 || live != 1 {
		t.Fatalf("dead=%d live=%d, want 1/1", dead, live)
	}
}

func TestDispatchFocusAllNotLiveReturnsFalse(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFocus, Target: window},
		func() bool { return false })
	if r.DispatchFocus(ev('r'), window) {
		t.Fatal("DispatchFocus must return false when no matching Focus binding is live")
	}
}

func TestDispatchFocusPicksBindingForFocusedTarget(t *testing.T) {
	// Two Focus bindings on the same chord but different targets: only the one whose
	// target contains the current focus fires.
	r := NewBindingRegistry()
	winA := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	winB := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	innerB := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	winB.AddChild(innerB)
	chord := Chord{Key: tui.KeyRune, Rune: 'r'}
	fired := ""
	r.Register(KeyBinding{Chord: chord, ActionID: "A", Scope: ScopeFocus, Target: winA},
		func() bool { fired = "A"; return true })
	r.Register(KeyBinding{Chord: chord, ActionID: "B", Scope: ScopeFocus, Target: winB},
		func() bool { fired = "B"; return true })

	r.DispatchFocus(ev('r'), innerB)
	if fired != "B" {
		t.Fatalf("the binding whose target holds focus must fire, fired %q", fired)
	}
}

func TestDispatchFocusIgnoresOtherScopes(t *testing.T) {
	// A Global and a Fallthrough binding on the focus chord must not fire from
	// DispatchFocus even when focus is within a (hypothetical) target.
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	globalFired, fallFired := 0, 0
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeGlobal},
		func() bool { globalFired++; return true })
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, Scope: ScopeFallthrough},
		func() bool { fallFired++; return true })
	if r.DispatchFocus(ev('r'), window) {
		t.Fatal("DispatchFocus must ignore Global and Fallthrough bindings")
	}
	if globalFired != 0 || fallFired != 0 {
		t.Fatalf("non-Focus handlers fired (global=%d fall=%d)", globalFired, fallFired)
	}
}

// --- DispatchFallthrough: liveness, focus-agnostic. ---

func TestDispatchFallthroughFiresAndHonoursLiveness(t *testing.T) {
	r := NewBindingRegistry()
	chord := Chord{Key: tui.KeyRune, Rune: '?'}
	dead, live := 0, 0
	r.Register(KeyBinding{Chord: chord, ActionID: "dead", Scope: ScopeFallthrough},
		func() bool { dead++; return false })
	r.Register(KeyBinding{Chord: chord, ActionID: "live", Scope: ScopeFallthrough},
		func() bool { live++; return true })
	if !r.DispatchFallthrough(ev('?')) {
		t.Fatal("DispatchFallthrough must fall through not-live to the live binding")
	}
	if dead != 1 || live != 1 {
		t.Fatalf("dead=%d live=%d, want 1/1", dead, live)
	}
	// Non-match neither fires nor consumes.
	if r.DispatchFallthrough(ev('!')) {
		t.Fatal("a non-matching DispatchFallthrough must return false")
	}
}

func TestDispatchFallthroughNilHandlerConsumes(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, Scope: ScopeFallthrough}, nil)
	if !r.DispatchFallthrough(ev('?')) {
		t.Fatal("a matched Fallthrough binding with a nil handler must consume the event")
	}
}

func TestDispatchFallthroughIgnoresOtherScopes(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	globalFired, focusFired := 0, 0
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, Scope: ScopeGlobal},
		func() bool { globalFired++; return true })
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: '?'}, Scope: ScopeFocus, Target: window},
		func() bool { focusFired++; return true })
	if r.DispatchFallthrough(ev('?')) {
		t.Fatal("DispatchFallthrough must ignore Global and Focus bindings")
	}
	if globalFired != 0 || focusFired != 0 {
		t.Fatalf("non-Fallthrough handlers fired (global=%d focus=%d)", globalFired, focusFired)
	}
}

// --- Match*/Dispatch* divergence on liveness (mirrors the Phase-1 contract). ---

func TestMatchFocusVsDispatchFocusDivergeOnLiveness(t *testing.T) {
	r := NewBindingRegistry()
	window := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	chord := Chord{Key: tui.KeyRune, Rune: 'r'}
	r.Register(KeyBinding{Chord: chord, ActionID: "disabled", Scope: ScopeFocus, Target: window},
		func() bool { return false })
	fired := ""
	r.Register(KeyBinding{Chord: chord, ActionID: "enabled", Scope: ScopeFocus, Target: window},
		func() bool { fired = "enabled"; return true })

	// MatchFocus is liveness-agnostic: it reports the first registered binding.
	if got, ok := r.MatchFocus(ev('r'), window); !ok || got.ActionID != "disabled" {
		t.Fatalf("MatchFocus = %q ok=%v, want the first-registered 'disabled'", got.ActionID, ok)
	}
	// DispatchFocus honours liveness: it fires the live one.
	r.DispatchFocus(ev('r'), window)
	if fired != "enabled" {
		t.Fatalf("DispatchFocus should fire the live binding, fired %q", fired)
	}
}

// FINDING pin (documented sharp edge): a ScopeFallthrough binding built with a zero
// Chord is NOT inert — the zero chord is a wildcard on the named-key axis and has no
// rune to compare, so it matches ANY modifier-free event at the fallthrough point
// (it would swallow every plain keystroke before unhandledKeyFn). This mirrors the
// Phase-1 TestZeroChordIsAWildcardNotInert note; it is a trap for anyone registering
// a Fallthrough binding without setting a chord. Pinned so a future guard is noticed.
func TestZeroChordFallthroughIsAWildcard(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Scope: ScopeFallthrough}, nil) // zero Chord
	if _, ok := r.MatchFallthrough(ev('a')); !ok {
		t.Fatal("a zero-chord Fallthrough binding matches any modifier-free rune event")
	}
	if _, ok := r.MatchFallthrough(tui.TypeEvent{Key: tui.KeyF5}); !ok {
		t.Fatal("a zero-chord Fallthrough binding matches any modifier-free named-key event")
	}
	// It still rejects a modified event (the bool axes are exact).
	if _, ok := r.MatchFallthrough(ctrl('a')); ok {
		t.Fatal("a zero-chord Fallthrough binding must still reject a Ctrl-modified event")
	}
}
