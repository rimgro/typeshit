package profile

import (
	"errors"
	"image"
	"math"

	"golang.org/x/image/math/f64"
)

// homography maps homogeneous 2D points with a 3×3 matrix.
type homography [3][3]float64

var identity = homography{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

var errDegenerate = errors.New("profile: degenerate homography")

// apply maps (x,y) through h.
func (h homography) apply(x, y float64) (float64, float64) {
	w := h[2][0]*x + h[2][1]*y + h[2][2]
	if w == 0 {
		w = 1e-12
	}
	return (h[0][0]*x + h[0][1]*y + h[0][2]) / w,
		(h[1][0]*x + h[1][1]*y + h[1][2]) / w
}

// mul returns h∘g (apply g first, then h).
func (h homography) mul(g homography) homography {
	var out homography
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = h[i][0]*g[0][j] + h[i][1]*g[1][j] + h[i][2]*g[2][j]
		}
	}
	return out
}

func (h homography) det() float64 {
	return h[0][0]*(h[1][1]*h[2][2]-h[1][2]*h[2][1]) -
		h[0][1]*(h[1][0]*h[2][2]-h[1][2]*h[2][0]) +
		h[0][2]*(h[1][0]*h[2][1]-h[1][1]*h[2][0])
}

// inv returns the inverse homography.
func (h homography) inv() (homography, error) {
	d := h.det()
	if math.Abs(d) < 1e-12 {
		return identity, errDegenerate
	}
	c := [3][3]float64{
		{h[1][1]*h[2][2] - h[1][2]*h[2][1], h[0][2]*h[2][1] - h[0][1]*h[2][2], h[0][1]*h[1][2] - h[0][2]*h[1][1]},
		{h[1][2]*h[2][0] - h[1][0]*h[2][2], h[0][0]*h[2][2] - h[0][2]*h[2][0], h[0][2]*h[1][0] - h[0][0]*h[1][2]},
		{h[1][0]*h[2][1] - h[1][1]*h[2][0], h[0][1]*h[2][0] - h[0][0]*h[2][1], h[0][0]*h[1][1] - h[0][1]*h[1][0]},
	}
	var out homography
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = c[i][j] / d
		}
	}
	return out, nil
}

// affine builds a transform from scale/rotation/shear/translation. Rotation is
// applied in image space (y down), so a positive slant leans the top right.
func affine(sx, sy, rotDeg, slantDeg, tx, ty float64) homography {
	r := rotDeg * math.Pi / 180
	c, s := math.Cos(r), math.Sin(r)
	k := math.Tan(slantDeg * math.Pi / 180)
	// shear: x' = x + k*(-y); compose shear after scale, before rotation
	m := homography{{sx, -k * sy, 0}, {0, sy, 0}, {0, 0, 1}}
	rot := homography{{c, -s, 0}, {s, c, 0}, {0, 0, 1}}
	tr := homography{{1, 0, tx}, {0, 1, ty}, {0, 0, 1}}
	return tr.mul(rot).mul(m)
}

// F64 returns the 3×2 affine part usable by golang.org/x/image/draw.
func (h homography) F64() f64.Aff3 {
	inv, err := h.inv()
	if err != nil {
		return f64.Aff3{1, 0, 0, 0, 1, 0}
	}
	return f64.Aff3{inv[0][0], inv[0][1], inv[0][2], inv[1][0], inv[1][1], inv[1][2]}
}

// quadToQuad returns the homography that maps the quad src (ordered TL, TR,
// BR, BL) onto the quad dst. It is the standard "unit square as an
// intermediate" construction.
func quadToQuad(src, dst [4][2]float64) (homography, error) {
	hs, err := squareToQuad(src)
	if err != nil {
		return identity, err
	}
	hd, err := squareToQuad(dst)
	if err != nil {
		return identity, err
	}
	hi, err := hs.inv()
	if err != nil {
		return identity, err
	}
	return hd.mul(hi), nil
}

// squareToQuad maps the unit square (0,0),(1,0),(1,1),(0,1) onto q.
func squareToQuad(q [4][2]float64) (homography, error) {
	dx1 := q[1][0] - q[2][0]
	dx2 := q[3][0] - q[2][0]
	dx3 := q[0][0] - q[1][0] + q[2][0] - q[3][0]
	dy1 := q[1][1] - q[2][1]
	dy2 := q[3][1] - q[2][1]
	dy3 := q[0][1] - q[1][1] + q[2][1] - q[3][1]
	det := dx1*dy2 - dx2*dy1
	if math.Abs(det) < 1e-12 {
		return identity, errDegenerate
	}
	g := (dx3*dy2 - dx2*dy3) / det
	hh := (dx1*dy3 - dx3*dy1) / det
	return homography{
		{q[1][0] - q[0][0] + g*q[1][0], q[3][0] - q[0][0] + hh*q[3][0], q[0][0]},
		{q[1][1] - q[0][1] + g*q[1][1], q[3][1] - q[0][1] + hh*q[3][1], q[0][1]},
		{g, hh, 1},
	}, nil
}

// warpRGBA resamples src into dst using h, which maps dst coordinates to src
// coordinates. Out-of-bounds samples become transparent.
func warpRGBA(src *image.NRGBA, dst *image.NRGBA, h homography) {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	db := dst.Rect
	sp := src.Pix
	dp := dst.Pix
	for y := db.Min.Y; y < db.Max.Y; y++ {
		row := dp[(y-db.Min.Y)*dst.Stride:]
		for x := db.Min.X; x < db.Max.X; x++ {
			fx, fy := h.apply(float64(x)+0.5, float64(y)+0.5)
			fx -= 0.5
			fy -= 0.5
			if fx < -1 || fy < -1 || fx > float64(sw) || fy > float64(sh) {
				continue
			}
			x0 := int(math.Floor(fx))
			y0 := int(math.Floor(fy))
			ax := fx - float64(x0)
			ay := fy - float64(y0)
			var acc [4]float64
			for j := 0; j < 2; j++ {
				yy := y0 + j
				wy := 1 - ay
				if j == 1 {
					wy = ay
				}
				if yy < 0 {
					yy = 0
				}
				if yy >= sh {
					yy = sh - 1
				}
				for i := 0; i < 2; i++ {
					xx := x0 + i
					wx := 1 - ax
					if i == 1 {
						wx = ax
					}
					if xx < 0 {
						xx = 0
					}
					if xx >= sw {
						xx = sw - 1
					}
					w := wx * wy
					o := yy*src.Stride + xx*4
					acc[0] += float64(sp[o+0]) * w
					acc[1] += float64(sp[o+1]) * w
					acc[2] += float64(sp[o+2]) * w
					acc[3] += float64(sp[o+3]) * w
				}
			}
			o := (x-db.Min.X)*4 + 0
			row[o+0] = uint8(acc[0] + 0.5)
			row[o+1] = uint8(acc[1] + 0.5)
			row[o+2] = uint8(acc[2] + 0.5)
			row[o+3] = uint8(acc[3] + 0.5)
		}
	}
}

// dist reports the euclidean distance between two points.
func dist(ax, ay, bx, by float64) float64 { return math.Hypot(ax-bx, ay-by) }
