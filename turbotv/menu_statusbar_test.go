package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Tests for the MenuBar right-anchored status slot + right-aligned top-level
// menus (issue #500, turbotui half). These exercise the four design gates:
// goal match (status slot + right-aligned menus, both generic), usability
// (clicks/mnemonics for right items, submenus on-screen, colours), no
// regressions (byte-for-byte backward-compat), and holistic (purely generic,
// no gogent coupling).
//
// Coordinate facts the assertions rely on:
//   - Rect.Right() is the INCLUSIVE last column: a W-wide bar at X=0 has
//     Right()==W-1 and valid columns 0..W-1 (rect.go).
//   - compose() sets the bar's bounds to {0,0,appW,1} every frame, so after a
//     desktop.Redraw() the bar spans the full width and abs.Right()==W-1.
//   - topItemWidth = len([]rune(parseMnemonic(label))) + 2, so "&File"=6,
//     "&Edit"=6, "&View"=6, "&Help"=6, "&Daemon"=8.
//   - Display widths: ●/○/◐ = 1, a CJK ideograph = 2, "…" = 1.

// renderBar installs bar on a fresh W-wide desktop and composes once, returning
// the desktop so the test can read composed cells via desktop.App().ReadCell.
// Height 3 lets us assert that the status never wraps onto row 1.
func renderBar(t *testing.T, w int, bar *MenuBar) *Desktop {
	t.Helper()
	d := newTestDesktop(t, w, 3)
	d.SetMenuBar(bar)
	d.Redraw()
	return d
}

func chAt(d *Desktop, x, y int) rune      { return d.App().ReadCell(x, y).Ch }
func fgAt(d *Desktop, x, y int) tui.Color { return d.App().ReadCell(x, y).FG }
func bgAt(d *Desktop, x, y int) tui.Color { return d.App().ReadCell(x, y).BG }

// labelCellX is the column of a top-level menu's first label glyph (rect.X+1).
func labelCellX(bar *MenuBar, idx int) int { return bar.topRects[idx].X + 1 }

// =====================================================================
// (1) Status slot: flush-right, exactly one pad, measured by display width
// =====================================================================

func TestStatusFlushRightWithOnePad(t *testing.T) {
	const w = 80
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1})
	bar.SetStatus("AB") // display width 2
	d := renderBar(t, w, bar)

	if bar.statusSlotW != 3 { // 2 text + 1 pad
		t.Fatalf("statusSlotW: want 3 (text+pad), got %d", bar.statusSlotW)
	}
	// startX = abs.Right() - StringWidth("AB") = 79 - 2 = 77 → text on 77–78.
	if got := chAt(d, 77, 0); got != 'A' {
		t.Fatalf("col 77: want 'A', got %q", got)
	}
	if got := chAt(d, 78, 0); got != 'B' {
		t.Fatalf("col 78: want 'B', got %q", got)
	}
	// Exactly one pad cell at the bar's last column; the column before the text
	// (76) is blank (no phantom second pad → guards the v1 off-by-one).
	if got := chAt(d, 79, 0); got != ' ' {
		t.Fatalf("col 79 (pad): want ' ', got %q", got)
	}
	if got := chAt(d, 76, 0); got != ' ' {
		t.Fatalf("col 76 (gutter before status): want ' ', got %q", got)
	}
}

func TestStatusNonASCIIGlyphWidth1(t *testing.T) {
	const w = 80
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1})
	bar.SetStatus("● connected") // ● is width 1; full width = 1+1+9 = 11
	d := renderBar(t, w, bar)

	wantW := tui.StringWidth("● connected")
	if bar.statusSlotW != wantW+1 {
		t.Fatalf("statusSlotW: want %d, got %d", wantW+1, bar.statusSlotW)
	}
	startX := (w - 1) - wantW // 79 - 11 = 68
	if got := chAt(d, startX, 0); got != '●' {
		t.Fatalf("start col %d: want '●', got %q", startX, got)
	}
	if got := chAt(d, w-1, 0); got != ' ' {
		t.Fatalf("pad col %d: want ' ', got %q", w-1, got)
	}
}

