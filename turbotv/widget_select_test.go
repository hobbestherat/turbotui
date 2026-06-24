package tv

import (
	"io"
	"reflect"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// setupSelect builds a w×h desktop whose root layer contains a Select with the
// given options/bounds, selects the requested index, and returns the desktop
// (for App()/Redraw()) and the widget.
func setupSelect(w, h int, opts []string, bounds Rect, selected int) (*Desktop, *Select) {
	app := tui.NewWithSize(w, h, io.Discard)
	desktop := NewDesktop(app)
	s := NewSelect(desktop, opts, bounds)
	if selected >= 0 && selected < len(opts) {
		s.SetSelected(selected)
	}
	root := NewComponent(Rect{X: 0, Y: 0, W: w, H: h})
	root.AddChild(s.Component)
	desktop.AddLayer(NewLayer("test", root, true, false))
	return desktop, s
}

// readRunes reads up to n glyphs starting at (x, y) from the app's back buffer.
func readRunes(app *tui.App, x, y, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if ch := app.ReadCell(x+i, y).Ch; ch != 0 {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// Issue #6: the collapsed control must ellipsize a long value instead of
// raw-clipping it, and must not touch a value that fits.
func TestSelectEllipsizesLongValueWhenCollapsed(t *testing.T) {
	long := "ThisIsAVeryLongModelNameThatDoesNotFit"
	desktop, s := setupSelect(40, 5, []string{"short", long}, Rect{X: 0, Y: 0, W: 12, H: 1}, 1)
	s.Component.hasFocus = true
	desktop.Redraw()
	app := desktop.App()
	// maxText = W-2 = 10, so the last text column is x=9 and must be the ellipsis.
	if got := app.ReadCell(9, 0).Ch; got != '…' {
		t.Fatalf("long value should end in an ellipsis at x=9, got %q", got)
	}
	if got := app.ReadCell(0, 0).Ch; got != 'T' {
		t.Fatalf("value should still start at the first column, got %q", got)
	}

	// A short value must be shown in full with no ellipsis.
	desktop2, s2 := setupSelect(40, 5, []string{"short", long}, Rect{X: 0, Y: 0, W: 12, H: 1}, 0)
	s2.Component.hasFocus = true
	desktop2.Redraw()
	if got := desktop2.App().ReadCell(9, 0).Ch; got == '…' {
		t.Fatalf("short value should not be ellipsized, got %q", got)
	}
	if got := readRunes(desktop2.App(), 0, 0, 5); got != "short" {
		t.Fatalf("short value should render in full, got %q", got)
	}
}

// Issue #6: the open popup widens past the control so the longest option is
// fully visible.
func TestSelectPopupWidensToFitLongOption(t *testing.T) {
	long := "Local LAN (env: GOGENT_CONFIG=path)"
	_, s := setupSelect(80, 24, []string{"x", long}, Rect{X: 5, Y: 5, W: 14, H: 1}, 0)
	s.open()
	rect := s.popupRect()
	need := tui.StringWidth(long) + 2
	if rect.W < need {
		t.Fatalf("popup width %d should be >= %d to fit the longest option", rect.W, need)
	}
	if rect.W <= 14 {
		t.Fatalf("popup should be wider than the 14-wide control, got %d", rect.W)
	}
}

// Issue #6: the longest option's full text actually renders in the open popup.
func TestSelectPopupRendersFullOptionText(t *testing.T) {
	long := "Local LAN (env: GOGENT_CONFIG=path)"
	desktop, s := setupSelect(80, 24, []string{"x", long}, Rect{X: 5, Y: 5, W: 14, H: 1}, 0)
	s.open()
	desktop.Redraw()
	rect := s.popupRect()
	inner := Rect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}
	// Two options, viewOffset 0 → the long option is on the second row.
	got := readRunes(desktop.App(), inner.X, inner.Y+1, tui.StringWidth(long))
	if got != long {
		t.Fatalf("popup should show the full option text, got %q", got)
	}
}

// Issue #32: a Select near the bottom of the screen must drop its popup upward
// instead of clipping it to a sliver or drawing it off-screen.
func TestSelectPopupFlipsUpNearBottom(t *testing.T) {
	opts := make([]string, 8)
	_, s := setupSelect(40, 10, opts, Rect{X: 0, Y: 8, W: 14, H: 1}, 0)
	s.open()
	rect := s.popupRect()
	abs := s.Component.AbsoluteBounds()
	// Control at y=8 with a 10-tall screen leaves only one row below, so the
	// popup must sit immediately above the control and stay on screen.
	if rect.Y+rect.H != abs.Y {
		t.Fatalf("popup should sit just above the control: bottom %d, control top %d", rect.Y+rect.H, abs.Y)
	}
	if rect.Y < 0 {
		t.Fatalf("popup should stay on screen, got Y=%d", rect.Y)
	}
}

// Issue #32: with room below, the popup keeps dropping down in the usual place.
func TestSelectPopupDropsDownWithRoomBelow(t *testing.T) {
	opts := make([]string, 8)
	_, s := setupSelect(40, 24, opts, Rect{X: 0, Y: 2, W: 14, H: 1}, 0)
	s.open()
	rect := s.popupRect()
	if rect.Y != 3 {
		t.Fatalf("popup should drop to y=3 (just below control at y=2), got y=%d", rect.Y)
	}
	if rect.H != len(opts)+2 {
		t.Fatalf("popup should show all options, got H=%d want %d", rect.H, len(opts)+2)
	}
}

// Issue #33: the highlighted row uses the theme's SelectionFG/SelectionBG instead
// of a hardcoded colour.
func TestSelectPopupHighlightUsesTheme(t *testing.T) {
	original := ActiveTheme()
	custom := original
	custom.SelectionFG = tui.ANSIColor(13)
	custom.SelectionBG = tui.ANSIColor(3)
	SetTheme(custom)
	defer SetTheme(original)

	desktop, s := setupSelect(40, 10, []string{"alpha", "bravo"}, Rect{X: 0, Y: 0, W: 14, H: 1}, 0)
	s.open()
	desktop.Redraw()
	app := desktop.App()
	rect := s.popupRect()
	inner := Rect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}
	high := app.ReadCell(inner.X, inner.Y) // highlight is on option 0
	if high.FG != custom.SelectionFG || high.BG != custom.SelectionBG {
		t.Fatalf("highlight row should use theme SelectionFG/BG, got FG=%v BG=%v", high.FG, high.BG)
	}
	other := app.ReadCell(inner.X, inner.Y+1) // non-highlight row
	if other.BG != custom.DialogBG {
		t.Fatalf("non-highlight row should use DialogBG, got BG=%v", other.BG)
	}
}

// Issue #34: clicking the scrollbar scrolls the list instead of committing an
// option, and the wheel scrolls too.
func TestSelectPopupScrollbarAndWheelScroll(t *testing.T) {
	opts := make([]string, 20)
	for i := range opts {
		opts[i] = "opt"
	}
	_, s := setupSelect(40, 10, opts, Rect{X: 0, Y: 0, W: 20, H: 1}, 0)
	s.open()
	rect := s.popupRect()
	inner := Rect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}
	trackX := inner.X + inner.W - 1

	// Pressing the scrollbar's bottom arrow scrolls one row down.
	s.popupClick(nil, tui.ClickEvent{X: trackX, Y: inner.Bottom(), Button: tui.MouseLeft, Down: true})
	if s.offset == 0 {
		t.Fatalf("scrollbar click should scroll the list, offset still 0")
	}
	if !s.IsOpen() {
		t.Fatalf("scrollbar click must not commit/close the popup")
	}

	// Wheel-down scrolls further.
	prev := s.offset
	s.popupScroll(nil, tui.ScrollEvent{Delta: -1})
	if s.offset <= prev {
		t.Fatalf("wheel down should increase offset, was %d now %d", prev, s.offset)
	}
}

