# design.md — TextBox: export `SelectAll()`

Task 01M2KZ7FZG0006FXTNSJQ5ZZ9M. Phase: DESIGN — no code or tests written yet.
Base commit: `4151a29`. Every line reference below was verified against that
commit while writing this.

## 0. What this design is judged against

This repository has **no `docs/SPEC.md` and no `docs/ROADMAP.md`** — there is no
`docs/` directory at all, and no file in the tree uses R-numbers:

```
$ git ls-files | grep -iE 'spec|roadmap|\.md$'
README.md
turbotv/README.md
turbotv/dialog_spec.go        # the declarative dialog-builder API, unrelated
turbotv/dialog_spec_test.go
```

Per the phase instructions, the specification for this change is therefore the
task description plus the repository's existing conventions. No section numbers
or R-numbers are cited, because inventing them would be worse than having none.

Conventions this design follows, each observed in the tree rather than assumed:

- Exported widget methods carry a doc comment written as full sentences
  (`SetText`, `SetTextChip`, `GetText` — `turbotv/widget_textbox.go:58-88`).
- Selection state stays private (`selAnchor`, `widget_textbox.go:25`); the public
  surface exposes behaviour, not the anchor.
- Widget tests live beside the widget in the same package (`package tv`), drive
  behaviour through the handler functions, and assert on public accessors plus
  private state where that is the point of the test
  (`turbotv/widget_textbox_test.go`).
- No test touches a real terminal: offscreen `tui.NewWithSize(w, h, &buf)` is the
  established pattern (`widget_paste_chip_test.go:636`,
  `widget_textbox_test.go:225`).

`critique.md` does not exist in the working tree, so there is no partner critique
to address this round and no 'Responses to critique' section.

## 1. Goal

Select-everything is currently reachable only by synthesising a Ctrl+A key
event. Export it, so an application can open a field with its value pre-selected
— a rename prompt where the first keystroke replaces the old name.

Consumer-visible contract after this change:

```go
box := tv.NewTextBox(currentName, bounds)
box.SelectAll()          // whole value selected, caret at the end
desktop.SetFocus(box)    // focus does not disturb the selection
// first typed rune replaces the value; Right/End collapse to "keep and append"
```

## 2. Packages, files and types

Package `tv` (directory `turbotv/`), type `TextBox`. Exactly two files change:

| File | Change |
| --- | --- |
| `turbotv/widget_textbox.go` | add exported method `func (t *TextBox) SelectAll()`; the Ctrl+A branch of `handleCtrlShortcut` calls it instead of duplicating the assignments |
| `turbotv/widget_textbox_test.go` | add the tests in §5 |

No new files, no new packages, no new types, no new struct fields. Untouched:
`turbotv/desktop.go`, `turbotv/component.go`, `turbotv/widget_multiline_input.go`,
`turbotv/theme.go`, the root `turbotui` package, and both READMEs.

## 3. Implementation

### 3.1 `SelectAll()` — new, placed immediately after `GetText()`

`SetText` / `SetTextChip` / `GetText` form the programmatic (non-event) API block
at `widget_textbox.go:58-88`. `SelectAll` belongs there, not among the event
handlers 170 lines further down.

```go
// SelectAll selects the entire text and parks the caret at the end. It is the
// programmatic equivalent of Ctrl+A, for callers that want a field to open with
// its initial value fully selected so the first keystroke replaces it.
//
// An empty field has nothing to select, so SelectAll clears the selection and
// parks the caret at 0. Clearing rather than returning early is deliberate: a
// prior click leaves selAnchor equal to the caret (see handleClick), and
// insertRune advances Cursor without touching the anchor, so a stale anchor would
// make the first typed rune select itself and the second one replace it.
func (t *TextBox) SelectAll() {
	if len(t.Text) == 0 {
		t.selAnchor = -1
		t.Cursor = 0
		return
	}
	t.selAnchor = 0
	t.Cursor = len(t.Text)
}
```

Three decisions worth recording:

- **Empty field: assign `-1`, do not merely return.** The task says "on an empty
  field leave `selAnchor` at -1", and §3.3 explains why that has to mean *assign*
  rather than *skip*. A bare `if len(t.Text) == 0 { return }` preserves whatever
  anchor is already there, which is not always -1.
- **Caret at the end, not the start.** Matches the existing Ctrl+A branch and is
  what makes Right/End behave as "keep the value and append":
  `moveCursor(pos, extend=false)` clears `selAnchor`
  (`widget_textbox.go:353-368`) and clamps `pos` to `len(t.Text)`. Anchoring the
  other way round would silently change Ctrl+A, which is out of scope.
