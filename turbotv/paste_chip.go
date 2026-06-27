package tv

import (
	"fmt"
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

// Paste-chip model (gogent #501, turbotui half).
//
// When a multi-line paste is collapsed into an atomic "[pasted N lines]" chip, the
// chip is represented inside the existing flat buffer as EXACTLY ONE rune — a unique
// code point allocated from Supplementary Private-Use Area-A (U+F0000…U+FFFFD). The
// verbatim original text (with its newlines) lives out-of-band in a chipStore keyed by
// that rune. Because a chip is a single rune in the line/text, the existing rune-offset
// caret machinery treats it atomically for free: Left/Right step over it, Backspace/
// Delete remove it whole, and selection slices include it whole.
//
// This is Approach B from design.md (sentinel rune + side store), chosen over the
// segment-model Approach A so the exported Lines/CursorX/CursorY (and Text/Cursor)
// fields keep their types — gogent reads and mutates those raw fields directly
// (ui/tui/mention_completer.go, ui/tui/commands_dialog.go), so a type change would
// force a large downstream rewrite. The trade-off B accepts is that Lines[y] may now
// contain an opaque chip sentinel rune; IsPasteChipRune lets a consumer detect/skip it,
// and every ingest path (paste, typed runes) strips any pre-existing SPUA-A rune so
// real user content can never collide with the marker.
const (
	chipRuneBase = 0xF0000
	chipRuneEnd  = 0xFFFFD
)

// IsPasteChipRune reports whether r is a paste-chip sentinel. It is exported so a host
// (e.g. gogent's @-mention / slash-command parsing) can skip chip markers when it
// scans the raw Lines/Text content, instead of hard-coding the private-use range.
func IsPasteChipRune(r rune) bool {
	return r >= chipRuneBase && r <= chipRuneEnd
}

// chipStore maps each allocated chip sentinel rune to its verbatim original text.
// Its zero value is ready to use (add lazily initialises the map). Markers are handed
// out from a monotonically increasing counter so a deleted-then-recreated chip can
// never alias a still-live one.
type chipStore struct {
	byRune map[rune]string
	next   rune
}

// add stores text under a freshly allocated chip rune and returns that rune.
func (s *chipStore) add(text string) rune {
	if s.byRune == nil {
		s.byRune = make(map[rune]string)
	}
	if s.next < chipRuneBase || s.next > chipRuneEnd {
		s.next = chipRuneBase
	}
	r := s.next
	s.byRune[r] = text
	s.next++
	return r
}

// text returns the original text for a chip rune, if known.
func (s *chipStore) text(r rune) (string, bool) {
	t, ok := s.byRune[r]
	return t, ok
}

// reset drops every stored chip. Callers use it when the whole buffer is replaced
// (SetText/Clear) so chip entries cannot leak across a content swap.
func (s *chipStore) reset() {
	s.byRune = nil
	s.next = chipRuneBase
}

// keepOnly prunes entries whose marker rune is no longer present in the buffer, so a
// long sequence of paste→delete cycles does not leak. present holds the chip runes
// still live in the buffer.
func (s *chipStore) keepOnly(present map[rune]bool) {
	for r := range s.byRune {
		if !present[r] {
			delete(s.byRune, r)
		}
	}
}

// expand turns a rune slice (possibly containing chip markers) into real text by
// replacing each marker with its stored original. A chip-free slice round-trips
// unchanged. An orphan marker with no store entry is dropped.
func (s chipStore) expand(runes []rune) string {
	hasChip := false
	for _, r := range runes {
		if IsPasteChipRune(r) {
			hasChip = true
			break
		}
	}
	if !hasChip {
		return string(runes)
	}
	var b strings.Builder
	for _, r := range runes {
		if IsPasteChipRune(r) {
			if t, ok := s.byRune[r]; ok {
				b.WriteString(t)
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// chipLabel is the compact display label for a chip holding text. N is the line count
// of the original (count('\n')+1); a chip only exists when the paste contained a
// newline, so N is always >= 2 and the label is always plural.
func chipLabel(text string) string {
	n := strings.Count(text, "\n") + 1
	return fmt.Sprintf("[pasted %d lines]", n)
}

// chipLabelFit returns the chip label truncated to at most width display columns,
// appending an ellipsis when it overflows ("[pasted 12 lines…]"). When even the
// ellipsis cannot fit it hard-truncates the raw label to width. The stored full text
// is never altered — only the rendered label shrinks.
func chipLabelFit(text string, width int) []rune {
	label := []rune(chipLabel(text))
	if width < 1 {
		width = 1
	}
	if tui.StringWidth(string(label)) <= width {
		return label
	}
	ell := '…'
	ellW := tui.RuneWidth(ell)
	if ellW < 1 {
		ellW = 1
	}
	if width <= ellW {
		return truncRunes(label, width)
	}
	return append(truncRunes(label, width-ellW), ell)
}

// truncRunes returns the longest prefix of runes whose display width is <= width.
func truncRunes(runes []rune, width int) []rune {
	out := make([]rune, 0, len(runes))
	w := 0
	for _, r := range runes {
		rw := tui.RuneWidth(r)
		if rw < 1 {
			rw = 1
		}
		if w+rw > width {
			break
		}
		out = append(out, r)
		w += rw
	}
	return out
}

// sanitizePaste cleans an incoming paste: CR is dropped (CRLF→LF), other control runes
// below 0x20 are dropped (except '\n'), and any pre-existing SPUA-A chip sentinel is
// stripped so user content cannot collide with the marker. It reports whether a '\n'
// survived, which is what decides chip-vs-literal in the widgets' handlePaste.
func sanitizePaste(s string) (string, bool) {
	var b strings.Builder
	b.Grow(len(s))
	hasNewline := false
	for _, r := range s {
		switch {
		case r == '\r':
			continue
		case r == '\n':
			hasNewline = true
			b.WriteRune(r)
		case r < 0x20:
			continue
		case IsPasteChipRune(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String(), hasNewline
}
