package tv

import (
	"strings"
	"testing"
)

// These tests cover gogent issue 298 / the turbotui root cause: a TextView pinned
// to the bottom (follow == true) must RE-ANCHOR to the new last line when the
// viewport is RESIZED, not merely clamp down. A non-following view (the user
// scrolled up) must instead keep its scroll position across a resize.

// resizeView changes the widget's bounds and redraws, mirroring what the host does
// when a window is resized: the new geometry takes effect on the next draw, where
// clampScroll runs.
func resizeView(desktop *Desktop, view *TextView, bounds Rect) {
	view.Component.SetBounds(bounds)
	desktop.Redraw()
}

// maxScrollNow returns the bottom-most valid scroll offset for the view's current
// bounds, derived from the live row layout so width/wrap-sensitive cases need no
// hand-computed constants.
func maxScrollNow(view *TextView, h int) int {
	rows, _, _ := view.metrics(view.Component.AbsoluteBounds())
	max := len(rows) - h
	if max < 0 {
		max = 0
	}
	return max
}

func makeLines(n int, text string) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = text
	}
	return strings.Join(lines, "\n")
}

// A following view whose viewport SHRINKS (smaller height -> a LARGER maxScroll)
// must re-anchor UP to the new bottom. This is the core regression: a clamp that
// only reduces scrollY would strand the view at the stale, smaller maxScroll.
func TestTextViewFollowReanchorsOnHeightShrink(t *testing.T) {
	desktop, view := setupTextView(40, 40, Rect{X: 0, Y: 0, W: 20, H: 10})
	view.Wrap = false
	view.SetText(makeLines(50, "row"))
	desktop.Redraw()

	if view.scrollY != 40 {
		t.Fatalf("precondition: following view should pin to bottom (40), got %d", view.scrollY)
	}

	// Shrink the height from 10 to 5: 50 rows in a 5-row pane -> maxScroll is now 45.
	resizeView(desktop, view, Rect{X: 0, Y: 0, W: 20, H: 5})
	if want := 45; view.scrollY != want {
		t.Fatalf("after height shrink a following view must re-anchor to the new bottom (%d), got %d", want, view.scrollY)
	}
	if !view.follow {
		t.Fatalf("a resize must not clear follow on a following view")
	}
}

// A following view whose viewport GROWS (larger height -> a SMALLER maxScroll)
// must also end up exactly at the new bottom.
func TestTextViewFollowReanchorsOnHeightGrow(t *testing.T) {
	desktop, view := setupTextView(40, 40, Rect{X: 0, Y: 0, W: 20, H: 10})
	view.Wrap = false
	view.SetText(makeLines(50, "row"))
	desktop.Redraw()
	if view.scrollY != 40 {
		t.Fatalf("precondition: following view should pin to bottom (40), got %d", view.scrollY)
	}

	// Grow the height from 10 to 20: 50 rows in a 20-row pane -> maxScroll is now 30.
	resizeView(desktop, view, Rect{X: 0, Y: 0, W: 20, H: 20})
	if want := 30; view.scrollY != want {
		t.Fatalf("after height grow a following view must sit at the new bottom (%d), got %d", want, view.scrollY)
	}
	if !view.follow {
		t.Fatalf("a resize must not clear follow on a following view")
	}
}

// When the viewport grows large enough that all content fits, a following view
// anchors at the top (maxScroll collapses to 0) and keeps following.
func TestTextViewFollowReanchorsToZeroWhenContentFits(t *testing.T) {
	desktop, view := setupTextView(40, 40, Rect{X: 0, Y: 0, W: 20, H: 10})
	view.Wrap = false
	view.SetText(makeLines(20, "row"))
	desktop.Redraw()
	if view.scrollY != 10 {
		t.Fatalf("precondition: 20 rows in H=10 should pin to 10, got %d", view.scrollY)
	}

	// 20 rows now fit in a 30-row pane: maxScroll is 0.
	resizeView(desktop, view, Rect{X: 0, Y: 0, W: 20, H: 30})
	if view.scrollY != 0 {
		t.Fatalf("a following view whose content fits should anchor at 0, got %d", view.scrollY)
	}
	if !view.follow {
		t.Fatalf("follow must be preserved when content fits")
	}
}

