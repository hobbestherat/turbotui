package tv

import (
	"io"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func setupColorPicker(t *testing.T, w, h int, level tui.ColorLevel, color tui.Color) (*Desktop, *ColorPicker) {
	t.Helper()
	saved := tui.GetColorLevel()
	tui.SetColorLevel(level)
	t.Cleanup(func() { tui.SetColorLevel(saved) })

	app := tui.NewWithSize(w, h, io.Discard)
	desktop := NewDesktop(app)
	p := NewColorPicker(desktop, Rect{X: 2, Y: 1, W: 10, H: 1})
	p.SetColor(color)
	root := NewComponent(Rect{X: 0, Y: 0, W: w, H: h})
	root.AddChild(p.Component)
	desktop.AddLayer(NewLayer("test", root, true, false))
	desktop.SetFocus(p.Component)
	return desktop, p
}

func TestColorPickerPaletteIsLevelAware(t *testing.T) {
	tests := []struct {
		name        string
		level       tui.ColorLevel
		wantCount   int
		wantCols    int
		wantSliders bool
	}{
		{"16 color", tui.ColorLevel16, 17, 8, false},
		{"256 color", tui.ColorLevel256, 257, 16, false},
		{"truecolor", tui.ColorLevelTrueColor, 257, 16, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, p := setupColorPicker(t, 80, 24, tc.level, tui.DefaultColor())
			p.open()
			if !p.IsOpen() {
				t.Fatalf("picker should open at color level %v", tc.level)
			}
			if got := len(p.colors); got != tc.wantCount {
				t.Fatalf("palette size = %d, want %d", got, tc.wantCount)
			}
			if got := p.cols; got != tc.wantCols {
				t.Fatalf("grid columns = %d, want %d", got, tc.wantCols)
			}
			if got := p.hasSliders(); got != tc.wantSliders {
				t.Fatalf("hasSliders = %v, want %v", got, tc.wantSliders)
			}
			if p.colors[0] != tui.DefaultColor() {
				t.Fatalf("first palette cell should be terminal default, got %+v", p.colors[0])
			}
			if p.colors[len(p.colors)-1] != tui.ANSIColor(uint8(tc.wantCount-2)) {
				t.Fatalf("last palette cell = %+v, want ANSI %d", p.colors[len(p.colors)-1], tc.wantCount-2)
			}
		})
	}
}

func TestColorPickerPaletteGridRendersEveryCellWhenItFits(t *testing.T) {
	tests := []struct {
		name      string
		level     tui.ColorLevel
		wantCount int
	}{
		{"16 color", tui.ColorLevel16, 17},
		{"256 color", tui.ColorLevel256, 257},
		{"truecolor", tui.ColorLevelTrueColor, 257},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desktop, p := setupColorPicker(t, 80, 30, tc.level, tui.ANSIColor(1))
			p.open()
			desktop.Redraw()
			lay := p.layout()
			if lay.visRows != p.gridRowCount() {
				t.Fatalf("test screen should fit whole palette, got visRows=%d rowCount=%d", lay.visRows, p.gridRowCount())
			}
			if rendered := lay.grid.H * p.cols; rendered < tc.wantCount {
				t.Fatalf("grid renders capacity for %d cells, want at least %d", rendered, tc.wantCount)
			}
			last := tc.wantCount - 1
			lastX := lay.grid.X + (last%p.cols)*colorSwatchW
			lastY := lay.grid.Y + last/p.cols
			if got := desktop.App().ReadCell(lastX, lastY).BG; got != tui.ANSIColor(uint8(tc.wantCount-2)) {
				t.Fatalf("last rendered palette cell BG = %+v, want ANSI %d", got, tc.wantCount-2)
			}
			after := tc.wantCount
			afterX := lay.grid.X + (after%p.cols)*colorSwatchW
			afterY := lay.grid.Y + after/p.cols
			if afterY <= lay.grid.Bottom() {
				if got := desktop.App().ReadCell(afterX, afterY).BG; got != activeTheme.DialogBG {
					t.Fatalf("cell after palette should remain dialog background, got BG=%+v", got)
				}
			}
		})
	}
}