- **`ScrollX` is not touched.** The Ctrl+A path does not touch it either, and
  `ensureCursorVisible` — called from both `draw` and `cursorPos` — scrolls the
  caret into view on the next paint. Adding `ScrollX = 0` here would diverge from
  Ctrl+A and would be extra behaviour needing its own test.

### 3.2 `handleCtrlShortcut` — one implementation

`widget_textbox.go:256-265` becomes:

```go
	case tui.KeyRune:
		if unicode.ToLower(event.Rune) == 'a' {
			t.SelectAll()
			return true
		}
		return false
```

The existing comment `// Select all: anchor at the start, caret at the end.`
(line 260) moves into the `SelectAll` doc comment — same statement, one place.
Ctrl+A still returns `true`, so the key is still consumed in every case,
including on an empty field.

### 3.3 Why the empty-field branch must assign, not skip

`selAnchor == 0, Cursor == 0` looks like "no selection" — `hasSelection()`
(`widget_textbox.go:370-372`) treats anchor == cursor as none — but it is an
**unstable** state, and this is the subtlety the whole design turns on.

`insertRune` (`widget_textbox.go:426-431`) appends the rune and increments
`Cursor` **without touching `selAnchor`**. So from `selAnchor == 0, Cursor == 0`,
typing `X` yields `selAnchor == 0, Cursor == 1` — `hasSelection()` is now true,
the just-typed `X` is selected and rendered highlighted, and typing `Y` deletes
it via `deleteSelection`, producing `"Y"` instead of `"XY"`.

That state is genuinely reachable, not theoretical. `handleClick`
(`widget_textbox.go:488-511`) assigns `t.Cursor = pos; t.selAnchor = pos` on
mouse-down (lines 503-504) and mouse-up clears only `t.selecting` (line 491). So
**clicking an empty box leaves `selAnchor == 0`.** If `SelectAll` returned early
it would walk straight past that anchor, and the select-on-open contract would
break for exactly the interaction a dialog user performs: click into the field,
then have the app call `SelectAll()`.

Assigning `-1` also makes the documented invariant at line 25 true rather than
merely usually-true, and makes the doc comment checkable by test T7.

**Consequence for Ctrl+A, stated plainly:** delegating changes Ctrl+A's behaviour
on an empty field. Previously Ctrl+A set `selAnchor = 0`; now it clears to -1.
On a *fresh* empty box that is unobservable (the anchor was already -1). After a
click it is observable and is a fix: Ctrl+A then `X`, `Y` now yields `"XY"` where
it used to yield `"Y"`. This is the only behavioural change in this task, it is
listed first in §6, and T7/T8 pin it.

## 4. Deliberately NOT in scope

- **The `SelectAllOnFocus` flag.** Per the task statement: the exported method
  alone unblocks the consumer. The flag would require overriding `HandleFocus` on
  `TextBox` and answering a question this task does not ask — does tabbing back
  into a half-edited field re-select it? Separate change, separate design.
- **Any `HandleFocus` / `OnFocusFn` override on `TextBox`.** There is none today,
  and `Desktop.setFocus` (`desktop.go:1396-1409`) only flips `hasFocus` and calls
  `HandleFocus`; it never touches `selAnchor`. So `SelectAll()` before `SetFocus`
  survives and no desktop change is needed. T6 asserts this rather than trusting
  the comment, because it is the property the consumer depends on and it lives in
  another file.
- **The general stale-anchor bug on the mouse path.** The §3.3 trace is not
  limited to empty fields: click *anywhere* in a non-empty box and type two
  runes, and the first is selected by the stale anchor and replaced by the
  second — `"ab"` + click between the characters + `XY` gives `"aYb"` instead of
  `"aXYb"`. That is a pre-existing bug on a different input path. The durable fix
  is to clear the anchor where the caret moves without extending (in the rune
  tail of `handleType`, or in `insertRune`), which would subsume this task's
  empty-field assignment. It is deliberately **not** done here: it changes
  behaviour for clicks, drags and every insert, and deserves its own design,
  its own regression suite and its own review. Flagged in §7 so it is not lost.
- **`MultiLineInput.SelectAll()`.** `MultiLineInput` has no Ctrl+A path at all
  today; its selection is the 2-D `selAnchorX`/`selAnchorY` pair, so select-all
  there is a different design, not a copy of this one. Parity is a follow-up.