// TestStatusWideRuneMeasuredByDisplayWidth is the key display-width guard: a
// 2-rune, 4-cell CJK string must reserve 4 text cells (not the rune count 2).
// A rune-count implementation would place the first glyph 2 columns too far right.
func TestStatusWideRuneMeasuredByDisplayWidth(t *testing.T) {
	const w = 80
	const status = "連線" // 2 runes, 4 display cells
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1})
	bar.SetStatus(status)
	d := renderBar(t, w, bar)

	sw := tui.StringWidth(status)
	if sw != 4 {
		t.Fatalf("precondition: StringWidth(%q) want 4, got %d", status, sw)
	}
	if bar.statusSlotW != sw+1 {
		t.Fatalf("statusSlotW: want %d (display-width+pad), got %d — rune-count bug?", sw+1, bar.statusSlotW)
	}
	startX := (w - 1) - sw // 79 - 4 = 75
	// Base cells of the two wide glyphs (their continuation cells are blank by
	// the wide-glyph cell model, so assert only the bases).
	if got := chAt(d, startX, 0); got != '連' {
		t.Fatalf("base col %d: want '連', got %q", startX, got)
	}
	if got := chAt(d, startX+2, 0); got != '線' {
		t.Fatalf("base col %d: want '線', got %q", startX+2, got)
	}
	if got := chAt(d, w-1, 0); got != ' ' {
		t.Fatalf("pad col %d: want ' ', got %q", w-1, got)
	}
}

// Status text is drawn literally via WriteString, NOT mnemonic-parsed: a '&'
// renders as '&' with no hot-key underline.
func TestStatusTextIsLiteralNotMnemonic(t *testing.T) {
	const w = 80
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1})
	bar.SetStatus("C&D") // display width 3
	d := renderBar(t, w, bar)

	startX := (w - 1) - tui.StringWidth("C&D") // 79 - 3 = 76
	if got := chAt(d, startX, 0); got != 'C' {
		t.Fatalf("col %d: want 'C', got %q", startX, got)
	}
	if got := chAt(d, startX+1, 0); got != '&' {
		t.Fatalf("col %d: want literal '&', got %q (status must not be mnemonic-parsed)", startX+1, got)
	}
	if got := chAt(d, startX+2, 0); got != 'D' {
		t.Fatalf("col %d: want 'D', got %q", startX+2, got)
	}
}

// =====================================================================
// (2) Status colours: provided pair applied; zero falls back to bar FG/BG
// =====================================================================

func TestStatusColorsAppliedAndFallback(t *testing.T) {
	const w = 80

	// Explicit colours are painted on every status cell.
	explicit := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1})
	explicit.SetStatus("OK")
	explicit.SetStatusColors(tui.ANSIColor(2), tui.ANSIColor(4)) // green on blue
	de := renderBar(t, w, explicit)
	startX := (w - 1) - tui.StringWidth("OK")
	if got := fgAt(de, startX, 0); got != tui.ANSIColor(2) {
		t.Fatalf("status FG: want ANSI 2, got %+v", got)
	}
	if got := bgAt(de, startX, 0); got != tui.ANSIColor(4) {
		t.Fatalf("status BG: want ANSI 4, got %+v", got)
	}

	// Zero colours fall back to the bar's own FG/BG.
	fb := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1})
	fb.SetStatus("OK")
	df := renderBar(t, w, fb)
	fx := (w - 1) - tui.StringWidth("OK")
	if got := fgAt(df, fx, 0); got != fb.FG {
		t.Fatalf("zero StatusFG should fall back to bar FG %+v, got %+v", fb.FG, got)
	}
	if got := bgAt(df, fx, 0); got != fb.BG {
		t.Fatalf("zero StatusBG should fall back to bar BG %+v, got %+v", fb.BG, got)
	}
}

// =====================================================================
// (3) Right-aligned top menus pack from the right, left of the status slot,
//     with left menus unchanged in position.
// =====================================================================

