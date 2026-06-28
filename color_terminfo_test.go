package tui

import (
	"os"
	"strings"
	"testing"
)

// Compile-time assertion that the terminfo seam is exported (criterion 4:
// turbotui remains the canonical detector and the seam is testable from BOTH
// repos — gogent injects a stub via this type).
var _ TerminfoCaps = InfocmpCaps

// TestColorLevelFromEnvWithTerminfoPrecedence exercises the documented 7-step
// precedence (first match wins) using a stub terminfo reader, so it never
// depends on the host's real terminfo DB.
func TestColorLevelFromEnvWithTerminfoPrecedence(t *testing.T) {
	// ti returns fixed capabilities, ignoring the term it is handed.
	ti := func(colors int, tc, ok bool) TerminfoCaps {
		return func(string) (int, bool, bool) { return colors, tc, ok }
	}

	cases := []struct {
		name string
		env  map[string]string
		ti   TerminfoCaps
		want ColorLevel
	}{
		// 1. NO_COLOR always wins, even over terminfo Tc.
		{"NO_COLOR beats terminfo Tc", map[string]string{"NO_COLOR": "1", "TERM": "xterm-direct"}, ti(256, true, true), ColorLevelNone},
		// 1b. NO_COLOR empty value is ignored.
		{"NO_COLOR empty ignored, terminfo applies", map[string]string{"NO_COLOR": "", "TERM": "xterm-direct"}, ti(256, true, true), ColorLevelTrueColor},
		// 2. dumb beats terminfo Tc.
		{"dumb beats terminfo Tc", map[string]string{"TERM": "dumb"}, ti(256, true, true), ColorLevelNone},
		// 3. COLORTERM wins over terminfo (256-only).
		{"COLORTERM=truecolor beats terminfo 256", map[string]string{"COLORTERM": "truecolor"}, ti(256, false, true), ColorLevelTrueColor},
		{"COLORTERM=24bit beats terminfo", map[string]string{"COLORTERM": "24bit"}, ti(0, false, false), ColorLevelTrueColor},
		// 3b. COLORTERM present but unrecognized does NOT block terminfo.
		{"COLORTERM garbage falls through to terminfo Tc", map[string]string{"COLORTERM": "3", "TERM": "xterm-direct"}, ti(256, true, true), ColorLevelTrueColor},
		// 4. terminfo Tc, no COLORTERM — the SSH-truecolor win.
		{"terminfo Tc -> truecolor (SSH win)", map[string]string{"TERM": "xterm-direct"}, ti(256, true, true), ColorLevelTrueColor},
		// 4b. terminfo RGB boolean -> truecolor.
		{"terminfo RGB -> truecolor", map[string]string{"TERM": "xterm-direct"}, ti(0x1000000, true, true), ColorLevelTrueColor},
		// truecolor takes precedence over a huge colors value.
		{"terminfo truecolor beats huge colors", map[string]string{"TERM": "xterm-direct"}, ti(0x1000000, true, true), ColorLevelTrueColor},
		// 5. terminfo colors >= 256 (hex) -> 256.
		{"terminfo colors#0x100 -> 256", map[string]string{"TERM": "xterm-256color"}, ti(0x100, false, true), ColorLevel256},
		// 5b. terminfo colors = 256 (decimal) -> 256 (boundary, inclusive).
		{"terminfo colors#256 -> 256", map[string]string{"TERM": "xterm-256color"}, ti(256, false, true), ColorLevel256},
		// boundary: colors = 255 (<256) does NOT reach 256; falls to substring/16.
		{"terminfo colors#255 -> 16 (boundary)", map[string]string{"TERM": "weird"}, ti(255, false, true), ColorLevel16},
		// huge colors, no Tc -> still 256.
		{"terminfo colors#0x1000000 no Tc -> 256", map[string]string{"TERM": "xterm-direct"}, ti(0x1000000, false, true), ColorLevel256},
		// terminfo says colors#8 (plain xterm) -> 16.
		{"terminfo colors#8 -> 16", map[string]string{"TERM": "xterm"}, ti(8, false, true), ColorLevel16},
		// SSH shape: TERM=xterm-256color, no COLORTERM, no Tc, colors#256 -> 256 (unchanged).
		{"SSH shape: xterm-256color no COLORTERM no Tc -> 256", map[string]string{"TERM": "xterm-256color"}, ti(256, false, true), ColorLevel256},
		// 6. terminfo unavailable (ok=false) falls back to TERM substring.
		{"terminfo ok=false, 256color substring -> 256", map[string]string{"TERM": "xterm-256color"}, ti(0, false, false), ColorLevel256},
		// 6b. terminfo ok=false, no substring -> 16.
		{"terminfo ok=false, plain xterm -> 16", map[string]string{"TERM": "xterm"}, ti(0, false, false), ColorLevel16},
		// empty TERM: reader returns ok=false -> 16 (matches ColorLevelFromEnv).
		{"empty TERM + terminfo ok=false -> 16", map[string]string{}, ti(0, false, false), ColorLevel16},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ColorLevelFromEnvWithTerminfo(envStub(c.env), c.ti)
			if got != c.want {
				t.Fatalf("ColorLevelFromEnvWithTerminfo(%v) = %d, want %d", c.env, got, c.want)
			}
		})
	}

	// nil ti behaves identically to ColorLevelFromEnv (pure env-only).
	t.Run("nil ti == ColorLevelFromEnv for all env shapes", func(t *testing.T) {
		envs := []map[string]string{
			{"TERM": "xterm-256color"},
			{"TERM": "xterm"},
			{"COLORTERM": "truecolor"},
			{"NO_COLOR": "1"},
			{"TERM": "dumb"},
			{},
		}
		for _, e := range envs {
			if ColorLevelFromEnvWithTerminfo(envStub(e), nil) != ColorLevelFromEnv(envStub(e)) {
				t.Fatalf("nil-ti path disagrees with ColorLevelFromEnv for %v", e)
			}
		}
	})
}

