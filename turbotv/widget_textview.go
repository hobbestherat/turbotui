package tv

import (
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

// TextEntry is one logical line in a TextView. Entries may carry their own colour
// and may be foldable, in which case their children are shown/hidden by clicking
// the ▸/▾ marker. Build a tree with Add/AddColored and stream text with AppendText.
type TextEntry struct {
	text      string
	fg        tui.Color
	hasFG     bool
	foldable  bool
	collapsed bool
	children  []*TextEntry
	parent    *TextEntry
	view      *TextView
}

func (e *TextEntry) GetText() string {
	return e.text
}

func (e *TextEntry) SetText(text string) {
	e.text = text
	e.view.touch()
}

// AppendText grows the entry's text in place; used for the teletype effect.
func (e *TextEntry) AppendText(text string) {
	e.text += text
	e.view.touch()
}

func (e *TextEntry) Add(text string) *TextEntry {
	return e.addChild(text, tui.Color{}, false)
}

func (e *TextEntry) AddColored(text string, fg tui.Color) *TextEntry {
	return e.addChild(text, fg, true)
}

func (e *TextEntry) Toggle() {
	e.collapsed = !e.collapsed
	e.view.touch()
}

func (e *TextEntry) SetCollapsed(collapsed bool) {
	e.collapsed = collapsed
	e.view.touch()
}

func (e *TextEntry) addChild(text string, fg tui.Color, hasFG bool) *TextEntry {
	e.foldable = true
	child := &TextEntry{text: text, fg: fg, hasFG: hasFG, parent: e, view: e.view}
	e.children = append(e.children, child)
	e.view.touch()
	return child
}

type TextView struct {
	Component *VisualComponent
	FG        tui.Color
	BG        tui.Color
	FocusFG   tui.Color
	Wrap      bool

	entries []*TextEntry
	scrollY int
	follow  bool
}

func NewTextView(text string, bounds Rect) *TextView {
	view := &TextView{
		FG:      DefaultTheme.WindowFG,
		BG:      DefaultTheme.WindowBG,
		FocusFG: DefaultTheme.MnemonicFG,
		follow:  true,
	}
	view.Component = NewComponent(bounds)
	view.Component.Focusable = true
	view.Component.DrawFn = view.draw
	view.Component.OnTypeFn = view.handleType
	view.Component.OnScrollFn = view.handleScroll
	view.Component.OnClickFn = view.handleClick
	view.SetText(text)
	return view
}

func (t *TextView) Root() *VisualComponent {
	return t.Component
}

// SetText replaces all content with plain lines (one entry per '\n').
func (t *TextView) SetText(text string) {
	t.entries = nil
	if text != "" {
		for _, line := range strings.Split(text, "\n") {
			t.AddLine(line)
		}
	}
}

func (t *TextView) Clear() {
	t.entries = nil
	t.scrollY = 0
	t.follow = true
}

func (t *TextView) AddLine(text string) *TextEntry {
	return t.add(text, tui.Color{}, false)
}

func (t *TextView) AddColored(text string, fg tui.Color) *TextEntry {
	return t.add(text, fg, true)
}

func (t *TextView) add(text string, fg tui.Color, hasFG bool) *TextEntry {
	entry := &TextEntry{text: text, fg: fg, hasFG: hasFG, view: t}
	t.entries = append(t.entries, entry)
	t.touch()
	return entry
}

func (t *TextView) ScrollToBottom() {
	t.follow = true
}

// touch is called whenever content changes; while following, the view stays
// pinned to the bottom so streamed text remains visible. The huge sentinel is
// clamped to the real maximum during draw.
func (t *TextView) touch() {
	if t.follow {
		t.scrollY = 1 << 30
	}
}

// renderRow is one physical display line: a (possibly wrapped) slice of an entry.
type renderRow struct {
	entry  *TextEntry
	text   string
	indent int
	marker rune // ▸/▾ on the first row of a foldable entry, else 0
}

func (t *TextView) layoutRows(width int) []renderRow {
	rows := make([]renderRow, 0, len(t.entries))
	var walk func(entries []*TextEntry, depth int)
	walk = func(entries []*TextEntry, depth int) {
		for _, entry := range entries {
			indent := depth * 2
			marker := rune(0)
			if entry.foldable {
				marker = '▾'
				if entry.collapsed {
					marker = '▸'
				}
			}
			avail := width - indent
			if marker != 0 {
				avail -= 2
			}
			if avail < 1 {
				avail = 1
			}
			segments := []string{entry.text}
			if t.Wrap {
				segments = wrapText(entry.text, avail)
			}
			for index, segment := range segments {
				row := renderRow{entry: entry, text: segment, indent: indent}
				if index == 0 {
					row.marker = marker
				}
				rows = append(rows, row)
			}
			if entry.foldable && !entry.collapsed {
				walk(entry.children, depth+1)
			}
		}
	}
	walk(t.entries, 0)
	return rows
}

func (t *TextView) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	surface.Fill(abs, tui.Cell{Ch: ' ', FG: t.FG, BG: t.BG})
	textWidth := abs.W - 1
	if textWidth < 1 {
		textWidth = abs.W
	}
	rows := t.layoutRows(textWidth)
	t.clampScroll(len(rows), abs.H)
	for screenRow := 0; screenRow < abs.H; screenRow++ {
		index := t.scrollY + screenRow
		if index < 0 || index >= len(rows) {
			continue
		}
		row := rows[index]
		x := abs.X + row.indent
		fg := t.FG
		if row.entry.hasFG {
			fg = row.entry.fg
		}
		if row.marker != 0 {
			surface.SetCell(x, abs.Y+screenRow, tui.Cell{Ch: row.marker, FG: t.FocusFG, BG: t.BG, Bold: true})
			x += 2
		}
		limit := textWidth - row.indent
		if row.marker != 0 {
			limit -= 2
		}
		text := []rune(row.text)
		if limit > 0 && len(text) > limit {
			text = text[:limit]
		}
		surface.WriteString(x, abs.Y+screenRow, string(text), tui.Cell{FG: fg, BG: t.BG})
	}
	if abs.W > 1 {
		t.drawScrollbar(surface, abs, component.HasFocus, len(rows))
	}
}

