package tv

import (
	"context"
	"sort"
	"sync"
	"time"

	tui "github.com/hobbestherat/turbotui"
)

// DefaultModalEnterGrace is the modal Enter-grace window (gogent#347): the span after
// a modal appears during which Enter is ignored for its focused button. It is the
// desktop's default (every NewDesktop starts with it), so all modal dialogs benefit
// without per-app wiring; SetEnterGrace overrides it (0 disables the grace entirely).
const DefaultModalEnterGrace = 300 * time.Millisecond

// Desktop threading contract: the desktop and the widgets it hosts are driven by
// a single goroutine — the App event loop. Input handlers, the coalesced redraw
// and Run all run there. Every public method that mutates desktop state
// (AddLayer, RemoveLayer, RemoveTopLayer, RaiseLayer, SetFocus, SetMenuBar,
// SetWorkArea, ResetWorkArea, SetTheme, SetBackground, OnResize, Redraw, …) and
// any direct widget mutation MUST be performed on that goroutine. Background
// goroutines (timers, network, streaming) must funnel their updates through
// Desktop.Post, which executes the closure on the loop and then requests a
// coalesced redraw. The layer stack itself is additionally guarded by a mutex so
// that an off-loop AddLayer/RemoveLayer cannot corrupt the slice that compose and
// hit-testing read concurrently (issue #56); the per-widget mutable state reached
// through Post stays loop-confined.
type Desktop struct {
	app            *tui.App
	layersMu       sync.Mutex // guards the layers slice header (issue #56)
	layers         []*Layer
	backgroundCell tui.Cell
	theme          Theme
	focused        *VisualComponent
	mouseCapture   *VisualComponent
	menuBar        *MenuBar
	// bindings is the desktop's durable BindingRegistry for the Focus and
	// Fallthrough scopes (see ScopedBindings). Unlike the menubar's Global registry
	// (Bindings), it is NOT reset by a menu rebuild, so scoped bindings an app
	// registers survive RebuildBindings. It is lazily created by ScopedBindings.
	bindings       *BindingRegistry
	unhandledKeyFn func(event tui.TypeEvent)
	onResize       []func()
	// onActiveLayerChange is fired when the top input layer changes; lastNotifiedTop
	// records the layer it was last reported with so a no-op mutation (raising the
	// already-top layer, a click that does not reorder, a removal below the top)
	// does not fire it (issue #304 / gogent#304).
	onActiveLayerChange func(top *Layer)
	lastNotifiedTop     *Layer
	workArea            Rect
	// cancel stops the run loop; it backs the default Ctrl+C quit (issue #75) and is
	// set by Run.
	cancel context.CancelFunc
	// nowFn is the injectable time source backing the modal Enter-grace (gogent#347)
	// and typing-awareness (gogent#346) clocks. It defaults to time.Now; tests replace
	// it via SetClock so grace/typing windows are deterministic without wall-clock
	// sleeps. now() reads it (falling back to time.Now when nil).
	nowFn func() time.Time
	// enterGrace is the modal Enter-grace window (gogent#347), defaulting to
	// DefaultModalEnterGrace so the safety applies toolkit-wide; SetEnterGrace overrides
	// it and 0 disables suppression. It only ever affects a focused button on an armed
	// modal, so non-modal layers and non-button focus are untouched.
	enterGrace time.Duration
	// lastInputAt is the timestamp of the most recent text input the focused widget
	// consumed (gogent#346), stamped from now(). RecentlyTyped queries it so a consumer
	// can defer focus-stealing modals while the user is actively typing.
	lastInputAt time.Time
}

func NewDesktop(app *tui.App) *Desktop {
	desktop := &Desktop{
		app:            app,
		theme:          DefaultTheme,
		backgroundCell: tui.Cell{Ch: ' ', FG: activeTheme.DesktopFG, BG: activeTheme.DesktopBG},
		enterGrace:     DefaultModalEnterGrace,
	}
	app.OnResize(func(event tui.ResizeEvent) {
		desktop.handleResize(event)
	})
	app.OnClick(func(event tui.ClickEvent) {
		desktop.handleClick(event)
	})
	app.OnScroll(func(event tui.ScrollEvent) {
		desktop.handleScroll(event)
	})
	app.OnType(func(event tui.TypeEvent) {
		desktop.handleType(event)
	})
	app.OnPaste(func(event tui.PasteEvent) {
		desktop.handlePaste(event)
	})
	// Drive coalesced redraws (issue #17): the run loop calls this at most once per
	// iteration after draining a burst of posts, instead of one Apply per post.
	app.SetRedrawFn(func() {
		desktop.compose()
		desktop.updateCursor()
		_ = desktop.app.Apply()
	})
	return desktop
}

func (d *Desktop) App() *tui.App {
	return d.app
}

// layerSnapshot returns a copy of the layer-stack slice header under the mutex so
// readers (compose, hit-testing, focus traversal) iterate a stable list even if a
// concurrent off-loop mutator appends or rebuilds d.layers (issue #56). The
// elements are shared pointers; only the slice structure is copied.
func (d *Desktop) layerSnapshot() []*Layer {
	d.layersMu.Lock()
	defer d.layersMu.Unlock()
	out := make([]*Layer, len(d.layers))
	copy(out, d.layers)
	return out
}

// Post runs fn on the event-loop goroutine and then requests a redraw. Background
// tasks use it to safely update widgets (e.g. streaming text) and refresh the
// screen. The redraw is coalesced: a burst of posts results in a single recompose
// and a single terminal write rather than one per post (issue #17).
func (d *Desktop) Post(fn func()) {
	if fn == nil {
		return
	}
	d.app.Post(func() {
		fn()
		d.app.RequestRedraw()
	})
}

func (d *Desktop) Theme() Theme {
	return d.theme
}

func (d *Desktop) SetTheme(theme Theme) {
	d.theme = theme
	// Keep the package-level active theme in step so newly constructed widgets
	// and draw-time chrome (menus, popups, selections) resolve the same palette.
	SetTheme(theme)
	d.backgroundCell = tui.Cell{Ch: ' ', FG: theme.DesktopFG, BG: theme.DesktopBG}
}

func (d *Desktop) SetBackground(cell tui.Cell) {
	d.backgroundCell = cell
}

