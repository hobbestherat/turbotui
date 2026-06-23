package tv

import (
	"fmt"
	"strconv"

	tui "github.com/hobbestherat/turbotui"
)

// colorSwatchW is the cell width of one palette swatch in the open grid. Two
// columns make the colour legible without making a 16-wide 256-colour grid too
// large to fit a typical terminal.
const colorSwatchW = 2

// ColorPicker is a colour swatch that, when activated, opens a popup letting the
// user pick a colour visually: a palette grid (level-aware for the terminal's
// detected ColorLevel) plus, on truecolor terminals, RGB sliders with a live
// preview. It is a peer of Select and mirrors its popup-layer mechanism: the
// popup lives on a desktop-owned input layer so it is never clipped by the host
// window, flips above the control when there is no room below, and dismisses on
// an outside click or Escape. On commit it calls OnChange with the chosen
// tui.Color.
//
// Level awareness (read from tui.GetColorLevel() at open time):
//   - ColorLevelNone:      open() is a no-op — colour is off, nothing to pick.
//   - ColorLevel16:        the 16 ANSI colours + a "default" cell, no sliders.
//   - ColorLevel256:       the full 256-colour palette + "default", no sliders
//     (arbitrary RGB would be silently quantised, so it is not offered).
//   - ColorLevelTrueColor: the 256 grid + "default" AND R/G/B sliders.
//
// Keyboard: Enter/Space opens; in the grid the arrows move the highlight (2-D),
// Home/End jump to the ends and PageUp/PageDown page; Tab switches to the RGB
// sliders (truecolor only) where Up/Down pick a channel and Left/Right adjust
// it; Enter commits and Escape cancels. Mouse: click a swatch to pick, click a
// slider to set it, wheel to scroll a long palette, click outside to dismiss.
type ColorPicker struct {
	Component *VisualComponent
	// Color is the current value, delivered to OnChange on commit.
	Color    tui.Color
	OnChange func(tui.Color)
	// FG/BG and FocusFG/FocusBG are the closed-swatch chrome colours (the open
	// popup uses the active theme's Dialog/Selection slots, like Select).
	FG      tui.Color
	BG      tui.Color
	FocusFG tui.Color
	FocusBG tui.Color
	// Shadow draws a drop shadow under the opened popup. Defaults to true; set it
	// false for a flat (no-shadow) theme, mirroring Select.Shadow.
	Shadow bool

	desktop *Desktop
	popup   *Layer

	// level, colors and cols are captured at open() so the grid and slider
	// availability reflect the colour level the popup was opened under.
	level  tui.ColorLevel
	colors []tui.Color
	cols   int

	// highlight is the flat index into colors of the active swatch; offset is the
	// index of the first visible grid row while the popup is open.
	highlight int
	offset    int

	// section selects which part of the popup the keyboard drives; rgb/channel
	// hold the RGB-slider state (only meaningful when the level offers sliders).
	section pickerSection
	rgb     [3]uint8
	channel int
}

// pickerSection is which region of the open popup keyboard input drives.
type pickerSection uint8

const (
	sectionGrid pickerSection = iota
	sectionSliders
)

// NewColorPicker builds a ColorPicker at bounds, seeding the closed-swatch chrome
// from the active theme's Input slots (matching Select) and enabling the popup
// shadow by default.
func NewColorPicker(desktop *Desktop, bounds Rect) *ColorPicker {
	p := &ColorPicker{
		Color:   tui.DefaultColor(),
		FG:      activeTheme.InputFG,
		BG:      activeTheme.InputBG,
		FocusFG: activeTheme.InputFocusFG,
		FocusBG: activeTheme.InputFocusBG,
		Shadow:  true,
		desktop: desktop,
	}
	p.Component = NewComponent(bounds)
	p.Component.Focusable = true
	p.Component.DrawFn = p.draw
	p.Component.OnTypeFn = p.handleType
	p.Component.OnClickFn = p.handleClick
	return p
}

func (p *ColorPicker) Root() *VisualComponent {
	return p.Component
}

// GetColor returns the current value.
func (p *ColorPicker) GetColor() tui.Color {
	return p.Color
}

// SetColor sets the current value without firing OnChange (the programmatic
// counterpart to a user commit).
func (p *ColorPicker) SetColor(c tui.Color) {
	p.Color = c
}

// IsOpen reports whether the picker popup is currently shown.
func (p *ColorPicker) IsOpen() bool {
	return p.popup != nil
}

