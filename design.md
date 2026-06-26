# Design — Issue #490 (TURBOTUI HALF)

**Marker-less, body-clickable tree node for the gogent sub-agent summary line**

Supports gogent #490: *"Sub-agent summary should be a single collapsible line with a
`[...]+`/`[...]-` suffix."* This is the **turbotui-first** half. It adds the minimal
widget primitive gogent needs; the gogent half (the `[...]+`/`[...]-` suffix text and
label refresh) follows after this merges. **turbotui-only — no gogent files change here.**

---

## Problem (confirmed against source)

`turbotv/widget_tree.go` (`v0.3.1-0.20260626065139-7db1e2fafccc`):

- **`draw()`** (lines ~220–227) hardcodes the leading column: any node with
  `len(Children) > 0` paints `▾` (Expanded) or `▸` (collapsed); leaves paint `" "`.
  The visible content per row is `Truncate(marker+" "+r.node.Label, avail, "…")` written
  at `abs.X + r.depth*2`. There is **no field** on `TreeNode`/`Tree` to suppress the marker.
- **`handleClick()`** (lines ~410–417) flips `node.Expanded` **only** when
  `len(Children) > 0 && event.X <= markerCol`, where `markerCol = abs.X + r.depth*2`.
  A click on the row body falls through to the repeat-click `OnActivate` path
  (guarded by the local `toggledMarker` flag).
- **`handleType()`** (lines ~338–366) toggles `Expanded` on Left/Right/Space. **Unchanged.**

The gogent sidebar wants a synthetic summary node that (a) renders with **no** leading
`▸`/`▾`, and (b) toggles expansion from a **row/body click**, not only the marker column.

Confirmed identifiers: `TreeNode{Label, Expanded, Children, Data}`; click-event type is
`tui.ClickEvent` (`app.go:81`, fields `X,Y,Button,Down,Drag,Move`);
`handleClick(component *VisualComponent, event tui.ClickEvent) bool`;
`draw(component *VisualComponent, surface Surface)`.

---

## Design

Two additive changes, both default-off so existing consumers are byte-identical.

### 1. `TreeNode.HideMarker bool` — rendering opt-out

Add the field to `TreeNode` (zero value `false`):

```go
type TreeNode struct {
    Label    string
    Expanded bool
    Children []*TreeNode
    Data     interface{}
    // HideMarker suppresses the leading ▸/▾ for a node that HAS children, painting
    // a blank leading column instead while keeping the same indentation so labels
    // stay aligned. Default false ⇒ ▸/▾ as before. Leaves are blank regardless.
    HideMarker bool
}
```

In `draw()`, gate the marker selection on `!HideMarker`:

```go
marker := " "
if len(r.node.Children) > 0 && !r.node.HideMarker {
    if r.node.Expanded { marker = "▾" } else { marker = "▸" }
}
```

When `HideMarker` is set on a parent, `marker` stays `" "`, so the written content is
`"  " + Label` — **identical column layout to a leaf**. Indentation (`r.depth*2`),
truncation, ellipsis, and the selection bar are untouched. Children still render when
`Expanded` (flatten() is unaffected — it keys off `Expanded`, not the marker).

### 2. `Tree.OnToggle func(node *TreeNode, ev tui.ClickEvent) bool` — host toggle path (PRIMARY)

Add an optional hook on `Tree`, tried in `handleClick` **before** the default
marker-column logic. If set and it returns `true`, the click is consumed as a toggle:
the default marker toggle and the repeat-click `OnActivate` are both suppressed; the
host owns whether/how to flip `Expanded` (and, in the gogent half, rewrites its
`[...]+`/`[...]-` label suffix in the same callback).

```go
// OnToggle, when set, is offered each committed row click BEFORE the default
// expand-marker logic. Returning true consumes the click as a toggle: the host has
// handled it (typically flipping node.Expanded and refreshing its label), so the
// widget skips its own marker-column toggle and the repeat-click OnActivate. Returning
// false lets the click fall through to the default behaviour. Nil ⇒ behaviour unchanged.
OnToggle func(node *TreeNode, ev tui.ClickEvent) bool
```

