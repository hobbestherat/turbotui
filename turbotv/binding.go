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

// Scope declares WHERE in the Desktop dispatch chain a binding is consulted. It
// makes explicit what Phase 1 left implicit (a binding's "scope" was an emergent
// property of which dispatch position claimed the key):
//
//   - ScopeGlobal     — the zero value and Phase-1 default. Menu accelerators
//     (e.g. Ctrl+N) live here; they fire from the menu-accelerator stage
//     regardless of focus, before the focused widget sees the key.
//   - ScopeFocus      — consulted at the focused-widget stage, scoped to a Target
//     component: the binding fires only when focus is within that Target and the
//     focused widget itself declined the key. gogent uses this in phase 4 to scope
//     transcript-letter keys to the session window.
//   - ScopeFallthrough — consulted at the unhandledKeyFn stage, after the focused
//     widget and focus navigation decline the key (the app-fallthrough point).
//
// Because Scope is an int whose zero value is ScopeGlobal, every Phase-1 binding
// (which sets no Scope) stays Global and keeps its current behavior.
type Scope int

const (
	// ScopeGlobal is the default scope: menu accelerators, fired before the focused
	// widget irrespective of focus.
	ScopeGlobal Scope = iota
	// ScopeFocus is consulted at the focused-widget stage and only for a binding
	// whose Target contains the focused component (see KeyBinding.Target).
	ScopeFocus
	// ScopeFallthrough is consulted at the unhandledKeyFn stage, after the focused
	// widget and focus navigation decline the key.
	ScopeFallthrough
)

// KeyBinding ties a Chord to an opaque ActionID, qualified by the Scope that
// decides where in the dispatch chain it is consulted. It is the first-class
// representation of "this combo triggers this action" that the toolkit stores and
// matches; menu accelerators are registered as KeyBindings (see MenuBar.Registry).
//
// Scope defaults to ScopeGlobal, so a zero-valued binding is a Global menu-style
// accelerator — every Phase-1 binding keeps its behavior unchanged.
type KeyBinding struct {
	Chord    Chord
	ActionID ActionID
	// Scope decides which dispatch position consults this binding. The zero value
	// (ScopeGlobal) preserves Phase-1 menu-accelerator behavior.
	Scope Scope
	// Target identifies the owner of a ScopeFocus binding: the binding is consulted
	// only when the desktop's focused component is Target or a descendant of it
	// (focus-within). It is ignored for ScopeGlobal and ScopeFallthrough bindings.
	// A ScopeFocus binding with a nil Target never matches — this prevents an
	// unscoped Focus binding from stealing a key from every focused widget.
	Target *VisualComponent
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
//
// Dispatch is scope-agnostic: it considers every registered binding. The Phase-1
// menu registry holds only Global bindings, so this is the menu-accelerator path.
// The scope-filtered Focus/Fallthrough points use DispatchFocus/DispatchFallthrough.
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

// applies reports whether entry participates at the given scope for the given
// focused component. For ScopeFocus the binding's Target must contain focused
// (focus-within); the other scopes ignore focused. The chord is NOT checked here —
// callers compare it separately so Match* and Dispatch* share this filter.
func (e bindingEntry) applies(scope Scope, focused *VisualComponent) bool {
	if e.binding.Scope != scope {
		return false
	}
	if scope == ScopeFocus {
		return focusWithin(focused, e.binding.Target)
	}
	return true
}

// MatchFocus returns the first ScopeFocus binding whose chord matches event and
// whose Target contains focused (the focus-within rule); ok is false when none
// applies. It is the pure lookup for the focused-widget dispatch position (use
// DispatchFocus to fire the action).
func (r *BindingRegistry) MatchFocus(event tui.TypeEvent, focused *VisualComponent) (KeyBinding, bool) {
	for _, entry := range r.entries {
		if entry.applies(ScopeFocus, focused) && entry.binding.Chord.Matches(event) {
			return entry.binding, true
		}
	}
	return KeyBinding{}, false
}

// DispatchFocus fires the first live ScopeFocus binding whose chord matches event
// and whose Target contains focused, returning whether the event was consumed. It
// honours handler liveness exactly like Dispatch (a not-live handler is skipped,
// a nil handler consumes). The desktop calls it at the focused-widget stage, after
// the focused widget itself declines the key.
func (r *BindingRegistry) DispatchFocus(event tui.TypeEvent, focused *VisualComponent) bool {
	return r.dispatchScope(event, ScopeFocus, focused)
}

// MatchFallthrough returns the first ScopeFallthrough binding whose chord matches
// event; ok is false when none matches. It is the pure lookup for the unhandledKeyFn
// dispatch position (use DispatchFallthrough to fire the action).
func (r *BindingRegistry) MatchFallthrough(event tui.TypeEvent) (KeyBinding, bool) {
	for _, entry := range r.entries {
		if entry.applies(ScopeFallthrough, nil) && entry.binding.Chord.Matches(event) {
			return entry.binding, true
		}
	}
	return KeyBinding{}, false
}

// DispatchFallthrough fires the first live ScopeFallthrough binding whose chord
// matches event, returning whether the event was consumed. It honours handler
// liveness like Dispatch. The desktop calls it at the unhandledKeyFn stage, before
// the app's unhandled-key handler runs.
func (r *BindingRegistry) DispatchFallthrough(event tui.TypeEvent) bool {
	return r.dispatchScope(event, ScopeFallthrough, nil)
}

// dispatchScope is the shared scope-filtered dispatch loop behind DispatchFocus and
// DispatchFallthrough. focused is only consulted for ScopeFocus.
func (r *BindingRegistry) dispatchScope(event tui.TypeEvent, scope Scope, focused *VisualComponent) bool {
	for _, entry := range r.entries {
		if !entry.applies(scope, focused) || !entry.binding.Chord.Matches(event) {
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

// focusWithin reports whether focused is target or a descendant of target, walking
// the parent chain. It is the focus-within test a ScopeFocus binding uses to decide
// whether it owns the current focus. A nil target or nil focused yields false, so a
// Focus binding with no Target never matches.
func focusWithin(focused *VisualComponent, target *VisualComponent) bool {
	if target == nil || focused == nil {
		return false
	}
	for current := focused; current != nil; current = current.Parent() {
		if current == target {
			return true
		}
	}
	return false
}