func TestRightAlignedMenuPacksFromRight(t *testing.T) {
	const w = 80
	file := NewSubMenu("&File", NewMenuItem("&Open", nil))
	edit := NewSubMenu("&Edit", NewMenuItem("&Cut", nil))
	daemon := NewSubMenu("&Daemon", NewMenuItem("&Start", nil)).AlignRight()
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1}, file, edit, daemon)
	bar.SetStatus("●") // width 1 → slot 2; forces Daemon to the left of the slot
	d := renderBar(t, w, bar)

	// Left menus unchanged: File at 0, Edit at 6 (byte-for-byte as today).
	if bar.topRects[0] != (Rect{X: 0, Y: 0, W: 6, H: 1}) {
		t.Fatalf("File rect: want {0 0 6 1}, got %+v", bar.topRects[0])
	}
	if bar.topRects[1] != (Rect{X: 6, Y: 0, W: 6, H: 1}) {
		t.Fatalf("Edit rect: want {6 0 6 1}, got %+v", bar.topRects[1])
	}
	// Daemon (width 8) packs flush to the left of the 2-cell slot:
	// slot owns [78,80); Daemon owns [70,78).
	if bar.topRects[2] != (Rect{X: 70, Y: 0, W: 8, H: 1}) {
		t.Fatalf("Daemon rect: want {70 0 8 1}, got %+v", bar.topRects[2])
	}
	// Status glyph lands at 78 with pad at 79, immediately right of Daemon.
	if got := chAt(d, 78, 0); got != '●' {
		t.Fatalf("status col 78: want '●', got %q", got)
	}
	if got := chAt(d, 79, 0); got != ' ' {
		t.Fatalf("status pad col 79: want ' ', got %q", got)
	}
	// No overlap: gutter between Edit (ends col 11) and Daemon (starts 70) blank.
	for x := 12; x < 70; x++ {
		if got := chAt(d, x, 0); got != ' ' {
			t.Fatalf("gutter col %d should be blank, got %q (overlap?)", x, got)
		}
	}
}

// Multiple right-aligned menus: last-declared packs nearest the slot, no overlap.
func TestMultipleRightAlignedMenusOrder(t *testing.T) {
	const w = 80
	file := NewSubMenu("&File", NewMenuItem("&Open", nil))
	daemon := NewSubMenu("&Daemon", NewMenuItem("&Start", nil)).AlignRight() // width 8
	help := NewSubMenu("&Help", NewMenuItem("&About", nil)).AlignRight()     // width 6
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1}, file, daemon, help)
	d := renderBar(t, w, bar)

	// No status: Help (6) flush right [74,80); Daemon (8) to its left [66,74).
	if bar.topRects[1] != (Rect{X: 66, Y: 0, W: 8, H: 1}) {
		t.Fatalf("Daemon rect: want {66 0 8 1}, got %+v", bar.topRects[1])
	}
	if bar.topRects[2] != (Rect{X: 74, Y: 0, W: 6, H: 1}) {
		t.Fatalf("Help rect (nearest right edge): want {74 0 6 1}, got %+v", bar.topRects[2])
	}
	// Disjoint and ordered: File end < Daemon start < Daemon end < Help start.
	if bar.topRects[0].Right() >= bar.topRects[1].X {
		t.Fatalf("File overlaps Daemon")
	}
	if bar.topRects[1].Right() >= bar.topRects[2].X {
		t.Fatalf("Daemon overlaps Help")
	}
	if bar.statusSlotW != 0 {
		t.Fatalf("no status set: statusSlotW want 0, got %d", bar.statusSlotW)
	}
	// Last-declared (Help) is nearest the right edge.
	if chAt(d, 74+1, 0) != 'H' {
		t.Fatalf("Help label 'H' should be nearest the right edge")
	}
}

