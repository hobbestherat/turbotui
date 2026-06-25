# Design: distinguishable Ctrl+Shift+&lt;letter&gt; on capable terminals

Cross-repo fix for hobbestherat/gogent#464 — **turbotui half**. The gogent half
(UX copy + `go.mod` pin bump) is done separately and depends only on the public
API staying source-compatible (see "Cross-repo seam").

## Problem recap

In gogent's Keybindings customizer, capturing `Ctrl+Shift+G` silently degrades to
`Ctrl+G`. Three things in turbotui combine to cause this:

1. `App.setupTerminal()` (app.go ~934) never enables an extended keyboard
   protocol, so capable terminals stay in legacy C0 encoding where `Ctrl+Shift+G`
   and `Ctrl+G` both arrive as the single byte `0x07` — the Shift bit is gone on
   the wire.
2. `parseOneInput`'s `head < 0x20` branch (app.go ~1128) folds that C0 byte to a
   Ctrl-only `KeyRune` with `Shift:false`. Correct and unavoidable for legacy.
3. `Deliverability` (app.go ~1186), which `tv.Chord.Deliverable()` delegates to,
   **blanket-refuses** every `Ctrl+Shift+<letter>` with "indistinguishable from
   Ctrl+letter" — even on terminals that *can* distinguish them.

The CSI-u decode path is already complete and correct: `parseCSI` case `'u'`
(~1313) + `decodeCSIModifier` (~1468) recover the Shift bit. The parser is ready;
the protocol is simply never turned on, and Deliverability hardcodes incapability.

## Chosen approach: Kitty keyboard protocol (disambiguate flag)

Enable the **Kitty keyboard protocol** with the *disambiguate escape codes* flag
(`0b1`), pushing it on setup and popping it on teardown:

- **Setup:** `CSI > 1 u`  (`\x1b[>1u`) — push flags `1` onto the terminal's stack.
- **Teardown:** `CSI < u`  (`\x1b[<u`) — pop the flags we pushed.

### Why Kitty over xterm modifyOtherKeys

The existing decoder only understands the **CSI-u** wire format (`CSI code ; mod u`,
`parseCSI` case `'u'`). The Kitty protocol *always* reports modified keys in that
exact format, so it slots into the ready-made path with zero parser changes.

xterm `modifyOtherKeys=2` in its *default* `formatOtherKeys=0` emits
`CSI 27 ; mod ; code ~` instead — that lands in the `'~'` branch with value `27`,
which is unhandled and would return `nil` (key silently dropped). Matching it would
require *new* parser code and risks regressing the `'~'` path. Kitty needs none of
that, so it is both the smaller and the safer change. We commit to Kitty as the one
coherent approach.

The disambiguate flag is the minimal flag that makes `Ctrl+Shift+G` distinct: it
tells the terminal to report any keypress that would be *ambiguous* in legacy
encoding (which `Ctrl+Shift+letter` is) as a `CSI-u` sequence. It does **not**
suppress ordinary text input, so plain typing, paste, mouse, and the four existing
modes are untouched.

### Graceful fallback (the critical no-regression property)

`CSI > 1 u` and `CSI < u` are **no-ops on terminals that don't implement them** —
unrecognized private CSI sequences are silently ignored. A legacy terminal
therefore behaves exactly as today: `Ctrl+Shift+G` still arrives as `0x07`, still
decodes to Ctrl-only, and Deliverability still refuses it (see capability gating
below). No new failure mode is introduced on any terminal.

## Capability state: how Deliverability learns the protocol is active

`Deliverability` is a package-level pure function and `Chord.Deliverable()` is a
zero-arg method — neither has an `*App`. To keep **both** signatures byte-for-byte
compatible (gogent calls `Chord.Deliverable()`), thread the capability through a
package-level flag in package `tui` that the binding layer already reaches via the
existing delegation:

```go
// app.go, package tui
var extendedKeyboardActive atomic.Bool   // new: sync/atomic added to imports
```

- `setupTerminal()` sets `extendedKeyboardActive.Store(true)` after writing the
  enable sequence.
- `restoreTerminal()` and `CloseWithMessage()`'s no-restoreState branch set
  `extendedKeyboardActive.Store(false)` alongside writing the teardown sequence, so
  the flag tracks the real terminal state across exit/crash.

