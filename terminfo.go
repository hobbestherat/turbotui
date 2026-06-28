package tui

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// TerminfoCaps looks up the colour-related capabilities of a terminal's terminfo
// entry, keyed by TERM. It reports the numeric "colors" capability, whether the
// entry advertises 24-bit colour (the boolean "Tc" or "RGB" capability), and
// ok=false when the entry — or the tool used to read it — is unavailable, so the
// caller can fall back to environment-only detection.
//
// It is the seam through which ColorLevelFromEnvWithTerminfo reads terminfo:
// production passes InfocmpCaps; tests pass a stub so they never depend on the
// host's real terminfo database.
type TerminfoCaps func(term string) (colors int, truecolor bool, ok bool)

// infocmpTimeout bounds the infocmp call so a wedged or slow terminfo tool cannot
// stall startup detection. infocmp is normally near-instant.
const infocmpTimeout = 2 * time.Second

// InfocmpCaps reads term's terminfo entry by shelling out to infocmp(1).
//
// Terminfo is the colour-capability source that survives an SSH hop: sshd sets a
// valid TERM on the remote but rarely forwards COLORTERM, while the remote
// terminfo DB (keyed by that TERM) still carries "colors" and the "Tc"/"RGB"
// truecolor booleans. Reading it lets a truecolor terminal be detected as
// truecolor over SSH.
//
// A minimal infocmp-based lookup is vendored here deliberately, rather than taking
// on a terminfo-parser dependency: the project is stdlib-first and must build
// without cgo. The function fails open (ok=false) whenever infocmp is missing, the
// entry is absent, or the output cannot be parsed — so terminfo can only add
// signal, never make detection worse than environment-only.
func InfocmpCaps(term string) (colors int, truecolor bool, ok bool) {
	if term == "" {
		return 0, false, false
	}
	// TERM is env-trusted, but a value containing a path separator could make
	// infocmp read a compiled terminfo file rather than a DB entry; refuse it.
	if strings.ContainsAny(term, "/\\") {
		return 0, false, false
	}
	bin, err := exec.LookPath("infocmp")
	if err != nil {
		return 0, false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), infocmpTimeout)
	defer cancel()
	// -x surfaces extended (user-defined) capabilities — where the Tc truecolor
	// boolean lives — and -1 prints one capability per line for easy parsing.
	out, err := exec.CommandContext(ctx, bin, "-x", "-1", term).Output()
	if err != nil {
		return 0, false, false
	}
	return parseTerminfoCaps(string(out))
}

// parseTerminfoCaps extracts the "colors" number and the Tc/RGB truecolor booleans
// from infocmp output. Capabilities are written as "name", "name#number" or
// "name=string", comma-separated and (with -1) one per line; the leading entry is
// the terminal's name/aliases and is ignored. We split on both newlines and commas
// so parsing is robust even if a vendor's infocmp ignores -1. Numbers may be
// decimal or 0x-hex — ncurses emits colors#0x100 for a 256-colour entry — so they
// are parsed with base 0. ok becomes true once any recognised capability is seen.
func parseTerminfoCaps(out string) (colors int, truecolor bool, ok bool) {
	fields := strings.FieldsFunc(out, func(r rune) bool { return r == '\n' || r == ',' })
	for _, field := range fields {
		tok := strings.TrimSpace(field)
		if tok == "" {
			continue
		}
		// Split a capability into its name and (for numeric/string caps) its value;
		// sep records which separator was seen so "colors" is only read as numeric.
		name, value, sep := tok, "", byte(0)
		if i := strings.IndexAny(tok, "#="); i >= 0 {
			name, value, sep = tok[:i], tok[i+1:], tok[i]
		}
		switch {
		case name == "Tc" || name == "RGB":
			// Tc and RGB advertise 24-bit colour; RGB may be boolean or numeric, so
			// match on the name regardless of any value.
			truecolor, ok = true, true
		case name == "colors" && sep == '#':
			if n, err := strconv.ParseInt(value, 0, 64); err == nil {
				colors, ok = int(n), true
			}
		}
	}
	return colors, truecolor, ok
}
