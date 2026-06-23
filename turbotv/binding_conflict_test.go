package tv

import (
	"errors"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Phase-3 tests for the conflict-detection registry service: Rebind, ConflictFor,
// and the ConflictError type (binding.go). These pin the toolkit contract the
// future customizer UI relies on: a TRUE conflict is same-chord AND same-scope;
// a cross-scope overlap is benign; rebinding to a free chord or to the action's
// own current chord succeeds. The helpers ev/ctrl come from binding_test.go.

// chordCtrl is a small constructor for a Ctrl+<rune> chord, the shape the menu
// path and the customizer build.
func chordCtrl(r rune) Chord { return Chord{Key: tui.KeyRune, Rune: r, Ctrl: true} }

// --- Rebind: the mutating conflict service. ---

func TestRebindToFreeChordSucceeds(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "file.new"}, nil)
	r.Register(KeyBinding{Chord: chordCtrl('w'), ActionID: "file.close"}, nil)

	// Ctrl+F is held by nobody: rebinding file.new onto it must succeed.
	if err := r.Rebind("file.new", chordCtrl('f')); err != nil {
		t.Fatalf("rebind to a free chord must succeed, got %v", err)
	}
	// The action now answers to Ctrl+F and no longer to Ctrl+N.
	if got, ok := r.Match(ctrl('f')); !ok || got.ActionID != "file.new" {
		t.Fatalf("after rebind Ctrl+F must resolve file.new: %q ok=%v", got.ActionID, ok)
	}
	if _, ok := r.Match(ctrl('n')); ok {
		t.Fatal("after rebind the old Ctrl+N chord must be free")
	}
}

func TestRebindToOwnCurrentChordIsNoopSuccess(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "file.new"}, nil)

	// Rebinding an action onto the chord it already holds is never a self-conflict.
	if err := r.Rebind("file.new", chordCtrl('n')); err != nil {
		t.Fatalf("self-rebind must be a no-op success, got %v", err)
	}
	if got, ok := r.Match(ctrl('n')); !ok || got.ActionID != "file.new" {
		t.Fatalf("self-rebind must leave the binding intact: %q ok=%v", got.ActionID, ok)
	}
}

func TestRebindSameChordSameScopeReturnsConflict(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "file.new"}, nil)
	r.Register(KeyBinding{Chord: chordCtrl('r'), ActionID: "view.refresh"}, nil)

	// file.new wants Ctrl+R, already held by view.refresh in the SAME (Global) scope.
	err := r.Rebind("file.new", chordCtrl('r'))
	if err == nil {
		t.Fatal("rebinding onto a chord held in the same scope must conflict")
	}
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("conflict must be a *ConflictError, got %T: %v", err, err)
	}
	if ce.Holder != "view.refresh" {
		t.Errorf("ConflictError.Holder = %q, want the OTHER action %q", ce.Holder, "view.refresh")
	}
	if ce.ActionID != "file.new" {
		t.Errorf("ConflictError.ActionID = %q, want the action being rebound %q", ce.ActionID, "file.new")
	}
	if ce.Scope != ScopeGlobal {
		t.Errorf("ConflictError.Scope = %v, want ScopeGlobal", ce.Scope)
	}
	if !ce.Chord.conflictsWith(chordCtrl('r')) {
		t.Errorf("ConflictError.Chord = %v, want the attempted Ctrl+R", ce.Chord)
	}
	// The message must name the holder and the chord for the UI.
	msg := err.Error()
	for _, want := range []string{"view.refresh", "Ctrl+R", "Global"} {
		if !containsSub(msg, want) {
			t.Errorf("ConflictError message %q must mention %q", msg, want)
		}
	}
}

func TestRebindConflictDoesNotMutate(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "file.new"}, nil)
	r.Register(KeyBinding{Chord: chordCtrl('r'), ActionID: "view.refresh"}, nil)

	_ = r.Rebind("file.new", chordCtrl('r'))
	// file.new must still answer to its ORIGINAL Ctrl+N after a rejected rebind.
	if got, ok := r.Match(ctrl('n')); !ok || got.ActionID != "file.new" {
		t.Fatalf("a rejected rebind must not mutate the binding: Ctrl+N -> %q ok=%v", got.ActionID, ok)
	}
	// And Ctrl+R must still resolve to its original holder, not the rejected rebinder.
	if got, ok := r.Match(ctrl('r')); !ok || got.ActionID != "view.refresh" {
		t.Fatalf("Ctrl+R holder must be unchanged after a rejected rebind: %q ok=%v", got.ActionID, ok)
	}
}

