# Design — MenuBar right-aligned status slot + right-aligned top menus

turbotui half of gogent issue #500: *"Surface connection/daemon status on the menu
bar (top-right) and move the Daemon menu to the right side."*

This is the **turbotui-first half ONLY**. It adds two generic, app-agnostic
rendering capabilities to `turbotv.MenuBar`; gogent wires them up in a follow-up.
**No gogent changes here, no daemon/SSH knowledge, no new dependencies.**

---

## 1. Summary of the change

`turbotv.MenuBar` currently packs all top-level items strictly left-to-right from
`X=0` and has no right-anchored slot. We add:

1. **A right-anchored STATUS slot** — `StatusText` (+ optional `StatusFG/StatusBG`),
   rendered flush-right within the bar, measured by display width, with documented
   narrow-terminal degradation.
2. **Right-aligned top-level menus** — a `RightAligned bool` flag on `MenuItem`;
   flagged items pack from the right edge inward, to the **left** of the status slot.

Both are opt-in. A bar with no `StatusText` and no `RightAligned` item renders
**byte-for-byte** as today.

### Chosen approaches (documented in code comments)

- **Right-aligned menus → approach (a), single `Menus` slice + per-item flag.**
  We add `RightAligned bool` to `MenuItem` and keep the one `Menus` slice. This is
  decisive for *no regressions*: `topRects` stays **index-aligned** with `Menus`,
  so `hitTest`, `handlePress`, `handleRelease`, `OpenTopByMnemonic`, `layoutPopups`,
  `adjacentTop`, `defaultOpenPath`, `currentItem`, `topMnemonics` all keep indexing
  `Menus[i] ↔ topRects[i]` and need **zero changes**. Only `layoutTopRects` (which
  assigns the rects) changes. The rejected approach (b) (split index) and a separate
  `RightMenus` slice would both desynchronise index↔rect and force edits across all
  those call sites — more surface area, more regression risk.

- **Status overflow → truncate-with-ellipsis ("…"), never wrap, never overdraw a
  left item.** Left menus have absolute priority over their cells; the status slot
  yields first (shrinks then hides), right-aligned menus yield next. Detail in §4.

### Coordinate convention (pinned — this caused the v1 off-by-one)

`Rect.Right()` is the **inclusive** last column: `X + W - 1` (`rect.go:29-31`). A
`{X:0,W:80}` bar has `Right()==79` and valid columns `0..79`. To avoid inclusive/
exclusive confusion in the packing math, §3/§4 compute against the **exclusive** end
`barEnd := abs.X + abs.W` (==80 here); the bar occupies columns `[abs.X, barEnd)`.
Every formula below is written in those terms and worked through with a concrete
example so the driver implements the columns exactly.

---

## 2. Files / functions touched

### `turbotv/menu.go` (all the real work)

- **`MenuItem`** — add field:
  ```go
  // RightAligned packs this top-level menu from the right edge inward (to the
  // left of the status slot) instead of left-packing it. It only affects a
  // top-level menu (an entry of MenuBar.Menus); it is ignored on child items.
  RightAligned bool
  ```
  Add a small builder to follow existing fluent style (`WithShortcut`, `WithActionID`):
  ```go
  func (m *MenuItem) AlignRight() *MenuItem { m.RightAligned = true; return m }
  ```

- **`MenuBar`** — add fields (placed with the other style fields so the zero value
  means "off"):
  ```go
  StatusText string        // right-anchored status; empty = nothing drawn (default)
  StatusFG   tui.Color     // zero => fall back to m.FG
  StatusBG   tui.Color     // zero => fall back to m.BG
  ```
  `NewMenuBar` is **unchanged** (these default to zero), keeping the constructor
  backward-compatible.

- **`(*MenuBar) layoutTopRects(abs Rect) []Rect`** — the core change. New algorithm
  (§3). Still returns one rect per `Menus` entry, index-aligned. When there are no
  right-aligned items and `StatusText == ""`, it produces the **exact** rects the
  current loop produces (same `len([]rune(text))+2` width formula, same left-pack
  from `abs.X`).

