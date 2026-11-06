// Package ink rasterises glyph sprites and pen strokes with a physical ink
// model: pressure, dryness, paper absorption, blots and smudges.
package ink

import (
	"image"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"

	"typeshit/internal/model"
)

const (
	penBallpoint = iota
	penGel
	penPencil
	penFountain
)

// Canvas holds the loaded profile, atlases and pen configuration.
type Canvas struct {
	Prof    *model.Profile
	Atlases []image.Image
	Pen     model.PenSpec
	Rng     *rand.Rand

	kind    int
	color   [3]float64 // 0..1 linear-ish ink colour
	base    float64    // base opacity
	density float64
	gamma   float64
	lut     [256]float64
	bleed   float64
	skip    float64 // probability that a glyph has a pen skip

	spriteCache map[*model.Sprite]*image.Alpha
	xformCache  map[xformKey]*image.Alpha
	xformOrder  []xformKey
}

type xformKey struct {
	sp    *model.Sprite
	scale int32 // px per em × 4
	rot   int16 // degrees × 2
	slant int16 // degrees × 2
}

// New builds a canvas for one page batch.
func New(prof *model.Profile, atlases []image.Image, pen model.PenSpec, rng *rand.Rand) *Canvas {
	if rng == nil {
		rng = rand.New(rand.NewPCG(1, 2))
	}
	c := &Canvas{
		Prof:        prof,
		Atlases:     atlases,
		Pen:         pen,
		Rng:         rng,
		spriteCache: map[*model.Sprite]*image.Alpha{},
		xformCache:  map[xformKey]*image.Alpha{},
	}
	c.color = parseColor(pen.Color)
	c.density = clamp(pen.InkDensity, 0, 1)
	if c.density <= 0 {
		c.density = 1
	}
	pressure := clamp(pen.Pressure, 0, 1)
	if pressure <= 0 {
		pressure = 0.55
	}
	width := pen.Width
	switch strings.ToLower(pen.Kind) {
	case "gel":
		c.kind, c.base, c.gamma, c.skip = penGel, 1.02, 0.56, 0.01
	case "pencil":
		c.kind, c.base, c.gamma, c.skip = penPencil, 0.55, 1.30, 0.06
	case "fountain", "pen", "перьевая":
		c.kind, c.base, c.gamma, c.skip = penFountain, 1.0, 0.60, 0.02
	default:
		c.kind, c.base, c.gamma, c.skip = penBallpoint, 1.0, 0.58, 0.045
	}
	if width > 0 {
		width = clamp(width, 0.3, 3)
		// A broader nib means fatter strokes: lower the gamma exponent.
		c.gamma *= clamp(1/width, 0.55, 1.9)
	}
	c.gamma *= clamp(1.35-0.6*pressure, 0.6, 1.6)
	c.base *= 0.72 + 0.28*pressure
	c.bleed = clamp(pen.Bleed, 0, 1)
	if c.bleed <= 0 && c.kind == penFountain {
		c.bleed = 0.35
	}
	for i := 0; i < 256; i++ {
		c.lut[i] = math.Pow(float64(i)/255, c.gamma)
	}
	return c
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func parseColor(s string) [3]float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return [3]float64{0.08, 0.11, 0.22}
	}
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		switch len(h) {
		case 3:
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		case 6, 8:
			h = h[:6]
		default:
			return [3]float64{0.08, 0.11, 0.22}
		}
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return [3]float64{0.08, 0.11, 0.22}
		}
		return [3]float64{
			float64((v>>16)&0xff) / 255,
			float64((v>>8)&0xff) / 255,
			float64(v&0xff) / 255,
		}
	}
	switch strings.ToLower(s) {
	case "black", "чёрный", "черный":
		return [3]float64{0.07, 0.07, 0.08}
	case "blue", "синий":
		return [3]float64{0.09, 0.16, 0.55}
	case "red", "красный":
		return [3]float64{0.65, 0.09, 0.11}
	case "green", "зелёный", "зеленый":
		return [3]float64{0.09, 0.42, 0.2}
	}
	return [3]float64{0.08, 0.11, 0.22}
}

