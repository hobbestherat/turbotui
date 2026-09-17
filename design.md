# Design — Fix the data race on the `turbotv` `AddLayer` path (task 01M2QWWSAS00064PF2ADAWM8CD)

## 0. Status of the specification

This repository has **no `docs/SPEC.md` and no `docs/ROADMAP.md`** (there is no `docs/` directory
at all — verified against the working tree). Per the task brief, the authoritative specification is
therefore the task description plus the repository's own stated conventions — principally the
**Desktop threading contract** comment block at `turbotv/desktop.go:18-29` and the doc comments on
the affected methods. No section numbers or R-numbers are cited below because the repository states
none; where a rule is cited, it is cited by file and line.

No `critique.md` is present in the working tree, so this is a first revision.

## 1. What the race actually is

The brief asks which shared state "(the desktop's layer list and any related counters/maps) is
touched without the desktop lock". Reading the code shows that framing is **partly incorrect**, and
the design has to be built on what is actually there.

`turbotv/desktop.go:270-291`:

```go
func (d *Desktop) AddLayer(layer *Layer) {
	d.layersMu.Lock()
	d.layers = append(d.layers, layer)   // 272 — ALREADY guarded
	d.layersMu.Unlock()
	if layer.window != nil { layer.window.desktop = d }
	if layer.Modal {
		layer.restoreFocus = d.focused   // 281 — unguarded read of d.focused
		if !layer.NoEnterGrace { layer.armedAt = d.now() }  // 283 — unguarded read of d.nowFn
	}
	if layer.FullScreen {
		layer.Root.SetBounds(Rect{..., W: d.app.Width(), H: d.app.Height()})  // 287
	}
	d.notifyActiveLayerChange()          // 289 — unguarded read+write of d.lastNotifiedTop
	d.Redraw()                           // 290 — THE reported racing access
}
```

The layer slice is already mutex-guarded (`layersMu`, `desktop.go:32`; `layerSnapshot`,
`desktop.go:113-119`) — issue #56's work. **No layers are actually lost**: the test's own assertion
(`len(desktop.layerSnapshot()) == n`) passes today without `-race`, which I confirmed by running
the suite on the clean base commit. The failure in the brief is a `-race` *warning*, not a
lost-layer *assertion failure*.

The reported racing access is **`desktop.go:290`, which is `d.Redraw()`** — line number confirmed
against the working tree. `Redraw` (`desktop.go:663-667`) is `compose()` + `updateCursor()` +
`app.Apply()`. Each touches process-wide, entirely unsynchronized state in the parent `turbotui`
package:

| Call | Unsynchronized shared state written |
| --- | --- |
| `compose()` → `d.app.Clear(...)` (`app.go:405-413`) | `a.back.cells` (every cell), `a.lines` (every entry) |
| `compose()` → `layer.Root.Draw(surface)` (`desktop.go:708`) | `a.back` cells, per-widget layout caches |
| `compose()` → `d.refreshMnemonics()` (`desktop.go:698`) | per-widget mnemonic state |
| `updateCursor()` → `SetCursor`/`HideCursor` (`app.go:200-209`) | `a.cursorVisible`, `a.cursorX`, `a.cursorY` |
| `app.Apply()` (`app.go:680-…`) | `a.flushBuf` (`app.go:691`), `a.front.cells` (`app.go:708-715`) |

Plus, inside `turbotv` itself, `notifyActiveLayerChange` (`desktop.go:364-373`) performs an
unguarded read-modify-write of `d.lastNotifiedTop`: 64 goroutines racing on
`if top == d.lastNotifiedTop` / `d.lastNotifiedTop = top` is a write-write race in its own right.

So the race is **not** the layer list. It is that `AddLayer` performs a **full synchronous screen
repaint on the calling goroutine**, and 64 goroutines repainting the same 20×10 grid concurrently
is a textbook race — even though the test's `syncWriter` (`window_issues_test.go:215-228`)
serializes the `io.Writer`, because the writer is only the last of five shared buffers.

`TestConcurrentAddLayerKeepsEveryLayer` (`window_issues_test.go:196-214`) is issue #56's test. Its
*intent* is narrow — prove the slice mutex keeps every layer — and its own comment says so
("targets the desktop's layer mutex rather than racing on the output buffer itself"). Its
*implementation* overreaches by calling the whole of `AddLayer`, which drags the entire paint
pipeline off the event loop.

## 2. Constraints discovered (these bound the solution space)

Each established empirically against the working tree, not by inspection alone.

**C1 — `AddLayer` must repaint synchronously on the calling goroutine.** I replaced `d.Redraw()` at
line 290 with `d.RequestRedraw()` (defer the paint to the loop) and ran
`go test ./turbotv/ -count=1`. Four tests fail:

- `TestCoalescedFlushPaintsFinalStateNotStale` (`desktop_redraw_coalesce_test.go:296`)
- `TestScrollOverDecliningWidgetRequestsNoRedraw` (`desktop_redraw_coalesce_test.go:474`)
- `TestReleaseOnEmptySpaceDefersAndPaintsNothing` (`desktop_redraw_coalesce_test.go:499`)
- `TestModalLayerBlocksMenuClick` (`menu_window_test.go:27`)