// TestColorLevelFromEnvIgnoresTerminfo pins the env-only contract: ColorLevelFromEnv
// (used by package init) must never consult terminfo, so a TERM whose terminfo
// would advertise truecolor still resolves via env-only rules.
func TestColorLevelFromEnvIgnoresTerminfo(t *testing.T) {
	// xterm-direct has no "256color" substring and no COLORTERM here; env-only
	// therefore yields 16 regardless of what the real terminfo entry says.
	got := ColorLevelFromEnv(envStub(map[string]string{"TERM": "xterm-direct"}))
	if got != ColorLevel16 {
		t.Fatalf("ColorLevelFromEnv(xterm-direct) = %d, want 16 (env-only must ignore terminfo)", got)
	}
	if got != ColorLevelFromEnvWithTerminfo(envStub(map[string]string{"TERM": "xterm-direct"}), nil) {
		t.Fatalf("ColorLevelFromEnv disagrees with ColorLevelFromEnvWithTerminfo(...,nil)")
	}
}

// TestTerminfoShortCircuitedWhenEnvResolves asserts the precedence short-circuit:
// when NO_COLOR / dumb / COLORTERM already resolve the level, terminfo is never
// consulted (no infocmp spawn). This is the "common local truecolor session
// never shells out" guarantee.
func TestTerminfoShortCircuitedWhenEnvResolves(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want ColorLevel
	}{
		{"COLORTERM=truecolor", map[string]string{"COLORTERM": "truecolor"}, ColorLevelTrueColor},
		{"COLORTERM=24bit", map[string]string{"COLORTERM": "24bit"}, ColorLevelTrueColor},
		{"NO_COLOR set", map[string]string{"NO_COLOR": "1"}, ColorLevelNone},
		{"TERM=dumb", map[string]string{"TERM": "dumb"}, ColorLevelNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &tiRecorder{colors: 256, truecolor: true, ok: true}
			got := ColorLevelFromEnvWithTerminfo(envStub(c.env), r.caps())
			if got != c.want {
				t.Fatalf("got level %d, want %d", got, c.want)
			}
			if r.calls != 0 {
				t.Fatalf("terminfo consulted %d time(s) (last term=%q); want 0 calls — env must short-circuit before terminfo",
					r.calls, r.last)
			}
		})
	}
}

// TestTerminfoConsultedOnlyAfterEnvMisses confirms terminfo IS reached (and the
// TERM is forwarded to it) when no env rule resolves — the path that lets an
// SSH session with COLORTERM dropped recover truecolor from Tc.
func TestTerminfoConsultedOnlyAfterEnvMisses(t *testing.T) {
	r := &tiRecorder{colors: 256, truecolor: true, ok: true}
	got := ColorLevelFromEnvWithTerminfo(envStub(map[string]string{"TERM": "xterm-direct"}), r.caps())
	if got != ColorLevelTrueColor {
		t.Fatalf("got %d, want truecolor", got)
	}
	if r.calls != 1 {
		t.Fatalf("terminfo called %d time(s), want exactly 1", r.calls)
	}
	if r.last != "xterm-direct" {
		t.Fatalf("terminfo called with term %q, want %q", r.last, "xterm-direct")
	}
}

