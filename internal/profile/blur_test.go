package profile

import "testing"

func TestBoxBlurInline(t *testing.T) {
	g := newGray(9, 5)
	for i := range g.p {
		g.p[i] = float32(i % 7)
	}
	for _, r := range []int{1, 2, 4} {
		got := boxBlur(g, r)
		// Compare against a brute force implementation.
		for y := 0; y < g.h; y++ {
			for x := 0; x < g.w; x++ {
				var sum float32
				n := 0
				for dy := -r; dy <= r; dy++ {
					for dx := -r; dx <= r; dx++ {
						sum += g.at(x+dx, y+dy)
						n++
					}
				}
				want := sum / float32(n)
				if d := got.p[y*g.w+x] - want; d > 1e-4 || d < -1e-4 {
					t.Fatalf("r=%d at (%d,%d): got %f want %f", r, x, y, got.p[y*g.w+x], want)
				}
			}
		}
	}
}