func TestRebindCaseInsensitiveConflict(t *testing.T) {
	// Ctrl+R and Ctrl+r are the same chord (case-insensitive rune); rebinding onto the
	// other case must still conflict.
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'R', Ctrl: true}, ActionID: "a"}, nil)
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "b"}, nil)
	if err := r.Rebind("b", Chord{Key: tui.KeyRune, Rune: 'r', Ctrl: true}); err == nil {
		t.Fatal("Ctrl+r must conflict with an existing Ctrl+R (case-insensitive)")
	}
}

func TestRebindAcrossScopesSucceeds(t *testing.T) {
	// A chord held only in a DIFFERENT scope is free for this scope: rebinding the
	// Global action onto Ctrl+R must succeed even though a Fallthrough binding holds
	// Ctrl+R. (Fallthrough avoids the Focus Target requirement.)
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "global.action", Scope: ScopeGlobal}, nil)
	r.Register(KeyBinding{Chord: chordCtrl('r'), ActionID: "fall.action", Scope: ScopeFallthrough}, nil)

	if err := r.Rebind("global.action", chordCtrl('r')); err != nil {
		t.Fatalf("rebinding Global onto a chord only used in Fallthrough must succeed, got %v", err)
	}
	if got, ok := r.Match(ctrl('r')); !ok || got.ActionID != "global.action" {
		t.Fatalf("Global Ctrl+R must now resolve global.action: %q ok=%v", got.ActionID, ok)
	}
	// The Fallthrough Ctrl+R is untouched and still resolves in its own scope.
	if got, ok := r.MatchFallthrough(ctrl('r')); !ok || got.ActionID != "fall.action" {
		t.Fatalf("the Fallthrough Ctrl+R must be unchanged: %q ok=%v", got.ActionID, ok)
	}
}

func TestRebindUnknownActionErrorsWithoutMutating(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "file.new"}, nil)

	err := r.Rebind("does.not.exist", chordCtrl('f'))
	if err == nil {
		t.Fatal("rebinding an unregistered action must error")
	}
	// It must NOT be a conflict error — it is a "no such action" error.
	var ce *ConflictError
	if errors.As(err, &ce) {
		t.Fatalf("unknown-action error must not be a *ConflictError, got %v", err)
	}
	// Nothing must have changed, and no phantom binding created for the missing action.
	if r.Len() != 1 {
		t.Fatalf("Len after failed rebind = %d, want 1 (no binding added)", r.Len())
	}
	if _, ok := r.Match(ctrl('f')); ok {
		t.Fatal("a failed unknown-action rebind must not register the chord")
	}
}

func TestRebindReassignAfterFreeing(t *testing.T) {
	// Realistic customizer flow: A holds Ctrl+R, B wants it (conflict), so the user
	// first frees A by moving it to Ctrl+G, then B's rebind succeeds.
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('r'), ActionID: "a"}, nil)
	r.Register(KeyBinding{Chord: chordCtrl('b'), ActionID: "b"}, nil)

	if err := r.Rebind("b", chordCtrl('r')); err == nil {
		t.Fatal("expected a conflict before freeing the holder")
	}
	if err := r.Rebind("a", chordCtrl('g')); err != nil {
		t.Fatalf("freeing the holder must succeed: %v", err)
	}
	if err := r.Rebind("b", chordCtrl('r')); err != nil {
		t.Fatalf("after freeing, the reassign must succeed: %v", err)
	}
	if got, _ := r.Match(ctrl('r')); got.ActionID != "b" {
		t.Fatalf("Ctrl+R must now belong to b, got %q", got.ActionID)
	}
}

