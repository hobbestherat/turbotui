package tv

import tui "github.com/hobbestherat/turbotui"

// Select is a drop-down combo box: it shows the current value with a ▼ marker and,
// when opened, drops a scrollable list on a desktop-owned popup layer so the list
// is never clipped by the window that hosts it. Keyboard: Enter/Space opens,
// Up/Down move, Enter picks, Escape cancels. Mouse: click to open, click an item
// to pick, click outside to dismiss.
type Select struct {
	Component *VisualComponent
	Options   []string
	Selected  int
	OnChange  func(index int)
	FG        tui.Color
	BG        tui.Color
	FocusFG   tui.Color
	FocusBG   tui.Color

	desktop   *Desktop
	popup     *Layer
	highlight int
}

func NewSelect(desktop *Desktop, options []string, bounds Rect) *Select {
	s := &Select{
		Options: options,
		FG:      DefaultTheme.InputFG,
		BG:      DefaultTheme.InputBG,
		FocusFG: DefaultTheme.InputFocusFG,
		FocusBG: DefaultTheme.InputFocusBG,
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

func (s *Select) IsOpen() bool {
	return s.popup != nil
}

func (s *Select) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	fg, bg := inputColors(component.HasFocus, s.FG, s.BG, s.FocusFG, s.FocusBG)
	style := tui.Cell{FG: fg, BG: bg}
	surface.Fill(abs, style)
	maxText := abs.W - 2
	if maxText > 0 {
		runes := []rune(s.Value())
		if len(runes) > maxText {
			runes = runes[:maxText]
		}
		surface.WriteString(abs.X, abs.Y, string(runes), style)
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
	catcher := NewComponent(Rect{X: 0, Y: 0, W: s.desktop.App().Width(), H: s.desktop.App().Height()})
	catcher.Focusable = true
	catcher.DrawFn = s.drawPopup
	catcher.OnTypeFn = s.popupType
	catcher.OnClickFn = s.popupClick
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

// popupRect is the box dropped below the select, clamped to the screen height.
func (s *Select) popupRect() Rect {
	abs := s.Component.AbsoluteBounds()
	height := len(s.Options) + 2
	maxHeight := s.desktop.App().Height() - (abs.Y + 1)
	if height > maxHeight {
		height = maxHeight
	}
	if height < 3 {
		height = 3
	}
	return Rect{X: abs.X, Y: abs.Y + 1, W: abs.W, H: height}
}

// viewOffset is the index of the first visible option, scrolled to keep the
// highlighted row in view.
func (s *Select) viewOffset() int {
	visible := s.popupRect().H - 2
	if visible < 1 {
		visible = 1
	}
	if s.highlight >= visible {
		return s.highlight - visible + 1
	}
	return 0
}

func (s *Select) drawPopup(_ *VisualComponent, surface Surface) {
	rect := s.popupRect()
	surface.DrawShadow(rect, DefaultTheme.WindowShadow)
	surface.Fill(rect, tui.Cell{Ch: ' ', FG: DefaultTheme.DialogFG, BG: DefaultTheme.DialogBG})
	surface.DrawBox(rect, tui.LineSingle, DefaultTheme.DialogBorderFG, DefaultTheme.DialogBG)
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
		fg, bg := DefaultTheme.DialogFG, DefaultTheme.DialogBG
		if index == s.highlight {
			fg, bg = tui.ANSIColor(15), tui.ANSIColor(4)
		}
		style := tui.Cell{FG: fg, BG: bg}
		surface.Fill(Rect{X: inner.X, Y: inner.Y + row, W: textW, H: 1}, style)
		runes := []rune(s.Options[index])
		if len(runes) > textW {
			runes = runes[:textW]
		}
		surface.WriteString(inner.X, inner.Y+row, string(runes), style)
	}
	if scrollbar {
		track := Rect{X: inner.X + inner.W - 1, Y: inner.Y, W: 1, H: inner.H}
		drawVScrollbar(surface, track, len(s.Options), inner.H, offset,
			DefaultTheme.DialogBorderFG, DefaultTheme.DialogBG, true)
	}
}

func (s *Select) popupType(_ *VisualComponent, event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyEscape:
		s.close()
	case tui.KeyUp:
		if s.highlight > 0 {
			s.highlight--
		}
	case tui.KeyDown:
		if s.highlight < len(s.Options)-1 {
			s.highlight++
		}
	case tui.KeyEnter:
		s.commit(s.highlight)
	case tui.KeyRune:
		if event.Rune == ' ' {
			s.commit(s.highlight)
		}
	}
	return true
}

func (s *Select) popupClick(_ *VisualComponent, event tui.ClickEvent) bool {
	if event.Down {
		return true
	}
	rect := s.popupRect()
	inner := Rect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}
	if inner.Contains(event.X, event.Y) {
		s.commit(s.viewOffset() + (event.Y - inner.Y))
		return true
	}
	s.close()
	return true
}