// A RightAligned menu in the MIDDLE of the slice: left-packing must skip it and
// keep packing left items adjacently, and mnemonic/index resolution still works.
func TestRightAlignedMenuInMiddleOfSlice(t *testing.T) {
	const w = 80
	file := NewSubMenu("&File", NewMenuItem("&Open", nil))                   // idx0, left
	daemon := NewSubMenu("&Daemon", NewMenuItem("&Start", nil)).AlignRight() // idx1, right
	edit := NewSubMenu("&Edit", NewMenuItem("&Cut", nil))                    // idx2, left
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1}, file, daemon, edit)
	renderBar(t, w, bar)

	// File packs at 0; Edit packs right after File (skipping Daemon), at 6.
	if bar.topRects[0] != (Rect{X: 0, Y: 0, W: 6, H: 1}) {
		t.Fatalf("File rect: got %+v", bar.topRects[0])
	}
	if bar.topRects[2] != (Rect{X: 6, Y: 0, W: 6, H: 1}) {
		t.Fatalf("Edit should pack right after File despite Daemon between them: got %+v", bar.topRects[2])
	}
	// Daemon packs flush right [72,80).
	if bar.topRects[1] != (Rect{X: 72, Y: 0, W: 8, H: 1}) {
		t.Fatalf("Daemon rect: got %+v", bar.topRects[1])
	}
	// Mnemonics resolve by index through the interleaving.
	for _, c := range []struct {
		r   rune
		idx int
	}{
		{'f', 0}, {'d', 1}, {'e', 2},
	} {
		bar.CloseMenus()
		if !bar.OpenTopByMnemonic(c.r) {
			t.Fatalf("OpenTopByMnemonic(%q) should open menu %d", c.r, c.idx)
		}
		if len(bar.openPath) == 0 || bar.openPath[0] != c.idx {
			t.Fatalf("mnemonic %q: want top index %d, got %v", c.r, c.idx, bar.openPath)
		}
	}
}

// =====================================================================
// (4) Narrow-terminal degradation: status truncates with ellipsis, then hides;
//     never wraps to a second row, never overdraws a left item.
// =====================================================================

func TestStatusTruncatesWithEllipsisWhenNarrow(t *testing.T) {
	const w = 6
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1})
	bar.SetStatus("ABCDEFG") // width 7; only 6 cols → slot caps at 6, text at 5
	d := renderBar(t, w, bar)

	if bar.statusSlotW != 6 {
		t.Fatalf("statusSlotW: want 6 (capped), got %d", bar.statusSlotW)
	}
	// textCols = 5 → "ABCD…" (4 chars + ellipsis). startX = 5 - 5 = 0.
	want := []rune("ABCD…")
	for i, r := range want {
		if got := chAt(d, i, 0); got != r {
			t.Fatalf("col %d: want %q, got %q", i, r, got)
		}
	}
	if got := chAt(d, 5, 0); got != ' ' {
		t.Fatalf("pad col 5: want ' ', got %q", got)
	}
	assertRowBlank(t, d, w, 1, "status must not wrap to row 1")
}

func TestStatusHidesWhenNoRoomLeftOfMenus(t *testing.T) {
	// Left menus alone overflow a 10-col bar (File+Edit = 12), so free<=0 and
	// the status slot clamps to 0 → status hidden, left items inviolate.
	const w = 10
	file := NewSubMenu("&File", NewMenuItem("&Open", nil))
	edit := NewSubMenu("&Edit", NewMenuItem("&Cut", nil))
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1}, file, edit)
	bar.SetStatus("●")
	d := renderBar(t, w, bar)

	if bar.statusSlotW != 0 {
		t.Fatalf("statusSlotW: want 0 (no room), got %d", bar.statusSlotW)
	}
	// Left items still render their glyphs (not overwritten).
	if got := chAt(d, 1, 0); got != 'F' {
		t.Fatalf("File 'F' at col 1: want 'F', got %q", got)
	}
	if got := chAt(d, 7, 0); got != 'E' {
		t.Fatalf("Edit 'E' at col 7: want 'E', got %q", got)
	}
	// The status glyph must not appear anywhere on the bar row.
	for x := 0; x < w; x++ {
		if got := chAt(d, x, 0); got == '●' {
			t.Fatalf("status glyph leaked at col %d while statusSlotW==0", x)
		}
	}
	assertRowBlank(t, d, w, 1, "no wrap to row 1")
}

