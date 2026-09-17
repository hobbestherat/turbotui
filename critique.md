No implementation defects found.

`TextBox.SelectAll()` selects the complete rune-backed contents, including an
atomic paste-chip sentinel, and places the caret at the end. On an empty box it
clears the otherwise reachable stale selection anchor and restores the cursor to
zero. The existing Ctrl+A path now delegates to the exported method without
changing event consumption or non-empty selection behavior. Selection replacement,
plain cursor movement, focus assignment, Unicode text, and empty-box editing are
covered by the current tests.

No `docs/SPEC.md` exists in this tree. The implementation agrees with the documented
selection behavior in `turbotv/README.md`. It introduces no persisted format,
serialization field, event schema, or compatibility migration.

CRITIQUE: CLEAN