// Issue #34: an ordinary item click still commits (regression guard for the
// scrollbar split-out), and a click outside dismisses.
func TestSelectPopupItemClickCommitsAndOutsideDismisses(t *testing.T) {
	desktop, s := setupSelect(40, 20, []string{"a", "b", "c"}, Rect{X: 0, Y: 0, W: 14, H: 1}, 0)
	s.open()
	rect := s.popupRect()
	inner := Rect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}

	// Press + release on item row 2 commits option 2 and closes.
	s.popupClick(nil, tui.ClickEvent{X: inner.X, Y: inner.Y + 2, Button: tui.MouseLeft, Down: true})
	s.popupClick(nil, tui.ClickEvent{X: inner.X, Y: inner.Y + 2, Button: tui.MouseLeft, Down: false})
	if s.Selected != 2 {
		t.Fatalf("click should commit item 2, got Selected=%d", s.Selected)
	}
	if s.IsOpen() {
		t.Fatalf("popup should close after committing an item")
	}

	// Reopen, then a release outside the popup dismisses it.
	_, s2 := setupSelect(40, 20, []string{"a", "b"}, Rect{X: 0, Y: 0, W: 14, H: 1}, 0)
	s2.open()
	s2.popupClick(nil, tui.ClickEvent{X: 30, Y: 15, Button: tui.MouseLeft, Down: false})
	if s2.IsOpen() {
		t.Fatalf("click outside popup should close it")
	}
	_ = desktop
}