// now returns the current time from the injectable clock, falling back to the wall
// clock when none was set. It is the single time source for the modal Enter-grace
// (gogent#347) and typing-awareness (gogent#346) windows.
func (d *Desktop) now() time.Time {
	if d.nowFn != nil {
		return d.nowFn()
	}
	return time.Now()
}

// SetClock replaces the desktop's time source, used by the modal Enter-grace and the
// typing-awareness query. Pass nil to restore the wall clock (time.Now). It exists so
// tests can drive grace/typing windows deterministically without real sleeps; apps
// normally leave it at the default. Like the rest of the desktop it is loop-confined —
// set it before Run or via Post.
func (d *Desktop) SetClock(now func() time.Time) {
	d.nowFn = now
}

// SetEnterGrace overrides the modal Enter-grace window (gogent#347): for this long
// after a modal layer is shown, Enter is ignored for a focused button so an Enter the
// user had already begun (e.g. submitting a message) cannot activate a freshly-appeared
// dialog button. Escape, mouse clicks, every non-Enter key, Enter on a non-button field
// (which still bubbles to a dialog's default handler), and Enter after the window
// elapses all work normally. The desktop already defaults to DefaultModalEnterGrace, so
// this is for tuning (or, with 0, disabling) the window rather than enabling it.
func (d *Desktop) SetEnterGrace(grace time.Duration) {
	d.enterGrace = grace
}

// EnterGrace reports the current modal Enter-grace window (0 when disabled).
func (d *Desktop) EnterGrace() time.Duration {
	return d.enterGrace
}

// RecentlyTyped reports whether the user typed into the focused widget less than
// `within` ago (gogent#346). It is the minimal typing-awareness signal a consumer uses
// to defer a background-triggered modal while the user is mid-keystroke: text runes
// (without Ctrl/Alt), Backspace, Delete, and pastes the focused widget consumed all
// count as typing; navigation, shortcuts, and Enter do not. The signal decays purely by
// elapsed time, so it self-clears once the user goes idle. Returns false when nothing
// has been typed yet.
func (d *Desktop) RecentlyTyped(within time.Duration) bool {
	if d.lastInputAt.IsZero() {
		return false
	}
	return d.now().Sub(d.lastInputAt) < within
}

// SetMenuBar registers the application menubar. The desktop owns it: it is drawn
// on top of every layer and receives global shortcuts (Alt-mnemonics and
// Ctrl-accelerators) and keyboard navigation while open. Do NOT also add it to a
// layer.
//
// Installing a menubar re-syncs its BindingRegistry from the current menu tree, so
// a bar whose Menus were assembled or mutated before being set is reflected on the
// accelerator path without the caller having to call RebuildBindings. Mutating the
// tree of an already-installed bar still requires MenuBar.RebuildBindings.
func (d *Desktop) SetMenuBar(bar *MenuBar) {
	d.menuBar = bar
	if bar != nil {
		bar.Component.SetBounds(Rect{X: 0, Y: 0, W: d.app.Width(), H: 1})
		bar.RebuildBindings()
	}
}

// Bindings returns the active menubar's persistent BindingRegistry — the single
// instance the desktop consults for menu accelerators — or nil when no menubar is
// set. It is the toolkit's first-class view of "which chord triggers which action"
// and the seam later binding scopes will hang off; it does not change the dispatch
// chain in handleType (only MenuBar.HandleAccelerator consults it). The returned
// registry is owned by the menubar and is re-synced from the menu tree by
// MenuBar.RebuildBindings.
//
// Phase-1 caveat: the instance is stable, but its contents are owned by the menu
// tree. A caller may Register an extra binding and it persists until the next
// RebuildBindings, which resets the registry to the menu bindings and drops the
// extra. For durable non-menu scopes (Focus/Fallthrough) use ScopedBindings, which
// survives a menu rebuild.
//
// Scope correspondence: this registry is the Global scope. Its consumers (Match,
// Dispatch via HandleAccelerator) only ever surface ScopeGlobal bindings, so a
// Focus or Fallthrough binding registered here is inert — it will not fire as a
// global accelerator. Register scoped bindings into ScopedBindings instead.
//
// Like the rest of the desktop, the registry is loop-confined: query or mutate it
// (Match/Register/Clear) only on the event-loop goroutine or via Post, since the
// accelerator path reads it there (see the Desktop threading contract).
func (d *Desktop) Bindings() *BindingRegistry {
	if d.menuBar == nil {
		return nil
	}
	return d.menuBar.Registry()
}

// ScopedBindings returns the desktop's durable BindingRegistry for the Focus and
// Fallthrough scopes, creating it on first use (it is never nil). This is the
// phase-2 home the Bindings() doc anticipated: bindings registered here outlive a
// menu RebuildBindings, because they belong to the desktop, not the menu tree.
//
// handleType consults it at two existing dispatch positions: ScopeFocus bindings at
// the focused-widget stage (after the focused widget declines the key, scoped to the
// binding's Target via focus-within) and ScopeFallthrough bindings at the
// unhandledKeyFn stage (before the app's unhandled-key handler). With nothing
// registered here both checks are inert, so the dispatch chain is unchanged.
//
// Use Bindings() for Global menu accelerators; register Focus/Fallthrough bindings
// here. Scope correspondence: this registry is consulted ONLY through DispatchFocus
// and DispatchFallthrough, so a ScopeGlobal binding registered here is inert (it
// fires nowhere — register Global accelerators on the menu via Bindings()). The
// mismatch is benign (dead, not dangerous) but silent, so keep each scope in its
// own registry.
//
// Like the rest of the desktop, the registry is loop-confined: query or mutate it
// only on the event-loop goroutine or via Post (see the Desktop threading contract).
func (d *Desktop) ScopedBindings() *BindingRegistry {
	if d.bindings == nil {
		d.bindings = NewBindingRegistry()
	}
	return d.bindings
}

