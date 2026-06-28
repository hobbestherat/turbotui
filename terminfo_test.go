package tui

import (
	"os/exec"
	"strings"
	"testing"
)

// --- shared helpers (used by terminfo_test.go and color_terminfo_test.go) ---

// infocmpAvailable reports whether infocmp(1) is on PATH, so terminfo
// integration tests can skip on hosts without it. turbotui has no CI, so the
// local dev gate is authoritative and these guarded tests run there.
func infocmpAvailable(t *testing.T) bool {
	t.Helper()
	_, err := exec.LookPath("infocmp")
	return err == nil
}

// makeInfocmpOutput builds a realistic `infocmp -x -1` transcript: a leading
// "#\tReconstructed…" comment line, the entry name/aliases header, then each
// capability tab-indented and comma-terminated — exactly the ncurses format the
// parser must handle.
func makeInfocmpOutput(entry string, caps ...string) string {
	var b strings.Builder
	b.WriteString("#\tReconstructed via infocmp\n")
	b.WriteString(entry + ",\n")
	for _, c := range caps {
		b.WriteString("\t" + c + ",\n")
	}
	return b.String()
}

// tiRecorder is a stub TerminfoCaps returning fixed values and recording how
// often (and for which term) it was called, so precedence tests can assert the
// short-circuit (terminfo must not be consulted when an earlier rule resolves).
type tiRecorder struct {
	calls     int
	last      string
	colors    int
	truecolor bool
	ok        bool
}

func (r *tiRecorder) caps() TerminfoCaps {
	return func(term string) (int, bool, bool) {
		r.calls++
		r.last = term
		return r.colors, r.truecolor, r.ok
	}
}

// --- parseTerminfoCaps unit tests ---

func TestParseTerminfoCaps(t *testing.T) {
	cases := []struct {
		name       string
		out        string
		wantColors int
		wantTC     bool
		wantOK     bool
	}{
		{
			name:       "xterm-256color shape: hex colors#0x100, no Tc/RGB",
			out:        makeInfocmpOutput("xterm-256color|xterm with 256 colors", "OTbs", "am", "colors#0x100", "pairs#0x10000"),
			wantColors: 256, wantTC: false, wantOK: true,
		},
		{
			name:       "decimal colors#256",
			out:        makeInfocmpOutput("screen-256color", "colors#256"),
			wantColors: 256, wantTC: false, wantOK: true,
		},
		{
			name:       "Tc boolean extension -> truecolor",
			out:        makeInfocmpOutput("tmux-256color|tmux with 256 colors", "colors#256", "Tc"),
			wantColors: 256, wantTC: true, wantOK: true,
		},
		{
			name:       "RGB boolean (xterm-direct) -> truecolor",
			out:        makeInfocmpOutput("xterm-direct|direct-color", "RGB", "colors#0x1000000"),
			wantColors: 0x1000000, wantTC: true, wantOK: true,
		},
		{
			name:       "RGB numeric (RGB#8) still truecolor by name",
			out:        makeInfocmpOutput("some-direct", "RGB#8", "colors#0x1000000"),
			wantColors: 0x1000000, wantTC: true, wantOK: true,
		},
		{
			name:       "colors#8 (plain xterm) -> 8, no truecolor",
			out:        makeInfocmpOutput("xterm|X11", "colors#8"),
			wantColors: 8, wantTC: false, wantOK: true,
		},
		{
			name:       "header + comment only, no recognised cap -> ok=false",
			out:        makeInfocmpOutput("weird-term|alias"),
			wantColors: 0, wantTC: false, wantOK: false,
		},
		{
			name:       "empty output -> ok=false",
			out:        "",
			wantColors: 0, wantTC: false, wantOK: false,
		},
		{
			name:       "garbage output -> ok=false",
			out:        "this is not terminfo output at all\nno caps here",
			wantColors: 0, wantTC: false, wantOK: false,
		},
		{
			name:       "cap line without trailing comma still parsed",
			out:        "xterm\n\tcolors#256\n",
			wantColors: 256, wantTC: false, wantOK: true,
		},
		{
			name:       "string capability (setrgbf=…) ignored, colors still read",
			out:        makeInfocmpOutput("xterm", "setrgbf=\\E[38;2;%p1%d", "colors#256"),
			wantColors: 256, wantTC: false, wantOK: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			colors, truecolor, ok := parseTerminfoCaps(tc.out)
			if colors != tc.wantColors || truecolor != tc.wantTC || ok != tc.wantOK {
				t.Fatalf("parseTerminfoCaps = (colors=%d, truecolor=%v, ok=%v), want (%d, %v, %v)",
					colors, truecolor, ok, tc.wantColors, tc.wantTC, tc.wantOK)
			}
		})
	}
}