// Issue #35: Home/End, arrows, clamping and first-letter type-ahead (incl. wrap).
func TestSelectPopupKeyboardNavigation(t *testing.T) {
	opts := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"}
	cases := []struct {
		name  string
		start int
		event tui.TypeEvent
		want  int
	}{
		{"home", 3, tui.TypeEvent{Key: tui.KeyHome}, 0},
		{"end", 1, tui.TypeEvent{Key: tui.KeyEnd}, 5},
		{"up", 3, tui.TypeEvent{Key: tui.KeyUp}, 2},
		{"down", 3, tui.TypeEvent{Key: tui.KeyDown}, 4},
		{"up clamps at top", 0, tui.TypeEvent{Key: tui.KeyUp}, 0},
		{"down clamps at bottom", 5, tui.TypeEvent{Key: tui.KeyDown}, 5},
		{"typeahead forward", 0, tui.TypeEvent{Key: tui.KeyRune, Rune: 'c'}, 2},
		{"typeahead next same letter", 0, tui.TypeEvent{Key: tui.KeyRune, Rune: 'e'}, 4},
		{"typeahead wraps", 2, tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'}, 0},
		{"typeahead case-insensitive", 0, tui.TypeEvent{Key: tui.KeyRune, Rune: 'B'}, 1},
		{"typeahead no match keeps", 0, tui.TypeEvent{Key: tui.KeyRune, Rune: 'z'}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, s := setupSelect(60, 24, opts, Rect{X: 0, Y: 0, W: 20, H: 1}, 0)
			s.open()
			s.highlight = c.start
			s.popupType(nil, c.event)
			if s.highlight != c.want {
				t.Fatalf("highlight = %d, want %d", s.highlight, c.want)
			}
		})
	}
}

// Issue #35: PageUp/PageDown page through a scrollable list, and opening on a
// low item scrolls it into view (ensureVisible).
func TestSelectPopupPagingAndEnsureVisible(t *testing.T) {
	opts := make([]string, 10)
	// screenH=7, control at y=0 → 6 rows below → height 6 → 4 visible rows.
	_, s := setupSelect(40, 7, opts, Rect{X: 0, Y: 0, W: 20, H: 1}, 8)
	s.open()
	// Opening on option 8 with 4 visible rows scrolls so offset = 8-4+1 = 5.
	if s.offset != 5 {
		t.Fatalf("offset after open = %d, want 5", s.offset)
	}
	if s.highlight != 8 {
		t.Fatalf("highlight = %d, want 8", s.highlight)
	}
	// PageUp moves up by a page (4) → 4.
	s.popupType(nil, tui.TypeEvent{Key: tui.KeyPageUp})
	if s.highlight != 4 {
		t.Fatalf("PageUp highlight = %d, want 4", s.highlight)
	}
	// PageDown moves back down by a page → 8.
	s.popupType(nil, tui.TypeEvent{Key: tui.KeyPageDown})
	if s.highlight != 8 {
		t.Fatalf("PageDown highlight = %d, want 8", s.highlight)
	}
	// PageDown past the end clamps to the last option (9).
	s.popupType(nil, tui.TypeEvent{Key: tui.KeyPageDown})
	if s.highlight != 9 {
		t.Fatalf("PageDown clamp highlight = %d, want 9", s.highlight)
	}
}

// viewOffset clamps the stored offset to the valid scroll range.
func TestSelectViewOffsetClamps(t *testing.T) {
	opts := make([]string, 10)
	_, s := setupSelect(40, 7, opts, Rect{X: 0, Y: 0, W: 20, H: 1}, 0)
	s.open() // 4 visible rows → max offset 6
	s.offset = -3
	if got := s.viewOffset(); got != 0 {
		t.Fatalf("viewOffset should clamp negatives to 0, got %d", got)
	}
	s.offset = 999
	if got := s.viewOffset(); got != 6 {
		t.Fatalf("viewOffset should clamp to max 6, got %d", got)
	}
}

