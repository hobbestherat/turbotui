# Design — Fix caret (hardware cursor) drift off the input form

**Issue:** turbotui bug (from gogent #454, FIX, authored by kloune). "Caret (hardware
cursor) drifts off the input form during streaming / after interruptions." The terminal
hardware caret only shows reliably after a few uninterrupted keystrokes; any frame that
changes cells elsewhere while the caret is stationary parks the real cursor on the last
painted cell.

This is **turbotui-first**. The whole fix lives in turbotui `app.go`. gogent needs only a
later `go.mod` dependency bump, handled as a separate follow-up after the turbotui merge SHA
is known.

---

## Root cause (confirmed against the code)

`App.Apply()` (app.go:679) renders a frame in two stages:

1. **Cell-diff loop** (app.go:696–741): for each run of changed cells it emits a CUP
   (`appendCursorMove`) followed by the cell bytes. After the loop the terminal's *real*
   hardware cursor sits just past the **last changed cell** — wherever that happened to be.
   The loop tracks this with locals `cursorX, cursorY` (start `-1,-1`; set only inside the
   changed-cell branch).

2. **`appendCursorEscapes(buf)`** (app.go:742, defined 772–794): brings the real cursor in
   line with the *desired* caret state (`cursorVisible/cursorX/cursorY`), de-duplicated
   against the `frontCursor*` record of what was last emitted.

The bug is the visible-branch early-out at app.go:780:

```go
if !force && a.frontCursorVisible && a.frontCursorX == a.cursorX && a.frontCursorY == a.cursorY {
    return buf   // emits NOTHING
}
```

`frontCursor*` only tracks cross-frame cursor escapes; it has **no knowledge of the cells the
diff loop just painted**. When the caret is visible and stationary (front matches desired),
the function returns nothing — but stage 1 already moved the real cursor to the last changed
cell. So on any frame where cells change *elsewhere* while the caret is still (streaming
transcript, 1 s status-ticker redraw, resize, coalesced redraw burst), the visible cursor is
left drifted and nothing homes it back.

**Why the symptoms match:** typing moves the caret → `frontCursorX != cursorX` → early-out
broken → CUP+`\x1b[?25h` emitted → cursor snaps to the caret. "No interruptions" = no other
cell changed that frame, so the real cursor stayed where the caret-CUP left it. Any
interrupting redraw re-drifts it.

---

## The fix

`appendCursorEscapes`'s visible-branch early-out is valid **only on a frame where the
hardware cursor did not move** — i.e. where the diff loop painted no cells. Signal that from
`Apply()`.

**`Apply()` (app.go:679):** after the cell-diff loop, derive whether any cell was written and
pass it down. The existing local already encodes this exactly — `cursorX` is `-1` iff the
loop emitted no CUP/cell, otherwise `≥0`:

```go
cellsWritten := cursorX != -1
buf = a.appendCursorEscapes(buf, cellsWritten)
```

**`appendCursorEscapes` (app.go:772):** take the new `cellsWritten bool` and gate the
**visible** early-out on it. When cells were written the hardware cursor has moved, so a
stationary visible caret must still be re-homed:

```go
func (a *App) appendCursorEscapes(buf []byte, cellsWritten bool) []byte {
    force := a.forceCursor
    a.forceCursor = false
    if a.cursorVisible {
        if !force && !cellsWritten &&
            a.frontCursorVisible && a.frontCursorX == a.cursorX && a.frontCursorY == a.cursorY {
            return buf // truly empty frame: real cursor never moved, nothing to do
        }
        a.frontCursorVisible = true
        a.frontCursorX = a.cursorX
        a.frontCursorY = a.cursorY
        buf = appendCursorMove(buf, a.cursorX, a.cursorY)
        return append(buf, "\x1b[?25h"...)
    }
    if !force && !a.frontCursorVisible {
        return buf
    }
    a.frontCursorVisible = false
    return append(buf, "\x1b[?25l"...)
}
```

Scope of change: the single added `&& !cellsWritten` clause (plus the new parameter and the
one-line caller). Everything else is unchanged.