// SpriteMask extracts a sprite from its atlas with a one pixel transparent
// margin so bilinear resampling never bleeds from the neighbours.
func SpriteMask(sp *model.Sprite, atlas image.Image) *image.Alpha {
	if sp == nil || atlas == nil {
		return image.NewAlpha(image.Rect(0, 0, 0, 0))
	}
	x0, y0, x1, y1 := sp.Pix[0], sp.Pix[1], sp.Pix[2], sp.Pix[3]
	w, h := x1-x0, y1-y0
	if w <= 0 || h <= 0 {
		return image.NewAlpha(image.Rect(0, 0, 0, 0))
	}
	m := image.NewAlpha(image.Rect(0, 0, w+2, h+2))
	b := atlas.Bounds()
	switch src := atlas.(type) {
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			sy := y0 + y
			if sy < b.Min.Y || sy >= b.Max.Y {
				continue
			}
			for x := 0; x < w; x++ {
				sx := x0 + x
				if sx < b.Min.X || sx >= b.Max.X {
					continue
				}
				m.Pix[(y+1)*m.Stride+(x+1)] = src.Pix[(sy-src.Rect.Min.Y)*src.Stride+(sx-src.Rect.Min.X)*4+3]
			}
		}
	case *image.RGBA:
		for y := 0; y < h; y++ {
			sy := y0 + y
			if sy < b.Min.Y || sy >= b.Max.Y {
				continue
			}
			for x := 0; x < w; x++ {
				sx := x0 + x
				if sx < b.Min.X || sx >= b.Max.X {
					continue
				}
				m.Pix[(y+1)*m.Stride+(x+1)] = src.Pix[(sy-src.Rect.Min.Y)*src.Stride+(sx-src.Rect.Min.X)*4+3]
			}
		}
	case *image.Alpha:
		for y := 0; y < h; y++ {
			sy := y0 + y
			if sy < b.Min.Y || sy >= b.Max.Y {
				continue
			}
			for x := 0; x < w; x++ {
				sx := x0 + x
				if sx < b.Min.X || sx >= b.Max.X {
					continue
				}
				m.Pix[(y+1)*m.Stride+(x+1)] = src.Pix[(sy-src.Rect.Min.Y)*src.Stride+(sx-src.Rect.Min.X)]
			}
		}
	default:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				_, _, _, a := atlas.At(x0+x, y0+y).RGBA()
				m.Pix[(y+1)*m.Stride+(x+1)] = uint8(a >> 8)
			}
		}
	}
	return m
}

func (c *Canvas) rawMask(sp *model.Sprite) *image.Alpha {
	if m, ok := c.spriteCache[sp]; ok {
		return m
	}
	var atlas image.Image
	if sp.Atlas >= 0 && sp.Atlas < len(c.Atlases) {
		atlas = c.Atlases[sp.Atlas]
	}
	m := SpriteMask(sp, atlas)
	c.spriteCache[sp] = m
	return m
}

// spriteBox returns the em box the sprite covers.
func spriteBox(sp *model.Sprite) model.Rect {
	b := sp.Box
	if b.W <= 0.001 {
		b.W = 0.3
	}
	if b.H <= 0.001 {
		b.H = 0.5
	}
	return b
}

// glyphPoint maps an em point of a glyph to page pixels. It mirrors the
// transform used by the layout package.
func glyphPointAt(op *model.GlyphOp, scale, rot, slant float64, p model.Point) (float64, float64) {
	k := math.Tan(slant * math.Pi / 180)
	th := rot * math.Pi / 180
	x1 := p.X * scale
	y1 := p.Y * scale
	x2 := x1 + y1*k
	y2 := y1
	cos, sin := math.Cos(th), math.Sin(th)
	x3 := x2*cos - y2*sin
	y3 := x2*sin + y2*cos
	return op.X + x3, op.Y - y3
}

// maskFor returns the transformed coverage mask of op, snapped to integers so
// that the transform cache cannot accumulate rounding drift.
func (c *Canvas) maskFor(op *model.GlyphOp) (*image.Alpha, int, int) {
	sp := op.Sprite
	scale := op.Scale
	if scale <= 0 {
		scale = 16
	}
	sB := int32(math.Round(scale * 4))
	scale = float64(sB) / 4
	rB := int16(math.Round(op.Rotate * 2))
	slB := int16(math.Round(op.Slant * 2))
	rot := float64(rB) / 2
	slant := float64(slB) / 2
	key := xformKey{sp: sp, scale: sB, rot: rB, slant: slB}
	if m, ok := c.xformCache[key]; ok {
		return m, keyOffsetX(m), keyOffsetY(m)
	}
	m, ox, oy := c.buildMask(sp, scale, rot, slant)
	c.xformCache[key] = m
	c.xformOrder = append(c.xformOrder, key)
	if len(c.xformOrder) > 4096 {
		for i := 0; i < 1024; i++ {
			delete(c.xformCache, c.xformOrder[i])
		}
		c.xformOrder = append([]xformKey{}, c.xformOrder[1024:]...)
	}
	return m, ox, oy
}