func TestColorPickerPopupClipsGridOnNarrowTerminals(t *testing.T) {
	desktop, p := setupColorPicker(t, 20, 10, tui.ColorLevel256, tui.DefaultColor())
	p.open()
	desktop.Redraw()
	lay := p.layout()
	border := tui.BorderStyleFor(tui.LineSingle)

	if lay.rect.W != desktop.App().Width() {
		t.Fatalf("test expects popup width clamped to screen, got rect=%+v screenW=%d", lay.rect, desktop.App().Width())
	}
	for y := lay.rect.Y + 1; y < lay.rect.Bottom(); y++ {
		if got := desktop.App().ReadCell(lay.rect.Right(), y).Ch; got != border.Vertical {
			t.Fatalf("right border at y=%d was overwritten by grid content: got %q want %q", y, got, border.Vertical)
		}
	}
}

func TestColorPickerPopupClipsTrueColorControlsOnShortTerminals(t *testing.T) {
	desktop, p := setupColorPicker(t, 80, 6, tui.ColorLevelTrueColor, tui.RGBColor(10, 20, 30))
	p.open()
	desktop.Redraw()
	lay := p.layout()
	border := tui.BorderStyleFor(tui.LineSingle)

	if lay.sliders.H == 0 {
		t.Fatalf("test expects truecolor sliders to be present")
	}
	if lay.sliders.Bottom() <= lay.rect.Bottom() {
		t.Fatalf("test expects sliders to be clipped by a short popup, layout=%+v", lay)
	}
	for x := lay.rect.X + 1; x < lay.rect.Right(); x++ {
		if got := desktop.App().ReadCell(x, lay.rect.Bottom()).Ch; got != border.Horizontal {
			t.Fatalf("bottom border at x=%d was overwritten by clipped controls: got %q want %q", x, got, border.Horizontal)
		}
	}
}

func TestColorPickerDoesNotOpenWhenColorDisabled(t *testing.T) {
	_, p := setupColorPicker(t, 40, 10, tui.ColorLevelNone, tui.ANSIColor(3))
	p.open()
	if p.IsOpen() {
		t.Fatalf("picker should not open when ColorLevelNone disables colour")
	}
	if len(p.colors) != 0 {
		t.Fatalf("disabled picker should not build a palette, got %d colors", len(p.colors))
	}
}

func TestColorPickerOpensOnKeyboardAndRestoresFocusOnClose(t *testing.T) {
	desktop, p := setupColorPicker(t, 40, 10, tui.ColorLevel16, tui.DefaultColor())
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter})
	if !p.IsOpen() {
		t.Fatalf("Enter on focused picker should open popup")
	}
	if desktop.TopLayer() != p.popup {
		t.Fatalf("popup layer should be on top after opening")
	}
	if !p.popup.Root.Focused() {
		t.Fatalf("popup catcher should receive focus")
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyEscape})
	if p.IsOpen() {
		t.Fatalf("Escape through desktop should close popup")
	}
	if !p.Component.Focused() {
		t.Fatalf("focus should return to picker after popup closes")
	}
}

func TestColorPickerKeyboardNavigationEnterAndEscape(t *testing.T) {
	_, p := setupColorPicker(t, 80, 24, tui.ColorLevel16, tui.DefaultColor())
	var got []tui.Color
	p.OnChange = func(c tui.Color) { got = append(got, c) }
	p.open()

	p.popupType(nil, tui.TypeEvent{Key: tui.KeyRight})
	if p.highlight != 1 {
		t.Fatalf("Right should move from default to first ANSI cell, highlight=%d", p.highlight)
	}
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyDown})
	if p.highlight != 9 {
		t.Fatalf("Down should move by one row of 8 columns, highlight=%d", p.highlight)
	}
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyHome})
	if p.highlight != 0 {
		t.Fatalf("Home should move to first cell, highlight=%d", p.highlight)
	}
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyEnd})
	if p.highlight != 16 {
		t.Fatalf("End should move to last cell, highlight=%d", p.highlight)
	}
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyEnter})
	if p.IsOpen() {
		t.Fatalf("Enter should close the popup after commit")
	}
	if len(got) != 1 || got[0] != tui.ANSIColor(15) {
		t.Fatalf("Enter delivered %+v, want one ANSI 15 callback", got)
	}

	p.open()
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyHome})
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyEscape})
	if p.IsOpen() {
		t.Fatalf("Escape should close the popup")
	}
	if p.GetColor() != tui.ANSIColor(15) {
		t.Fatalf("Escape should not change current color, got %+v", p.GetColor())
	}
	if len(got) != 1 {
		t.Fatalf("Escape should not fire OnChange, got %d callbacks", len(got))
	}
}