### Why `cursorX != -1` is exactly the right signal (load-bearing)
The fix rests on `cursorX != -1` ⟺ "the hardware cursor moved this frame." Proof from the
loop body:
- `cursorX` starts `-1` and is assigned (`x + width`) **only** in the changed-cell emit
  branch, which is the **only** branch that writes a CUP + cell bytes. So `cursorX != -1` ⟺
  at least one CUP+cell was emitted ⟺ the real cursor moved.
- The continuation half of a wide glyph (`next.cont`, app.go:713) takes the early `continue`
  and emits **nothing**, leaving `cursorX` untouched. The only way a frame touches a `cont`
  cell without also emitting its lead glyph is a degenerate state (lead unchanged, cont
  changed) — and in that case the loop writes no bytes and the cursor genuinely did not move,
  so `cellsWritten == false` is correct, not a miss.
- `appendStyle`/`appendRune` never move the cursor without the preceding CUP, so there is no
  "wrote bytes but `cursorX` still `-1`" path.

Equivalently, the same signal could be read off `len(buf) != bodyStart` before calling
`appendCursorEscapes`; `cursorX != -1` is preferred only because it is local to the loop and
needs no second length capture.

### Why `cellsWritten` only gates the *visible* branch
- **Hidden branch:** a hidden cursor's position is invisible, so cell-write drift can't be
  seen. Its early-out (`!force && !a.frontCursorVisible`) stays valid regardless of cells
  written — leaving it untouched also avoids emitting a redundant `\x1b[?25l` every streaming
  frame.
- **`forceCursor` (invalidateFront):** still honored first and still forces a re-emit; the
  new clause only *adds* a reason to re-emit, never removes the force path.

### Truly-empty-frame optimization preserved
If no cells changed and the caret is unchanged: `cursorX == -1` → `cellsWritten == false` →
visible early-out still returns `buf` unchanged → `len(buf) == bodyStart` (app.go:743) →
`Apply` writes nothing. No-op frames stay no-ops; no per-frame churn when genuinely idle.

