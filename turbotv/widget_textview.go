package tv

import (
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

type TextView struct {
	Component *VisualComponent
	Lines     []string
	ScrollY   int
	FG        tui.Color
	BG        tui.Color
	FocusFG   tui.Color
}

func NewTextView(text string, bounds Rect) *TextView {
	view := &TextView{
		Lines:   strings.Split(text, "\n"),
		FG:      DefaultTheme.WindowFG,
		BG:      DefaultTheme.WindowBG,
		FocusFG: DefaultTheme.MnemonicFG,
	}
	view.Component = NewComponent(bounds)
	view.Component.Focusable = true
	view.Component.DrawFn = view.draw
	view.Component.OnTypeFn = view.handleType
	view.Component.OnScrollFn = view.handleScroll
	view.Component.OnClickFn = view.handleClick
	return view
}

func (t *TextView) Root() *VisualComponent {
	return t.Component
}

func (t *TextView) SetText(text string) {
	t.Lines = strings.Split(text, "\n")
}

func (t *TextView) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	surface.Fill(abs, tui.Cell{Ch: ' ', FG: t.FG, BG: t.BG})
	// The rightmost column is a scrollbar; it doubles as the focus indicator
	// (bright when focused, dim otherwise) so it is obvious which view is active.
	textWidth := abs.W - 1
	if textWidth < 1 {
		textWidth = abs.W
	}
	for row := 0; row < abs.H; row++ {
		lineIndex := t.ScrollY + row
		if lineIndex < 0 || lineIndex >= len(t.Lines) {
			continue
		}
		line := []rune(t.Lines[lineIndex])
		if len(line) > textWidth {
			line = line[:textWidth]
		}
		surface.WriteString(abs.X, abs.Y+row, string(line), tui.Cell{FG: t.FG, BG: t.BG})
	}
	if abs.W > 1 {
		t.drawScrollbar(surface, abs, component.HasFocus)
	}
}

// drawScrollbar paints the right-hand track with up/down arrows and a thumb whose
// position reflects ScrollY. Focused views use FocusFG; unfocused ones are dimmed.
func (t *TextView) drawScrollbar(surface Surface, abs Rect, focused bool) {
	color := tui.ANSIColor(8)
	if focused {
		color = t.FocusFG
	}
	x := abs.Right()
	style := tui.Cell{FG: color, BG: t.BG, Bold: focused}
	for row := 0; row < abs.H; row++ {
		surface.SetCell(x, abs.Y+row, tui.Cell{Ch: '│', FG: color, BG: t.BG})
	}
	surface.SetCell(x, abs.Y, tui.Cell{Ch: '▲', FG: color, BG: t.BG, Bold: focused})
	surface.SetCell(x, abs.Bottom(), tui.Cell{Ch: '▼', FG: color, BG: t.BG, Bold: focused})
	span := len(t.Lines) - 1
	// The thumb lives strictly between the two arrows (abs.H-2 cells), so it never
	// overpaints the bottom arrow even when scrolled to the end.
	track := abs.H - 2
	if span > 0 && track > 0 {
		thumb := t.ScrollY * (track - 1) / span
		if thumb > track-1 {
			thumb = track - 1
		}
		surface.SetCell(x, abs.Y+1+thumb, tui.Cell{Ch: '█', FG: style.FG, BG: t.BG, Bold: focused})
	}
}

// handleClick lets the up/down scrollbar arrows scroll the view by one line.
func (t *TextView) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	if !event.Down {
		return true
	}
	abs := component.AbsoluteBounds()
	if abs.W <= 1 || event.X != abs.Right() {
		return false
	}
	if event.Y == abs.Y {
		if t.ScrollY > 0 {
			t.ScrollY--
		}
		return true
	}
	if event.Y == abs.Bottom() {
		if t.ScrollY < len(t.Lines)-1 {
			t.ScrollY++
		}
		return true
	}
	return false
}

func (t *TextView) handleScroll(_ *VisualComponent, event tui.ScrollEvent) bool {
	t.ScrollY -= event.Delta
	if t.ScrollY < 0 {
		t.ScrollY = 0
	}
	if t.ScrollY > len(t.Lines)-1 {
		t.ScrollY = len(t.Lines) - 1
	}
	return true
}

func (t *TextView) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyUp:
		if t.ScrollY > 0 {
			t.ScrollY--
		}
		return true
	case tui.KeyDown:
		if t.ScrollY < len(t.Lines)-1 {
			t.ScrollY++
		}
		return true
	case tui.KeyPageUp:
		t.ScrollY -= 5
		if t.ScrollY < 0 {
			t.ScrollY = 0
		}
		return true
	case tui.KeyPageDown:
		t.ScrollY += 5
		if t.ScrollY > len(t.Lines)-1 {
			t.ScrollY = len(t.Lines) - 1
		}
		return true
	}
	return false
}