func TestColorPickerPagingKeepsHighlightVisible(t *testing.T) {
	_, p := setupColorPicker(t, 80, 10, tui.ColorLevel256, tui.DefaultColor())
	p.open()
	lay := p.layout()
	if lay.visRows < 1 || !lay.scrollbar {
		t.Fatalf("test expects a short, scrollable 256-color popup, layout=%+v", lay)
	}

	p.popupType(nil, tui.TypeEvent{Key: tui.KeyPageDown})
	want := p.cols * lay.visRows
	if p.highlight != want {
		t.Fatalf("PageDown highlight = %d, want %d", p.highlight, want)
	}
	if row := p.highlight / p.cols; row < p.offset || row >= p.offset+lay.visRows {
		t.Fatalf("highlight row %d should be visible in offset %d visRows %d", row, p.offset, lay.visRows)
	}

	p.popupType(nil, tui.TypeEvent{Key: tui.KeyEnd})
	if p.highlight != len(p.colors)-1 {
		t.Fatalf("End highlight = %d, want last index %d", p.highlight, len(p.colors)-1)
	}
	if row := p.highlight / p.cols; row < p.offset || row >= p.offset+p.layout().visRows {
		t.Fatalf("last highlight row %d should be visible in offset %d visRows %d", row, p.offset, p.layout().visRows)
	}
}

func TestColorPickerRGBSlidersOnlyOnTrueColorAndCommitRGB(t *testing.T) {
	_, p := setupColorPicker(t, 80, 24, tui.ColorLevelTrueColor, tui.RGBColor(10, 20, 30))
	var got tui.Color
	var calls int
	p.OnChange = func(c tui.Color) {
		got = c
		calls++
	}
	p.open()
	if p.section != sectionSliders {
		t.Fatalf("arbitrary RGB seed should open in slider section, section=%d", p.section)
	}
	if p.rgb != [3]uint8{10, 20, 30} {
		t.Fatalf("slider seed = %v, want [10 20 30]", p.rgb)
	}

	p.popupType(nil, tui.TypeEvent{Key: tui.KeyEnd})  // R = 255
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyDown}) // G
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyHome}) // G = 0
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyDown}) // B
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyPageUp})
	p.popupType(nil, tui.TypeEvent{Key: tui.KeyEnter})

	want := tui.RGBColor(255, 0, 46)
	if calls != 1 || got != want {
		t.Fatalf("slider commit delivered calls=%d color=%+v, want one %+v", calls, got, want)
	}
	if p.GetColor() != want {
		t.Fatalf("picker color = %+v, want %+v", p.GetColor(), want)
	}
}

func TestColorPickerClickingSliderUpdatesPreviewWithoutCommitting(t *testing.T) {
	desktop, p := setupColorPicker(t, 80, 24, tui.ColorLevelTrueColor, tui.RGBColor(0, 0, 0))
	var calls int
	p.OnChange = func(tui.Color) { calls++ }
	p.open()
	lay := p.layout()

	p.popupClick(nil, tui.ClickEvent{
		X:      lay.sliders.X + 3 + lay.sliderBarW - 1,
		Y:      lay.sliders.Y + 1,
		Button: tui.MouseLeft,
		Down:   false,
	})
	if p.section != sectionSliders || p.channel != 1 {
		t.Fatalf("clicking G slider should focus slider section/channel 1, section=%d channel=%d", p.section, p.channel)
	}
	if p.rgb[1] != 255 {
		t.Fatalf("clicking slider end should set channel to 255, got %d", p.rgb[1])
	}
	if calls != 0 {
		t.Fatalf("slider drag/click should update preview only, got %d callbacks before commit", calls)
	}
	desktop.Redraw()
	if got := readRunes(desktop.App(), lay.preview.X+3, lay.preview.Y, 7); got != "#00ff00" {
		t.Fatalf("preview after slider click = %q, want #00ff00", got)
	}
}