// AddLayer pushes a layer onto the top of the stack. Must be called on the event
// loop or via Post (see the Desktop threading contract).
//
// A Modal layer additionally (a) pushes the currently-focused widget onto the focus-
// history stack so closing the modal can restore it (gogent#348), and (b) is armed with
// the current clock time for the Enter-grace window (gogent#347).
func (d *Desktop) AddLayer(layer *Layer) {
	d.layersMu.Lock()
	d.layers = append(d.layers, layer)
	d.layersMu.Unlock()
	if layer.window != nil {
		layer.window.desktop = d
	}
	if layer.Modal {
		// Remember the pre-modal focus on the layer itself so closing it can restore
		// that widget (gogent#348), and arm the Enter-grace window from the (injectable)
		// clock unless the layer opted out (user-initiated dialogs, gogent#347).
		layer.restoreFocus = d.focused
		if !layer.NoEnterGrace {
			layer.armedAt = d.now()
		}
	}
	if layer.FullScreen {
		layer.Root.SetBounds(Rect{X: 0, Y: 0, W: d.app.Width(), H: d.app.Height()})
	}
	d.notifyActiveLayerChange()
	d.Redraw()
}

// WorkArea is the region windows are constrained to when dragged, resized or
// maximized (unless a window sets its own ConstrainTo). It defaults to the whole
// desktop minus the menu-bar row, and can be narrowed with SetWorkArea to reserve
// a region (e.g. a pinned sidebar) that windows must keep clear.
func (d *Desktop) WorkArea() Rect {
	if !d.workArea.Empty() {
		return d.workArea
	}
	top := 0
	if d.menuBar != nil {
		top = 1
	}
	height := d.app.Height() - top
	if height < 0 {
		height = 0
	}
	return Rect{X: 0, Y: top, W: d.app.Width(), H: height}
}

// SetWorkArea reserves the area outside r: windows constrained to the desktop can
// no longer be dragged, resized or maximized over it. Pass an empty rect (or call
// ResetWorkArea) to fall back to the default full-desktop work area.
func (d *Desktop) SetWorkArea(r Rect) {
	d.workArea = r
	d.Redraw()
}

// ResetWorkArea clears a reserved region set with SetWorkArea.
func (d *Desktop) ResetWorkArea() {
	d.workArea = Rect{}
	d.Redraw()
}

// OnResize registers a callback fired (on the event loop) after the terminal is
// resized and every windowed layer has been clamped back into view. It is the
// sanctioned hook for apps that want to reflow their own chrome — use it instead
// of reaching into App.OnResize, which fights the desktop's own resize handling
// (issue #71). Multiple callbacks may be registered; they run in registration
// order. Must be called on the event loop or via Post.
func (d *Desktop) OnResize(fn func()) {
	if fn == nil {
		return
	}
	d.onResize = append(d.onResize, fn)
}

// OnActiveLayerChange registers a callback fired (on the event loop) whenever the
// desktop's top input layer changes — raised by a click, a programmatic
// RaiseLayer, or an AddLayer/RemoveLayer/RemoveTopLayer that moves a different
// layer to the top. It receives the new top layer (nil when the stack is empty).
// It is a member of the desktop's "the desktop did something, the app may react"
// callback family (OnResize, SetUnhandledKeyFn, per-layer OnClickOutside) and the
// sanctioned way to re-sync state derived from the active window (e.g. a sidebar
// selection) without instrumenting every activation path.
//
// The callback fires at most once per mutation and never when the top layer is
// unchanged, so a single user gesture (a click, one mutator call) yields one
// notification carrying the final top. It runs on the event loop, after the layer
// stack has been updated, so a handler reading TopLayer() observes the new top.
// Only one callback may be registered; a later call replaces it, and a nil fn
// disables notification. Must be called on the event loop or via Post.
func (d *Desktop) OnActiveLayerChange(fn func(top *Layer)) {
	d.onActiveLayerChange = fn
}

// notifyActiveLayerChange fires OnActiveLayerChange when the top input layer
// differs from the one last reported. Tracking the last-notified top dedupes
// no-op mutations and coalesces a single gesture down to one callback with the
// final top. It runs even when no callback is registered so that a callback
// registered after layers already exist is not fired spuriously on the next
// mutation. Call it on the event loop after the layer stack has been updated.
func (d *Desktop) notifyActiveLayerChange() {
	top := d.TopLayer()
	if top == d.lastNotifiedTop {
		return
	}
	d.lastNotifiedTop = top
	if d.onActiveLayerChange != nil {
		d.onActiveLayerChange(top)
	}
}

// handleResize is the desktop's terminal-resize handler. It first clamps every
// windowed (non-fullscreen) layer so its title bar stays on-screen and grabbable,
// then notifies each layer's OnResize hook and the desktop-level OnResize
// callbacks, and finally repaints. FullScreen layers are restretched by compose
// (issue #71).
func (d *Desktop) handleResize(_ tui.ResizeEvent) {
	d.clampLayers()
	for _, layer := range d.layerSnapshot() {
		if layer == nil || layer.Root == nil || layer.FullScreen {
			continue
		}
		if layer.OnResize != nil {
			layer.OnResize(layer.Root.Bounds)
		}
	}
	for _, fn := range d.onResize {
		if fn != nil {
			fn()
		}
	}
	d.Redraw()
}

// clampLayers pulls every windowed layer back inside the current viewport so a
// window that was positioned near the old (larger) bounds cannot end up clipped
// entirely off-screen after a shrink. Windows reuse their own constraint-aware
// clamp (keeping the title bar grabbable); plain layers keep a small handle of
// their top-left corner on screen.
func (d *Desktop) clampLayers() {
	viewW, viewH := d.app.Width(), d.app.Height()
	for _, layer := range d.layerSnapshot() {
		if layer == nil || layer.Root == nil || layer.FullScreen {
			continue
		}
		bounds := layer.Root.Bounds
		if layer.window != nil {
			nx, ny := layer.window.clampMove(bounds.X, bounds.Y, bounds.W)
			if nx != bounds.X || ny != bounds.Y {
				layer.Root.SetBounds(Rect{X: nx, Y: ny, W: bounds.W, H: bounds.H})
			}
			continue
		}
		if nx, ny := clampIntoView(bounds, viewW, viewH); nx != bounds.X || ny != bounds.Y {
			layer.Root.SetBounds(Rect{X: nx, Y: ny, W: bounds.W, H: bounds.H})
		}
	}
}

// clampIntoView returns a top-left for bounds that keeps at least a small handle
// of the rect within a viewW×viewH viewport (top/left edges never leave it).
func clampIntoView(bounds Rect, viewW int, viewH int) (int, int) {
	keep := bounds.W
	if keep > 4 {
		keep = 4
	}
	if keep < 1 {
		keep = 1
	}
	x, y := bounds.X, bounds.Y
	maxX := viewW - keep
	if maxX < 0 {
		maxX = 0
	}
	if x > maxX {
		x = maxX
	}
	if x < 0 {
		x = 0
	}
	maxY := viewH - 1
	if maxY < 0 {
		maxY = 0
	}
	if y > maxY {
		y = maxY
	}
	if y < 0 {
		y = 0
	}
	return x, y
}