- **`(*MenuBar) draw(component, surface)`** — after the existing left/right top-item
  draw loop (which is unchanged: it iterates `m.Menus`/`m.topRects` by index and now
  simply paints each at whatever rect `layoutTopRects` assigned, left or right), add
  a final block that renders the status text right-aligned within `abs` (§4). Guarded
  by `if m.StatusText != ""`, so the no-status path is untouched.

- **`(*MenuBar) SetStatus(text string)`** and **`(*MenuBar) SetStatusColors(fg, bg tui.Color)`**
  — setters (§5).

- One new unexported helper:
  ```go
  // topItemWidth returns the cell width of a top-level menu label cell box. It
  // MUST mirror the historical formula EXACTLY to keep existing bars byte-for-byte
  // unchanged:
  //     text, _ := parseMnemonic(item.Label)   // <-- strip the '&' marker first
  //     return len([]rune(text)) + 2           // one pad cell each side
  // The parseMnemonic step is mandatory, not incidental: gogent's labels are all
  // mnemonic-bearing ("&File", "&Daemon", …); measuring the raw Label would count
  // the '&' and shift every cell right by one, breaking the byte-for-byte guarantee
  // for every consumer. Keep len([]rune) (rune count), NOT tui.StringWidth, so the
  // measurement is identical to today even for a hypothetical wide-rune top label
  // (status alone uses StringWidth — see §4). Used for both left and right items.
  func topItemWidth(item *MenuItem) int
  ```

- **`layoutPopups`, `hitTest`, `handleClick`/`handlePress`/`handleRelease`,
  `OpenTopByMnemonic` — NOT modified.** They already key off `topRects[idx]`, which
  now carries the right-side X for a right-aligned item, so:
  - clicks on a right-aligned label resolve to the correct item (the rect is there),
  - its dropdown opens under its real (right-side) X — `layoutPopups` already does
    `anchor := m.topRects[topIndex]` and already clamps `if x+width > appW { x = appW-width }`,
    so a right-edge `Daemon` submenu stays on-screen with no new code,
  - `OpenTopByMnemonic` already scans `m.Menus`, so a right-aligned menu opens by
    mnemonic unchanged.

### `turbotv/desktop.go` — **read-only (no change)**

`SetMenuBar` and `compose` already set the bar's bounds to `{0,0,app.Width(),1}`
every frame, so `abs.Right()` tracks the terminal's right edge and the status slot
re-anchors on resize automatically. The "menu bar draws on top of all layers"
guarantee in `compose` is untouched. `WorkArea` (Y=0 reserved) is unaffected. No
setter needs to be exposed — `MenuBar` fields/methods are public and the bar is
reachable by the app that constructed it.

### `turbotv/menu_*_test.go` — new tests (§7)

---

## 3. Layout algorithm (`layoutTopRects`)

Row precedence, left → right:

```
[ left-packed top menus ] ... gutter ... [ right-aligned top menus ] [ status slot ]
```

All arithmetic uses the **exclusive** end `barEnd := abs.X + abs.W` (see the pinned
coordinate convention in §1). Worked example throughout: an 80-wide bar at `X=0`
(`barEnd==80`, columns `0..79`), with left menus consuming columns `0..9`
(`leftEnd==10`) and a 2-cell status text `"AB"`.

