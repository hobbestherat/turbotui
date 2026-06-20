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
	// Wrap controls whether long lines are soft-wrapped to fit the width.
	// NewTextView defaults it to true (a scrolling viewer almost always wants
	// wrapping); with it off, clipped lines are marked with a trailing ellipsis.
	Wrap bool

	entries       []*TextEntry
	scrollY       int
	follow        bool
	draggingThumb bool
	viewH         int // last drawn viewport height, used for PageUp/PageDown

	// metric memo: caches the (rows, content width, scrollbar?) decision for a
	// given content version and viewport size, so the steady-state redraw of an
	// overflowing view does not repeat the two-width scrollbar probe every frame.
	metricCached    bool
	metricVersion   uint64
	metricW         int
	metricH         int
	metricRows      []renderRow
	metricTextWidth int
	metricBar       bool

	// layoutRows is memoised by (layoutVersion, cachedWidth, Wrap). layoutVersion
	// is bumped by every content change (via touch), so an unchanged view drawn
	// frame after frame — or queried several times during one event (draw, click,
	// scroll, thumb-drag) — wraps its text once instead of on every call.
	layoutVersion uint64
	cachedRows    []renderRow
	cachedWidth   int
	cachedWrap    bool
	cachedVersion uint64
	layoutCached  bool
}

func NewTextView(text string, bounds Rect) *TextView {
	view := &TextView{
		FG:      activeTheme.WindowFG,
		BG:      activeTheme.WindowBG,
		FocusFG: activeTheme.MnemonicFG,
		follow:  true,
		Wrap:    true,
	}
	view.Component = NewComponent(bounds)
	view.Component.Focusable = true
	view.Component.DrawFn = view.draw
	view.Component.OnTypeFn = view.handleType
	view.Component.OnScrollFn = view.handleScroll
	view.Component.OnClickFn = view.handleClick
	view.Component.CopyFn = view.copyAll
	view.SetText(text)
	return view
}

// AllText returns the full logical content, one entry per line, including the
// children of foldable entries regardless of whether they are collapsed.
func (t *TextView) AllText() string {
	var builder strings.Builder
	for _, entry := range t.entries {
		entry.appendAllText(&builder)
	}
	return strings.TrimRight(builder.String(), "\n")
}

func (e *TextEntry) appendAllText(builder *strings.Builder) {
	builder.WriteString(e.text)
	builder.WriteByte('\n')
	for _, child := range e.children {
		child.appendAllText(builder)
	}
}

