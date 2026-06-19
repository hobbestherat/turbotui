package tv

type Rect struct {
	X int
	Y int
	W int
	H int
}

func (r Rect) Empty() bool {
	return r.W <= 0 || r.H <= 0
}

func (r Rect) Contains(x int, y int) bool {
	if x < r.X || y < r.Y {
		return false
	}
	if x >= r.X+r.W || y >= r.Y+r.H {
		return false
	}
	return true
}

func (r Rect) Right() int {
	return r.X + r.W - 1
}

func (r Rect) Bottom() int {
	return r.Y + r.H - 1
}

func (r Rect) Center() (int, int) {
	return r.X + r.W/2, r.Y + r.H/2
}

// Size returns the (width, height) of the rect.
func (r Rect) Size() (int, int) {
	return r.W, r.H
}

// Inset returns a copy shrunk by n on every side (and grown when n is negative).
// Width/height clamp at zero so the result is never negative.
func (r Rect) Inset(n int) Rect {
	r.X += n
	r.Y += n
	r.W -= n * 2
	r.H -= n * 2
	if r.W < 0 {
		r.W = 0
	}
	if r.H < 0 {
		r.H = 0
	}
	return r
}

// Move returns a copy translated by (dx, dy), leaving the size unchanged.
func (r Rect) Move(dx int, dy int) Rect {
	r.X += dx
	r.Y += dy
	return r
}

// Union returns the smallest rect containing both r and other. If either rect is
// empty, the other is returned unchanged.
func (r Rect) Union(other Rect) Rect {
	if r.Empty() {
		return other
	}
	if other.Empty() {
		return r
	}
	left := r.X
	if other.X < left {
		left = other.X
	}
	top := r.Y
	if other.Y < top {
		top = other.Y
	}
	right := r.X + r.W
	if other.X+other.W > right {
		right = other.X + other.W
	}
	bottom := r.Y + r.H
	if other.Y+other.H > bottom {
		bottom = other.Y + other.H
	}
	return Rect{
		X: left,
		Y: top,
		W: right - left,
		H: bottom - top,
	}
}

func (r Rect) Intersect(other Rect) Rect {
	left := r.X
	if other.X > left {
		left = other.X
	}
	top := r.Y
	if other.Y > top {
		top = other.Y
	}
	right := r.X + r.W
	if other.X+other.W < right {
		right = other.X + other.W
	}
	bottom := r.Y + r.H
	if other.Y+other.H < bottom {
		bottom = other.Y + other.H
	}
	result := Rect{
		X: left,
		Y: top,
		W: right - left,
		H: bottom - top,
	}
	if result.W < 0 {
		result.W = 0
	}
	if result.H < 0 {
		result.H = 0
	}
	return result
}
