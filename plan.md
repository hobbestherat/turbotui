# Plan: flush-bracket Button.draw (gogent#259)

## Problem
`Button.draw` centers the whole `[ caption ]` group inside the face, so the visible
outline width is `displayW = StringWidth(label)+4`, not `face.W`. Buttons given equal
bounds but different labels paint ragged right edges (gogent's Queue/Interject/Stop).

## Fix (turbotv/widget_button.go, Button.draw)
Pin the brackets flush to the face bounds; float only the caption between them, so
`face.W` controls the visible box:
- left bracket at `face.X`
- right bracket at `face.X + face.W - rightW` (clamped to `>= face.X` for degenerate
  faces narrower than the bracket, so no negative offset / no panic)
- caption floats centered: `captionStart := face.X + leftW + (avail-captionW)/2`,
  drawn with the existing `drawMnemonicClipped(..., avail, ...)`.

Drop the now-unused `displayW`/`start` locals (centering the whole group is gone).

## Why this is safe for every button
The caption X is **unchanged** by construction:
`old start+leftW = face.X + (face.W-displayW)/2 + leftW = face.X + leftW + (avail-captionW)/2 = new captionStart`.
So only the brackets move outward to the edges; the caption never moves.
- Buttons sized exactly to `buttonWidth(label)` (face.W == displayW): `avail==captionW`
  ⇒ captionStart==face.X+leftW, rightX==face.X+leftW+captionW — identical to before.
- Wider face, differing labels: brackets land at the same X for equal bounds ⇒ equal
  boxes. Bug fixed.
- Narrow face (face.W < displayW): `avail` clamps to `>=0`, `captionW<=avail` so the
  centered offset is non-negative; caption clips via drawMnemonicClipped; rightX clamp
  prevents a negative bracket offset. Degraded but no panic, matching prior clipping.

## Existing tests
Both `widget_button_test.go` tests pass unchanged (caption position invariant; the one
bracket assertion is at a face where displayW==face.W). No test edits needed.

## Tester adds (GLM)
- Two buttons, same bounds (W wider than both labels), different label lengths ⇒ `[`
  at the same X and `]` at the same X (bug repro).
- A button sized exactly to its label renders identically to the pre-fix layout.
- Narrow face: no panic, no bleed.

## Constraints
turbotv only; no new deps; gofmt clean; golangci-lint ./... = 0; go test ./... green.
PR to main referencing gogent#259. No replace directive.