// REGRESSION GUARD (round-2 fix): the empty ActionID is a documented-allowed value,
// and exclusion now uses entry IDENTITY (index) for the rebind target rather than the
// ActionID alone. So a no-op self-rebind of an anonymous (empty-ActionID) binding onto
// its own chord must SUCCEED — round 1 regressed this into a spurious self-conflict
// because "" was overloaded as the "exclude nothing" sentinel.
func TestRebindEmptyActionSelfRebindSucceeds(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n') /* ActionID "" */}, nil)

	// Self-rebind onto the same chord: no-op success, not a self-conflict.
	if err := r.Rebind("", chordCtrl('n')); err != nil {
		t.Fatalf("empty-action self-rebind must succeed, got %v", err)
	}
	// Rebinding the anonymous binding onto a free chord also succeeds.
	if err := r.Rebind("", chordCtrl('f')); err != nil {
		t.Fatalf("empty-action rebind onto a free chord must succeed, got %v", err)
	}
	if got, ok := r.Match(ctrl('f')); !ok {
		t.Fatalf("after rebind the anonymous binding must answer to Ctrl+F: ok=%v got=%q", ok, got.ActionID)
	}
}

// REGRESSION GUARD (round-2 fix): the index/ActionID exclusions are independent. The
// multi-registration guard (skip same ActionID) must apply ONLY for a non-empty
// ActionID, so two DISTINCT anonymous bindings are not collapsed: a genuine clash
// between them is still detected. Rebinding the first anonymous binding onto the chord
// a SECOND anonymous binding already holds must conflict.
func TestRebindDistinctAnonymousBindingsStillConflict(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n') /* "" #1 */}, nil)
	r.Register(KeyBinding{Chord: chordCtrl('w') /* "" #2 */}, nil)

	err := r.Rebind("", chordCtrl('w')) // targets #1 (first ""), collides with #2
	if err == nil {
		t.Fatal("two distinct anonymous bindings clashing on a chord must conflict")
	}
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected a *ConflictError, got %T: %v", err, err)
	}
	// #1 must NOT have been mutated by the rejected rebind.
	if _, ok := r.Match(ctrl('n')); !ok {
		t.Fatal("a rejected rebind must leave the first anonymous binding on Ctrl+N")
	}
}

// ConflictFor excludes nothing, so a customizer pre-warning with the edited action's
// OWN current chord gets that action back as the holder (documented round-2 behaviour);
// the UI is expected to ignore a holder equal to the action being edited.
func TestConflictForReturnsEditedActionAsHolder(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('r'), ActionID: "view.refresh"}, nil)
	if holder, ok := r.ConflictFor(chordCtrl('r'), ScopeGlobal); !ok || holder != "view.refresh" {
		t.Fatalf("ConflictFor on an action's own chord must return that action: %q ok=%v", holder, ok)
	}
	// Rebind, by contrast, excludes the target and treats the same chord as a no-op.
	if err := r.Rebind("view.refresh", chordCtrl('r')); err != nil {
		t.Fatalf("Rebind onto own chord must be a no-op success, got %v", err)
	}
}

// --- ConflictFor: the non-mutating query for the customizer's pre-commit warning. ---

func TestConflictForReportsHolderWithoutMutating(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('r'), ActionID: "view.refresh"}, nil)

	holder, ok := r.ConflictFor(chordCtrl('r'), ScopeGlobal)
	if !ok || holder != "view.refresh" {
		t.Fatalf("ConflictFor must name the holder: %q ok=%v", holder, ok)
	}
	// The query must not mutate: the binding is still resolvable unchanged afterwards.
	if got, ok := r.Match(ctrl('r')); !ok || got.ActionID != "view.refresh" {
		t.Fatalf("ConflictFor must be non-mutating: %q ok=%v", got.ActionID, ok)
	}
	if r.Len() != 1 {
		t.Fatalf("ConflictFor must not add entries: Len=%d", r.Len())
	}
}

func TestConflictForFreeChordIsFalse(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('r'), ActionID: "view.refresh"}, nil)
	if holder, ok := r.ConflictFor(chordCtrl('f'), ScopeGlobal); ok {
		t.Fatalf("a free chord must report no conflict, got holder %q", holder)
	}
}

