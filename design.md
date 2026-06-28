# Design — TUI mis-detects terminal color over SSH (gogent #549)

Cross-repo fix. **turbotui owns the canonical color-level detector; gogent defers to it.**
This is a *detection-correctness fix*, not a new knob: no CLI flags, no config keys, no
new module dependencies, no cgo. We add **terminfo** as a detection input (the one
capability source that survives SSH) and collapse two independent env-only detectors into
one shared implementation.

> Revised after design-critic review. Key changes from v1: terminfo is **not** baked into
> `ColorLevelFromEnv` or package `init()` (no import-time subprocess, no host-terminfo test
> flakiness); the terminfo seam is **exported** so gogent can test the headline case
> deterministically; the version-pin guard test is in the file list; "no behavior change"
> claims are corrected to name the real, deliberate behavior changes.

## Problem recap

Workflow **A** (affected): `ssh host` then run `gogent` on the remote. sshd sets a valid
`TERM` on the remote but does *not* forward `COLORTERM` (unless admin-whitelisted). So
remote gogent sees `TERM=xterm-256color`, empty `COLORTERM`, and concludes 256-color even
when the local emulator is truecolor — RGB themes get quantized.

Workflow **B** (not affected): `gogent --connect ssh://host` runs the TUI *locally* and
reads the local `COLORTERM` directly. Must stay correct.

Two root causes (both verified in code):
1. **Detection is env-only** — `NO_COLOR`/`TERM`/`COLORTERM`. Terminfo (`colors`, `Tc`),
   which lives in the *remote* terminfo DB keyed by the propagated `TERM` and therefore
   survives SSH, is never consulted (gogent `theme.go:719`, turbotui `color.go:61`).
2. **Two detectors can disagree** — gogent `detectColorLevel` (theme resolver) and
   turbotui `ColorLevelFromEnv` (cell renderer) are *separate* env-only copies, and gogent
   `ApplyTheme` (`theme.go:1133`) never calls `tui.SetColorLevel`, so the renderer runs at
   its own `init()`-detected level independent of gogent's resolved theme level. They
   already diverge on edge cases (see "Divergences") and would drift further if terminfo
   were added to only one.

## Approach (lowest-risk, single source of truth)

**turbotui is the one detector.** It gains a terminfo-aware detection path. gogent's
`detectColorLevel` stops re-implementing env rules and *delegates* to that path, mapping
the result into gogent's parallel `ColorLevel` enum. Both layers run the *same detection
code over the same inputs* and cannot drift. The runtime render level is installed once per
entry point via turbotui's existing `SetColorLevel`, so the cell renderer and the theme
resolver agree on the active level.

We deliberately **do not** change `ResolveTheme`'s signature (it has 120 call sites across
~20 gogent test files), and we deliberately **do not** change `ColorLevelFromEnv` or
package `init()` (keeps turbotui's existing behavior, tests, and zero-subprocess import
contract intact). Terminfo lives in a *new* function.

### Precedence (documented; verified against no-color.org + ncurses `terminfo(5)`/`tic`)

Evaluated in this exact order by the new terminfo-aware path; first match wins:

