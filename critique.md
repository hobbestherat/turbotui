# Design critique

There is no `docs/SPEC.md`, so the task description and the repository's existing conventions are the specification. The design conforms to both and is ready for implementation.

The diagnosis is specific and credible: `layersMu` already protects the slice update, while concurrent `AddLayer` calls race through `lastNotifiedTop` and the synchronous `compose`/`updateCursor`/`Apply` pipeline. The proposed `mutateMu` closes those accesses without weakening or replacing `TestConcurrentAddLayerKeepsEveryLayer`.

The design now handles the important behavioral constraints correctly:

- `OnActiveLayerChange` remains synchronous and still runs before the repaint.
- The mutex is released around `OnActiveLayerChange`, preserving re-entrant `AddLayer` behavior.
- Concurrent callback delivery is explicitly specified as exactly once per added layer but unordered, and the proposed assertions test only schedule-independent guarantees.
- The final `lastNotifiedTop == TopLayer()` assertion is valid after `wg.Wait()` because append and notification bookkeeping share one critical section.
- The loop redraw callback uses the same locked paint path, preventing it from interleaving with an `AddLayer` repaint.
- The public documentation retains loop confinement and does not overclaim general thread safety.

The prior `DrawFn` contradiction is resolved. The design now accurately says:

> “It IS held across drawing, and therefore across user `DrawFn` callbacks: a `DrawFn` must not call `AddLayer` or `Redraw`, or it will deadlock.”

It also correctly replaces the earlier “pure de-duplication” claim with an explicit acknowledgment that the new critical section changes re-entrancy behavior. Changing an already-invalid recursive draw path from unbounded recursion to deadlock is a documented, acceptable regression risk for this narrowly scoped fix.

Testability is adequate without a cluster: all proposed tests are in-process and buffer-backed, with no external services, sleeps, or wall-clock dependence. This development host's unsupported ThreadSanitizer VMA is properly treated as an environmental caveat rather than a design blocker; requiring `go test ./turbotv -race` and the full race suite on a supported host before merge satisfies the task's acceptance condition.

Maintenance cost is proportionate. Production changes remain confined to `desktop.go`, lock ordering is explicit (`mutateMu` before `layersMu`), the common paint sequence is centralized in `Redraw`, and sibling mutators are deliberately left to a separately tested follow-up rather than expanding this task into general Desktop thread safety.

No revisions are required.

DESIGN: APPROVED
