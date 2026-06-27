# Design — Taller (2-row) buttons: turbotui-wide support (Phase 1)

gogent issue #529, turbotui half. Make `tv.Button` render correctly at any
`bounds.H >= 1`; height comes purely from bounds. `H == 1` must render
**byte-for-byte identically to today** (zero regression). The default everywhere
stays 1-row unless a caller explicitly sizes a button taller. gogent's Phase-2
half consumes this after a turbotui release.

This is the turbotui repo only. No new dependencies; stdlib-first.

---

## What I'm changing (exact files / functions)

### 1. `turbotv/widget_button.go` — `(*Button).draw` (lines ~69-120)

Today `draw` implicitly assumes `face.H == 1`: it fills the face, then writes the
left bracket, caption, and right bracket all on the single row `face.Y`.

Change it to draw at any height while keeping every existing sub-behaviour
(pressed offset, clipped `faceSurface`, flush brackets, caption centring,
ellipsis, mnemonic underline) intact:

- Keep the existing pressed-offset `face`, the `surface.DrawShadow(abs, …)` call,
  the `faceSurface := surface.WithClip(face)` clip, and `faceSurface.Fill(face,
  style)`. **`Fill` already paints the whole face rectangle (all rows)** — so the
  solid-block fill comes for free at any H; no change needed there.
- Compute the centred caption row:
  `centerY := face.Y + face.H/2` (integer division → rounds down on even heights,
  exactly as the issue specifies: H=2 → caption on the lower of the two rows,
  matching the existing pressed-H=2 test).
- **Brackets span the full height (design decision #1 — solid box).** Loop
  `y` from `face.Y` to `face.Bottom()`:
  - On every row, draw the **box brackets** `[` … `]` at the face edges
    (`left="[ "`, `right=" ]"`), using the exact flush-bracket positioning the
    code already computes (`face.X` for the left glyph; `rightX = face.X +
    face.W - rightW`, clamped to `>= face.X`).
  - On `centerY` only, draw the **caption** between the brackets via
    `drawMnemonicClipped(...)` at `centerY`, and — when focused — replace that
    row's brackets with the `►ＣＡＰＴＩＯＮ◄` chevrons (decision #1: chevrons sit
    only on the centred caption row; the other rows keep the plain box brackets
    `[`/`]` for visual weight).
- The left/right bracket strings, `leftW`/`rightW`, `avail`, `captionW`,
  `captionStart`, and `rightX` math are all **unchanged** — they are simply
  evaluated per the focus state and reused for the bracket rows. The unfocused
  bracket strings (`"[ "`, `" ]"`) are used on non-caption rows even when focused,
  so a focused tall button is a `[`-bordered box with `►…◄` only on the caption.

**Why H==1 stays identical:** with `H == 1`, `face.Bottom() == face.Y == centerY`.
The loop runs exactly once, on the only row, drawing brackets + caption exactly
as today. The fill, clip, shadow, colours and bracket formulas are untouched.
Every assertion in `widget_button_test.go` and `widget_button_bracket_test.go`
holds.

**Why the existing H==2 pressed test still passes:** `TestButtonPressedOffsetsFaceBracketsFlush`
uses `abs={2,0,12,2}`, pressed → `face={3,1,12,2}`. Full-height brackets now paint
rows 1 **and** 2; the test only asserts `'['` at `(3,1)` and `']'` at `(14,1)`
(row 1, a bracket row) and that `(2,0)` is blank — all still true.

### 2. `turbotv/dialog_layout.go` — `NewButtonRow` (lines ~24-57)

Today every button is force-sized `SetBounds(Rect{W: width, H: 1})` and the HBox
is `Rect{…, H: 1}`. **Key constraint discovered in `turbotv/layout.go`:** an HBox
defaults to `Align: AlignStretch` (`newBox`, line 48), and `crossPlace`'s stretch
branch returns `(0, boxSize)` — so the box **overwrites each child's H with the
box's H** during layout. Therefore propagating taller buttons means deriving the
*box* height from the buttons, not just the per-child H.

Change:

- Read each button's **own** `Bounds.H`; default to `1` when `H == 0`
  (`h := button.Component.Bounds.H; if h < 1 { h = 1 }`). Size the button with
  its real height: `SetBounds(Rect{W: width, H: h})`.
- Track `rowH := 1`; raise it to the tallest button's `h`.
- Build the HBox with that height: `NewHBox(Rect{X: boxX, Y: rowY, W: boxW, H: rowH})`.
  `AlignStretch` then makes every button in the row share `rowH` — the desired
  uniform footer height, and what gogent's `H:2` buttons will produce.
- `DefaultButtonGap` (=2), the alignment/clamp math, and `Spacing` are unchanged.

**Why all current callers are unaffected:** every existing caller passes buttons
with a zero `Rect{}` (H=0). `h` defaults to 1, `rowH` stays 1, the box is `H:1` —
byte-identical to today. `dialog_layout_test.go` (buttons built from `Rect{}`,
asserts box X/Y and child W/gap, never H) passes unchanged.

### 3. `turbotv/measure.go` — `ButtonLabelWidth` UNCHANGED

A tall button is not wider: width stays `max(StringWidth(clean)+4, minButtonWidth)`.
I will add only a one-line doc comment noting that **height comes purely from
`bounds.H`** (no width coupling). Optionally a tiny `ButtonHeight(bounds Rect) int`
helper that returns `max(1, bounds.H)` to document the "default 1" rule in one
place — additive, no behaviour change. `TestButtonLabelWidth` is untouched.