// TestTerminfoAddsTruecolorOverEnvOnly demonstrates the actual fix end-to-end:
// for an RGB-advertising entry, env-only says 16 while terminfo-aware says
// truecolor. Guarded so it skips where the entry/infocmp is absent.
func TestTerminfoAddsTruecolorOverEnvOnly(t *testing.T) {
	if !infocmpAvailable(t) {
		t.Skip("infocmp not available")
	}
	lookup := envStub(map[string]string{"TERM": "xterm-direct"})
	envOnly := ColorLevelFromEnv(lookup)
	aware := ColorLevelFromEnvWithTerminfo(lookup, InfocmpCaps)
	if envOnly != ColorLevel16 {
		t.Fatalf("env-only xterm-direct = %d, want 16", envOnly)
	}
	if aware != ColorLevelTrueColor {
		if aware == ColorLevel16 {
			t.Skip("xterm-direct terminfo entry absent on this host")
		}
		t.Fatalf("terminfo-aware xterm-direct = %d, want truecolor (RGB)", aware)
	}
}

// TestRuntimeColorLevelHandle verifies turbotui remains the single runtime source
// via Set/GetColorLevel and that the per-cell adaptColor honours the installed
// level (criterion 4).
func TestRuntimeColorLevelHandle(t *testing.T) {
	saved := GetColorLevel()
	defer SetColorLevel(saved)

	rgb := RGBColor(0xC6, 0x8F, 0xD6)
	for _, lvl := range []ColorLevel{ColorLevelNone, ColorLevel16, ColorLevel256, ColorLevelTrueColor} {
		SetColorLevel(lvl)
		if got := GetColorLevel(); got != lvl {
			t.Fatalf("GetColorLevel() = %d, want %d", got, lvl)
		}
		switch lvl {
		case ColorLevelNone:
			if got := adaptColor(rgb, GetColorLevel()); got != DefaultColor() {
				t.Fatalf("level None: adaptColor = %+v, want DefaultColor", got)
			}
		case ColorLevelTrueColor:
			if got := adaptColor(rgb, GetColorLevel()); got != rgb {
				t.Fatalf("level TrueColor: adaptColor = %+v, want RGB passthrough", got)
			}
		case ColorLevel256:
			if got := adaptColor(rgb, GetColorLevel()); got.Mode != ColorANSI {
				t.Fatalf("level 256: adaptColor = %+v, want ANSI index", got)
			}
		case ColorLevel16:
			if got := adaptColor(rgb, GetColorLevel()); got.Mode != ColorANSI {
				t.Fatalf("level 16: adaptColor = %+v, want ANSI index", got)
			}
		}
	}
}

// TestDetectColorLevelEnvShortCircuits checks the production detector's env
// short-circuits via os.LookupEnv. NO_COLOR and COLORTERM resolve before
// terminfo, so these are deterministic without depending on infocmp.
func TestDetectColorLevelEnvShortCircuits(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TERM", "xterm-direct")
	if got := DetectColorLevel(); got != ColorLevelNone {
		t.Fatalf("DetectColorLevel with NO_COLOR set = %d, want None", got)
	}

	// NO_COLOR present-but-empty is ignored, so COLORTERM then wins.
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORTERM", "truecolor")
	if got := DetectColorLevel(); got != ColorLevelTrueColor {
		t.Fatalf("DetectColorLevel with COLORTERM=truecolor = %d, want TrueColor", got)
	}
}

// TestInitDoesNotShellOutAtImport guards a design contract (design.md:103 and the
// DetectColorLevel doc comment): package init() must install the env-only level
// via ColorLevelFromEnv — NOT DetectColorLevel() — so merely importing turbotui
// never shells out to infocmp. DetectColorLevel became terminfo-aware in this
// change; if init() still calls it, every import spawns infocmp (a criterion-3/4
// regression). This reads the source so the contract cannot silently regress.
func TestInitDoesNotShellOutAtImport(t *testing.T) {
	body := readFuncBody(t, "color.go", "func init()")
	if strings.Contains(body, "DetectColorLevel(") {
		t.Fatalf("init() shells out at import: its body calls DetectColorLevel() (now terminfo-aware). "+
			"Per the approved design (design.md:103) and the DetectColorLevel doc comment, init() must call "+
			"the env-only ColorLevelFromEnv(os.LookupEnv) so importing the package never spawns infocmp.\n"+
			"init body:\n%s", body)
	}
	if !strings.Contains(body, "ColorLevelFromEnv(") {
		t.Fatalf("init() does not install a level via ColorLevelFromEnv.\ninit body:\n%s", body)
	}
}

// readFuncBody extracts the body lines of the top-level function whose
// declaration begins with decl (e.g. "func init()"), up to its closing brace.
func readFuncBody(t *testing.T, path, decl string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var body []string
	inFunc := false
	for _, ln := range strings.Split(string(data), "\n") {
		if !inFunc {
			if strings.HasPrefix(ln, decl) {
				inFunc = true
			}
			continue
		}
		if ln == "}" { // gofmt: closing brace on its own line at column 0
			break
		}
		body = append(body, ln)
	}
	if !inFunc {
		t.Fatalf("%q not found in %s", decl, path)
	}
	return strings.Join(body, "\n")
}