1. `NO_COLOR` present and non-empty → **none** (always wins; https://no-color.org/)
2. `TERM == "dumb"` → **none**
3. `COLORTERM == "truecolor" | "24bit"` → **truecolor**
4. terminfo boolean `Tc` **or** `RGB` advertised → **truecolor**   ← new
5. terminfo numeric `colors` ≥ 256 → **256**                       ← new
6. `TERM` contains `"256color"` → **256** (substring fallback when terminfo unavailable)
7. otherwise → **16**

Terminfo (steps 4–5) sits *after* `COLORTERM` (the authoritative local signal, when
present) and *before* the `TERM`-substring fallback. Consequence: the common local
truecolor case (COLORTERM set) resolves at step 3 and **never spawns `infocmp`**; the SSH
case (COLORTERM dropped) gets the truecolor answer from `Tc`/`RGB`.

> `RGB` is mandatory, not optional polish: empirically on the dev host `xterm-direct`
> advertises the boolean `RGB,` (with `colors#0x1000000`) and **no** `Tc`. Recognizing only
> `Tc` would miss every `*-direct` entry.

### Honest-detection caveat (documented in code)

Detection is faithful to observable signals; it **cannot invent capability**. It stays at
256 (correctly) in any of these cases:
- the propagated `TERM`'s terminfo entry carries `colors#256` but **no** `Tc`/`RGB` — true
  of *generic* `xterm-256color`, and (verified on this host) of `alacritty` and
  `tmux-256color`, which advertise only `colors#0x100`; **and**
- the **remote** host has **no `infocmp`** (minimal jump-hosts/containers) or no terminfo
  entry for `TERM` — we fail open to env-only detection.

So the realistic win-set is narrower than "every modern terminal": it is sessions where the
propagated `TERM`'s remote terminfo entry *does* advertise `Tc`/`RGB` — e.g. `*-direct`
entries (via `RGB`), and distro/version-patched entries that carry `Tc` — and `COLORTERM`
was the lost signal. The fix makes those correct; it makes nothing worse.

---

## Files & functions

### turbotui (`github.com/hobbestherat/turbotui`) — canonical detector

**`color.go`**
- `ColorLevelFromEnv(lookup func(string)(string,bool)) ColorLevel` — **env-only, behavior
  unchanged.** Reimplement it as a one-liner: `return ColorLevelFromEnvWithTerminfo(lookup,
  nil)`. With a `nil` terminfo reader, steps 4–5 are skipped → identical results to today.
  Existing `TestColorLevelFromEnv` rows stay green and hermetic (no terminfo, no subprocess).
- **New exported** `func ColorLevelFromEnvWithTerminfo(lookup func(string)(string,bool), ti TerminfoCaps) ColorLevel`
  — the unified detector implementing the full 7-step precedence. When `ti == nil`, steps
  4–5 are skipped. **This is the single shared detector and the testable seam** (exported so
  *both* repos can inject a stub `ti`).
- `DetectColorLevel()` → `return ColorLevelFromEnvWithTerminfo(os.LookupEnv, InfocmpCaps)`
  — the production entry that *does* consult terminfo (lazy: only runs when a host calls it).
- Package `init()` — **unchanged**: still `colorLevel.Store(uint32(ColorLevelFromEnv(os.LookupEnv)))`.
  Env-only → **no `infocmp` fork at import** for any turbotui consumer. A host that wants
  terminfo-aware detection calls `SetColorLevel(DetectColorLevel())` at startup (one line —
  gogent does exactly this at every entry point). Document this contract in the doc comment.
- `SetColorLevel`/`GetColorLevel` — unchanged; the single runtime handle, read per cell via
  `adaptColor(c, GetColorLevel())` (atomic load — detection is never re-run per cell).

**`terminfo.go`** (new) — minimal, stdlib-only terminfo reader
- `type TerminfoCaps func(term string) (colors int, truecolor bool, ok bool)` — exported seam.
- `func InfocmpCaps(term string) (colors int, truecolor bool, ok bool)` — exported default
  reader (so gogent can wire it into its test-overridable package var). `DetectColorLevel`
  passes `InfocmpCaps`; turbotui tests pass a stub via the `ti` parameter, not a global.
  - empty `term` → `ok=false`.
  - `exec.LookPath("infocmp")`; absent → `ok=false` (graceful: fall back to env-only).
  - Run `infocmp -x -1 <term>` (`-x` is required to surface the extended boolean `Tc`/`RGB`;
    `-1` = one cap per line, eases parsing). No stdin; capture stdout only; ignore non-zero
    exit → `ok=false`.
  - Parse line-by-line, trim trailing `,` and spaces. A numeric cap is `name#value`; **value
    may be decimal *or* `0x`-hex** — ncurses emits `colors#0x100` (256) here, `colors#8` for
    `xterm`, `colors#0x1000000` for `xterm-direct` — so use `strconv.ParseInt(v, 0, 64)`
    (base 0 honors `0x`). A boolean cap is a bare token. Recognize `colors#…` → `colors`;
    bare `Tc` or `RGB` → `truecolor=true`. Set `ok=true` if any recognized cap was found.
  - File-header doc: *vendored minimal `infocmp -x` lookup chosen over a terminfo-parser
    dependency to stay stdlib-first and cgo-free, per project policy; fails open to env-only
    detection when `infocmp`/the entry is unavailable.*

**`color_test.go`** — existing `TestColorLevelFromEnv` **unchanged** (env-only path, still
hermetic). Add `TestColorLevelFromEnvWithTerminfo` driving the new seam with a **stub
`TerminfoCaps`** (never touches host terminfo):
- stub advertises `Tc` (or `RGB`), no `COLORTERM` → **truecolor** (the SSH-truecolor win)
- stub `colors=256`, no `Tc`, no `COLORTERM` → **256**
- SSH shape: `TERM=xterm-256color`, no `COLORTERM`, stub `colors=256` no `Tc` → **256**
  (unchanged — honest caveat)
- `COLORTERM=truecolor` present → **truecolor**, and assert the stub is **not invoked**
  (precedence + perf guard: COLORTERM short-circuits before terminfo)
- stub `ok=false` (terminfo unavailable) → identical to env-only (`TERM`-substring fallback)
- `nil` ti → identical to `ColorLevelFromEnv` (cross-check the one-liner delegation)

### gogent (`github.com/hobbestherat/gogent`) — defers to turbotui

**`go.mod`** — bump pin to the merged turbotui SHA via `go get github.com/hobbestherat/turbotui@<sha> && go mod tidy`.

**`ui/tui/keybindings_issue401_test.go`** — `TestIssue401GoModHasRequestedTurbotuiAndNoReplace`
(line ~175) hard-asserts the exact pin string `github.com/hobbestherat/turbotui
v0.3.1-0.20260627191040-1cdd5ba10982`. **This test fails the moment go.mod is bumped** —
update the asserted pseudo-version (and the "#529 tall-button bump" comment) to the new
merge SHA. *(Omitting this would break `go test ./...` — it is on the required-edit list.)*

**`ui/tui/theme.go`**
- `var terminfoCaps tui.TerminfoCaps = tui.InfocmpCaps` — package var holding the real
  reader in production, **overridable in tests** (with `t.Cleanup`). This requires turbotui
  to **export** the default reader as `tui.InfocmpCaps` (the `terminfo.go` `infocmpCaps`
  promoted to exported). gogent injects a stub here for hermetic, deterministic tests.
- `detectColorLevel(env func(string) string) ColorLevel` — **keep signature** (preserves all
  120 `ResolveTheme` call sites). Replace the body: adapt `env` to a lookup
  (`func(k string)(string,bool){ v:=env(k); return v, v!="" }`), call
  `tui.ColorLevelFromEnvWithTerminfo(lookup, terminfoCaps)`, and map turbotui→gogent via an
  explicit `fromTUILevel` switch (the enums are value-parallel but **different types** —
  `tui.ColorLevel uint8` vs gogent `ColorLevel int` — so map by switch, never by cast).
  This deletes gogent's re-implemented env rules — the heart of Cause 2.
- `ResolveTheme(cfg, env, noColorFlag)` — unchanged signature/body; still calls
  `detectColorLevel(env)`, which now *is* turbotui's detector. `noColorFlag || cfg.NoColor`
  still forces `ColorNone` here (matches the runtime install below).
- **New** `func InstallColorLevel(env func(string)string, noColor bool)` — the **single
  runtime install point**: computes `detectColorLevel(env)`, overrides to `ColorNone` when
  `noColor`, and calls `tui.SetColorLevel(toTUILevel(level))`. Honouring `--no-color` /
  `cfg.NoColor` *here* (not only inside `ResolveTheme`) is what makes turbotui's renderer,
  its `tv.DefaultTheme` chrome, and markdown auto-disable go monochrome together (today they
  don't: turbotui `init()` reads env, not the flag, so `--no-color` leaks color from the
  renderer).

**Entry points — install the level once each, immediately before `ApplyTheme(ResolveTheme(...))`:**
- `cmd/main.go:199` (embedded TUI) — `noColor = *noColor`
- `cmd/attach.go:149` (attach startup) — `noColor = noColorFlag`
- `cmd/attach.go:302` (`installPresentationHandlers` → `SetTheme` live switch) — re-install
- `cmd/embedded_handlers.go:145` (`SetTheme` live switch) — re-install
  Each adds `tuipkg.InstallColorLevel(os.Getenv, noColor)` so install and resolve read
  identical inputs and the two layers can't diverge across a live env/theme change.

**`ui/tui/theme_test.go`**
- `TestDetectColorLevel`: override the gogent `terminfoCaps` package var with a **no-op stub
  (`ok=false`)** for the duration (with `t.Cleanup`) so the table is **hermetic** — it tests
  env rules without touching host terminfo. Update the **`{"missing term", {}, ColorNone}`**
  case to `Color16` (gogent now adopts turbotui's canonical empty-TERM result — see
  Divergences). Other rows (`xterm`, `screen-256color`, lowercase `truecolor`/`24bit`,
  `dumb`, `NO_COLOR`) stay green.
- New `TestColorLevelLayersAgree` (Cause-2 guard, **deterministic via the exported seam**):
  override `terminfoCaps` with a stub that returns `Tc=true` for the SSH `TERM`; for the
  rows {SSH-truecolor, plain-256, NO_COLOR, `--no-color` flag} call `InstallColorLevel` then
  assert `toTUILevel(ResolveTheme(cfg, env, flag).Level) == tui.GetColorLevel()`. Restore
  `tui.SetColorLevel` and `terminfoCaps` in cleanup. This pins the **headline
  truecolor-over-SSH case** end-to-end, not just the uninteresting levels.
- `TestResolveThemeNoColor` — stays green: NO_COLOR / `--no-color` / `cfg.NoColor` all
  flatten to `ColorNone` at both layers.

---

## Divergences resolved (this is the "two detectors agree now" outcome)

By delegating, gogent adopts turbotui's exact env rules. The pre-existing mismatches and
their honest consequences:

| input | gogent (old) | turbotui (canonical) | after fix | consequence |
|---|---|---|---|---|
| empty/unset `TERM` | none | 16 | **16** | **deliberate behavior change, not inert**: cron / `env -i` / non-TTY paths that had no `TERM` now emit 16-color SGR where before they emitted none. Acceptable (a real PTY always sets `TERM`; `NO_COLOR`/`dumb` still force none), but it is a real, documented change. |
| `COLORTERM=TrueColor` (mixed case) | truecolor | not matched (exact) | ≤256 | user-visible **downgrade** for the rare mixed-case env (COLORTERM is conventionally lowercase). |
| `TERM=vt256` (`"256"` not `"256color"`) | 256 | 16 | 16 | user-visible **downgrade** for the rare `256`-but-not-`256color` TERM. |

The production-meaningful inputs (`xterm`, `*-256color`, lowercase `COLORTERM=truecolor`,
`dumb`, `NO_COLOR`) resolve identically across both detectors today, so the overwhelming
majority of local users see the **same level as before**; the three rows above are the
honest exceptions.

---

## Design criteria

### (1) Goal match — OK
Terminfo `colors`/`Tc`/`RGB` consulted (SSH-safe); one shared detector
(`ColorLevelFromEnvWithTerminfo`), turbotui canonical, gogent delegates; precedence
documented vs no-color.org/tic; no new flag/config; honest caveat documented (now naming
*both* "no `Tc`/`RGB` in the entry" *and* "no `infocmp` on the remote"). The win is real but
per-remote and best-effort — stated plainly, not oversold.

### (2) Usability — addressed
Fully automatic; no knob. An SSH session whose remote terminfo advertises `Tc`/`RGB` now
renders RGB themes at full fidelity instead of quantized; nothing else changes. When the
remote can't prove truecolor (no `Tc`/`RGB`, or no `infocmp`), detection stays honestly at
256 and fails open to today's behavior — we respect the terminal's real capability and never
force-up. The fix is **best-effort per remote, not a guarantee**, and the design says so.
The headline behavior is now deterministically testable (exported seam), closing the
verify-ability gap from v1.

### (3) No regressions — addressed
- `ColorLevelFromEnv` and package `init()` **unchanged** → no import-time subprocess for any
  turbotui consumer; turbotui's existing tests stay green and host-independent.
- `ResolveTheme` signature + 120 call sites untouched.
- gogent's terminfo-driven tests use an injected **stub** → hermetic, not host-terminfo
  dependent.
- The **version-pin guard test** (`keybindings_issue401_test.go`) is on the edit list →
  `go test ./...` stays green.
- Behavior changes are bounded and documented (the three Divergence rows); production PTY
  paths are identical to today.
- `infocmp` absent / sandboxed / no entry → graceful fall-back to exact pre-fix behavior; no
  cgo.
- Gates both repos: `gofmt`/`go build`/`go vet`/`golangci-lint` (0 new) clean; `go test ./...`
  green (gogent's pre-existing `TestUserSessionSendMessage` 404 the only accepted failure;
  turbotui has no CI → local gate authoritative).

### (4) Holistic / cross-repo seam — addressed
Detector lives in turbotui (it owns ANSI emission + `Set/GetColorLevel`); gogent owns policy
(`--no-color`, `cfg.NoColor`, palette) and defers capability. The seam
(`ColorLevelFromEnvWithTerminfo` + exported `TerminfoCaps` + exported `InfocmpCaps`) is
**public**, so the "single source of truth" is verifiable from *both* repos and gogent can
deterministically test the truecolor-over-SSH agreement. Downstream effect on turbotui's
other consumers is bounded to a *new, opt-in* function — `init()` behavior is unchanged.
Sequencing: land turbotui first → record merge SHA → bump gogent's pin **and** the
`keybindings_issue401_test.go` assertion → land gogent ("Closes #549"); turbotui PR
references gogent #549. gogent half serializes in the ui/tui lane (after #548); turbotui half
is conflict-free with in-flight gogent work.

---

## Regression risks (watch-list)
- **`infocmp` output format variance.** Numeric caps may be hex (`colors#0x100`) or decimal;
  parse base-0. `-x` mandatory for `Tc`/`RGB`. Unknown-format / missing `infocmp` → fail
  open to env-only, never worse than today.
- **No import-time subprocess.** terminfo is consulted only via `DetectColorLevel()` /
  `ColorLevelFromEnvWithTerminfo(...,non-nil)`, which hosts call explicitly (gogent at entry
  points) — *not* from `init()`.
- **Enum mapping drift.** turbotui↔gogent `ColorLevel` mapped via explicit switch, not cast.
- **Live theme switch.** Re-installing the level on `SetTheme` keeps the two layers aligned
  after an env/theme change mid-session.
- **Behavior changes** (empty-TERM none→16; mixed-case COLORTERM and `vt256` downgrades) are
  documented above; verify none are exercised by existing gogent tests beyond the one
  `missing term` row already accounted for.

## Open questions (with recommendations)
1. **Empty/unset `TERM` direction.** Recommend gogent adopts turbotui's `→16` (one gogent
   test row updated; documented behavior change, not inert). Alternative (turbotui `→none`)
   perturbs turbotui standalone consumers and their test — rejected. Confirm acceptable.
2. **`RGB` boolean alongside `Tc`.** **Resolved: yes, mandatory** — `*-direct` entries
   advertise `RGB`, not `Tc` (verified). Both recognized as truecolor.
3. **Export the terminfo seam.** **Resolved: yes** — export `ColorLevelFromEnvWithTerminfo`,
   `TerminfoCaps`, and `InfocmpCaps` so gogent can inject a stub and test the headline case
   deterministically (the v1 unexported-seam choice created the testability defect).
4. **`infocmp -1` portability.** Assumed present (GNU/ncurses, macOS). If a target lacks
   `-1`, drop it and split on commas — behaviorally identical. Confirm target set.
