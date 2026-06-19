package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestSurfaceWriteStringWideAdvances(t *testing.T) {
	app := tui.NewWithSize(10, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	surface.WriteString(0, 0, "世a", tui.Cell{FG: tui.ANSIColor(15), BG: tui.ANSIColor(0)})

	if got := app.ReadCell(0, 0).Ch; got != '世' {
		t.Fatalf("cell 0 = %q, want 世", got)
	}
	// 'a' must follow the wide glyph at column 2, keeping the row aligned.
	if got := app.ReadCell(2, 0).Ch; got != 'a' {
		t.Fatalf("cell 2 = %q, want a", got)
	}
}

func TestSurfaceWideGlyphStraddlingClipIsBlanked(t *testing.T) {
	app := tui.NewWithSize(10, 1, &bytes.Buffer{})
	// Clip ends at column 2 (covers columns 0..1); a wide glyph at column 1 would
	// spill its right half into column 2, outside the clip.
	surface := newRootSurface(app).WithClip(Rect{X: 0, Y: 0, W: 2, H: 1})
	surface.WriteString(0, 0, "a世b", tui.Cell{FG: tui.ANSIColor(15), BG: tui.ANSIColor(0)})

	if got := app.ReadCell(0, 0).Ch; got != 'a' {
		t.Fatalf("cell 0 = %q, want a", got)
	}
	// The wide glyph cannot fit inside the clip, so its visible half is blanked
	// and it never overdraws column 2.
	if got := app.ReadCell(1, 0).Ch; got != ' ' {
		t.Fatalf("cell 1 = %q, want blank", got)
	}
	if got := app.ReadCell(2, 0).Ch; got == 'b' || got == '世' {
		t.Fatalf("cell 2 should be untouched by clipped write, got %q", got)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		maxWidth int
		ellipsis string
		want     string
	}{
		{"fits", "hello", 10, "…", "hello"},
		{"exact", "hello", 5, "…", "hello"},
		{"cut with ellipsis", "hello world", 7, "…", "hello …"},
		{"cut no ellipsis", "hello world", 5, "", "hello"},
		{"wide not split", "世界世界", 5, "", "世界"},
		{"wide with ellipsis", "世界世界", 5, "…", "世界…"},
		{"zero width", "hello", 0, "…", ""},
		{"ellipsis wider than max", "hello", 1, "...", "h"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Truncate(tc.text, tc.maxWidth, tc.ellipsis); got != tc.want {
				t.Fatalf("Truncate(%q, %d, %q) = %q, want %q", tc.text, tc.maxWidth, tc.ellipsis, got, tc.want)
			}
		})
	}
}

func TestWriteStringClippedStopsAtMaxWidth(t *testing.T) {
	app := tui.NewWithSize(20, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	surface.WriteStringClipped(0, 0, 3, "abcdef", tui.Cell{FG: tui.ANSIColor(15), BG: tui.ANSIColor(0)})

	for x, want := range []rune{'a', 'b', 'c'} {
		if got := app.ReadCell(x, 0).Ch; got != want {
			t.Fatalf("cell %d = %q, want %q", x, got, want)
		}
	}
	// Column 3 must remain the default blank: the write stopped at maxWidth.
	if got := app.ReadCell(3, 0).Ch; got != ' ' {
		t.Fatalf("cell 3 = %q, want blank (clipped)", got)
	}
}
