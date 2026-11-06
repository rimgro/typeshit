package ink

import (
	"image"
	"math"

	"typeshit/internal/model"
)

// blendPixel composites the ink colour over dst at coverage a.
func (c *Canvas) blendPixel(dst *image.NRGBA, x, y int, a float64) {
	if a <= 0.001 {
		return
	}
	b := dst.Bounds()
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return
	}
	if a > 1 {
		a = 1
	}
	di := (y-b.Min.Y)*dst.Stride + (x-b.Min.X)*4
	inv := 1 - a
	dst.Pix[di] = uint8(float64(dst.Pix[di])*inv + c.color[0]*255*a)
	dst.Pix[di+1] = uint8(float64(dst.Pix[di+1])*inv + c.color[1]*255*a)
	dst.Pix[di+2] = uint8(float64(dst.Pix[di+2])*inv + c.color[2]*255*a)
}

// disc stamps an antialiased round dot, optionally with a ragged edge.
func (c *Canvas) disc(dst *image.NRGBA, cx, cy, r, alpha float64, seed uint32) {
	if r <= 0 || alpha <= 0 {
		return
	}
	b := dst.Bounds()
	x0 := maxInt(int(math.Floor(cx-r-1)), b.Min.X)
	x1 := minInt(int(math.Ceil(cx+r+1)), b.Max.X)
	y0 := maxInt(int(math.Floor(cy-r-1)), b.Min.Y)
	y1 := minInt(int(math.Ceil(cy+r+1)), b.Max.Y)
	for y := y0; y < y1; y++ {
		dy := float64(y) + 0.5 - cy
		for x := x0; x < x1; x++ {
			dx := float64(x) + 0.5 - cx
			d := math.Sqrt(dx*dx + dy*dy)
			cov := (r - d) + 0.5
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			n := float64(hash2(x, y, seed)&255) / 255
			a := cov * alpha * (0.88 + 0.12*n)
			c.blendPixel(dst, x, y, a)
		}
	}
}

// line draws a slightly bent, trembling stroke with a pressure profile.
func (c *Canvas) line(dst *image.NRGBA, x0, y0, x1, y1, width, ink float64, seed uint32, taper bool) {
	dx := x1 - x0
	dy := y1 - y0
	length := math.Hypot(dx, dy)
	if length < 0.5 || width <= 0 {
		c.disc(dst, (x0+x1)/2, (y0+y1)/2, math.Max(width/2, 0.4), ink, seed)
		return
	}
	steps := int(length/1.2) + 2
	if steps > 4000 {
		steps = 4000
	}
	nx := -dy / length
	ny := dx / length
	// A hand never draws a straight line: bow it a little.
	bow := (float64(hash2(1, 2, seed)&255)/255 - 0.5) * math.Min(length*0.02, 3)
	tremorAmp := math.Min(0.55, 6/length)
	base := c.base * c.density
	var px, py float64
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		off := bow * math.Sin(math.Pi*t)
		trem := tremorAmp * math.Sin(t*float64(steps)*0.7+float64(seed&15))
		cx := x0 + dx*t + nx*(off+trem)
		cy := y0 + dy*t + ny*(off+trem)
		w := width
		if taper {
			w = width * (0.55 + 0.45*math.Sin(math.Pi*t))
		} else {
			w = width * (0.8 + 0.2*math.Sin(math.Pi*t))
		}
		// Ink pooling when the pen lands and a dry tail when it lifts.
		if t < 0.04 {
			w *= 1.25
		}
		a := ink
		if t > 0.9 {
			a *= 1 - (t-0.9)*2.5
		}
		c.disc(dst, cx, cy, math.Max(w/2, 0.35), a*base, seed+uint32(i))
		px, py = cx, cy
	}
	_ = px
	_ = py
}

// drawLink renders a pseudo-cursive connector.
func (c *Canvas) drawLink(dst *image.NRGBA, l *model.LinkOp) {
	if l == nil {
		return
	}
	ink := clamp(l.Ink, 0.1, 1)
	w := l.Width
	if w <= 0 {
		w = 1
	}
	length := math.Hypot(l.X1-l.X0, l.Y1-l.Y0) + math.Hypot(l.Ctrl.X-l.X0, l.Ctrl.Y-l.Y0)
	steps := int(length/1.3) + 3
	if steps > 400 {
		steps = 400
	}
	seed := uint32(l.X0*11) ^ uint32(l.Y0*7) ^ 0x51ED
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		mt := 1 - t
		x := mt*mt*l.X0 + 2*mt*t*l.Ctrl.X + t*t*l.X1
		y := mt*mt*l.Y0 + 2*mt*t*l.Ctrl.Y + t*t*l.Y1
		// Thin at both ends, a hair thicker in the middle.
		ww := w * (0.65 + 0.35*math.Sin(math.Pi*t))
		c.disc(dst, x, y, math.Max(ww/2, 0.3), ink*c.base*c.density*0.92, seed+uint32(i))
	}
}

