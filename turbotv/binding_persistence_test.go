package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Round-2 tests: the registry is now a PERSISTENT instance owned by the MenuBar
// (built by NewMenuBar, re-synced by RebuildBindings) instead of being rebuilt per
// keystroke. These pin the new ownership contract and, adversarially, document the
// staleness it introduces relative to the pre-refactor per-keystroke walk.

func ctrlN() tui.TypeEvent { return tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true} }
func ctrlM() tui.TypeEvent { return tui.TypeEvent{Key: tui.KeyRune, Rune: 'm', Ctrl: true} }

// --- Persistent identity: Registry()/Bindings() return the same instance. ---

func TestRegistryIsPersistentInstance(t *testing.T) {
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	r1 := bar.Registry()
	r2 := bar.Registry()
	if r1 != r2 {
		t.Fatal("Registry() must return the same persistent instance across calls")
	}

	desktop := newTestDesktop(t, 40, 12)
	desktop.SetMenuBar(bar)
	if desktop.Bindings() != r1 {
		t.Fatal("Desktop.Bindings() must expose the menubar's persistent registry, not a copy")
	}
}

// A struct-literal MenuBar (assembled without NewMenuBar) must lazily build its
// registry on first Registry() call.
func TestRegistryLazyBuildsForStructLiteralMenuBar(t *testing.T) {
	bar := &MenuBar{
		Menus: []*MenuItem{
			NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
		},
	}
	got, ok := bar.Registry().Match(ctrlN())
	if !ok || got.Chord.Rune != 'n' {
		t.Fatalf("lazy Registry() must build from the menu tree: %+v ok=%v", got, ok)
	}
	// And HandleAccelerator works on a lazily-built registry too.
	fired := false
	bar.Menus[0].Children[0].OnSelect = func() { fired = true }
	if !bar.HandleAccelerator(ctrlN()) || !fired {
		t.Fatal("HandleAccelerator must fire through a lazily-built registry")
	}
}

// --- Caller registrations persist (the fix for the ephemeral-registry footgun). ---

func TestCallerRegistrationPersistsAndIsConsulted(t *testing.T) {
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	fired := false
	bar.Registry().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'k', Ctrl: true}, ActionID: "palette"},
		func() bool { fired = true; return true },
	)
	// The extra binding must still be present on the next access...
	if _, ok := bar.Registry().Match(tui.TypeEvent{Key: tui.KeyRune, Rune: 'k', Ctrl: true}); !ok {
		t.Fatal("a caller-registered binding must persist across Registry() calls")
	}
	// ...and the accelerator path (which goes through the same instance) must fire it.
	if !bar.HandleAccelerator(tui.TypeEvent{Key: tui.KeyRune, Rune: 'k', Ctrl: true}) || !fired {
		t.Fatal("HandleAccelerator must consult the persistent registry including caller bindings")
	}
}

// --- Live vs snapshot semantics. ---

// Enabled toggles are read LIVE (no rebuild needed) — preserved behaviour.
func TestEnabledToggleIsLiveWithoutRebuild(t *testing.T) {
	fired := 0
	item := NewMenuItem("New", func() { fired++ }).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File", item))

	item.Enabled = false
	if bar.HandleAccelerator(ctrlN()) {
		t.Fatal("disabling an item must take effect live (no rebuild) — accelerator must not fire")
	}
	item.Enabled = true
	if !bar.HandleAccelerator(ctrlN()) || fired != 1 {
		t.Fatalf("re-enabling must take effect live; fired=%d", fired)
	}
}

// OnSelect swaps are read LIVE too.
func TestOnSelectSwapIsLiveWithoutRebuild(t *testing.T) {
	item := NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File", item))
	swapped := false
	item.OnSelect = func() { swapped = true }
	bar.HandleAccelerator(ctrlN())
	if !swapped {
		t.Fatal("a swapped OnSelect must be invoked live by the accelerator")
	}
}