The experiment was fully reverted; the tree is clean apart from this file. This kills every "just
post the work to the event loop" design.

**C2 — `OnActiveLayerChange` must fire synchronously before `AddLayer` returns.**
`TestOnActiveLayerChange_FiresSynchronouslyOnEventLoop` (`desktop_active_layer_test.go:494-513`)
asserts exactly this, on a desktop that never calls `Run`, so a posted closure would never execute.
This kills "defer the notify".

**C2b — `OnActiveLayerChange` must fire *before* the repaint it triggers.** This is today's order
(`desktop.go:289` then `:290`). No test currently pins it, but callbacks here are synchronous and
may inspect desktop/app state, so the order is observable and must be preserved. "No test detects
it" is not evidence of compatibility.

**C3 — no `AddLayer` call originates from inside `compose()`/`Draw`.** The only in-package
`AddLayer` callers are `widget_select.go:170` (from `Select.open`, reached only from the key/click
handlers at `widget_select.go:130,134,148`), `widget_colorpicker.go:263`, and
`dialog_helpers.go:115`. None is on a paint path.

**C4 — no caller holds `layersMu` when calling `AddLayer`.** Every `layersMu` critical section in
`desktop.go` (lines 114, 271, 460, 479, 539, 565) is short and calls nothing re-entrant. A new
outer mutex can be ordered strictly *above* `layersMu` with no inversion. This is the brief's
"check existing callers of AddLayer for locks they hold" check: none holds one.

**C5 — a re-entrant `AddLayer` from inside an `OnActiveLayerChange` callback works today.** A
plausible app pattern ("when the top window changes, open a panel"). A lock held across the
callback would silently turn working user code into a hang.

**C6 — no `turbotv` test replaces the desktop's `redrawFn`.** Every `SetRedrawFn` call in the
repository is in the root package's own tests (`app_input_coalesce_test.go`,
`app_input_pipeline_test.go`, `app_issues_test.go:258`) against bare `App`s with no desktop
attached. `turbotv/desktop.go:97` is the only `turbotv` registration. This makes §3.1(4) safe.

## 3. The design

**Serialize the two racy regions of `AddLayer` behind one new `Desktop` mutex, fire the user
callback between them so the existing callback-before-repaint order is preserved, and make
`Desktop.Redraw` self-guarding so the loop's coalesced repaint takes the same lock.**

Given C1 and C2 the repaint and the notify must both happen inline on the calling goroutine; the
only way to make inline work from N goroutines race-free is mutual exclusion. Given C5 the one
piece of arbitrary user code on this path must sit outside the lock. Given C2b it must sit
*between* the state mutation and the repaint. Those three constraints determine the shape: **two
critical sections with the callback in the gap.**

### 3.1 Files, types and functions changed

**Production changes are confined to `turbotv/desktop.go`.** Regression tests are added to
`turbotv/window_issues_test.go` and `turbotv/desktop_active_layer_test.go` (§5). No other file and
no other package is touched.

1. **`type Desktop`** (`desktop.go:30-71`): add one field beside `layersMu`:

   ```go
   // mutateMu serializes the desktop's paint pipeline (compose/updateCursor/Apply)
   // and the mutator bookkeeping that must not interleave with it, so two AddLayer
   // calls from different goroutines cannot both be inside compose/Apply — which
   // write the App's shared back/front cell buffers, flush buffer and cursor state
   // (issue #56). layersMu guards only the slice header; this guards the state reads
   // and the repaint around it.
   //
   // It is deliberately NOT held across OnActiveLayerChange, so that callback may
   // re-enter AddLayer. It IS held across drawing, and therefore across user DrawFn
   // callbacks: a DrawFn must not call AddLayer or Redraw, or it will deadlock.
   //
   // Lock ordering: mutateMu is always acquired BEFORE layersMu, never while holding it.
   mutateMu sync.Mutex
   ```

2. **`notifyActiveLayerChange`** (`desktop.go:364-373`): split in two, with **no behaviour change
   for its five existing callers** (`:289, :469, :496, :554, :766`):

   ```go
   // takeActiveLayerChange updates the last-notified top and reports the new top
   // together with whether it changed and the callback to invoke. It exists so a
   // caller holding mutateMu can do the bookkeeping under the lock and invoke the
   // callback after releasing it.
   func (d *Desktop) takeActiveLayerChange() (top *Layer, cb func(*Layer), changed bool) {
       top = d.TopLayer()
       if top == d.lastNotifiedTop {
           return top, nil, false
       }
       d.lastNotifiedTop = top
       return top, d.onActiveLayerChange, true
   }

   func (d *Desktop) notifyActiveLayerChange() {
       if top, cb, changed := d.takeActiveLayerChange(); changed && cb != nil {
           cb(top)
       }
   }
   ```

   Returning `cb` (rather than reading `d.onActiveLayerChange` outside the lock) keeps that field
   read inside the critical section too. This preserves the documented quirk that `lastNotifiedTop`
   is updated even when no callback is registered (`desktop.go:359-363`).