// RemoveTopLayer pops the topmost layer. Must be called on the event loop or via
// Post (see the Desktop threading contract).
func (d *Desktop) RemoveTopLayer() {
	d.layersMu.Lock()
	if len(d.layers) == 0 {
		d.layersMu.Unlock()
		return
	}
	removed := d.layers[len(d.layers)-1]
	d.layers = d.layers[:len(d.layers)-1]
	d.layersMu.Unlock()
	d.restoreFocusAfterRemoval(removed)
	d.notifyActiveLayerChange()
	d.Redraw()
}

// RemoveLayer removes layer from the stack. Must be called on the event loop or
// via Post (see the Desktop threading contract).
func (d *Desktop) RemoveLayer(layer *Layer) {
	if layer == nil {
		return
	}
	d.layersMu.Lock()
	removed := false
	next := make([]*Layer, 0, len(d.layers))
	for _, existing := range d.layers {
		if existing == layer {
			removed = true
			continue
		}
		next = append(next, existing)
	}
	d.layers = next
	d.layersMu.Unlock()
	if removed {
		d.restoreFocusAfterRemoval(layer)
	} else {
		d.ensureFocusInTopLayer()
	}
	d.notifyActiveLayerChange()
	d.Redraw()
}

// restoreFocusAfterRemoval re-homes keyboard focus once `removed` has left the stack
// (gogent#348). For a non-modal layer it keeps the existing clear-to-nil
// ensureFocusInTopLayer behaviour, leaving non-modal removal unchanged. For a Modal
// layer:
//   - If the focused widget still lives in the new top layer, the removed modal was
//     buried beneath the one the user is on, so focus is left untouched (closing a
//     background modal must not yank focus off the active one).
//   - Otherwise the modal that held focus is closing: re-focus the widget recorded on
//     that layer before it opened, if it is still focusable in the new top layer; else
//     fall back to clear-to-nil. The target is stored per-layer, so removing modals
//     out of order cannot desynchronise restore targets.
func (d *Desktop) restoreFocusAfterRemoval(removed *Layer) {
	if removed == nil || !removed.Modal {
		d.ensureFocusInTopLayer()
		return
	}
	if d.focused != nil && d.focusableInTopLayer(d.focused) {
		return
	}
	if prev := removed.restoreFocus; prev != nil && d.focusableInTopLayer(prev) {
		d.setFocus(prev)
		return
	}
	d.ensureFocusInTopLayer()
}

// focusableInTopLayer reports whether c is currently a focusable widget within the top
// input layer (visible, enabled, and focus-ordered there) — the test used to decide
// whether a remembered pre-modal widget can be re-focused after a modal closes.
func (d *Desktop) focusableInTopLayer(c *VisualComponent) bool {
	for _, item := range d.focusablesInTopLayer() {
		if item == c {
			return true
		}
	}
	return false
}

func (d *Desktop) TopLayer() *Layer {
	d.layersMu.Lock()
	defer d.layersMu.Unlock()
	if len(d.layers) == 0 {
		return nil
	}
	return d.layers[len(d.layers)-1]
}

// RaiseLayer moves layer to the front of the z-stack, keeping it below any modal
// layers that are currently on top. Fullscreen (background) layers are never
// raised so they stay behind real windows. It is a no-op when the layer is
// already as high as it is allowed to go.
func (d *Desktop) RaiseLayer(layer *Layer) {
	if d.raiseLayer(layer) {
		d.ensureFocusInTopLayer()
		d.notifyActiveLayerChange()
		d.Redraw()
	}
}

// raiseLayer performs the reordering without redrawing, returning true when the
// stack changed. Callers that already redraw (e.g. handleClick) use this form.
func (d *Desktop) raiseLayer(layer *Layer) bool {
	if layer == nil || layer.FullScreen {
		return false
	}
	d.layersMu.Lock()
	defer d.layersMu.Unlock()
	index := -1
	for i, existing := range d.layers {
		if existing == layer {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	// Target position: above every other layer, but below any modal layers that
	// currently sit at the very top of the stack.
	insert := len(d.layers) - 1
	for insert > 0 && d.layers[insert].Modal && d.layers[insert] != layer {
		insert--
	}
	if index == insert {
		return false
	}
	d.layers = append(d.layers[:index], d.layers[index+1:]...)
	if insert > len(d.layers) {
		insert = len(d.layers)
	}
	d.layers = append(d.layers, nil)
	copy(d.layers[insert+1:], d.layers[insert:])
	d.layers[insert] = layer
	return true
}

// layerForComponent returns the layer whose root is an ancestor of c, or nil.
func (d *Desktop) layerForComponent(c *VisualComponent) *Layer {
	root := c
	for root.parent != nil {
		root = root.parent
	}
	for _, layer := range d.layerSnapshot() {
		if layer != nil && layer.Root == root {
			return layer
		}
	}
	return nil
}

// focusIntoLayer ensures the keyboard goes to the just-clicked layer. If the
// clicked target is itself focusable it gets focus; otherwise, when focus is not
// already inside this layer, the layer's first focusable widget is focused (so a
// click on a window's title bar or empty area still makes it typeable).
func (d *Desktop) focusIntoLayer(layer *Layer, target *VisualComponent) {
	if target != nil && target.Focusable {
		d.setFocus(target)
		return
	}
	if layer == nil {
		return
	}
	if d.focused != nil && componentInLayer(d.focused, layer) {
		return
	}
	var items []*VisualComponent
	collectFocusable(layer.Root, &items)
	sortFocusOrder(items)
	if len(items) > 0 {
		d.setFocus(items[0])
	}
}

// sortFocusOrder arranges focusables for Tab traversal: ascending TabIndex, then
// on-screen reading order (top-to-bottom, then left-to-right). Stable so equal
// keys keep their tree order, making the result deterministic (issues #50).
func sortFocusOrder(items []*VisualComponent) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.TabIndex != b.TabIndex {
			return a.TabIndex < b.TabIndex
		}
		ra, rb := a.AbsoluteBounds(), b.AbsoluteBounds()
		if ra.Y != rb.Y {
			return ra.Y < rb.Y
		}
		return ra.X < rb.X
	})
}

