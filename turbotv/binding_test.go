package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Phase-1 unit tests for the first-class binding mechanism (binding.go):
// Chord matching, KeyBinding, and BindingRegistry. These pin the contract the
// menu accelerator path now shares, so a regression in the lookup core trips
// here rather than in a higher-level dispatch test.

func ev(r rune) tui.TypeEvent { return tui.TypeEvent{Key: tui.KeyRune, Rune: r} }

func ctrl(r rune) tui.TypeEvent {
	return tui.TypeEvent{Key: tui.KeyRune, Rune: r, Ctrl: true}
}

// --- Chord.Matches: the single source of truth for chord comparison. ---

func TestChordMatchesExactRuneWithCtrl(t *testing.T) {
	c := Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}
	if !c.Matches(ctrl('n')) {
		t.Fatal("Ctrl+N chord must match a Ctrl+N event")
	}
}

func TestChordMatchesRuneCaseInsensitively(t *testing.T) {
	c := Chord{Key: tui.KeyRune, Rune: 'q', Ctrl: true}
	if !c.Matches(ctrl('Q')) {
		t.Fatal("a lower-case rune chord must match the upper-case event rune (case-insensitive)")
	}
	// And symmetrically: an upper-case chord rune matches a lower-case event.
	up := Chord{Key: tui.KeyRune, Rune: 'Q', Ctrl: true}
	if !up.Matches(ctrl('q')) {
		t.Fatal("an upper-case rune chord must match the lower-case event rune")
	}
}

func TestChordModifierSensitivity(t *testing.T) {
	// Ctrl+N must never match a plain N — the modifier axis is exact.
	ctrlN := Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}
	if ctrlN.Matches(ev('n')) {
		t.Fatal("Ctrl+N must NOT match plain N")
	}
	plainN := Chord{Key: tui.KeyRune, Rune: 'n'}
	if plainN.Matches(ctrl('n')) {
		t.Fatal("plain N must NOT match a Ctrl+N event")
	}

	// Shift must match exactly.
	shiftS := Chord{Key: tui.KeyRune, Rune: 's', Ctrl: true, Shift: true}
	if shiftS.Matches(ctrl('s')) {
		t.Fatal("Ctrl+Shift+S must NOT match Ctrl+S (Shift missing)")
	}
	if !shiftS.Matches(tui.TypeEvent{Key: tui.KeyRune, Rune: 's', Ctrl: true, Shift: true}) {
		t.Fatal("Ctrl+Shift+S must match a Ctrl+Shift+S event")
	}

	// Alt must match exactly.
	altA := Chord{Key: tui.KeyRune, Rune: 'a', Alt: true}
	if altA.Matches(ev('a')) {
		t.Fatal("Alt+A must NOT match plain A")
	}
	if !altA.Matches(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a', Alt: true}) {
		t.Fatal("Alt+A must match an Alt+A event")
	}
}

func TestChordNamedKeyMatch(t *testing.T) {
	f1 := Chord{Key: tui.KeyF1}
	if !f1.Matches(tui.TypeEvent{Key: tui.KeyF1}) {
		t.Fatal("F1 chord must match an F1 event")
	}
	if f1.Matches(tui.TypeEvent{Key: tui.KeyF2}) {
		t.Fatal("F1 chord must not match an F2 event")
	}
	// A bare named-key chord must respect modifiers too.
	if f1.Matches(tui.TypeEvent{Key: tui.KeyF1, Shift: true}) {
		t.Fatal("bare F1 chord must not match Shift+F1")
	}
	shiftF1 := Chord{Key: tui.KeyF1, Shift: true}
	if !shiftF1.Matches(tui.TypeEvent{Key: tui.KeyF1, Shift: true}) {
		t.Fatal("Shift+F1 chord must match a Shift+F1 event")
	}
}

func TestChordRuneOnlyRequiresKeyRune(t *testing.T) {
	// A rune-bearing chord (zero Key) is a wildcard on the named-key axis but the
	// event must still be a KeyRune event — a function-key event carrying a stray
	// rune must not match.
	c := Chord{Rune: 'n', Ctrl: true}
	if !c.Matches(ctrl('n')) {
		t.Fatal("rune-only Ctrl+N chord must match a KeyRune Ctrl+N event")
	}
	if c.Matches(tui.TypeEvent{Key: tui.KeyF1, Rune: 'n', Ctrl: true}) {
		t.Fatal("a rune chord must not match a non-KeyRune event even if Rune coincides")
	}
}