// Status yields to a right-aligned menu: when both cannot fit, the right menu
// keeps its full width and the status truncates (never the reverse).
func TestStatusYieldsToRightMenuWhenTight(t *testing.T) {
	const w = 20
	file := NewSubMenu("&File", NewMenuItem("&Open", nil))                   // 6
	daemon := NewSubMenu("&Daemon", NewMenuItem("&Start", nil)).AlignRight() // 8
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1}, file, daemon)
	bar.SetStatus("Connected") // width 9, wants 10, but only 6 remain → "Conn…"
	d := renderBar(t, w, bar)

	// free = 20 - 6(File) = 14; rightMenusW=8; maxSlot=6 → statusSlotW=6.
	if bar.statusSlotW != 6 {
		t.Fatalf("statusSlotW: want 6 (yielded from 10), got %d", bar.statusSlotW)
	}
	// Daemon keeps its full 8 cells at [6,14).
	if bar.topRects[1] != (Rect{X: 6, Y: 0, W: 8, H: 1}) {
		t.Fatalf("Daemon should keep full width at {6 0 8 1}, got %+v", bar.topRects[1])
	}
	// Status truncated to "Conn…" starting at col 14, pad at 19.
	want := []rune("Conn…")
	for i, r := range want {
		if got := chAt(d, 14+i, 0); got != r {
			t.Fatalf("status col %d: want %q, got %q", 14+i, r, got)
		}
	}
	if got := chAt(d, 19, 0); got != ' ' {
		t.Fatalf("pad col 19: want ' ', got %q", got)
	}
	assertRowBlank(t, d, w, 1, "no wrap to row 1")
}

// assertRowBlank verifies every cell on row y is a space (no second-row draw).
func assertRowBlank(t *testing.T, d *Desktop, w, y int, msg string) {
	t.Helper()
	for x := 0; x < w; x++ {
		if got := chAt(d, x, y); got != ' ' {
			t.Fatalf("%s: col %d row %d want ' ', got %q", msg, x, y, got)
		}
	}
}

// =====================================================================
// (5) Click resolution resolves to the correct top item for BOTH alignments,
//     and a leaf under a right-aligned menu fires on release.
// =====================================================================

func TestClickResolvesLeftAndRightTopMenus(t *testing.T) {
	const w = 80
	fileFired, connectFired := 0, 0
	file := NewSubMenu("&File", NewMenuItem("&Open", func() { fileFired++ }))
	daemon := NewSubMenu("&Daemon", NewMenuItem("&Connect", func() { connectFired++ })).AlignRight()
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1}, file, daemon)
	d := renderBar(t, w, bar)

	// Click the LEFT menu's label cell → opens File (index 0).
	bar.handleClick(bar.Component, down(labelCellX(bar, 0), 0))
	if len(bar.openPath) == 0 || bar.openPath[0] != 0 {
		t.Fatalf("left click should open File (idx 0), got openPath=%v", bar.openPath)
	}
	bar.CloseMenus()

	// Click the RIGHT menu's label cell → opens THAT menu (Daemon, index 1),
	// not a neighbour and not the gutter.
	rightLabel := labelCellX(bar, 1) // 73
	bar.handleClick(bar.Component, down(rightLabel, 0))
	if len(bar.openPath) == 0 || bar.openPath[0] != 1 {
		t.Fatalf("right click should open Daemon (idx 1), got openPath=%v", bar.openPath)
	}

	// Activate a leaf under the right-aligned menu (press then release).
	d.Redraw() // populate popupLayouts for the open Daemon menu
	leaf := bar.popupLayouts[0].itemRects[0]
	bar.handleClick(bar.Component, down(leaf.X, leaf.Y))
	if connectFired != 0 {
		t.Fatalf("leaf must not fire on press, got %d", connectFired)
	}
	bar.handleClick(bar.Component, up(leaf.X, leaf.Y))
	if connectFired != 1 {
		t.Fatalf("right-menu leaf should fire on release, got %d", connectFired)
	}
	if bar.IsOpen() {
		t.Fatalf("menu should close after activating a leaf")
	}

	// Sanity: the File leaf still works too.
	bar.handleClick(bar.Component, down(labelCellX(bar, 0), 0))
	d.Redraw()
	fleaf := bar.popupLayouts[0].itemRects[0]
	bar.handleClick(bar.Component, down(fleaf.X, fleaf.Y))
	bar.handleClick(bar.Component, up(fleaf.X, fleaf.Y))
	if fileFired != 1 {
		t.Fatalf("left-menu leaf should fire on release, got %d", fileFired)
	}
}

