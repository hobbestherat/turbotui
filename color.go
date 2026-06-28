package tui

import (
	"os"
	"strings"
	"sync/atomic"
)

// ColorLevel describes how much colour the active terminal can render. The
// renderer downsamples every Color to the active level before emitting SGR
// codes, so a theme authored in truecolor still renders sensibly on a 16-colour
// terminal and disappears entirely under NO_COLOR.
type ColorLevel uint8

const (
	// ColorLevelNone disables colour entirely (honours the NO_COLOR convention,
	// https://no-color.org/). Bold/underline attributes are still emitted.
	ColorLevelNone ColorLevel = iota
	// ColorLevel16 supports the 16 standard ANSI colours only.
	ColorLevel16
	// ColorLevel256 supports the xterm 256-colour palette.
	ColorLevel256
	// ColorLevelTrueColor supports 24-bit RGB.
	ColorLevelTrueColor
)

// colorLevel holds the active level. It is read on the render goroutine and may
// be set from another, so it is accessed atomically.
var colorLevel atomic.Uint32

func init() {
	colorLevel.Store(uint32(DetectColorLevel()))
}

// SetColorLevel overrides the colour level used by the renderer. Apps rarely
// need this — DetectColorLevel runs at startup — but it lets a host force a
// level (e.g. a "--no-color" flag mapping to ColorLevelNone).
func SetColorLevel(level ColorLevel) {
	colorLevel.Store(uint32(level))
}

// GetColorLevel reports the active colour level.
func GetColorLevel() ColorLevel {
	return ColorLevel(colorLevel.Load())
}

// DetectColorLevel chooses a colour level from the process environment and, when
// available, the terminal's terminfo entry. It is the production detector: it
// consults terminfo (via InfocmpCaps), which is the one colour-capability source
// that survives an SSH hop. See ColorLevelFromEnvWithTerminfo for the exact rules.
//
// Note that package init() uses the env-only ColorLevelFromEnv, not this function,
// so merely importing the package never shells out to infocmp. A host that wants
// terminfo-aware detection installs it once at startup with
// SetColorLevel(DetectColorLevel()).
func DetectColorLevel() ColorLevel {
	return ColorLevelFromEnvWithTerminfo(os.LookupEnv, InfocmpCaps)
}

// ColorLevelFromEnv computes the colour level from a lookup function (os.LookupEnv
// in production, a stub in tests) using environment variables only. It is exactly
// ColorLevelFromEnvWithTerminfo with no terminfo reader; see that function for the
// full precedence. Because it never consults terminfo it never spawns a
// subprocess, which is why package init() uses it.
func ColorLevelFromEnv(lookup func(string) (string, bool)) ColorLevel {
	return ColorLevelFromEnvWithTerminfo(lookup, nil)
}

// ColorLevelFromEnvWithTerminfo computes the colour level from environment
// variables and, when ti is non-nil, the terminal's terminfo entry. Terminfo is
// the colour-capability source that survives an SSH hop: sshd sets a valid TERM on
// the remote but rarely forwards COLORTERM, and the remote terminfo DB (keyed by
// the propagated TERM) carries the "colors" number and the "Tc"/"RGB" truecolor
// booleans. Consulting it lets a truecolor terminal be detected as truecolor even
// when COLORTERM was dropped in transit.
//
// Rules, in order (first match wins):
//
//   - NO_COLOR present and non-empty            -> ColorLevelNone (https://no-color.org/)
//   - TERM == "dumb"                            -> ColorLevelNone
//   - COLORTERM == "truecolor" | "24bit"        -> ColorLevelTrueColor
//   - terminfo "Tc" or "RGB" boolean advertised -> ColorLevelTrueColor
//   - terminfo "colors" >= 256                  -> ColorLevel256
//   - TERM contains "256color"                  -> ColorLevel256
//   - otherwise                                 -> ColorLevel16
//
// Terminfo (the two terminfo rules) is consulted only after COLORTERM — the
// authoritative local signal when present, so the common local truecolor session
// resolves without ever spawning infocmp — and before the TERM-substring fallback.
// A nil ti skips the terminfo rules entirely, leaving pure environment detection.
//
// Honest detection: a generic xterm-256color terminfo entry carries colors#256 but
// no Tc/RGB, so an SSH session whose propagated TERM advertises only 256 colours
// still (correctly) detects 256 — detection is faithful to observable signals and
// cannot invent capability. When ti reports ok=false (no infocmp, no entry) the
// result is identical to env-only detection.
func ColorLevelFromEnvWithTerminfo(lookup func(string) (string, bool), ti TerminfoCaps) ColorLevel {
	// NO_COLOR: any non-empty value disables colour, regardless of its content.
	if v, ok := lookup("NO_COLOR"); ok && v != "" {
		return ColorLevelNone
	}
	term, _ := lookup("TERM")
	if term == "dumb" {
		return ColorLevelNone
	}
	if v, ok := lookup("COLORTERM"); ok {
		if v == "truecolor" || v == "24bit" {
			return ColorLevelTrueColor
		}
	}
	// Terminfo is consulted only when COLORTERM did not already prove truecolor, so
	// a local truecolor session (COLORTERM set) never pays for an infocmp spawn.
	if ti != nil {
		if colors, truecolor, ok := ti(term); ok {
			if truecolor {
				return ColorLevelTrueColor
			}
			if colors >= 256 {
				return ColorLevel256
			}
		}
	}
	if strings.Contains(term, "256color") {
		return ColorLevel256
	}
	return ColorLevel16
}

