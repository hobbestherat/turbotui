# Critique

## High — Concurrent callbacks can report and finally publish a stale active layer

**File:** `turbotv/desktop.go:338`

`pushLayer` reserves one notification per append under `mutateMu`, but `AddLayer` delivers the resulting closures after releasing the mutex. Two deliveries can therefore overlap and complete in the opposite order from their stack mutations. This contradicts the documented `OnActiveLayerChange` invariant at `desktop.go:435-438`: the callback carries the final top and a handler reading `TopLayer()` observes that same layer. The newer `AddLayer` comment says concurrent callback order is unspecified, but it does not—and cannot without breaking the callback's meaning—redefine the callback argument as a historical layer that may no longer be active.

A deterministic failure scenario uses callback A to wait on a channel before reading `TopLayer()` or publishing derived state. Goroutine 1 adds layer A and enters callback A. While it waits, goroutine 2 adds B, runs callback B, and updates a sidebar selection to B. Callback A then resumes: its argument is A while `TopLayer()` is B, and if it performs the documented use case of re-syncing derived state from its argument, it overwrites the sidebar selection back to A. Both calls return with B actually on top but application state identifying A as active. The implementation and its concurrency test conflate “notify once for every append” with “notify the current active layer”; those are different guarantees when delivery overlaps.

CRITIQUE: CONCERNS
