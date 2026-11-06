// Package paper paints page backgrounds: the sheet itself, the printed ruling
// of a notebook and the accumulated wear of a page that has been carried
// around in a bag.
package paper

import (
	"image"
	"math"
	"math/rand/v2"
	"strings"

	"typeshit/internal/model"
)

// rgb is a plain opaque colour.
type rgb struct{ r, g, b uint8 }

var (
	defaultPaper  = rgb{0xFB, 0xF8, 0xF1}
	defaultRuling = rgb{0x93, 0xAC, 0xCB}
	defaultMargin = rgb{0xD2, 0x8E, 0x8E}

	agingTint = rgb{0xEF, 0xE3, 0xC4}
	foxTint   = rgb{0xBF, 0xA4, 0x72}
	stainTint = rgb{0xC9, 0xB6, 0x8F}
	wornTint  = rgb{0xE4, 0xD2, 0xA8}
)

// Render paints a w×h page background.
//
// dpi is the raster resolution used to convert millimetres to pixels; pass 0
// to fall back to 96 dpi. The ruling pitch is exactly CellMM*dpi/25.4 px and
// ruling line k sits at y = k*pitch, which is what the layout engine uses to
// put the baselines on the printed lines.
//
// At Aging == 0 the sheet is a clean flat surface: every pixel outside the
// printed ruling is exactly the base colour. Any positive Aging switches on
// the grain, yellowing and wear layers.
func Render(spec model.PaperSpec, w, h int, dpi float64, rng *rand.Rand) *image.NRGBA {
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	if dpi <= 0 {
		dpi = 96
	}
	if rng == nil {
		rng = rand.New(rand.NewPCG(1, 2))
	}

	c := &canvas{img: image.NewNRGBA(image.Rect(0, 0, w, h)), w: w, h: h}
	base := parseHex(spec.Color, defaultPaper)
	c.fill(base)

	aging := clamp01(spec.Aging)
	if aging > 0 {
		addGrain(c, aging, rng)
		addOxidation(c, aging, base)
		addWear(c, aging, rng)
	}

	pitch := spec.CellMM * dpi / 25.4
	if spec.CellMM <= 0 || pitch < 6 {
		pitch = 8 * dpi / 25.4
	}
	if pitch < 6 {
		pitch = 6
	}

	ruling := mix(parseHex(spec.RulingColor, defaultRuling), base, 0.08*aging)
	switch strings.ToLower(strings.TrimSpace(spec.Kind)) {
	case "grid":
		drawHorizontalRuling(c, pitch, ruling, rng)
		drawVerticalRuling(c, pitch, ruling, rng)
	case "dot":
		drawDots(c, pitch, ruling, rng)
	case "blank", "custom":
		// printed ruling is absent; the burning question is only the paper
	case "lined", "":
		drawHorizontalRuling(c, pitch, ruling, rng)
	default:
		drawHorizontalRuling(c, pitch, ruling, rng)
	}

	drawMargin(c, spec, base, dpi, rng)
	return c.img
}

// ---------- canvas helpers ----------

type canvas struct {
	img  *image.NRGBA
	w, h int
}

func (c *canvas) fill(col rgb) {
	p := c.img.Pix
	for i := 0; i+3 < len(p); i += 4 {
		p[i] = col.r
		p[i+1] = col.g
		p[i+2] = col.b
		p[i+3] = 0xFF
	}
}

func clampInt(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// blend draws col over (x,y) with the given opacity (0..1).
func (c *canvas) blend(x, y int, col rgb, a float64) {
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	i := y*c.img.Stride + x*4
	p := c.img.Pix
	p[i] = uint8(float64(p[i])*(1-a) + float64(col.r)*a + 0.5)
	p[i+1] = uint8(float64(p[i+1])*(1-a) + float64(col.g)*a + 0.5)
	p[i+2] = uint8(float64(p[i+2])*(1-a) + float64(col.b)*a + 0.5)
}

// add shifts the pixel by d (can be negative) on all three channels.
func (c *canvas) add(x, y, d int) {
	if d == 0 || x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	i := y*c.img.Stride + x*4
	p := c.img.Pix
	p[i] = clampInt(int(p[i]) + d)
	p[i+1] = clampInt(int(p[i+1]) + d)
	p[i+2] = clampInt(int(p[i+2]) + d)
}

// addTinted shifts the pixel by d on all channels plus a per-channel tint.
func (c *canvas) addTinted(x, y int, dr, dg, db int) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	i := y*c.img.Stride + x*4
	p := c.img.Pix
	p[i] = clampInt(int(p[i]) + dr)
	p[i+1] = clampInt(int(p[i+1]) + dg)
	p[i+2] = clampInt(int(p[i+2]) + db)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// mix blends a towards b by t (0..1).
func mix(a, b rgb, t float64) rgb {
	t = clamp01(t)
	lerp := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return rgb{lerp(a.r, b.r), lerp(a.g, b.g), lerp(a.b, b.b)}
}

// ---------- colour parsing ----------

func parseHex(s string, def rgb) rgb {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return def
	}
	var v [3]uint8
	for i := 0; i < 3; i++ {
		hi, ok1 := hexVal(s[i*2])
		lo, ok2 := hexVal(s[i*2+1])
		if !ok1 || !ok2 {
			return def
		}
		v[i] = hi<<4 | lo
	}
	return rgb{v[0], v[1], v[2]}
}