// rgb unpacks an RGB color's components.
func (c Color) rgb() (uint8, uint8, uint8) {
	return uint8(c.Value >> 16), uint8(c.Value >> 8), uint8(c.Value)
}

// adaptColor maps c down to what the given level can render. It never upgrades a
// color (an ANSI index stays as-is on a truecolor terminal); it only degrades
// RGB/256 values that the terminal cannot show.
func adaptColor(c Color, level ColorLevel) Color {
	switch level {
	case ColorLevelNone:
		return DefaultColor()
	case ColorLevelTrueColor:
		return c
	case ColorLevel256:
		if c.Mode == ColorRGB {
			return ANSIColor(rgbTo256(c.rgb()))
		}
		return c
	default: // ColorLevel16
		switch c.Mode {
		case ColorRGB:
			return ANSIColor(rgbTo16(c.rgb()))
		case ColorANSI:
			if c.Value >= 16 {
				return ANSIColor(rgbTo16(ansi256ToRGB(uint8(c.Value))))
			}
			return c
		}
		return c
	}
}

// cubeLevels are the six RGB levels of the xterm 6×6×6 colour cube.
var cubeLevels = [6]int{0, 95, 135, 175, 215, 255}

// cubeIndex maps a single 0-255 channel onto its nearest cube level index (0-5).
func cubeIndex(v uint8) int {
	if v < 48 {
		return 0
	}
	if v < 115 {
		return 1
	}
	return (int(v) - 35) / 40
}

func dist3(r1, g1, b1, r2, g2, b2 int) int {
	dr, dg, db := r1-r2, g1-g2, b1-b2
	return dr*dr + dg*dg + db*db
}

// rgbTo256 converts an RGB triple to the nearest xterm-256 index, choosing
// between the 6×6×6 colour cube and the 24-step grayscale ramp by distance.
func rgbTo256(r, g, b uint8) uint8 {
	ri, gi, bi := cubeIndex(r), cubeIndex(g), cubeIndex(b)
	cr, cg, cb := cubeLevels[ri], cubeLevels[gi], cubeLevels[bi]
	cubeIdx := 16 + 36*ri + 6*gi + bi
	cubeDist := dist3(int(r), int(g), int(b), cr, cg, cb)

	gray := (int(r) + int(g) + int(b)) / 3
	gi24 := (gray - 8) / 10
	if gi24 < 0 {
		gi24 = 0
	}
	if gi24 > 23 {
		gi24 = 23
	}
	gv := 8 + 10*gi24
	grayDist := dist3(int(r), int(g), int(b), gv, gv, gv)
	if grayDist < cubeDist {
		return uint8(232 + gi24)
	}
	return uint8(cubeIdx)
}

// std16 are canonical RGB values for the 16 ANSI colours (xterm defaults), used
// to map RGB/256 values down to the nearest basic colour.
var std16 = [16][3]int{
	{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0},
	{0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
	{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
	{92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

// rgbTo16 finds the nearest of the 16 standard ANSI colours to an RGB triple.
func rgbTo16(r, g, b uint8) uint8 {
	best, bestDist := 0, 1<<31-1
	for i, c := range std16 {
		d := dist3(int(r), int(g), int(b), c[0], c[1], c[2])
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return uint8(best)
}

// ansi256ToRGB approximates the RGB value of an xterm-256 palette index.
func ansi256ToRGB(index uint8) (uint8, uint8, uint8) {
	i := int(index)
	switch {
	case i < 16:
		c := std16[i]
		return uint8(c[0]), uint8(c[1]), uint8(c[2])
	case i >= 232:
		v := uint8(8 + 10*(i-232))
		return v, v, v
	default:
		i -= 16
		r := cubeLevels[i/36]
		g := cubeLevels[(i/6)%6]
		b := cubeLevels[i%6]
		return uint8(r), uint8(g), uint8(b)
	}
}
