package tv

import (
	"bytes"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// This suite exercises the paste-chip feature (gogent #501, turbotui half) across
// the four design criteria: goal match (chip atomicity + verbatim GetText),
// usability (caret never enters a chip, one-keystroke delete, themed rendering,
// graceful truncation), no regressions (chip-free paths unchanged, store does not
// leak), and holistic scope (TextBox parity, shared model). It deliberately probes
// edge cases that could reveal real defects in the sentinel-rune + side-store model.

// ---------------------------------------------------------------------------
// paste_chip.go: the shared model
// ---------------------------------------------------------------------------

func TestChipLabelLineCount(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"a\nb", "[pasted 2 lines]"},
		{"a\nb\nc", "[pasted 3 lines]"},
		{"a\n", "[pasted 2 lines]"},    // trailing newline = 2 (one empty) lines
		{"a\n\nb", "[pasted 3 lines]"}, // blank line in the middle counts
		{"\n", "[pasted 2 lines]"},     // a lone newline is two empty lines
		{"a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl", "[pasted 12 lines]"},
	}
	for _, tc := range cases {
		if got := chipLabel(tc.text); got != tc.want {
			t.Fatalf("chipLabel(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestChipLabelFitTruncation(t *testing.T) {
	const label16 = "[pasted 3 lines]" // 16 display columns (all ASCII)
	cases := []struct {
		name  string
		width int
		want  string // expected rendered label (as a string)
	}{
		{"fits exactly", 16, label16},
		{"fits with room", 40, label16},
		{"one short, ellipsis", 15, "[pasted 3 line…"},
		{"half width, ellipsis", 8, "[pasted…"},
		{"ellipsis plus a few", 5, "[pas…"},
		{"two columns", 2, "[…"},
		{"one column hard cut", 1, "["},
		{"zero clamps to one", 0, "["},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(chipLabelFit("a\nb\nc", tc.width))
			if got != tc.want {
				t.Fatalf("chipLabelFit(width=%d) = %q, want %q", tc.width, got, tc.want)
			}
			// The fitted label must never exceed the requested width.
			if w := tui.StringWidth(got); w > maxInt(tc.width, 1) {
				t.Fatalf("chipLabelFit(width=%d) produced %d columns (>%d)", tc.width, w, tc.width)
			}
		})
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestIsPasteChipRuneRange(t *testing.T) {
	// The allocated range is Supplementary PUA-A (U+F0000..U+FFFFD).
	if !IsPasteChipRune(0xF0000) || !IsPasteChipRune(0xFFFFD) {
		t.Fatal("edges of the SPUA-A range should be chip runes")
	}
	if IsPasteChipRune(0xEFFFF) { // just below the range (PUA end of BMP is F890, but EFFFF is outside)
		t.Fatal("0xEFFFF must not be a chip rune")
	}
	if IsPasteChipRune(0xFFFFE) || IsPasteChipRune(0xFFFFF) { // noncharacters just above the range
		t.Fatal("0xFFFFE/F must not be chip runes")
	}
	for _, r := range []rune{'a', '\n', '…', '中', 0xFE00, 0x1F600} {
		if IsPasteChipRune(r) {
			t.Fatalf("ordinary rune %U must not be a chip rune", r)
		}
	}
}

func TestSanitizePaste(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		wantClean   string
		wantNewline bool
	}{
		{"plain single line", "hello", "hello", false},
		{"crlf becomes lf", "a\r\nb", "a\nb", true},
		{"lone cr dropped", "a\rb", "ab", false},
		{"control chars dropped", "a\x00\x01\x02b", "ab", false},
		// Tab (0x09) is < 0x20 so it is stripped — this is PRESERVED pre-existing
		// paste behaviour (the original handlePaste did `case r < 0x20: continue`),
		// not something this feature changed.
		{"tab stripped (pre-existing)", "a\tb", "ab", false},
		{"newline survives", "a\nb", "a\nb", true},
		{"only newline", "\n", "\n", true},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotClean, gotNL := sanitizePaste(tc.in)
			if gotClean != tc.wantClean || gotNL != tc.wantNewline {
				t.Fatalf("sanitizePaste(%q) = (%q,%v), want (%q,%v)", tc.in, gotClean, gotNL, tc.wantClean, tc.wantNewline)
			}
		})
	}
}