// The bbox origin is stored in the mask rectangle's Min field so that the
// cache stays a plain map.
func keyOffsetX(m *image.Alpha) int { return m.Rect.Min.X }
func keyOffsetY(m *image.Alpha) int { return m.Rect.Min.Y }

func (c *Canvas) buildMask(sp *model.Sprite, scale, rot, slant float64) (*image.Alpha, int, int) {
	box := spriteBox(sp)
	raw := c.rawMask(sp)
	if raw == nil || raw.Rect.Dx() == 0 {
		return image.NewAlpha(image.Rect(0, 0, 0, 0)), 0, 0
	}
	empty := &model.GlyphOp{Sprite: sp, Scale: scale, Rotate: rot, Slant: slant}
	corners := [4]model.Point{
		{X: box.X, Y: box.Y},
		{X: box.X + box.W, Y: box.Y},
		{X: box.X, Y: box.Y + box.H},
		{X: box.X + box.W, Y: box.Y + box.H},
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range corners {
		x, y := glyphPointAt(empty, scale, rot, slant, p)
		minX = math.Min(minX, x)
		maxX = math.Max(maxX, x)
		minY = math.Min(minY, y)
		maxY = math.Max(maxY, y)
	}
	ox := int(math.Floor(minX)) - 1
	oy := int(math.Floor(minY)) - 1
	w := int(math.Ceil(maxX)) + 1 - ox
	h := int(math.Ceil(maxY)) + 1 - oy
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return image.NewAlpha(image.Rect(0, 0, 0, 0)), 0, 0
	}
	// Pad for the antialiased skirt.
	pad := 1
	m := image.NewAlpha(image.Rect(ox-pad, oy-pad, ox+w+pad, oy+h+pad))
	ox -= pad
	oy -= pad
	w += 2 * pad
	h += 2 * pad

	k := math.Tan(slant * math.Pi / 180)
	cos, sin := math.Cos(rot*math.Pi/180), math.Sin(rot*math.Pi/180)
	inv := 1 / scale
	mw := float64(raw.Rect.Dx() - 2)
	mh := float64(raw.Rect.Dy() - 2)
	if mw < 1 || mh < 1 {
		return image.NewAlpha(image.Rect(0, 0, 0, 0)), 0, 0
	}

	for iy := 0; iy < h; iy++ {
		py := float64(oy+iy) + 0.5
		row := m.Pix[iy*m.Stride:]
		for ix := 0; ix < w; ix++ {
			px := float64(ox+ix) + 0.5
			dx := px
			dy := -py
			rx := dx*cos + dy*sin
			ry := -dx*sin + dy*cos
			ex := (rx - ry*k) * inv
			ey := ry * inv
			u := (ex - box.X) / box.W
			v := (box.Y + box.H - ey) / box.H
			if u < 0 || u > 1 || v < 0 || v > 1 {
				continue
			}
			sx := 1 + u*mw
			sy := 1 + v*mh
			row[ix] = sampleAlpha(raw, sx, sy)
		}
	}
	return m, ox, oy
}

// sampleAlpha does bilinear sampling on the padded sprite mask.
func sampleAlpha(m *image.Alpha, sx, sy float64) uint8 {
	x0 := int(sx)
	y0 := int(sy)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x0 >= m.Rect.Dx()-1 {
		x0 = m.Rect.Dx() - 2
	}
	if y0 >= m.Rect.Dy()-1 {
		y0 = m.Rect.Dy() - 2
	}
	if x0 < 0 || y0 < 0 {
		return 0
	}
	fx := sx - float64(x0)
	fy := sy - float64(y0)
	st := m.Stride
	p := m.Pix
	a := float64(p[y0*st+x0]) * (1 - fx)
	a += float64(p[y0*st+x0+1]) * fx
	b := float64(p[(y0+1)*st+x0]) * (1 - fx)
	b += float64(p[(y0+1)*st+x0+1]) * fx
	v := a*(1-fy) + b*fy
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v + 0.5)
}

// DrawPage stamps every glyph op onto dst, links first so the glyph body covers
// their ends.
func (c *Canvas) DrawPage(dst *image.NRGBA, ops []model.GlyphOp) {
	if dst == nil {
		return
	}
	for i := range ops {
		op := &ops[i]
		if op.Sprite == nil {
			continue
		}
		if op.Link != nil {
			c.drawLink(dst, op.Link)
		}
		c.drawGlyph(dst, op)
	}
}

// glyphNoise is the per-glyph recording of dry patches and pen skips.
type glyphNoise struct {
	seed    uint32
	skip    bool
	skipLo  float64
	skipHi  float64
	dryBias float64
}