func TestConflictForDifferentScopeIsFalse(t *testing.T) {
	// Ctrl+R held in Global is NOT a conflict for the same chord queried in
	// Fallthrough scope — conflict is keyed on (chord, scope).
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('r'), ActionID: "global.refresh", Scope: ScopeGlobal}, nil)
	if holder, ok := r.ConflictFor(chordCtrl('r'), ScopeFallthrough); ok {
		t.Fatalf("a chord held only in Global must not conflict in Fallthrough, got %q", holder)
	}
	// And symmetrically it IS a conflict when queried in its own scope.
	if _, ok := r.ConflictFor(chordCtrl('r'), ScopeGlobal); !ok {
		t.Fatal("the same chord must conflict when queried in its own scope")
	}
}

func TestConflictForNamedKeyChord(t *testing.T) {
	// Conflict detection must work for named-key chords (function keys), not only runes.
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyF5}, ActionID: "view.refresh"}, nil)
	if holder, ok := r.ConflictFor(Chord{Key: tui.KeyF5}, ScopeGlobal); !ok || holder != "view.refresh" {
		t.Fatalf("F5 must conflict with the registered F5 binding: %q ok=%v", holder, ok)
	}
	// A different function key is free.
	if _, ok := r.ConflictFor(Chord{Key: tui.KeyF6}, ScopeGlobal); ok {
		t.Fatal("F6 must not conflict with an F5-only registry")
	}
}

func TestConflictForModifierSensitivity(t *testing.T) {
	// Ctrl+R is a different chord from plain R: querying the plain R must not conflict.
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('r'), ActionID: "ctrlR"}, nil)
	if holder, ok := r.ConflictFor(Chord{Key: tui.KeyRune, Rune: 'r'}, ScopeGlobal); ok {
		t.Fatalf("plain R must not conflict with Ctrl+R (modifier sensitivity), got %q", holder)
	}
}

func TestConflictErrorImplementsError(t *testing.T) {
	// ConflictError must be usable through the error interface and errors.As.
	var err error = &ConflictError{
		Chord:    chordCtrl('r'),
		Scope:    ScopeFocus,
		ActionID: "x",
		Holder:   "y",
	}
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatal("errors.As must recover a *ConflictError")
	}
	if ce.Error() == "" {
		t.Fatal("ConflictError.Error must render a non-empty message")
	}
}

// --- BindingFor: the read side the customizer needs (added in fixes round 1). ---

func TestBindingForReturnsCurrentBinding(t *testing.T) {
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "file.new"}, nil)

	got, ok := r.BindingFor("file.new")
	if !ok {
		t.Fatal("BindingFor must find a registered action")
	}
	if !got.Chord.conflictsWith(chordCtrl('n')) {
		t.Fatalf("BindingFor chord = %v, want Ctrl+N", got.Chord)
	}
	// It must track Rebind: after moving the action, BindingFor reflects the new chord.
	if err := r.Rebind("file.new", chordCtrl('f')); err != nil {
		t.Fatalf("rebind failed: %v", err)
	}
	got, _ = r.BindingFor("file.new")
	if !got.Chord.conflictsWith(chordCtrl('f')) {
		t.Fatalf("after rebind BindingFor chord = %v, want Ctrl+F", got.Chord)
	}
	// Unknown action → not found.
	if _, ok := r.BindingFor("nope"); ok {
		t.Fatal("BindingFor must report false for an unregistered action")
	}
}

func TestBindingForReturnsFirstWhenMultiRegistered(t *testing.T) {
	// When an action is registered more than once, BindingFor returns the FIRST,
	// mirroring which entry Rebind targets.
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "dup", Scope: ScopeGlobal}, nil)
	r.Register(KeyBinding{Chord: chordCtrl('w'), ActionID: "dup", Scope: ScopeFallthrough}, nil)
	got, ok := r.BindingFor("dup")
	if !ok || !got.Chord.conflictsWith(chordCtrl('n')) || got.Scope != ScopeGlobal {
		t.Fatalf("BindingFor must return the first registration: %+v ok=%v", got, ok)
	}
}

