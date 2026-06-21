package tv

import (
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

// StyledSpan is a contiguous run of text within a single logical line that shares
// one style. A line built from spans can mix colours, bold, italic, underline and
// background per run — the foundation for rendering inline-formatted text such as
// Markdown. HasFG/HasBG select whether the span overrides the view's default
// foreground/background; when false the TextView's FG/BG is used.
type StyledSpan struct {
	Text      string
	FG        tui.Color
	HasFG     bool
	BG        tui.Color
	HasBG     bool
	Bold      bool
	Underline bool
	Italic    bool
}

// TextEntry is one logical line in a TextView. Entries may carry their own colour
// and may be foldable, in which case their children are shown/hidden by clicking
// the ▸/▾ marker. Build a tree with Add/AddColored and stream text with AppendText.
// An entry created with AddStyled additionally carries per-span styling in spans;
// its text mirrors the concatenated span text so AllText/copy stay correct.
type TextEntry struct {
	text      string
	fg        tui.Color
	hasFG     bool
	spans     []StyledSpan
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
	e.view.clearSelection()
	e.view.touch()
}

func (e *TextEntry) SetCollapsed(collapsed bool) {
	e.collapsed = collapsed
	e.view.clearSelection()
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

	// Drag-to-select state, mirroring MultiLineInput. Selection coordinates are
	// (visual-row index, column) in the flattened layout returned by layoutRows.
	// That layout depends only on content and width — not on scrollY — so the
	// anchor/active rows stay valid while the user scrolls. selAnchorRow is -1 when
	// there is no selection.
	selAnchorRow int
	selAnchorCol int
	selActiveRow int
	selActiveCol int
	// selecting is true between a press and its release; while it is set, motion
	// events extend the selection. pressRow/pressCol remember where the button went
	// down so the anchor is only committed once the pointer drags away from that
	// point — a plain click therefore leaves no selection.
	selecting bool
	pressRow  int
	pressCol  int
	// selWidth records the content width the selection's row indices were resolved
	// against. Those indices address a layout that depends on width, so if the view
	// is later drawn at a different width (a resize re-wraps the content into a
	// different number of rows) the selection is dropped rather than left pointing
	// at the wrong rows.
	selWidth int

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
		FG:           activeTheme.WindowFG,
		BG:           activeTheme.WindowBG,
		FocusFG:      activeTheme.MnemonicFG,
		follow:       true,
		Wrap:         true,
		selAnchorRow: -1,
	}
	view.Component = NewComponent(bounds)
	view.Component.Focusable = true
	view.Component.DrawFn = view.draw
	view.Component.OnTypeFn = view.handleType
	view.Component.OnScrollFn = view.handleScroll
	view.Component.OnClickFn = view.handleClick
	view.Component.CopyFn = view.copySelection
	view.Component.OnFocusFn = view.handleFocus
	view.SetText(text)
	return view
}