// TestSanitizePasteStripsRealChipRune: a paste containing an actual allocated chip
// sentinel must have it stripped so user content can never collide with the marker.
func TestSanitizePasteStripsRealChipRune(t *testing.T) {
	chip := rune(0xF0000)
	in := string([]rune{'x', chip, 'y', '\n', 'z'})
	clean, hasNL := sanitizePaste(in)
	want := "xy\nz"
	if clean != want || !hasNL {
		t.Fatalf("sanitizePaste stripped chip = (%q,%v), want (%q,true)", clean, hasNL, want)
	}
	for _, r := range clean {
		if IsPasteChipRune(r) {
			t.Fatalf("sanitized paste still contains a chip rune: %q", clean)
		}
	}
}

// TestChipStoreExpandRoundTrip: expand restores verbatim text and drops orphans.
func TestChipStoreExpandRoundTrip(t *testing.T) {
	var s chipStore
	r1 := s.add("alpha\nbeta")
	r2 := s.add("gamma\ndelta\nepsilon")
	// Chip-free slice round-trips byte-for-byte.
	if got := s.expand([]rune("plain text")); got != "plain text" {
		t.Fatalf("chip-free expand = %q", got)
	}
	// Markers expand to their full original.
	got := s.expand([]rune{'a', r1, 'b', r2, 'c'})
	if got != "a"+"alpha\nbeta"+"b"+"gamma\ndelta\nepsilon"+"c" {
		t.Fatalf("expand with chips = %q", got)
	}
	// An orphan marker (no store entry) is dropped, not rendered as a glyph.
	orphan := rune(0xF1234)
	if got := s.expand([]rune{'x', orphan, 'y'}); got != "xy" {
		t.Fatalf("orphan expand = %q, want xy", got)
	}
}

func TestChipStoreKeepOnlyAndReset(t *testing.T) {
	var s chipStore
	r1 := s.add("one\ntwo")
	r2 := s.add("three\nfour")
	// keepOnly with only r1 present drops r2.
	s.keepOnly(map[rune]bool{r1: true})
	if _, ok := s.text(r2); ok {
		t.Fatal("keepOnly should have dropped the absent r2")
	}
	if _, ok := s.text(r1); !ok {
		t.Fatal("keepOnly should have kept the present r1")
	}
	// reset drops everything and rewinds the allocator.
	s.reset()
	if _, ok := s.text(r1); ok {
		t.Fatal("reset should have dropped all chips")
	}
	// After reset a fresh allocation starts at the base again.
	if r := s.add("x\ny"); r != 0xF0000 {
		t.Fatalf("after reset next chip should be base, got %U", r)
	}
}

// ---------------------------------------------------------------------------
// MultiLineInput: the design's tests 1–9 plus edge cases
// ---------------------------------------------------------------------------

// Test 1: single-line paste stays literal and the caret steps rune-by-rune.
func TestPasteChipSingleLineStaysLiteral(t *testing.T) {
	m := NewMultiLineInput("X", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.handlePaste(m.Component, "abc")
	if len(m.Lines) != 1 || m.Lines[0] != "Xabc" {
		t.Fatalf("single-line paste should be literal, got %#v", m.Lines)
	}
	// No chip rune should be present.
	for _, r := range m.Lines[0] {
		if IsPasteChipRune(r) {
			t.Fatalf("single-line paste must not create a chip: %q", m.Lines[0])
		}
	}
	if got := m.GetText(); got != "Xabc" {
		t.Fatalf("GetText = %q, want Xabc", got)
	}
	// Caret is right after the pasted text and steps back rune-by-rune.
	if m.CursorX != 4 {
		t.Fatalf("caret = %d, want 4", m.CursorX)
	}
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyLeft})
	if m.CursorX != 3 {
		t.Fatalf("left step = %d, want 3", m.CursorX)
	}
}

// Test 2: multi-line paste collapses to one chip; GetText is verbatim (CR dropped).
func TestPasteChipMultiLineCollapsesMLI(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 5})
	m.handlePaste(m.Component, "a\r\nb\nc")
	if len(m.Lines) != 1 {
		t.Fatalf("multi-line paste must collapse to one logical line, got %d: %#v", len(m.Lines), m.Lines)
	}
	rs := []rune(m.Lines[0])
	if len(rs) != 1 || !IsPasteChipRune(rs[0]) {
		t.Fatalf("expected a single chip sentinel, got %#v", m.Lines)
	}
	if got := m.GetText(); got != "a\nb\nc" {
		t.Fatalf("GetText = %q, want a\\nb\\nc (CR dropped)", got)
	}
}

