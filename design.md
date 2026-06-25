# Design: distinguishable Ctrl+Shift+&lt;letter&gt; on capable terminals (v2)

Cross-repo fix for hobbestherat/gogent#464 — **turbotui half**. The gogent half
(UX copy, override-lifecycle change, `go.mod` pin bump) is done separately; the
contract it must honour is spelled out in "Cross-repo seam".

v2 revises v1 after design review. The load-bearing change is that capability is
now **confirmed by a terminal query handshake** rather than assumed from writing the
enable sequence — which is what makes "Deliverability reflects *real* capability"
actually true and removes the silent false-positives v1 introduced on
tmux / Terminal.app. Two further gaps the review found are closed: the `Ctrl+Space`
CSI-u drop, and the unenumerated `Ctrl+M/I/H/J/[` transition.

## Problem recap

gogent's Keybindings customizer silently degrades a captured `Ctrl+Shift+G` to
`Ctrl+G`. Causes, all in turbotui:

1. `App.setupTerminal()` (app.go ~934) never enables an extended keyboard protocol,
   so capable terminals stay in legacy C0 encoding where `Ctrl+Shift+G` and `Ctrl+G`
   both arrive as the single byte `0x07` — the Shift bit is gone on the wire.
2. `parseOneInput`'s `head < 0x20` branch (~1128) folds that byte to a Ctrl-only
   `KeyRune` with `Shift:false` (correct/unavoidable for legacy).
3. `Deliverability` (~1186), which `tv.Chord.Deliverable()` delegates to,
   **blanket-refuses** every `Ctrl+Shift+<letter>` even on terminals that *can*
   distinguish it.

The CSI-u decode path is already complete: `parseCSI` case `'u'` (~1313) +
`decodeCSIModifier` (~1468) recover the Shift bit; `csiUSpecialKey` (~1412) maps
9/13/27/127 → Tab/Enter/Esc/Backspace. The parser is ready; the protocol is never
enabled and Deliverability hardcodes incapability.

## Chosen approach: Kitty keyboard protocol + capability handshake

### Enable (Kitty, disambiguate flag) and confirm (query)

- **Setup writes** (appended to the existing modes, in this order):
  - `CSI > 1 u` (`\x1b[>1u`) — push the *disambiguate escape codes* flag onto the
    terminal's keyboard stack.
  - `CSI ? u` (`\x1b[?u`) — query the terminal's current keyboard flags.
- **Teardown writes** `CSI < u` (`\x1b[<u`) — pop the flags we pushed.

A Kitty-capable terminal answers the query with `CSI ? <flags> u`
(`\x1b[?<flags>u`). Because we queried *after* pushing, a capable terminal's reply
has the disambiguate bit set. That reply — and **only** that reply — flips the
capability flag true. A terminal (or an intervening multiplexer) that doesn't
implement the protocol sends no reply, so the flag stays false and behaviour is
identical to today.

### Why Kitty over xterm modifyOtherKeys

The decoder only understands the **CSI-u** wire format (`parseCSI` case `'u'`). Kitty
always reports modified keys in exactly that format, so it reuses the ready-made
path with no new key-decode code. xterm `modifyOtherKeys=2` in its default
`formatOtherKeys=0` emits `CSI 27 ; mod ; code ~`, which lands in the `'~'` branch
with value `27` (unhandled → `nil` → key dropped) and would need fresh parser code.
Kitty is both the smaller and the safer change. We commit to Kitty as the one
coherent approach.

The *disambiguate* flag (`0b1`) is the minimal flag that makes `Ctrl+Shift+G`
distinct, and crucially it leaves **plain** Enter/Tab/Backspace as their legacy bytes
and only reports *ambiguous* (modified/conflated) keys as CSI-u. It does not suppress
text input, paste, or mouse — the four existing modes are untouched.

### Why the handshake (not assume-on-enable, as in v1)