// copyAll is the CopyFn: a focused TextView copies its whole content.
func (t *TextView) copyAll(_ *VisualComponent) (string, bool) {
	text := t.AllText()
	return text, text != ""
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
	t.layoutVersion++
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
// clamped to the real maximum during draw. It also bumps layoutVersion so the
// next layoutRows re-wraps the (now changed) content.
func (t *TextView) touch() {
	t.layoutVersion++
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

// layoutRows returns the wrapped display rows for the current content at the
// given width. The result is memoised by (content version, width, Wrap), so the
// expensive per-entry wrapText pass runs at most once per change: repeated calls
// during a single frame (draw plus scroll/click/thumb-drag helpers) and across
// idle redraws return the cached slice without re-allocating.
func (t *TextView) layoutRows(width int) []renderRow {
	if t.layoutCached && t.cachedVersion == t.layoutVersion &&
		t.cachedWidth == width && t.cachedWrap == t.Wrap {
		return t.cachedRows
	}
	rows := t.computeRows(width)
	t.cachedRows = rows
	t.cachedWidth = width
	t.cachedWrap = t.Wrap
	t.cachedVersion = t.layoutVersion
	t.layoutCached = true
	return rows
}

func (t *TextView) computeRows(width int) []renderRow {
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
	t.viewH = abs.H
	if abs.W < 1 || abs.H < 1 {
		return
	}
	rows, textWidth, bar := t.metrics(abs)
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
		// With wrap off a too-long line is clipped with a trailing ellipsis so the
		// dropped tail is signalled rather than vanishing silently; wrapped rows
		// already fit, so they are only width-clipped (no ellipsis) for safety.
		ellipsis := "…"
		if t.Wrap {
			ellipsis = ""
		}
		surface.WriteString(x, abs.Y+screenRow, Truncate(row.text, limit, ellipsis), tui.Cell{FG: fg, BG: t.BG})
	}
	if bar {
		t.drawScrollbar(surface, abs, component.Focused(), len(rows))
	}
}

// metrics resolves the row layout, the usable content width and whether a
// scrollbar is shown for the given bounds. A bar (and its reserved column) is
// only present when the content overflows the viewport, matching Tree/Select.
// Narrowing by the reserved column can only add wrapped rows, so an overflow at
// full width remains an overflow at width-1 — the decision is stable. The result
// is memoised per (content version, width, height) so an idle overflowing view
// does not re-run the probe each frame.
func (t *TextView) metrics(abs Rect) ([]renderRow, int, bool) {
	if t.metricCached && t.metricVersion == t.layoutVersion &&
		t.metricW == abs.W && t.metricH == abs.H {
		return t.metricRows, t.metricTextWidth, t.metricBar
	}
	textWidth := abs.W
	if textWidth < 1 {
		textWidth = 1
	}
	rows := t.layoutRows(textWidth)
	bar := false
	if abs.W > 1 && len(rows) > abs.H {
		bar = true
		textWidth = abs.W - 1
		rows = t.layoutRows(textWidth)
	}
	t.metricCached = true
	t.metricVersion = t.layoutVersion
	t.metricW = abs.W
	t.metricH = abs.H
	t.metricRows = rows
	t.metricTextWidth = textWidth
	t.metricBar = bar
	return rows, textWidth, bar
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
// position reflects scrollY, delegating to the shared scrollbar so all widgets
// look and behave the same. Focused views use FocusFG; unfocused ones are dimmed.
func (t *TextView) drawScrollbar(surface Surface, abs Rect, focused bool, total int) {
	color := tui.ANSIColor(8)
	if focused {
		color = t.FocusFG
	}
	track := Rect{X: abs.Right(), Y: abs.Y, W: 1, H: abs.H}
	drawVScrollbar(surface, track, total, abs.H, t.scrollY, color, t.BG, focused)
}

func (t *TextView) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	abs := component.AbsoluteBounds()
	// A release ends any in-progress thumb drag.
	if !event.Down {
		t.draggingThumb = false
		return true
	}
	// While dragging the thumb, every motion event maps the pointer Y to scroll,
	// even if the pointer drifts off the 1-column track.
	if t.draggingThumb {
		t.scrollToThumb(abs, event.Y)
		return true
	}
	rows, _, bar := t.metrics(abs)
	if bar && event.X == abs.Right() {
		if event.Y == abs.Y {
			t.scrollBy(-1)
			return true
		}
		if event.Y == abs.Bottom() {
			t.scrollBy(1)
			return true
		}
		// Anywhere on the track between the arrows grabs the thumb and starts a drag.
		if event.Y > abs.Y && event.Y < abs.Bottom() {
			t.draggingThumb = true
			t.scrollToThumb(abs, event.Y)
			return true
		}
	}
	// A click on a fold marker toggles that entry.
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

// scrollToThumb maps a Y coordinate on the scrollbar track to a scroll offset.
func (t *TextView) scrollToThumb(abs Rect, y int) {
	track := abs.H - 2
	if track < 1 {
		return
	}
	rows, _, _ := t.metrics(abs)
	span := len(rows) - abs.H
	if span <= 0 {
		return
	}
	pos := y - (abs.Y + 1)
	if pos < 0 {
		pos = 0
	}
	if pos > track-1 {
		pos = track - 1
	}
	denom := track - 1
	if denom < 1 {
		denom = 1
	}
	t.scrollY = pos * span / denom
	if t.scrollY >= span {
		t.scrollY = span
		t.follow = true
	} else {
		t.follow = false
	}
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
	rows, _, _ := t.metrics(abs)
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
		t.scrollBy(-t.page())
		return true
	case tui.KeyPageDown:
		t.scrollBy(t.page())
		return true
	}
	return false
}

// page is the PageUp/PageDown step: one viewport minus a line of overlap so the
// reader keeps a line of context across the jump. It is derived from the height
// recorded by the last draw.
func (t *TextView) page() int {
	page := t.viewH - 1
	if page < 1 {
		page = 1
	}
	return page
}

// textViewTabWidth is the tab stop used when expanding tabs before wrapping, so
// indented and column-aligned content keeps its layout instead of collapsing
// each tab to a single break.
const textViewTabWidth = 8

// wrapText breaks text into segments no wider than width runes, preserving the
// inter-word whitespace — leading indentation and internal runs of spaces — that
// a naive strings.Fields pass would discard or collapse. Tabs are first expanded
// to tab stops. Tokens (words or whitespace runs) longer than width are
// hard-split; a line that fits whole is emitted verbatim, so the displayed text
// matches the source.
func wrapText(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	if text == "" {
		return []string{""}
	}
	text = expandTabs(text, textViewTabWidth)
	var rows []string
	line := make([]rune, 0, width)
	flush := func() {
		rows = append(rows, string(line))
		line = line[:0]
	}
	// place lays content (leading indentation or a word) onto the current line,
	// breaking when full and hard-splitting a token wider than a whole line.
	place := func(runes []rune) {
		for len(runes) > 0 {
			room := width - len(line)
			if room <= 0 {
				flush()
				continue
			}
			if len(runes) <= room {
				line = append(line, runes...)
				return
			}
			if len(line) > 0 {
				flush()
				continue
			}
			line = append(line, runes[:width]...)
			runes = runes[width:]
			flush()
		}
	}
	first := true
	for _, token := range tokenizeWhitespace(text) {
		runes := []rune(token)
		if token[0] == ' ' && !first {
			// Internal whitespace: keep a run of spaces when it still fits on the
			// current line so alignment survives; otherwise let it be absorbed by
			// the line break instead of carrying a lone separator to the next row.
			if len(line) > 0 && len(runes) <= width-len(line) {
				line = append(line, runes...)
			} else if len(line) > 0 {
				flush()
			}
		} else {
			// Leading indentation (the first token) and words are content.
			place(runes)
		}
		first = false
	}
	flush()
	return rows
}

// tokenizeWhitespace splits text into a sequence of maximal runs that alternate
// between spaces and non-spaces, so wrapping can keep the original separators
// instead of discarding them. Tabs are assumed already expanded to spaces.
func tokenizeWhitespace(text string) []string {
	runes := []rune(text)
	var tokens []string
	for i := 0; i < len(runes); {
		space := runes[i] == ' '
		j := i + 1
		for j < len(runes) && (runes[j] == ' ') == space {
			j++
		}
		tokens = append(tokens, string(runes[i:j]))
		i = j
	}
	return tokens
}

// expandTabs replaces each tab with spaces up to the next multiple of tab
// columns, so aligned content survives wrapping. Columns are counted in runes,
// matching wrapText's width measure.
func expandTabs(text string, tab int) string {
	if !strings.ContainsRune(text, '\t') {
		return text
	}
	if tab < 1 {
		tab = 1
	}
	var b strings.Builder
	col := 0
	for _, r := range text {
		if r == '\t' {
			n := tab - col%tab
			for i := 0; i < n; i++ {
				b.WriteByte(' ')
			}
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}