// --- InfocmpCaps fail-open tests (deterministic, no host terminfo dependency) ---

func TestInfocmpCapsFailOpen(t *testing.T) {
	t.Run("empty term -> ok=false", func(t *testing.T) {
		if _, _, ok := InfocmpCaps(""); ok {
			t.Fatalf("InfocmpCaps(\"\") ok=true, want false (empty TERM must fail open)")
		}
	})

	t.Run("unknown term -> ok=false", func(t *testing.T) {
		// Whether infocmp is absent (LookPath fails) or present but exits non-zero
		// for an unknown entry, the result must be ok=false (fail open).
		if _, _, ok := InfocmpCaps("gogent-nonexistent-terminal-zzz-9999"); ok {
			t.Fatalf("InfocmpCaps(unknown term) ok=true, want false")
		}
	})

	t.Run("infocmp not on PATH -> ok=false", func(t *testing.T) {
		// Force exec.LookPath to fail by pointing PATH at a non-existent dir.
		// This exercises the LookPath branch directly, regardless of whether
		// infocmp is installed on the host.
		t.Setenv("PATH", "/nonexistent-turbotui-test-path")
		if _, _, ok := InfocmpCaps("xterm"); ok {
			t.Fatalf("InfocmpCaps with infocmp absent from PATH ok=true, want false")
		}
	})
}

// --- InfocmpCaps integration tests (guarded by infocmp availability) ---

func TestInfocmpCapsIntegration(t *testing.T) {
	if !infocmpAvailable(t) {
		t.Skip("infocmp not available; local gate host has it")
	}

	t.Run("xterm-256color: >=256 colours, no Tc", func(t *testing.T) {
		colors, truecolor, ok := InfocmpCaps("xterm-256color")
		if !ok {
			t.Skip("xterm-256color terminfo entry absent on this host")
		}
		if colors < 256 {
			t.Fatalf("xterm-256color colors=%d, want >=256", colors)
		}
		if truecolor {
			t.Fatalf("generic xterm-256color advertises Tc/RGB=true; want false (honest-256 caveat)")
		}
	})

	t.Run("xterm-direct: RGB -> truecolor", func(t *testing.T) {
		_, truecolor, ok := InfocmpCaps("xterm-direct")
		if !ok {
			t.Skip("xterm-direct terminfo entry absent on this host")
		}
		if !truecolor {
			t.Fatalf("xterm-direct truecolor=false, want true (RGB boolean)")
		}
	})
}

// TestParseTerminfoCapsFromRealInfocmp feeds the actual infocmp output for an
// RGB-bearing entry through the parser, end-to-end (guarded).
func TestParseTerminfoCapsFromRealInfocmp(t *testing.T) {
	if !infocmpAvailable(t) {
		t.Skip("infocmp not available")
	}
	out, err := exec.Command("infocmp", "-x", "-1", "xterm-direct").Output()
	if err != nil {
		t.Skip("xterm-direct entry not readable on this host")
	}
	colors, truecolor, ok := parseTerminfoCaps(string(out))
	if !ok || !truecolor {
		t.Fatalf("real infocmp xterm-direct parsed as (colors=%d truecolor=%v ok=%v), want truecolor & ok", colors, truecolor, ok)
	}
}
