package profile

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"typeshit/internal/assets"
)

// typeFace wraps an OpenType face at a fixed pixel size.
type typeFace struct {
	f    *opentype.Font
	face font.Face
	size float64
}

// newTypeFace parses font data and creates a face whose em is sizePx pixels.
func newTypeFace(data []byte, sizePx float64) (*typeFace, error) {
	if len(data) == 0 {
		return nil, errors.New("profile: empty font data")
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    sizePx,
		DPI:     72, // 1pt == 1px, so Size is exactly the em in pixels
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, err
	}
	return &typeFace{f: f, face: face, size: sizePx}, nil
}

func (t *typeFace) close() {
	if t != nil && t.face != nil {
		_ = t.face.Close()
	}
}

// has reports whether the font contains a real glyph for r.
func (t *typeFace) has(r rune) bool {
	var b sfnt.Buffer
	gi, err := t.f.GlyphIndex(&b, r)
	return err == nil && gi != 0
}

// draw renders s with its baseline starting at (x, y).
func (t *typeFace) draw(dst draw.Image, x, y float64, s string, col color.Color) {
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: t.face,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)},
	}
	d.DrawString(s)
}

// xHeight returns the font's x-height in pixels at the current size.
func (t *typeFace) xHeight() float64 {
	m := t.face.Metrics()
	return float64(m.XHeight) / 64
}

// measure returns the advance width of s in pixels.
func (t *typeFace) measure(s string) float64 {
	return float64(font.MeasureString(t.face, s)) / 64
}

// rasterGlyph is one glyph rendered on a canvas with known baseline geometry.
type rasterGlyph struct {
	mask      *image.Alpha
	ink       image.Rectangle // ink bounds inside mask
	originX   float64         // px x of the pen origin inside mask
	baselineY float64         // px y of the baseline inside mask
	advance   float64         // px
	density   float64         // mean coverage inside ink bounds
	weight    float64         // approx. stroke thickness in px
}