- **`Deselect()` / `SetSelection(lo, hi)` companions.** Not requested.
  Speculative API is exactly the scope creep this design avoids.
- **Any drawing change, and no new render test.** `SelectAll()` introduces no
  rendering path: `draw` reads only `hasSelection()` + `selRange()` and cannot
  distinguish state set by Ctrl+A from state set by `SelectAll()`. A redraw test
  here would couple a small state-mutator to theme internals while testing
  nothing this change introduces.
- **Any paste-chip work.** A chip is one rune, so `SelectAll` covers it for free;
  `TestPasteChipTextBoxSelectAllReplacesChip` already exercises the Ctrl+A form
  and keeps doing so through the shared implementation.

The public surface gains exactly one method on an existing exported type. No
signature, field, constant or event type changes; `tui.TypeEvent` is read, never
modified. Existing callers compile unchanged, and existing behaviour is preserved
except for the intentional empty-field Ctrl+A correction described in §3.3.

## 5. Test plan — deterministic

All tests go in `turbotv/widget_textbox_test.go`, `package tv`, so
`selAnchor`/`selRange`/`hasSelection`/`handleType`/`handleClick` are reachable —
the same access the neighbouring tests already use. Style follows
`TestTextBoxCtrlASelectAll` (`widget_textbox_test.go:84`) and
`TestTextBoxTypingReplacesSelection` (line 62).

Determinism: pure in-memory struct manipulation. No network, no goroutines, no
clock or sleeps, no live cluster, no real terminal. T6 is the only test needing a
`tui.App`; it is built offscreen with `tui.NewWithSize(w, h, &bytes.Buffer{})` —
the pattern used by `TestDesktopCtrlXCutsToClipboard`
(`widget_textbox_test.go:225`). Output goes to a buffer; nothing reads stdin or a
tty. Every assertion is on an exact value, so there is no flakiness surface.

| # | Test | Asserts |
| --- | --- | --- |
| T1 | `TestTextBoxSelectAllThenTypeReplaces` | `SelectAll()`, type `'X'` → `GetText() == "X"`, `Cursor == 1`, `!hasSelection()`. Use a multi-rune, multi-byte fixture (e.g. `"hé🙂"`) so a byte/rune index confusion would fail |
| T2 | `TestTextBoxSelectAllThenRightAppends` | `SelectAll()`, `KeyRight`, type `'X'` → `GetText() == "helloX"`, `Cursor == 6`, `!hasSelection()` |
| T3 | `TestTextBoxSelectAllThenEndAppends` | same via `KeyEnd` — separate, because `End` and `Right` reach the end through different `moveCursor` arguments |
| T4 | `TestTextBoxSelectAllThenBackspaceEmpties` | `SelectAll()`, `KeyBackspace` → `GetText() == ""`, `Cursor == 0`, `!hasSelection()` |
| T5 | `TestTextBoxSelectAllEmptyFieldLeavesNoAnchor` | fresh `NewTextBox("")`, `SelectAll()` → no panic, `selAnchor == -1`, `Cursor == 0`, `!hasSelection()`; then type `'X'` → `GetText() == "X"` **and `!hasSelection()`**; then `'Y'` → `"XY"` |
| T6 | `TestTextBoxSelectAllSurvivesSetFocus` | `SelectAll()` then `desktop.SetFocus(box)` → `hasSelection()`, `selRange() == (0, len(Text))`; then type `'X'` → `GetText() == "X"`. Also assert `box.Component.Focused()`, so the test cannot pass vacuously if `SetFocus` ever no-ops |
| T7 | `TestTextBoxSelectAllClearsStaleAnchorOnEmptyField` | **the load-bearing one.** Establish the stale anchor *through `handleClick`* (mouse-down then mouse-up on an empty box), assert the precondition `selAnchor == 0`, call `SelectAll()`, assert `selAnchor == -1`/`Cursor == 0`/no selection, then type `'X'`, `'Y'` → `"XY"` |
| T8 | `TestTextBoxCtrlAClearsStaleAnchorOnEmptyField` | same stale-anchor setup, driven through Ctrl+A via `handleType`, then `'X'`, `'Y'` → `"XY"`, no selection. Pins §3.3's behaviour change on the real key path |
| T9 | `TestTextBoxCtrlAAndSelectAllAgree` | table-driven over `""`, `"hello"`, `"hé🙂"`: Ctrl+A on one box and `SelectAll()` on an identical box leave identical `selAnchor` and `Cursor`. Guards behavioural equivalence of the two entry points — note it cannot detect a re-inlining of identical assignments, only later divergence |