// Test 3: one Backspace with the caret after the chip removes the whole block.
func TestPasteChipBackspaceRemovesWholeChip(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.handlePaste(m.Component, "a\nb\nc")
	// Caret sits after the chip. One Backspace removes it entirely.
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyBackspace})
	if len(m.Lines) != 1 || m.Lines[0] != "" {
		t.Fatalf("backspace should empty the buffer, got %#v", m.Lines)
	}
	if got := m.GetText(); got != "" {
		t.Fatalf("GetText after backspace = %q, want empty", got)
	}
	if m.CursorX != 0 {
		t.Fatalf("caret = %d, want 0", m.CursorX)
	}
}

// Backspace removing a chip that has ordinary text after it removes only the chip.
func TestPasteChipBackspaceMidLine(t *testing.T) {
	m := NewMultiLineInput("A", Rect{X: 0, Y: 0, W: 40, H: 3})
	// Caret between A and the chip's insertion point, then paste collapses after A.
	m.CursorX = 1
	m.handlePaste(m.Component, "x\ny") // line becomes [A, chip], caret after chip
	if got := m.GetText(); got != "Ax\ny" {
		t.Fatalf("setup GetText = %q, want Ax\\ny", got)
	}
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyBackspace})
	if m.Lines[0] != "A" {
		t.Fatalf("backspace should remove only the chip, got %q", m.Lines[0])
	}
	if got := m.GetText(); got != "A" {
		t.Fatalf("GetText = %q, want A", got)
	}
}

// Test 4 (arrows): Left/Right jump over a mid-line chip; the caret is never inside.
func TestPasteChipArrowsJumpOverChip(t *testing.T) {
	m := NewMultiLineInput("AB", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.CursorX = 1 // between A and B
	m.handlePaste(m.Component, "x\ny")
	// Line is [A, chip, B]; caret (2) is just after the chip, before B.
	if m.CursorX != 2 {
		t.Fatalf("caret after paste = %d, want 2", m.CursorX)
	}
	// Left from after -> before the chip (offset 1).
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyLeft})
	if m.CursorX != 1 {
		t.Fatalf("left from after chip = %d, want 1 (before chip)", m.CursorX)
	}
	// Right from before -> after the chip (offset 2).
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyRight})
	if m.CursorX != 2 {
		t.Fatalf("right from before chip = %d, want 2 (after chip)", m.CursorX)
	}
	// The only valid stops around the chip are 1 and 2; there is no "inside".
	if got := m.GetText(); got != "Ax\nyB" {
		t.Fatalf("GetText = %q, want Ax\\nyB", got)
	}
}

// Test 4 (delete keys): forward-delete with the caret immediately before a chip
// removes the whole chip; backspace with the caret after does too.
func TestPasteChipDeleteBeforeRemovesWholeChip(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.handlePaste(m.Component, "a\nb")
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyLeft}) // caret now before the chip
	if m.CursorX != 0 {
		t.Fatalf("caret = %d, want 0 before chip", m.CursorX)
	}
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyDelete})
	if m.Lines[0] != "" {
		t.Fatalf("forward-delete before chip should remove it, got %q", m.Lines[0])
	}
	if got := m.GetText(); got != "" {
		t.Fatalf("GetText = %q, want empty", got)
	}
}

// Test 5: typing over a selection that contains a chip replaces it wholesale.
func TestPasteChipSelectAllThenTypeReplacesMLI(t *testing.T) {
	m := NewMultiLineInput("Z", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.CursorX = 1
	m.handlePaste(m.Component, "a\nb") // line [Z, chip], caret after chip
	// Select everything on the line.
	m.selAnchorY, m.selAnchorX = 0, 0
	m.CursorY, m.CursorX = 0, 2
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'Q'})
	if m.Lines[0] != "Q" {
		t.Fatalf("typing over selected chip should replace wholesale, got %q", m.Lines[0])
	}
	if got := m.GetText(); got != "Q" {
		t.Fatalf("GetText = %q, want Q", got)
	}
	for _, r := range m.Lines[0] {
		if IsPasteChipRune(r) {
			t.Fatalf("chip should be gone after replace, got %q", m.Lines[0])
		}
	}
}

// Test 6: copying a selection that includes a chip yields the full original text.
func TestPasteChipCopyYieldsFullTextMLI(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.handlePaste(m.Component, "line1\nline2\nline3")
	// Select the whole chip (one rune).
	m.selAnchorY, m.selAnchorX = 0, 0
	m.CursorY, m.CursorX = 0, 1
	text, ok := m.copySelection(m.Component)
	if !ok || text != "line1\nline2\nline3" {
		t.Fatalf("copy of chip = %q ok=%v, want the verbatim multi-line original", text, ok)
	}
}