```
rects := make([]Rect, len(m.Menus))
barEnd := abs.X + abs.W                         // EXCLUSIVE right end (==80)

// (1) Left-pack left-aligned items from abs.X, exactly as today.
x := abs.X
for idx, item := range m.Menus {
    if item.RightAligned { continue }
    w := topItemWidth(item)
    rects[idx] = Rect{X: x, Y: abs.Y, W: w, H: 1}
    x += w
}
leftEnd := x                                    // first FREE column after left items (==10)

// (2) Decide the status SLOT width (text + exactly 1 pad cell). The slot is the
//     LOWEST priority: it yields to both the left menus and the right menus.
desiredSlotW := 0
if m.StatusText != "" {
    desiredSlotW = tui.StringWidth(m.StatusText) + 1   // 2 text + 1 pad == 3
}
rightMenusW := sum of topItemWidth over RightAligned items
free := barEnd - leftEnd                         // columns available right of left menus (==70)
slotW := clamp(desiredSlotW, 0, max(0, free-rightMenusW))   // ==3
m.statusSlotW = slotW                            // stash for draw() — single source of truth

// (3) Right-pack right-aligned items, iterating Menus in REVERSE so declared
//     left-to-right reading order is preserved (last-declared sits nearest status).
//     They pack to the LEFT of the slot: the slot occupies columns [barEnd-slotW, barEnd).
cursor := barEnd - slotW                          // first column the slot owns (==77)
for idx := len(m.Menus)-1; idx >= 0; idx-- {
    item := m.Menus[idx]
    if !item.RightAligned { continue }
    w := topItemWidth(item)
    cursor -= w
    rx := cursor
    if rx < leftEnd { rx = leftEnd }              // never start left of the left menus
    rects[idx] = Rect{X: rx, Y: abs.Y, W: w, H: 1}
}
return rects
```

`slotW` is the *actual* reserved slot (post-clamp). It is stashed in the unexported
field `m.statusSlotW` during `layoutTopRects` (which `draw` calls at `menu.go:200`
before painting), so `draw` and the hit/edge geometry use one identical value — no
recomputation, no drift.

**Backward-compat invariant:** with no right items and `StatusText==""`, steps (2)
and (3) are inert (`slotW==0`, `m.statusSlotW==0`, the reverse loop assigns nothing)
and step (1) is the verbatim current loop → identical `rects`.

---

## 4. Status rendering (`draw`)

After the top-item loop, guarded by `m.StatusText != ""`, using the stashed
`m.statusSlotW` and `barEnd := abs.X + abs.W`:

```
statusFG := m.StatusFG; if statusFG == zero { statusFG = m.FG }
statusBG := m.StatusBG; if statusBG == zero { statusBG = m.BG }
// m.statusSlotW is the clamped slot width from layout (0 => no room => draw nothing).
if m.statusSlotW > 0 {
    textCols := m.statusSlotW - 1                 // reserve exactly 1 pad cell on the right
    text := Truncate(m.StatusText, textCols, "…") // width-aware, never splits a wide glyph
    startX := barEnd - 1 - tui.StringWidth(text)  // == abs.Right() - StringWidth(text)
    surface.WriteString(startX, abs.Y, text, tui.Cell{FG: statusFG, BG: statusBG})
}
```

Worked example (`barEnd==80`, `"AB"`, `slotW==3`, `textCols==2`, text un-truncated):
`startX = 80 - 1 - 2 = 77`; text fills columns **77–78**; column **79 (==abs.Right())**
is the blank pad. (The v1 draft's `abs.Right()-1-Wt` mistreated the inclusive
`Right()` and placed the text one column too far left, leaving cols 78 **and** 79
blank — a 2-cell pad that never reached the true right edge. Fixed.)

- **Display-width correct:** widths via `tui.StringWidth`, truncation via `Truncate`
  (both already used throughout `measure.go`/`surface.go`); `●`/`○`/`◐` and any wide
  rune are measured/cut correctly. `surface.WriteString` is itself width-aware.
- **Flush right with exactly 1 pad:** rightmost text glyph lands at `abs.Right()-1`;
  column `abs.Right()` (the bar's last column) stays blank.
- **StatusText is drawn literally** via `WriteString` (NOT `drawMnemonic`): no `&`
  mnemonic parsing, no hot-key underline — a `&` in the status renders as `&`. This is
  deliberate (a status label is not an accelerator target) and documented in §8.
- **Overflow = truncate-with-ellipsis, then hide:** as the terminal narrows, the slot
  width shrinks (§3 clamp); `Truncate` ellipsises the text to `textCols`; when the
  clamp drives `slotW` to 0 the status is drawn nowhere. **Never** a second row,
  **never** over a left item's glyph (left items are drawn first and the slot is
  reserved to the right of `leftEnd`; the surface clip also confines every write to
  the bar).

The default colors (zero `StatusFG/StatusBG` → bar `FG/BG`) keep an app that sets
only `StatusText` visually consistent with the bar, satisfying the usability gate.