`atomic.Bool` (not a plain bool) because setup runs on the `Run` goroutine while
gogent may query `Deliverable()` from its UI goroutine — keeps `-race` clean.

This is "track whether it was *enabled*", per the issue — not confirmed terminal
support (see Open questions on the optional `CSI ? u` handshake).

## Exact changes (files / functions)

All in **turbotui**; nothing else.

### app.go

1. **`setupTerminal()` (~934)** — extract the inline setup string into a named
   `const setupSequence` (symmetry with `teardownSequence`, and makes it testable
   without a real tty), append the Kitty push, and set the flag:
   ```go
   const setupSequence = "\x1b[?1049h\x1b[?25l\x1b[?1002h\x1b[?1006h\x1b[?2004h\x1b[>1u"
   ...
   err := a.writeOut(setupSequence)
   if err == nil {
       extendedKeyboardActive.Store(true)
   }
   return err
   ```
   The four existing modes are written in the identical order; only `\x1b[>1u` is
   appended.

2. **`teardownSequence` (~899)** — append the Kitty pop so the user's shell is
   restored on every teardown path (including the `defer a.Close()` crash path):
   ```go
   const teardownSequence = "\x1b[0m\x1b[?2004l\x1b[?1002l\x1b[?1006l\x1b[?25h\x1b[?1049l\x1b[<u"
   ```

3. **`restoreTerminal()` (~903)** and **`CloseWithMessage()` (~920)** — set
   `extendedKeyboardActive.Store(false)` where they emit `teardownSequence`.

4. **`Deliverability()` (~1186)** — make the one `Ctrl+Shift+<letter>` branch
   capability-aware; everything else byte-identical:
   ```go
   if shift && lower >= 'a' && lower <= 'z' {
       if extendedKeyboardActive.Load() {
           return true, ""   // capable terminal: Shift is recoverable via CSI-u
       }
       return false, "Ctrl+Shift+letter is indistinguishable from Ctrl+letter on most terminals"
   }
   ```
   The Ctrl-only verdicts (`Ctrl+M`/`I`/`H`/`J`/`[`, `Ctrl+S`/`Q`/`Z`) are left
   exactly as-is — they describe collisions of the bare-Ctrl chords and are out of
   scope for this issue. Returning early in the shift branch preserves the current
   control flow precisely.

5. **`sync/atomic`** added to the import block; declare `extendedKeyboardActive`
   near the other package-level decoder state.

### turbotv/binding.go

**No code change.** `Chord.Deliverable()` (~96) keeps delegating to
`tui.Deliverability` and automatically becomes capability-aware. `Chord.Matches`
(~37) already compares `Shift` exactly (line 49–51), so once events carry
`Shift:true` from the CSI-u path, dispatch routes `Ctrl+Shift+G` correctly. A doc
touch-up to the `Deliverable` comment (it asserts the indistinguishability as
unconditional) is optional and non-functional.

## Tests to add (turbotui)