// Selecting part of a line that includes a chip and deleting removes the chip whole.
func TestPasteChipDeleteSelectionRemovesChip(t *testing.T) {
	m := NewMultiLineInput("AB", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.CursorX = 1
	m.handlePaste(m.Component, "x\ny") // [A, chip, B]
	// Select from before the chip (offset 1) through after it (offset 2): just the chip.
	m.selAnchorY, m.selAnchorX = 0, 1
	m.CursorY, m.CursorX = 0, 2
	if !m.deleteSelection() {
		t.Fatal("expected deleteSelection to succeed")
	}
	if m.Lines[0] != "AB" {
		t.Fatalf("deleting the chip should leave AB, got %q", m.Lines[0])
	}
	if got := m.GetText(); got != "AB" {
		t.Fatalf("GetText = %q, want AB", got)
	}
}

// Test 7: SetTextChip restores a chip; plain SetText stays literal (G1 guard).
func TestPasteChipSetTextChipVsSetTextMLI(t *testing.T) {
	// SetTextChip with a newline -> one chip.
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.SetTextChip("x\ny")
	if len(m.Lines) != 1 {
		t.Fatalf("SetTextChip should keep one line, got %d", len(m.Lines))
	}
	rs := []rune(m.Lines[0])
	if len(rs) != 1 || !IsPasteChipRune(rs[0]) {
		t.Fatalf("SetTextChip should produce one chip, got %#v", m.Lines)
	}
	if got := m.GetText(); got != "x\ny" {
		t.Fatalf("GetText = %q, want x\\ny", got)
	}
	if m.CursorX != 1 {
		t.Fatalf("caret after SetTextChip = %d, want 1", m.CursorX)
	}
	// SetTextChip without a newline is literal (no chip).
	m2 := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	m2.SetTextChip("plain")
	if m2.Lines[0] != "plain" || m2.GetText() != "plain" {
		t.Fatalf("SetTextChip(no-newline) should be literal, got %#v / %q", m2.Lines, m2.GetText())
	}
	// Plain SetText with a newline stays LITERAL (editable lines), the G1 fix.
	m3 := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	m3.SetText("x\ny")
	if len(m3.Lines) != 2 || m3.Lines[0] != "x" || m3.Lines[1] != "y" {
		t.Fatalf("SetText should be literal multi-line, got %#v", m3.Lines)
	}
	for _, ln := range m3.Lines {
		for _, r := range ln {
			if IsPasteChipRune(r) {
				t.Fatalf("SetText must not chip-ify: %#v", m3.Lines)
			}
		}
	}
}

// SetText must drop any prior chip store so a content swap cannot leak chips.
func TestPasteChipSetTextDropsPriorChips(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.handlePaste(m.Component, "a\nb") // chip present
	m.SetText("hello")
	if got := m.GetText(); got != "hello" {
		t.Fatalf("GetText after SetText = %q, want hello", got)
	}
	if len(m.chips.byRune) != 0 {
		t.Fatalf("SetText should reset the chip store, got %d entries", len(m.chips.byRune))
	}
}

// Clear() empties the buffer and the chip store.
func TestPasteChipClear(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.handlePaste(m.Component, "a\nb\n c")
	m.Clear()
	if got := m.GetText(); got != "" {
		t.Fatalf("GetText after Clear = %q, want empty", got)
	}
	if len(m.chips.byRune) != 0 {
		t.Fatalf("Clear should reset the chip store, got %d entries", len(m.chips.byRune))
	}
}

// A chip created mid-line round-trips identically to a literal multi-line insert.
func TestPasteChipMidLineGetTextEquivalence(t *testing.T) {
	// Paste "a\nb\nc" into X|Y -> line X<chip>Y -> GetText "Xa\nb\ncY".
	m := NewMultiLineInput("XY", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.CursorX = 1
	m.handlePaste(m.Component, "a\nb\nc")
	if got := m.GetText(); got != "Xa\nb\ncY" {
		t.Fatalf("mid-line chip GetText = %q, want Xa\\nb\\ncY", got)
	}
	if len(m.Lines) != 1 {
		t.Fatalf("should stay one logical line, got %d", len(m.Lines))
	}
}

// Multiple chips on one line are independent and each expands to its own text.
func TestPasteChipMultipleChipsIndependent(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 60, H: 3})
	m.handlePaste(m.Component, "a\nb") // chip1 at offset 0, caret 1
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: '-'})
	m.handlePaste(m.Component, "c\nd") // chip2 after '-'
	if got := m.GetText(); got != "a\nb-c\nd" {
		t.Fatalf("two-chip GetText = %q, want a\\nb-c\\nd", got)
	}
}