// A following view must stay pinned across a SEQUENCE of resizes, each time landing
// on the live bottom — the property the fix guarantees frame after frame.
func TestTextViewFollowStaysPinnedAcrossResizeSequence(t *testing.T) {
	desktop, view := setupTextView(40, 40, Rect{X: 0, Y: 0, W: 20, H: 10})
	view.Wrap = false
	view.SetText(makeLines(60, "row"))
	desktop.Redraw()

	for _, h := range []int{7, 25, 3, 18, 10} {
		resizeView(desktop, view, Rect{X: 0, Y: 0, W: 20, H: h})
		want := maxScrollNow(view, h)
		if view.scrollY != want {
			t.Fatalf("following view at H=%d should be at bottom %d, got %d", h, want, view.scrollY)
		}
	}
}

// A NON-following view (the user scrolled up) must keep its scroll position across
// a resize when that position is still in range — for both a smaller and a larger
// viewport.
func TestTextViewNonFollowPreservesScrollOnResize(t *testing.T) {
	desktop, view := setupTextView(40, 40, Rect{X: 0, Y: 0, W: 20, H: 10})
	view.Wrap = false
	view.SetText(makeLines(50, "row"))
	desktop.Redraw()

	// Scroll up off the bottom: follow clears and scrollY holds at 20.
	view.scrollBy(-20)
	if view.follow {
		t.Fatalf("scrolling up must clear follow")
	}
	if view.scrollY != 20 {
		t.Fatalf("precondition: expected scrollY=20 after scrolling up, got %d", view.scrollY)
	}

	// Shrink to H=5 (maxScroll 45): 20 is still in range, so it is preserved.
	resizeView(desktop, view, Rect{X: 0, Y: 0, W: 20, H: 5})
	if view.scrollY != 20 {
		t.Fatalf("non-following view must keep scrollY=20 after shrink, got %d", view.scrollY)
	}
	if view.follow {
		t.Fatalf("a resize must not re-enable follow on a non-following view")
	}

	// Grow to H=25 (maxScroll 25): 20 is still in range, so it is preserved.
	resizeView(desktop, view, Rect{X: 0, Y: 0, W: 20, H: 25})
	if view.scrollY != 20 {
		t.Fatalf("non-following view must keep scrollY=20 after grow, got %d", view.scrollY)
	}
	if view.follow {
		t.Fatalf("a resize must not re-enable follow on a non-following view")
	}
}

// A non-following view whose preserved position falls OUT of range after a resize
// (the viewport grew past it) is clamped down to the new maxScroll — but follow
// stays off, so it does not snap to the very bottom.
func TestTextViewNonFollowClampsWhenResizePushesOutOfRange(t *testing.T) {
	desktop, view := setupTextView(40, 50, Rect{X: 0, Y: 0, W: 20, H: 10})
	view.Wrap = false
	view.SetText(makeLines(50, "row"))
	desktop.Redraw()

	view.scrollBy(-10) // scrollY=30, follow=false
	if view.scrollY != 30 || view.follow {
		t.Fatalf("precondition: expected scrollY=30 follow=false, got %d follow=%v", view.scrollY, view.follow)
	}

	// Grow to H=40: maxScroll is now 10, so 30 is out of range and clamps to 10.
	resizeView(desktop, view, Rect{X: 0, Y: 0, W: 20, H: 40})
	if view.scrollY != 10 {
		t.Fatalf("non-following view should clamp to new maxScroll (10), got %d", view.scrollY)
	}
	if view.follow {
		t.Fatalf("clamping into range must not re-enable follow")
	}
}