// renderGlyph rasterises one character at the face size onto a padded canvas.
func (t *typeFace) renderGlyph(r rune) (*rasterGlyph, bool) {
	em := t.size
	pad := int(em + 0.5)
	w := pad * 3
	h := pad * 3
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	baseline := float64(2 * pad)
	origin := float64(pad)
	t.draw(mask, origin, baseline, string(r), color.Alpha{A: 255})
	ink := inkBounds(mask)
	if ink.Empty() {
		return nil, false
	}
	adv := t.measure(string(r))
	if adv <= 0 {
		// Fallback: some glyphs report zero advance; use the ink width.
		adv = float64(ink.Dx()) * 1.08
	}
	area := 0.0
	var sum, inkPixels float64
	for y := ink.Min.Y; y < ink.Max.Y; y++ {
		row := mask.Pix[y*mask.Stride:]
		for x := ink.Min.X; x < ink.Max.X; x++ {
			a := float64(row[x]) / 255
			if a > 0.02 {
				area++
				sum += a
				inkPixels++
			}
		}
	}
	perim := alphaBoundary(mask, ink)
	weight := 0.0
	if perim > 0 {
		weight = 2 * area / perim
	}
	return &rasterGlyph{
		mask:      mask,
		ink:       ink,
		originX:   origin,
		baselineY: baseline,
		advance:   adv,
		density:   sum / math.Max(1, inkPixels),
		weight:    weight,
	}, true
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// inkBounds returns the bounding box of non-transparent alpha pixels.
func inkBounds(a *image.Alpha) image.Rectangle {
	b := a.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := a.Pix[y*a.Stride:]
		for x := b.Min.X; x < b.Max.X; x++ {
			if row[x] > 6 { // ignore barely-visible anti-aliasing
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < minX || maxY < minY {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

// alphaBoundary counts the pixels of the ink boundary inside r, used for the
// area/perimeter stroke thickness estimate.
func alphaBoundary(a *image.Alpha, r image.Rectangle) float64 {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := a.Pix[y*a.Stride:]
		for x := r.Min.X; x < r.Max.X; x++ {
			if row[x] <= 6 {
				continue
			}
			if x == r.Min.X || x == r.Max.X-1 || y == r.Min.Y || y == r.Max.Y-1 {
				n++
				continue
			}
			if row[x-1] <= 6 || row[x+1] <= 6 || a.Pix[(y-1)*a.Stride+x] <= 6 || a.Pix[(y+1)*a.Stride+x] <= 6 {
				n++
			}
		}
	}
	return float64(n)
}

// copyAlpha crops src to r and returns a standalone alpha image.
func cropAlpha(src *image.Alpha, r image.Rectangle) *image.Alpha {
	out := image.NewAlpha(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+r.Dx()], src.Pix[(r.Min.Y+y)*src.Stride+r.Min.X:(r.Min.Y+y)*src.Stride+r.Max.X])
	}
	return out
}

// uiFace loads a bundled legible Cyrillic font for printable sheet chrome.
func uiFace(sizePx float64) *typeFace {
	data, ok := assets.FontData("neucha")
	if !ok {
		return nil
	}
	tf, err := newTypeFace(data, sizePx)
	if err != nil {
		return nil
	}
	return tf
}

// fillRect paints an opaque rectangle into an NRGBA image.
func fillRect(dst *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	r = r.Intersect(dst.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := dst.Pix[y*dst.Stride:]
		for x := r.Min.X; x < r.Max.X; x++ {
			o := x * 4
			row[o+0], row[o+1], row[o+2], row[o+3] = c.R, c.G, c.B, 255
		}
	}
}

// blendRect paints c over dst with alpha blending.
func blendRect(dst *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	r = r.Intersect(dst.Bounds())
	a := float64(c.A) / 255
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := dst.Pix[y*dst.Stride:]
		for x := r.Min.X; x < r.Max.X; x++ {
			o := x * 4
			row[o+0] = uint8(float64(row[o+0])*(1-a) + float64(c.R)*a)
			row[o+1] = uint8(float64(row[o+1])*(1-a) + float64(c.G)*a)
			row[o+2] = uint8(float64(row[o+2])*(1-a) + float64(c.B)*a)
			row[o+3] = 255
		}
	}
}

// drawDashedLine draws a dashed line with the given dash/gap lengths.
func drawDashedLine(dst *image.NRGBA, x0, y0, x1, y1, dash, gap float64, c color.NRGBA) {
	length := dist(x0, y0, x1, y1)
	if length < 1 {
		return
	}
	ux, uy := (x1-x0)/length, (y1-y0)/length
	for t := 0.0; t < length; t += dash + gap {
		t2 := t + dash
		if t2 > length {
			t2 = length
		}
		n := int(t2 - t)
		for i := 0; i < n; i++ {
			x := int(x0 + ux*(t+float64(i)))
			y := int(y0 + uy*(t+float64(i)))
			if x < 0 || y < 0 || x >= dst.Rect.Dx() || y >= dst.Rect.Dy() {
				continue
			}
			o := y*dst.Stride + x*4
			dst.Pix[o+0], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = c.R, c.G, c.B, 255
		}
	}
}

// drawLine draws a solid 1px line.
func drawLine(dst *image.NRGBA, x0, y0, x1, y1 float64, c color.NRGBA) {
	dx, dy := x1-x0, y1-y0
	steps := int(maxf(absf(dx), absf(dy)))
	if steps <= 0 {
		return
	}
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		x := int(x0 + dx*f)
		y := int(y0 + dy*f)
		if x < 0 || y < 0 || x >= dst.Rect.Dx() || y >= dst.Rect.Dy() {
			continue
		}
		o := y*dst.Stride + x*4
		dst.Pix[o+0], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = c.R, c.G, c.B, 255
	}
}

func sortedInts(a, b int) (int, int) {
	if a > b {
		return b, a
	}
	return a, b
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