func TestColorPickerTabDoesNotEnterSlidersOnLowerColorLevels(t *testing.T) {
	for _, level := range []tui.ColorLevel{tui.ColorLevel16, tui.ColorLevel256} {
		t.Run(string(rune('0'+level)), func(t *testing.T) {
			_, p := setupColorPicker(t, 80, 24, level, tui.DefaultColor())
			p.open()
			p.popupType(nil, tui.TypeEvent{Key: tui.KeyTab})
			if p.section != sectionGrid {
				t.Fatalf("Tab should stay in grid without truecolor sliders, section=%d", p.section)
			}
		})
	}
}

func TestColorPickerMouseClickSelectsSwatchAndOutsideDismisses(t *testing.T) {
	_, p := setupColorPicker(t, 80, 24, tui.ColorLevel16, tui.DefaultColor())
	var got []tui.Color
	p.OnChange = func(c tui.Color) { got = append(got, c) }
	p.open()
	lay := p.layout()

	// Row 1, col 2 is flat palette index 10, i.e. ANSI 9 because index 0 is default.
	x := lay.grid.X + 2*colorSwatchW
	y := lay.grid.Y + 1
	p.popupClick(nil, tui.ClickEvent{X: x, Y: y, Button: tui.MouseLeft, Down: true})
	if !p.IsOpen() {
		t.Fatalf("mouse down on swatch should not commit yet")
	}
	p.popupClick(nil, tui.ClickEvent{X: x, Y: y, Button: tui.MouseLeft, Down: false})
	if p.IsOpen() {
		t.Fatalf("mouse release on swatch should close after commit")
	}
	if len(got) != 1 || got[0] != tui.ANSIColor(9) {
		t.Fatalf("swatch click delivered %+v, want one ANSI 9 callback", got)
	}

	p.open()
	p.popupClick(nil, tui.ClickEvent{X: 79, Y: 23, Button: tui.MouseLeft, Down: false})
	if p.IsOpen() {
		t.Fatalf("click outside popup should dismiss")
	}
	if len(got) != 1 {
		t.Fatalf("outside click should not fire OnChange, got %d callbacks", len(got))
	}
}

func TestColorPickerMouseWheelScrollsLongPalette(t *testing.T) {
	_, p := setupColorPicker(t, 80, 10, tui.ColorLevel256, tui.DefaultColor())
	p.open()
	if !p.layout().scrollbar {
		t.Fatalf("test expects 256-color popup to need scrolling")
	}
	p.popupScroll(nil, tui.ScrollEvent{Delta: -1})
	if p.offset != 1 {
		t.Fatalf("wheel down offset = %d, want 1", p.offset)
	}
	p.popupScroll(nil, tui.ScrollEvent{Delta: 1})
	if p.offset != 0 {
		t.Fatalf("wheel up offset = %d, want 0", p.offset)
	}
}

func TestColorPickerClosedSwatchAndLivePreviewRenderCurrentSelection(t *testing.T) {
	desktop, p := setupColorPicker(t, 80, 24, tui.ColorLevelTrueColor, tui.RGBColor(1, 2, 3))
	desktop.Redraw()
	if got := desktop.App().ReadCell(2, 1).BG; got != tui.RGBColor(1, 2, 3) {
		t.Fatalf("closed swatch BG = %+v, want RGB(1,2,3)", got)
	}

	p.open()
	desktop.Redraw()
	lay := p.layout()
	if got := readRunes(desktop.App(), lay.preview.X+3, lay.preview.Y, 7); got != "#010203" {
		t.Fatalf("preview label = %q, want #010203", got)
	}
}
