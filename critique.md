# Critique

## 1. High — re-entrant fullscreen layout reports a stale layer as active

File: `turbotv/desktop.go:321-325`

`pushLayer` records the outer layer as `lastNotifiedTop` and returns a closure for notifying it, but `AddLayer` runs the fullscreen layer's `LayoutFn` before invoking that closure. If the layout callback calls `AddLayer` for an overlay, the nested call pushes and notifies the overlay first. When control returns, the outer call then invokes its saved closure and reports the background as active even though the overlay remains the actual top layer.

Concrete failure scenario: register `OnActiveLayerChange`, create a fullscreen background whose `LayoutFn` calls `d.AddLayer(overlay)` once, then call `d.AddLayer(background)`. The callback sequence is `overlay, background`, while `d.TopLayer()` is `overlay`; after the final callback, an observer therefore believes the inactive background is the active layer. This violates the documented behavior at `desktop.go:310-312` that the callback observes the new top. The new `TestAddLayerFullScreenLayoutFnMayAddLayer` does not install a callback, so it does not detect the incorrect notification ordering.

CRITIQUE: CONCERNS
