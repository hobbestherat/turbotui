package tv

import tui "github.com/hobbestherat/turbotui"

// Select is a drop-down combo box: it shows the current value with a ▼ marker
// and, when opened, drops a scrollable list on a desktop-owned popup layer so the
// list is never clipped by the window that hosts it. The popup widens to fit the
// longest option and flips above the control when there is no room below.
//
// Keyboard: Enter/Space opens, Up/Down move, Home/End jump to the ends,
// PageUp/PageDown page, Enter picks, Escape cancels, and typing a letter jumps
// (type-ahead) to the next option beginning with it. Mouse: click to open, click
// an item to pick, click or drag the scrollbar to scroll, wheel to scroll, click
// outside to dismiss.
type Select struct {
	Component *VisualComponent
	Options   []string
	Selected  int
	OnChange  func(index int)
	FG        tui.Color
	BG        tui.Color
	FocusFG   tui.Color
	FocusBG   tui.Color
	// Shadow draws a drop shadow under the opened dropdown popup. It defaults to
	// true; set it to false to render the list flat (e.g. for a no-shadow theme),
	// mirroring the Shadow field on Window, Button and MenuBar.
	Shadow bool

	desktop   *Desktop
	popup     *Layer
	highlight int
	// offset is the index of the first visible option while the list is open.
	// Keyboard navigation keeps the highlight on screen; wheel and scrollbar
	// scrolling move it freely without dragging the highlight along.
	offset int
}

func NewSelect(desktop *Desktop, options []string, bounds Rect) *Select {
	s := &Select{
		Options: options,
		FG:      activeTheme.InputFG,
		BG:      activeTheme.InputBG,
		FocusFG: activeTheme.InputFocusFG,
		FocusBG: activeTheme.InputFocusBG,
		Shadow:  true,
		desktop: desktop,
	}
	s.Component = NewComponent(bounds)
	s.Component.Focusable = true
	s.Component.DrawFn = s.draw
	s.Component.OnTypeFn = s.handleType
	s.Component.OnClickFn = s.handleClick
	return s
}

func (s *Select) Root() *VisualComponent {
	return s.Component
}

// Value returns the currently selected option text ("" when nothing is selected).
func (s *Select) Value() string {
	if s.Selected < 0 || s.Selected >= len(s.Options) {
		return ""
	}
	return s.Options[s.Selected]
}

func (s *Select) GetSelected() int {
	return s.Selected
}

func (s *Select) SetSelected(index int) {
	if index < 0 || index >= len(s.Options) {
		return
	}
	s.Selected = index
}

// SetOptions replaces the dropdown's option list. The current selection is
// preserved when its value still exists among the new options; otherwise it
// clamps to 0. Any open popup is closed.
func (s *Select) SetOptions(opts []string) {
	// Capture the current value only when a real option is selected, so an
	// empty-string option is preserved by value while a "nothing selected"
	// state correctly clamps to 0.
	hadSelection := s.Selected >= 0 && s.Selected < len(s.Options)
	prev := s.Value()
	s.close()
	s.Options = opts
	s.Selected = 0
	if hadSelection {
		for i, opt := range opts {
			if opt == prev {
				s.Selected = i
				break
			}
		}
	}
	// Reset scroll/cursor state so the next open starts from a consistent view.
	s.offset = 0
	s.highlight = s.Selected
}

func (s *Select) IsOpen() bool {
	return s.popup != nil
}

func (s *Select) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	fg, bg := focusColors(component.Focused(), s.FG, s.BG, s.FocusFG, s.FocusBG)
	style := tui.Cell{FG: fg, BG: bg}
	surface.Fill(abs, style)
	// Reserve the last column for the ▼/▲ marker and a one-column gap before it;
	// ellipsize (rather than raw-clip) when the value is too long, so a truncated
	// label is signalled instead of silently cut.
	maxText := abs.W - 2
	if maxText > 0 {
		surface.WriteString(abs.X, abs.Y, Truncate(s.Value(), maxText, "…"), style)
	}
	arrow := '▼'
	if s.popup != nil {
		arrow = '▲'
	}
	surface.SetCell(abs.Right(), abs.Y, tui.Cell{Ch: arrow, FG: fg, BG: bg, Bold: true})
}

func (s *Select) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyEnter:
		s.open()
		return true
	case tui.KeyRune:
		if event.Rune == ' ' {
			s.open()
			return true
		}
	}
	return false
}

func (s *Select) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	if event.Down || !component.AbsoluteBounds().Contains(event.X, event.Y) {
		return true
	}
	if s.popup != nil {
		s.close()
	} else {
		s.open()
	}
	return true
}