3. **`AddLayer`** (`desktop.go:270-291`): two critical sections, callback in the gap.

   ```go
   func (d *Desktop) AddLayer(layer *Layer) {
       d.mutateMu.Lock()
       d.layersMu.Lock()
       d.layers = append(d.layers, layer)
       d.layersMu.Unlock()
       if layer.window != nil { layer.window.desktop = d }
       if layer.Modal {
           layer.restoreFocus = d.focused
           if !layer.NoEnterGrace { layer.armedAt = d.now() }
       }
       if layer.FullScreen {
           layer.Root.SetBounds(Rect{X: 0, Y: 0, W: d.app.Width(), H: d.app.Height()})
       }
       top, cb, changed := d.takeActiveLayerChange()
       d.mutateMu.Unlock()

       if changed && cb != nil {
           cb(top)          // outside the lock: preserves C5 re-entrancy …
       }
       d.Redraw()           // … and C2b callback-before-repaint order.
   }
   ```

   The body is otherwise verbatim. Nothing moves except the lock boundaries.

4. **`Redraw`** (`desktop.go:663-667`) becomes self-guarding, and the `redrawFn` registered in
   `NewDesktop` (`desktop.go:97-101`) is routed through it:

   ```go
   func (d *Desktop) Redraw() {
       d.mutateMu.Lock()
       defer d.mutateMu.Unlock()
       d.compose()
       d.updateCursor()
       _ = d.app.Apply()
   }
   ```
   ```go
   app.SetRedrawFn(func() { desktop.Redraw() })   // was: compose(); updateCursor(); Apply()
   ```

   The `redrawFn` body at `desktop.go:98-100` is **byte-for-byte identical** to `Redraw`'s, so the
   *paint sequence* is de-duplicated and the two paint paths can no longer drift apart. But this is
   **not** a behaviour-neutral refactor, and should not be described as one: it introduces a new
   critical section around the entire paint pipeline — `compose()` included, and therefore around
   arbitrary user `DrawFn` callbacks. Two things change:

   - **Synchronization:** `AddLayer`'s repaint and the run loop's coalesced repaint now serialize
     against each other instead of racing. This is the point of the change.
   - **Re-entrancy:** a `DrawFn` that calls `Redraw` or `AddLayer` now deadlocks, where today it
     recurses until the stack overflows. Both are bugs and neither is a supported pattern (C3
     confirms no in-package path does it), but the failure mode changes. This is the accepted
     trade-off, documented on the `Redraw` doc comment, on the `mutateMu` field, and as R2.

   C6 shows no `turbotv` test overrides this `redrawFn`, so frame *accounting* is unaffected: the
   new body performs exactly the same single compose+Apply the old one did.

5. **Doc comments** on `AddLayer` (`desktop.go:263-269`), `Redraw` (`desktop.go:662`) and the
   threading-contract block (`desktop.go:18-29`): state the narrow, implemented guarantee only, and
   state the new `DrawFn` constraint on `Redraw` — see §3.3.

### 3.2 Why this removes the race

Every shared location enumerated in §1 — `d.focused`, `d.nowFn`, `d.lastNotifiedTop`,
`d.onActiveLayerChange`, `a.back`, `a.front`, `a.lines`, `a.flushBuf`, `a.cursor*` — is accessed
inside one of the two `mutateMu` critical sections. The only code between them is the user
callback, which in `TestConcurrentAddLayerKeepsEveryLayer` is unregistered (no callback ⇒ `cb` is
nil ⇒ the gap is empty). The mutex establishes happens-before edges between consecutive `AddLayer`
calls, so the detector sees no unsynchronized concurrent access — the argument is over the
enumerated set, not over an observed run (see §5.1 for why that distinction matters here).

The two sections are *not* atomic with respect to each other: between them another `AddLayer` may
complete. That is harmless — the second section is a full recompose from current state, so the
final screen reflects every layer added, and a redundant identical repaint writes the same bytes.
Under the supported single-goroutine contract the interleaving cannot occur at all.

### 3.3 The guarantee that will be documented (and its exact limits)

The doc comments will **retain** the existing "call it on the event loop or via `Post`" requirement
and add only this:

> The layer-slice update, the desktop-state reads around it and the repaint it performs are each
> protected against a concurrent off-loop `AddLayer`, so concurrent `AddLayer` calls cannot corrupt
> the stack or interleave inside the paint pipeline (issue #56). This is **not** general
> thread-safety: `AddLayer` still races with the input handlers, `SetFocus`, `RemoveLayer`,
> `RaiseLayer` and every other desktop mutator, none of which take this lock. Callers must still
> use the event loop or `Post` while `Run` is active.

and on `Redraw`:

> A `DrawFn` invoked during compose runs with the desktop's paint lock held and must not call
> `Redraw` or `AddLayer`. (Doing so already recursed without bound; it now deadlocks instead.)

Specifically **not** claimed: that `AddLayer` is "safe to call from any goroutine", or that the
whole mutate-and-repaint operation is atomic. Claiming either would document behaviour the
implementation does not provide, since no competing *mutator* takes `mutateMu`.

### 3.4 Concurrent `OnActiveLayerChange` semantics (specified, not left implicit)

Because the callback is delivered outside the lock, concurrent `AddLayer` needs a stated rule:

> With N goroutines each adding a distinct layer and a callback registered, the callback is invoked
> **exactly N times — once per added layer, each receiving that layer**. Delivery *order* is
> unspecified: the invocation for a layer added earlier may arrive after one for a layer added
> later, and two invocations may overlap in time. Callers needing ordered notification must observe
> the existing contract and call `AddLayer` on the event loop or via `Post`, where delivery is
> strictly ordered and never overlaps.

Exactly-N is a *guarantee*, not an accident: a goroutine holds `mutateMu` across both the append
and `takeActiveLayerChange`, so `TopLayer()` necessarily returns the layer it just appended, which
necessarily differs from `lastNotifiedTop`. Each of the N calls therefore reserves exactly one
notification carrying its own layer.

Unordered/overlapping delivery is an accepted, documented limit rather than a defect, because
`OnActiveLayerChange` is loop-confined by the same contract that governs `AddLayer`
(`desktop.go:20-25`, `desktop.go:344-346`). Making delivery both ordered *and* re-entrant-safe
requires either per-goroutine ownership tracking (Go has no portable goroutine identity) or a
ticket lock that deadlocks on re-entrancy — cost far out of proportion to a race fix, for a
configuration the repository does not support. §8 Q2 records this if a maintainer wants it
revisited.

### 3.5 Lock ordering

`mutateMu` → `layersMu`. `AddLayer` acquires `mutateMu`, then `layersMu` (the append, plus each
`layerSnapshot`/`TopLayer` reached from `compose`/`takeActiveLayerChange`). By C4 nothing ever
acquires `mutateMu` while holding `layersMu`, so no inversion is possible. The rule lives in the
field comment so it survives future edits.

### 3.6 Alternatives considered and rejected

- **Post the repaint to the event loop** (`RequestRedraw`, or `d.app.Post`). Architecturally
  cleanest and what the contract nominally prescribes — **rejected by C1**: breaks four tests.
- **Post the notify to the loop** — **rejected by C2**.
- **Guard only `lastNotifiedTop`.** Fixes one of six raced locations; the `compose`/`Apply` buffer
  races at :290 — the actually-reported one — remain. A non-fix.
- **One flat critical section covering mutation + callback + repaint** (~5 lines shorter).
  **Rejected by C5**: deadlocks a re-entrant callback, silently converting working user code into a
  hang.
- **One flat critical section with the callback fired after unlocking.** Avoids the deadlock but
  **rejected by C2b**: it reverses the callback/repaint order, an observable API change this
  narrowly scoped race fix has no business making.
- **Take the documented-unsupported branch** (document concurrent `AddLayer` as unsupported, make
  the test single-goroutine). The hatch *is* open — `AddLayer`'s doc at `desktop.go:263-264` says
  "Must be called on the event loop or via Post", and the contract block at `desktop.go:20-25`
  names `AddLayer`. **Rejected anyway**: the same paragraph (`desktop.go:26-29`) says the stack "is
  additionally guarded by a mutex so that an off-loop `AddLayer`/`RemoveLayer` cannot corrupt the
  slice" — issue #56 deliberately hardened this exact path, and this is #56's own test. Removing
  the concurrency from it would undo the hardening it exists to protect. The brief's stated default
  (fix the race, not the test) and #56's intent agree, so the default is taken.
- **Extend `mutateMu` to `RemoveLayer`/`RemoveTopLayer`/`RaiseLayer`** (identical
  mutate-then-`Redraw` shape at `:459-471`, `:475-498`, `:551-557`) — see §4.

## 4. Deliberately NOT in scope

- **`RemoveLayer`, `RemoveTopLayer`, `RaiseLayer`, `SetFocus`, `SetTheme`, `SetWorkArea`,
  `handleClick`.** They share `AddLayer`'s shape and are equally unsafe off-loop, but none is
  covered by a concurrent test and the brief scopes this task to `AddLayer`. Adding untested locks
  to five more methods enlarges the deadlock surface without evidence. (Their *repaints* do now
  serialize, via §3.1(4); their *state mutations* do not.) The asymmetry is recorded in §8 Q1
  rather than silently fixed.
- **Any change to the parent `turbotui` package** (`app.go`, `cell.go`, …). `App`'s cell buffers
  and `a.dirty` flag are genuinely unsynchronized, but making `App` thread-safe is a much larger
  redesign than this race needs. It is excluded because it is **unnecessary** — §3.2 shows the race
  is fully closed from inside `turbotv` — and because widening a narrow race fix into a
  parent-package concurrency redesign raises long-term maintenance cost for no gain here.
- **Making the whole desktop thread-safe / removing loop-confinement.**
- **Changing `TestConcurrentAddLayerKeepsEveryLayer`'s concurrency or its assertion.** It stays 64
  goroutines and keeps asserting all 64 layers survive.
- **Ordered concurrent `OnActiveLayerChange` delivery** (§3.4).
- **Any behavioural change to z-order, focus restoration, Enter-grace, or coalesced-redraw
  accounting.**

## 5. Test plan (deterministic — no network, no sleeps, no wall-clock, no live anything)

`TestConcurrentAddLayerKeepsEveryLayer` is the primary test and is kept **verbatim**. It is
deterministic in the way that matters: no sleeps, no clock reads, no timing assertions; ordering via
`sync.WaitGroup`, and an exact assertion. Race *detection* is inherently schedule-dependent, which
is why `-count=5` is in the acceptance criteria — the repository's own mitigation, kept.

Additions (all hermetic: in-process, buffer-backed `App`, no I/O, no goroutine sleeps):

1. **`turbotv/window_issues_test.go` — `TestConcurrentAddLayerNotifiesEachLayerExactlyOnce`.**
   Register an `OnActiveLayerChange` callback that, under its own `sync.Mutex`, increments a
   per-layer counter in a `map[*Layer]int`. Add N=64 distinct layers from N goroutines;
   `wg.Wait()`. Assert:
   - total invocations `== N` (exactly, per §3.4 — **not** a loose `>= 1`);
   - the map has exactly N entries and every count is exactly 1;
   - the key set equals the set of layers added (one-to-one; no stale or duplicated argument);
   - `len(layerSnapshot()) == N`, and `TopLayer()` is *a member of* the added-layer set;
   - `d.lastNotifiedTop == d.TopLayer()` — the bookkeeping converged on the final top.

   Note on the last two: the tempting stronger assertion, "`TopLayer()` is the layer carried by the
   *last* invocation received", is **wrong and would flake even on a correct implementation**.
   Delivery happens outside `mutateMu`, so goroutine A can reserve notification A, B can reserve and
   deliver B, and A can then deliver last — final top B, last-arriving callback A. It is excluded
   deliberately. `lastNotifiedTop == TopLayer()` is the strongest *schedule-independent* statement
   about final bookkeeping, and it is genuinely guaranteed: whichever goroutine holds `mutateMu`
   last in the first critical section sets `lastNotifiedTop` to the layer it just appended, and no
   append can follow, so that layer is the final top. It is reachable because these are internal
   (`package tv`) tests.

   Delivery **order** is not asserted (§3.4). Every assertion above is schedule-independent, so the
   test is deterministic despite exercising concurrency. It is also the test that fails if a future
   edit collapses the bookkeeping split and loses notifications.

2. **`turbotv/desktop_active_layer_test.go` — two single-goroutine ordering/re-entrancy tests:**
   - `TestOnActiveLayerChange_MayAddLayerReentrantly` (pins C5): a callback that adds one extra
     layer the first time it fires; assert `AddLayer` returns and both layers are present. Fails as
     a hang — caught by the Go test timeout — if a future edit pulls the callback inside the lock.
   - `TestOnActiveLayerChange_FiresBeforeAddLayerRepaints` (pins C2b, which nothing pins today):
     add a layer whose root paints a known marker cell; inside the callback read that cell via
     `app.ReadCell` and record it; after `AddLayer` returns, assert the callback observed the
     **pre-repaint** cell and the post-return screen shows the marker. This codifies the ordering so
     the next person cannot silently flip it.

3. **Existing suites that must stay green, unchanged:** `desktop_active_layer_test.go` (all
   `OnActiveLayerChange` ordering/dedupe assertions, especially
   `TestOnActiveLayerChange_FiresSynchronouslyOnEventLoop`), `desktop_redraw_coalesce_test.go`
   (frame accounting — the C1 canaries), `menu_window_test.go`, `menu_mnemonic_test.go` (calls
   `desktop.compose()` directly, which stays unlocked), `desktop_modal_guard_test.go` (Enter-grace
   still reads the injectable clock), `widget_select_test.go`, `widget_colorpicker_test.go`.

**Commands (the acceptance gate):**

```
go test ./turbotv/ -race -count=5
go test ./... -race -count=1
go test ./...
go vet ./...
```

### 5.1 Verification caveat — `-race` cannot run on this development host

A caveat, not a blocker: it gates neither this design nor its implementation. The race detector does
not run on this box at all:

```
$ go test ./turbotv/ -race -count=1
FATAL: ThreadSanitizer: unsupported VMA range
FATAL: Found 47 - Supported 48
```

This is `aarch64` (Raspberry Pi, `Linux 6.18.39+rpt-rpi-2712`, Go 1.26.4); Go's bundled
ThreadSanitizer needs a 48-bit user VMA and this kernel provides 47. Not a missing-`gcc` problem and
not fixable from userspace — `setarch -R` does not help (ASLR is not the cause) and there is no
`docker`/`podman` available. Every `-race` build fails this way on every package, including on the
base commit, so the original warning can be neither reproduced nor locally disproved here.

How confidence is obtained instead:

- The reported race is **not** taken at face value. It is re-derived from source: `desktop.go:290`
  is `d.Redraw()`, and §1 enumerates the specific unsynchronized memory it reaches. §3.2 then shows
  every enumerated access ends up inside a critical section — race-free by construction rather than
  "green on my machine".
- Everything verifiable locally will be verified locally: `go test ./... -count=1` (green on the
  clean base commit, confirmed) and `go vet ./...` (green, confirmed), plus the new tests run at
  `-count=20` without `-race` to shake out non-race regressions under repeated concurrent execution.
- `go test ./turbotv/ -race -count=5` and `go test ./... -race -count=1` must be run on a supported
  host (amd64, or an arm64 kernel with 48-bit VMA) before merge. §8 Q4 asks where that is, as
  information rather than as a gate.

## 6. Regression risks — existing behaviour that must not change

| # | Risk | Why it could break | Mitigation / check |
| --- | --- | --- | --- |
| R1 | **Deadlock via re-entrancy.** | `sync.Mutex` is not reentrant; any path reaching `AddLayer` or `Redraw` while `mutateMu` is held hangs. | C3: no in-package path does. The callback — the one plausible case (C5) — is fired between the two critical sections, and §5 test (2a) locks that in. |
| R2 | **`Redraw` is now self-locking: a user `DrawFn` that calls `Redraw` deadlocks instead of overflowing the stack.** | `compose()` runs under `mutateMu` and invokes user `DrawFn`s. | Both are bugs and neither is supported (today it recurses infinitely). The failure mode changes from crash to hang; called out in the `Redraw` doc comment (§3.3), the `mutateMu` field comment and §3.1(4). |
| R3 | **Lock-ordering inversion with `layersMu`.** | `AddLayer` nests `layersMu` inside `mutateMu`. | C4: no site acquires `mutateMu` while holding `layersMu`. Rule documented on the field. |
| R4 | **Over-reading the guarantee.** | `mutateMu` makes `AddLayer` safe against `AddLayer`, and its repaint safe against the loop's repaint — not against input handlers or the other mutators, which do not take it. | §3.3 states the limit precisely; the "call it on the loop or via `Post`" requirement is retained verbatim, and the PR description will repeat it. |
| R5 | **Coalesced-redraw frame accounting changes.** | `desktop_redraw_coalesce_test.go` counts frames; §3.1(4) rewrites the `redrawFn`. | The new `redrawFn` body is byte-for-byte what `Redraw` already does, so paint count is unchanged; C6 confirms no `turbotv` test replaces it. These tests are the C1 canaries and must stay green. |
| R6 | **`go vet` copylocks.** | A second `sync.Mutex` in `Desktop`. | `Desktop` already holds `layersMu`, so it is already non-copyable; `go vet ./...` covers it. |
| R7 | **Callback ordering under concurrency.** | Delivery outside the lock permits reordered/overlapping callbacks. | Not a change in *supported* behaviour (single-goroutine order is unchanged, pinned by §5 test 2b). Specified in §3.4, exactly-once asserted by §5 test (1), flagged in §8 Q2. |
| R8 | **Throughput under contention.** | 64 goroutines serialize through a full compose+Apply each. | Bounded and tiny at 20×10; correctness outranks it, and supported usage is single-goroutine. |
| R9 | **Modal reads under lock** (`d.focused`, `d.now()`). | `d.now()` invokes the injectable `nowFn` inside the critical section. | Test clocks return a time and do not call back into the desktop (`SetClock`, `desktop.go:166-168`). `desktop_modal_guard_test.go` covers it. |

## 7. Responses to critique

**Point 1 — the `mutateMu` field comment was false about `DrawFn`. Accepted; the critique is
exactly right and the contradiction was mine.** `Redraw` holds `mutateMu` across `compose()`, and
`compose()` calls `layer.Root.Draw(surface)`, which invokes user `DrawFn` code — so the lock *is*
held across every `DrawFn`, directly contradicting the field comment while R2 said the opposite.
The comment in §3.1(1) now reads: not held across `OnActiveLayerChange` (so that callback may
re-enter `AddLayer`); **held** across drawing and therefore across `DrawFn`, which must not call
`AddLayer` or `Redraw`. I also propagated the constraint to §3.3, so it appears in the user-facing
`Redraw` doc comment rather than living only in an internal field comment — a `DrawFn` author is
the person who needs to read it.

**Point 2 — "pure de-duplication" / "does not enlarge any critical section" was inaccurate.
Accepted.** Routing the loop's `redrawFn` through the self-locking `Redraw` de-duplicates the paint
*sequence*, but it does create a new critical section around the whole paint pipeline including
arbitrary draw callbacks. §3.1(4) no longer claims neutrality; it now names both changes explicitly
— synchronization (the two paint paths serialize, which is the intent) and re-entrancy (an invalid
recursive draw changes from stack overflow to deadlock, the accepted trade-off) — and keeps the
narrower true claim that frame *accounting* is unaffected because the new body performs the same
single compose+Apply.

**Point 3 — duplicated sentence in §3.2. Acted on, with a correction.** There is no literal
duplicate inside §3.2; I re-read the section and grepped the file, and the phrase occurs once
(the `**by\nconstruction**` bold spanning a line break may have read as one). The underlying
observation is fair, though: the "race-free by construction, not by observation" claim was made
twice in the document — once in §3.2 and again in §5.1. I removed the §3.2 instance and left it in
§5.1, where it earns its place as the answer to "you cannot run `-race` locally, so why should
anyone believe this?"

**Also fixed, not raised in the critique:** the cross-references to "§8 Q1 / Q2 / Q4" were stale —
Open questions was §7. Inserting this section as §7 and renumbering Open questions to §8 makes
every one of them resolve correctly.

**No other changes.** The critique states no further architectural change is needed, and I have not
altered the lock design, the two-critical-section structure of `AddLayer`, the callback placement,
the documented guarantee, the test plan, or the open questions.

## 8. Open questions

1. **Should the sibling mutators get the same treatment?** `RemoveLayer`, `RemoveTopLayer` and
   `RaiseLayer` have the identical mutate-then-`Redraw` shape and the same state race (their
   repaints are now serialized by §3.1(4); their mutations are not). Fixing only `AddLayer` leaves
   an asymmetry in the API's safety story; fixing all four quadruples the deadlock surface and goes
   beyond the brief. Recommendation: ship `AddLayer` now, file a follow-up with per-method
   concurrency tests. Flagged rather than decided unilaterally, since it is a scope call.

2. **Is unordered concurrent `OnActiveLayerChange` delivery acceptable long-term?** §3.4 specifies
   and tests exactly-once delivery but explicitly not order. Ordered *and* re-entrant-safe delivery
   is not achievable with a plain mutex in Go. If a maintainer wants ordered delivery, the realistic
   route is to declare `OnActiveLayerChange` deliverable only on the loop and have off-loop
   `AddLayer` enqueue the notification via `d.app.Post` — which C2 forbids for the on-loop case, so
   it would need a loop-detection mechanism the codebase does not have.

3. **Is `AddLayer`'s synchronous repaint intended API, or an accident the tests have ossified?** C1
   shows four tests depend on it, yet the contract's spirit (coalesce paints on the loop,
   `desktop.go:95-101`, issue #17) suggests `RequestRedraw` is the better primitive. If the
   synchronous paint is incidental, the much cleaner fix is to defer the paint and update those four
   tests — which would remove the need for `mutateMu` entirely. **Not assumed**, because rewriting
   four green tests is close to the "weaken the tests" move the brief rules out, but worth a
   maintainer's opinion.

4. **Where does the `-race` gate run?** §5.1: it cannot run on this host. This is a question about
   CI topology, not a blocker on this design. If the gate runs on this same hardware then the stated
   definition of done is unreachable for every turbotui task, and the fix belongs in the gate
   environment rather than in this code.

5. **What was the *other* racing access in the original report?** Only "racing access at
   `turbotv/desktop.go:290`" is available from the brief, and the report cannot be reproduced here
   (§5.1). §1 enumerates every shared location on the path, so the fix should subsume whatever the
   second stack was — but pasting the full report into the PR would let this be confirmed rather
   than inferred.

## 9. Deviations from this design, forced by implementation-stage critique

Two rounds of critique found defects in callback paths §2 did not enumerate, and the fixes take the
implementation outside what §3.1 scoped. Recorded here so the design and the code agree.

**9.1 — `LayoutFn` is a second piece of arbitrary user code on the `AddLayer` path.** §3 reasoned
that "the one piece of arbitrary user code on this path" is `OnActiveLayerChange` (C5). That is
wrong: a `FullScreen` layer's `SetBounds` synchronously invokes the root's `LayoutFn`, which may
call `AddLayer` just as plausibly as C5's callback may. Holding `mutateMu` across it self-deadlocks.
The `LayoutFn` invocation therefore also runs outside the lock, and both critical sections unlock
via `defer` so a panic out of a callback (including the injectable clock, the one user callback that
remains inside) cannot leave the desktop permanently locked once the application recovers it.

**9.2 — the notification is delivered before that `LayoutFn`, and `setBoundsNoLayout` splits
`SetBounds` so this costs nothing observable.** §3.4's exactly-once guarantee requires the
notification to be *reserved* under `mutateMu`, which is precisely what the pre-lock implementation
did not do: it recomputed `TopLayer()` at delivery time, so a nested `AddLayer` from a `LayoutFn`
self-corrected (the outer call found `top == lastNotifiedTop` and correctly emitted nothing).
Reserving early loses that self-correction — the outer call would report the background as active
while the overlay its own `LayoutFn` just added is the real top. The two properties cannot both
hold: ordered-and-re-entrant delivery needs goroutine identity (§3.4). Exactly-once wins, so any
user code able to add a layer must run *after* delivery.

Deferring the whole `SetBounds` past the notification would do that, but would regress what a
callback observes: the new top's root would still carry its pre-stretch bounds. Instead
`VisualComponent.setBoundsNoLayout` (`component.go`, the sole production change outside
`desktop.go`) applies the bounds change and hands back the `LayoutFn` invocation. `pushLayer`
stretches the root under the lock — which also puts that write inside the paint lock, so a
concurrent repaint cannot compose against a half-written `Rect` — and `AddLayer` delivers the
notification, then runs the `LayoutFn`. `SetBounds` is re-expressed in terms of the split, so it
remains the single definition of what setting bounds does.

Residual observable change, accepted and documented on `AddLayer`: a callback for a fullscreen layer
sees the stretched root but not yet whatever its `LayoutFn` does to the children. That is the
narrowest available consequence of giving up delivery-time recomputation.

**9.3 — the deferred layout closure pins the function it checked.** Splitting `SetBounds` in 9.2
puts application code (the notification) between the moment `setBoundsNoLayout` nil-checks
`LayoutFn` and the moment the returned closure calls it, and that callback is handed the layer, so
it can reach `top.Root.LayoutFn`. A closure that re-reads the field calls a nil function — a panic —
when the callback clears it, and silently runs a substitute when the callback replaces it. The
closure therefore captures the checked function value in a local and calls that. The gap is created
by this design, so closing it is part of it: `SetBounds` never had the gap, because it checked and
called in consecutive statements.

**9.4 — `mutateMu` serializes the desktop's state, not the application's callbacks.** §2 framed the
hazard as "two `AddLayer` calls corrupting the stack or interleaving inside the paint pipeline", and
that is what the lock fixes. It does not make application callbacks mutually exclusive, and cannot.
`VisualComponent.Draw` invokes `LayoutFn` and `DrawFn` for every visible component on every paint
(`component.go:305`), so a second goroutine's `AddLayer` reaches, under `mutateMu`, the same user
callbacks a first goroutine may be running outside it — whether the layer is fullscreen or not.
Reproduced: park goroutine 1 inside a fullscreen root's `LayoutFn`, call `AddLayer` for any second
layer from goroutine 2, and that root's `LayoutFn` is in flight twice.

Closing it would mean holding `mutateMu` across a callback that is free to re-enter `AddLayer` —
the self-deadlock 9.1 was forced to remove, and which `TestAddLayerFullScreenLayoutFnMayAddLayer`
now pins. The two cannot both hold. This is also not a regression: before the task `SetBounds` ran
with no lock at all (`desktop.go:286-288` at `4151a29`) and `Redraw` took none, so the overlap
predates the lock; 9.2 narrowed it by moving the `Rect` write inside the critical section, leaving
only the callback bodies outside.

The residual is therefore documented rather than fixed, on both `mutateMu` and `AddLayer`: the lock
protects desktop state, and a callback reached from two goroutines is safe only if the callback
itself is — one more reason the threading contract requires `Post`. The disclosure 9.2's doc rewrite
dropped ("a repaint racing it may compose the layer mid-layout") is restored in that stronger form.
`TestConcurrentAddLayerFullScreenWithLayoutFnKeepsEveryLayer` covers the fullscreen-with-`LayoutFn`
form of the concurrency class the target test leaves untouched, asserting the guarantees that do
hold — no lost layer, exactly-once notification, every root stretched, bookkeeping converged — and
using an atomic counter to model the rule the callback itself must follow.

**9.5 — notify-once-per-append and argument-is-the-live-top cannot both hold off the loop.** The
critique is right that a parked handler can publish a stale active layer: goroutine 1 adds A and
blocks inside the callback, goroutine 2 adds B and runs its callback to completion, goroutine 1
resumes and re-syncs derived state from its argument A while `TopLayer()` is B. Reproduced exactly
as described.

It is not fixable alongside what is already pinned. §3.4's exactly-once guarantee
(`TestConcurrentAddLayerNotifiesEachLayerExactlyOnce`) forces n concurrent adds to produce n
callbacks, so n-1 of the arguments name a layer that is not the final top. For each of those to
satisfy `argument == TopLayer()` when the handler reads it, the next append must be excluded for
the duration of the callback — a lock held across application code free to re-enter `AddLayer`,
which is the self-deadlock 9.1 removed and which `TestOnActiveLayerChange_MayAddLayerReentrantly`
and `TestAddLayerFullScreenLayoutFnMayAddLayer` now forbid. Satisfying all three at once needs a
re-entrant lock keyed on goroutine identity, which §3.4 already rejected and Go does not offer. The
other escape, recomputing the argument at delivery, is the pre-lock behaviour 9.2 gave up: it
delivers n callbacks all naming the final top, which breaks exactly-once.

So the defect is in what the doc claimed, not in what the code does. `OnActiveLayerChange` stated
"a handler reading `TopLayer()` observes the new top" without a qualifier, so it read as a promise
that survives off-loop concurrency; the qualification lived only in `AddLayer`'s comment, which is
not where a reader of the callback contract looks. Both are now explicit: the guarantee is
loop-scoped — on the loop, mutations and deliveries strictly alternate and the argument IS the live
top, which is what makes the sanctioned "re-sync derived state from the argument" use sound — and
off the loop the contract does not apply rather than applying in a weakened form.

`TestAddLayerFullScreenLayoutFnNotifiesInTopOrder` already pins the loop-scoped property, asserting
`argument == TopLayer()` on entry to every delivery, including a re-entrant one. On entry is the
exact claim: a handler that itself adds a layer displaces the top for the rest of its own body,
sequentially and by its own action, and the nested add delivers its own notification first. No test asserts the
concurrent case, deliberately: its outcome is a race between two goroutines, and a test that pinned
either resolution would pin non-determinism.