func hexVal(b byte) (uint8, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

// ---------- printed ruling ----------

// drawHorizontalRuling paints the notebook ruling. Every line gets its own
// phase and amplitude so the printed lines are not digitally perfect.
func drawHorizontalRuling(c *canvas, pitch float64, col rgb, rng *rand.Rand) {
	amp := math.Min(pitch*0.02, 1.6)
	for k := 1; ; k++ {
		baseY := float64(k) * pitch
		if baseY > float64(c.h-1) {
			break
		}
		phase := rng.Float64() * math.Pi * 2
		freq := 0.0035 + rng.Float64()*0.004
		depth := 0.05 + rng.Float64()*0.06
		for x := 0; x < c.w; x++ {
			yf := baseY + amp*math.Sin(float64(x)*freq+phase) + amp*0.45*math.Sin(float64(x)*freq*3.7+phase*1.7)
			a := 0.60 * (1 - depth + depth*math.Sin(float64(x)*0.011+phase))
			drawSoftPixel(c, x, yf, col, a)
			// faint printed double edge, as if the ruling ink spread
			drawSoftPixel(c, x, yf+1.6, col, a*0.18)
		}
	}
}

func drawVerticalRuling(c *canvas, pitch float64, col rgb, rng *rand.Rand) {
	amp := math.Min(pitch*0.02, 1.6)
	for k := 1; ; k++ {
		baseX := float64(k) * pitch
		if baseX > float64(c.w-1) {
			break
		}
		phase := rng.Float64() * math.Pi * 2
		freq := 0.0035 + rng.Float64()*0.004
		depth := 0.05 + rng.Float64()*0.06
		for y := 0; y < c.h; y++ {
			xf := baseX + amp*math.Sin(float64(y)*freq+phase) + amp*0.45*math.Sin(float64(y)*freq*3.4+phase*1.9)
			a := 0.52 * (1 - depth + depth*math.Sin(float64(y)*0.013+phase))
			drawSoftPixelX(c, xf, y, col, a)
		}
	}
}

// drawSoftPixel antialiases a point across the two rows it lands on.
func drawSoftPixel(c *canvas, x int, yf float64, col rgb, a float64) {
	y0 := int(math.Floor(yf))
	f := yf - float64(y0)
	c.blend(x, y0, col, a*(1-f))
	c.blend(x, y0+1, col, a*f)
}

// drawSoftPixelX antialiases a point across the two columns it lands on.
func drawSoftPixelX(c *canvas, xf float64, y int, col rgb, a float64) {
	x0 := int(math.Floor(xf))
	f := xf - float64(x0)
	c.blend(x0, y, col, a*(1-f))
	c.blend(x0+1, y, col, a*f)
}

func drawDots(c *canvas, pitch float64, col rgb, rng *rand.Rand) {
	r := math.Max(1.0, pitch*0.035)
	for y := pitch; y < float64(c.h); y += pitch {
		for x := pitch; x < float64(c.w); x += pitch {
			jx := (rng.Float64() - 0.5) * 1.2
			jy := (rng.Float64() - 0.5) * 1.2
			disc(c, x+jx, y+jy, r, col, 0.5)
		}
	}
}

// disc fills a soft-edged circle.
func disc(c *canvas, cx, cy, r float64, col rgb, a float64) {
	x0 := int(cx - r - 1)
	x1 := int(cx + r + 1)
	y0 := int(cy - r - 1)
	y1 := int(cy + r + 1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			d := math.Hypot(dx, dy)
			if d > r+0.5 {
				continue
			}
			f := 1.0
			if d > r-0.5 {
				f = r + 0.5 - d
			}
			c.blend(x, y, col, a*f)
		}
	}
}

// drawMargin paints the vertical notebook margin ("красная линия").
func drawMargin(c *canvas, spec model.PaperSpec, base rgb, dpi float64, rng *rand.Rand) {
	side := strings.ToLower(strings.TrimSpace(spec.MarginSide))
	if side == "none" || spec.MarginMM <= 0 {
		return
	}
	x := spec.MarginMM * dpi / 25.4
	if side == "right" {
		x = float64(c.w) - x
	}
	if x < 4 || x > float64(c.w-4) {
		return
	}
	col := mix(defaultMargin, base, 0.12)
	phase := rng.Float64() * math.Pi * 2
	for y := 0; y < c.h; y++ {
		xf := x + 0.9*math.Sin(float64(y)*0.004+phase)
		a := 0.5 * (0.85 + 0.15*math.Sin(float64(y)*0.009+phase))
		xi := int(math.Round(xf))
		c.blend(xi, y, col, a)
		c.blend(xi+1, y, col, a*0.35)
		// faint printed double edge
		c.blend(xi+2, y, col, a*0.10)
	}
}
