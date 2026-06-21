package tv_test

import (
	"io"
	"testing"

	tui "github.com/hobbestherat/turbotui"
	tv "github.com/hobbestherat/turbotui/turbotv"
)

// TestSelectShadowFieldIsExportedAndSettable guards the public-field contract at
// the heart of gogent #231 from the OUTSIDE — the vantage point gogent itself
// occupies. gogent is an external package and must be able to write
// `sel.Shadow = shadowsEnabled`, exactly mirroring applyWindowShadow /
// applyButtonShadow / applyMenuBarShadow. The internal `package tv` tests cannot
// detect an accidentally-unexported field (they see lower-case fields too), so
// this test lives in `package tv_test`: it compiles only while Shadow stays an
// exported, settable bool.
func TestSelectShadowFieldIsExportedAndSettable(t *testing.T) {
	app := tui.NewWithSize(40, 12, io.Discard)
	desktop := tv.NewDesktop(app)
	sel := tv.NewSelect(desktop, []string{"a", "b"}, tv.Rect{X: 2, Y: 2, W: 10, H: 1})

	// The constructor default must match Window/Button/MenuBar: shadow on.
	if !sel.Shadow {
		t.Fatalf("NewSelect must default Shadow to true so existing UIs keep their shadow")
	}
	// gogent's exact usage: assign a preference bool to the field.
	sel.Shadow = false
	if sel.Shadow {
		t.Fatalf("Shadow must be settable to false (no-shadow preference)")
	}
	sel.Shadow = true
	if !sel.Shadow {
		t.Fatalf("Shadow must be settable back to true")
	}
}
