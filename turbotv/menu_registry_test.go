package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Phase-1 integration tests: menu items register their existing shortcuts into a
// BindingRegistry, and MenuBar.HandleAccelerator / Desktop dispatch resolve a chord
// to the firing item THROUGH that registry. These pin behaviour preservation —
// every assertion is something the pre-refactor walk also guaranteed.

// --- MenuShortcut -> Chord bridge (rendering hint stays separate). ---

func TestMenuShortcutChordBridgesAllFields(t *testing.T) {
	sc := &MenuShortcut{Display: "Ctrl+Shift+Alt+S", Key: tui.KeyRune, Rune: 's', Ctrl: true, Shift: true, Alt: true}
	c := sc.Chord()
	want := Chord{Key: tui.KeyRune, Rune: 's', Ctrl: true, Shift: true, Alt: true}
	if c != want {
		t.Fatalf("Chord() = %+v, want %+v", c, want)
	}
	// The Display string is NOT part of the chord — it stays purely for rendering.
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
	// matchShortcut must agree with Chord.Matches for every case — they are now one
	// comparison and must never drift.
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
	// nil shortcut: matchShortcut short-circuits to false regardless of the event.
	if matchShortcut(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'}, nil) {
		t.Fatal("matchShortcut(nil) must be false")
	}
}

// --- WithActionID + ActionID carry. ---

func TestWithActionIDTagsItemAndDefaultsEmpty(t *testing.T) {
	plain := NewMenuItem("New", nil)
	if plain.ActionID != "" {
		t.Fatalf("default ActionID = %q, want empty", plain.ActionID)
	}
	tagged := NewMenuItem("New", nil).WithActionID("file.new")
	if tagged.ActionID != "file.new" {
		t.Fatalf("WithActionID set %q, want %q", tagged.ActionID, "file.new")
	}
	// WithActionID is chainable with WithShortcut and additive (does not disturb it).
	chained := NewMenuItem("New", nil).
		WithShortcut("Ctrl+N", tui.KeyRune, 'n', true).
		WithActionID("file.new")
	if chained.Shortcut == nil || chained.Shortcut.Display != "Ctrl+N" || chained.ActionID != "file.new" {
		t.Fatalf("WithActionID must compose with WithShortcut: %#v / %q", chained.Shortcut, chained.ActionID)
	}
}

// --- MenuBar.Registry: pre-order, only items with shortcuts, nesting. ---

func TestRegistryOnlyIncludesItemsWithShortcuts(t *testing.T) {
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File",
			NewMenuItem("New", nil).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true),
			NewMenuItem("No shortcut", nil), // not registered
			NewSeparator(),                  // no shortcut, not registered
			NewMenuItem("Open", nil).WithShortcut("Ctrl+O", tui.KeyRune, 'o', true),
		),
	)
	if got := bar.Registry().Len(); got != 2 {
		t.Fatalf("Registry Len = %d, want 2 (only shortcut-bearing items)", got)
	}
}

func TestRegistryWalksNestedChildrenInPreOrder(t *testing.T) {
	// Pre-order: a parent's own shortcut registers before its children, and an
	// earlier sibling subtree before a later one. Build duplicate Ctrl+N bindings at
	// known positions and confirm the FIRST in pre-order is the one Dispatch fires.
	fired := ""
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&Edit",
			NewMenuItem("Outer", func() { fired = "outer" }).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true),
			NewSubMenu("More",
				NewMenuItem("Inner", func() { fired = "inner" }).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true),
			),
		),
	)
	reg := bar.Registry()
	if reg.Len() != 2 {
		t.Fatalf("expected 2 registered bindings, got %d", reg.Len())
	}
	if !reg.Dispatch(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true}) {
		t.Fatal("Ctrl+N must dispatch")
	}
	if fired != "outer" {
		t.Fatalf("pre-order first-match must fire the outer item, fired %q", fired)
	}
}

// --- HandleAccelerator through the registry. ---

func TestHandleAcceleratorResolvesThroughRegistry(t *testing.T) {
	fired := 0
	item := NewMenuItem("New", func() { fired++ }).
		WithShortcut("Ctrl+N", tui.KeyRune, 'n', true).
		WithActionID("file.new")
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File", item))

	// The chord resolves to its ActionID via the registry's Match...
	got, ok := bar.Registry().Match(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true})
	if !ok || got.ActionID != "file.new" {
		t.Fatalf("registry Match = %q ok=%v, want file.new", got.ActionID, ok)
	}
	// ...and HandleAccelerator fires the item through that same registry.
	if !bar.HandleAccelerator(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true}) {
		t.Fatal("HandleAccelerator must consume the registered Ctrl+N")
	}
	if fired != 1 {
		t.Fatalf("OnSelect fired %d times, want 1", fired)
	}

	// The item KEEPS its display shortcut for rendering — the match path going
	// through the registry must not strip it.
	if item.Shortcut == nil || item.Shortcut.Display != "Ctrl+N" {
		t.Fatal("menu item must keep its display shortcut after registry dispatch")
	}

	// A non-registered chord is not consumed.
	if bar.HandleAccelerator(tui.TypeEvent{Key: tui.KeyRune, Rune: 'x', Ctrl: true}) {
		t.Fatal("an unregistered chord must not be consumed")
	}
	// Modifier sensitivity end-to-end: plain N must not fire Ctrl+N.
	if bar.HandleAccelerator(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n'}) {
		t.Fatal("plain N must not fire the Ctrl+N accelerator")
	}
	if fired != 1 {
		t.Fatalf("only the real Ctrl+N should have fired; count=%d", fired)
	}
}

