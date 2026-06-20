package tui

import "testing"

// envStub builds a lookup function over a fixed map for ColorLevelFromEnv.
func envStub(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestColorLevelFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want ColorLevel
	}{
		{"no_color set", map[string]string{"NO_COLOR": "1", "COLORTERM": "truecolor"}, ColorLevelNone},
		{"no_color empty ignored", map[string]string{"NO_COLOR": "", "TERM": "xterm-256color"}, ColorLevel256},
		{"dumb terminal", map[string]string{"TERM": "dumb"}, ColorLevelNone},
		{"truecolor", map[string]string{"COLORTERM": "truecolor"}, ColorLevelTrueColor},
		{"24bit", map[string]string{"COLORTERM": "24bit"}, ColorLevelTrueColor},
		{"256color", map[string]string{"TERM": "screen-256color"}, ColorLevel256},
		{"basic", map[string]string{"TERM": "xterm"}, ColorLevel16},
		{"empty env", map[string]string{}, ColorLevel16},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ColorLevelFromEnv(envStub(tc.env)); got != tc.want {
				t.Fatalf("ColorLevelFromEnv(%v) = %d, want %d", tc.env, got, tc.want)
			}
		})
	}
}

func TestRGBTo16(t *testing.T) {
	tests := []struct {
		r, g, b uint8
		want    uint8
	}{
		{0, 0, 0, 0},        // black
		{255, 0, 0, 9},      // bright red
		{0, 255, 0, 10},     // bright green
		{255, 255, 255, 15}, // white
		{255, 255, 0, 11},   // bright yellow
		{10, 10, 10, 0},     // near black
	}
	for _, tc := range tests {
		if got := rgbTo16(tc.r, tc.g, tc.b); got != tc.want {
			t.Fatalf("rgbTo16(%d,%d,%d) = %d, want %d", tc.r, tc.g, tc.b, got, tc.want)
		}
	}
}

func TestRGBTo256(t *testing.T) {
	if got := rgbTo256(0, 0, 0); got != 16 {
		t.Fatalf("rgbTo256 black = %d, want 16", got)
	}
	if got := rgbTo256(255, 255, 255); got != 231 {
		t.Fatalf("rgbTo256 white = %d, want 231", got)
	}
	// A mid grey should resolve to the grayscale ramp (232-255), not the cube.
	if got := rgbTo256(128, 128, 128); got < 232 {
		t.Fatalf("rgbTo256 grey = %d, want grayscale ramp (>=232)", got)
	}
}

func TestAdaptColor(t *testing.T) {
	rgb := RGBColor(255, 0, 0)
	if got := adaptColor(rgb, ColorLevelNone); got != DefaultColor() {
		t.Fatalf("None should yield default color, got %+v", got)
	}
	if got := adaptColor(rgb, ColorLevelTrueColor); got != rgb {
		t.Fatalf("TrueColor should pass RGB through, got %+v", got)
	}
	if got := adaptColor(rgb, ColorLevel256); got.Mode != ColorANSI {
		t.Fatalf("256 should map RGB to ANSI index, got %+v", got)
	}
	if got := adaptColor(rgb, ColorLevel16); got != ANSIColor(9) {
		t.Fatalf("16 should map pure red to ANSIColor(9), got %+v", got)
	}
	// A 256-cube index degrades to a basic colour at level 16.
	if got := adaptColor(ANSIColor(231), ColorLevel16); got != ANSIColor(15) {
		t.Fatalf("256 index 231 (white) -> 16 should be ANSIColor(15), got %+v", got)
	}
	// Basic ANSI indices are left untouched everywhere.
	if got := adaptColor(ANSIColor(4), ColorLevel16); got != ANSIColor(4) {
		t.Fatalf("basic ANSI should be unchanged, got %+v", got)
	}
}

func TestColorCodeHonorsNoColor(t *testing.T) {
	saved := GetColorLevel()
	defer SetColorLevel(saved)

	SetColorLevel(ColorLevelNone)
	if got := RGBColor(10, 20, 30).fgCode(); got != "39" {
		t.Fatalf("NO_COLOR fg code = %q, want 39", got)
	}
	if got := ANSIColor(4).bgCode(); got != "49" {
		t.Fatalf("NO_COLOR bg code = %q, want 49", got)
	}

	SetColorLevel(ColorLevelTrueColor)
	if got := RGBColor(10, 20, 30).fgCode(); got != "38;2;10;20;30" {
		t.Fatalf("truecolor fg code = %q, want 38;2;10;20;30", got)
	}
}
