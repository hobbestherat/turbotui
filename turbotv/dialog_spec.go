package tv

// DialogSpec describes how a dialog wants to be sized in terms of intent — floors,
// caps, a content-driven preferred size and edge margin — rather than absolute
// dimensions. ResolveDialogRect turns it into a concrete centered rectangle for a
// given screen, so a dialog grows and shrinks with the terminal instead of being
// pinned to a magic number.
//
// All fields are in terminal cells. A zero field means "no opinion" and falls back
// to the policy default:
//
//   - MinW, MinH: absolute floor the dialog never shrinks below (e.g. 40×10). 0 = no floor.
//   - MaxW, MaxH: absolute cap the dialog never grows past. 0 = cap at screen − 2*Margin.
//   - PreferredW, PrefH: the content-driven ideal. It is honoured even when smaller
//     than the percentage default (80% wide / 85% tall), which acts as an upper
//     *cap*, not a floor — so a dialog sizes to its content and only fills the
//     default share of the screen when it leaves these at 0. 0 = use the default.
//   - Margin: breathing room kept clear on each side. 0 = the default of 2.
//
// A dialog with no content opinion (PreferredW/PrefH left at 0) fills ~80%×85% of
// the terminal; one that declares a content-driven preferred size is sized to that
// (down to the Min floor), with the percentage acting as the upper bound so it
// never grows absurdly large (#309).
type DialogSpec struct {
	MinW, MinH        int
	MaxW, MaxH        int
	PreferredW, PrefH int
	Margin            int
}

// DefaultDialogMargin is the per-side breathing room ResolveDialogRect keeps clear
// when a DialogSpec leaves Margin at 0.
const DefaultDialogMargin = 2

const (
	// dialogWidthPercent and dialogHeightPercent are the largest share of the
	// terminal a dialog grows to: the cap a content-driven preferred size is bounded
	// by, and the size used when a spec declares no preferred size at all (#309).
	dialogWidthPercent  = 80
	dialogHeightPercent = 85
)

// ResolveDialogRect turns a DialogSpec into a concrete, centered dialog rectangle
// for a screenW×screenH terminal. It returns the top-left origin (x, y) and the
// size (w, h).
//
// Policy:
//  1. The size is the spec's PreferredW/PrefH, capped at a percentage of the screen
//     (80% wide, 85% tall); a zero preferred falls back to that percentage (#309).
//  2. That size is clamped to [Min, min(effectiveMax, screen − 2*Margin)], where
//     effectiveMax is the spec's Max when set and otherwise screen − 2*Margin. The
//     Min floor is applied last, so a dialog on a tiny terminal honours its floor
//     even if that slightly exceeds the screen.
//  3. The result is centered, with the origin floored at 0 so it never goes
//     off-screen top/left.
func ResolveDialogRect(spec DialogSpec, screenW, screenH int) (x, y, w, h int) {
	margin := spec.Margin
	if margin == 0 {
		margin = DefaultDialogMargin
	}
	w = resolveDimension(spec.PreferredW, screenW*dialogWidthPercent/100, spec.MinW, spec.MaxW, screenW, margin)
	h = resolveDimension(spec.PrefH, screenH*dialogHeightPercent/100, spec.MinH, spec.MaxH, screenH, margin)
	x = (screenW - w) / 2
	if x < 0 {
		x = 0
	}
	y = (screenH - h) / 2
	if y < 0 {
		y = 0
	}
	return x, y, w, h
}

// resolveDimension applies the width/height policy to one axis: start from the
// caller's preferred size (or the percentage default when it is 0), cap it at the
// percentage default and at min(effectiveMax, screen − 2*margin), then floor it to
// the minimum. The percentage is a ceiling the content grows toward, not a floor it
// is forced up to (#309).
func resolveDimension(preferred, percentDefault, minV, maxV, screen, margin int) int {
	value := preferred
	if value <= 0 {
		value = percentDefault
	}
	// The percentage default is an upper cap on a content-driven preferred size, so
	// a small dialog keeps its content size instead of inflating to the default.
	if percentDefault > 0 && value > percentDefault {
		value = percentDefault
	}
	upper := maxV
	if upper <= 0 {
		upper = screen - 2*margin
	}
	if marginCap := screen - 2*margin; upper > marginCap {
		upper = marginCap
	}
	if value > upper {
		value = upper
	}
	if value < minV {
		value = minV
	}
	return value
}
