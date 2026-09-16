No implementation defects found.

`TextBox.SelectAll()` selects the full rune slice with the caret at its end,
normalizes an empty field to `Cursor == 0` and `selAnchor == -1`, and therefore
also clears the reachable stale anchor left by clicking an empty field. The
Ctrl+A handler delegates to the method while continuing to consume the event.
Non-empty Ctrl+A selection behavior, paste-chip handling, rendering state, and
the existing editing transitions are otherwise unchanged.

The change introduces no persisted format, serialization field, event schema,
or compatibility migration.

CRITIQUE: CLEAN