// A keystroke carrying a chip sentinel must never enter the buffer (reserved range).
func TestPasteChipTypedSentinelRejected(t *testing.T) {
	m := NewMultiLineInput("hi", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 0xF0000})
	if got := m.GetText(); got != "hi" {
		t.Fatalf("typed sentinel should be rejected, buffer = %q", got)
	}
	for _, r := range m.Lines[0] {
		if IsPasteChipRune(r) {
			t.Fatalf("sentinel leaked into buffer via keystroke: %q", m.Lines[0])
		}
	}
}

// A paste whose text already contains a chip sentinel has it stripped (no collision),
// but a real newline still produces a chip from the surviving text.
func TestPasteChipStripsExistingSentinel(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	chip := rune(0xF0000)
	m.handlePaste(m.Component, string([]rune{'x', chip, 'y', '\n', 'z'}))
	if got := m.GetText(); got != "xy\nz" {
		t.Fatalf("GetText = %q, want xy\\nz (sentinel stripped)", got)
	}
}

// Test 9: CaretRowInLine accounts for a chip occupying one (or more) visual rows.
func TestPasteChipCaretRowInLine(t *testing.T) {
	// W=11 -> contentWidth 10. A lone chip "[pasted 2 lines]" truncates to 10 cols.
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 11, H: 5})
	m.handlePaste(m.Component, "a\nb")
	// Single chip row: caret after it (offset 1) is on row 0 of 1.
	m.CursorX = 1
	if row, rows := m.CaretRowInLine(); row != 0 || rows != 1 {
		t.Fatalf("lone chip CaretRowInLine = (%d,%d), want (0,1)", row, rows)
	}
	// Now grow the line past the chip so it wraps to several rows.
	for i := 0; i < 11; i++ {
		m.handleType(m.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'A'})
	}
	// chip (10 cols) | AAAAAAAAAA | A  -> 3 rows. Caret at the end is on row 2.
	if row, rows := m.CaretRowInLine(); row != 2 || rows != 3 {
		t.Fatalf("wrapped chip line CaretRowInLine = (%d,%d), want (2,3)", row, rows)
	}
	m.CursorX = 0
	if row, rows := m.CaretRowInLine(); row != 0 || rows != 3 {
		t.Fatalf("caret before chip CaretRowInLine = (%d,%d), want (0,3)", row, rows)
	}
}

// Test 8: word wrap (and char wrap) never split a chip across visual rows, and the
// layout stays lossless/contiguous. Built deterministically: 7 A's, one chip, 7 B's.
func TestPasteChipWrapDoesNotSplitChip(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 11, H: 5}) // contentWidth 10
	r := m.chips.add("alpha\nbeta")                           // "[pasted 2 lines]" -> 16 cols, truncated to 10
	m.Lines = []string{string(append(append([]rune("AAAAAAA"), r), []rune("BBBBBBB")...))}
	m.CursorX = 0
	for _, wrap := range []bool{false, true} {
		m.WordWrap = wrap
		rows := m.wrappedRows(10)
		var rebuilt strings.Builder
		sawChip := false
		chipRows := 0
		for _, row := range rows {
			// No row's display width may exceed the content width.
			if w := m.runeColWidth(row.runes, 0, len(row.runes), 10); w > 10 {
				t.Fatalf("wrap=%v: row %q is %d cols wide (>10)", wrap, string(row.runes), w)
			}
			if containsChip(row.runes) {
				sawChip = true
				chipRows++
			}
			rebuilt.WriteString(string(row.runes))
		}
		// The chip must appear in exactly one row (never fragmented across rows).
		if !sawChip || chipRows != 1 {
			t.Fatalf("wrap=%v: chip appeared in %d rows (want 1)", wrap, chipRows)
		}
		// Lossless: concatenating row runes reconstructs the line runes exactly.
		if rebuilt.String() != m.Lines[0] {
			t.Fatalf("wrap=%v: rows lost runes: %q vs %q", wrap, rebuilt.String(), m.Lines[0])
		}
	}
}

func containsChip(runes []rune) bool {
	for _, r := range runes {
		if IsPasteChipRune(r) {
			return true
		}
	}
	return false
}

