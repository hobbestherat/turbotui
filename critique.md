# Critique

## 1. High — deferred layout closure can call a nil or replaced `LayoutFn`

File: `turbotv/component.go:219-222`

`setBoundsNoLayout` checks that `c.LayoutFn` is non-nil, but the returned closure does not capture that function value; it reads `c.LayoutFn` again when invoked. `Desktop.AddLayer` deliberately runs `OnActiveLayerChange` between those two moments (`desktop.go:328-332`), and that application callback may mutate the new layer, including its public `LayoutFn` field.

Concrete failure scenario: create a fullscreen layer whose root has a non-nil `LayoutFn`, register `OnActiveLayerChange(func(top *Layer) { top.Root.LayoutFn = nil })`, and call `AddLayer`. `setBoundsNoLayout` returns a non-nil closure, the notification clears the field, and then the closure executes `c.LayoutFn(c)` and panics by calling a nil function. Replacing the field instead silently invokes the replacement even though the original function was the one observed when layout was scheduled. Capture the checked function in a local and close over that value so notification-time mutation cannot invalidate the deferred call.

CRITIQUE: CONCERNS
