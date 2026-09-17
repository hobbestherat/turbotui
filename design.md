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
   // and the repaint around it. It is deliberately NOT held across user callbacks
   // (OnActiveLayerChange, DrawFn), so a callback may re-enter AddLayer.
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

   The `redrawFn` body at `desktop.go:98-100` is **byte-for-byte identical** to `Redraw`'s, so this
   is a pure de-duplication — and it means `AddLayer`'s repaint and the run loop's coalesced
   repaint serialize against each other rather than racing. C6 shows no `turbotv` test overrides
   this `redrawFn`, so frame accounting is unaffected. This does not enlarge any critical section;
   it applies the same-size one at the other paint call site.

5. **Doc comments** on `AddLayer` (`desktop.go:263-269`) and the threading-contract block
   (`desktop.go:18-29`): state the narrow, implemented guarantee only — see §3.3.

### 3.2 Why this removes the race

Every shared location enumerated in §1 — `d.focused`, `d.nowFn`, `d.lastNotifiedTop`,
`d.onActiveLayerChange`, `a.back`, `a.front`, `a.lines`, `a.flushBuf`, `a.cursor*` — is accessed
inside one of the two `mutateMu` critical sections. The only code between them is the user
callback, which in `TestConcurrentAddLayerKeepsEveryLayer` is unregistered (no callback ⇒ `cb` is
nil ⇒ the gap is empty). The mutex establishes happens-before edges between consecutive `AddLayer`
calls, so the detector sees no unsynchronized concurrent access. The fix is race-free **by
construction** over the enumerated set, not by observation.

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
| R2 | **`Redraw` is now self-locking: a user `DrawFn` that calls `Redraw` deadlocks instead of overflowing the stack.** | `compose()` runs under `mutateMu` and invokes user `DrawFn`s. | Both are bugs and neither is supported (today it recurses infinitely). The failure mode changes from crash to hang; called out in the `Redraw` doc comment. |
| R3 | **Lock-ordering inversion with `layersMu`.** | `AddLayer` nests `layersMu` inside `mutateMu`. | C4: no site acquires `mutateMu` while holding `layersMu`. Rule documented on the field. |
| R4 | **Over-reading the guarantee.** | `mutateMu` makes `AddLayer` safe against `AddLayer`, and its repaint safe against the loop's repaint — not against input handlers or the other mutators, which do not take it. | §3.3 states the limit precisely; the "call it on the loop or via `Post`" requirement is retained verbatim, and the PR description will repeat it. |
| R5 | **Coalesced-redraw frame accounting changes.** | `desktop_redraw_coalesce_test.go` counts frames; §3.1(4) rewrites the `redrawFn`. | The new `redrawFn` body is byte-for-byte what `Redraw` already does, so paint count is unchanged; C6 confirms no `turbotv` test replaces it. These tests are the C1 canaries and must stay green. |
| R6 | **`go vet` copylocks.** | A second `sync.Mutex` in `Desktop`. | `Desktop` already holds `layersMu`, so it is already non-copyable; `go vet ./...` covers it. |
| R7 | **Callback ordering under concurrency.** | Delivery outside the lock permits reordered/overlapping callbacks. | Not a change in *supported* behaviour (single-goroutine order is unchanged, pinned by §5 test 2b). Specified in §3.4, exactly-once asserted by §5 test (1), flagged in §8 Q2. |
| R8 | **Throughput under contention.** | 64 goroutines serialize through a full compose+Apply each. | Bounded and tiny at 20×10; correctness outranks it, and supported usage is single-goroutine. |
| R9 | **Modal reads under lock** (`d.focused`, `d.now()`). | `d.now()` invokes the injectable `nowFn` inside the critical section. | Test clocks return a time and do not call back into the desktop (`SetClock`, `desktop.go:166-168`). `desktop_modal_guard_test.go` covers it. |

## 7. Open questions

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