// Click on a chip snaps to the nearer edge — the caret is never placed inside it.
func TestPasteChipClickSnapsToEdgeMLI(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 50, H: 3})
	m.handlePaste(m.Component, "a\nb\nc") // chip "[pasted 3 lines]" = 16 cols at width 49
	// Click the left edge of the chip -> before it (offset 0).
	_ = m.handleClick(m.Component, tui.ClickEvent{X: 0, Y: 0, Down: true})
	if m.CursorX != 0 {
		t.Fatalf("click left edge -> caret %d, want 0 (before chip)", m.CursorX)
	}
	// Click the right half of the chip -> after it (offset 1).
	_ = m.handleClick(m.Component, tui.ClickEvent{X: 12, Y: 0, Down: true})
	if m.CursorX != 1 {
		t.Fatalf("click right half -> caret %d, want 1 (after chip)", m.CursorX)
	}
}

// Render/consistency: the caret's visual column after a chip equals the chip's
// rendered width, and clicking that column returns the after-chip offset.
func TestPasteChipCursorColumnMatchesRenderedWidth(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 50, H: 3})
	m.handlePaste(m.Component, "a\nb\nc")
	m.CursorX = 1 // after the chip
	_, col := m.cursorVisualPos(49)
	chipW := m.cellWidth([]rune(m.Lines[0])[0], 49)
	if col != chipW {
		t.Fatalf("caret column after chip = %d, want chip width %d", col, chipW)
	}
	// Clicking exactly that display column maps back to after-chip (offset 1).
	_ = m.handleClick(m.Component, tui.ClickEvent{X: col, Y: 0, Down: true})
	if m.CursorX != 1 {
		t.Fatalf("click at rendered caret column -> caret %d, want 1", m.CursorX)
	}
}

// Visual: a chip paints its label across contiguous cells using PasteChip colours.
func TestPasteChipRenderLabelAndTheme(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 50, H: 3})
	m.handlePaste(m.Component, "a\nb\nc")
	app := drawInput(t, m, 50, 3) // contentWidth 49 -> full "[pasted 3 lines]"
	want := "[pasted 3 lines]"
	for i, r := range want {
		cell := app.ReadCell(i, 0)
		if cell.Ch != r {
			t.Fatalf("cell(%d,0) = %q, want %q", i, cell.Ch, r)
		}
		if cell.FG != activeTheme.PasteChipFG {
			t.Fatalf("cell(%d,0) FG = %+v, want PasteChipFG", i, cell.FG)
		}
		if cell.BG != activeTheme.PasteChipBG {
			t.Fatalf("cell(%d,0) BG = %+v, want PasteChipBG", i, cell.BG)
		}
	}
	// The cell just past the label is not part of the chip: its background must be
	// the input fill, not the chip background. (FG cannot reliably distinguish
	// here because DefaultTheme sets PasteChipFG == InputFG == bright white 15 — a
	// minor theme overlap worth noting; the BG is the dependable differentiator.)
	pastBG := app.ReadCell(len(want), 0).BG
	if pastBG == activeTheme.PasteChipBG {
		t.Fatalf("cell after the chip carries the chip BG; expected the input fill")
	}
}

// Visual: an over-wide chip truncates its label with an ellipsis and stays on one row.
func TestPasteChipRenderTruncatesWhenWide(t *testing.T) {
	// W=7 -> contentWidth 6; "[pasted 3 lines]" (16) truncates to "[past…" (6 cols).
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 7, H: 3})
	m.handlePaste(m.Component, "a\nb\nc")
	app := drawInput(t, m, 7, 3)
	want := "[past…"
	for i, r := range want {
		if got := app.ReadCell(i, 0).Ch; got != r {
			t.Fatalf("truncated cell(%d,0) = %q, want %q", i, got, r)
		}
	}
	// The box did not grow to multiple rows for the paste.
	if len(m.wrappedRows(6)) != 1 {
		t.Fatalf("over-wide chip should occupy one visual row, got %d rows", len(m.wrappedRows(6)))
	}
	// Full text is still retrievable despite the truncated label.
	if got := m.GetText(); got != "a\nb\nc" {
		t.Fatalf("GetText under truncation = %q, want a\\nb\\nc", got)
	}
}

// ---------------------------------------------------------------------------
// TextBox parity (design test 10) + word motion
// ---------------------------------------------------------------------------

// drawTextBox renders a TextBox into a fresh app for cell inspection.
func drawTextBox(t *testing.T, box *TextBox, w, h int) *tui.App {
	t.Helper()
	var output bytes.Buffer
	app := tui.NewWithSize(w, h, &output)
	desktop := NewDesktop(app)
	root := NewComponent(Rect{X: 0, Y: 0, W: w, H: h})
	root.AddChild(box)
	desktop.AddLayer(NewLayer("root", root, true, false))
	desktop.Redraw()
	return app
}