// hasSliders reports whether the level captured at open offers RGB sliders. Only
// truecolor does: on 256 colour an arbitrary RGB is quantised to the nearest
// palette index, so the sliders would mislead and the 256 grid is offered alone.
func (p *ColorPicker) hasSliders() bool {
	return p.level == tui.ColorLevelTrueColor
}

// colorPickerPalette is the level-aware list of selectable colours: index 0 is
// the terminal default, followed by the ANSI indices the level can show (16 or
// 256). It returns nil for ColorLevelNone (colour is off — nothing to pick).
func colorPickerPalette(level tui.ColorLevel) []tui.Color {
	var n int
	switch level {
	case tui.ColorLevel16:
		n = 16
	case tui.ColorLevel256, tui.ColorLevelTrueColor:
		n = 256
	default:
		return nil
	}
	out := make([]tui.Color, 0, n+1)
	out = append(out, tui.DefaultColor())
	for i := 0; i < n; i++ {
		out = append(out, tui.ANSIColor(uint8(i)))
	}
	return out
}

// gridColumns is the swatch-grid column count for a level: a compact 8-wide grid
// for the 16 ANSI colours, 16 wide for the 256-colour palette.
func gridColumns(level tui.ColorLevel) int {
	if level == tui.ColorLevel16 {
		return 8
	}
	return 16
}

// indexOf returns the palette index of c (matching default and ANSI entries), or
// -1 when c is not a palette colour (e.g. an arbitrary RGB seed).
func (p *ColorPicker) indexOf(c tui.Color) int {
	for i, pc := range p.colors {
		if pc == c {
			return i
		}
	}
	return -1
}

// ---- closed state -----------------------------------------------------------

func (p *ColorPicker) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	fg, bg := focusColors(component.Focused(), p.FG, p.BG, p.FocusFG, p.FocusBG)
	chrome := tui.Cell{Ch: ' ', FG: fg, BG: bg}
	surface.Fill(abs, chrome)
	// Reserve the last column for the open marker; the rest shows the colour.
	swatchW := abs.W - 1
	if swatchW < 1 {
		swatchW = abs.W
	}
	if p.Color.Mode == tui.ColorDefault {
		surface.WriteStringClipped(abs.X, abs.Y, swatchW, "default", chrome)
	} else {
		surface.Fill(Rect{X: abs.X, Y: abs.Y, W: swatchW, H: abs.H}, tui.Cell{Ch: ' ', BG: p.Color})
	}
	marker := '▾'
	if p.popup != nil {
		marker = '▴'
	}
	surface.SetCell(abs.Right(), abs.Y, tui.Cell{Ch: marker, FG: fg, BG: bg, Bold: true})
}

func (p *ColorPicker) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyEnter:
		p.open()
		return true
	case tui.KeyRune:
		if event.Rune == ' ' {
			p.open()
			return true
		}
	}
	return false
}

func (p *ColorPicker) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	if event.Down || !component.AbsoluteBounds().Contains(event.X, event.Y) {
		return true
	}
	if p.popup != nil {
		p.close()
	} else {
		p.open()
	}
	return true
}

// ---- open / close / commit --------------------------------------------------

func (p *ColorPicker) open() {
	if p.popup != nil {
		return
	}
	p.level = tui.GetColorLevel()
	p.colors = colorPickerPalette(p.level)
	if len(p.colors) == 0 {
		// ColorLevelNone: colour is off, so there is nothing to pick.
		return
	}
	p.cols = gridColumns(p.level)

	// Seed the cursor from the current value: land on its palette cell when it is
	// one, otherwise (an arbitrary RGB) open straight into the sliders.
	p.highlight = p.indexOf(p.Color)
	p.section = sectionGrid
	if p.highlight < 0 {
		p.highlight = 0
		if p.Color.Mode == tui.ColorRGB && p.hasSliders() {
			p.section = sectionSliders
		}
	}
	if p.Color.Mode == tui.ColorRGB {
		p.rgb = [3]uint8{
			uint8(p.Color.Value >> 16),
			uint8(p.Color.Value >> 8),
			uint8(p.Color.Value),
		}
	} else {
		p.rgb = [3]uint8{128, 128, 128}
	}
	p.channel = 0
	p.offset = 0
	p.ensureVisible()

	catcher := NewComponent(Rect{X: 0, Y: 0, W: p.desktop.App().Width(), H: p.desktop.App().Height()})
	catcher.Focusable = true
	catcher.DrawFn = p.drawPopup
	catcher.OnTypeFn = p.popupType
	catcher.OnClickFn = p.popupClick
	catcher.OnScrollFn = p.popupScroll
	p.popup = NewLayer("color-picker-popup", catcher, true, false)
	p.desktop.AddLayer(p.popup)
	p.desktop.SetFocus(catcher)
}