v1 set the flag true merely because it *wrote* `\x1b[>1u`. On a terminal that ignores
the push — old xterm, macOS Terminal.app, **and default tmux, which swallows the
sequence and never forwards it to the inner terminal** — the flag would be true while
events still arrive as legacy `Ctrl+G`. `Deliverability` would then report
`Ctrl+Shift+G` deliverable, gogent's capture would *accept* it, but `Chord.Matches`
(needs `Shift==true`, binding.go:49) would never fire it: a **silent
accept-that-never-fires**, strictly worse than today's surfaced refusal.

The query handshake removes this: the flag tracks *the terminal answered*, not *we
asked*. No reply ⇒ flag false ⇒ the existing explanatory refusal is shown ⇒ the
failure stays surfaced. This directly resolves review concerns (2) and the tmux part
of (4). `\x1b[>1u` and `\x1b[?u` are both no-ops on terminals that don't grok them.

## Exact changes (files / functions) — all in turbotui

### app.go

1. **Capability state.** Add a package-level
   `var extendedKeyboardActive atomic.Bool` (import `sync/atomic`). `atomic.Bool`
   (not a plain bool) because it is *written* from the input-loop goroutine when the
   query reply is parsed and may be *read* by a binding/capture query — keeps `-race`
   clean without assuming caller goroutine. Default zero value `false` = legacy
   baseline, so every existing Deliverability test stays green untouched.

2. **`setupTerminal()` (~934).** Extract the inline setup string to a named
   `const setupSequence` (symmetry with `teardownSequence`; makes it assertable
   without a real tty) and append the enable + query bytes. **Do not** set the flag
   here — the handshake sets it:
   ```go
   const setupSequence = "\x1b[?1049h\x1b[?25l\x1b[?1002h\x1b[?1006h\x1b[?2004h" +
       "\x1b[>1u" + "\x1b[?u"
   ```
   The four existing modes are written first, in the identical order; only the two
   additive sequences follow.

3. **Parse the capability reply.** In `parseCSI`, before the existing `case 'u'`
   key-decode, detect the private-marker reply `CSI ? <flags> u` (i.e. `params`
   begins with `?` and `final == 'u'`) and return a small internal sentinel
   `capabilityReport{flags}` instead of a `TypeEvent`. In the event loop where
   `parser.Feed(...)` results are ranged over (app.go ~864), handle that sentinel by
   `extendedKeyboardActive.Store(report.flags&1 != 0)` and **not** dispatching it as
   a key. (`dispatchEvent`'s type switch already ignores unknown event types, so a
   stray sentinel can never reach a handler.) This is the only new control-flow in
   the hot path and it is a single typed branch.