`handleClick` change (rename the local `toggledMarker` → `toggled`, add the hook
before the marker test; everything else, incl. selection update, `OnSelect`,
`OnSelectMouse`, scrollbar/Down/bounds handling, stays put):

```go
wasSelected := t.selected == idx
t.selected = idx
r := rows[idx]
toggled := false
// Host toggle hook: lets a marker-less node toggle from a row/body click. Tried
// before the default marker-column logic; returning true consumes the toggle.
if t.OnToggle != nil && t.OnToggle(r.node, event) {
    toggled = true
}
markerCol := abs.X + r.depth*2
if !toggled && len(r.node.Children) > 0 && event.X <= markerCol {
    r.node.Expanded = !r.node.Expanded
    toggled = true
}
clicked := r.node
t.fireSelect(rows)
t.fireSelectMouse(clicked)
if wasSelected && !toggled && t.OnActivate != nil {
    t.OnActivate(r.node)
}
return true
```

Order rationale: selection still moves to the clicked row first (a click on a summary
line should select it), then `OnToggle` decides the toggle, then `OnSelect`/
`OnSelectMouse` fire (consistent with the existing marker-toggle path, which also fires
both). `OnToggle` returning true behaves exactly like the existing `toggledMarker=true`
branch w.r.t. suppressing `OnActivate`.

### Keyboard (unchanged)

`handleType` Left/Right/Space keep flipping `Expanded` directly for any node with
children, `HideMarker` or not. `OnToggle` is mouse-only by design; the gogent half keeps
its suffix in sync by deriving it from `Expanded` on its own redraw, which already covers
both keyboard and mouse toggles (see the seam note below). No keyboard change is needed
or made here.

### Why `OnToggle` over the relaxed-guard alternative

The simpler alternative is: when `node.HideMarker`, relax the `event.X <= markerCol`
guard so any body click flips `Expanded`. It is fewer lines and fully internal. I chose
`OnToggle` as the primary because:

- It gives gogent an **explicit toggle signal** at the click site, so the host can flip
  `Expanded` **and** rewrite its `[...]+`/`[...]-` suffix in one place — the cleanest
  seam (gogent owns the suffix text; turbotui owns rendering/hit-testing).
- It is a general, reusable primitive (any host toggle policy, e.g. suffix-region-only
  hit testing later) without baking gogent-specific UX into the widget.
- It is strictly additive: nil hook ⇒ zero behavioural change, so it cannot regress
  existing consumers regardless of `HideMarker`.

`HideMarker` and `OnToggle` are **independent** fields (one controls rendering, one
controls click toggling) but are **designed to pair**: gogent sets `HideMarker=true` for
the summary node and an `OnToggle` that toggles only when `node.HideMarker` (or by `Data`
identity). I am implementing `OnToggle`; the relaxed-guard option is noted in Open
Questions as a documented fallback the gogent side could request instead.

---

## Files / functions touched (turbotui only)

- `turbotv/widget_tree.go`
  - `TreeNode` struct: **+`HideMarker bool`**.
  - `Tree` struct: **+`OnToggle func(node *TreeNode, ev tui.ClickEvent) bool`**.
  - `draw()`: gate marker selection on `!r.node.HideMarker`.
  - `handleClick()`: try `OnToggle` before the marker-column test; rename local
    `toggledMarker` → `toggled`.
- `turbotv/widget_tree_hidemarker_test.go` (new): tests below, reusing existing helpers
  `drawTree`, `leafTree`, `clickRow`, `clickRowUp`, and `app.ReadCell(x,y)`.

No other files. No gogent files. No new dependencies (stdlib + existing `tui` import only).

---

## Tests to add

1. **Render blank marker (`HideMarker=true`)** — parent with children, `HideMarker=true`,
   `Expanded=true`: leading column at the row origin is `' '` (not `▸`/`▾`), the label
   starts at the same column as a leaf would (`abs.X+2`), and expanded children rows
   still appear. Column alignment unchanged vs a leaf.
