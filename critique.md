The revised design is sound.

## Specification conformance

There is no `docs/SPEC.md`, so the task description and existing repository
conventions are the specification. The design adds exactly the requested
`TextBox.SelectAll()` API and reuses it from Ctrl+A. It does not invent a focus
flag, companion selection APIs, multiline parity, rendering changes, or a
general mouse-input repair.

The empty-field branch now fulfills the documented contract rather than merely
assuming it: it explicitly clears `selAnchor` and normalizes `Cursor` to the
only valid empty-text position. The wider click-then-type bug is correctly
identified but left for a separate change with a larger regression surface.

## Testability without a cluster

All proposed tests are deterministic and local. The only application-level
test uses the repository's buffer-backed `tui.NewWithSize` seam; no network,
cluster, timing, sleep, stdin, or physical terminal is involved.

T7 and T8 establish the stale anchor through real `handleClick` transitions and
assert that precondition before exercising `SelectAll()` and Ctrl+A. They
therefore cannot pass vacuously on a freshly initialized anchor. T6 similarly
asserts that focus was actually acquired.

## Regressions to existing behavior

The design identifies the sole intentional behavior change precisely: after an
empty field has been clicked, Ctrl+A now clears the latent anchor, so subsequent
`X`, `Y` input yields `"XY"` instead of the buggy `"Y"`. The Ctrl+A event remains
consumed, and non-empty selection bounds and caret placement remain unchanged.

The plan covers replacement, Right, End, Backspace, Unicode rune indexing,
focus survival, fresh-empty state, click-established stale state, the actual
Ctrl+A path, and method/key equivalence. Existing paste-chip and clipboard tests
remain part of the regression gate. No persisted format is affected.

## Maintenance cost

The production change remains small and centralized: one documented method and
one delegation site. The design avoids a redundant render test and accurately
limits T9 to behavioral equivalence rather than claiming it enforces source
structure. The two stale-anchor tests exercise distinct public entry paths and
justify their small duplication because either path could regress independently.

No line requires revision.

DESIGN: APPROVED