func (t *TextView) clampScroll(total int, height int) {
	maxScroll := total - height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if t.scrollY > maxScroll {
		t.scrollY = maxScroll
	}
	if t.scrollY < 0 {
		t.scrollY = 0
	}
}

// drawScrollbar paints the right-hand track with up/down arrows and a thumb whose
// position reflects scrollY. Focused views use FocusFG; unfocused ones are dimmed.
func (t *TextView) drawScrollbar(surface Surface, abs Rect, focused bool, total int) {
	color := tui.ANSIColor(8)
	if focused {
		color = t.FocusFG
	}
	x := abs.Right()
	for row := 0; row < abs.H; row++ {
		surface.SetCell(x, abs.Y+row, tui.Cell{Ch: '│', FG: color, BG: t.BG})
	}
	surface.SetCell(x, abs.Y, tui.Cell{Ch: '▲', FG: color, BG: t.BG, Bold: focused})
	surface.SetCell(x, abs.Bottom(), tui.Cell{Ch: '▼', FG: color, BG: t.BG, Bold: focused})
	span := total - abs.H
	track := abs.H - 2
	if span > 0 && track > 0 {
		thumb := t.scrollY * (track - 1) / span
		if thumb > track-1 {
			thumb = track - 1
		}
		surface.SetCell(x, abs.Y+1+thumb, tui.Cell{Ch: '█', FG: color, BG: t.BG, Bold: focused})
	}
}

func (t *TextView) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	if !event.Down {
		return true
	}
	abs := component.AbsoluteBounds()
	if abs.W > 1 && event.X == abs.Right() {
		if event.Y == abs.Y {
			t.scrollBy(-1)
			return true
		}
		if event.Y == abs.Bottom() {
			t.scrollBy(1)
			return true
		}
	}
	// A click on a fold marker toggles that entry.
	textWidth := abs.W - 1
	if textWidth < 1 {
		textWidth = abs.W
	}
	rows := t.layoutRows(textWidth)
	index := t.scrollY + (event.Y - abs.Y)
	if index >= 0 && index < len(rows) {
		row := rows[index]
		if row.marker != 0 && event.X >= abs.X+row.indent && event.X <= abs.X+row.indent+1 {
			row.entry.Toggle()
			return true
		}
	}
	return false
}

func (t *TextView) handleScroll(_ *VisualComponent, event tui.ScrollEvent) bool {
	t.scrollBy(-event.Delta)
	return true
}

func (t *TextView) scrollBy(delta int) {
	t.scrollY += delta
	if t.scrollY < 0 {
		t.scrollY = 0
	}
	abs := t.Component.AbsoluteBounds()
	textWidth := abs.W - 1
	if textWidth < 1 {
		textWidth = abs.W
	}
	rows := t.layoutRows(textWidth)
	maxScroll := len(rows) - abs.H
	if maxScroll < 0 {
		maxScroll = 0
	}
	// Re-enable follow only when the user is back at the very bottom.
	if t.scrollY >= maxScroll {
		t.scrollY = maxScroll
		t.follow = true
	} else {
		t.follow = false
	}
}

func (t *TextView) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyUp:
		t.scrollBy(-1)
		return true
	case tui.KeyDown:
		t.scrollBy(1)
		return true
	case tui.KeyPageUp:
		t.scrollBy(-5)
		return true
	case tui.KeyPageDown:
		t.scrollBy(5)
		return true
	}
	return false
}

// wrapText breaks text into segments no wider than width, preferring word breaks
// but hard-splitting words that are themselves too long.
func wrapText(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	if text == "" {
		return []string{""}
	}
	var rows []string
	for _, word := range strings.Fields(text) {
		if len(rows) == 0 {
			rows = append(rows, "")
		}
		last := rows[len(rows)-1]
		candidate := word
		if last != "" {
			candidate = last + " " + word
		}
		if len([]rune(candidate)) <= width {
			rows[len(rows)-1] = candidate
			continue
		}
		if last != "" {
			rows = append(rows, "")
		}
		runes := []rune(word)
		for len(runes) > width {
			rows[len(rows)-1] = string(runes[:width])
			rows = append(rows, "")
			runes = runes[width:]
		}
		rows[len(rows)-1] = string(runes)
	}
	if len(rows) == 0 {
		rows = append(rows, "")
	}
	return rows
}