// noiseFor derives the per-glyph ink accidents from the op seed, so a page is
// reproducible byte for byte for a given request.
func noiseFor(op *model.GlyphOp, skipChance float64) glyphNoise {
	s := uint32(op.Seed) ^ uint32(uint64(op.Seed)>>32)*0x9e3779b1
	g := glyphNoise{seed: s, dryBias: float64(s&255) / 255}
	if float64((s>>8)&1023)/1023 < skipChance {
		g.skip = true
		g.skipLo = 0.15 + float64((s>>12)&255)/255*0.55
		g.skipHi = g.skipLo + 0.04 + float64((s>>20)&255)/255*0.12
	}
	return g
}

func (c *Canvas) drawGlyph(dst *image.NRGBA, op *model.GlyphOp) {
	m, ox, oy := c.maskFor(op)
	if m.Rect.Dx() <= 0 {
		return
	}
	gx := int(math.Round(op.X)) + ox
	gy := int(math.Round(op.Y)) + oy

	db := dst.Bounds()
	x0 := maxInt(gx, db.Min.X)
	y0 := maxInt(gy, db.Min.Y)
	x1 := minInt(gx+m.Rect.Dx(), db.Max.X)
	y1 := minInt(gy+m.Rect.Dy(), db.Max.Y)
	if x1 <= x0 || y1 <= y0 {
		return
	}

	alpha := op.Alpha
	if alpha <= 0 {
		alpha = 1
	}
	inkAmount := op.Ink
	if inkAmount <= 0 {
		inkAmount = 1
	}
	// Lighter or heavier pressure changes how much pigment reaches the paper.
	coeff := c.density * c.base * alpha * (0.6 + 0.4*inkAmount)
	coeff *= clamp(op.Weight, 0.55, 1.8)
	if c.kind == penPencil {
		coeff *= 0.85
	}
	if coeff <= 0.002 {
		return
	}

	g := noiseFor(op, c.skip)
	cr := c.color[0] * 255
	cg := c.color[1] * 255
	cb := c.color[2] * 255
	// Pencil graphite is a little greyer and warmer.
	if c.kind == penPencil {
		cr, cg, cb = 0.9*cr+25, 0.9*cg+25, 0.9*cb+25
	}

	seed := g.seed
	for y := y0; y < y1; y++ {
		miy := y - gy
		mrow := m.Pix[miy*m.Stride:]
		frac := (float64(y-gy) + 0.5) / float64(m.Rect.Dy())
		skipRow := g.skip && frac >= g.skipLo && frac <= g.skipHi
		drow := dst.Pix[(y-db.Min.Y)*dst.Stride:]
		for x := x0; x < x1; x++ {
			mi := x - gx
			cov := mrow[mi]
			if cov == 0 {
				continue
			}
			a := c.lut[cov]
			if a <= 0.002 {
				continue
			}
			switch c.kind {
			case penBallpoint:
				n := float64(hash2(x, y, seed)&1023) / 1023
				a *= 0.86 + 0.16*n
				if n < 0.06*(0.5+g.dryBias) {
					a *= 0.55 // a dry speck of ballpoint ink
				}
				if skipRow {
					a *= 0.35
				}
			case penGel:
				a = math.Min(1, a*1.06)
			case penPencil:
				n := float64(hash2(x, y, seed)&511) / 511
				a *= 0.32 + 0.68*n
				if a < 0.08 {
					continue
				}
			case penFountain:
				edge := 4 * a * (1 - a)
				a += 0.16 * edge * (1 - a)
				if skipRow {
					a *= 0.6
				}
			}
			if c.bleed > 0.05 {
				// Paper absorption halo: a faint blur around the stroke.
				n := float64(hash2(x, y, seed^0x51ed) & 255)
				a += c.bleed * 0.05 * n / 255
			}
			a *= coeff
			if a <= 0.002 {
				continue
			}
			if a > 1 {
				a = 1
			}
			di := (x - db.Min.X) * 4
			inv := 1 - a
			drow[di] = uint8(float64(drow[di])*inv + cr*a)
			drow[di+1] = uint8(float64(drow[di+1])*inv + cg*a)
			drow[di+2] = uint8(float64(drow[di+2])*inv + cb*a)
		}
	}
}

func hash2(x, y int, seed uint32) uint32 {
	h := uint32(x)*374761393 + uint32(y)*668265263 + seed*1274126177
	h = (h ^ (h >> 13)) * 1274126177
	return h ^ (h >> 16)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