func (p *ColorPicker) close() {
	if p.popup == nil {
		return
	}
	layer := p.popup
	p.popup = nil
	p.desktop.RemoveLayer(layer)
	p.desktop.SetFocus(p.Component)
}

// commit sets the value, fires OnChange when it changed, and closes the popup.
func (p *ColorPicker) commit(c tui.Color) {
	changed := c != p.Color
	p.Color = c
	if changed && p.OnChange != nil {
		p.OnChange(c)
	}
	p.close()
}

// currentColor is the colour the highlight/sliders point at — what Enter commits
// and the preview shows.
func (p *ColorPicker) currentColor() tui.Color {
	if p.section == sectionSliders {
		return tui.RGBColor(p.rgb[0], p.rgb[1], p.rgb[2])
	}
	if p.highlight >= 0 && p.highlight < len(p.colors) {
		return p.colors[p.highlight]
	}
	return tui.DefaultColor()
}

// ---- geometry ---------------------------------------------------------------

// gridRowCount is the number of rows the palette occupies at the current column
// count.
func (p *ColorPicker) gridRowCount() int {
	if p.cols < 1 {
		return 0
	}
	return (len(p.colors) + p.cols - 1) / p.cols
}

// reservesScrollbar reports whether the popup keeps a column for the grid
// scrollbar. The 256-colour palette is tall enough to scroll on a normal
// terminal; the 16-colour grid never does.
func (p *ColorPicker) reservesScrollbar() bool {
	return p.level == tui.ColorLevel256 || p.level == tui.ColorLevelTrueColor
}

// fixedRows is the number of non-grid content rows pinned to the bottom of the
// popup: the live-preview row plus, on truecolor, a gap and the three sliders.
func (p *ColorPicker) fixedRows() int {
	rows := 1 // preview
	if p.hasSliders() {
		rows += 1 + 3 // separator gap + R/G/B
	}
	return rows
}

// pickerLayout holds the resolved popup rectangles for one frame, so drawPopup,
// popupClick and the scroll helpers all agree on where everything is.
type pickerLayout struct {
	rect       Rect // outer box including the border
	inner      Rect // inside the border
	grid       Rect // visible swatch area
	visRows    int  // visible grid rows
	sliders    Rect // R/G/B rows (H==0 when the level has none)
	preview    Rect // live-preview row
	sliderBarW int  // width of a slider's [..] bar
	scrollbar  bool // whether the grid shows a scrollbar this frame
}

// popupRect is the popup box anchored below (or flipped above) the swatch, sized
// to the grid plus the fixed rows and clamped to the screen — modelled on
// Select.popupRect.
func (p *ColorPicker) popupRect() Rect {
	abs := p.Component.AbsoluteBounds()
	screenW := p.desktop.App().Width()
	screenH := p.desktop.App().Height()

	natural := p.gridRowCount() + p.fixedRows() + 2 // + borders
	spaceBelow := screenH - (abs.Y + 1)
	spaceAbove := abs.Y
	flipUp := natural > spaceBelow && spaceAbove > spaceBelow
	height := natural
	y := abs.Y + 1
	if flipUp {
		if height > spaceAbove {
			height = spaceAbove
		}
		y = abs.Y - height
	} else if height > spaceBelow {
		height = spaceBelow
	}
	if height < 3 {
		height = 3
	}
	if y < 0 {
		y = 0
	}

	width := p.cols*colorSwatchW + 2 // grid + borders
	if p.reservesScrollbar() {
		width++ // scrollbar column
	}
	if width > screenW {
		width = screenW
	}
	x := abs.X
	if x+width > screenW {
		x = screenW - width
	}
	if x < 0 {
		x = 0
	}
	return Rect{X: x, Y: y, W: width, H: height}
}