// componentInLayer reports whether c is the layer root or a descendant of it.
func componentInLayer(c *VisualComponent, layer *Layer) bool {
	if layer == nil {
		return false
	}
	for current := c; current != nil; current = current.parent {
		if current == layer.Root {
			return true
		}
	}
	return false
}

func (d *Desktop) Redraw() {
	d.compose()
	d.updateCursor()
	_ = d.app.Apply()
}

// RequestRedraw marks the desktop dirty without painting now. The run loop
// coalesces every request accumulated while draining one batch of input events
// (or posts) into a single compose + Apply per iteration (issue #17). The
// input-event handlers use it instead of Redraw so a burst of mouse-motion
// reports — the terminal emits one per cell crossed during a drag (?1002h) —
// collapses into one repaint and one terminal write per read batch, instead of
// a full synchronous flush per event that lets the event queue outrun the
// drain and makes a dragged window trail the cursor (gogent#239). Redraw stays
// the right call for paths that must be on screen synchronously within the same
// handler; this is the hot-path counterpart that defers to the loop.
func (d *Desktop) RequestRedraw() {
	d.app.RequestRedraw()
}

// updateCursor positions the real terminal cursor at the focused widget's text
// caret (via its CursorFn), or hides it when no focused input exposes one or a
// menu is open.
func (d *Desktop) updateCursor() {
	menuOpen := d.menuBar != nil && d.menuBar.IsOpen()
	if !menuOpen && d.focused != nil && d.focused.visibleInTree() {
		if x, y, ok := d.focused.Cursor(); ok {
			d.app.SetCursor(x, y)
			return
		}
	}
	d.app.HideCursor()
}

func (d *Desktop) compose() {
	d.refreshMnemonics()
	d.app.Clear(d.backgroundCell)
	surface := newRootSurface(d.app)
	for _, layer := range d.layerSnapshot() {
		if layer == nil || layer.Root == nil {
			continue
		}
		if layer.FullScreen {
			layer.Root.SetBounds(Rect{X: 0, Y: 0, W: d.app.Width(), H: d.app.Height()})
		}
		layer.Root.Draw(surface)
	}
	// The menubar (and any dropped-down menu) always renders on top of every
	// layer, so windows never cover it.
	if d.menuBar != nil {
		d.menuBar.Component.SetBounds(Rect{X: 0, Y: 0, W: d.app.Width(), H: 1})
		d.menuBar.Component.Draw(surface)
	}
}

func (d *Desktop) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	defer cancel()
	d.compose()
	d.updateCursor()
	return d.app.Run(ctx)
}

// Quit stops the run loop. It is what the default Ctrl+C handler calls and is safe
// to invoke from a handler or a Post callback.
func (d *Desktop) Quit() {
	if d.cancel != nil {
		d.cancel()
	}
}

func (d *Desktop) handleClick(event tui.ClickEvent) {
	// The menubar sits above every layer, so it gets first claim on clicks that
	// fall on the bar or an open dropdown. A click anywhere else closes the menu.
	// It is gated by menuInScope so that, while a modal dialog is up, clicks on
	// row 0 behave like any other click outside the modal (the bar is unreachable
	// by mouse just as it is by keyboard).
	if d.menuInScope() && (d.menuBar.IsOpen() || d.menuBar.HitTest(event.X, event.Y)) {
		// Route both press and release to the menubar so leaf items can activate
		// on release (press-drag-release), letting the bar decide based on coords.
		_ = d.menuBar.Component.BubbleClick(event)
		d.RequestRedraw()
		return
	}
	if event.Down {
		target := d.mouseCapture
		if target == nil {
			target = d.hitTestTopLayer(event.X, event.Y)
		}
		if target != nil {
			newPress := d.mouseCapture == nil
			if newPress {
				d.mouseCapture = target
				// A fresh press on a window raises it to the front and routes the
				// keyboard into it, so clicking anywhere on a background window
				// both surfaces it and makes it typeable.
				if layer := d.layerForComponent(target); layer != nil {
					d.raiseLayer(layer)
					d.focusIntoLayer(layer, target)
					// The raise may have moved a new layer to the top; let the app
					// re-sync state derived from the active window. notifyActiveLayerChange
					// dedupes the common case where the clicked window was already on top.
					d.notifyActiveLayerChange()
				} else if target.Focusable {
					d.setFocus(target)
				}
			} else if target.Focusable {
				d.setFocus(target)
			}
			_ = target.BubbleClick(event)
			d.RequestRedraw()
			return
		}
		d.mouseCapture = nil
		// The click missed every input layer. When a modal is on top it has swallowed
		// the click (hitTestTopLayer stops at it, issue #42); give the app a chance to
		// react via OnClickOutside instead of letting anything below activate.
		if top := d.topInputLayer(); top != nil && top.Modal && top.OnClickOutside != nil {
			top.OnClickOutside(top)
			// The app's outside-click handler may mutate visible state (flag an
			// error, nudge the dialog, …) without doing a layer operation of its own,
			// so request a coalesced redraw; it paints nothing if nothing changed.
			d.RequestRedraw()
		}
		return
	}
	target := d.mouseCapture
	if target == nil {
		target = d.hitTestTopLayer(event.X, event.Y)
	}
	if target != nil {
		_ = target.BubbleClick(event)
	}
	d.mouseCapture = nil
	d.RequestRedraw()
}

func (d *Desktop) handleScroll(event tui.ScrollEvent) {
	target := d.hitTestTopLayer(event.X, event.Y)
	if target == nil {
		return
	}
	if target.BubbleScroll(event) {
		d.RequestRedraw()
	}
}

