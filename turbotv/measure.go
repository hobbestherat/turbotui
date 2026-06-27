package tv

import (
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

// measure.go is the one public source of truth for the text-measurement
// primitives the dialog-sizing policy (DialogSpec/ResolveDialogRect) and external
// consumers need to predict how wide and tall content will render. Every function
// here measures in display cells via tui.StringWidth/tui.RuneWidth — turbotui's
// self-contained width logic (no go-runewidth) — so a size prediction matches what
// the widgets actually draw. The unexported helpers that historically lived next to
// their widgets now delegate here so the logic exists in exactly one place.

// LongestLineWidth returns the display width of the widest hard line (split on
// '\n') in s. It is the canonical way to size a dialog to a message before that
// message is word-wrapped.
func LongestLineWidth(s string) int {
	widest := 0
	for _, line := range strings.Split(s, "\n") {
		if width := tui.StringWidth(line); width > widest {
			widest = width
		}
	}
	return widest
}

// ButtonLabelWidth is the natural cell width a button needs to show its label with
// the "[ … ]" / "► … ◄" chrome and stay readable. It strips the mnemonic marker
// first so "&Yes" measures as "Yes", and clamps up to minButtonWidth so short
// captions (OK/Yes/No) do not render as a cramped "[…]". Width is independent of
// height: a taller button is not wider, so this is unaffected by bounds.H (see
// ButtonHeight).
func ButtonLabelWidth(label string) int {
	clean, _ := ParseMnemonic(label)
	width := tui.StringWidth(clean) + 4 // two cells of chrome on each side
	if width < minButtonWidth {
		width = minButtonWidth
	}
	return width
}

// ButtonHeight is the cell height a button renders at: it comes purely from the
// button's bounds, defaulting to a single row when bounds.H is unset (0 or less).
// A button fills every row of its bounds as a solid "[ … ]" block with the caption
// and focus chevrons on the vertically-centred row, so a caller makes a taller
// button simply by giving it a taller Rect. NewButtonRow uses this to size a footer
// row to its tallest button (gogent#529).
func ButtonHeight(bounds Rect) int {
	if bounds.H < 1 {
		return 1
	}
	return bounds.H
}

// ParseMnemonic strips the '&' mnemonic marker from a label and returns the clean
// text plus the rune index that was marked (-1 when none). A literal '&' is written
// as "&&".
func ParseMnemonic(label string) (string, int) {
	runes := []rune(label)
	out := make([]rune, 0, len(runes))
	hotIndex := -1
	for index := 0; index < len(runes); index++ {
		if runes[index] == '&' && index+1 < len(runes) {
			if runes[index+1] == '&' {
				out = append(out, '&')
				index++
				continue
			}
			if hotIndex < 0 {
				hotIndex = len(out)
			}
			continue
		}
		out = append(out, runes[index])
	}
	return string(out), hotIndex
}

// LabelRow is one wrapped display line produced by WrapLabelRunes: a contiguous
// slice of the source runes and the index in the source at which it begins. Keeping
// the runes as a faithful sub-slice lets a caller locate any rune — e.g. a mnemonic
// hot character — by its index in the original text.
type LabelRow struct {
	Runes []rune
	Start int
}

// WrapLabelRunes word-wraps clean into rows no wider than width terminal columns,
// preferring breaks at spaces and hard-splitting words longer than a row. Newlines
// force a break (and are themselves dropped). Each returned row is a contiguous
// sub-slice of clean, so the caller can locate any rune by its index in clean.
// len(WrapLabelRunes(runes, w)) predicts a Wrap-enabled Label's height at width w.
func WrapLabelRunes(clean []rune, width int) []LabelRow {
	if width < 1 {
		width = 1
	}
	var rows []LabelRow
	n := len(clean)
	rowStart := 0   // index in clean of the first rune on the current row
	col := 0        // display width consumed on the current row
	lastSpace := -1 // index in clean of the most recent space on the current row
	commit := func(end int) {
		rows = append(rows, LabelRow{Runes: clean[rowStart:end], Start: rowStart})
	}
	for i := 0; i < n; i++ {
		ch := clean[i]
		if ch == '\n' {
			// Hard break: close the row before the newline and resume after it.
			commit(i)
			rowStart = i + 1
			col = 0
			lastSpace = -1
			continue
		}
		cw := tui.RuneWidth(ch)
		if col+cw <= width {
			if ch == ' ' {
				lastSpace = i
			}
			col += cw
			continue
		}
		// The rune does not fit on the current row.
		if ch == ' ' {
			// A space that overflows is never useful at a row end; drop it and break.
			commit(i)
			rowStart = i + 1
			col = 0
			lastSpace = -1
			continue
		}
		if lastSpace >= rowStart {
			// Break just after the last space so the current word starts the next row.
			commit(lastSpace)
			rowStart = lastSpace + 1
			col = 0
			for k := rowStart; k <= i; k++ {
				col += tui.RuneWidth(clean[k])
			}
			lastSpace = -1
			if col <= width {
				continue
			}
			// The word fragment alone is wider than a row: hard-split it below.
		}
		// No breakable space (or a single word longer than a row): hard-split here.
		commit(i)
		rowStart = i
		col = cw
		lastSpace = -1
	}
	commit(n)
	return rows
}

// WrapText breaks text into segments no wider than width runes, preserving the
// inter-word whitespace — leading indentation and internal runs of spaces — that a
// naive strings.Fields pass would discard or collapse. Tabs are first expanded to
// tab stops. Tokens (words or whitespace runs) longer than width are hard-split; a
// line that fits whole is emitted verbatim, so the displayed text matches the
// source. This is the exact wrap a TextView renders, so len(WrapText(text, w))
// predicts its row count.
func WrapText(text string, width int) []string {
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
			// current line so alignment survives; otherwise let it be absorbed by the
			// line break instead of carrying a lone separator to the next row.
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