// A disabled right-aligned menu cannot be opened by click or mnemonic.
func TestDisabledRightAlignedMenuCannotOpen(t *testing.T) {
	const w = 80
	daemon := NewSubMenu("&Daemon", NewMenuItem("&Start", nil)).AlignRight()
	daemon.Enabled = false
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1}, daemon)
	renderBar(t, w, bar)

	if bar.OpenTopByMnemonic('d') {
		t.Fatalf("disabled right menu should not open via mnemonic")
	}
	if bar.IsOpen() {
		t.Fatalf("disabled right menu must stay closed")
	}
	bar.handleClick(bar.Component, down(labelCellX(bar, 0), 0))
	if bar.IsOpen() {
		t.Fatalf("clicking a disabled right menu must not open it")
	}
}

// =====================================================================
// (6) Mnemonics open right-aligned menus (direct API + desktop routing).
// =====================================================================

func TestOpenTopByMnemonicRightAligned(t *testing.T) {
	const w = 80
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", nil)),
		NewSubMenu("&Daemon", NewMenuItem("&Start", nil)).AlignRight(),
	)
	renderBar(t, w, bar)

	if !bar.OpenTopByMnemonic('d') {
		t.Fatalf("OpenTopByMnemonic('d') should open the right-aligned Daemon")
	}
	if len(bar.openPath) == 0 || bar.openPath[0] != 1 {
		t.Fatalf("mnemonic 'd' should open Daemon (idx 1), got %v", bar.openPath)
	}
	bar.CloseMenus()
	if bar.OpenTopByMnemonic('z') {
		t.Fatalf("OpenTopByMnemonic('z') should find nothing")
	}
}

func TestDesktopRoutesAltMnemonicToRightMenu(t *testing.T) {
	const w = 80
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", nil)),
		NewSubMenu("&Daemon", NewMenuItem("&Start", nil)).AlignRight(),
	)
	d := newTestDesktop(t, w, 3)
	d.SetMenuBar(bar)
	d.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: w, H: 3})))
	d.Redraw()

	d.handleType(altRune('d'))
	if !bar.IsOpen() || len(bar.openPath) == 0 || bar.openPath[0] != 1 {
		t.Fatalf("Alt+d should open the right-aligned Daemon via desktop routing, got %v", bar.openPath)
	}
}

// =====================================================================
// (7) A right-aligned top menu's dropdown opens on-screen (clamped), under the
//     item's real right-side X.
// =====================================================================

func TestRightMenuDropdownClampedOnScreen(t *testing.T) {
	const w = 80
	// Children chosen wide enough that the dropdown would overflow the right
	// edge if not clamped (popupWidth("Disconnect…") pushes past the anchor).
	daemon := NewSubMenu("&Daemon",
		NewMenuItem("&Connect…", nil),
		NewMenuItem("&Disconnect remote session", nil),
	).AlignRight()
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1}, daemon)
	d := renderBar(t, w, bar)

	anchor := bar.topRects[0]
	bar.handleClick(bar.Component, down(anchor.X+1, 0)) // open Daemon
	d.Redraw()

	if len(bar.popupLayouts) != 1 {
		t.Fatalf("want 1 popup, got %d", len(bar.popupLayouts))
	}
	pr := bar.popupLayouts[0].rect
	if pr.X < 0 || pr.X+pr.W > w {
		t.Fatalf("right-menu dropdown must stay on-screen: rect=%+v (w=%d)", pr, w)
	}
	// It opens below the bar (not above it / not on row 0).
	if pr.Y < anchor.Y+1 {
		t.Fatalf("dropdown should open below the bar, got Y=%d", pr.Y)
	}
}

// =====================================================================
// (8) Backward-compat: no status + no right-aligned item == byte-for-byte today.
// =====================================================================

