package tv

type Rect struct {
	X int
	Y int
	W int
	H int
}

func (r Rect) Empty() bool {
	if r.W <= 0 || r.H <= 0 {
		return true
	}
	return false
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