// REGRESSION GUARD (was a FINDING): an action registered more than once must never be
// reported as conflicting with ITSELF. Conflict exclusion is now by ActionID, not by
// index, so rebinding one of an action's entries onto the chord held by another of its
// OWN entries succeeds (Holder == ActionID would be a nonsensical self-conflict).
func TestRebindMultiRegisteredActionNoSelfConflict(t *testing.T) {
	r := NewBindingRegistry()
	// Same action holds two Global chords (e.g. a primary + alias accelerator).
	r.Register(KeyBinding{Chord: chordCtrl('n'), ActionID: "act", Scope: ScopeGlobal}, nil)
	r.Register(KeyBinding{Chord: chordCtrl('w'), ActionID: "act", Scope: ScopeGlobal}, nil)

	// Rebinding "act" onto Ctrl+W (held by its own second entry) must NOT self-conflict.
	if err := r.Rebind("act", chordCtrl('w')); err != nil {
		t.Fatalf("rebinding an action onto its own other chord must succeed, got %v", err)
	}
}

// FINDING (non-blocking, sharp edge): conflict detection is keyed on (chord, scope)
// and does NOT consider the Focus Target. Two ScopeFocus bindings on DISTINCT targets
// (two different windows each binding plain 'r') are reported as conflicting even
// though at dispatch they never collide — MatchFocus filters by focus-within so each
// 'r' only fires in its own window. A customizer that allows per-window letter
// bindings would get a spurious conflict warning. This test pins the ACTUAL behaviour
// (they conflict) so a future Target-aware refinement is a conscious change.
func TestConflictForFocusIgnoresTarget(t *testing.T) {
	r := NewBindingRegistry()
	winA := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 5})
	winB := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 5})
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "a.toggle", Scope: ScopeFocus, Target: winA}, nil)
	r.Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'r'}, ActionID: "b.toggle", Scope: ScopeFocus, Target: winB}, nil)

	holder, ok := r.ConflictFor(Chord{Key: tui.KeyRune, Rune: 'r'}, ScopeFocus)
	if !ok || holder != "a.toggle" {
		t.Fatalf("current impl reports a scope-level (Target-blind) conflict naming the first holder: %q ok=%v", holder, ok)
	}
	// But at dispatch the two are isolated by focus-within, proving the conflict is spurious.
	if got, _ := r.MatchFocus(ev('r'), winB); got.ActionID != "b.toggle" {
		t.Fatalf("focus 'r' within winB must resolve b.toggle, got %q", got.ActionID)
	}
}

// REGRESSION GUARD (was a FINDING): a rune-only chord (Key == KeyUnknown, the wildcard
// form) and the explicit KeyRune form of the same combo Match identical events, so the
// conflict service must treat them as the SAME chord. The fix normalises the named-key
// axis (effectiveKey: any rune-bearing chord constrains KeyRune) so the overlap is
// detected regardless of how the Key field was spelled. This guard pins both directions.
func TestConflictForNormalisesRuneKeyAxis(t *testing.T) {
	// Both chords match a Ctrl+N event — the precondition for the conflict to be real.
	wild := Chord{Rune: 'n', Ctrl: true}
	keyed := Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}
	if !wild.Matches(ctrl('n')) || !keyed.Matches(ctrl('n')) {
		t.Fatal("both chords must match a Ctrl+N event")
	}

	// wildcard-Key registered, KeyRune queried → conflict detected.
	r := NewBindingRegistry()
	r.Register(KeyBinding{Chord: wild, ActionID: "wild"}, nil)
	if holder, ok := r.ConflictFor(keyed, ScopeGlobal); !ok || holder != "wild" {
		t.Fatalf("KeyRune query must conflict with the wildcard-Key binding: %q ok=%v", holder, ok)
	}

	// Symmetric: KeyRune registered, wildcard-Key queried → conflict detected.
	r2 := NewBindingRegistry()
	r2.Register(KeyBinding{Chord: keyed, ActionID: "keyed"}, nil)
	if holder, ok := r2.ConflictFor(wild, ScopeGlobal); !ok || holder != "keyed" {
		t.Fatalf("wildcard-Key query must conflict with the KeyRune binding: %q ok=%v", holder, ok)
	}
}

// containsSub is a tiny substring helper (avoids importing strings just for tests
// in this file).
func containsSub(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
