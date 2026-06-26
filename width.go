package tui

import "unicode"

// RuneWidth reports how many terminal columns r occupies when rendered:
//
//	0 — combining marks, zero-width and format characters (they attach to the
//	    preceding base glyph and do not advance the cursor),
//	2 — East-Asian wide / fullwidth glyphs and most emoji,
//	1 — everything else.
//
// It is the single source of truth the renderer and every text writer use to
// keep the framework's cell model in step with the terminal cursor. Without it
// a CJK ideograph or emoji (2 columns wide) stored in a single cell would shift
// the remainder of the line.
func RuneWidth(r rune) int {
	if r == 0 {
		return 0
	}
	// C0/C1 control characters are not printable text; treat them as a single
	// placeholder column rather than attaching them to the previous glyph.
	if r < 0x20 || (r >= 0x7f && r < 0xa0) {
		return 1
	}
	if isZeroWidth(r) {
		return 0
	}
	if isWide(r) {
		return 2
	}
	return 1
}

// StringWidth returns the total number of terminal columns s occupies, summing
// RuneWidth across its runes (combining marks contribute 0).
func StringWidth(s string) int {
	total := 0
	for _, r := range s {
		total += RuneWidth(r)
	}
	return total
}

// isZeroWidth reports whether r is a combining mark or other zero-advance code
// point that should be folded into the preceding cell instead of consuming one.
func isZeroWidth(r rune) bool {
	switch r {
	case 0x200B, 0x200C, 0x200D, 0xFEFF: // ZWSP, ZWNJ, ZWJ, BOM/ZWNBSP
		return true
	}
	if r >= 0xFE00 && r <= 0xFE0F { // variation selectors
		return true
	}
	// Nonspacing (Mn) and enclosing (Me) combining marks, plus format (Cf)
	// characters, all render without advancing the cursor.
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf)
}

// isWide reports whether r is an East-Asian wide / fullwidth glyph (two
// columns). The ranges below cover CJK, Kana, Hangul, fullwidth forms and the
// common emoji blocks; they are looked up by binary search.
func isWide(r rune) bool {
	lo, hi := 0, len(wideRanges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		rng := wideRanges[mid]
		switch {
		case r < rng[0]:
			hi = mid - 1
		case r > rng[1]:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// wideRanges lists the inclusive code-point ranges of double-width glyphs, kept
// sorted and non-overlapping for binary search.
var wideRanges = [][2]rune{
	{0x1100, 0x115F}, // Hangul Jamo
	{0x231A, 0x231B}, // watch, hourglass
	{0x2329, 0x232A}, // angle brackets
	// Emoji with default emoji (width-2) presentation scattered through the
	// symbol blocks. Only Emoji_Presentation=Yes code points are listed; text-
	// presentation symbols in the same blocks (e.g. ☢ U+2622, ✓ U+2713, arrows)
	// stay width 1. Regional indicators (U+1F1E6–U+1F1FF) are deliberately NOT
	// here: with no grapheme clustering a 2-rune flag only measures correctly
	// (2 cells) while each indicator is width 1.
	{0x23E9, 0x23EC},   // fast-forward / rewind controls
	{0x23F0, 0x23F0},   // alarm clock
	{0x23F3, 0x23F3},   // hourglass not done
	{0x25FD, 0x25FE},   // medium-small squares
	{0x2614, 0x2615},   // umbrella, hot beverage
	{0x2648, 0x2653},   // zodiac signs
	{0x267F, 0x267F},   // wheelchair
	{0x2693, 0x2693},   // anchor
	{0x26A1, 0x26A1},   // high voltage
	{0x26AA, 0x26AB},   // medium circles
	{0x26BD, 0x26BE},   // soccer ball, baseball
	{0x26C4, 0x26C5},   // snowman, sun behind cloud
	{0x26CE, 0x26CE},   // ophiuchus
	{0x26D4, 0x26D4},   // no entry
	{0x26EA, 0x26EA},   // church
	{0x26F2, 0x26F3},   // fountain, golf
	{0x26F5, 0x26F5},   // sailboat
	{0x26FA, 0x26FA},   // tent
	{0x26FD, 0x26FD},   // fuel pump
	{0x2705, 0x2705},   // white heavy check mark ✅
	{0x270A, 0x270B},   // raised fist, raised hand
	{0x2728, 0x2728},   // sparkles
	{0x274C, 0x274C},   // cross mark
	{0x274E, 0x274E},   // negative squared cross mark
	{0x2753, 0x2755},   // question/exclamation ornaments
	{0x2757, 0x2757},   // heavy exclamation mark
	{0x2795, 0x2797},   // heavy plus / minus / division
	{0x27B0, 0x27B0},   // curly loop
	{0x27BF, 0x27BF},   // double curly loop
	{0x2B1B, 0x2B1C},   // large black/white squares
	{0x2B50, 0x2B50},   // star
	{0x2B55, 0x2B55},   // heavy large circle
	{0x2E80, 0x303E},   // CJK radicals, Kangxi, symbols
	{0x3041, 0x33FF},   // Hiragana, Katakana, CJK symbols & punctuation
	{0x3400, 0x4DBF},   // CJK Extension A
	{0x4E00, 0x9FFF},   // CJK Unified Ideographs
	{0xA000, 0xA4CF},   // Yi syllables / radicals
	{0xAC00, 0xD7A3},   // Hangul syllables
	{0xF900, 0xFAFF},   // CJK compatibility ideographs
	{0xFE10, 0xFE19},   // vertical forms
	{0xFE30, 0xFE6F},   // CJK compatibility / small forms
	{0xFF00, 0xFF60},   // fullwidth forms
	{0xFFE0, 0xFFE6},   // fullwidth signs
	{0x1F004, 0x1F004}, // mahjong red dragon
	{0x1F0CF, 0x1F0CF}, // playing card black joker
	{0x1F18E, 0x1F18E}, // negative squared AB
	{0x1F191, 0x1F19A}, // squared symbols
	{0x1F200, 0x1F2FF}, // enclosed ideographic supplement
	{0x1F300, 0x1F64F}, // misc symbols, pictographs & emoticons
	{0x1F680, 0x1F6FF}, // transport & map symbols
	{0x1F7E0, 0x1F7EB}, // colored circles & squares
	{0x1F7F0, 0x1F7F0}, // heavy equals sign
	{0x1F900, 0x1F9FF}, // supplemental symbols & pictographs
	{0x1FA70, 0x1FAFF}, // symbols & pictographs extended-A
	{0x20000, 0x2FFFD}, // CJK Extensions B–F
	{0x30000, 0x3FFFD}, // CJK Extension G
}