func (d *Desktop) handleType(event tui.TypeEvent) {
	// A dropped-down menu captures the keyboard for navigation keys. Keys it does
	// not handle (Ctrl accelerators, function keys) fall through so global
	// shortcuts still fire while a menu is open.
	if d.menuBar != nil && d.menuBar.IsOpen() {
		if d.menuBar.HandleKey(event) {
			d.RequestRedraw()
			return
		}
	}
	// Ctrl accelerators from the menubar, unless a modal layer blocks it.
	if d.menuInScope() && d.menuBar.HandleAccelerator(event) {
		d.RequestRedraw()
		return
	}
	// Alt+mnemonic activation within the current scope (top layer + menubar).
	if event.Key == tui.KeyRune && event.Alt {
		if d.dispatchMnemonic(unicodeLower(event.Rune)) {
			d.RequestRedraw()
			return
		}
	}
	// Ctrl+C / Ctrl+Shift+C copies the focused widget's selection (or all of its
	// content) and is consumed only when there was something to copy, so it can
	// still fall through to an app quit handler otherwise.
	if isCopyKey(event) && d.copyFocused() {
		return
	}
	// Ctrl+X cuts the focused widget's selection: the widget deletes it and the
	// desktop puts the removed text on the clipboard. Consumed only when the
	// widget had something cuttable, so the keystroke otherwise falls through.
	if isCutKey(event) && d.cutFocused() {
		// The cut mutated the focused widget (it removed the selection), so the
		// screen must repaint. Copy above changes nothing visible and so requests
		// no redraw — the asymmetry is deliberate.
		d.RequestRedraw()
		return
	}
	// Ctrl+V reads the system clipboard and routes it through the focused widget's
	// paste path — the same path bracketed paste uses. Consumed only when there was
	// a focused widget to receive a non-empty read; a failed or empty clipboard read
	// is a graceful no-op (pasteClipboard handles its own redraw via handlePaste).
	if isPasteKey(event) && d.pasteClipboard() {
		return
	}
	// Only deliver to the focused widget when it (and all its ancestors) are
	// visible; a focused descendant of a just-hidden container must not receive
	// keystrokes. Hidden-focus is cleared on minimize, but guard here too so types
	// never leak to an off-screen widget.
	if d.focused != nil && d.focused.visibleInTree() {
		// Modal Enter-grace (gogent#347): for a short window after a modal appears,
		// swallow Enter for a focused button so a keystroke the user had already begun
		// (e.g. submitting a message) cannot activate the freshly-focused dialog button.
		// Only Enter on a button is affected — every other key reaches the widget below,
		// and Escape, mouse clicks, a focused field/input, and Enter after the grace
		// elapses all work normally. enterSuppressed encodes the full condition.
		if event.Key == tui.KeyEnter && d.enterSuppressed() {
			return
		}
		if d.focused.BubbleType(event) {
			// Typing-awareness (gogent#346): record when the focused TEXT field consumes
			// a text-editing key so RecentlyTyped can report the user is mid-keystroke.
			// Gated on focusedIsTextInput so activating a focused button with Space — a
			// plain rune the button consumes — does not masquerade as the user typing.
			if isTypingKey(event) && d.focusedIsTextInput() {
				d.lastInputAt = d.now()
			}
			d.RequestRedraw()
			return
		}
		// Focus-scope bindings: consulted only after the focused widget itself
		// declines the key, and only for a binding whose Target contains the focused
		// component (focus-within). No Focus bindings are registered by default, so
		// this is inert and the focused-widget stage behaves exactly as before.
		if d.bindings != nil && d.bindings.DispatchFocus(event, d.focused) {
			d.RequestRedraw()
			return
		}
	}
	switch event.Key {
	case tui.KeyTab:
		d.moveFocus(true)
		d.RequestRedraw()
		return
	case tui.KeyBackTab:
		d.moveFocus(false)
		d.RequestRedraw()
		return
	case tui.KeyLeft, tui.KeyRight, tui.KeyUp, tui.KeyDown:
		if d.moveFocusDirection(event.Key) {
			d.RequestRedraw()
			return
		}
	}
	// Fallthrough-scope bindings run at the app-fallthrough point, before the
	// app's unhandledKeyFn — the same dispatch position the app's global shortcuts
	// occupy. No Fallthrough bindings are registered by default, so this is inert and
	// the unhandledKeyFn/quit tail behaves exactly as before.
	if d.bindings != nil && d.bindings.DispatchFallthrough(event) {
		d.RequestRedraw()
		return
	}
	if d.unhandledKeyFn != nil {
		d.unhandledKeyFn(event)
		// Like the modal OnClickOutside callback, an app's global-shortcut handler
		// may change visible state without a layer operation; request a coalesced
		// redraw so it paints (a no-op flush if nothing changed).
		d.RequestRedraw()
		return
	}
	// With no app-supplied handler, Ctrl+C is the conventional interrupt. Raw mode
	// swallows the SIGINT terminals would normally send, so without this default
	// Ctrl+C would do nothing and the app would feel hung (issue #75). It only
	// reaches here when no focused widget consumed it for copy.
	if isQuitKey(event) {
		d.Quit()
	}
}

// isQuitKey reports the default quit chord: Ctrl+C (with or without Shift, since
// many terminals cannot distinguish Ctrl+C from Ctrl+Shift+C).
func isQuitKey(event tui.TypeEvent) bool {
	return event.Key == tui.KeyRune && event.Ctrl && unicodeLower(event.Rune) == 'c'
}

// SetUnhandledKeyFn registers a callback invoked when a key event was not
// consumed by the menu, focus navigation, copy, or the focused widget. Apps use
// it for global shortcuts (e.g. a Ctrl+C quit confirmation) without racing the
// widgets that might legitimately want the same key. Registering a handler
// REPLACES the built-in default (Ctrl+C quits): an app that still wants Ctrl+C to
// quit should call Quit from its handler.
func (d *Desktop) SetUnhandledKeyFn(fn func(event tui.TypeEvent)) {
	d.unhandledKeyFn = fn
}

func isCopyKey(event tui.TypeEvent) bool {
	return event.Key == tui.KeyRune && event.Ctrl && unicodeLower(event.Rune) == 'c'
}

func isCutKey(event tui.TypeEvent) bool {
	return event.Key == tui.KeyRune && event.Ctrl && unicodeLower(event.Rune) == 'x'
}

func isPasteKey(event tui.TypeEvent) bool {
	return event.Key == tui.KeyRune && event.Ctrl && unicodeLower(event.Rune) == 'v'
}

// copyFocused copies the focused component's CopyFn text to the clipboard,
// returning true when something was copied.
func (d *Desktop) copyFocused() bool {
	if d.focused == nil {
		return false
	}
	text, ok := d.focused.Copy()
	if !ok {
		return false
	}
	d.app.CopyToClipboard(text)
	return true
}