func TestChordControlPunctuationRange(t *testing.T) {
	// The gogent #208 punctuation range (Ctrl+\ ] ^ _) must match through Chord.
	for _, r := range []rune{'\\', ']', '^', '_'} {
		c := Chord{Key: tui.KeyRune, Rune: r, Ctrl: true}
		if !c.Matches(ctrl(r)) {
			t.Errorf("Ctrl+%c chord must match its event", r)
		}
		if c.Matches(ev(r)) {
			t.Errorf("Ctrl+%c chord must require the Ctrl modifier", r)
		}
	}
}

// FINDING (non-blocking, documented behaviour): the doc comment on Chord()/Matches
// claims the zero Chord "matches nothing", but a zero Chord (KeyUnknown, Rune 0, no
// modifiers) actually matches ANY modifier-free event — the named-key axis is a
// wildcard and there is no rune to compare. This is unreachable in phase 1 (the
// menu path guards `Shortcut != nil` before registering), but it is a sharp edge
// for the phase-2 Fallthrough scope. This test pins the ACTUAL behaviour so a
// future change is noticed; see the test-report for the recommendation to fix the
// comment (or guard the zero chord).
func TestZeroChordIsAWildcardNotInert(t *testing.T) {
	var zero Chord
	if !zero.Matches(ev('a')) {
		t.Fatal("documented-as-'matches nothing' zero chord in fact matches a plain rune event")
	}
	if !zero.Matches(tui.TypeEvent{Key: tui.KeyF5}) {
		t.Fatal("zero chord matches any modifier-free named-key event")
	}
	// It does, however, reject any event carrying a modifier (the bool axes are
	// compared exactly), so it is not a universal wildcard.
	if zero.Matches(ctrl('a')) {
		t.Fatal("zero chord must reject a Ctrl-modified event")
	}
}

// --- BindingRegistry: store + resolve. ---

func TestRegistryEmptyMatchesNothing(t *testing.T) {
	r := NewBindingRegistry()
	if r.Len() != 0 {
		t.Fatalf("fresh registry Len = %d, want 0", r.Len())
	}
	if _, ok := r.Match(ctrl('n')); ok {
		t.Fatal("empty registry must not match")
	}
	if r.Dispatch(ctrl('n')) {
		t.Fatal("empty registry must not dispatch")
	}
}

func TestRegistryMatchResolvesActionID(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}, ActionID: "file.new"}, nil)
	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1", r.Len())
	}
	got, ok := r.Match(ctrl('n'))
	if !ok {
		t.Fatal("Ctrl+N must match the registered binding")
	}
	if got.ActionID != "file.new" {
		t.Fatalf("Match ActionID = %q, want %q", got.ActionID, "file.new")
	}
	// A non-matching chord resolves to none.
	if _, ok := r.Match(ctrl('w')); ok {
		t.Fatal("Ctrl+W must not match a Ctrl+N-only registry")
	}
	// Modifier sensitivity inside the registry: plain N must not match Ctrl+N.
	if _, ok := r.Match(ev('n')); ok {
		t.Fatal("plain N must not match a Ctrl+N binding (modifier sensitivity)")
	}
}

func TestRegistryDispatchFiresHandlerOnce(t *testing.T) {
	r := NewBindingRegistry()
	fired := 0
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}}, func() bool {
		fired++
		return true
	})
	if !r.Dispatch(ctrl('n')) {
		t.Fatal("Dispatch must report the event consumed")
	}
	if fired != 1 {
		t.Fatalf("handler fired %d times, want 1", fired)
	}
	// A non-match neither fires nor consumes.
	if r.Dispatch(ctrl('w')) {
		t.Fatal("non-matching Dispatch must return false")
	}
	if fired != 1 {
		t.Fatalf("non-matching Dispatch fired the handler (count=%d)", fired)
	}
}

func TestRegistryDispatchNilHandlerConsumes(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}}, nil)
	if !r.Dispatch(ctrl('n')) {
		t.Fatal("a matched binding with a nil handler must consume the event")
	}
}