---

## 5. Setters

`MenuBar` does not own a Desktop reference, and the toolkit's convention (see
`VisualComponent.SetVisible/SetEnabled/SetBounds`) is that widget mutators are pure
state changes performed on the event loop; the **Desktop** drives the single
coalesced redraw (see the Desktop threading contract in `desktop.go`). We follow that
convention exactly:

```go
// SetStatus updates the right-anchored status text. Like the other MenuBar/widget
// mutators it only changes state; it must be called on the event loop (or via
// Desktop.Post from a background goroutine), which drives the coalesced redraw.
func (m *MenuBar) SetStatus(text string) { m.StatusText = text }

// SetStatusColors overrides the status text colors; a zero color falls back to the
// bar's FG/BG. Same threading contract as SetStatus.
func (m *MenuBar) SetStatusColors(fg, bg tui.Color) { m.StatusFG, m.StatusBG = fg, bg }
```

gogent's consumer will call these inside `desktop.Post(...)` on daemon mode/disconnect
transitions, which both mutates and requests the coalesced redraw — the documented,
race-free path for background updates. Right-alignment is set at construction
(`item.AlignRight()` or `RightAligned: true`); a runtime setter is unnecessary for v1
and omitted to keep the surface minimal.

**Explicit repaint caveat (stated, not hidden):** `SetStatus`/`SetStatusColors` are
**pure mutations** — they do **not** themselves repaint. Unlike a hypothetical
`Invalidate`, a bare `bar.SetStatus("…")` will not show until the next compose.
Callers MUST update via `Desktop.Post(func(){ bar.SetStatus(...) })` (Post calls
`app.RequestRedraw()` after the closure, `desktop.go:125-134`) or pair the mutation
with an explicit `Desktop.Redraw()`. This matches the `SetVisible`/`SetEnabled`
convention and is the deliberate division of labour (the Desktop owns the single
coalesced redraw); it is called out here because it is a real dependency the consumer
must honour. gogent already does — it rebuilds the bar and calls `Redraw`/uses `Post`
on every mode transition.

**v1 color limitation (acknowledged, not oversold):** `StatusFG/StatusBG` color the
**entire** status string with one pair. So gogent can render the whole slot in one
color, but **cannot** put a green `●` next to a default-colored "connected" within the
same slot. Issue #500's `●/○/◐` framing can still be satisfied (color the whole
string by state, or bake the state into the glyph choice), but per-glyph coloring is
out of scope for v1. If it is later needed, the forward-compatible extension is an
optional styled-segment API (e.g. `SetStatusSegments([]StatusSpan)`) layered on top of
the same right-anchored slot — the field/setter added here do not foreclose it.

---

## 6. Design criteria (the 4 gates)

**(1) Goal match.** Exactly the two capabilities the issue asks for, both generic:
(i) an always-rendered, right-aligned, display-width-measured status slot, and
(ii) right-aligned top-level menus that pack from the right, to the left of the slot.
No daemon/SSH semantics leak in (gogent supplies the string and sets the flag). No
scope creep — `layoutPopups`/hit-testing/mnemonics are reused, not redesigned.

**(2) Usability.** Clicks resolve to the right item for both left and right menus
(shared `topRects` path); a right-aligned `Daemon` opens by mnemonic
(`OpenTopByMnemonic`) and its dropdown opens under its real X, clamped on-screen by
the existing `layoutPopups` logic. Status colors default sensibly to the bar's FG/BG
(zero → bar FG/BG), with the single-pair limitation acknowledged above. The status is
surfaced flush-right and always visible (not buried), which is the user-visible point
of the issue. The flush-right placement is now correct against inclusive `Rect.Right()`
(§4), and the repaint dependency of `SetStatus` is stated for the consumer (§5).

**(3) No regressions.** The index-aligned single-slice approach means every existing
call site is untouched; only rect *assignment* changes. The no-status/no-right path
reproduces the current `len([]rune)+2` left-pack byte-for-byte (guarded, inert new
code). Local gate: `gofmt`/`go build`/`go vet`/`golangci-lint` (0 new) /`go test ./...`
must be green (turbotui has no CI, so this is authoritative). A dedicated
backward-compat test (render an existing 3-menu bar, assert cells unchanged) guards
this.

