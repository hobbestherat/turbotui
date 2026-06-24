package tv

import (
	"fmt"
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

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

// conflictsWith reports whether c and other collide: i.e. whether some single event
// would Match both. Two chords collide when they constrain the same effective named-key
// axis (see effectiveKey), carry the same rune compared case-insensitively (so Ctrl+R
// and Ctrl+r are one chord), and share the same Ctrl/Shift/Alt modifiers. This is the
// equality the registry's conflict service uses to decide whether two bindings fight
// over the same combination.
func (c Chord) conflictsWith(other Chord) bool {
	return c.effectiveKey() == other.effectiveKey() &&
		unicodeLower(c.Rune) == unicodeLower(other.Rune) &&
		c.Ctrl == other.Ctrl &&
		c.Shift == other.Shift &&
		c.Alt == other.Alt
}

// effectiveKey is the named-key axis a chord actually constrains, normalised so the two
// equivalent spellings of a printable/control chord compare equal. Chord.Matches forces
// event.Key == KeyRune whenever Rune is non-zero, so a rune-bearing chord constrains the
// KeyRune axis regardless of whether its Key field was left KeyUnknown (the wildcard) or
// set explicitly to KeyRune. Without this normalisation {Rune:'n', Ctrl:true} and
// {Key:KeyRune, Rune:'n', Ctrl:true} — which Match exactly the same events — would be
// treated as non-conflicting, so the conflict service could miss a real overlap.
func (c Chord) effectiveKey() tui.KeyCode {
	if c.Rune != 0 {
		return tui.KeyRune
	}
	return c.Key
}

// Deliverable reports whether this chord can actually be delivered to the application
// by a terminal in raw mode, and a human-readable reason when it cannot (for a capture
// UI to show). It is a thin delegation to tui.Deliverability, the single source of
// truth that lives next to the byte→TypeEvent decoder, so the terminal-ambiguity
// knowledge (Ctrl+M==Enter, Ctrl+[==Esc, Ctrl+Z→SIGTSTP, Ctrl+S/Q flow control,
// Ctrl+Shift+letter indistinguishable from Ctrl+letter, …) is never duplicated here.
//
// ok=true means the chord is bindable; ok=false means a binding on it could never fire
// distinctly. Ordinary combinations (Ctrl+N, Ctrl+F, plain letters, function keys)
// report true with an empty reason.
func (c Chord) Deliverable() (ok bool, reason string) {
	return tui.Deliverability(c.Key, c.Rune, c.Ctrl, c.Shift, c.Alt)
}

// String renders the chord in conventional "Ctrl+Shift+Alt+Key" notation for error
// messages, conflict warnings, and the future customizer UI. Modifiers are emitted in
// the stable order Ctrl, Alt, Shift; the key is the rune (upper-cased for letters) or
// a named-key label.
func (c Chord) String() string {
	var parts []string
	if c.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if c.Alt {
		parts = append(parts, "Alt")
	}
	if c.Shift {
		parts = append(parts, "Shift")
	}
	parts = append(parts, c.keyLabel())
	return strings.Join(parts, "+")
}

// keyLabel is the bare key portion of String: the named-key label when Key is set, or
// the rune (upper-cased for ASCII letters so "Ctrl+R" reads naturally). An empty chord
// renders as "?".
func (c Chord) keyLabel() string {
	if c.Key != tui.KeyUnknown && c.Key != tui.KeyRune {
		if name, ok := keyNames[c.Key]; ok {
			return name
		}
		return fmt.Sprintf("Key(%d)", int(c.Key))
	}
	if c.Rune == 0 {
		return "?"
	}
	r := c.Rune
	if r >= 'a' && r <= 'z' {
		r -= 'a' - 'A'
	}
	return string(r)
}

// keyNames maps named KeyCodes to their conventional labels for Chord.String.
var keyNames = map[tui.KeyCode]string{
	tui.KeyEnter:     "Enter",
	tui.KeyTab:       "Tab",
	tui.KeyBackspace: "Backspace",
	tui.KeyEscape:    "Esc",
	tui.KeyBackTab:   "BackTab",
	tui.KeyUp:        "Up",
	tui.KeyDown:      "Down",
	tui.KeyLeft:      "Left",
	tui.KeyRight:     "Right",
	tui.KeyHome:      "Home",
	tui.KeyEnd:       "End",
	tui.KeyPageUp:    "PageUp",
	tui.KeyPageDown:  "PageDown",
	tui.KeyInsert:    "Insert",
	tui.KeyDelete:    "Delete",
	tui.KeyF1:        "F1",
	tui.KeyF2:        "F2",
	tui.KeyF3:        "F3",
	tui.KeyF4:        "F4",
	tui.KeyF5:        "F5",
	tui.KeyF6:        "F6",
	tui.KeyF7:        "F7",
	tui.KeyF8:        "F8",
	tui.KeyF9:        "F9",
	tui.KeyF10:       "F10",
	tui.KeyF11:       "F11",
	tui.KeyF12:       "F12",
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

// String returns the scope's name for error messages and the customizer UI.
func (s Scope) String() string {
	switch s {
	case ScopeGlobal:
		return "Global"
	case ScopeFocus:
		return "Focus"
	case ScopeFallthrough:
		return "Fallthrough"
	default:
		return fmt.Sprintf("Scope(%d)", int(s))
	}
}

// KeyBinding ties a Chord to an opaque ActionID, qualified by the Scope that
// decides where in the dispatch chain it is consulted. It is the first-class
// representation of "this combo triggers this action" that the toolkit stores and
// matches; global accelerators are registered as KeyBindings (see Desktop.Bindings).
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
// "what is a keybinding": the Desktop owns a single registry holding all three
// scopes (Global, Focus, Fallthrough), and each dispatch position filters it by
// scope via applies — so global accelerators, focus-scoped keys, and fallthrough
// bindings all live in one structure (gogent #401).
//
// Bindings are matched in registration order, so the first registered binding
// whose chord matches an event wins.
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

// Match returns the first registered ScopeGlobal binding whose chord matches event,
// regardless of whether its handler is live; ok is false when nothing matches. It
// is the pure chord -> binding lookup for the Global (menu-accelerator) scope — use
// Dispatch to actually fire the action, and MatchFocus/MatchFallthrough for the
// scoped lookups.
//
// Match is deliberately Global-only, not scope-agnostic: the registry holds all
// three scopes in one entries slice, but Match only ever surfaces Global bindings.
// This means a Focus or Fallthrough binding in the same registry is inert here
// rather than silently firing as a global accelerator — which would bypass
// focus-scoping and modal blocking. Use MatchFocus/MatchFallthrough for those.
func (r *BindingRegistry) Match(event tui.TypeEvent) (KeyBinding, bool) {
	for _, entry := range r.entries {
		if entry.applies(ScopeGlobal, nil) && entry.binding.Chord.Matches(event) {
			return entry.binding, true
		}
	}
	return KeyBinding{}, false
}

// Dispatch fires the first matching ScopeGlobal binding that is live and reports
// whether the event was consumed. Matching bindings are tried in registration order:
// a binding whose handler reports it was not live (returns false) is skipped and the
// next match is tried, so a disabled menu accelerator does not swallow the chord. A
// matched binding with a nil handler consumes the event without doing anything.
//
// Like Match, Dispatch is Global-only. The desktop's menu-accelerator stage calls it
// (Desktop.handleType) on the single Desktop registry, so this is exactly the
// global-accelerator dispatch. Focus/Fallthrough bindings in the same registry are
// inert here (never fire as a global accelerator); the scoped dispatch positions use
// DispatchFocus/DispatchFallthrough.
func (r *BindingRegistry) Dispatch(event tui.TypeEvent) bool {
	for _, entry := range r.entries {
		if !entry.applies(ScopeGlobal, nil) || !entry.binding.Chord.Matches(event) {
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

// ConflictError reports that a chord could not be bound to an action because another
// action already holds it in the same scope. It is the typed error Rebind returns so a
// customizer UI can name the conflicting action ("⚠ Already bound to <Holder>") and
// offer to reassign. Holder is the OTHER action already on the chord; ActionID is the
// action the caller was trying to (re)bind.
type ConflictError struct {
	Chord    Chord
	Scope    Scope
	ActionID ActionID
	Holder   ActionID
}

// Error renders the conflict, naming the holder, the chord, and the scope.
func (e *ConflictError) Error() string {
	return fmt.Sprintf("cannot bind %s to %q: already bound to %q in %s scope",
		e.Chord, e.ActionID, e.Holder, e.Scope)
}

// ConflictFor reports the action already holding chord in the given scope, if any. It
// is a NON-MUTATING query the customizer UI can call to warn before committing a
// rebind: ok is true and the returned ActionID names the holder when chord is already
// bound IN THE SAME scope, and false when the chord is free in that scope.
//
// Conflict is keyed on (chord, scope): a chord held only in a DIFFERENT scope is not a
// conflict (a Global Ctrl+R and a Focus-scope plain r coexist — they are consulted at
// different dispatch positions). Chord equality is case-insensitive on the rune and
// normalised across the Key axis for runes, and exact on the named key and
// Ctrl/Shift/Alt modifiers (see Chord.conflictsWith).
//
// ConflictFor excludes nothing: a customizer that pre-warns by calling
// ConflictFor(currentChord, scope) for the action it is editing gets that same action
// back as the holder, because the action's own current binding matches. Callers editing
// a known action should ignore a holder equal to that action (compare against
// BindingFor(actionID) or the row's ActionID). Use Rebind to commit, which excludes the
// target binding automatically.
func (r *BindingRegistry) ConflictFor(chord Chord, scope Scope) (ActionID, bool) {
	return r.conflictScan(chord, scope, -1, "")
}

// conflictScan is the shared lookup behind ConflictFor and Rebind. It returns the first
// registered binding in scope whose chord collides with chord, skipping the entry at
// excludeIdx (pass -1 to skip none) and any OTHER entry whose ActionID equals the
// non-empty excludeID. The two exclusions are independent on purpose:
//
//   - excludeIdx gives entry identity, so Rebind never reports the exact binding it is
//     rebinding as a self-conflict — this works even for an empty ActionID, which two
//     distinct anonymous bindings may share (the ActionID doc allows the empty value).
//   - excludeID is the multi-registration guard: an action registered more than once is
//     not reported as conflicting with its own other entries. It is skipped only when
//     non-empty, so distinct anonymous (empty-ActionID) bindings are NOT collapsed and a
//     genuine clash between two of them is still detected.
//
// ConflictFor passes (-1, "") to consider every entry; Rebind passes the target index
// and its ActionID.
func (r *BindingRegistry) conflictScan(chord Chord, scope Scope, excludeIdx int, excludeID ActionID) (ActionID, bool) {
	for i := range r.entries {
		if i == excludeIdx {
			continue
		}
		binding := r.entries[i].binding
		if binding.Scope != scope {
			continue
		}
		if excludeID != "" && binding.ActionID == excludeID {
			continue
		}
		if binding.Chord.conflictsWith(chord) {
			return binding.ActionID, true
		}
	}
	return "", false
}

// BindingFor returns the first binding registered for actionID. It is the read side of
// the conflict service the customizer UI needs: to render "action … current binding"
// and to resolve the row it is about to rebind. ok is false when no binding carries
// actionID. When an action is registered more than once (e.g. in two scopes) the first
// registered binding is returned, mirroring which entry Rebind targets.
func (r *BindingRegistry) BindingFor(actionID ActionID) (KeyBinding, bool) {
	for i := range r.entries {
		if r.entries[i].binding.ActionID == actionID {
			return r.entries[i].binding, true
		}
	}
	return KeyBinding{}, false
}

// Rebind changes the chord of the binding identified by actionID to chord, enforcing
// same-scope conflict detection. It is the toolkit's conflict service for a customizer
// UI: the rebind keeps the binding's existing Scope and Target (only the Chord changes)
// and is checked against the other bindings IN THAT SAME scope.
//
// Outcomes:
//   - chord is free in the binding's scope (or only used in a different scope) → the
//     chord is updated and nil is returned.
//   - chord is the action's OWN current chord → no-op success (nil). A self-rebind is
//     never a conflict.
//   - chord is already held by a DIFFERENT action in the same scope → nothing changes
//     and a *ConflictError naming that holder is returned.
//   - actionID is not registered → a descriptive error is returned and nothing changes.
//
// Rebind matches the FIRST binding registered with actionID and only that binding;
// register a distinct, non-empty ActionID per rebindable action so the target is
// unambiguous. The conflict check excludes the target binding by its entry identity
// (so a self-rebind succeeds even for the documented empty ActionID) and, when actionID
// is non-empty, every other entry sharing it (so an action registered more than once is
// never reported as conflicting with itself).
func (r *BindingRegistry) Rebind(actionID ActionID, chord Chord) error {
	idx := -1
	for i := range r.entries {
		if r.entries[i].binding.ActionID == actionID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("rebind: no binding registered for action %q", actionID)
	}
	scope := r.entries[idx].binding.Scope
	if holder, ok := r.conflictScan(chord, scope, idx, actionID); ok {
		return &ConflictError{Chord: chord, Scope: scope, ActionID: actionID, Holder: holder}
	}
	r.entries[idx].binding.Chord = chord
	return nil
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