4. **Close the `Ctrl+Space` CSI-u drop** (review defect #1). Today `case 'u'` guards
   the rune fall-through with `keyCode > 0x20 && keyCode != 0x7f`, so codepoint `0x20`
   (Space, e.g. Kitty's `CSI 32;5u` for Ctrl+Space) matches no arm and is dropped,
   whereas legacy delivers it. Widen the guard to `keyCode >= 0x20 && keyCode != 0x7f`
   so Space becomes `{Key:KeyRune, Rune:' ', Ctrl/Shift/Alt:…}`, and defensively map
   any remaining non-special low codepoint (`1..0x1f`, not 9/13/27) to its rune so no
   CSI-u key can silently vanish. (In practice Kitty reports the *base* codepoint, so
   letters arrive as 97–122 — already handled — and Space `0x20` is the one real gap.)
   Note the deliberate cross-mode nuance: legacy Ctrl+Space decodes to `{Rune:'@'}`
   (C0 `0x00`), the CSI-u form to `{Rune:' '}`; both are now *delivered* (the
   regression was the drop, not the spelling) — see Open questions.

5. **`teardownSequence` (~899) + flag reset.** Append the pop and clear the flag on
   every teardown path so the user's shell is restored on exit/crash via the existing
   `defer a.Close()` → `restoreTerminal` route:
   ```go
   const teardownSequence = "\x1b[0m\x1b[?2004l\x1b[?1002l\x1b[?1006l\x1b[?25h\x1b[?1049l\x1b[<u"
   ```
   In `restoreTerminal()` (~903) and `CloseWithMessage()`'s no-restoreState branch
   (~924), `extendedKeyboardActive.Store(false)` alongside writing the sequence.

6. **`Deliverability()` (~1186)** — make only the `Ctrl+Shift+<letter>` branch
   (line 1203) capability-aware; everything else byte-identical:
   ```go
   if shift && lower >= 'a' && lower <= 'z' {
       if extendedKeyboardActive.Load() {
           return true, ""   // confirmed-capable terminal: Shift is recoverable via CSI-u
       }
       return false, "Ctrl+Shift+letter is indistinguishable from Ctrl+letter on most terminals"
   }
   ```
   Returns early exactly where the current code does, preserving control flow.

### turbotv/binding.go

**No code change.** `Chord.Deliverable()` (~96) keeps delegating to
`tui.Deliverability` and becomes capability-aware for free. `Chord.Matches` (~37)
already compares `Shift` exactly (49–51), so once events carry `Shift:true` the
`Ctrl+Shift+G` binding routes correctly. The `Deliverable` doc comment (which states
the indistinguishability unconditionally) gets a one-line wording refresh to "on
legacy terminals" — non-functional.

## The Ctrl+M/I/H/J/[ transition on Kitty (review defects #2, #5) — enumerated

With disambiguate active, the five chords that legacy folds into named keys instead
arrive as CSI-u rune events:

| Chord | legacy today | Kitty (disambiguate) |
|------|---------------|----------------------|
| Ctrl+M | `0x0d` → `KeyEnter` | `CSI 109;5u` → `{Rune:'m',Ctrl}` |
| Ctrl+I | `0x09` → `KeyTab` | `CSI 105;5u` → `{Rune:'i',Ctrl}` |
| Ctrl+H | `0x08` → `KeyBackspace` | `CSI 104;5u` → `{Rune:'h',Ctrl}` |
| Ctrl+J | `0x0a` → `{KeyEnter,Ctrl}` (submit) | `CSI 106;5u` → `{Rune:'j',Ctrl}` |
| Ctrl+[ | `0x1b` → `KeyEscape` | `CSI 91;5u` → `{Rune:'[',Ctrl}` |

Two deliberate decisions:

- **The plain named keys are unaffected.** Under disambiguate, bare Enter/Tab/
  Backspace keep sending their legacy bytes, and bare Esc arrives as `CSI 27u` →
  `csiUSpecialKey(27)` → `KeyEscape`. So Enter/Tab/Esc/Backspace — which widgets and
  text fields depend on — keep working on Kitty. Only the *Ctrl-modified* spellings
  shift, and only on confirmed-Kitty terminals. The practical exposure (someone
  pressing **Ctrl+M expecting Enter** inside a field) is marginal; we accept it as the
  cost of disambiguation and document it here rather than leaving it implicit.

- **Their Deliverability verdicts stay refused** (we do *not* gate them on
  capability), and here is the asymmetry the review asked us to articulate:
  re-enabling `Ctrl+Shift+letter` recovers functionality the user *has no other way to
  express* — they pressed exactly that chord and meant it. By contrast Ctrl+M/I/H/J/[
  each have a perfectly good alias already bindable (Enter/Tab/Esc/Backspace/the
  Ctrl+Enter submit key), so refusing them loses no capability — it is merely
  conservative. A conservative *false-negative* (refuse a technically-bindable chord)
  is safe; a permissive *false-positive* on a key that doubles as Enter would be
  actively dangerous if the handshake ever mis-fired. Given issue #464 is scoped to
  Ctrl+Shift+letter, expanding to five more chords is scope creep with no demand;
  gating them is a clean follow-up if asked. (Listed in Open questions.)

## Cross-repo seam — what gogent must do (review defect #4)

The public API is held stable: `tui.Deliverability(KeyCode,rune,bool,bool,bool)` and
`tv.Chord.Deliverable() (bool,string)` keep their exact signatures, so the pin bump is
a source-clean `go get …@<commit> && go mod tidy` with no call-site edits. But "clean
at the API level" is **not** "behaviorally complete" without one gogent-side change,
which the review correctly flagged:

- **Default `Ctrl+Shift+*` bindings** (e.g. the command-palette `Ctrl+Shift+G`)
  register without a `Deliverable()` check, so they fire on a confirmed-Kitty terminal
  purely from the protocol enable — the turbotui change alone fixes them.
- **Live capture** in the customizer runs *inside* the running TUI, after
  `setupTerminal()` + handshake, so `Deliverable()` there sees the real capability and
  accepts/refuses correctly — fixed by the turbotui change.
- **Persisted overrides re-validated at startup** are the gap. gogent validates
  saved overrides through `Deliverable()` during `LoadKeybindings`/`rebuildBindings`,
  which run during Workbench construction **before** `desktop.Run()` calls
  `setupTerminal()`. At that moment the flag is false (no terminal, no handshake), so a
  previously-captured `Ctrl+Shift+T` would be **silently dropped on restart**.

  **Contract for the gogent half:** do not re-gate *persisted* overrides through
  `Deliverable()` at load — they were already validated against the live terminal at
  capture time, so load them unconditionally (or re-validate only *after* `Run` has
  started and the handshake has settled). `Deliverable()` should gate the *live
  capture UI only*, never the reload of trusted, already-saved config. This is the
  cleanest split and makes the fix survive restart. It is gogent-half work; turbotui
  exposes everything needed (capability via `Deliverable()` once the terminal is up).

- **tmux / multiplexers:** default tmux swallows `\x1b[>1u`/`\x1b[?u`, so no reply
  arrives, the flag stays false, and capture *refuses with the reason* — surfaced, not
  silent. If the user enables tmux key passthrough, the reply flows and capture
  works. The handshake makes both outcomes correct with no special-casing.

## Tests to add (turbotui)

1. **Decoder — Ctrl+Shift+G** (app_parser_test.go): feed `[]byte("\x1b[103;6u")`,
   assert `TypeEvent{Key:KeyRune, Rune:'g', Shift:true, Ctrl:true}`. (Math: wire `6`
   → `flags 5` → shift+ctrl; `103='g'`, not special, decoded as rune.)
2. **Decoder — Ctrl+Space not dropped** (app_parser_test.go): feed
   `[]byte("\x1b[32;5u")`, assert a non-nil `{Key:KeyRune, Rune:' ', Ctrl:true}`
   (pins review defect #1 closed and guards against re-narrowing the guard).
3. **Handshake flips capability** (app_parser_test.go / app_deliverability_test.go):
   feed the reply `[]byte("\x1b[?1u")` through the event-loop handling, assert
   `extendedKeyboardActive.Load() == true`; `defer extendedKeyboardActive.Store(false)`.
   A negative case: a reply with the bit clear (`\x1b[?0u`) leaves it false.
4. **Deliverability capability-gated** (app_deliverability_test.go, `package tui` so it
   can touch the unexported flag): `Deliverability(KeyRune,'g',true,true,false)` →
   refused+reason when flag false; deliverable+empty when flag true. Plus a `tv`-level
   test through `Chord{Rune:'g',Ctrl:true,Shift:true}.Deliverable()` for the public
   seam. Each test `defer`s the flag back to false so the global never leaks
   (no `t.Parallel()` exists in the suite, so serial toggling is safe).
5. **Setup/teardown bytes** (app_issues_test.go style): assert `setupSequence`
   contains `\x1b[>1u` and `\x1b[?u`, `teardownSequence` contains `\x1b[<u`, **and**
   both still contain every existing mode (`?1049/?25/?1002/?1006/?2004`) — pinning the
   four-modes no-regression. The existing `TestTeardownSequenceWrittenOnClose`
   (`Contains(output, teardownSequence)`) stays green against the new constant.

## Design criteria

**(1) Goal match.** Enables a protocol that makes `Ctrl+Shift+<letter>` distinguishable
and makes `Deliverability` report the *confirmed* capability — exactly the issue, no
widget/customizer changes (gogent's half), no reworking of unrelated verdicts. The one
adjacent parser fix (Ctrl+Space) is not scope creep but a regression *caused by* this
change, fixed in the same commit.

**(2) Usability.** On a confirmed-capable terminal the customizer accepts and fires
`Ctrl+Shift+G` as pressed. On any terminal that doesn't confirm — legacy, Terminal.app,
default tmux — capture still *refuses with the explanatory reason* instead of silently
accepting a binding that never fires. The right thing is surfaced in both branches; the
v1 silent-false-positive is gone. Terminal is cleanly restored on normal/error/panic
exit via the existing defer path, now including the Kitty pop.

**(3) No regressions.** Legacy terminals: byte-for-byte identical behaviour (flag false,
no reply). Capable terminals: the Ctrl+Space drop is fixed; the Ctrl+M/I/H/J/[
transition is enumerated and bounded (plain Enter/Tab/Esc/Backspace unaffected); the
four existing modes are written unchanged and first. The only new hot-path code is one
typed sentinel branch in the loop and one widened CSI-u guard. Default Deliverability is
false, so every current test passes untouched; new tests pin the additions.

**(4) Holistic cross-repo.** Fix lives in turbotui because that's where the protocol and
the capability table physically are. The API seam is stable (no signature changes), so
the pin bump is source-clean. The one behavioral dependency — persisted overrides must
not be re-gated pre-`Run` — is stated as an explicit contract for the gogent half, with
the recommended split (Deliverable gates live capture only; trusted saved config loads
unconditionally). tmux degrades to a surfaced refusal, not a silent failure.

## Regression risks (and mitigations)

- **Handshake reply races a very early keypress.** The flag is false for the few ms
  between setup and reply. Any user navigation to the customizer happens far later, so
  in practice the flag is settled. We never block on the reply, so a non-answering
  terminal simply stays in legacy mode — no hang. Mitigation: accept the millisecond
  window; document it.
- **Ctrl+Space spelling differs across modes** (`'@'` legacy vs `' '` CSI-u). Both are
  now delivered; the cross-mode inconsistency is pre-existing in spirit and out of #464
  scope. Flagged in Open questions.
- **Stacked Kitty users:** we push/pop (`>`/`<`), stack-correct, so we never clobber an
  outer app's flags (unlike the absolute `= u` set form).
- **Partial-Kitty terminals** that reply to the query but mis-encode keys: the flag
  trusts the reply. This is the standard kitty negotiation; terminals that advertise
  support are expected to honour it.

## Open questions

1. **Ctrl+M/I/H/J/[ verdicts** — gate them on capability too (they *are* deliverable on
   confirmed-Kitty) or keep the conservative refusal? This design keeps them refused
   with the asymmetry rationale above; revisit if users ask to bind them.
2. **Ctrl+Space spelling** — should the CSI-u decode normalise Space back to the legacy
   `{Rune:'@'}` for cross-mode binding consistency, or is `{Rune:' ',Ctrl}` the more
   correct representation to standardise on (and instead normalise the legacy side)?
   Either is a small, separable follow-up; this design only stops the drop.
3. **Confirm-flag granularity** — we trust bit `0b1` of the reply. If a future need
   arises to distinguish "enabled but not disambiguating", we'd inspect more bits; not
   needed for #464.
