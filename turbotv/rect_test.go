package tv

import "testing"

func TestRectSize(t *testing.T) {
	r := Rect{X: 3, Y: 4, W: 10, H: 7}
	w, h := r.Size()
	if w != 10 || h != 7 {
		t.Fatalf("Size() = %d,%d; want 10,7", w, h)
	}
}

func TestRectInset(t *testing.T) {
	cases := []struct {
		name string
		in   Rect
		n    int
		want Rect
	}{
		{"positive", Rect{X: 0, Y: 0, W: 20, H: 10}, 1, Rect{X: 1, Y: 1, W: 18, H: 8}},
		{"zero", Rect{X: 5, Y: 5, W: 12, H: 12}, 0, Rect{X: 5, Y: 5, W: 12, H: 12}},
		{"negative-grows", Rect{X: 1, Y: 1, W: 8, H: 8}, -1, Rect{X: 0, Y: 0, W: 10, H: 10}},
		{"clamp-width", Rect{X: 0, Y: 0, W: 3, H: 10}, 2, Rect{X: 2, Y: 2, W: 0, H: 6}},
		{"clamp-both", Rect{X: 0, Y: 0, W: 2, H: 2}, 3, Rect{X: 3, Y: 3, W: 0, H: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Inset(tc.n)
			if got != tc.want {
				t.Fatalf("Inset(%d) = %+v; want %+v", tc.n, got, tc.want)
			}
		})
	}
}

func TestRectMove(t *testing.T) {
	r := Rect{X: 1, Y: 2, W: 5, H: 6}
	got := r.Move(3, -1)
	want := Rect{X: 4, Y: 1, W: 5, H: 6}
	if got != want {
		t.Fatalf("Move = %+v; want %+v", got, want)
	}
	// Original is untouched.
	if r.X != 1 || r.Y != 2 {
		t.Fatalf("Move mutated the receiver: %+v", r)
	}
}

func TestRectUnion(t *testing.T) {
	cases := []struct {
		name string
		a, b Rect
		want Rect
	}{
		{"disjoint", Rect{X: 0, Y: 0, W: 2, H: 2}, Rect{X: 5, Y: 5, W: 2, H: 2}, Rect{X: 0, Y: 0, W: 7, H: 7}},
		{"overlapping", Rect{X: 0, Y: 0, W: 4, H: 4}, Rect{X: 2, Y: 2, W: 4, H: 4}, Rect{X: 0, Y: 0, W: 6, H: 6}},
		{"contained", Rect{X: 0, Y: 0, W: 10, H: 10}, Rect{X: 2, Y: 2, W: 2, H: 2}, Rect{X: 0, Y: 0, W: 10, H: 10}},
		{"left-empty", Rect{}, Rect{X: 1, Y: 1, W: 3, H: 3}, Rect{X: 1, Y: 1, W: 3, H: 3}},
		{"right-empty", Rect{X: 1, Y: 1, W: 3, H: 3}, Rect{}, Rect{X: 1, Y: 1, W: 3, H: 3}},
		{"negative-origin", Rect{X: -3, Y: 0, W: 2, H: 2}, Rect{X: 0, Y: 0, W: 2, H: 2}, Rect{X: -3, Y: 0, W: 5, H: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.a.Union(tc.b)
			if got != tc.want {
				t.Fatalf("Union = %+v; want %+v", got, tc.want)
			}
		})
	}
}
