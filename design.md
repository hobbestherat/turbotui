# Design — TUI mis-detects terminal color over SSH (gogent #549)

Cross-repo fix. **turbotui owns the canonical color-level detector; gogent defers to it.**
This is a *detection-correctness fix*, not a new knob: no CLI flags, no config keys, no
new module dependencies, no cgo. We add **terminfo** as a detection input (the one
capability source that survives SSH) and collapse two independent env-only detectors into
one shared implementation.

## Problem recap

Workflow **A** (affected): `ssh host` then run `gogent` on the remote. sshd sets a valid
`TERM` on the remote but does *not* forward `COLORTERM` (unless admin-whitelisted). So
remote gogent sees `TERM=xterm-256color`, empty `COLORTERM`, and concludes 256-color even
when the local emulator is truecolor — RGB themes get quantized.

Workflow **B** (not affected): `gogent --connect ssh://host` runs the TUI *locally* and
reads the local `COLORTERM` directly. Must stay correct.

Two root causes:
1. **Detection is env-only** — `COLORTERM`/`TERM`/`NO_COLOR`. Terminfo (`colors`, `Tc`),
   which lives in the *remote* terminfo DB keyed by the propagated `TERM` and therefore
   survives SSH, is never consulted.
2. **Two detectors can disagree** — gogent `detectColorLevel` (theme resolver) and
   turbotui `ColorLevelFromEnv` (cell renderer) each re-implement env rules. They already
   diverge on edge cases (see "Divergences" below) and would drift further once terminfo
   is added to only one.

## Approach (lowest-risk, single source of truth)

**turbotui is the one detector.** It gains terminfo consultation. gogent's
`detectColorLevel` stops re-implementing env rules and *delegates* to turbotui's detector,
mapping the result into gogent's parallel `ColorLevel` enum. Both layers therefore run the
*same detection code over the same inputs* and cannot drift. The runtime render level is
installed once per entry point via turbotui's existing `SetColorLevel`, so the cell
renderer and the theme resolver agree on the active level.

We deliberately **do not** change `ResolveTheme`'s signature (it has 120 call sites across
~20 gogent test files). Instead we change only the *body* of `detectColorLevel`, add a
runtime-install seam, and add terminfo to turbotui. This keeps blast radius to two
functions plus one new file per repo's tests.

### Precedence (documented; verified against no-color.org + ncurses `terminfo(5)`/`tic`)

Evaluated in this exact order; first match wins:

1. `NO_COLOR` present and non-empty → **none** (always wins; https://no-color.org/)
2. `TERM == "dumb"` → **none**
3. `COLORTERM == "truecolor" | "24bit"` → **truecolor**
4. terminfo boolean `Tc` (or `RGB`) advertised → **truecolor**   ← new
5. terminfo numeric `colors` ≥ 256 → **256**                      ← new
6. `TERM` contains `"256color"` → **256** (substring fallback when terminfo unavailable)
7. otherwise → **16**

Terminfo (steps 4–5) sits *after* `COLORTERM` (the authoritative local signal, when
present) and *before* the `TERM`-substring fallback — so the common local truecolor case
(COLORTERM set) never even shells out to terminfo, and the SSH case (COLORTERM dropped)
gets the truecolor answer from `Tc`.

### Honest-detection caveat (documented in code)

A *generic* `xterm-256color` terminfo entry historically carries `colors#256` but **no
`Tc`**. So an SSH session whose propagated `TERM` is exactly `xterm-256color` with
`COLORTERM` dropped still **correctly** detects 256 — we make detection faithful to
observable signals; we cannot invent capability. The win is the common case where terminfo
*does* advertise `Tc`/`RGB` (`xterm-direct`, `alacritty`, `wezterm`, `tmux-256color` and
distro-patched `xterm-256color` on tic-2.x systems) and `COLORTERM` is the lost signal.

---

## Files & functions

### turbotui (`github.com/hobbestherat/turbotui`) — canonical detector

**`color.go`**
- `ColorLevelFromEnv(lookup func(string)(string,bool)) ColorLevel` — keep the exported
  signature stable (don't break consumers). Refactor its body to delegate to a new
  unexported `colorLevelFrom(lookup, ti)` passing the default terminfo reader. Insert
  steps 4–5 between the `COLORTERM` check and the `TERM`-substring check. Env rules
  otherwise **unchanged** (no perturbation of turbotui standalone behavior).
- New unexported `func colorLevelFrom(lookup func(string)(string,bool), ti terminfoCaps) ColorLevel`
  — testable seam; tests inject a stub `ti` so they never depend on the host's real
  terminfo DB.
- `DetectColorLevel()` / `SetColorLevel` / `GetColorLevel` / package `init()` — unchanged;
  `GetColorLevel` remains the single runtime handle, read per cell in `cell.go` via
  `adaptColor(c, GetColorLevel())` (atomic load — detection is not re-run per cell).

**`terminfo.go`** (new) — minimal, stdlib-only terminfo reader
- `type terminfoCaps func(term string) (colors int, truecolor bool, ok bool)` — the seam.
- `var terminfoLookup terminfoCaps = infocmpCaps` — package var so `colorLevelFrom` calls
  the default; nothing overrides it in production.
- `func infocmpCaps(term string) (colors int, truecolor bool, ok bool)`:
  - `exec.LookPath("infocmp")`; if absent → `ok=false` (graceful: fall back to env-only).
  - Run `infocmp -x -1 <term>` (the `-x` flag is required to surface the extended boolean
    `Tc`; `-1` one-cap-per-line eases parsing). No stdin; capture stdout only.
  - Parse: split on commas/newlines, trim. A numeric cap is `name#value`; **value may be
    decimal *or* `0x`-hex** (ncurses emits `colors#0x100` for 256 — confirmed on the dev
    host) — use `strconv.ParseInt(v, 0, ...)` so `0x` is honored. A boolean cap is a bare
    `name`. Recognize `colors#…` → `colors`; bare `Tc` or `RGB` → `truecolor=true`.
  - On exec error / empty TERM / no entry → `ok=false`.
- Document the choice in the file header: *vendored minimal `infocmp -x` lookup chosen over
  a terminfo-parser dependency to stay stdlib-first and cgo-free, per project policy;
  fails open to env-only detection when `infocmp` is unavailable.*
- Performance note: `infocmp` is only spawned when steps 1–3 didn't already resolve, i.e.
  never in the common local-truecolor (COLORTERM-set) path; once per detection call
  otherwise (entry points are few/rare). Optional small `map[string]…` memo by TERM if
  profiling ever flags it — not needed initially.

**`color_test.go`** — extend `TestColorLevelFromEnv` via `colorLevelFrom` with a stub
`terminfoCaps`:
- entry advertises `Tc`, no `COLORTERM` → **truecolor** (the SSH-truecolor win)
- `colors#256`, no `Tc`, no `COLORTERM` → **256**
- SSH shape: `TERM=xterm-256color`, no `COLORTERM`, terminfo `colors#256` no `Tc` → **256**
  (unchanged — honest caveat)
- `COLORTERM=truecolor` present → **truecolor**, and assert the stub terminfo reader is
  **not** consulted (precedence/perf guard)
- terminfo unavailable (`ok=false`) → behaves exactly as today (TERM-substring fallback)
- Existing env-only cases stay green (turbotui env semantics untouched).

### gogent (`github.com/hobbestherat/gogent`) — defers to turbotui

**`go.mod`** — bump pin to the merged turbotui SHA: `go get github.com/hobbestherat/turbotui@<sha> && go mod tidy`.

**`ui/tui/theme.go`**
- `detectColorLevel(env func(string) string) ColorLevel` — **keep signature** (preserves
  all `ResolveTheme` call sites). Replace the body: adapt `env` (a `func(string)string`)
  to a `func(string)(string,bool)` lookup (`v := env(k); return v, v != ""`) and call
  turbotui's shared detector, then map turbotui→gogent level via a tiny `fromTUILevel`.
  The two enums are parallel (`None/16/256/True`, same iota order) but map **explicitly**
  (switch), not by numeric cast, so a future enum change can't silently mis-map.
  This deletes gogent's re-implemented env rules — the heart of Cause 2.
- `ResolveTheme(cfg, env, noColorFlag)` — unchanged signature/body; it still calls
  `detectColorLevel(env)`, which now *is* turbotui's detector. `noColorFlag || cfg.NoColor`
  still forces `ColorNone` at this layer (defensive; matches the runtime install below).
- New `func InstallColorLevel(env func(string)string, noColor bool)` (or inline helper):
  computes `detectColorLevel(env)`, overrides to `ColorNone` when `noColor`, and calls
  `tui.SetColorLevel(toTUILevel(level))`. This is the **single runtime install point** so
  the cell renderer's level matches what the theme resolved at. Honouring `--no-color` /
  `cfg.NoColor` here (not just inside `ResolveTheme`) is what makes turbotui's renderer,
  its `tv.DefaultTheme` chrome, and markdown auto-disable all go monochrome together.

**Entry points — install the level once each (before the workbench is built):**
- `cmd/main.go:199` (embedded TUI) — `noColor = *noColor`
- `cmd/attach.go:149` (attach startup) — `noColor = noColorFlag`
- `cmd/attach.go:302` (`installPresentationHandlers` → `SetTheme` live switch) — re-install
  on theme change so a live env/theme change reflects consistently
- `cmd/embedded_handlers.go:145` (`SetTheme` live switch) — same
  Each already calls `tui.ApplyTheme(tui.ResolveTheme(cfg, os.Getenv, noColor))`; we add
  `InstallColorLevel(os.Getenv, noColor)` immediately before so the install and the resolve
  read identical inputs.

**`ui/tui/theme_test.go`**
- `TestDetectColorLevel`: update the **`{"missing term", {}, ColorNone}`** case to
  `Color16` — gogent now adopts turbotui's canonical empty-TERM behavior (see Divergences).
  Documented as inert in production (a real PTY always has `TERM`). All other cases (which
  use lowercase `truecolor`/`24bit` and `256color`/`xterm`) stay green unchanged.
- New `TestColorLevelLayersAgree` (Cause-2 guard): for representative envs — SSH-truecolor
  (`TERM=xterm-256color`, no COLORTERM, terminfo `Tc`), plain 256, NO_COLOR, `--no-color`
  flag — call `InstallColorLevel` then assert `toTUILevel(ResolveTheme(...).Level) ==
  tui.GetColorLevel()`. Inject the same terminfo stub seam used in turbotui (exposed for
  test, or drive via a controlled `TERM`). Restore `tui.SetColorLevel` in cleanup.
- `TestResolveThemeNoColor` — must stay green: NO_COLOR / `--no-color` / `cfg.NoColor` all
  flatten to `ColorNone` at both layers.

---

## Divergences resolved (this is the "two detectors agree now" outcome)

By delegating, gogent adopts turbotui's exact env rules. The pre-existing mismatches:

| input | gogent (old) | turbotui (canonical) | after fix |
|---|---|---|---|
| empty/unset `TERM` | none | 16 | **16** (gogent test updated; inert in prod — PTY always sets TERM) |
| `COLORTERM=TrueColor` (mixed case) | truecolor (lowercased) | not matched (exact) | exact-match — COLORTERM is conventionally lowercase; negligible real-world risk |
| `TERM=vt256` (`"256"` not `"256color"`) | 256 | 16 | 16 — stricter/correct; no known gogent user relies on it |

These are all narrow edge cases; the production-meaningful behavior (`xterm`,
`*-256color`, `COLORTERM=truecolor`, `dumb`, `NO_COLOR`) is identical across both detectors
today, so local users see the **same level as before**.

---

## Design criteria

### (1) Goal match
Terminfo `colors`/`Tc` is now consulted (SSH-safe, survives in the remote DB). Single
shared detector: turbotui canonical, gogent delegates — no second env-rule implementation.
Precedence is explicit and documented against no-color.org + tic docs. Truecolor over SSH
works when terminfo advertises `Tc`/`RGB`. No new flag/config — it's a detection fix, not a
feature. The honest-256 caveat (bare `xterm-256color`, no `Tc`) is documented, not papered
over.

### (2) Usability
Fully automatic — the user does nothing; an SSH session into a truecolor-capable terminal
that advertises `Tc` now renders RGB themes at full fidelity instead of quantized. No
prompt, no knob. When nothing observable advertises truecolor, detection stays honestly at
256 (no force-up override — we respect the terminal's real capability, per the issue). The
result is *surfaced* via the rendered colors themselves; nothing silent goes wrong because
detection fails open to today's env-only behavior when `infocmp` is missing.

### (3) No regressions
- `ResolveTheme` signature and its 120 call sites untouched → no cascade.
- turbotui env semantics unchanged → turbotui standalone consumers (turbotv, examples)
  unaffected; only a new terminfo step *adds* signal, gated behind COLORTERM precedence.
- Only one gogent test expectation changes (`missing term`), in a case that never occurs in
  production; documented.
- Workflow B (local attach) reads local `COLORTERM` → still truecolor; untouched.
- `NO_COLOR` / `--no-color` / `cfg.NoColor` → none at *both* layers (the new runtime install
  strengthens this).
- `infocmp` absent / sandboxed / no entry → graceful fall-back to exact pre-fix behavior;
  no cgo.
- Gates both repos: `gofmt`/`go build`/`go vet`/`golangci-lint` (0 new) clean; `go test ./...`
  green (gogent's pre-existing `TestUserSessionSendMessage` 404 is the only accepted
  failure; turbotui has no CI → local gate authoritative).

### (4) Holistic / cross-repo seam
The capability detector belongs in turbotui — it emits the ANSI and already owns
`Set/GetColorLevel`, the single render handle. gogent owns *policy* (`--no-color`,
`cfg.NoColor`, which palette) and *defers capability* to turbotui. The seam is respected:
turbotui exposes detection + the runtime handle; gogent consumes both and never
re-implements env rules. Downstream effect on turbotui's other consumers is bounded to a
purely additive terminfo step. Sequencing: land turbotui first → record merge SHA → bump
gogent's pin → land gogent ("Closes #549"); turbotui PR references gogent #549. gogent half
serializes in the ui/tui lane (after #548); turbotui half is conflict-free with in-flight
gogent work.

---

## Regression risks (watch-list)
- **`infocmp` output format variance.** Numeric caps may be hex (`colors#0x100`) or decimal
  (`colors#256`); parse with base-0. `-x` is mandatory for `Tc`. If a vendor's `infocmp`
  differs wildly, we fail open to env-only — never worse than today.
- **`init()`-time exec.** turbotui's package `init()` calls `DetectColorLevel()`; with the
  COLORTERM short-circuit it won't exec in the common local case, but a TERM-less/256 path
  spawns `infocmp` once at import. Bounded and cached in the atomic; acceptable.
- **Enum mapping drift.** turbotui↔gogent `ColorLevel` mapped via explicit switch, not cast.
- **Live theme switch.** Re-installing the level on `SetTheme` keeps the two layers aligned
  after an env or theme change mid-session.

## Open questions
1. **Empty/unset `TERM` unification direction.** Recommended: gogent adopts turbotui's
   `→16` (update one gogent test; production inert). Alternative: make turbotui `→none`
   (more conservative; changes turbotui standalone behavior + its test). Picked the former
   to avoid perturbing turbotui's other consumers — confirm acceptable.
2. **Recognize `RGB` boolean as truecolor in addition to `Tc`?** `xterm-direct` uses `RGB`,
   not `Tc`. Recommended: yes (trivial, strictly more correct). Confirm in scope.
3. **`infocmp -1` portability.** Assumed present (GNU/ncurses, macOS). If a target lacks
   `-1`, drop it and split on commas only — behaviorally identical. Confirm target set.
4. **Expose the terminfo stub seam to gogent tests** for `TestColorLevelLayersAgree`, or
   drive agreement purely via a controlled real `TERM`? Recommended: keep the seam
   turbotui-internal and have the gogent agreement test drive via env + a `TERM` whose host
   terminfo is deterministic (or skip terminfo by asserting on the 256/none/flag rows that
   don't need `Tc`). Confirm preference.