T7 and T8 are the tests that matter most and the ones easiest to omit. A suite
built only on fresh `NewTextBox("")` boxes passes whether `SelectAll` *restores*
the invariant or merely *fails to disturb* it — the two are indistinguishable
when the anchor is already -1. Only a click-established anchor tells them apart.
Both must fail against an early-return implementation; if they pass against it,
they are not testing what they claim.

Existing tests that must keep passing unchanged — the no-regression evidence for
the shared implementation: `TestTextBoxCtrlASelectAll`,
`TestTextBoxTypingReplacesSelection`, `TestTextBoxBackspaceDeletesSelection`,
`TestPasteChipTextBoxSelectAllReplacesChip`, `TestDesktopCtrlXCutsToClipboard`.

Gate: `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...` from the
repository root. Note `gofmt -l .` already reports four pre-existing files at
`4151a29` (`app_issues_test.go`, `signal_unix.go`, `signal_windows.go`,
`turbotv/binding_deliverable_capability_test.go`); they are not this task's to
fix, and the check for this change is that no *newly touched* file appears.

## 6. Regression risks — existing behaviour that must not change

1. **Ctrl+A on an empty field changes behaviour — intentionally** (§3.3). After a
   click, Ctrl+A then `X`, `Y` yields `"XY"` where it previously yielded `"Y"`.
   A fix, but a real user-visible change on an existing key path, and the only
   one here. Mitigation: T8 pins the new behaviour, T5/T7 pin the state, T9 pins
   that both entry points agree. If anyone considers the old behaviour
   load-bearing, this is the line to object to.
2. **Ctrl+A on a non-empty box must be byte-for-byte unchanged** — same anchor,
   same caret, still consumes the key. Mitigation: `TestTextBoxCtrlASelectAll`
   and `TestPasteChipTextBoxSelectAllReplacesChip` unchanged, plus T9's
   non-empty rows.
3. **Caret-at-end is contract, not accident.** A later change parking the caret
   at the start would silently change Ctrl+A with it. Mitigation: the doc comment
   states it; T1-T3 fail loudly.
4. **`Cursor = 0` in the empty branch must not surprise anyone.** With empty text
   the only valid caret position is 0, and `moveCursor` clamps there anyway, so
   this normalises a caller that left `Cursor` dangling rather than changing any
   reachable behaviour.
5. **Chip behaviour.** `SelectAll` selects chip sentinel runes like any other
   rune; `deleteSelection` → `pruneChips` handles removal and `copySelection`
   expands chips. Existing coverage:
   `TestPasteChipTextBoxSelectAllReplacesChip`, which now runs through the shared
   implementation and doubles as this change's regression test.
6. **Selection rendering.** Unchanged by construction — no `draw` code is
   touched; the highlight is resolved from `hasSelection()` + `selRange()` as
   before.
7. **Scroll position.** `ScrollX` untouched, so a caller reading it between
   `SelectAll()` and the first paint sees a stale value — already true of Ctrl+A
   today, so no new risk.
8. **Concurrency.** None introduced. `SelectAll` writes two fields of a
   caller-owned struct exactly as the Ctrl+A path does on the event-loop
   goroutine.

## 7. Open questions

1. **Is the empty-field Ctrl+A change acceptable inside this task?** I believe
   yes: it falls directly out of the task's own instruction to leave `selAnchor`
   at -1, and shipping the delegation without it would mean knowingly
   reproducing a bug. But it is a behaviour change to an existing key binding, so
   it is the reviewer's call. The alternative — `t.selAnchor = 0` unconditionally
   — preserves today's behaviour exactly and keeps the bug.
2. **The general mouse-path stale anchor (§4).** Click anywhere in a non-empty
   box and type two runes: the first is swallowed (`"ab"` → `"aYb"` instead of
   `"aXYb"`). Same root cause, wider blast radius, deliberately out of scope
   here. It should get its own issue; the likely fix is clearing `selAnchor` in
   `handleType`'s rune tail, which would then make this task's empty-field
   assignment redundant but harmless. Want it filed, or folded into a follow-up
   task?
3. **Nothing in the task statement is ambiguous.** The required behaviour, the
   required tests and the excluded flag are stated explicitly, and every claim
   the task makes about the current tree was verified against `4151a29` before
   writing this. I would rather say so than manufacture a third question.
