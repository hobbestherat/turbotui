package tv

import (
	"context"

	tui "github.com/hobbestherat/turbotui"
)

type Desktop struct {
	app            *tui.App
	layers         []*Layer
	backgroundCell tui.Cell
	theme          Theme
	focused        *VisualComponent
	mouseCapture   *VisualComponent
	menuBar        *MenuBar
	unhandledKeyFn func(event tui.TypeEvent)
}

func NewDesktop(app *tui.App) *Desktop {
	desktop := &Desktop{
		app:            app,
		theme:          DefaultTheme,
		backgroundCell: tui.Cell{Ch: ' ', FG: activeTheme.DesktopFG, BG: activeTheme.DesktopBG},
	}
	app.OnResize(func(_ tui.ResizeEvent) {
		desktop.Redraw()
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
	return desktop
}

func (d *Desktop) App() *tui.App {
	return d.app
}

// Post runs fn on the event-loop goroutine and then redraws. Background tasks use
// it to safely update widgets (e.g. streaming text) and refresh the screen.
func (d *Desktop) Post(fn func()) {
	if fn == nil {
		return
	}
	d.app.Post(func() {
		fn()
		d.Redraw()
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

// SetMenuBar registers the application menubar. The desktop owns it: it is drawn
// on top of every layer and receives global shortcuts (Alt-mnemonics and
// Ctrl-accelerators) and keyboard navigation while open. Do NOT also add it to a
// layer.
func (d *Desktop) SetMenuBar(bar *MenuBar) {
	d.menuBar = bar
	if bar != nil {
		bar.Component.SetBounds(Rect{X: 0, Y: 0, W: d.app.Width(), H: 1})
	}
}

func (d *Desktop) AddLayer(layer *Layer) {
	d.layers = append(d.layers, layer)
	if layer.FullScreen {
		layer.Root.SetBounds(Rect{X: 0, Y: 0, W: d.app.Width(), H: d.app.Height()})
	}
	d.Redraw()
}

func (d *Desktop) RemoveTopLayer() {
	if len(d.layers) == 0 {
		return
	}
	d.layers = d.layers[:len(d.layers)-1]
	d.ensureFocusInTopLayer()
	d.Redraw()
}

func (d *Desktop) RemoveLayer(layer *Layer) {
	if layer == nil {
		return
	}
	next := make([]*Layer, 0, len(d.layers))
	for _, existing := range d.layers {
		if existing == layer {
			continue
		}
		next = append(next, existing)
	}
	d.layers = next
	d.ensureFocusInTopLayer()
	d.Redraw()
}

func (d *Desktop) TopLayer() *Layer {
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
		d.Redraw()
	}
}

// raiseLayer performs the reordering without redrawing, returning true when the
// stack changed. Callers that already redraw (e.g. handleClick) use this form.
func (d *Desktop) raiseLayer(layer *Layer) bool {
	if layer == nil || layer.FullScreen {
		return false
	}
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
	for root.Parent != nil {
		root = root.Parent
	}
	for _, layer := range d.layers {
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
	if len(items) > 0 {
		d.setFocus(items[0])
	}
}

// componentInLayer reports whether c is the layer root or a descendant of it.
func componentInLayer(c *VisualComponent, layer *Layer) bool {
	if layer == nil {
		return false
	}
	for current := c; current != nil; current = current.Parent {
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

// updateCursor positions the real terminal cursor at the focused widget's text
// caret (via its CursorFn), or hides it when no focused input exposes one or a
// menu is open.
func (d *Desktop) updateCursor() {
	menuOpen := d.menuBar != nil && d.menuBar.IsOpen()
	if !menuOpen && d.focused != nil && d.focused.Visible && d.focused.CursorFn != nil {
		if x, y, ok := d.focused.CursorFn(d.focused); ok {
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
	for _, layer := range d.layers {
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
	d.compose()
	d.updateCursor()
	return d.app.Run(ctx)
}

func (d *Desktop) handleClick(event tui.ClickEvent) {
	// The menubar sits above every layer, so it gets first claim on clicks that
	// fall on the bar or an open dropdown. A click anywhere else closes the menu.
	if d.menuBar != nil && (d.menuBar.IsOpen() || d.menuBar.HitTest(event.X, event.Y)) {
		if event.Down {
			if d.menuBar.HitTest(event.X, event.Y) {
				_ = d.menuBar.Component.BubbleClick(event)
			} else {
				d.menuBar.CloseMenus()
			}
		}
		d.Redraw()
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
				} else if target.Focusable {
					d.setFocus(target)
				}
			} else if target.Focusable {
				d.setFocus(target)
			}
			_ = target.BubbleClick(event)
			d.Redraw()
			return
		}
		d.mouseCapture = nil
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
	d.Redraw()
}

func (d *Desktop) handleScroll(event tui.ScrollEvent) {
	target := d.hitTestTopLayer(event.X, event.Y)
	if target == nil {
		return
	}
	if target.BubbleScroll(event) {
		d.Redraw()
	}
}

func (d *Desktop) handleType(event tui.TypeEvent) {
	// A dropped-down menu captures the keyboard entirely.
	if d.menuBar != nil && d.menuBar.IsOpen() {
		d.menuBar.HandleKey(event)
		d.Redraw()
		return
	}
	// Ctrl accelerators from the menubar, unless a modal layer blocks it.
	if d.menuInScope() && d.menuBar.HandleAccelerator(event) {
		d.Redraw()
		return
	}
	// Alt+mnemonic activation within the current scope (top layer + menubar).
	if event.Key == tui.KeyRune && event.Alt {
		if d.dispatchMnemonic(unicodeLower(event.Rune)) {
			d.Redraw()
			return
		}
	}
	// Ctrl+C / Ctrl+Shift+C copies the focused widget's selection (or all of its
	// content) and is consumed only when there was something to copy, so it can
	// still fall through to an app quit handler otherwise.
	if isCopyKey(event) && d.copyFocused() {
		return
	}
	if d.focused != nil {
		if d.focused.BubbleType(event) {
			d.Redraw()
			return
		}
	}
	switch event.Key {
	case tui.KeyTab:
		d.moveFocus(true)
		d.Redraw()
		return
	case tui.KeyBackTab:
		d.moveFocus(false)
		d.Redraw()
		return
	case tui.KeyLeft, tui.KeyRight, tui.KeyUp, tui.KeyDown:
		if d.moveFocusDirection(event.Key) {
			d.Redraw()
			return
		}
	}
	if d.unhandledKeyFn != nil {
		d.unhandledKeyFn(event)
	}
}

// SetUnhandledKeyFn registers a callback invoked when a key event was not
// consumed by the menu, focus navigation, copy, or the focused widget. Apps use
// it for global shortcuts (e.g. a Ctrl+C quit confirmation) without racing the
// widgets that might legitimately want the same key.
func (d *Desktop) SetUnhandledKeyFn(fn func(event tui.TypeEvent)) {
	d.unhandledKeyFn = fn
}

func isCopyKey(event tui.TypeEvent) bool {
	return event.Key == tui.KeyRune && event.Ctrl && unicodeLower(event.Rune) == 'c'
}

// copyFocused copies the focused component's CopyFn text to the clipboard,
// returning true when something was copied.
func (d *Desktop) copyFocused() bool {
	if d.focused == nil || d.focused.CopyFn == nil {
		return false
	}
	text, ok := d.focused.CopyFn(d.focused)
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
		d.Redraw()
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
	for _, layer := range d.layers {
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
	for _, child := range root.Children {
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
	for _, child := range root.Children {
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
	for _, child := range root.Children {
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
	for index := len(d.layers) - 1; index >= 0; index-- {
		layer := d.layers[index]
		if layer == nil || layer.Root == nil || !layer.AcceptInput {
			continue
		}
		target := layer.Root.HitTestDeep(x, y)
		if target != nil {
			return target
		}
	}
	return nil
}

func (d *Desktop) topInputLayer() *Layer {
	for index := len(d.layers) - 1; index >= 0; index-- {
		layer := d.layers[index]
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

func (d *Desktop) moveFocusDirection(key tui.KeyCode) bool {
	items := d.focusablesInTopLayer()
	if len(items) == 0 || d.focused == nil {
		return false
	}
	baseRect := d.focused.AbsoluteBounds()
	baseX, baseY := baseRect.Center()
	best := (*VisualComponent)(nil)
	bestScore := int(^uint(0) >> 1)
	for _, item := range items {
		if item == d.focused {
			continue
		}
		rect := item.AbsoluteBounds()
		cx, cy := rect.Center()
		dx := cx - baseX
		dy := cy - baseY
		if !isInDirection(key, dx, dy) {
			continue
		}
		score := dx*dx + dy*dy
		if score < bestScore {
			bestScore = score
			best = item
		}
	}
	if best == nil {
		return false
	}
	d.setFocus(best)
	return true
}

func isInDirection(key tui.KeyCode, dx int, dy int) bool {
	switch key {
	case tui.KeyLeft:
		return dx < 0
	case tui.KeyRight:
		return dx > 0
	case tui.KeyUp:
		return dy < 0
	case tui.KeyDown:
		return dy > 0
	default:
		return false
	}
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
		d.focused.HasFocus = false
		if d.focused.OnFocusFn != nil {
			d.focused.OnFocusFn(d.focused, false)
		}
	}
	d.focused = next
	if d.focused != nil {
		d.focused.HasFocus = true
		if d.focused.OnFocusFn != nil {
			d.focused.OnFocusFn(d.focused, true)
		}
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
