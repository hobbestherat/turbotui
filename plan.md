# Fix: control bytes 0x1B–0x1F mis-decoded in input parser (gogent #208)

## Problem
`parseOneInput` in `app.go` decodes every C0 control byte `< 0x20` with the
letter-only offset `rune(head + 'a' - 1)`. That formula is correct only for
`0x01..0x1A` (`^A..^Z` → `a..z`). For `0x1C..0x1F` it produces the wrong rune:

| Byte | Key     | Correct rune | Old (buggy) rune |
|------|---------|--------------|------------------|
| 0x1C | Ctrl+\  | `\` (92)     | `{` (123)        |
| 0x1D | Ctrl+]  | `]` (93)     | `}` (125)        |
| 0x1E | Ctrl+^  | `^` (94)     | wrong            |
| 0x1F | Ctrl+_  | `_` (95)     | wrong            |

So Ctrl+] (byte `0x1D`) yielded `{Key: KeyRune, Rune: '}', Ctrl: true}` and never
matched the registered `Ctrl+]` accelerator. ESC (`0x1B`) is already handled earlier
(`parseEscape`) so it is not affected.

## Fix
Use the standard C0 → printable mapping (caret notation), `head ^ 0x40`, which is
correct across the whole `0x00..0x1F` range:
`0x1D ^ 0x40 = 0x5D = ']'`.

To keep observable behavior identical for the already-working Ctrl+letter shortcuts
(which historically emitted lower-case runes), fold the resulting `A..Z` back to
`a..z`. Shortcut matching (`matchShortcut`) and text widgets already compare
case-insensitively, so punctuation is now correct and letters are unchanged.

```go
if head < 0x20 {
    r := rune(head ^ 0x40) // caret notation: 0x1D -> ']'
    if r >= 'A' && r <= 'Z' {
        r += 'a' - 'A' // preserve historical lower-case letter output
    }
    return TypeEvent{Key: KeyRune, Rune: r, Ctrl: true}, 1, true
}
```

## Scope / constraints
- Single change in `app.go` `parseOneInput`. No new deps. gofmt. Package style preserved.
- ESC (0x1B) path unchanged (handled before this branch).
- `\t` (0x09), `\r` (0x0D), `\n` (0x0A) still handled before this branch.

## Tests (GLM partner writes these)
- `parser.Feed([]byte{0x1C})` → `TypeEvent{Key: KeyRune, Rune: '\\', Ctrl: true}`
- `parser.Feed([]byte{0x1D})` → `TypeEvent{Key: KeyRune, Rune: ']', Ctrl: true}` (regression for Ctrl+])
- `parser.Feed([]byte{0x1E})` → `Rune: '^'`; `0x1F` → `Rune: '_'`
- Existing Ctrl+letter bytes (e.g. 0x0E → `n`) still decode to lower-case Ctrl runes
- ESC (0x1B) handling unchanged
