# Fix: screen-artifact / shadow-leak (gogent #213)

## Root cause (turbotui side)

`turbotv/surface.go` `drawShadowCell` reads the underlying cell and, unless it is
blank, **re-emits the underlying foreground glyph** recoloured to the shadow
colour:

```go
under := s.app.ReadCell(x, y)
ch := under.Ch
if ch == 0 || ch == ' ' { ch = '░' }
s.app.WriteCell(x, y, tui.Cell{Ch: ch, FG: color, BG: under.BG})
```

So the shadow band is **not a pure function of geometry** — it mirrors whatever
glyph happens to sit in the back buffer at that cell. When that glyph is stale or
a bleed-through from an adjacent widget (e.g. an `e` from a "Session" label, or a
column the compositor did not clear before composing the next frame), the shadow
paints that letter instead of shadow texture. The result is the stray `e`s and
the corrupted `│░` divider region in the report.

## Part B (persistence) — mostly gogent side

The artifacts surviving an ordinary repaint is the signature of the App's `front`
buffer disagreeing with the real terminal: gogent writes notification / clipboard
escapes to `os.Stdout` out of band (under their own private locks), splicing bytes
into an `Apply()` frame and scrambling terminal state while `front` still records
"drawn correctly". turbotui already serialises its own frame writes and OSC 52
(`CopyToClipboard`) under `App.writeMu`; gogent simply does not route its writes
through `App`. The turbotui-side enabler is to expose the primitives gogent needs.

## Changes (all in turbotui)

1. **`turbotv/surface.go` — own the shadow cell.** Rewrite `drawShadowCell` so the
   shadow always lays down a single deterministic shadow glyph (`░`) in the shadow
   colour, preserving only the underlying background. It no longer reads/re-emits
   the underlying foreground rune, so no stale or bleed-through glyph can ever land
   in the shadow band. Because it writes a deterministic cell, the value differs
   from any prior content in the back buffer, so the front/back diff repaints
   (heals) it on an ordinary frame. Keeps `░` so existing geometry tests and the
   classic Turbo-Vision look are unchanged.

2. **`app.go` — `App.WriteControl(seq string)`.** The notification equivalent of
   `CopyToClipboard`: serialises an out-of-band escape/control sequence write
   through `App.writeMu` so it cannot splice into an `Apply()` frame. Documented to
   require a self-contained sequence that does not move the cursor or alter SGR
   (BEL, OSC 9, OSC 777, OSC 52). Lets gogent's notifier stop writing `os.Stdout`
   directly.

3. **`app.go` — `App.Invalidate()`.** Public wrapper over `invalidateFront`: marks
   the front buffer stale so the next `Apply()` repaints every cell. The force-redraw
   primitive that heals a terminal that has drifted out of sync with `front`.

## Residual gogent-side work (report, not fixed here — dep bump)

- Route `internal/notify` through `App.WriteControl` instead of `os.Stdout`.
- Route `internal/clipboard` through `App.CopyToClipboard`.
- Optionally `App.Invalidate()` after any unavoidable out-of-band write, or on the
  redraw following a notification, to heal pre-existing drift.
- Optionally inset window usable area / repaint the pinned sidebar after windows so
  shadow spill never reaches the divider column.

## Tests (GLM partner writes)

Target: draw a widget into cells, remove it (leave stale glyph in back buffer),
`DrawShadow` over those cells, assert every shadow cell renders `░` (not the stale
glyph) in the shadow colour. Plus `WriteControl`/`Invalidate` behaviour.