// layout resolves every popup rectangle from popupRect and the current state.
func (p *ColorPicker) layout() pickerLayout {
	rect := p.popupRect()
	inner := Rect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}

	gridW := inner.W
	scrollbar := p.reservesScrollbar() && inner.W > 1
	if scrollbar {
		gridW = inner.W - 1
	}

	visRows := inner.H - p.fixedRows()
	if visRows < 0 {
		visRows = 0
	}
	if rows := p.gridRowCount(); visRows > rows {
		visRows = rows
	}
	// A scrollbar is only meaningful while the grid actually scrolls.
	scrollbar = scrollbar && p.gridRowCount() > visRows

	grid := Rect{X: inner.X, Y: inner.Y, W: gridW, H: visRows}
	yy := inner.Y + visRows
	var sliders Rect
	if p.hasSliders() {
		yy++ // separator gap
		sliders = Rect{X: inner.X, Y: yy, W: inner.W, H: 3}
		yy += 3
	}
	preview := Rect{X: inner.X, Y: yy, W: inner.W, H: 1}

	barW := inner.W - sliderFixedW
	if barW < 1 {
		barW = 1
	}
	return pickerLayout{
		rect:       rect,
		inner:      inner,
		grid:       grid,
		visRows:    visRows,
		sliders:    sliders,
		preview:    preview,
		sliderBarW: barW,
		scrollbar:  scrollbar,
	}
}

// sliderFixedW is the non-bar width of a slider row: "R " + "[" + "]" + " 255".
const sliderFixedW = 2 + 1 + 1 + 4

func (p *ColorPicker) clampOffset() {
	max := scrollbarMaxOffset(p.gridRowCount(), p.layout().visRows)
	if p.offset < 0 {
		p.offset = 0
	} else if p.offset > max {
		p.offset = max
	}
}

// ensureVisible scrolls the minimum amount so the highlighted swatch's row is on
// screen after a keyboard move.
func (p *ColorPicker) ensureVisible() {
	vis := p.layout().visRows
	if vis < 1 || p.cols < 1 {
		return
	}
	row := p.highlight / p.cols
	if row < p.offset {
		p.offset = row
	} else if row >= p.offset+vis {
		p.offset = row - vis + 1
	}
	p.clampOffset()
}

// ---- drawing ----------------------------------------------------------------

func (p *ColorPicker) drawPopup(_ *VisualComponent, surface Surface) {
	lay := p.layout()
	if p.Shadow {
		// The shadow is cast outside the box, so paint it before clipping.
		surface.DrawShadow(lay.rect, activeTheme.WindowShadow, DefaultShadowStyle)
	}
	// Confine box content to the popup rect: on a narrow/short terminal the
	// width/height clamp in popupRect can leave the grid's configured columns or
	// the slider+preview rows reaching past the box, and the catcher is a
	// full-screen layer (its surface clips to the screen, not the box), so without
	// this they would overdraw neighbouring UI. Each region is then clipped to its
	// own rect so a swatch can never bleed over the border or scrollbar column.
	box := surface.WithClip(lay.rect)
	box.Fill(lay.rect, tui.Cell{Ch: ' ', FG: activeTheme.DialogFG, BG: activeTheme.DialogBG})
	box.DrawBox(lay.rect, tui.LineSingle, activeTheme.DialogBorderFG, activeTheme.DialogBG)

	p.drawGrid(box, lay)
	if lay.sliders.H > 0 {
		p.drawSliders(box.WithClip(lay.sliders), lay)
	}
	p.drawPreview(box.WithClip(lay.preview), lay.preview)
}

func (p *ColorPicker) drawGrid(surface Surface, lay pickerLayout) {
	cells := surface.WithClip(lay.grid)
	for row := 0; row < lay.visRows; row++ {
		gridRow := p.offset + row
		for col := 0; col < p.cols; col++ {
			index := gridRow*p.cols + col
			if index >= len(p.colors) {
				break
			}
			cellX := lay.grid.X + col*colorSwatchW
			cellRect := Rect{X: cellX, Y: lay.grid.Y + row, W: colorSwatchW, H: 1}
			p.drawSwatch(cells, cellRect, p.colors[index], index == p.highlight && p.section == sectionGrid)
		}
	}
	if lay.scrollbar {
		track := Rect{X: lay.inner.X + lay.inner.W - 1, Y: lay.grid.Y, W: 1, H: lay.visRows}
		drawVScrollbar(surface, track, p.gridRowCount(), lay.visRows, p.offset,
			activeTheme.DialogBorderFG, activeTheme.DialogBG, true)
	}
}