func TestBackwardCompatNoStatusNoRightIdentical(t *testing.T) {
	const w = 80
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", nil)),
		NewSubMenu("&Edit", NewMenuItem("&Cut", nil)),
		NewSubMenu("&View", NewMenuItem("&Zoom", nil)),
	)
	d := renderBar(t, w, bar)

	// Exact historical left-pack rects.
	want := []Rect{{X: 0, Y: 0, W: 6, H: 1}, {X: 6, Y: 0, W: 6, H: 1}, {X: 12, Y: 0, W: 6, H: 1}}
	for i, wr := range want {
		if bar.topRects[i] != wr {
			t.Fatalf("topRects[%d]: want %+v, got %+v", i, wr, bar.topRects[i])
		}
	}
	if bar.statusSlotW != 0 {
		t.Fatalf("no status: statusSlotW want 0, got %d", bar.statusSlotW)
	}
	// Labels at rect.X+1; nothing rendered at the far right.
	if got := chAt(d, 1, 0); got != 'F' {
		t.Fatalf("col 1: want 'F', got %q", got)
	}
	if got := chAt(d, 7, 0); got != 'E' {
		t.Fatalf("col 7: want 'E', got %q", got)
	}
	if got := chAt(d, 13, 0); got != 'V' {
		t.Fatalf("col 13: want 'V', got %q", got)
	}
	if got := chAt(d, w-1, 0); got != ' ' {
		t.Fatalf("far-right col %d should be blank (no status), got %q", w-1, got)
	}
}

// A bar that previously had a status (right menus packed left of it) must
// re-pack flush right once the status is cleared — dynamic re-layout.
func TestClearingStatusRePacksRightMenuFlushRight(t *testing.T) {
	const w = 80
	daemon := NewSubMenu("&Daemon", NewMenuItem("&Start", nil)).AlignRight()
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: w, H: 1}, daemon)
	bar.SetStatus("●") // slot 2 → Daemon at [70,78)
	d := renderBar(t, w, bar)
	if bar.topRects[0].X != 70 {
		t.Fatalf("with status, Daemon should start at 70, got %d", bar.topRects[0].X)
	}

	bar.SetStatus("") // slot gone → Daemon re-packs flush right [72,80)
	d.Redraw()
	if bar.statusSlotW != 0 {
		t.Fatalf("after clearing status, statusSlotW want 0, got %d", bar.statusSlotW)
	}
	if bar.topRects[0] != (Rect{X: 72, Y: 0, W: 8, H: 1}) {
		t.Fatalf("Daemon should re-pack flush right to {72 0 8 1}, got %+v", bar.topRects[0])
	}
	// Label "Daemon" draws at rect.X+1=73 → D,a,e,m,o,n on cols 73..78, so the
	// bar's last column (79) is the menu box's trailing pad and must be blank.
	if got := chAt(d, 78, 0); got != 'n' {
		t.Fatalf("after re-pack, col 78 should hold last glyph 'n', got %q", got)
	}
	if got := chAt(d, w-1, 0); got != ' ' {
		t.Fatalf("after re-pack, last col %d should be blank, got %q", w-1, got)
	}
}

// =====================================================================
// Setters are pure state mutations (round-trip) — no redraw side effects
// required for correctness of the field updates themselves.
// =====================================================================

func TestStatusSettersRoundTrip(t *testing.T) {
	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 40, H: 1})
	bar.SetStatus("hello")
	if bar.StatusText != "hello" {
		t.Fatalf("SetStatus: want StatusText 'hello', got %q", bar.StatusText)
	}
	bar.SetStatusColors(tui.ANSIColor(3), tui.ANSIColor(5))
	if bar.StatusFG != tui.ANSIColor(3) || bar.StatusBG != tui.ANSIColor(5) {
		t.Fatalf("SetStatusColors: want FG=3 BG=5, got %+v %+v", bar.StatusFG, bar.StatusBG)
	}
	bar.SetStatus("")
	if bar.StatusText != "" {
		t.Fatalf("SetStatus(\"\") should clear, got %q", bar.StatusText)
	}
}
