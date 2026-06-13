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
}

func NewTextView(text string, bounds Rect) *TextView {
	view := &TextView{
		Lines: strings.Split(text, "\n"),
		FG:    DefaultTheme.WindowFG,
		BG:    DefaultTheme.WindowBG,
	}
	view.Component = NewComponent(bounds)
	view.Component.Focusable = true
	view.Component.DrawFn = view.draw
	view.Component.OnTypeFn = view.handleType
	view.Component.OnScrollFn = view.handleScroll
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
	for row := 0; row < abs.H; row++ {
		lineIndex := t.ScrollY + row
		if lineIndex < 0 || lineIndex >= len(t.Lines) {
			continue
		}
		surface.WriteString(abs.X, abs.Y+row, t.Lines[lineIndex], tui.Cell{FG: t.FG, BG: t.BG})
	}
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