1. **Decoder (app_parser_test.go):** feed `[]byte("\x1b[103;6u")` through the
   parser and assert `TypeEvent{Key: KeyRune, Rune: 'g', Shift: true, Ctrl: true}`.
   Math check against this repo's `decodeCSIModifier`: wire param `6` → `flags =
   6-1 = 5` → `shift = 5&1 = true`, `alt = 5&2 = false`, `ctrl = 5&4 = true`;
   keyCode `103 = 'g'`, not special, `> 0x20` → `KeyRune`. Confirms the parser was
   already correct (a guard against future regression).

2. **Deliverability (app_deliverability_test.go):** for `Ctrl+Shift+G`
   (`Deliverability(KeyRune, 'g', true, true, false)`):
   - with `extendedKeyboardActive` false → `ok == false`, reason contains
     "indistinguishable";
   - with `extendedKeyboardActive` true → `ok == true`, empty reason.
   Wrap with `defer extendedKeyboardActive.Store(false)` so toggling the package
   flag doesn't leak into other tests. Same-package (`package tui`) test, so it can
   touch the unexported flag directly. Also add a `tv`-level test through
   `Chord{Rune:'g', Ctrl:true, Shift:true}.Deliverable()` for the public seam.

3. **Setup/teardown bytes (app_issues_test.go style):** assert
   `strings.Contains(setupSequence, "\x1b[>1u")` and
   `strings.Contains(teardownSequence, "\x1b[<u")`, **and** that both still contain
   each existing mode (`?1049`, `?25`, `?1002`, `?1006`, `?2004`) so the no-regression
   on the four modes is pinned. Extend the existing
   `TestTeardownSequenceWrittenOnClose` expectation to the new constant (it already
   asserts `Contains(output, teardownSequence)`, so it stays green automatically).

## Design criteria

**(1) Goal match.** Exactly the issue's ask, no more: turn on a protocol that makes
`Ctrl+Shift+<letter>` distinguishable, and make Deliverability report the *real*
capability. No new widgets, no customizer changes (that's gogent's half), no
reworking of the unrelated Ctrl-only verdicts. Pure fix.

**(2) Usability.** On capable terminals the customizer now accepts and fires
`Ctrl+Shift+G` as pressed — the user drives the input and gets what they typed. On
legacy terminals the customizer still *refuses with the existing explanatory
reason* rather than silently degrading to `Ctrl+G` — the failure stays surfaced,
not silent. Terminal is cleanly restored on normal exit, error exit, and panic via
the existing `defer a.Close()` → `restoreTerminal` path, now including the Kitty
pop.

**(3) No regressions.** The four existing modes are written unchanged and in the
same order; only additive escape bytes are appended, and they are no-ops on
non-Kitty terminals. The parser is untouched (the CSI-u path already existed and is
now merely exercised). Deliverability changes one branch and returns early exactly
where it already did. Existing tests stay valid: `TestTeardownSequenceWrittenOnClose`
tracks the constant; the Ctrl-only deliverability cases are unchanged. New tests pin
the setup bytes and both legacy/active Deliverability verdicts. No exported
signature changes anywhere.

**(4) Holistic cross-repo design.** The fix lives entirely on the turbotui side
because that's where the protocol enable and the capability table physically are —
gogent cannot fix terminal encoding from its UI layer. The seam is the *public API*,
which is held stable: `tui.Deliverability(KeyCode, rune, bool, bool, bool)` and
`tv.Chord.Deliverable() (bool, string)` keep their exact signatures, so gogent's
bump is a clean `go get github.com/hobbestherat/turbotui@<commit> && go mod tidy`
with no call-site edits. gogent's customizer already trusts `Deliverable()` for its
accept/refuse decision, so it inherits the capability-aware behavior for free; its
half only needs the pin bump plus any UX copy referencing the new "works on capable
terminals" reality.

## Regression risks (and mitigations)

- **A terminal that partially understands Kitty but not the disambiguate semantics**
  could report `Ctrl+Shift+G` differently. Mitigation: disambiguate is the
  best-specified, most widely implemented flag; unknown terminals ignore the push
  entirely (legacy baseline holds).
- **Flag says "active" but terminal silently ignored the enable** → Deliverability
  reports `Ctrl+Shift+letter` deliverable on a terminal that can't deliver it, so a
  binding could be accepted but never fire. This is the explicit trade-off the issue
  accepts ("track whether it was enabled"). See Open questions for the optional
  query-based hardening.
- **Nested/stacked Kitty users:** we push on setup and pop on teardown
  (`>`/`<`), which is stack-correct and won't clobber an outer app's flags, unlike
  the absolute `= u` set form.

## Open questions

1. **Confirmed support vs. assumed support.** Should we additionally send the Kitty
   query `CSI ? u` at setup and flip `extendedKeyboardActive` only on receiving a
   `CSI ? <flags> u` reply (parsed in the input loop)? It eliminates the
   false-positive risk above but adds an async handshake and a new parse branch.
   The issue's wording ("track whether it was enabled") suggests the simpler
   assume-on-enable is acceptable for this fix; the query is a clean follow-up.
2. **`Ctrl+Shift` on non-letter keys** (digits, punctuation) — out of scope here;
   the issue is specifically letters. Current behavior for those is unchanged.
3. **Doc comment on `Chord.Deliverable`** still states the indistinguishability as
   unconditional. Worth a wording refresh to "on legacy terminals" — cosmetic,
   flagged so a reviewer can confirm it's in-scope for this commit.