func TestHandleAcceleratorResetsOpenPaths(t *testing.T) {
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)),
	)
	// Pretend a menu is hovered/open, then fire an accelerator: the old behaviour
	// clears the open/hover paths.
	bar.openPath = []int{0}
	bar.hoverPath = []int{0, 0}
	if !bar.HandleAccelerator(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true}) {
		t.Fatal("accelerator should fire")
	}
	if len(bar.openPath) != 0 || len(bar.hoverPath) != 0 {
		t.Fatalf("HandleAccelerator must reset openPath/hoverPath, got %v / %v", bar.openPath, bar.hoverPath)
	}
}

// --- Disabled accelerator: skipped, not swallowed. ---

func TestDisabledAcceleratorIsSkippedNotSwallowed(t *testing.T) {
	enabledFired := 0
	disabled := NewMenuItem("Disabled New", func() { t.Fatal("disabled item must not fire") }).
		WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)
	disabled.Enabled = false
	enabled := NewMenuItem("Enabled New", func() { enabledFired++ }).
		WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File", disabled, enabled),
	)
	if !bar.HandleAccelerator(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true}) {
		t.Fatal("a later enabled duplicate must still fire Ctrl+N")
	}
	if enabledFired != 1 {
		t.Fatalf("enabled duplicate fired %d times, want 1", enabledFired)
	}
}

func TestLoneDisabledAcceleratorIsNotConsumed(t *testing.T) {
	// A single disabled accelerator must report the chord unconsumed so the dispatch
	// chain continues past it (falls through to the focused widget etc.).
	disabled := NewMenuItem("Disabled", func() { t.Fatal("must not fire") }).
		WithShortcut("Ctrl+N", tui.KeyRune, 'n', true)
	disabled.Enabled = false
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1}, NewSubMenu("&File", disabled))
	if bar.HandleAccelerator(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true}) {
		t.Fatal("a lone disabled accelerator must not be consumed")
	}
}

// --- Function-key accelerator through the registry. ---

func TestFunctionKeyAcceleratorThroughRegistry(t *testing.T) {
	fired := 0
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&Help",
			NewMenuItem("Help", func() { fired++ }).WithShortcutMod("F1", tui.KeyF1, 0, false, false, false),
		),
	)
	if got, ok := bar.Registry().Match(tui.TypeEvent{Key: tui.KeyF1}); !ok || got.Chord.Key != tui.KeyF1 {
		t.Fatalf("F1 must resolve in the registry: %+v ok=%v", got, ok)
	}
	if !bar.HandleAccelerator(tui.TypeEvent{Key: tui.KeyF1}) || fired != 1 {
		t.Fatalf("F1 accelerator must fire once through the registry, fired=%d", fired)
	}
}

// --- Desktop.Bindings + end-to-end dispatch + modal blocking. ---

func TestDesktopBindingsNilWithoutMenuBar(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	if desktop.Bindings() != nil {
		t.Fatal("Desktop.Bindings() must be nil when no menubar is set")
	}
}

func TestDesktopBindingsExposesRegistry(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File",
			NewMenuItem("New", func() {}).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true).WithActionID("file.new"),
		),
	)
	desktop.SetMenuBar(bar)
	reg := desktop.Bindings()
	if reg == nil {
		t.Fatal("Desktop.Bindings() must expose the menubar registry")
	}
	got, ok := reg.Match(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true})
	if !ok || got.ActionID != "file.new" {
		t.Fatalf("desktop registry Match = %q ok=%v, want file.new", got.ActionID, ok)
	}
}

func TestDesktopHandleTypeRoutesAcceleratorThroughRegistry(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	fired := 0
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File",
			NewMenuItem("New", func() { fired++ }).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true),
		),
	)
	desktop.SetMenuBar(bar)
	base := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true})
	if fired != 1 {
		t.Fatalf("Ctrl+N accelerator must fire through Desktop dispatch once, got %d", fired)
	}
}

func TestModalLayerBlocksRegistryAccelerator(t *testing.T) {
	// menuInScope must still gate the accelerator: while a modal layer is on top the
	// registry is NOT consulted, so the menu accelerator does not fire. The registry
	// rationalizes the binding-shaped parts; it does not bypass modal blocking.
	desktop := newTestDesktop(t, 40, 12)
	fired := 0
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1},
		NewSubMenu("&File",
			NewMenuItem("New", func() { fired++ }).WithShortcut("Ctrl+N", tui.KeyRune, 'n', true),
		),
	)
	desktop.SetMenuBar(bar)
	base := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 12})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	// Sanity: without a modal it fires.
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true})
	if fired != 1 {
		t.Fatalf("precondition: accelerator should fire with no modal, got %d", fired)
	}

	dialog := NewComponent(Rect{X: 5, Y: 5, W: 20, H: 5})
	desktop.AddLayer(NewModalLayer("modal", dialog))
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'n', Ctrl: true})
	if fired != 1 {
		t.Fatalf("modal layer must block the registry accelerator, fired=%d", fired)
	}
}