// drawSwatch paints one palette cell: the colour as a fill (or a "·" marker on
// the dialog background for the terminal-default cell), with a selection-coloured
// frame on the highlighted cell.
func (p *ColorPicker) drawSwatch(surface Surface, rect Rect, c tui.Color, highlighted bool) {
	if c.Mode == tui.ColorDefault {
		fg, bg := activeTheme.DialogFG, activeTheme.DialogBG
		if highlighted {
			fg, bg = activeTheme.SelectionFG, activeTheme.SelectionBG
		}
		surface.Fill(rect, tui.Cell{Ch: '·', FG: fg, BG: bg})
		return
	}
	if highlighted {
		// Frame the colour in selection-bg so the cursor reads against any hue.
		surface.Fill(rect, tui.Cell{Ch: '▉', FG: c, BG: activeTheme.SelectionBG})
		return
	}
	surface.Fill(rect, tui.Cell{Ch: ' ', BG: c})
}

func (p *ColorPicker) drawSliders(surface Surface, lay pickerLayout) {
	labels := [3]rune{'R', 'G', 'B'}
	dialog := tui.Cell{FG: activeTheme.DialogFG, BG: activeTheme.DialogBG}
	for ch := 0; ch < 3; ch++ {
		y := lay.sliders.Y + ch
		active := p.section == sectionSliders && p.channel == ch
		lblFG, lblBG := activeTheme.DialogFG, activeTheme.DialogBG
		if active {
			lblFG, lblBG = activeTheme.SelectionFG, activeTheme.SelectionBG
		}
		x := lay.sliders.X
		surface.SetCell(x, y, tui.Cell{Ch: labels[ch], FG: lblFG, BG: lblBG, Bold: true})
		surface.SetCell(x+1, y, tui.Cell{Ch: ' ', FG: lblFG, BG: lblBG})
		barX := x + 2
		surface.SetCell(barX, y, tui.Cell{Ch: '[', FG: dialog.FG, BG: dialog.BG})
		filled := int(p.rgb[ch]) * lay.sliderBarW / 255
		for i := 0; i < lay.sliderBarW; i++ {
			glyph := '░'
			if i < filled {
				glyph = '█'
			}
			surface.SetCell(barX+1+i, y, tui.Cell{Ch: glyph, FG: dialog.FG, BG: dialog.BG})
		}
		surface.SetCell(barX+1+lay.sliderBarW, y, tui.Cell{Ch: ']', FG: dialog.FG, BG: dialog.BG})
		surface.WriteString(barX+lay.sliderBarW+3, y, fmt.Sprintf("%3d", p.rgb[ch]), dialog)
	}
}

func (p *ColorPicker) drawPreview(surface Surface, row Rect) {
	c := p.currentColor()
	// Two colour blocks in the selected colour, then its canonical label.
	surface.WriteString(row.X, row.Y, "▉▉", tui.Cell{FG: c, BG: activeTheme.DialogBG})
	surface.WriteStringClipped(row.X+3, row.Y, row.W-3, colorPickerLabel(c),
		tui.Cell{FG: activeTheme.DialogFG, BG: activeTheme.DialogBG})
}

// colorPickerLabel renders a colour to a short human label for the preview row.
func colorPickerLabel(c tui.Color) string {
	switch c.Mode {
	case tui.ColorANSI:
		return "ansi " + strconv.Itoa(int(c.Value))
	case tui.ColorRGB:
		r := (c.Value >> 16) & 0xff
		g := (c.Value >> 8) & 0xff
		b := c.Value & 0xff
		return fmt.Sprintf("#%02x%02x%02x", r, g, b)
	default:
		return "default"
	}
}

// ---- popup input ------------------------------------------------------------

func (p *ColorPicker) popupType(_ *VisualComponent, event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyEscape:
		p.close()
	case tui.KeyTab, tui.KeyBackTab:
		p.toggleSection()
	case tui.KeyEnter:
		p.commit(p.currentColor())
	case tui.KeyRune:
		if event.Rune == ' ' {
			p.commit(p.currentColor())
		}
	default:
		if p.section == sectionSliders {
			p.sliderKey(event)
		} else {
			p.gridKey(event)
		}
	}
	return true
}

// toggleSection flips between the palette grid and the RGB sliders. It is a no-op
// when the level offers no sliders, so the grid stays the only section.
func (p *ColorPicker) toggleSection() {
	if !p.hasSliders() {
		return
	}
	if p.section == sectionGrid {
		p.section = sectionSliders
	} else {
		p.section = sectionGrid
	}
}