---

## User-facing behaviour

- **Default (today):** every button is 1 row; pixel-identical output. No caller
  changes, no visual diff.
- **Opt-in tall:** a caller sets `button.Component.Bounds.H = 2` (or more) before
  `NewButtonRow`, or passes a taller `Rect` to `NewButton`. The button renders as
  a solid `[ … ]` block filling all rows, caption + focus chevrons vertically
  centred (lower-middle row on even heights), drop shadow hugging the new bottom
  edge, and the entire face is click-activatable.
- Ellipsis truncation, mnemonic underline, flush-bracket alignment, and pressed
  down-right offset all behave exactly as at H=1, just on the centred row.

---

## Verification of the "don't break" seams (no code change needed, must hold)

- **Drop shadow** — `Surface.DrawShadow` keys off `rect.Right()`/`rect.Bottom()`
  (`surface.go:230`), and `draw` already passes the full `abs`. At H=2 the bottom
  band is computed from `abs.Bottom()` automatically → shadow hugs the taller box.
  New test will assert the shadow row sits one below the new bottom.
- **Mouse hit-test** — `handleClick` uses `component.AbsoluteBounds().Contains`
  (`rect.go:19`, `x>=X && x<X+W && y>=Y && y<Y+H`). The full H-row face is already
  hit-testable; a click on **either** row of an H=2 button passes `Contains`.
  New test asserts activation on both rows.
- **HBox stretch** — covered above; box H now drives child H.

---

## The four design gates

**(1) Goal match.** Exactly the issue's ask: Button renders at any `H>=1` with a
centred caption and full-height face; `H==1` unchanged; `NewButtonRow` propagates
each button's H (default 1). No scope creep — row-count selection and the `H:2`
dialog wiring are explicitly gogent Phase-2 concerns and are **not** in this half.
`ButtonLabelWidth` and the global default height (1) are untouched.

**(2) Usability.** Tall buttons read as solid, weighty blocks (full-height
brackets + filled face, decision #1) with the caption and `►…◄` focus chrome
optically centred. Ellipsis and mnemonic underline still land on the caption row.
The whole face is clickable, so the larger target behaves the way a user expects.
Height is driven by bounds — the natural, predictable knob.

**(3) No regressions.** `H==1` is byte-identical (single-row loop reduces to
today's code path). All existing tests pass **unchanged**:
`widget_button_test.go`, `widget_button_bracket_test.go` (incl. the H=2 pressed
case), `dialog_layout_test.go`, `surface_shadow*_test.go`. New tests added for
H>=2. Gate: `gofmt`/`go build`/`go vet`/`golangci-lint` (0 new issues)/`go test
./...` all clean — turbotui has no CI, so this local gate is authoritative.

**(4) Holistic / cross-repo seam.** The change lives entirely in turbotui's
rendering+layout layer, which is the correct place: gogent should not reach into
glyph drawing. The seam is respected — turbotui exposes "height comes from
bounds; `NewButtonRow` honours per-button H", and gogent Phase-2 consumes it by
(a) adding an H param to its `dialog_buttons.go`, (b) setting `H:2` in
`model_dialog.go`, and (c) bumping `go.mod` to the merged turbotui SHA. Nothing
here presupposes gogent internals, and the default-1 behaviour means the bump is
safe even before gogent opts in. After merge to `hobbestherat/turbotui` main I
record the merge SHA for that go.mod bump.

---

## New tests (turbotui, to be written in the build phase)

In `turbotv/widget_button_test.go` (or a new `widget_button_tall_test.go`):

1. **Caption on the centre row** — H=2 button: caption glyph appears on
   `face.Y + 1`, not row 0.
2. **Face spans both rows** — both rows are filled with the button bg style and
   carry the `[`/`]` box brackets at the face edges; brackets vertically aligned.
3. **Chevrons centred** — focused H=2 button: `►`/`◄` only on the caption row;
   plain `[`/`]` on the other row.
4. **Shadow hugs the new bottom** — H=2 button with shadow on: a shadow cell
   exists on the row just below `abs.Bottom()`, none above the new top.
5. **Click on either row activates** — `handleClick` down+up on row 0 *and*
   separately on row 1 both fire `OnPress`.
6. **`NewButtonRow` propagates H** — a row containing one `H:2` button yields a
   box of `H==2` and all children laid out at H==2; a row of default buttons
   stays `H==1` (guards the zero-regression default).

---

## Open questions

None blocking. Decisions taken per the issue's open questions:

1. **Bracket span on tall buttons** → full-height solid box (`[`/`]` on every
   row); `►…◄` focus chevrons only on the centred caption row. (Chosen for visual
   weight, matches the mock.)
2. **Row count** → gogent-side concern, out of scope for this turbotui half.
3. **`NewButtonRow` default height** → 1 (zero behaviour change); callers opt into
   taller explicitly by setting a button's `Bounds.H`. Global default unchanged.

One worth a sentence in the PR (not blocking): because the HBox is
`AlignStretch`, a button row's height is the **tallest** button in it — mixed-
height rows are normalised to the max. This is the desired uniform-footer
behaviour and the simplest semantics; documenting it avoids surprise if a future
caller mixes H=1 and H=2 buttons in one row.
