package tv

import tui "github.com/hobbestherat/turbotui"

// ActionID is an opaque identifier naming the action a key binding triggers.
// turbotui stores and matches ActionIDs without interpreting them; an application
// (e.g. gogent) maps each ActionID to the behaviour it should run. The empty
// ActionID is allowed: in phase 1 menu items carry their runnable action directly
// (via OnSelect), so an ActionID is optional and only labels the binding.
type ActionID string

// Chord is a single key combination: a named key (Key, e.g. KeyF1) or a
// printable/control rune (Rune) together with the Ctrl/Shift/Alt modifier flags.
// It is exactly the set of fields the menu accelerator matcher compares an
// incoming TypeEvent against, lifted out of MenuShortcut so the same comparison
// can be reused by the BindingRegistry (and by future binding scopes) instead of
// being duplicated.
type Chord struct {
	Key   tui.KeyCode
	Rune  rune
	Ctrl  bool
	Shift bool
	Alt   bool
}

// Matches reports whether event is this chord. A zero Key (KeyUnknown) is a
// wildcard on the named-key axis, so a rune-only chord ignores event.Key; a
// non-zero Rune is compared case-insensitively and only against KeyRune events.
// The Ctrl/Shift/Alt modifiers must match exactly, so Ctrl+N never matches a plain
// N. This is the single source of truth shared by the menu accelerator path and
// the registry, so the two can never drift (the dispatch tests pin this contract).
func (c Chord) Matches(event tui.TypeEvent) bool {
	if c.Key != tui.KeyUnknown && event.Key != c.Key {
		return false
	}
	if c.Rune != 0 {
		if event.Key != tui.KeyRune || unicodeLower(event.Rune) != unicodeLower(c.Rune) {
			return false
		}
	}
	if c.Ctrl != event.Ctrl {
		return false
	}
	if c.Shift != event.Shift {
		return false
	}
	if c.Alt != event.Alt {
		return false
	}
	return true
}

// KeyBinding ties a Chord to an opaque ActionID. It is the first-class
// representation of "this combo triggers this action" that the toolkit stores and
// matches; menu accelerators are registered as KeyBindings (see MenuBar.Registry).
type KeyBinding struct {
	Chord    Chord
	ActionID ActionID
}

// BindingRegistry stores key bindings and resolves an incoming event chord to the
// binding (and ActionID) it triggers. It is the toolkit's first-class home for
// "what is a keybinding": the menu accelerator path consults it instead of walking
// per-item shortcut fields, and later binding scopes can register into the same
// structure.
//
// Bindings are matched in registration order, so the first registered binding
// whose chord matches an event wins. The menu builds its registry in menu-tree
// pre-order, which preserves the first-match-wins semantics of the old recursive
// walk exactly.
type BindingRegistry struct {
	entries []bindingEntry
}

// bindingEntry pairs a stored binding with an optional handler. The handler lets
// the registry both look a chord up (Match) and act on it (Dispatch): it returns
// true when it consumed the event and false when it was not live (e.g. a disabled
// menu item), in which case Dispatch continues to the next matching binding —
// mirroring the menu's "disabled accelerator is skipped, not swallowed" rule.
type bindingEntry struct {
	binding KeyBinding
	handler func() bool
}

// NewBindingRegistry returns an empty registry.
func NewBindingRegistry() *BindingRegistry {
	return &BindingRegistry{}
}

// Register adds a binding. handler may be nil for a match-only binding (one that
// participates in Match lookups and, when dispatched, simply consumes the event
// without doing anything). A non-nil handler returns true when it consumed the
// event and false when it was not live; see bindingEntry.
func (r *BindingRegistry) Register(binding KeyBinding, handler func() bool) {
	r.entries = append(r.entries, bindingEntry{binding: binding, handler: handler})
}

// Clear drops every registered binding.
func (r *BindingRegistry) Clear() {
	r.entries = r.entries[:0]
}

// Len reports how many bindings are registered.
func (r *BindingRegistry) Len() int {
	return len(r.entries)
}

// Match returns the first registered binding whose chord matches event,
// regardless of whether its handler is live; ok is false when nothing matches. It
// is the pure chord -> binding lookup (use Dispatch to actually fire the action).
func (r *BindingRegistry) Match(event tui.TypeEvent) (KeyBinding, bool) {
	for _, entry := range r.entries {
		if entry.binding.Chord.Matches(event) {
			return entry.binding, true
		}
	}
	return KeyBinding{}, false
}

// Dispatch fires the first matching binding that is live and reports whether the
// event was consumed. Matching bindings are tried in registration order: a binding
// whose handler reports it was not live (returns false) is skipped and the next
// match is tried, so a disabled menu accelerator does not swallow the chord. A
// matched binding with a nil handler consumes the event without doing anything.
func (r *BindingRegistry) Dispatch(event tui.TypeEvent) bool {
	for _, entry := range r.entries {
		if !entry.binding.Chord.Matches(event) {
			continue
		}
		if entry.handler == nil {
			return true
		}
		if entry.handler() {
			return true
		}
	}
	return false
}