func (s *Select) open() {
	if s.popup != nil || len(s.Options) == 0 {
		return
	}
	s.highlight = s.Selected
	if s.highlight < 0 {
		s.highlight = 0
	}
	s.offset = 0
	s.ensureVisible()
	catcher := NewComponent(Rect{X: 0, Y: 0, W: s.desktop.App().Width(), H: s.desktop.App().Height()})
	catcher.Focusable = true
	catcher.DrawFn = s.drawPopup
	catcher.OnTypeFn = s.popupType
	catcher.OnClickFn = s.popupClick
	catcher.OnScrollFn = s.popupScroll
	s.popup = NewLayer("select-popup", catcher, true, false)
	s.desktop.AddLayer(s.popup)
	s.desktop.SetFocus(catcher)
}

func (s *Select) close() {
	if s.popup == nil {
		return
	}
	layer := s.popup
	s.popup = nil
	s.desktop.RemoveLayer(layer)
	s.desktop.SetFocus(s.Component)
}

func (s *Select) commit(index int) {
	if index >= 0 && index < len(s.Options) {
		changed := index != s.Selected
		s.Selected = index
		if changed && s.OnChange != nil {
			s.OnChange(index)
		}
	}
	s.close()
}

// popupRect is the box dropped below (or above) the select, sized to fit every
// option and clamped to the screen. It flips above the control when the list does
// not fit below and the space above is the larger region, so a Select placed low
// in a window stays usable. The width grows to fit the longest option (plus a
// scrollbar column when the list scrolls): the popup lives on its own layer and
// is not clipped by the host window, so it can be wider than the control.
func (s *Select) popupRect() Rect {
	abs := s.Component.AbsoluteBounds()
	screenW := s.desktop.App().Width()
	screenH := s.desktop.App().Height()

	natural := len(s.Options) + 2
	spaceBelow := screenH - (abs.Y + 1)
	spaceAbove := abs.Y
	// Flip up only when the list does not fit below AND the space above is the
	// larger region; otherwise keep dropping down (the familiar direction).
	flipUp := natural > spaceBelow && spaceAbove > spaceBelow
	height := natural
	y := abs.Y + 1
	if flipUp {
		if height > spaceAbove {
			height = spaceAbove
		}
		y = abs.Y - height
	} else if height > spaceBelow {
		height = spaceBelow
	}
	if height < 1 {
		height = 1
	}
	if y < 0 {
		y = 0
	}

	innerH := height - 2
	if innerH < 1 {
		innerH = 1
	}
	scrolling := len(s.Options) > innerH
	textWidth := 0
	for _, opt := range s.Options {
		if w := tui.StringWidth(opt); w > textWidth {
			textWidth = w
		}
	}
	width := abs.W
	need := textWidth + 2
	if scrolling {
		need++ // reserve the scrollbar column
	}
	if need > width {
		width = need
	}
	if width > screenW {
		width = screenW
	}
	x := abs.X
	if x+width > screenW {
		x = screenW - width
	}
	if x < 0 {
		x = 0
	}

	return Rect{X: x, Y: y, W: width, H: height}
}

// visibleCount is the number of option rows the open popup can show.
func (s *Select) visibleCount() int {
	h := s.popupRect().H - 2
	if h < 1 {
		h = 1
	}
	return h
}

func (s *Select) scrollMax() int {
	return scrollbarMaxOffset(len(s.Options), s.visibleCount())
}

func (s *Select) clampOffset() {
	max := s.scrollMax()
	if s.offset < 0 {
		s.offset = 0
	} else if s.offset > max {
		s.offset = max
	}
}

// viewOffset is the index of the first visible option, clamped to the scrollable
// range.
func (s *Select) viewOffset() int {
	max := s.scrollMax()
	if s.offset < 0 {
		return 0
	}
	if s.offset > max {
		return max
	}
	return s.offset
}

// ensureVisible scrolls the minimum amount so the highlighted row is on screen.
// It is called after keyboard moves; wheel and scrollbar scrolling bypass it so
// the view can be scrolled freely without dragging the highlight along.
func (s *Select) ensureVisible() {
	visible := s.visibleCount()
	if visible < 1 {
		return
	}
	if s.highlight < s.offset {
		s.offset = s.highlight
	} else if s.highlight >= s.offset+visible {
		s.offset = s.highlight - visible + 1
	}
	s.clampOffset()
}

