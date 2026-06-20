package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// rowsText flattens wrapped rows back to their text for compact comparison.
func rowsText(rows []labelRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = string(r.runes)
	}
	return out
}

func TestWrapLabelRunes(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"fits one line", "hello", 10, []string{"hello"}},
		{"exact width", "hello", 5, []string{"hello"}},
		{"wrap at space", "hello world", 7, []string{"hello", "world"}},
		{"two words per row", "a b c", 3, []string{"a b", "c"}},
		{"hard split long word", "abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"empty", "", 5, []string{""}},
		{"newline is a hard break", "line1\nline2", 10, []string{"line1", "line2"}},
		{"wide glyphs counted by width", "世界test", 5, []string{"世界t", "est"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rowsText(wrapLabelRunes([]rune(tc.text), tc.width))
			if len(got) != len(tc.want) {
				t.Fatalf("wrap %q width %d = %v (rows=%d), want %v (rows=%d)",
					tc.text, tc.width, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("wrap %q width %d row %d = %q, want %q\nfull: %v",
						tc.text, tc.width, i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

// TestWrapLabelRunesPreservesRuneOffsets checks each row is a faithful contiguous
// slice of the source (start+len matches), which is what lets the mnemonic hot
// index map onto a row after wrapping.
func TestWrapLabelRunesPreservesRuneOffsets(t *testing.T) {
	clean := []rune("hello world foo")
	rows := wrapLabelRunes(clean, 7)
	wantStarts := []int{0, 6, 12}
	if len(rows) != len(wantStarts) {
		t.Fatalf("got %d rows, want %d: %v", len(rows), len(wantStarts), rowsText(rows))
	}
	for i, r := range rows {
		if r.start != wantStarts[i] {
			t.Fatalf("row %d start = %d, want %d", i, r.start, wantStarts[i])
		}
		// The slice must be the actual source bytes at that offset.
		if string(r.runes) != string(clean[r.start:r.start+len(r.runes)]) {
			t.Fatalf("row %d runes are not a contiguous slice of the source", i)
		}
	}
}

// TestLabelDrawWrapsAcrossRows verifies a wrapped label paints its continuation
// onto the rows below instead of clipping to the first line.
func TestLabelDrawWrapsAcrossRows(t *testing.T) {
	app := tui.NewWithSize(16, 4, &bytes.Buffer{})
	surface := newRootSurface(app)
	label := NewLabel("Hello World Foo", Rect{X: 0, Y: 0, W: 7, H: 3})
	label.draw(label.Component, surface)

	for x, want := range []rune("Hello") {
		if got := app.ReadCell(x, 0).Ch; got != want {
			t.Fatalf("row0 cell %d = %q, want %q", x, got, want)
		}
	}
	for x, want := range []rune("World") {
		if got := app.ReadCell(x, 1).Ch; got != want {
			t.Fatalf("row1 cell %d = %q, want %q", x, got, want)
		}
	}
	for x, want := range []rune("Foo") {
		if got := app.ReadCell(x, 2).Ch; got != want {
			t.Fatalf("row2 cell %d = %q, want %q", x, got, want)
		}
	}
}

// TestLabelDrawWrapsMnemonic verifies the '&' mnemonic highlight lands on the
// wrapped row that actually contains the hot character.
func TestLabelDrawWrapsMnemonic(t *testing.T) {
	app := tui.NewWithSize(16, 4, &bytes.Buffer{})
	surface := newRootSurface(app)
	label := NewLabel("Hello &World Foo", Rect{X: 0, Y: 0, W: 7, H: 3})
	label.Component.mnemonicActive = true
	label.draw(label.Component, surface)

	// The 'W' mnemonic is on row 1 (the second wrapped line), column 0.
	cell := app.ReadCell(0, 1)
	if cell.Ch != 'W' {
		t.Fatalf("expected 'W' at (0,1), got %q", cell.Ch)
	}
	if !cell.Underline {
		t.Fatalf("expected the mnemonic 'W' to be underlined")
	}
	if cell.FG != label.HotFG {
		t.Fatalf("expected mnemonic FG to be the label HotFG")
	}
	// Row 0 holds plain "Hello" with no underline highlight.
	if app.ReadCell(0, 0).Underline {
		t.Fatalf("row 0 should not carry the mnemonic highlight")
	}
}

// TestLabelWrapDisabledClipsToFirstLine guards the legacy behaviour: with Wrap
// off a label draws only its first line (the rest is clipped by the surface).
func TestLabelWrapDisabledClipsToFirstLine(t *testing.T) {
	app := tui.NewWithSize(16, 4, &bytes.Buffer{})
	surface := newRootSurface(app)
	label := NewLabel("Hello World Foo", Rect{X: 0, Y: 0, W: 7, H: 3})
	label.Wrap = false
	label.draw(label.Component, surface)

	// Row 1 stays blank: nothing wrapped onto it.
	if got := app.ReadCell(0, 1).Ch; got != ' ' {
		t.Fatalf("with Wrap off, row1 should be blank, got %q", got)
	}
}