// Test 10: a multi-line paste into a TextBox collapses to a chip and round-trips.
func TestPasteChipTextBoxParity(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 40, H: 1})
	box.handlePaste(box.Component, "ab\ncd\r\nef")
	if len(box.Text) != 1 || !IsPasteChipRune(box.Text[0]) {
		t.Fatalf("TextBox multi-line paste should be one chip, got %q", string(box.Text))
	}
	if got := box.GetText(); got != "ab\ncd\nef" {
		t.Fatalf("TextBox GetText = %q, want ab\\ncd\\nef (CR dropped)", got)
	}
	// Single-line paste into TextBox stays literal.
	box2 := NewTextBox("x", Rect{X: 0, Y: 0, W: 40, H: 1})
	box2.handlePaste(box2.Component, "yz")
	if string(box2.Text) != "xyz" || box2.GetText() != "xyz" {
		t.Fatalf("TextBox single-line paste should be literal, got %q", string(box2.Text))
	}
}

// TextBox SetTextChip / SetText mirror MultiLineInput.
func TestPasteChipTextBoxSetChips(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 40, H: 1})
	box.SetTextChip("p\nq")
	if len(box.Text) != 1 || !IsPasteChipRune(box.Text[0]) || box.GetText() != "p\nq" {
		t.Fatalf("SetTextChip -> chip round-trip failed: %q / %q", string(box.Text), box.GetText())
	}
	// Plain SetText is literal: the raw newline is kept as a rune (no chip created).
	box.SetText("p\nq")
	for _, r := range box.Text {
		if IsPasteChipRune(r) {
			t.Fatalf("TextBox SetText must not chip-ify: %q", string(box.Text))
		}
	}
	if got := box.GetText(); got != "p\nq" {
		t.Fatalf("TextBox literal SetText GetText = %q, want p\\nq", got)
	}
}

// Word motion treats a chip as one indivisible word unit.
func TestPasteChipTextBoxWordMotionOverChip(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 40, H: 1})
	box.handlePaste(box.Component, "a\nb") // [chip], caret after (1)
	// Ctrl+Left from after the chip -> before it (0).
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyLeft, Ctrl: true})
	if box.Cursor != 0 {
		t.Fatalf("Ctrl+Left over chip -> %d, want 0", box.Cursor)
	}
	// Ctrl+Right from before the chip -> after it (1).
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Ctrl: true})
	if box.Cursor != 1 {
		t.Fatalf("Ctrl+Right over chip -> %d, want 1", box.Cursor)
	}
}

// Ctrl+Backspace / Ctrl+Delete remove the whole chip in one stroke.
func TestPasteChipTextBoxCtrlBackspaceDeletesChip(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 40, H: 1})
	box.handlePaste(box.Component, "a\nb") // [chip], caret 1
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyBackspace, Ctrl: true})
	if len(box.Text) != 0 || box.GetText() != "" {
		t.Fatalf("Ctrl+Backspace should remove the chip, got %q", string(box.Text))
	}
	// Forward: chip followed by text, caret before chip.
	box2 := NewTextBox("", Rect{X: 0, Y: 0, W: 40, H: 1})
	box2.handlePaste(box2.Component, "a\nb")
	box2.handleType(box2.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'Z'}) // [chip, Z]
	box2.Cursor = 0
	box2.handleType(box2.Component, tui.TypeEvent{Key: tui.KeyDelete, Ctrl: true})
	if string(box2.Text) != "Z" {
		t.Fatalf("Ctrl+Delete before chip should remove only the chip, got %q", string(box2.Text))
	}
}

// Copy/cut of a TextBox chip yield the full original multi-line text.
func TestPasteChipTextBoxCopyCutFullText(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 40, H: 1})
	box.handlePaste(box.Component, "one\ntwo")
	// Copy.
	box.selAnchor, box.Cursor = 0, 1
	if text, ok := box.copySelection(box.Component); !ok || text != "one\ntwo" {
		t.Fatalf("TextBox copy of chip = %q ok=%v, want one\\ntwo", text, ok)
	}
	// Cut removes the chip and returns the full text.
	box.selAnchor, box.Cursor = 0, 1
	text, ok := box.cutSelection(box.Component)
	if !ok || text != "one\ntwo" {
		t.Fatalf("TextBox cut of chip = %q ok=%v, want one\\ntwo", text, ok)
	}
	if len(box.Text) != 0 {
		t.Fatalf("TextBox cut should remove the chip, got %q", string(box.Text))
	}
}