2. **Regression: default still paints `▸`/`▾`** — same parent with `HideMarker=false`:
   collapsed shows `▸`, expanded shows `▾` (guards byte-identical default rendering).
3. **Click toggle via `OnToggle`** — set `HideMarker=true` and an `OnToggle` that flips
   `node.Expanded` and returns true; a body-row click toggles `Expanded`; `OnActivate`
   does **not** fire on that click even when the row was already selected.
4. **`OnToggle` tried before default + consume suppresses default** — `OnToggle` returning
   true on a normal parent suppresses the default marker-column toggle (Expanded stays as
   the host left it); returning false lets the default marker-column toggle run.
5. **Default body click does NOT toggle** — `HideMarker=false`, `OnToggle=nil`: a click on
   the body (X past the marker column) leaves `Expanded` unchanged; a click on the marker
   column still toggles (existing behaviour preserved).
6. **Keyboard still toggles a `HideMarker` node** — Left/Right/Space on a `HideMarker=true`
   parent flips `Expanded` exactly as for a normal parent.

---

## Design criteria — the 4 gates

**(1) Goal match.** Delivers exactly the issue's primitive: a host can render a parent
node with no leading `▸`/`▾` (`HideMarker`) and toggle it from a row/body click
(`OnToggle`). No suffix logic, no summary-node modelling — those are the gogent half. No
scope creep, no missing ask.

**(2) Usability.** Column alignment/indentation are unchanged when the marker is hidden
(blank column keeps `"  "+Label` aligned with leaves). Body click gives a large hit target
for the single-line summary; keyboard toggle still works. The toggle is surfaced (the host
acts on it), not silent.

**(3) No regressions.** Both additions default off: `HideMarker=false` ⇒ identical
rendering; `OnToggle=nil` ⇒ identical click handling (the only code change on the nil path
is the local rename `toggledMarker`→`toggled`, semantically identical). `flatten()`,
selection, scrollbar, `OnSelect`/`OnSelectMouse`/`OnActivate`, and keyboard paths are
untouched. Existing `widget_tree*_test.go` suites must stay green. Local gate is
authoritative (no CI): `gofmt`, `go vet`, `golangci-lint`, `go test ./...`.

**(4) Holistic / seam across gogent + turbotui.** The primitive lives in turbotui
(rendering + hit-testing belong to the widget); the `[...]+`/`[...]-` **suffix text and
label refresh stay in gogent** (it owns `Label`). The seam is respected: turbotui's
`draw()` never needs to know the suffix exists — it only omits the marker; gogent derives
its suffix from `Expanded` on its redraw, which keeps suffix state correct after **both**
mouse (`OnToggle`) and keyboard toggles with a single host rule. The public API is
additive and backward-compatible, so this can merge before the gogent half and the gogent
side adapts to a stable contract. No new deps.

---

## Open questions

1. **`OnToggle` vs relaxed-guard.** I am implementing `OnToggle` (explicit host signal,
   cleanest seam). If the gogent side prefers zero new callback wiring, the documented
   fallback is: when `node.HideMarker`, relax `event.X <= markerCol` so any body click on
   that node flips `Expanded` internally — fewer lines, but no explicit toggle signal and
   it ties body-click-toggles to `HideMarker`. Confirm preference before the gogent half.
2. **Should `OnToggle` see press events too?** Current design offers only the committed
   (release) click, matching the existing toggle/activate path (`Down` events return early
   at line ~398). Assumed sufficient; flag if gogent wants press-time toggling.
3. **`OnToggle` scope.** Offered for *every* row click, not only `HideMarker` nodes, so it
   stays a general hook. gogent's handler is expected to self-filter (e.g. act only when
   `node.HideMarker`/by `Data`). Confirm this is the desired contract vs. restricting the
   hook to `HideMarker` nodes inside the widget.