// Stroke draws a single pen stroke: underline, strikethrough, caret, dot.
func (c *Canvas) Stroke(dst *image.NRGBA, m model.LineMark) {
	if dst == nil {
		return
	}
	ink := clamp(m.Ink, 0.05, 1)
	if ink <= 0 {
		ink = 0.9
	}
	w := m.Width
	if w <= 0 {
		w = 1.2
	}
	seed := uint32(int(m.X0)*31) ^ uint32(int(m.Y0)*17) ^ 0x9E37
	switch m.Kind {
	case "caret":
		hw := m.X1
		h := m.Y1
		if hw <= 0 {
			hw = 4
		}
		if h <= 0 {
			h = hw
		}
		c.line(dst, m.X0-hw, m.Y0+h, m.X0, m.Y0, w, ink*0.95, seed, true)
		c.line(dst, m.X0, m.Y0, m.X0+hw, m.Y0+h, w, ink*0.95, seed+5, true)
	case "dot":
		c.disc(dst, m.X0, m.Y0, math.Max(w*0.7, 0.6), ink*0.9*c.base, seed)
	case "scribble":
		c.line(dst, m.X0, m.Y0, m.X1, m.Y1, w, ink*0.95, seed, false)
	default:
		c.line(dst, m.X0, m.Y0, m.X1, m.Y1, w, ink, seed, false)
	}
}

// DrawFlaws renders page-level accidents: smudges, blots, stains, creases.
func (c *Canvas) DrawFlaws(dst *image.NRGBA, flaws []model.Flaw) {
	if dst == nil {
		return
	}
	for _, f := range flaws {
		switch f.Kind {
		case "smudge":
			c.smudge(dst, f)
		case "blot":
			c.blot(dst, f)
		case "stain":
			c.stain(dst, f)
		case "crease":
			c.crease(dst, f)
		}
	}
}

func (c *Canvas) smudge(dst *image.NRGBA, f model.Flaw) {
	r := f.R
	if r <= 0 {
		r = 8
	}
	amt := clamp(f.Amount, 0, 1)
	rot := f.Rotate * math.Pi / 180
	cos, sin := math.Cos(rot), math.Sin(rot)
	ry := r * 0.55
	seed := uint32(f.X*13) ^ uint32(f.Y*29) ^ 0x5A17
	b := dst.Bounds()
	x0 := maxInt(int(f.X-r*1.6), b.Min.X)
	x1 := minInt(int(f.X+r*1.6), b.Max.X)
	y0 := maxInt(int(f.Y-r*1.2), b.Min.Y)
	y1 := minInt(int(f.Y+r*1.2), b.Max.Y)
	for y := y0; y < y1; y++ {
		dy := float64(y) + 0.5 - f.Y
		drow := dst.Pix[(y-b.Min.Y)*dst.Stride:]
		for x := x0; x < x1; x++ {
			dx := float64(x) + 0.5 - f.X
			// Rotate into the smudge's own frame.
			u := dx*cos + dy*sin
			v := -dx*sin + dy*cos
			d := (u*u)/(r*r) + (v*v)/(ry*ry)
			if d >= 1 {
				continue
			}
			n := float64(hash2(x>>1, y>>1, seed)&255) / 255
			fall := (1 - d) * (0.55 + 0.45*n)
			factor := 1 - amt*0.24*fall
			di := (x - b.Min.X) * 4
			// Smudged graphite/ink greys the paper and adds a faint tint.
			drow[di] = uint8(float64(drow[di])*factor + 16*amt*fall)
			drow[di+1] = uint8(float64(drow[di+1])*factor + 15*amt*fall)
			drow[di+2] = uint8(float64(drow[di+2])*factor + 14*amt*fall)
		}
	}
}

func (c *Canvas) blot(dst *image.NRGBA, f model.Flaw) {
	r := f.R
	if r <= 0 {
		r = 3
	}
	amt := clamp(f.Amount, 0, 1)
	seed := uint32(f.X*7) ^ uint32(f.Y*31) ^ 0xB107
	b := dst.Bounds()
	x0 := maxInt(int(f.X-r*1.5-1), b.Min.X)
	x1 := minInt(int(f.X+r*1.5+1), b.Max.X)
	y0 := maxInt(int(f.Y-r*1.5-1), b.Min.Y)
	y1 := minInt(int(f.Y+r*1.5+1), b.Max.Y)
	for y := y0; y < y1; y++ {
		dy := float64(y) + 0.5 - f.Y
		for x := x0; x < x1; x++ {
			dx := float64(x) + 0.5 - f.X
			d := math.Hypot(dx, dy)
			ang := math.Atan2(dy, dx)
			// Ragged outline: coffee-dark ink soaks into the fibres.
			jag := 0.75 + 0.25*float64(hash2(int(ang*12), 3, seed)&255)/255
			rr := r * jag
			if d > rr+1 {
				continue
			}
			cov := (rr - d) + 0.5
			if cov > 1 {
				cov = 1
			}
			if cov <= 0 {
				continue
			}
			n := float64(hash2(x, y, seed)&255) / 255
			a := cov * amt * (0.85 + 0.15*n) * c.base * 1.1
			c.blendPixel(dst, x, y, a)
		}
	}
}

