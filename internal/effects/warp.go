package effects

import (
	"image"
	"math"
)

// pt is a 2D point used by the homography helpers.
type pt struct{ x, y float64 }

// homography is a 3x3 projective transform mapping (x,y) to
//
//	((h0*x + h1*y + h2) / (h6*x + h7*y + h8),
//	 (h3*x + h4*y + h5) / (h6*x + h7*y + h8))
type homography [9]float64

func (h homography) apply(x, y float64) (float64, float64) {
	d := h[6]*x + h[7]*y + h[8]
	if math.Abs(d) < 1e-12 {
		return math.Inf(1), math.Inf(1)
	}
	return (h[0]*x + h[1]*y + h[2]) / d, (h[3]*x + h[4]*y + h[5]) / d
}

// invert returns the inverse transform (adjugate / determinant).
func (h homography) invert() (homography, bool) {
	a, b, c := h[0], h[1], h[2]
	d, e, f := h[3], h[4], h[5]
	g, i, j := h[6], h[7], h[8]

	A := e*j - f*i
	B := -(d*j - f*g)
	C := d*i - e*g
	D := -(b*j - c*i)
	E := a*j - c*g
	F := -(a*i - b*g)
	G := b*f - c*e
	H := -(a*f - c*d)
	I := a*e - b*d

	det := a*A + b*B + c*C
	if math.Abs(det) < 1e-12 {
		return homography{}, false
	}
	return homography{
		A / det, D / det, G / det,
		B / det, E / det, H / det,
		C / det, F / det, I / det,
	}, true
}

// homographyFromQuads builds the transform that maps the four src points onto
// the four dst points (Gaussian elimination on the standard 8x8 system).
func homographyFromQuads(src, dst [4]pt) (homography, bool) {
	var m [8][9]float64
	for i := 0; i < 4; i++ {
		x, y := src[i].x, src[i].y
		u, v := dst[i].x, dst[i].y
		m[2*i] = [9]float64{x, y, 1, 0, 0, 0, -u * x, -u * y, u}
		m[2*i+1] = [9]float64{0, 0, 0, x, y, 1, -v * x, -v * y, v}
	}
	for col := 0; col < 8; col++ {
		p := col
		for r := col + 1; r < 8; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[p][col]) {
				p = r
			}
		}
		if math.Abs(m[p][col]) < 1e-12 {
			return homography{}, false
		}
		m[col], m[p] = m[p], m[col]
		piv := m[col][col]
		for k := col; k < 9; k++ {
			m[col][k] /= piv
		}
		for r := 0; r < 8; r++ {
			if r == col || m[r][col] == 0 {
				continue
			}
			f := m[r][col]
			for k := col; k < 9; k++ {
				m[r][k] -= f * m[col][k]
			}
		}
	}
	var h homography
	for i := 0; i < 8; i++ {
		h[i] = m[i][8]
	}
	h[8] = 1
	return h, true
}

func rotatePoint(p pt, cx, cy, cos, sin float64) pt {
	dx, dy := p.x-cx, p.y-cy
	return pt{cx + dx*cos - dy*sin, cy + dx*sin + dy*cos}
}

// warpOptions configures warpPage.
type warpOptions struct {
	// OffsetX/OffsetY are added to the output pixel coordinates before the
	// homography is applied. Used to centre-crop an expanded canvas.
	OffsetX, OffsetY float64
	OutW, OutH       int
	Desk             rgb
	// ShadowPx is the falloff distance of the drop shadow cast on the desk.
	ShadowPx  float64
	ShadowAmt float64
	// DeskGrain adds a mottled texture to the desk, in 0..255 levels.
	DeskGrain float64
	DeskSeed  uint32
}

// warpPage resamples src through h into an OutW×OutH image. Pixels that map
// outside src are filled with the desk colour, shaded near the page edge.
func warpPage(src *image.NRGBA, h homography, opt warpOptions) *image.NRGBA {
	w, hgt := src.Rect.Dx(), src.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, opt.OutW, opt.OutH))
	op := out.Pix
	stride := out.Stride
	sp := src.Pix
	sstride := src.Stride
	maxXf, maxYf := float64(w-1), float64(hgt-1)
	invShadow := 0.0
	if opt.ShadowPx > 0 {
		invShadow = 1 / opt.ShadowPx
	}
	deskR := float64(opt.Desk.r)
	deskG := float64(opt.Desk.g)
	deskB := float64(opt.Desk.b)

	for y := 0; y < opt.OutH; y++ {
		cy := float64(y) + opt.OffsetY + 0.5
		row := y * stride
		for x := 0; x < opt.OutW; x++ {
			sx, sy := h.apply(float64(x)+opt.OffsetX+0.5, cy)
			sx -= 0.5
			sy -= 0.5
			i := row + x*4
			if sx < 0 || sy < 0 || sx > maxXf || sy > maxYf {
				// Desk with a shadow that fades away from the page.
				dx := math.Max(0, math.Max(-sx, sx-maxXf))
				dy := math.Max(0, math.Max(-sy, sy-maxYf))
				dist := math.Hypot(dx, dy)
				sh := opt.ShadowAmt * math.Exp(-dist*invShadow)
				if !isFinite(sx) || !isFinite(sy) {
					sh = opt.ShadowAmt
				}
				f := 1 - 0.75*sh
				g := 0.0
				if opt.DeskGrain > 0 {
					g = (hashNoise(x, y, opt.DeskSeed) - 0.5) * opt.DeskGrain
					g += (hashNoise(x>>4, y>>4, opt.DeskSeed+7) - 0.5) * opt.DeskGrain * 2
				}
				op[i] = clampU8(deskR*f + g)
				op[i+1] = clampU8(deskG*f + g*0.95)
				op[i+2] = clampU8(deskB*f + g*0.85)
				op[i+3] = 0xFF
				continue
			}
			x0 := int(sx)
			y0 := int(sy)
			fx := sx - float64(x0)
			fy := sy - float64(y0)
			x1 := x0 + 1
			if x1 >= w {
				x1 = w - 1
			}
			y1 := y0 + 1
			if y1 >= hgt {
				y1 = hgt - 1
			}
			i00 := y0*sstride + x0*4
			i10 := y0*sstride + x1*4
			i01 := y1*sstride + x0*4
			i11 := y1*sstride + x1*4
			w00 := (1 - fx) * (1 - fy)
			w10 := fx * (1 - fy)
			w01 := (1 - fx) * fy
			w11 := fx * fy
			op[i] = clampU8(float64(sp[i00])*w00 + float64(sp[i10])*w10 + float64(sp[i01])*w01 + float64(sp[i11])*w11 + 0.5)
			op[i+1] = clampU8(float64(sp[i00+1])*w00 + float64(sp[i10+1])*w10 + float64(sp[i01+1])*w01 + float64(sp[i11+1])*w11 + 0.5)
			op[i+2] = clampU8(float64(sp[i00+2])*w00 + float64(sp[i10+2])*w10 + float64(sp[i01+2])*w01 + float64(sp[i11+2])*w11 + 0.5)
			op[i+3] = 0xFF
		}
	}
	return out
}

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