func (p *ColorPicker) gridKey(event tui.TypeEvent) {
	n := len(p.colors)
	if n == 0 {
		return
	}
	switch event.Key {
	case tui.KeyLeft:
		if p.highlight > 0 {
			p.highlight--
		}
	case tui.KeyRight:
		if p.highlight < n-1 {
			p.highlight++
		}
	case tui.KeyUp:
		if p.highlight-p.cols >= 0 {
			p.highlight -= p.cols
		}
	case tui.KeyDown:
		if p.highlight+p.cols < n {
			p.highlight += p.cols
		}
	case tui.KeyHome:
		p.highlight = 0
	case tui.KeyEnd:
		p.highlight = n - 1
	case tui.KeyPageUp:
		p.highlight = clampInt(p.highlight-p.cols*p.layout().visRows, 0, n-1)
	case tui.KeyPageDown:
		p.highlight = clampInt(p.highlight+p.cols*p.layout().visRows, 0, n-1)
	default:
		return
	}
	p.ensureVisible()
}

func (p *ColorPicker) sliderKey(event tui.TypeEvent) {
	switch event.Key {
	case tui.KeyUp:
		if p.channel > 0 {
			p.channel--
		}
	case tui.KeyDown:
		if p.channel < 2 {
			p.channel++
		}
	case tui.KeyLeft:
		p.adjustChannel(-1)
	case tui.KeyRight:
		p.adjustChannel(1)
	case tui.KeyPageUp:
		p.adjustChannel(16)
	case tui.KeyPageDown:
		p.adjustChannel(-16)
	case tui.KeyHome:
		p.rgb[p.channel] = 0
	case tui.KeyEnd:
		p.rgb[p.channel] = 255
	}
}

// adjustChannel nudges the active RGB channel by delta, clamped to 0..255.
func (p *ColorPicker) adjustChannel(delta int) {
	p.rgb[p.channel] = uint8(clampInt(int(p.rgb[p.channel])+delta, 0, 255))
}

func (p *ColorPicker) popupScroll(_ *VisualComponent, event tui.ScrollEvent) bool {
	if p.popup == nil {
		return false
	}
	// Delta is +1 for wheel-up and -1 for wheel-down, so subtracting scrolls the
	// grid in the natural direction.
	p.offset -= event.Delta
	p.clampOffset()
	return true
}

func (p *ColorPicker) popupClick(_ *VisualComponent, event tui.ClickEvent) bool {
	lay := p.layout()
	// Route presses/drags on the grid scrollbar to scrolling, not picking.
	if lay.scrollbar {
		track := Rect{X: lay.inner.X + lay.inner.W - 1, Y: lay.grid.Y, W: 1, H: lay.visRows}
		if event.X == track.X && event.Y >= track.Y && event.Y <= track.Bottom() {
			if off, ok := scrollbarOffsetForY(track, p.gridRowCount(), lay.visRows, p.offset, event.Y); ok {
				p.offset = off
			}
			return true
		}
	}
	if event.Down {
		return true
	}
	if lay.grid.Contains(event.X, event.Y) {
		col := (event.X - lay.grid.X) / colorSwatchW
		row := p.offset + (event.Y - lay.grid.Y)
		index := row*p.cols + col
		if col >= 0 && col < p.cols && index >= 0 && index < len(p.colors) {
			p.commit(p.colors[index])
		}
		return true
	}
	if lay.sliders.H > 0 && lay.sliders.Contains(event.X, event.Y) {
		p.clickSlider(event, lay)
		return true
	}
	if lay.rect.Contains(event.X, event.Y) {
		return true // inside the chrome but not a hot region — ignore
	}
	p.close() // outside click dismisses
	return true
}

// clickSlider focuses the clicked channel and sets its value from the pointer's
// position along the bar.
func (p *ColorPicker) clickSlider(event tui.ClickEvent, lay pickerLayout) {
	p.section = sectionSliders
	p.channel = event.Y - lay.sliders.Y
	barX := lay.sliders.X + 3 // "R [" before the first bar cell
	rel := event.X - barX
	if rel < 0 {
		rel = 0
	}
	if rel > lay.sliderBarW-1 {
		rel = lay.sliderBarW - 1
	}
	if lay.sliderBarW > 1 {
		p.rgb[p.channel] = uint8(rel * 255 / (lay.sliderBarW - 1))
	} else {
		p.rgb[p.channel] = 255
	}
}
