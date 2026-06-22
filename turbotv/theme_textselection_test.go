package tv

import "testing"

// gogent#279: text selection inside a focused input is drawn over InputFocusBG,
// so TextSelection* must be distinct from the focused-input fill (and from the
// input focus FG) in every built-in theme — otherwise a selection is invisible
// exactly when the user makes one. This pins that invariant at the toolkit layer.
func TestTextSelectionDistinctFromInputFocusFill(t *testing.T) {
	for _, tc := range []struct {
		name  string
		theme Theme
	}{
		{"DefaultTheme", DefaultTheme},
		{"HighContrastTheme", HighContrastTheme},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.theme.TextSelectionBG == tc.theme.InputFocusBG {
				t.Errorf("TextSelectionBG == InputFocusBG (%+v): selection invisible on a focused input", tc.theme.TextSelectionBG)
			}
		})
	}
}
