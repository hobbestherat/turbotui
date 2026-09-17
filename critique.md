# Implementation critique

## 1. High — fullscreen `LayoutFn` can self-deadlock by calling `AddLayer`

Files: `turbotv/desktop.go:308-325`, `turbotv/component.go:199-204`

`AddLayer` acquires `mutateMu` at `desktop.go:308` and, for a fullscreen layer, calls `layer.Root.SetBounds` before releasing it. `SetBounds` synchronously invokes the component's user-provided `LayoutFn` at `component.go:202-203`.

Concrete failure scenario:

1. An application creates a fullscreen root with a `LayoutFn` closure that calls `desktop.AddLayer(...)`, for example to create a dependent overlay after laying out the root.
2. It calls `desktop.AddLayer(fullscreenLayer)`.
3. The outer call holds `mutateMu`, reaches `SetBounds`, and invokes `LayoutFn`.
4. The nested `AddLayer` blocks forever at `desktop.go:308` trying to acquire the same non-reentrant mutex.

This worked before the change: no mutex was held while the fullscreen layer's `LayoutFn` ran. The implementation explicitly handles the analogous re-entrant `OnActiveLayerChange` case by releasing the lock, and documents only `DrawFn` as forbidden from calling `AddLayer`; it neither documents nor avoids this new `LayoutFn` restriction. Move the fullscreen `SetBounds`/`LayoutFn` invocation outside the non-reentrant critical section, or otherwise ensure arbitrary layout callbacks are not invoked while `mutateMu` is held.

## 2. Medium — a recovered callback panic permanently leaves `mutateMu` locked

Files: `turbotv/desktop.go:308-328`, `turbotv/component.go:202-203`

The first `mutateMu` critical section uses a manual unlock at `desktop.go:328` despite invoking user code before that point: the injectable clock through `d.now()` and fullscreen `LayoutFn` through `SetBounds`. If either callback panics, the unlock is skipped.

Concrete failure scenario:

1. A fullscreen root's `LayoutFn` panics because of an application layout error.
2. The application has a normal outer recovery boundary and recovers the panic.
3. Any later `desktop.Redraw()` or `desktop.AddLayer(...)` blocks forever because the prior call never released `mutateMu`.

The panic itself belongs to the application, but permanently poisoning the desktop after the application recovers is introduced by this implementation. Structure the critical section so the mutex is released during unwinding (or, preferably, do not execute user callbacks while holding it). This is distinct from the documented recursive-`DrawFn` restriction: the failure occurs from an ordinary layout callback panic without recursively calling a desktop method.

No persisted format is involved in this change. The concurrent layer-stack and paint serialization otherwise matches the task, and the standard local suite is green; these callback paths are the remaining implementation concerns.

CRITIQUE: CONCERNS