**(4) Holistic / cross-repo seam.** Change is confined to `turbotv/` (essentially
`menu.go`); `desktop.go` is read-only and the "menu bar on top of all layers" +
"WorkArea reserves Y=0" guarantees are preserved. No new external dependency
(stdlib + existing `tui.StringWidth`/`Truncate`). The seam is respected: turbotui
gains a *generic* capability; gogent is a thin consumer in the follow-up (feeds the
status string, sets `RightAligned` on its `Daemon` menu, refreshes via `Desktop.Post`
on transitions, after a `go.mod` bump to the new turbotui version). Nothing here
pre-commits gogent to a shape.

**Cross-repo correctness note for the gogent half (so the seam isn't misused):** the
per-redraw status string must be derived from the **cheap synchronous** signals —
`Handlers.DaemonMode()` plus connection state — **never** from `DaemonStatusInfo()`/
`DaemonStatusReport`, which does an HTTP round-trip (`daemon_menu.go:153-167`).
turbotui's `draw` may run on any compose; sourcing the status from a blocking call
would stall the event loop. The v1 design draft conflated the two; this is the
authoritative guidance for the follow-up. (No turbotui code depends on this — it is
purely a note to keep the consumer honest.)

---

## 7. Tests (turbotv, `app.ReadCell(x,y).Ch` cell assertions, as the package does)

Construct via `tui.NewWithSize(w, h, &output)` + `NewDesktop` + `SetMenuBar`, then
`desktop.Redraw()` and read cells (mirrors existing menu/desktop tests).

   Column expectations are pinned to the **corrected** flush-right math (§4),
   `startX == abs.Right() - tui.StringWidth(text)` with the pad at `abs.Right()` — NOT
   "whatever the code emits". The golden/cell asserts are the guard against the
   off-by-one, so they must encode the intended columns.
1. **Status flush-right on a wide bar** — set `StatusText="AB"`, render on an 80-wide
   bar (`abs.Right()==79`); assert the glyphs land at columns **77 and 78**, and that
   column **79** is blank (exactly 1 pad). (A naive `Right()-1-Wt` impl would put them
   at 76–77 and fail this.)
2. **Non-ASCII status width** — `StatusText` containing a wide rune and/or `●`/`◐`;
   assert `startX == abs.Right() - tui.StringWidth(text)` (proves display-width, not
   byte/rune-count, measurement) and that a wide glyph is not split at the edge.
3. **Right-aligned menu packed from the right** — a bar with left `File`/`Edit` and a
   `Daemon`.AlignRight(); assert `Daemon`'s rect is flush-right (left of the status
   slot) and `File`/`Edit` rects are byte-for-byte where they are today.
4. **No overlap on a wide terminal; graceful degradation on a narrow one** — wide:
   assert left rects and the right/status region are disjoint. Narrow: shrink width so
   the slot cannot fit; assert the status truncates (ellipsis) and then hides
   (`m.statusSlotW==0`), that no cell of a left item is overwritten, and that nothing
   is drawn on row 1 (no wrap).
5. **Click resolution, left AND right** — `handlePress(down(x,0))` on a left item
   opens it; on a right-aligned item's cell opens *that* item (not a neighbour);
   release/activation fires the right leaf.
6. **Mnemonic on a right-aligned menu** — `OpenTopByMnemonic('d')` opens the
   right-aligned `Daemon`.
7. **Right-aligned submenu stays on-screen** — open the right-edge menu; assert its
   `layoutPopups` rect satisfies `0 <= X && X+W <= appW`.
8. **Backward-compat golden** — a `MenuBar` with no `StatusText` and no `RightAligned`
   item renders identical `topRects` and identical row-0 cells to the pre-change
   behavior (regression guard for other turbotui consumers).

---

## 8. Regression risks & mitigations

- **Index/rect desync** — avoided by design: one `Menus` slice, one index-aligned
  `topRects`; `layoutTopRects` is the only writer. (The reverse-iteration right-pack
  writes into the same indices.)