func (s *Select) drawPopup(_ *VisualComponent, surface Surface) {
	rect := s.popupRect()
	if s.Shadow {
		surface.DrawShadow(rect, activeTheme.WindowShadow, DefaultShadowStyle)
	}
	surface.Fill(rect, tui.Cell{Ch: ' ', FG: activeTheme.DialogFG, BG: activeTheme.DialogBG})
	surface.DrawBox(rect, tui.LineSingle, activeTheme.DialogBorderFG, activeTheme.DialogBG)
	inner := Rect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}
	offset := s.viewOffset()
	// When the list is taller than the visible area, reserve the last column for
	// a scrollbar so the user can see there is more to scroll to.
	scrollbar := len(s.Options) > inner.H && inner.W > 1
	textW := inner.W
	if scrollbar {
		textW = inner.W - 1
	}
	for row := 0; row < inner.H; row++ {
		index := offset + row
		if index >= len(s.Options) {
			break
		}
		fg, bg := activeTheme.DialogFG, activeTheme.DialogBG
		if index == s.highlight {
			fg, bg = activeTheme.SelectionFG, activeTheme.SelectionBG
		}
		style := tui.Cell{FG: fg, BG: bg}
		surface.Fill(Rect{X: inner.X, Y: inner.Y + row, W: textW, H: 1}, style)
		surface.WriteString(inner.X, inner.Y+row, Truncate(s.Options[index], textW, "…"), style)
	}
	if scrollbar {
		track := Rect{X: inner.X + inner.W - 1, Y: inner.Y, W: 1, H: inner.H}
		drawVScrollbar(surface, track, len(s.Options), inner.H, offset,
			activeTheme.DialogBorderFG, activeTheme.DialogBG, true)
	}
}

func (s *Select) popupType(_ *VisualComponent, event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyEscape:
		s.close()
	case tui.KeyUp:
		if s.highlight > 0 {
			s.highlight--
			s.ensureVisible()
		}
	case tui.KeyDown:
		if s.highlight < len(s.Options)-1 {
			s.highlight++
			s.ensureVisible()
		}
	case tui.KeyHome:
		s.highlight = 0
		s.ensureVisible()
	case tui.KeyEnd:
		if len(s.Options) > 0 {
			s.highlight = len(s.Options) - 1
			s.ensureVisible()
		}
	case tui.KeyPageUp:
		s.pageBy(-s.visibleCount())
	case tui.KeyPageDown:
		s.pageBy(s.visibleCount())
	case tui.KeyEnter:
		s.commit(s.highlight)
	case tui.KeyRune:
		if event.Rune == ' ' {
			s.commit(s.highlight)
		} else {
			s.typeAhead(event.Rune)
		}
	}
	return true
}

// pageBy moves the highlight by step rows (clamped to the list bounds) and keeps
// it on screen. Used for PageUp/PageDown.
func (s *Select) pageBy(step int) {
	if len(s.Options) == 0 {
		return
	}
	s.highlight += step
	if s.highlight < 0 {
		s.highlight = 0
	}
	if last := len(s.Options) - 1; s.highlight > last {
		s.highlight = last
	}
	s.ensureVisible()
}

// typeAhead jumps the highlight to the next option whose text begins with r
// (case-insensitive), searching forward from the current position and wrapping
// around — standard combo-box behaviour.
func (s *Select) typeAhead(r rune) {
	n := len(s.Options)
	if n == 0 {
		return
	}
	want := unicodeLower(r)
	for i := 1; i <= n; i++ {
		idx := (s.highlight + i) % n
		if firstOptionRune(s.Options[idx]) == want {
			s.highlight = idx
			s.ensureVisible()
			return
		}
	}
}

// firstOptionRune returns the lowercased first rune of s (0 for an empty string),
// for type-ahead matching.
func firstOptionRune(s string) rune {
	for _, r := range s {
		return unicodeLower(r)
	}
	return 0
}

func (s *Select) popupScroll(_ *VisualComponent, event tui.ScrollEvent) bool {
	if s.popup == nil {
		return false
	}
	// Delta is +1 for wheel-up and -1 for wheel-down, so subtracting scrolls the
	// list in the natural direction.
	s.offset -= event.Delta
	s.clampOffset()
	return true
}

func (s *Select) popupClick(_ *VisualComponent, event tui.ClickEvent) bool {
	rect := s.popupRect()
	inner := Rect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}
	// The scrollbar occupies the last inner column when the list scrolls; route
	// presses and drags there to the scrollbar instead of picking an item.
	if len(s.Options) > inner.H && inner.W > 1 {
		track := Rect{X: inner.X + inner.W - 1, Y: inner.Y, W: 1, H: inner.H}
		if event.X == track.X && event.Y >= track.Y && event.Y <= track.Bottom() {
			if off, ok := scrollbarOffsetForY(track, len(s.Options), inner.H, s.offset, event.Y); ok {
				s.offset = off
			}
			return true
		}
	}
	if event.Down {
		return true
	}
	if inner.Contains(event.X, event.Y) {
		s.commit(s.viewOffset() + (event.Y - inner.Y))
		return true
	}
	s.close()
	return true
}