func TestRegistryDispatchSkipsNotLiveHandler(t *testing.T) {
	// A handler that reports it is not live (returns false, e.g. a disabled menu
	// item) must be SKIPPED, not swallow the chord: the next matching live binding
	// fires. This is the "disabled accelerator is skipped, not swallowed" rule.
	r := NewBindingRegistry()
	chord := Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}
	deadCalls, liveCalls := 0, 0
	r.Register(KeyBinding{Chord: chord, ActionID: "dead"}, func() bool { deadCalls++; return false })
	r.Register(KeyBinding{Chord: chord, ActionID: "live"}, func() bool { liveCalls++; return true })

	if !r.Dispatch(ctrl('n')) {
		t.Fatal("Dispatch must fall through the not-live binding to the live one")
	}
	if deadCalls != 1 {
		t.Fatalf("not-live handler should have been tried once, got %d", deadCalls)
	}
	if liveCalls != 1 {
		t.Fatalf("live handler should have fired once, got %d", liveCalls)
	}
}

func TestRegistryDispatchAllNotLiveReturnsFalse(t *testing.T) {
	// When every matching binding is not live, Dispatch reports the event
	// unconsumed so the dispatch chain continues past it.
	r := NewBindingRegistry()
	chord := Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}
	r.Register(KeyBinding{Chord: chord}, func() bool { return false })
	if r.Dispatch(ctrl('n')) {
		t.Fatal("Dispatch must return false when no matching binding is live")
	}
}

func TestRegistryFirstMatchWinsInRegistrationOrder(t *testing.T) {
	// Two live bindings on the same chord: the first registered wins (first-match-
	// wins preserves the old recursive pre-order walk).
	r := NewBindingRegistry()
	chord := Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}
	order := []string{}
	r.Register(KeyBinding{Chord: chord, ActionID: "first"}, func() bool { order = append(order, "first"); return true })
	r.Register(KeyBinding{Chord: chord, ActionID: "second"}, func() bool { order = append(order, "second"); return true })
	r.Dispatch(ctrl('n'))
	if len(order) != 1 || order[0] != "first" {
		t.Fatalf("first registered live binding must win, fired order = %v", order)
	}

	// Match likewise returns the FIRST registered binding for the chord.
	got, _ := r.Match(ctrl('n'))
	if got.ActionID != "first" {
		t.Fatalf("Match returned %q, want the first-registered %q", got.ActionID, "first")
	}
}

// FINDING (non-blocking): Match ignores handler liveness while Dispatch honours it,
// so for duplicate chords where the first binding is not live, Match reports the
// first binding's ActionID but Dispatch fires the SECOND. A phase-4 customizer that
// renders Match/BindingFor would show a different action than the one that fires.
// This pins the divergence so it is a conscious contract, not an accident.
func TestRegistryMatchVsDispatchDivergeOnLiveness(t *testing.T) {
	r := NewBindingRegistry()
	chord := Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}
	r.Register(KeyBinding{Chord: chord, ActionID: "disabled"}, func() bool { return false })
	fired := ""
	r.Register(KeyBinding{Chord: chord, ActionID: "enabled"}, func() bool { fired = "enabled"; return true })

	got, ok := r.Match(ctrl('n'))
	if !ok || got.ActionID != "disabled" {
		t.Fatalf("Match ignores liveness: got %q ok=%v, want %q", got.ActionID, ok, "disabled")
	}
	r.Dispatch(ctrl('n'))
	if fired != "enabled" {
		t.Fatalf("Dispatch honours liveness and should fire %q, fired %q", "enabled", fired)
	}
}

func TestRegistryClear(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}}, func() bool { return true })
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'w', Ctrl: true}}, func() bool { return true })
	if r.Len() != 2 {
		t.Fatalf("Len = %d, want 2", r.Len())
	}
	r.Clear()
	if r.Len() != 0 {
		t.Fatalf("after Clear Len = %d, want 0", r.Len())
	}
	if _, ok := r.Match(ctrl('n')); ok {
		t.Fatal("after Clear nothing must match")
	}
	// Registry is reusable after Clear.
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'q', Ctrl: true}, ActionID: "quit"}, nil)
	if got, ok := r.Match(ctrl('q')); !ok || got.ActionID != "quit" {
		t.Fatalf("registry must be reusable after Clear: got %q ok=%v", got.ActionID, ok)
	}
}
