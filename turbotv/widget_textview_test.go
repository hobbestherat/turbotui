package tv

import "testing"

func TestTextViewAllTextIncludesChildren(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 40, H: 10})
	parent := view.AddLine("[Agent] summary")
	parent.Add("detail one")
	parent.Add("detail two")
	parent.SetCollapsed(true)
	view.AddLine("plain line")
	got := view.AllText()
	want := "[Agent] summary\ndetail one\ndetail two\nplain line"
	if got != want {
		t.Fatalf("AllText mismatch:\n got %q\nwant %q", got, want)
	}
}
