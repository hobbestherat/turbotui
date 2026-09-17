# Critique

## High — Concurrent fullscreen adds can invoke one layer's `LayoutFn` simultaneously

**File:** `turbotv/desktop.go:332`

`pushLayer` releases `mutateMu` before the deferred `layout` closure is invoked. While that closure is still running, a second goroutine can complete another `AddLayer`, enter its locked `Redraw`, and have `compose` call `SetBounds`/`LayoutFn` on the first fullscreen layer. The same callback can therefore execute concurrently even though the implementation states that `mutateMu` makes `AddLayer` safe against another `AddLayer` and protects the paint-related state reads and writes.

A concrete failure scenario is a fullscreen layer A whose `LayoutFn` increments ordinary widget layout state and then waits on a channel. Goroutine 1 calls `AddLayer(A)` and blocks inside that first `LayoutFn` invocation. Goroutine 2 calls `AddLayer(B)` for any layer; its final `Redraw` composes A and invokes A's `LayoutFn` again while goroutine 1 is still in it. The two invocations race on A's layout state (and can produce a lost increment or a race-detector failure). This means the fix does not cover the fullscreen form of the exact AddLayer-vs-AddLayer concurrency class it claims to cover; the current target test misses it because all 64 layers are non-fullscreen and have no `LayoutFn`.

CRITIQUE: CONCERNS
