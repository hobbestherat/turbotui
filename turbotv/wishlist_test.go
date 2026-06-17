package tv

import (
	tui "github.com/hobbestherat/turbotui"
	"testing"
)

func TestCheckboxToggle(t *testing.T) {
	var got bool
	calls := 0
	cb := NewCheckbox("&Enable", Rect{X: 0, Y: 0, W: 20, H: 1}, func(v bool) {
		got = v
		calls++
	})
	if cb.IsChecked() {
		t.Fatal("checkbox should start unchecked")
	}
	cb.Component.OnTypeFn(cb.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: ' '})
	if !cb.IsChecked() || !got {
		t.Fatal("space should check the box and fire OnToggle")
	}
	// A mouse release inside toggles back off.
	cb.Component.OnClickFn(cb.Component, tui.ClickEvent{X: 2, Y: 0, Down: false})
	if cb.IsChecked() {
		t.Fatal("click should uncheck the box")
	}
	if calls != 2 {
		t.Fatalf("expected OnToggle to fire twice, got %d", calls)
	}
}
func TestWindowMinimizeRestore(t *testing.T) {
	w := NewWindow("T", Rect{X: 2, Y: 3, W: 40, H: 20}, tui.LineSingle)
	w.Minimizable = true
	w.Minimize()
	if !w.IsMinimized() {
		t.Fatal("window should report minimized")
	}
	if w.Component.Bounds.H != 1 {
		t.Fatalf("minimized height should be 1, got %d", w.Component.Bounds.H)
	}
	if w.Content.Visible {
		t.Fatal("content should be hidden when minimized")
	}
	w.Restore()
	if w.IsMinimized() {
		t.Fatal("window should report restored")
	}
	if w.Component.Bounds.H != 20 {
		t.Fatalf("restored height should be 20, got %d", w.Component.Bounds.H)
	}
	if !w.Content.Visible {
		t.Fatal("content should be visible after restore")
	}
}
func TestWindowResizeGripClampsToMinimum(t *testing.T) {
	w := NewWindow("T", Rect{X: 0, Y: 0, W: 40, H: 20}, tui.LineSingle)
	w.Resizable = true
	abs := w.Component.AbsoluteBounds()
	// Press on the bottom-right grip, then drag far past the minimum.
	w.handleClick(w.Component, tui.ClickEvent{X: abs.Right(), Y: abs.Bottom(), Down: true})
	w.handleClick(w.Component, tui.ClickEvent{X: 1, Y: 1, Down: true})
	if w.Component.Bounds.W != w.MinWidth || w.Component.Bounds.H != w.MinHeight {
		t.Fatalf("resize should clamp to %dx%d, got %dx%d",
			w.MinWidth, w.MinHeight, w.Component.Bounds.W, w.Component.Bounds.H)
	}
	w.handleClick(w.Component, tui.ClickEvent{X: 1, Y: 1, Down: false})
}
func TestTreeExpandCollapseAndSelect(t *testing.T) {
	tr := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
	root := tr.AddRoot(NewTreeNode("root"))
	root.AddLeaf("a")
	root.AddLeaf("b")
	if rows := tr.flatten(); len(rows) != 1 {
		t.Fatalf("collapsed tree should flatten to 1 row, got %d", len(rows))
	}
	tr.Component.OnTypeFn(tr.Component, tui.TypeEvent{Key: tui.KeyRight})
	if !root.Expanded {
		t.Fatal("Right should expand the selected parent")
	}
	if rows := tr.flatten(); len(rows) != 3 {
		t.Fatalf("expanded tree should flatten to 3 rows, got %d", len(rows))
	}
	tr.Component.OnTypeFn(tr.Component, tui.TypeEvent{Key: tui.KeyDown})
	if sel := tr.Selected(); sel == nil || sel.Label != "a" {
		t.Fatalf("Down should select child 'a', got %v", tr.Selected())
	}
	// Left on a leaf jumps to its parent.
	tr.Component.OnTypeFn(tr.Component, tui.TypeEvent{Key: tui.KeyLeft})
	if sel := tr.Selected(); sel == nil || sel.Label != "root" {
		t.Fatalf("Left on a leaf should select parent 'root', got %v", tr.Selected())
	}
}
