# Design critique

There is no `docs/SPEC.md`, so the task description and repository conventions are the specification. The proposed synchronization is otherwise well aligned with the task: it keeps `TestConcurrentAddLayerKeepsEveryLayer` intact, serializes concurrent `AddLayer` bookkeeping and repaint work, preserves synchronous callback-before-repaint behavior, and avoids broad parent-package changes. The tests are hermetic and require no cluster, network, sleeps, or wall clock. Requiring the actual `-race` gate on a supported host is an acceptable response to the local ThreadSanitizer limitation.

One internal contradiction still prevents approval.

## The mutex documentation is false about `DrawFn` re-entrancy

Section 3.1 proposes this comment for `mutateMu`:

> “It is deliberately NOT held across user callbacks (`OnActiveLayerChange`, `DrawFn`), so a callback may re-enter `AddLayer`.”

The proposed `Redraw`, however, does this:

> “`d.mutateMu.Lock()` … `d.compose()` … `d.mutateMu.Unlock()`”

`compose()` calls `layer.Root.Draw(surface)`, which can invoke user `DrawFn` code. Consequently, `mutateMu` **is held** across every `DrawFn`. A `DrawFn` that calls either `AddLayer` or `Redraw` will deadlock. Section 6 R2 acknowledges the recursive-`Redraw` case, directly contradicting the field comment.

The comment should instead say:

> “It is deliberately not held across `OnActiveLayerChange`, so that callback may re-enter `AddLayer`. It is held across drawing and therefore across user `DrawFn` callbacks; those callbacks must not call `AddLayer` or `Redraw`.”

Section 3.1 also says:

> “This is a pure de-duplication” and “This does not enlarge any critical section.”

That is inaccurate. Routing the redraw callback through the self-locking `Redraw` creates a new critical section around the entire paint pipeline, including arbitrary draw callbacks. The paint sequence is deduplicated, but synchronization and re-entrancy behavior change. It should say exactly that, with the already documented tradeoff that an invalid recursive draw changes from recursion/stack failure to deadlock.

The design also contains a duplicated sentence in section 3.2:

> “construction** over the enumerated set, not by observation.”

Remove the duplicate before implementation; it is editorial but makes the design look mechanically inconsistent.

Apart from these corrections, the design is sound. The two-critical-section structure correctly leaves `OnActiveLayerChange` outside the mutex, the exactly-N callback reasoning is valid, `lastNotifiedTop == TopLayer()` is schedule-independent after the wait group completes, lock ordering is explicit, and the documented concurrency guarantee remains appropriately narrow. No further architectural change is needed.

DESIGN: REVISE