### Alternative considered (rejected)
Pass the loop's terminal cursor position into `appendCursorEscapes` and compare to the caret,
skipping the re-home when they happen to coincide. More precise but materially more complex,
and re-emitting a CUP is cheap; coincidence is rare and a redundant CUP is harmless. The
boolean is the minimal, clearest expression of the invariant ("a painted frame moved the
hardware cursor").

---

## Criteria

### (1) Goal match — fix, not feature/refactor
Targets exactly the early-return optimization named in the issue. One added condition + a
plumbed boolean. No new widgets, no API change (`SetCursor`/`HideCursor`/`Apply` signatures
unchanged), no behavior change beyond "re-home a visible stationary caret after a painted
frame." No scope creep.

### (2) Usability — the caret is surfaced, the user drives input
After the fix the hardware caret stays pinned to the focused input's insertion point through
streaming output, status-ticker redraws, resizes and coalesced redraw bursts — it no longer
requires uninterrupted typing to appear. The visible blinking caret (not a painted block) is
exactly what a user expects on the field they are typing into. Re-homing happens silently and
correctly every painted frame; nothing the user must trigger.

### (3) No regressions
- **Existing drift tests (`app_drift_test.go`):**
  - `TestInvalidateForcesFullRepaint`, `TestInvalidateReIssuesCursorState`,
    `TestInvalidateSettlesBackToNoop` — drive through `Invalidate` (`force=true`), which
    already bypasses the early-out; unaffected.
  - `TestInvalidate*` "settle back to no-op" — relies on idle frames writing nothing; the
    empty-frame path (`cellsWritten==false`) is preserved, so these stay green.
  - `TestWriteControl*` — independent of cursor escapes; unaffected.
  - `TestInvalidateReAssertsHiddenCursor` — a **pre-existing, expected-to-fail** test about
    the *hidden*-cursor invalidate path (a separate defect: `invalidateFront` can't force the
    hidden branch). My change does not touch the hidden branch, so this test's status is
    unchanged. **Out of scope** for this fix; called out so it isn't mistaken for a
    regression I introduced.
- **Synchronized-frame framing / write-mutex / error path:** untouched.
- **Performance:** on streaming frames we now emit ~9 extra bytes (CUP + `\x1b[?25h`) that
  were previously (incorrectly) omitted. These frames already write changed cells, so the
  marginal cost is negligible and the whole frame is wrapped in one DEC-2026 synchronized
  update — no flicker.
- **Idle frames:** still write zero bytes (verified via the `cellsWritten==false` path).

### (4) Holistic design across both repos
- **Right place:** the drift originates entirely in turbotui's render loop; `Desktop.updateCursor`
  (turbotv/desktop.go:686) already feeds the correct desired caret via `SetCursor`/`HideCursor`.
  The defect is purely in how `Apply` reconciles that desired state with the cells it paints.
  Fixing it in `appendCursorEscapes` is the correct layer — no change needed in `turbotv`,
  `Desktop`, or any widget `Cursor()`.
- **Seam respected:** the turbotui public API is unchanged, so gogent consumes the fix purely
  by version bump. No gogent code change.
- **Cross-repo sequencing:** (a) land turbotui fix — fork + PR to `hobbestherat/turbotui` main;
  turbotui has **no CI**, so the local gate (`go build ./... && go test ./...`) is
  authoritative. (b) record the merge SHA. (c) gogent #454 follow-up:
  `go get github.com/hobbestherat/turbotui@<sha> && go mod tidy`. This design covers the
  turbotui half only; the gogent bump is a separate PR.

---

## Files / functions touched

**turbotui (this repo):**
- `app.go` — `App.Apply()` (compute `cellsWritten := cursorX != -1`, pass it).
- `app.go` — `App.appendCursorEscapes(buf, cellsWritten bool)` (new param; add `&& !cellsWritten`
  to the visible early-out).
- New test file `app_caret_drift_test.go` (or extend `app_drift_test.go`):
  - **happy / keystroke moves caret** → CUP-to-caret + `\x1b[?25h` emitted (already true; lock it in).
  - **unhappy / streaming frame, caret stationary, other cells change** → frame still re-issues
    CUP to the caret coords + `\x1b[?25h` (the core regression guard; fails before the fix).
  - **unhappy / truly empty frame** (no cells, caret unchanged) → still a no-op (optimization kept).
  - **unhappy / caret hidden** → `\x1b[?25l` semantics intact; no spurious show.

**gogent (separate follow-up PR, no code):**
- `go.mod` / `go.sum` — bump `github.com/hobbestherat/turbotui` to the merge SHA via
  `go get …@<sha> && go mod tidy`.

---

## Regression risks (explicit)
- **Streaming-frame byte growth** — intended; quantified above as negligible and flicker-free.
- **Hidden-cursor invalidate defect** (`TestInvalidateReAssertsHiddenCursor`) — pre-existing
  and deliberately left alone; do not let it masquerade as a regression from this change.
- **Test assertions on exact byte counts** — the new streaming re-home adds bytes to frames
  that change cells; any test asserting "only N bytes for a cell change" would need updating.
  None of the current tests assert that (verified), but new tests must assert *presence* of
  the CUP/show, not total length.

---

## Open questions
1. **Re-emit `\x1b[?25h` every streaming frame, or only CUP when already visible?** The issue
   explicitly says "always re-emit the CUP move … and ensure visible," and the existing code
   pairs move+show, so I keep them paired (simplest, matches house style). A micro-optimization
   (emit `?25h` only on a visibility transition) is possible but adds a branch for ~6 bytes/frame;
   I recommend *not* doing it unless a terminal is observed to flicker on repeated `?25h`.
2. **Fold the hidden-cursor invalidate fix in, or keep separate?** Recommend **separate** —
   it's a distinct defect with its own failing test and the issue scopes us to the drift bug.
   Flag for kloune in the PR description so the two aren't conflated.
3. **New test file vs. extending `app_drift_test.go`?** Leaning toward a new
   `app_caret_drift_test.go` to keep the streaming-drift scenario self-documenting; will match
   whatever the reviewer prefers.