// cutFocused asks the focused component to cut (delete + return) its selection
// and puts the removed text on the clipboard, returning true when something was
// cut. It mirrors copyFocused so copy and cut stay symmetric.
func (d *Desktop) cutFocused() bool {
	if d.focused == nil {
		return false
	}
	text, ok := d.focused.Cut()
	if !ok {
		return false
	}
	d.app.CopyToClipboard(text)
	return true
}

// handlePaste routes a bracketed-paste block to the focused widget as literal
// text. A dropped-down menu swallows it (paste makes no sense in a menu).
func (d *Desktop) handlePaste(event tui.PasteEvent) {
	if d.menuBar != nil && d.menuBar.IsOpen() {
		return
	}
	if d.focused == nil {
		return
	}
	if d.focused.BubblePaste(event.Text) {
		// A consumed paste into a text field is text input, so it counts toward
		// typing-awareness (gogent#346) just like keystrokes do.
		if d.focusedIsTextInput() {
			d.lastInputAt = d.now()
		}
		d.RequestRedraw()
	}
}

// pasteClipboard reads the system clipboard and routes a non-empty read through
// the focused widget's paste path (the same path bracketed paste uses), returning
// true when a read was delivered to a focused widget. It bails — returning false,
// a graceful no-op — when a menu is open, nothing is focused, or the clipboard
// read fails or is empty. Clipboard read is best effort (see App.ReadClipboard):
// it never panics, and the synchronous read is bounded so it cannot hang the loop.
// It backs both the Ctrl+V key path and the exported Paste.
func (d *Desktop) pasteClipboard() bool {
	if d.menuBar != nil && d.menuBar.IsOpen() {
		return false
	}
	if d.focused == nil {
		return false
	}
	text, err := d.app.ReadClipboard()
	if err != nil || text == "" {
		return false
	}
	d.handlePaste(tui.PasteEvent{Text: text})
	return true
}

// CopyFocused copies the focused widget's selection (or its full copyable content
// when nothing is selected) to the system clipboard, returning true when there was
// something to copy. It is the exported entry point behind Ctrl+C and an app's
// Edit→Copy menu item, delegating to the same focused-widget path as the key
// binding. It is a graceful no-op (returns false) when nothing is focused or the
// focused widget has nothing to copy. Copy changes nothing visible, so — unlike
// CutFocused — it requests no redraw.
func (d *Desktop) CopyFocused() bool {
	return d.copyFocused()
}

// CutFocused cuts the focused widget's selection to the system clipboard (the
// widget deletes it and the desktop copies the removed text), returning true when
// something was cut. It is the exported entry point behind Ctrl+X and an app's
// Edit→Cut menu item, delegating to the same focused-widget path as the key
// binding. On success it requests a redraw, since the cut mutated the widget, so a
// menu-invoked cut paints without the caller arranging it; it is a graceful no-op
// (returns false) when there was nothing to cut.
func (d *Desktop) CutFocused() bool {
	if d.cutFocused() {
		d.RequestRedraw()
		return true
	}
	return false
}

// Paste reads the system clipboard and routes it through the focused widget's
// paste path — the same path a bracketed (terminal) paste uses. It is the exported
// entry point behind Ctrl+V and an app's Edit→Paste menu item, returning true when
// a non-empty clipboard read was delivered to a focused widget. Clipboard read is
// best effort (see App.ReadClipboard): when no native reader backend is available,
// or the read fails or is empty, Paste is a graceful no-op — it never panics and
// does not block indefinitely.
func (d *Desktop) Paste() bool {
	return d.pasteClipboard()
}

// enterSuppressed reports whether Enter should currently be swallowed under the modal
// Enter-grace (gogent#347): the grace must be enabled, the focused widget must be a
// button (the only consequential Enter-activation #347 guards), the top input layer must
// be an armed modal, and less than the grace duration must have elapsed since it was
// armed (measured on the injectable clock). Scoping to a focused button means a focused
// text input still edits and a focused non-button field still bubbles Enter to a
// dialog's default/cancel handler — only the dangerous case (a freshly auto-focused
// button) is held off.
func (d *Desktop) enterSuppressed() bool {
	if d.enterGrace <= 0 {
		return false
	}
	if d.focused == nil || !d.focused.activatesOnEnter {
		return false
	}
	top := d.topInputLayer()
	if top == nil || !top.Modal || top.armedAt.IsZero() {
		return false
	}
	return d.now().Sub(top.armedAt) < d.enterGrace
}

// focusedIsTextInput reports whether the focused widget is a text-entry field: it
// exposes a text caret via CursorFn, as TextBox and MultiLineInput do and a Button or
// Checkbox does not. It scopes the typing-awareness signal (gogent#346) so activating a
// focused button does not register as the user typing into an input.
func (d *Desktop) focusedIsTextInput() bool {
	return d.focused != nil && d.focused.CursorFn != nil
}

// isTypingKey reports whether a key event represents text input into a field (a
// printable rune without Ctrl/Alt, or Backspace/Delete) as opposed to navigation,
// shortcuts, or activation. It gates the typing-awareness timestamp (gogent#346).
func isTypingKey(event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyRune:
		return !event.Ctrl && !event.Alt
	case tui.KeyBackspace, tui.KeyDelete:
		return true
	default:
		return false
	}
}

// menuInScope reports whether the menubar's mnemonics/accelerators are currently
// live: it must exist and no modal layer may sit on top.
func (d *Desktop) menuInScope() bool {
	if d.menuBar == nil {
		return false
	}
	top := d.topInputLayer()
	return top == nil || !top.Modal
}

// refreshMnemonics recomputes which components own their mnemonic (and thus show
// the highlight). The menubar is reserved first so its hot keys win clashes; the
// rest of the top input layer is walked in tree order, first occurrence wins.
func (d *Desktop) refreshMnemonics() {
	for _, layer := range d.layerSnapshot() {
		if layer != nil && layer.Root != nil {
			clearMnemonicActive(layer.Root)
		}
	}
	seen := make(map[rune]bool)
	if d.menuInScope() {
		d.menuBar.Component.mnemonicActive = true
		for _, r := range d.menuBar.topMnemonics() {
			seen[r] = true
		}
	} else if d.menuBar != nil {
		d.menuBar.Component.mnemonicActive = false
	}
	top := d.topInputLayer()
	if top == nil {
		return
	}
	activateMnemonics(top.Root, seen)
}

