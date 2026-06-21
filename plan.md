# Plan — gogent#239: coalesce input-event redraws (turbotv)

## Problem
Button-event mouse tracking (`?1002h`) emits one motion event per cell crossed.
A 256-byte read carries 10–20 motion events. Each turbotv input handler
(`handleClick`, `handleScroll`, `handleType`, `handlePaste`) calls
`Desktop.Redraw()` directly — a full synchronous `compose()` + blocking
`App.Apply()` terminal flush. So one read batch = 10–20 full redraws + blocking
writes; the event queue drains slower than it fills → the dragged window trails
the cursor, lag compounding the faster/longer you drag.

## Existing machinery (reuse it)
turbotv already coalesces the streaming/`Post` path:
- `App.RequestRedraw()` sets a `dirty` flag (no paint).
- The `Run` loop drains a whole burst of events/posts, then calls `flushDirty()`
  **once per iteration**, which runs `redrawFn` (the desktop's compose + Apply).
- `Desktop.Post` already routes through this (`app.RequestRedraw()`).

The input handlers bypass it by calling `Redraw()` immediately.

## Fix — Option A (primary, low risk)
Route the four input-event handlers through the coalescing path instead of an
immediate flush.

1. Add `Desktop.RequestRedraw()` — a thin, documented wrapper over
   `d.app.RequestRedraw()` that marks the desktop dirty so the loop coalesces a
   burst of input events into a single compose + Apply per iteration.
2. In `turbotv/desktop.go`, replace every `d.Redraw()` inside `handleClick`,
   `handleScroll`, `handleType`, `handlePaste` with `d.RequestRedraw()`.
3. Keep `Desktop.Redraw()` for the structural/state callers that intentionally
   flush synchronously (`AddLayer`, `RemoveLayer`, `RemoveTopLayer`,
   `RaiseLayer`, `SetWorkArea`, `ResetWorkArea`, `handleResize`). These are not
   on the mouse-motion hot path and several need an immediate repaint.

### Why correctness is preserved
The `Run` loop calls `flushDirty()` once at the end of **every** iteration,
including the `readChannel` case that dispatches input events. So:
- A burst of N motion events in one read → N `RequestRedraw()` calls (set the
  same flag) → exactly **one** compose + Apply for the batch.
- A single click/keystroke still sets the flag → flushed once that iteration →
  no "lost final frame".
- Nothing in these handlers reads back painted state before the iteration ends,
  so deferring the paint to `flushDirty()` changes timing, not behavior.

## Option B (frame-rate cap) — deferred to follow-up
The issue also recommends a ~60 fps cap in `flushDirty`/`Apply`
(`lastFlush time.Time`, skip+re-arm if `<16ms`). A correct implementation must
re-arm a **timer** in the `Run` select so the trailing frame after a burst is
not lost — that touches the core event loop, adds time-based behavior that is
hard to verify deterministically, and risks exactly the "lost final frame"
regression this task flags as critical. Option A alone stops the queue growth
(one flush per read batch regardless of event count), which resolves the
"extremely slow" drag. B is left as a documented follow-up.

## Tests (written by GLM partner — seam notes)
- Coalescing: register a counting `app.SetRedrawFn`, dispatch a burst of N
  click/motion events through the desktop's handlers, call `app.flushDirty()`,
  assert the counter == 1 (mirrors `TestRequestRedrawCoalesces`).
- Single event still paints: one event → `flushDirty()` → counter == 1.
- No regression: `go test ./...` green.

## Constraints
turbotv only; no new deps; gofmt clean; golangci-lint 0; `go test ./...` green.
