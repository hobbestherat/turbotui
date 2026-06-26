package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestRuneWidth(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want int
	}{
		{"ascii", 'a', 1},
		{"space", ' ', 1},
		{"latin1", 'é', 1},
		{"box drawing", '│', 1},
		{"cjk ideograph", '世', 2},
		{"hiragana", 'あ', 2},
		{"hangul", '한', 2},
		{"fullwidth A", '！', 2},
		{"emoji", '😀', 2},
		{"rocket emoji", '🚀', 2},
		// Issue #470: BMP emoji-presentation symbols that previously measured as
		// width 1, leaving an uncleared continuation cell ("dirt") on scroll.
		{"white heavy check mark", '✅', 2},
		{"cross mark", '❌', 2},
		{"sparkles", '✨', 2},
		{"star", '⭐', 2},
		{"high voltage", '⚡', 2},
		{"colored circle", '🟢', 2},
		// Text-presentation symbols in the same blocks must stay width 1.
		{"text check (not emoji)", '✓', 1},
		{"radioactive text symbol", '☢', 1},
		{"combining acute", '́', 0},
		{"zero width joiner", '‍', 0},
		{"zero width space", '​', 0},
		{"variation selector", '️', 0},
		{"control tab treated as one", '\t', 1},
		{"nul", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RuneWidth(tc.r); got != tc.want {
				t.Fatalf("RuneWidth(%q) = %d, want %d", tc.r, got, tc.want)
			}
		})
	}
}

func TestStringWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"世界", 4},
		{"a世b", 4},
		{"é", 1},  // base + combining accent
		{"👍‍👍", 4}, // ZWJ between two wide emoji
	}
	for _, tc := range cases {
		if got := StringWidth(tc.in); got != tc.want {
			t.Fatalf("StringWidth(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestWriteStringWideGlyphAdvancesTwoColumns(t *testing.T) {
	app := NewWithSize(6, 1, &bytes.Buffer{})
	style := Cell{FG: ANSIColor(15), BG: ANSIColor(0)}
	app.WriteString(0, 0, "世a", style)

	if got := app.back.get(0, 0).Ch; got != '世' {
		t.Fatalf("cell 0 = %q, want 世", got)
	}
	if cont := app.back.get(1, 0); !cont.cont {
		t.Fatalf("cell 1 should be a wide continuation, got %#v", cont)
	}
	// The 'a' must land at column 2, not column 1, so it is not swallowed by the
	// wide glyph.
	if got := app.back.get(2, 0).Ch; got != 'a' {
		t.Fatalf("cell 2 = %q, want a", got)
	}
}

func TestApplySkipsContinuationAndIsIdempotent(t *testing.T) {
	var out bytes.Buffer
	app := NewWithSize(6, 1, &out)
	app.Clear(DefaultCell())
	app.WriteString(0, 0, "世a", Cell{FG: ANSIColor(15), BG: ANSIColor(0)})

	if err := app.Apply(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	frame := out.String()
	if strings.Count(frame, "世") != 1 {
		t.Fatalf("wide glyph should be emitted exactly once, got %q", frame)
	}
	if !strings.Contains(frame, "a") {
		t.Fatalf("expected trailing 'a' in frame %q", frame)
	}

	// A continuation cell must be synced into the front buffer; a redraw of the
	// identical screen produces no output.
	out.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected idempotent redraw, got %q", out.String())
	}
}

func TestWriteCombiningMarkFoldsIntoBase(t *testing.T) {
	var out bytes.Buffer
	app := NewWithSize(4, 1, &out)
	app.Clear(DefaultCell())
	app.WriteString(0, 0, "éx", Cell{FG: ANSIColor(15), BG: ANSIColor(0)})

	base := app.back.get(0, 0)
	if base.Ch != 'e' || base.Combining != "́" {
		t.Fatalf("expected 'e' + combining accent, got Ch=%q Combining=%q", base.Ch, base.Combining)
	}
	// The accent must not consume a cell: 'x' stays at column 1.
	if got := app.back.get(1, 0).Ch; got != 'x' {
		t.Fatalf("cell 1 = %q, want x", got)
	}

	if err := app.Apply(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(out.String(), "é") {
		t.Fatalf("expected base+combining emitted together, got %q", out.String())
	}
}

func TestOverwritingWideGlyphClearsOrphanHalf(t *testing.T) {
	app := NewWithSize(4, 1, &bytes.Buffer{})
	style := Cell{FG: ANSIColor(15), BG: ANSIColor(0)}

	// Overwrite the LEFT half: the continuation column must be blanked.
	app.WriteString(0, 0, "世", style)
	app.WriteCell(0, 0, Cell{Ch: 'a', FG: ANSIColor(15), BG: ANSIColor(0)})
	if c := app.back.get(1, 0); c.cont || c.Ch != ' ' {
		t.Fatalf("orphaned continuation not cleared: %#v", c)
	}

	// Overwrite the RIGHT (continuation) half: the wide base must be blanked.
	app.WriteString(0, 0, "世", style)
	app.WriteCell(1, 0, Cell{Ch: 'b', FG: ANSIColor(15), BG: ANSIColor(0)})
	if c := app.back.get(0, 0); c.Ch != ' ' {
		t.Fatalf("orphaned wide base not cleared: %#v", c)
	}
	if c := app.back.get(1, 0); c.Ch != 'b' {
		t.Fatalf("cell 1 = %q, want b", c.Ch)
	}
}

func TestWideGlyphAtRightEdgeRendersBlank(t *testing.T) {
	app := NewWithSize(3, 1, &bytes.Buffer{})
	// Column 2 is the last column; a wide glyph cannot fit without wrapping.
	app.WriteString(2, 0, "世", Cell{FG: ANSIColor(15), BG: ANSIColor(0)})
	if got := app.back.get(2, 0).Ch; got != ' ' {
		t.Fatalf("expected blank at clipped wide edge, got %q", got)
	}
}

func TestWriteWrappedTextCountsWideWidth(t *testing.T) {
	app := NewWithSize(20, 5, &bytes.Buffer{})
	style := Cell{FG: ANSIColor(15), BG: ANSIColor(0)}
	// Four wide glyphs are 8 columns; with width 5 they must wrap.
	lines := app.WriteWrappedText(0, 0, 5, "世界 世界", style)
	if lines < 2 {
		t.Fatalf("expected wide text to wrap, got %d line(s)", lines)
	}
	// The first wide glyph sits at column 0 with a continuation at column 1.
	if app.back.get(0, 0).Ch != '世' || !app.back.get(1, 0).cont {
		t.Fatalf("first wide glyph mislaid: %#v / %#v", app.back.get(0, 0), app.back.get(1, 0))
	}
}