// FINDING (behaviour change vs pre-refactor): the chord is SNAPSHOTTED at
// registration time. Re-binding an item's Shortcut after construction is NOT
// reflected until RebuildBindings() — the pre-refactor matchShortcut read
// item.Shortcut live every keystroke, so it would have matched the new chord
// immediately. This test pins the new (stale) contract so the regression is
// explicit; an app that mutates chords in place MUST call RebuildBindings.
func TestChordRebindIsStaleUntilRebuild(t *testing.T) {
	item := NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File", item))

	// Re-bind to Ctrl+M in place, WITHOUT calling RebuildBindings.
	item.WithShortcut("Ctrl+M", tui.KeyRune, 'm', true)

	// New chord does NOT fire yet; the stale snapshot still matches the OLD chord.
	if bar.HandleAccelerator(ctrlM()) {
		t.Fatal("Ctrl+M should not fire before RebuildBindings (chord is snapshotted)")
	}
	if !bar.HandleAccelerator(ctrlN()) {
		t.Fatal("stale registry still matches the OLD Ctrl+N chord until rebuild — pinning the behaviour change")
	}

	// After RebuildBindings the new chord is live and the old one is gone.
	bar.RebuildBindings()
	if !bar.HandleAccelerator(ctrlM()) {
		t.Fatal("after RebuildBindings the new Ctrl+M chord must fire")
	}
	if bar.HandleAccelerator(ctrlN()) {
		t.Fatal("after RebuildBindings the old Ctrl+N chord must no longer match")
	}
}

// FINDING (same class): ActionID is snapshotted at registration; changing it in
// place is not reflected by Match until RebuildBindings.
func TestActionIDIsSnapshottedUntilRebuild(t *testing.T) {
	item := NewMenuItem("New", func() {}).
		WithShortcut("Ctrl+N", tui.KeyRune, 'n', true).
		WithActionID("file.new")
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File", item))

	item.WithActionID("file.create") // mutate in place, no rebuild
	got, _ := bar.Registry().Match(ctrlN())
	if got.ActionID != "file.new" {
		t.Fatalf("ActionID is snapshotted; Match should still report %q, got %q", "file.new", got.ActionID)
	}
	bar.RebuildBindings()
	got, _ = bar.Registry().Match(ctrlN())
	if got.ActionID != "file.create" {
		t.Fatalf("after RebuildBindings Match should report %q, got %q", "file.create", got.ActionID)
	}
}

// --- RebuildBindings hygiene: idempotent, picks up structural changes, drops extras. ---

func TestRebuildBindingsIsIdempotentNoDuplicates(t *testing.T) {
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File",
			NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true),
			NewMenuItem("Open", func() {}).WithShortcut("Ctrl+O", tui.KeyRune, 'o', true),
		),
	)
	want := bar.Registry().Len()
	if want != 2 {
		t.Fatalf("expected 2 bindings after construction, got %d", want)
	}
	bar.RebuildBindings()
	bar.RebuildBindings()
	if got := bar.Registry().Len(); got != want {
		t.Fatalf("RebuildBindings must Clear before re-registering (no accumulation): Len=%d, want %d", got, want)
	}
}

func TestRebuildBindingsPicksUpAddedItem(t *testing.T) {
	sub := NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true))
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, sub)

	// A newly-added item's accelerator is inert until RebuildBindings.
	openFired := 0
	sub.Children = append(sub.Children, NewMenuItem("Open", func() { openFired++ }).WithShortcut("Ctrl+M", tui.KeyRune, 'm', true))
	if bar.HandleAccelerator(ctrlM()) {
		t.Fatal("a structurally-added item must be inert before RebuildBindings")
	}
	bar.RebuildBindings()
	if !bar.HandleAccelerator(ctrlM()) || openFired != 1 {
		t.Fatalf("after RebuildBindings the added item must fire; openFired=%d", openFired)
	}
}

func TestRebuildBindingsDropsCallerRegistrations(t *testing.T) {
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	bar.Registry().Register(KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'k', Ctrl: true}}, func() bool { return true })
	bar.RebuildBindings()
	if _, ok := bar.Registry().Match(tui.TypeEvent{Key: tui.KeyRune, Rune: 'k', Ctrl: true}); ok {
		t.Fatal("RebuildBindings resets to menu bindings, so caller-registered extras are dropped (documented)")
	}
	// The menu binding survives the rebuild.
	if _, ok := bar.Registry().Match(ctrlN()); !ok {
		t.Fatal("the menu's own binding must survive RebuildBindings")
	}
}
