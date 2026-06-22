package tv

import (
	"reflect"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// TestLongestLineWidth checks the widest hard line is measured in display cells.
func TestLongestLineWidth(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"single line", "hello", 5},
		{"widest of several", "hi\nworld!\nyo", 6},
		{"trailing newline ignored", "abc\n", 3},
		{"leading newline", "\nlonger", 6},
		{"blank lines only", "\n\n", 0},
		{"cjk counted as two cells", "世界", 4},
		{"mixed ascii and cjk", "ab\n世界界", 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LongestLineWidth(tc.in); got != tc.want {
				t.Fatalf("LongestLineWidth(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestLongestLineWidthMatchesStringWidthOnSingleLine pins the helper to the
// underlying cell-width source of truth for a line with no newlines.
func TestLongestLineWidthMatchesStringWidth(t *testing.T) {
	for _, s := range []string{"plain", "tab\tless", "émoji-ish", "a世b界c"} {
		if got, want := LongestLineWidth(s), tui.StringWidth(s); got != want {
			t.Errorf("LongestLineWidth(%q)=%d, want StringWidth=%d", s, got, want)
		}
	}
}

// TestParseMnemonic exercises the '&' marker rules: first marker wins, '&&' is a
// literal ampersand, a trailing lone '&' is kept verbatim, and the returned hot
// index is into the cleaned text.
func TestParseMnemonicExported(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantHot int
	}{
		{"no marker", "Yes", "Yes", -1},
		{"leading marker", "&Yes", "Yes", 0},
		{"interior marker", "Sa&ve", "Save", 2},
		{"literal double ampersand", "a&&b", "a&b", -1},
		{"only first marker counts", "&A&B", "AB", 0},
		{"trailing lone ampersand kept", "a&", "a&", -1},
		{"single ampersand kept", "&", "&", -1},
		{"double then marker", "&&x&Y", "&xY", 2},
		{"empty", "", "", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, hot := ParseMnemonic(tc.in)
			if got != tc.want || hot != tc.wantHot {
				t.Fatalf("ParseMnemonic(%q) = (%q, %d), want (%q, %d)", tc.in, got, hot, tc.want, tc.wantHot)
			}
		})
	}
}

// TestParseMnemonicHotIndexAddressesCleanText verifies the hot index points at
// the marked rune in the *cleaned* string (the contract WrapLabelRunes relies on).
func TestParseMnemonicHotIndexAddressesCleanText(t *testing.T) {
	clean, hot := ParseMnemonic("Op&en file")
	runes := []rune(clean)
	if hot < 0 || hot >= len(runes) || runes[hot] != 'e' {
		t.Fatalf("hot index %d does not address 'e' in %q", hot, clean)
	}
}

// TestButtonLabelWidthExported documents the exported width policy independently
// of the in-package alias: chrome adds 4 cells, the mnemonic marker is free, and
// short captions floor at minButtonWidth.
func TestButtonLabelWidthExported(t *testing.T) {
	cases := []struct {
		label string
		want  int
	}{
		{"OK", minButtonWidth},
		{"&OK", minButtonWidth},
		{"Cancel", minButtonWidth}, // 6+4 == floor
		{"Continue", len("Continue") + 4},
		{"&Save as", len("Save as") + 4},
		{"世界世界", tui.StringWidth("世界世界") + 4}, // 8 cells + 4 chrome, above the floor
		{"世界", minButtonWidth},                // 4 cells + 4 chrome = 8, floored to the minimum
	}
	for _, tc := range cases {
		if got := ButtonLabelWidth(tc.label); got != tc.want {
			t.Errorf("ButtonLabelWidth(%q) = %d, want %d", tc.label, got, tc.want)
		}
	}
}

// TestWrapText covers the TextView wrap: whitespace preservation, hard splits,
// the empty-string base case, width clamping, and the (intentional) fact that a
// raw newline is *not* a break here.
func TestWrapText(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"empty yields one empty row", "", 5, []string{""}},
		{"fits whole", "hello", 10, []string{"hello"}},
		{"exact width", "hello", 5, []string{"hello"}},
		{"wrap keeps trailing space", "hello world", 7, []string{"hello ", "world"}},
		{"hard split long word", "abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"leading indentation preserved", "  hi", 10, []string{"  hi"}},
		{"interior spaces kept when they fit", "a   b", 10, []string{"a   b"}},
		{"interior spaces dropped at a break", "aaa   bbb", 5, []string{"aaa", "bbb"}},
		{"width below one clamps to one", "ab", 0, []string{"a", "b"}},
		{"raw newline is not a break", "a\nb", 10, []string{"a\nb"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WrapText(tc.text, tc.width)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("WrapText(%q, %d) = %#v, want %#v", tc.text, tc.width, got, tc.want)
			}
		})
	}
}

// TestWrapTextExpandsTabs checks tabs are expanded to the tab stop before wrapping
// so aligned content survives, matching the TextView's own measure.
func TestWrapTextExpandsTabs(t *testing.T) {
	got := WrapText("\tx", 20)
	want := []string{"        x"} // tab -> 8 spaces to the next stop, then x
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WrapText(\"\\tx\", 20) = %#v, want %#v", got, want)
	}
}

// TestWrapTextRowCountPredictsHeight is the property the sizing policy depends on:
// the number of wrapped rows is what a caller uses to predict TextView height.
func TestWrapTextRowCountPredictsHeight(t *testing.T) {
	text := "the quick brown fox jumps over the lazy dog"
	for _, w := range []int{5, 10, 20, 80} {
		rows := WrapText(text, w)
		for i, r := range rows {
			if l := len([]rune(r)); l > w && w >= 1 {
				t.Fatalf("row %d %q has %d runes, exceeds width %d", i, r, l, w)
			}
		}
	}
}

// --- wrapper parity ----------------------------------------------------------
//
// The in-package unexported helpers must remain thin wrappers over the exported
// source of truth (the task forbids duplicating the logic). These tests fail if a
// wrapper is ever re-implemented and drifts.

func TestUnexportedHelpersDelegateToExported(t *testing.T) {
	for _, s := range []string{"", "one line", "a\nbb\nccc", "世界\nx"} {
		if longestLineWidth(s) != LongestLineWidth(s) {
			t.Errorf("longestLineWidth(%q)=%d != LongestLineWidth=%d", s, longestLineWidth(s), LongestLineWidth(s))
		}
	}
	for _, s := range []string{"OK", "&Save", "Very long button label"} {
		if buttonLabelWidth(s) != ButtonLabelWidth(s) {
			t.Errorf("buttonLabelWidth(%q)=%d != ButtonLabelWidth=%d", s, buttonLabelWidth(s), ButtonLabelWidth(s))
		}
	}
	for _, s := range []string{"Yes", "&No", "a&&b", "Op&en"} {
		gc, gh := parseMnemonic(s)
		ec, eh := ParseMnemonic(s)
		if gc != ec || gh != eh {
			t.Errorf("parseMnemonic(%q)=(%q,%d) != ParseMnemonic=(%q,%d)", s, gc, gh, ec, eh)
		}
	}
	for _, s := range []string{"", "hello world wrap me", "tab\tstop", "loooooongword"} {
		for _, w := range []int{1, 4, 8, 40} {
			if !reflect.DeepEqual(wrapText(s, w), WrapText(s, w)) {
				t.Errorf("wrapText(%q,%d) != WrapText", s, w)
			}
		}
	}
}