func (c *Canvas) stain(dst *image.NRGBA, f model.Flaw) {
	r := f.R
	if r <= 0 {
		r = 40
	}
	amt := clamp(f.Amount, 0, 1)
	seed := uint32(f.X*3) ^ uint32(f.Y*11) ^ 0x5781
	col := [3]float64{0.62, 0.48, 0.28}
	b := dst.Bounds()
	x0 := maxInt(int(f.X-r-2), b.Min.X)
	x1 := minInt(int(f.X+r+2), b.Max.X)
	y0 := maxInt(int(f.Y-r-2), b.Min.Y)
	y1 := minInt(int(f.Y+r+2), b.Max.Y)
	ringW := r * 0.09
	if ringW < 1.5 {
		ringW = 1.5
	}
	for y := y0; y < y1; y++ {
		dy := float64(y) + 0.5 - f.Y
		drow := dst.Pix[(y-b.Min.Y)*dst.Stride:]
		for x := x0; x < x1; x++ {
			dx := float64(x) + 0.5 - f.X
			d := math.Hypot(dx, dy)
			if d > r+ringW*3 {
				continue
			}
			// Irregular ring plus a faint interior tint.
			ring := math.Exp(-math.Pow(d-r, 2) / (2 * ringW * ringW))
			inside := 0.0
			if d < r {
				inside = 0.22 * (1 - d/r)
			}
			n := float64(hash2(x>>2, y>>2, seed)&255) / 255
			a := (ring*0.5 + inside) * amt * (0.7 + 0.3*n)
			if a <= 0.002 {
				continue
			}
			di := (x - b.Min.X) * 4
			a = math.Min(a, 0.5)
			inv := 1 - a
			drow[di] = uint8(float64(drow[di])*inv + col[0]*255*a)
			drow[di+1] = uint8(float64(drow[di+1])*inv + col[1]*255*a)
			drow[di+2] = uint8(float64(drow[di+2])*inv + col[2]*255*a)
		}
	}
}

func (c *Canvas) crease(dst *image.NRGBA, f model.Flaw) {
	length := f.R
	if length <= 0 {
		length = float64(dst.Bounds().Dx()) / 2
	}
	amt := clamp(f.Amount, 0, 1)
	rot := f.Rotate * math.Pi / 180
	ux, uy := math.Cos(rot), math.Sin(rot)
	nx, ny := -uy, ux
	seed := uint32(f.X*17) ^ uint32(f.Y*5) ^ 0x3C1E
	b := dst.Bounds()
	reach := 26.0
	x0 := maxInt(int(f.X-length*math.Abs(ux)-reach), b.Min.X)
	x1 := minInt(int(f.X+length*math.Abs(ux)+reach), b.Max.X)
	y0 := maxInt(int(f.Y-length*math.Abs(uy)-reach), b.Min.Y)
	y1 := minInt(int(f.Y+length*math.Abs(uy)+reach), b.Max.Y)
	for y := y0; y < y1; y++ {
		dy := float64(y) + 0.5 - f.Y
		drow := dst.Pix[(y-b.Min.Y)*dst.Stride:]
		for x := x0; x < x1; x++ {
			dx := float64(x) + 0.5 - f.X
			along := dx*ux + dy*uy
			if math.Abs(along) > length {
				continue
			}
			perp := dx*nx + dy*ny
			dist := math.Abs(perp)
			if dist > reach {
				continue
			}
			n := float64(hash2(x>>2, y>>2, seed)&255) / 255
			// A fold darkens one side and lifts the paper on the other.
			fold := math.Exp(-dist*dist/(2*2.2*2.2)) * (0.18 + 0.1*n)
			cast := math.Exp(-(dist-3)*(dist-3)/(2*5.0*5.0)) * 0.08
			shift := (fold - cast) * amt
			di := (x - b.Min.X) * 4
			for k := 0; k < 3; k++ {
				v := float64(drow[di+k])
				if shift < 0 {
					v *= 1 + shift
				} else {
					v *= 1 - shift
				}
				if v < 0 {
					v = 0
				}
				if v > 255 {
					v = 255
				}
				drow[di+k] = uint8(v)
			}
		}
	}
}