// A WIDTH change re-wraps the content (more/fewer rows) — a following view must
// still anchor on the last line. Asserted against the live layout so the exact row
// count from wrapping need not be hand-computed.
func TestTextViewFollowReanchorsOnWidthChangeWhileWrapping(t *testing.T) {
	desktop, view := setupTextView(80, 40, Rect{X: 0, Y: 0, W: 40, H: 10})
	view.Wrap = true
	// Lines long enough to wrap once narrowed.
	view.SetText(makeLines(30, "the quick brown fox jumps over the lazy dog and then keeps on running"))
	desktop.Redraw()
	if want := maxScrollNow(view, 10); view.scrollY != want {
		t.Fatalf("precondition: following view should be at bottom %d, got %d", want, view.scrollY)
	}

	// Narrow the width: each logical line wraps into more rows, raising maxScroll.
	resizeView(desktop, view, Rect{X: 0, Y: 0, W: 18, H: 10})
	if want := maxScrollNow(view, 10); view.scrollY != want {
		t.Fatalf("after narrowing, a following view must re-anchor to the new bottom %d, got %d", want, view.scrollY)
	}

	// Widen again: fewer rows, lower maxScroll, still pinned to the bottom.
	resizeView(desktop, view, Rect{X: 0, Y: 0, W: 60, H: 10})
	if want := maxScrollNow(view, 10); view.scrollY != want {
		t.Fatalf("after widening, a following view must re-anchor to the new bottom %d, got %d", want, view.scrollY)
	}
}

// Direct unit tests of clampScroll, isolating the re-anchor/clamp/preserve rules
// from the draw path.
func TestClampScrollRules(t *testing.T) {
	cases := []struct {
		name    string
		follow  bool
		scrollY int
		total   int
		height  int
		want    int
	}{
		{"follow re-anchors up on shrink", true, 40, 50, 5, 45},
		{"follow re-anchors down on grow", true, 40, 50, 20, 30},
		{"follow to zero when content fits", true, 7, 20, 30, 0},
		{"follow ignores stale scrollY below bottom", true, 0, 50, 10, 40},
		{"non-follow preserves in-range scrollY", false, 20, 50, 5, 20},
		{"non-follow clamps out-of-range scrollY down", false, 30, 50, 40, 10},
		{"non-follow clamps negative scrollY to 0", false, -3, 50, 10, 0},
		{"non-follow preserves 0", false, 0, 50, 10, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: tc.height})
			view.follow = tc.follow
			view.scrollY = tc.scrollY
			view.clampScroll(tc.total, tc.height)
			if view.scrollY != tc.want {
				t.Fatalf("clampScroll(follow=%v, scrollY=%d, total=%d, h=%d) = %d, want %d",
					tc.follow, tc.scrollY, tc.total, tc.height, view.scrollY, tc.want)
			}
			// clampScroll must never flip the follow flag — toggle semantics live in
			// ScrollToBottom/ScrollToTop/scrollBy, not here.
			if view.follow != tc.follow {
				t.Fatalf("clampScroll must not change follow (was %v, now %v)", tc.follow, view.follow)
			}
		})
	}
}

// The fix must not change the follow-toggle semantics: ScrollToBottom enables
// follow, scrolling up (and ScrollToTop) clears it, and a resize never overrides
// an explicit ScrollToTop.
func TestTextViewResizeRespectsScrollToTop(t *testing.T) {
	desktop, view := setupTextView(40, 40, Rect{X: 0, Y: 0, W: 20, H: 10})
	view.Wrap = false
	view.SetText(makeLines(50, "row"))
	desktop.Redraw()

	view.ScrollToTop()
	if view.follow || view.scrollY != 0 {
		t.Fatalf("ScrollToTop should set follow=false scrollY=0, got follow=%v scrollY=%d", view.follow, view.scrollY)
	}

	// Resizing must keep a top-anchored view at the top, not snap it to the bottom.
	resizeView(desktop, view, Rect{X: 0, Y: 0, W: 20, H: 5})
	if view.scrollY != 0 {
		t.Fatalf("after ScrollToTop a resize must stay at the top, got scrollY=%d", view.scrollY)
	}
	if view.follow {
		t.Fatalf("a resize must not re-enable follow after ScrollToTop")
	}

	// ScrollToBottom re-enables following; the next draw pins to the live bottom.
	view.ScrollToBottom()
	desktop.Redraw()
	if want := maxScrollNow(view, 5); view.scrollY != want {
		t.Fatalf("ScrollToBottom should re-pin to the bottom (%d), got %d", want, view.scrollY)
	}
}