func TestSelectSetOptionsReplacesOptionsAndPreservesSelectionByValue(t *testing.T) {
	_, s := setupSelect(40, 10, []string{"alpha", "bravo", "charlie"}, Rect{X: 0, Y: 0, W: 20, H: 1}, 1)

	next := []string{"charlie", "alpha", "bravo", "delta"}
	s.SetOptions(next)

	if !reflect.DeepEqual(s.Options, next) {
		t.Fatalf("Options = %#v, want %#v", s.Options, next)
	}
	if s.Selected != 2 {
		t.Fatalf("Selected = %d, want 2 for preserved value %q", s.Selected, "bravo")
	}
	if got := s.Value(); got != "bravo" {
		t.Fatalf("Value() = %q, want preserved value %q", got, "bravo")
	}
}

func TestSelectSetOptionsClampsToZeroWhenSelectedValueIsGone(t *testing.T) {
	_, s := setupSelect(40, 10, []string{"alpha", "bravo", "charlie"}, Rect{X: 0, Y: 0, W: 20, H: 1}, 1)

	s.SetOptions([]string{"delta", "echo"})

	if s.Selected != 0 {
		t.Fatalf("Selected = %d, want 0 when old value is absent", s.Selected)
	}
	if got := s.Value(); got != "delta" {
		t.Fatalf("Value() = %q, want first replacement option %q", got, "delta")
	}
}

func TestSelectSetOptionsClosesOpenPopupAndResetsScrollState(t *testing.T) {
	opts := make([]string, 20)
	for i := range opts {
		opts[i] = "option"
	}
	opts[12] = "keep"
	_, s := setupSelect(40, 7, opts, Rect{X: 0, Y: 0, W: 20, H: 1}, 12)
	s.open()
	if !s.IsOpen() {
		t.Fatalf("precondition: popup should be open")
	}
	if s.offset == 0 {
		t.Fatalf("precondition: opening selected option 12 should scroll it into view")
	}
	s.popupScroll(nil, tui.ScrollEvent{Delta: -1})
	s.highlight = 15
	s.SetOptions([]string{"first", "keep", "last"})

	if s.IsOpen() {
		t.Fatalf("SetOptions should close an open popup")
	}
	if s.Selected != 1 {
		t.Fatalf("Selected = %d, want 1 for preserved value after replacement", s.Selected)
	}
	if s.offset != 0 {
		t.Fatalf("offset = %d, want reset to 0", s.offset)
	}
	if s.highlight != s.Selected {
		t.Fatalf("highlight = %d, want selected index %d after reset", s.highlight, s.Selected)
	}
}

func TestSelectSetOptionsHandlesEmptyOptions(t *testing.T) {
	_, s := setupSelect(40, 10, []string{"alpha", "bravo"}, Rect{X: 0, Y: 0, W: 20, H: 1}, 1)
	s.open()

	s.SetOptions(nil)

	if s.IsOpen() {
		t.Fatalf("SetOptions(nil) should close the popup")
	}
	if len(s.Options) != 0 {
		t.Fatalf("len(Options) = %d, want 0", len(s.Options))
	}
	if s.Selected != 0 {
		t.Fatalf("Selected = %d, want 0 for an empty option list", s.Selected)
	}
	if got := s.Value(); got != "" {
		t.Fatalf("Value() = %q, want empty string for an empty option list", got)
	}
	s.open()
	if s.IsOpen() {
		t.Fatalf("open on an empty option list should leave popup closed")
	}
}

func TestSelectSetOptionsPreservesEmptyStringSelectionByValue(t *testing.T) {
	_, s := setupSelect(40, 10, []string{"alpha", "", "charlie"}, Rect{X: 0, Y: 0, W: 20, H: 1}, 1)

	s.SetOptions([]string{"first", "second", ""})

	if s.Selected != 2 {
		t.Fatalf("Selected = %d, want 2 to preserve the selected empty-string option by value", s.Selected)
	}
	if got := s.Value(); got != "" {
		t.Fatalf("Value() = %q, want preserved empty string", got)
	}
}

func TestSelectSetOptionsInvalidCurrentSelectionDoesNotPreserveEmptyValue(t *testing.T) {
	_, s := setupSelect(40, 10, []string{"alpha", "bravo"}, Rect{X: 0, Y: 0, W: 20, H: 1}, 0)
	s.Selected = 99

	s.SetOptions([]string{"first", "", "last"})

	if s.Selected != 0 {
		t.Fatalf("Selected = %d, want 0 when there was no valid current selection", s.Selected)
	}
	if got := s.Value(); got != "first" {
		t.Fatalf("Value() = %q, want first option after clamping invalid selection", got)
	}
}