- **`draw`/hit geometry disagreeing on the slot width** — compute the clamped slot
  width once in `layoutTopRects`, stash it in `m.statusSlotW`, and read that same field
  in `draw` (§3/§4) so the reserved slot and the painted text never drift.
- **Byte-for-byte drift via mnemonic measurement (high-likelihood landmine)** —
  `topItemWidth` MUST call `parseMnemonic(item.Label)` before `len([]rune)+2`
  (§2). Measuring the raw `Label` counts the `&` and shifts every mnemonic-bearing
  bar (all of gogent's: `&File`, `&Daemon`, …) one cell right — breaking the #1
  guarantee for *every* consumer. Keep `len([]rune)` (rune count), NOT `StringWidth`,
  so even a hypothetical CJK top label measures as today. Guarded by the backward-
  compat golden test (#8), but pinned in the spec so it is not discovered downstream.
- **Off-by-one against inclusive `Rect.Right()` (resolved in design)** — all flush-
  right/edge math is now written against the exclusive `barEnd := abs.X + abs.W`
  (§1/§3/§4) with worked columns; status start is `abs.Right() - StringWidth(text)`
  (1 pad at `abs.Right()`). Tests #1/#2 encode the *correct* columns, not the emitted
  ones, so they fail a re-introduced off-by-one.
- **Narrow-terminal overdraw** — left drawn first, slot reserved right of `leftEnd`,
  right items clamped to `>= leftEnd`, and the surface clip confines all writes to the
  bar; combined, a left glyph can never be overwritten and nothing escapes row 0.

---

## 8a. Documented limitations (in code comments + here)

These are deliberate v1 boundaries, written down so the capability isn't oversold:

1. **Status text uses one color pair** (`StatusFG/StatusBG`) for the whole string —
   no per-glyph coloring (e.g. a green `●` beside default text). See §5 for the
   forward-compatible `SetStatusSegments` extension path.
2. **Status text is literal** — not `&`-mnemonic-parsed; no hot-key underline (§4).
3. **`SetStatus` does not repaint by itself** — caller must `Post`/`Redraw` (§5).
4. **Multiple right-aligned menus on a too-narrow terminal can overlap each other.**
   The §3 reverse-pack clamps each right rect's X up to `leftEnd` (so right menus never
   start left of the left menus), but when several right items are clamped to the same
   floor they can overlap *one another*. For the issue's actual use (a single `Daemon`
   menu) this never triggers. For N>1 right menus the documented degradation is:
   right-to-left priority (last-declared, nearest the status slot, wins its cells;
   earlier ones are squeezed/clipped); never a second row; left menus and the surface
   clip are still inviolate. This is acceptable because a bar narrower than its own
   menus is already pathological; the guarantee we make is "no overdraw of left items,
   no wrap," not "all right menus always fully visible."

## 9. Open questions

1. **Arrow-key order for mixed bars.** Left/Right arrow navigation (`adjacentTop`)
   steps in **declared slice order**, not visual order — so on `[File, Edit, Daemon(R)]`,
   Right from `Edit` lands on `Daemon` (which is visually far right). This matches
   declaration order and is conventional; I propose leaving it as-is (documented in a
   comment) rather than sorting by X. Flag if visual-order arrow nav is wanted.
2. **`AlignRight()` builder vs field-only.** Plan adds both the public `RightAligned`
   field and an `AlignRight()` fluent helper (matches `WithShortcut`/`WithActionID`).
   If the maintainer prefers field-only construction, drop the helper.
3. **Pad width.** Status reserves exactly 1 blank cell at the far right (the issue's
   "≥1"). If a 2-cell right margin is preferred for breathing room, it's a one-constant
   change.
4. **Multiple right-aligned menus ordering/degradation** — assumed declared-left-to-
   right reading order ending at the status slot (last-declared nearest the slot), with
   the narrow-terminal overlap behavior specified in §8a(4). gogent only needs one
   (`Daemon`), so this is forward-looking; confirm the ordering convention if more are
   expected.