// handleFocus aborts an in-progress drag when the view loses focus. Without it a
// drag interrupted by a focus change (no release event is delivered) would leave
// selecting set, so the next press would be misread as a drag continuation that
// extends the stale selection instead of starting a fresh one. The committed
// selection itself is left intact so its highlight survives the focus change.
func (t *TextView) handleFocus(_ *VisualComponent, focused bool) {
	if !focused {
		t.selecting = false
		t.draggingThumb = false
	}
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

// copySelection is the CopyFn: when the user has drag-selected a range it copies
// exactly that range; otherwise it falls back to copying the whole content, the
// long-standing behaviour. The empty case reports ok=false so a copy of nothing
// is a no-op.
func (t *TextView) copySelection(_ *VisualComponent) (string, bool) {
	if t.hasSelection() {
		text := t.selectionText()
		return text, text != ""
	}
	text := t.AllText()
	return text, text != ""
}

// clearSelection drops any active selection and aborts an in-progress drag. It is
// called when the content or fold layout changes, since the row indices a
// selection is expressed in would no longer line up with the new layout.
func (t *TextView) clearSelection() {
	t.selAnchorRow = -1
	t.selecting = false
}

// hasSelection reports whether a non-empty selection is active.
func (t *TextView) hasSelection() bool {
	return t.selAnchorRow >= 0 &&
		(t.selAnchorRow != t.selActiveRow || t.selAnchorCol != t.selActiveCol)
}

// selectionOrdered returns the selection as a forward-ordered (row0,col0,row1,col1)
// so callers need not worry which end is the anchor.
func (t *TextView) selectionOrdered() (int, int, int, int) {
	r0, c0, r1, c1 := t.selAnchorRow, t.selAnchorCol, t.selActiveRow, t.selActiveCol
	if r0 > r1 || (r0 == r1 && c0 > c1) {
		r0, c0, r1, c1 = r1, c1, r0, c0
	}
	return r0, c0, r1, c1
}

// isSelected reports whether the given column of the given visual row falls inside
// the selection. Columns are rune indices into the row's text; a column at or past
// the end of the text can still be selected (the blank tail of a spanned row),
// which the draw loop paints out to the right edge.
func (t *TextView) isSelected(row int, col int) bool {
	if !t.hasSelection() {
		return false
	}
	r0, c0, r1, c1 := t.selectionOrdered()
	if row < r0 || row > r1 {
		return false
	}
	if r0 == r1 {
		return col >= c0 && col < c1
	}
	if row == r0 {
		return col >= c0
	}
	if row == r1 {
		return col < c1
	}
	return true
}

// selectionText reconstructs the selected text from the wrapped layout. Rows that
// belong to the same entry (wrapped continuations of one logical line) are joined
// with no separator; rows that belong to different entries are joined with a
// newline, so the copy matches AllText's one-line-per-entry shape. The result is
// exactly the run of characters highlighted on screen.
func (t *TextView) selectionText() string {
	if !t.hasSelection() {
		return ""
	}
	rows, _, _ := t.metrics(t.Component.AbsoluteBounds())
	if len(rows) == 0 {
		return ""
	}
	r0, c0, r1, c1 := t.selectionOrdered()
	if r0 < 0 {
		r0 = 0
	}
	if r1 >= len(rows) {
		r1 = len(rows) - 1
	}
	var builder strings.Builder
	for row := r0; row <= r1; row++ {
		if row > r0 && rows[row].entry != rows[row-1].entry {
			builder.WriteByte('\n')
		}
		runes := []rune(rows[row].text)
		lo, hi := 0, len(runes)
		if row == r0 {
			lo = clampCol(len(runes), c0)
		}
		if row == r1 {
			hi = clampCol(len(runes), c1)
		}
		if lo < hi {
			builder.WriteString(string(runes[lo:hi]))
		}
	}
	return builder.String()
}

func (t *TextView) Root() *VisualComponent {
	return t.Component
}

// SetText replaces all content with plain lines (one entry per '\n').
func (t *TextView) SetText(text string) {
	t.entries = nil
	t.clearSelection()
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
	t.clearSelection()
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

// AddStyled appends one logical line built from per-span styling. The entry's
// plain text is the concatenation of the span texts, so AllText and copy behave
// exactly as for AddLine/AddColored; the spans drive rendering (colour, bold,
// italic, underline, background) and are split span-aware when the line is
// wrapped. Like AddLine, the spans are treated as a single line: embedded newlines
// are not split into separate rows. For a styled entry the per-entry fg/hasFG
// fields are unused — each span carries its own colour.
func (t *TextView) AddStyled(spans []StyledSpan) *TextEntry {
	entry := &TextEntry{text: spansText(spans), spans: spans, view: t}
	t.entries = append(t.entries, entry)
	t.touch()
	return entry
}

// spansText concatenates the text of every span, giving the plain-text form of a
// styled line.
func spansText(spans []StyledSpan) string {
	var b strings.Builder
	for _, span := range spans {
		b.WriteString(span.Text)
	}
	return b.String()
}

func (t *TextView) ScrollToBottom() {
	t.follow = true
}

// ScrollToTop anchors the view at the first line and stops following the bottom,
// so the content opens at the top and stays there until the user scrolls (or
// ScrollToBottom re-enables following as content is added). Use it for views
// that should open at the top — info/help dialogs — rather than pinned to the
// bottom like chat or logs.
func (t *TextView) ScrollToTop() {
	t.follow = false
	t.scrollY = 0
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
// spans, when non-nil, carries the per-span styling covering this visual row (a
// span-aware slice of the entry's spans); rows without spans render with a single
// foreground via the plain path.
type renderRow struct {
	entry  *TextEntry
	text   string
	spans  []StyledSpan
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
			if len(entry.spans) > 0 {
				// Styled line: wrap span-aware so each visual row keeps the per-span
				// styling covering its segment.
				rowSpans := [][]StyledSpan{entry.spans}
				if t.Wrap {
					rowSpans = wrapStyledSpans(entry.spans, avail)
				}
				for index, segSpans := range rowSpans {
					row := renderRow{entry: entry, text: spansText(segSpans), spans: segSpans, indent: indent}
					if index == 0 {
						row.marker = marker
					}
					rows = append(rows, row)
				}
			} else {
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
	// A selection's row indices address the layout at the width it was made; if the
	// view is now wider or narrower the content has re-wrapped, so the indices no
	// longer line up — drop the selection rather than highlight (and later copy) the
	// wrong rows.
	if t.hasSelection() && t.selWidth != textWidth {
		t.clearSelection()
	}
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
		// A styled entry paints per span so each run keeps its own colour and
		// attributes; a plain entry takes the single-fg fast path below. The entry's
		// spans (not the per-row slice, which is nil for an empty wrapped row) decide
		// the path, so an empty styled line stays on the styled path and paints
		// nothing rather than silently falling back.
		if len(row.entry.spans) > 0 {
			t.drawStyledRow(surface, x, abs.Y+screenRow, row.spans, limit)
		} else {
			// With wrap off a too-long line is clipped with a trailing ellipsis so the
			// dropped tail is signalled rather than vanishing silently; wrapped rows
			// already fit, so they are only width-clipped (no ellipsis) for safety.
			ellipsis := "…"
			if t.Wrap {
				ellipsis = ""
			}
			surface.WriteString(x, abs.Y+screenRow, Truncate(row.text, limit, ellipsis), tui.Cell{FG: fg, BG: t.BG})
		}
		// Overlay the selection on top of the base render (plain or styled), so the
		// highlight covers both paths uniformly.
		t.drawSelection(surface, x, abs.Y+screenRow, index, limit)
	}
	if bar {
		t.drawScrollbar(surface, abs, component.Focused(), len(rows))
	}
}

// drawSelection repaints the selected cells of one visual row in the selection
// colours, on top of whatever the base render already drew. limit is the number
// of columns available to the row (after indent and any fold marker). It recolours
// each selected cell in place — reading back the cell the base pass drew and
// swapping only FG/BG — so a styled span keeps its bold/italic/underline under the
// highlight (matching MultiLineInput, which only overrides the colours). Columns
// past the end of the text read back as blanks, so a multi-row selection runs to
// the right edge. Columns are rune indices — the same one-rune-per-column
// simplification MultiLineInput uses — exact for the ASCII content selection is
// used on.
func (t *TextView) drawSelection(surface Surface, x int, y int, row int, limit int) {
	if !t.hasSelection() {
		return
	}
	for col := 0; col < limit; col++ {
		if !t.isSelected(row, col) {
			continue
		}
		cell := surface.ReadCell(x+col, y)
		if cell.Ch == 0 {
			cell.Ch = ' '
		}
		cell.FG = activeTheme.SelectionFG
		cell.BG = activeTheme.SelectionBG
		surface.SetCell(x+col, y, cell)
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
	// A release ends any in-progress thumb drag or text selection.
	if !event.Down {
		t.draggingThumb = false
		t.selecting = false
		return true
	}
	// While dragging the thumb, every motion event maps the pointer Y to scroll,
	// even if the pointer drifts off the 1-column track.
	if t.draggingThumb {
		t.scrollToThumb(abs, event.Y)
		return true
	}
	rows, textWidth, bar := t.metrics(abs)
	// Normalise scrollY to the valid range before mapping the pointer to a row, so a
	// click that arrives before the first draw (while following, scrollY is a huge
	// sentinel) lands on the right row instead of the last one.
	t.clampScroll(len(rows), abs.H)
	// The scrollbar only claims the pointer when a selection drag is not already in
	// progress; mid-drag the pointer wandering onto the track must keep extending the
	// selection rather than silently grabbing the thumb.
	if !t.selecting && bar && event.X == abs.Right() {
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
	// A fresh press on a fold marker toggles that entry (never during a drag, so
	// dragging a selection across a marker does not collapse it).
	index := t.scrollY + (event.Y - abs.Y)
	if !t.selecting && index >= 0 && index < len(rows) {
		row := rows[index]
		if row.marker != 0 && event.X >= abs.X+row.indent && event.X <= abs.X+row.indent+1 {
			row.entry.Toggle()
			return true
		}
	}
	if len(rows) == 0 {
		return false
	}
	// Map the pointer to a visual row and a rune column within that row's text,
	// accounting for the row's indent and any fold marker.
	row := index
	if row < 0 {
		row = 0
	}
	if row >= len(rows) {
		row = len(rows) - 1
	}
	rr := rows[row]
	textX := abs.X + rr.indent
	if rr.marker != 0 {
		textX += 2
	}
	col := event.X - textX
	if col < 0 {
		col = 0
	}
	col = clampCol(len([]rune(rr.text)), col)
	if !t.selecting {
		// Fresh press: it must land inside the view. Record the press point and clear
		// any old selection, but do not anchor yet — only a drag starts a selection,
		// so a plain click leaves nothing selected and copy still falls back to all.
		if !abs.Contains(event.X, event.Y) {
			return false
		}
		t.pressRow = row
		t.pressCol = col
		t.selAnchorRow = -1
		t.selecting = true
		// Remember the width this gesture's row indices are resolved against, so a
		// later draw at a different width drops the selection instead of mis-mapping it.
		t.selWidth = textWidth
		return true
	}
	// Drag motion: the first time the pointer leaves the press point, anchor the
	// selection there; then extend the active end to the current pointer position.
	if row != t.pressRow || col != t.pressCol {
		if t.selAnchorRow < 0 {
			t.selAnchorRow = t.pressRow
			t.selAnchorCol = t.pressCol
		}
		t.selActiveRow = row
		t.selActiveCol = col
	}
	return true
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

// drawStyledRow paints a styled visual row one span at a time, giving each span its
// own foreground, background, bold, underline and italic, clipped to limit terminal
// columns. Each span is a uniform-style run drawn with Surface.WriteString, so the
// styled path inherits the plain path's display-width, combining-mark folding and
// double-width handling rather than re-deriving it per rune.
//
// Two details keep it cell-for-cell faithful to the single-pass plain path:
//   - A span's leading zero-width runes (combining marks) are folded onto the end of
//     the previous span's text, so a mark whose base glyph sits in an earlier span
//     still attaches to that base instead of being dropped at the WriteString seam.
//   - Once a span does not fully fit (it had to be truncated), drawing stops: the
//     row is full, so a later narrow span cannot leak into a column the plain path —
//     which truncates the whole row as one unit — would leave blank.
func (t *TextView) drawStyledRow(surface Surface, x int, y int, spans []StyledSpan, limit int) {
	// segment is a uniform-style run to draw; building these first lets a span's
	// leading combining marks migrate onto the previous run.
	type segment struct {
		text  string
		style tui.Cell
	}
	segs := make([]segment, 0, len(spans))
	for _, span := range spans {
		runes := []rune(span.Text)
		lead := 0
		for lead < len(runes) && tui.RuneWidth(runes[lead]) == 0 {
			lead++
		}
		if lead > 0 && len(segs) > 0 {
			// Fold leading combining marks onto the previous run so WriteString attaches
			// them to its last base glyph. With no previous run they stay put, matching
			// the plain path (a leading mark there also has no base).
			segs[len(segs)-1].text += string(runes[:lead])
			runes = runes[lead:]
		}
		if len(runes) == 0 {
			continue
		}
		style := tui.Cell{FG: t.FG, BG: t.BG, Bold: span.Bold, Underline: span.Underline, Italic: span.Italic}
		if span.HasFG {
			style.FG = span.FG
		}
		if span.HasBG {
			style.BG = span.BG
		}
		segs = append(segs, segment{text: string(runes), style: style})
	}
	col := 0
	for _, seg := range segs {
		if col >= limit {
			return
		}
		text := Truncate(seg.text, limit-col, "")
		if text != "" {
			surface.WriteString(x+col, y, text, seg.style)
			col += tui.StringWidth(text)
		}
		if text != seg.text {
			// This run was clipped to the limit; nothing after it can render.
			return
		}
	}
}

// styledRune is a single rune tagged with the index of the span it belongs to, so
// span-aware wrapping can reconstruct per-row spans after breaking the line.
type styledRune struct {
	r    rune
	span int
}

// wrapStyledSpans word-wraps a styled line to width terminal columns, returning one
// []StyledSpan per visual row. It mirrors wrapText (same word-break and hard-split
// rules, whitespace dropped at a wrap point) while preserving each span's styling:
// the line is flattened to span-tagged runes, wrapped, then each row's runes are
// regrouped back into spans by their span tag. Width is measured in runes, exactly
// as wrapText measures it, so styled and plain lines wrap identically; double-width
// glyphs are not counted as two columns here (a deliberate parity with wrapText).
func wrapStyledSpans(spans []StyledSpan, width int) [][]StyledSpan {
	rows := wrapStyledRunes(styledRunesOf(spans), width)
	out := make([][]StyledSpan, len(rows))
	for i, row := range rows {
		out[i] = spansOfStyledRow(row, spans)
	}
	return out
}

// styledRunesOf flattens spans into a single tagged-rune stream, expanding tabs to
// tab stops (matching wrapText) and attributing each inserted space to the tab's
// span so styling stays aligned.
func styledRunesOf(spans []StyledSpan) []styledRune {
	var out []styledRune
	col := 0
	for i, span := range spans {
		for _, r := range span.Text {
			if r == '\t' {
				n := textViewTabWidth - col%textViewTabWidth
				for k := 0; k < n; k++ {
					out = append(out, styledRune{r: ' ', span: i})
				}
				col += n
				continue
			}
			out = append(out, styledRune{r: r, span: i})
			col++
		}
	}
	return out
}

// wrapStyledRunes is the span-aware twin of wrapText: it breaks runes into rows no
// wider than width runes, preferring word boundaries, hard-splitting tokens longer
// than a whole line, and letting whitespace at a wrap point be absorbed by the
// break so the following row starts aligned.
func wrapStyledRunes(runes []styledRune, width int) [][]styledRune {
	if width < 1 {
		width = 1
	}
	if len(runes) == 0 {
		return [][]styledRune{nil}
	}
	var rows [][]styledRune
	line := make([]styledRune, 0, width)
	flush := func() {
		row := make([]styledRune, len(line))
		copy(row, line)
		rows = append(rows, row)
		line = line[:0]
	}
	place := func(token []styledRune) {
		for len(token) > 0 {
			room := width - len(line)
			if room <= 0 {
				flush()
				continue
			}
			if len(token) <= room {
				line = append(line, token...)
				return
			}
			if len(line) > 0 {
				flush()
				continue
			}
			line = append(line, token[:width]...)
			token = token[width:]
			flush()
		}
	}
	first := true
	for _, token := range tokenizeStyledWhitespace(runes) {
		if token[0].r == ' ' && !first {
			// Internal whitespace: keep the run while it still fits, else let the break
			// absorb it rather than carrying a lone separator to the next row.
			if len(line) > 0 && len(token) <= width-len(line) {
				line = append(line, token...)
			} else if len(line) > 0 {
				flush()
			}
		} else {
			place(token)
		}
		first = false
	}
	flush()
	return rows
}

// tokenizeStyledWhitespace splits a tagged-rune stream into maximal runs that
// alternate between spaces and non-spaces, the span-aware analogue of
// tokenizeWhitespace. Tabs are assumed already expanded to spaces.
func tokenizeStyledWhitespace(runes []styledRune) [][]styledRune {
	var tokens [][]styledRune
	for i := 0; i < len(runes); {
		space := runes[i].r == ' '
		j := i + 1
		for j < len(runes) && (runes[j].r == ' ') == space {
			j++
		}
		tokens = append(tokens, runes[i:j])
		i = j
	}
	return tokens
}

// spansOfStyledRow regroups one wrapped row's tagged runes back into StyledSpans,
// merging consecutive runes that share a span tag and copying that span's style.
func spansOfStyledRow(row []styledRune, spans []StyledSpan) []StyledSpan {
	var out []StyledSpan
	for i := 0; i < len(row); {
		j := i + 1
		for j < len(row) && row[j].span == row[i].span {
			j++
		}
		text := make([]rune, 0, j-i)
		for _, sr := range row[i:j] {
			text = append(text, sr.r)
		}
		seg := spans[row[i].span]
		seg.Text = string(text)
		out = append(out, seg)
		i = j
	}
	return out
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