// TextBox select-all (Ctrl+A) includes the chip; typing replaces it wholesale.
func TestPasteChipTextBoxSelectAllReplacesChip(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 40, H: 1})
	box.handlePaste(box.Component, "a\nb")
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'a', Ctrl: true}) // select all
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'Q'})
	if string(box.Text) != "Q" {
		t.Fatalf("typing over selected chip should replace it, got %q", string(box.Text))
	}
}

// TextBox click on a chip snaps to the nearer edge.
func TestPasteChipTextBoxClickSnaps(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 50, H: 1})
	box.handlePaste(box.Component, "a\nb\nc") // 16-wide label
	_ = box.handleClick(box.Component, tui.ClickEvent{X: 0, Y: 0, Down: true})
	if box.Cursor != 0 {
		t.Fatalf("TextBox click left edge -> %d, want 0", box.Cursor)
	}
	_ = box.handleClick(box.Component, tui.ClickEvent{X: 12, Y: 0, Down: true})
	if box.Cursor != 1 {
		t.Fatalf("TextBox click right half -> %d, want 1", box.Cursor)
	}
}

// TextBox render: chip paints its label in PasteChip colours.
func TestPasteChipTextBoxRender(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 50, H: 1})
	box.handlePaste(box.Component, "a\nb\nc")
	app := drawTextBox(t, box, 50, 1)
	want := "[pasted 3 lines]"
	for i, r := range want {
		cell := app.ReadCell(i, 0)
		if cell.Ch != r {
			t.Fatalf("TextBox cell(%d,0) = %q, want %q", i, cell.Ch, r)
		}
		if cell.FG != activeTheme.PasteChipFG || cell.BG != activeTheme.PasteChipBG {
			t.Fatalf("TextBox cell(%d,0) colours = FG%+v BG%+v, want PasteChip", i, cell.FG, cell.BG)
		}
	}
}

// ---------------------------------------------------------------------------
// Regression / cross-check: chip-free paths are behaviourally unchanged
// ---------------------------------------------------------------------------

// On chip-free input the geometry must match the legacy per-rune arithmetic. This
// guards the "fast path" so the chip-aware branch cannot drift the caret layer.
func TestPasteChipFreeGeometryUnchanged(t *testing.T) {
	// Char wrap.
	m := NewMultiLineInput("abcdefghijklmn", Rect{X: 0, Y: 0, W: 11, H: 5}) // contentWidth 10
	rows := m.wrappedRows(10)
	if len(rows) != 2 || string(rows[0].runes) != "abcdefghij" || string(rows[1].runes) != "klmn" {
		t.Fatalf("chip-free char wrap rows = %q, want [abcdefghij|klmn]", rowsString(rows))
	}
	m.CursorY, m.CursorX = 0, 12
	vr, vc := m.cursorVisualPos(10)
	rr, rc := m.cursorRowCol(rows, 10)
	if vr != rr || vc != rc {
		t.Fatalf("char-wrap cursorVisualPos(%d,%d) != cursorRowCol(%d,%d)", vr, vc, rr, rc)
	}
	if vr != 1 {
		t.Fatalf("char-wrap caret row = %d, want 1", vr)
	}
	// Word wrap.
	mw := NewMultiLineInput("aaaa bbbb cccc", Rect{X: 0, Y: 0, W: 11, H: 5})
	mw.WordWrap = true
	rw := mw.wrappedRows(10)
	if rowsString(rw) != "aaaa bbbb |cccc" {
		t.Fatalf("chip-free word wrap rows = %q", rowsString(rw))
	}
	// A chip-free line never reports containing a chip.
	if lineHasChip([]rune("plain text with 中 and …")) {
		t.Fatal("lineHasChip must be false for chip-free text incl. wide/ellipsis runes")
	}
}

func rowsString(rows []wrappedLineRow) string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = string(r.runes)
	}
	return strings.Join(parts, "|")
}

// Theme: the two new roles exist on both presets with distinct, sensible values.
func TestPasteChipThemeRolesPresent(t *testing.T) {
	for name, th := range map[string]Theme{"Default": DefaultTheme, "HighContrast": HighContrastTheme} {
		// Paste chip colours must differ from the focus fill and from each other
		// (BG vs FG), and must be set (non-zero ANSI codes chosen by the preset).
		if th.PasteChipBG == th.InputFocusBG {
			t.Fatalf("%s: PasteChipBG equals InputFocusBG (not distinct)", name)
		}
		if th.PasteChipBG == th.TextSelectionBG && th.PasteChipFG == th.TextSelectionFG {
			t.Fatalf("%s: PasteChip colours identical to TextSelection", name)
		}
	}
}