func clearMnemonicActive(root *VisualComponent) {
	root.mnemonicActive = false
	for _, child := range root.children {
		clearMnemonicActive(child)
	}
}

func activateMnemonics(root *VisualComponent, seen map[rune]bool) {
	if !root.Visible || !root.Enabled {
		return
	}
	if root.Mnemonic != 0 && !seen[root.Mnemonic] {
		seen[root.Mnemonic] = true
		root.mnemonicActive = true
	}
	for _, child := range root.children {
		activateMnemonics(child, seen)
	}
}

// dispatchMnemonic triggers the component owning lower in the current scope. The
// menubar wins first (its hot keys are reserved during refreshMnemonics).
func (d *Desktop) dispatchMnemonic(lower rune) bool {
	if d.menuInScope() && d.menuBar.OpenTopByMnemonic(lower) {
		return true
	}
	top := d.topInputLayer()
	if top == nil {
		return false
	}
	return d.dispatchMnemonicTree(top.Root, lower)
}

func (d *Desktop) dispatchMnemonicTree(root *VisualComponent, lower rune) bool {
	if !root.Visible || !root.Enabled {
		return false
	}
	if root.OnMnemonicFn != nil && root.OnMnemonicFn(root, lower) {
		return true
	}
	if root.mnemonicActive && root.Mnemonic == lower {
		d.activateMnemonic(root)
		return true
	}
	for _, child := range root.children {
		if d.dispatchMnemonicTree(child, lower) {
			return true
		}
	}
	return false
}

func (d *Desktop) activateMnemonic(component *VisualComponent) {
	if component.OnActivateFn != nil {
		component.OnActivateFn(component)
		return
	}
	target := component
	if component.MnemonicTarget != nil {
		target = component.MnemonicTarget
	}
	if target.Focusable {
		d.setFocus(target)
	}
}

func (d *Desktop) hitTestTopLayer(x int, y int) *VisualComponent {
	layers := d.layerSnapshot()
	for index := len(layers) - 1; index >= 0; index-- {
		layer := layers[index]
		if layer == nil || layer.Root == nil || !layer.AcceptInput {
			continue
		}
		target := layer.Root.HitTestDeep(x, y)
		if target != nil {
			return target
		}
		// A modal layer captures all input while it is on top: a click (or scroll)
		// that misses its root must not fall through to lower layers (issue #42).
		if layer.Modal {
			return nil
		}
	}
	return nil
}

func (d *Desktop) topInputLayer() *Layer {
	layers := d.layerSnapshot()
	for index := len(layers) - 1; index >= 0; index-- {
		layer := layers[index]
		if layer != nil && layer.AcceptInput && layer.Root != nil {
			return layer
		}
	}
	return nil
}

func (d *Desktop) focusablesInTopLayer() []*VisualComponent {
	layer := d.topInputLayer()
	if layer == nil {
		return nil
	}
	items := make([]*VisualComponent, 0, 16)
	collectFocusable(layer.Root, &items)
	sortFocusOrder(items)
	return items
}

func (d *Desktop) moveFocus(forward bool) {
	items := d.focusablesInTopLayer()
	if len(items) == 0 {
		d.setFocus(nil)
		return
	}
	current := -1
	for index, item := range items {
		if item == d.focused {
			current = index
			break
		}
	}
	next := 0
	if current >= 0 {
		if forward {
			next = (current + 1) % len(items)
		} else {
			next = (current - 1 + len(items)) % len(items)
		}
	}
	d.setFocus(items[next])
}

// moveFocusDirection implements arrow-key spatial navigation. For each candidate
// it projects the vector from the focused widget onto the pressed direction's
// primary axis; only candidates with a positive projection (genuinely in that
// direction) are eligible. Among those it picks the one closest in the
// perpendicular axis first, breaking ties by primary distance — so → lands on the
// widget directly to the right rather than one that is far down and slightly
// right (issue #51).
func (d *Desktop) moveFocusDirection(key tui.KeyCode) bool {
	items := d.focusablesInTopLayer()
	if len(items) == 0 || d.focused == nil {
		return false
	}
	baseX, baseY := d.focused.AbsoluteBounds().Center()
	var best *VisualComponent
	var bestPrimary, bestPerp int
	for _, item := range items {
		if item == d.focused {
			continue
		}
		cx, cy := item.AbsoluteBounds().Center()
		primary, perp, ok := directionScore(key, cx-baseX, cy-baseY)
		if !ok {
			continue
		}
		if best == nil || perp < bestPerp || (perp == bestPerp && primary < bestPrimary) {
			best = item
			bestPrimary = primary
			bestPerp = perp
		}
	}
	if best == nil {
		return false
	}
	d.setFocus(best)
	return true
}

// directionScore projects (dx, dy) onto the axis of key and returns the primary
// (along-axis) distance, the perpendicular distance, and whether the target lies
// strictly in that direction. abs is taken so distances rank by magnitude.
func directionScore(key tui.KeyCode, dx int, dy int) (primary int, perp int, ok bool) {
	switch key {
	case tui.KeyLeft:
		return -dx, abs(dy), dx < 0
	case tui.KeyRight:
		return dx, abs(dy), dx > 0
	case tui.KeyUp:
		return -dy, abs(dx), dy < 0
	case tui.KeyDown:
		return dy, abs(dx), dy > 0
	default:
		return 0, 0, false
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// SetFocus moves keyboard focus to w (or clears it when w is nil). Popup widgets
// such as Select use it to grab the keyboard while their dropdown is open.
func (d *Desktop) SetFocus(w Widget) {
	if w == nil {
		d.setFocus(nil)
		return
	}
	d.setFocus(w.Root())
}

func (d *Desktop) setFocus(next *VisualComponent) {
	if d.focused == next {
		return
	}
	if d.focused != nil {
		d.focused.hasFocus = false
		d.focused.HandleFocus(false)
	}
	d.focused = next
	if d.focused != nil {
		d.focused.hasFocus = true
		d.focused.HandleFocus(true)
	}
}

func (d *Desktop) ensureFocusInTopLayer() {
	if d.focused == nil {
		return
	}
	for _, item := range d.focusablesInTopLayer() {
		if item == d.focused {
			return
		}
	}
	d.setFocus(nil)
}
