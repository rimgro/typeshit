package profile

import (
	"image"
	"math"
	"sort"

	"golang.org/x/image/draw"
)

// grayImage is a luminance image in 0..1 (1 = white paper).
type grayImage struct {
	w, h int
	p    []float32
}

func newGray(w, h int) *grayImage {
	return &grayImage{w: w, h: h, p: make([]float32, w*h)}
}

func (g *grayImage) at(x, y int) float32 {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= g.w {
		x = g.w - 1
	}
	if y >= g.h {
		y = g.h - 1
	}
	return g.p[y*g.w+x]
}

// toGray converts any image to luminance using Rec.709 weights, applying the
// alpha channel as a white background (transparent scans read as paper).
func toGray(img image.Image) *grayImage {
	b := img.Bounds()
	g := newGray(b.Dx(), b.Dy())
	switch src := img.(type) {
	case *image.NRGBA:
		for y := 0; y < g.h; y++ {
			srow := src.Pix[y*src.Stride:]
			drow := g.p[y*g.w:]
			for x := 0; x < g.w; x++ {
				o := x * 4
				a := float32(srow[o+3]) / 255
				r := float32(srow[o+0]) / 255
				gg := float32(srow[o+1]) / 255
				bb := float32(srow[o+2]) / 255
				drow[x] = (0.2126*r + 0.7152*gg + 0.0722*bb) * a
			}
		}
		return g
	case *image.RGBA:
		for y := 0; y < g.h; y++ {
			srow := src.Pix[y*src.Stride:]
			drow := g.p[y*g.w:]
			for x := 0; x < g.w; x++ {
				o := x * 4
				a := float32(srow[o+3]) / 255
				r := float32(srow[o+0]) / 255
				gg := float32(srow[o+1]) / 255
				bb := float32(srow[o+2]) / 255
				drow[x] = (0.2126*r + 0.7152*gg + 0.0722*bb) * a
			}
		}
		return g
	case *image.Gray:
		for y := 0; y < g.h; y++ {
			srow := src.Pix[y*src.Stride:]
			drow := g.p[y*g.w:]
			for x := 0; x < g.w; x++ {
				drow[x] = float32(srow[x]) / 255
			}
		}
		return g
	}
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			r, gg, bb, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			f := float32(a) / 65535
			g.p[y*g.w+x] = (0.2126*float32(r) + 0.7152*float32(gg) + 0.0722*float32(bb)) / 65535 * f
		}
	}
	return g
}

// toNRGBA renders the grey image as an opaque NRGBA image.
func (g *grayImage) toNRGBA() *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, g.w, g.h))
	for y := 0; y < g.h; y++ {
		row := dst.Pix[y*dst.Stride:]
		for x := 0; x < g.w; x++ {
			v := clamp01(g.p[y*g.w+x])
			c := uint8(v*255 + 0.5)
			o := x * 4
			row[o+0], row[o+1], row[o+2], row[o+3] = c, c, c, 255
		}
	}
	return dst
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// boxBlur applies a separable box blur with the given radius. Edges replicate.
// The inner loops avoid bounds checks because this runs on multi-megapixel
// pages several times per capture.
func boxBlur(g *grayImage, r int) *grayImage {
	if r < 1 {
		return g
	}
	w, h := g.w, g.h
	tmp := newGray(w, h)
	out := newGray(w, h)
	inv := 1 / float32(2*r+1)
	for y := 0; y < h; y++ {
		row := g.p[y*w : (y+1)*w]
		dst := tmp.p[y*w : (y+1)*w]
		var sum float32
		for i := -r; i <= r; i++ {
			sum += row[clampi(i, 0, w-1)]
		}
		for x := 0; x < w; x++ {
			dst[x] = sum * inv
			ox := x - r
			if ox < 0 {
				ox = 0
			}
			ix := x + r + 1
			if ix >= w {
				ix = w - 1
			}
			sum -= row[ox]
			sum += row[ix]
		}
	}
	for x := 0; x < w; x++ {
		var sum float32
		for i := -r; i <= r; i++ {
			sum += tmp.p[clampi(i, 0, h-1)*w+x]
		}
		for y := 0; y < h; y++ {
			out.p[y*w+x] = sum * inv
			oy := y - r
			if oy < 0 {
				oy = 0
			}
			iy := y + r + 1
			if iy >= h {
				iy = h - 1
			}
			sum -= tmp.p[oy*w+x]
			sum += tmp.p[iy*w+x]
		}
	}
	return out
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// flattenIllumination divides out a heavily blurred copy of the image, which
// removes gradients and shadows from phone photos while keeping ink.
func flattenIllumination(g *grayImage, radius int) *grayImage {
	bg := boxBlur(g, radius)
	// Normalise against the mean background so overall exposure is kept.
	var mean float32
	n := len(bg.p)
	step := 1
	if n > 400000 {
		step = n / 400000
	}
	var cnt int
	for i := 0; i < n; i += step {
		mean += bg.p[i]
		cnt++
	}
	if cnt > 0 {
		mean /= float32(cnt)
	}
	if mean < 0.2 {
		mean = 0.2
	}
	// The correction is applied in place to keep peak memory down: callers
	// always hand in a freshly converted grey image.
	for i := range g.p {
		b := bg.p[i]
		if b < 0.05 {
			b = 0.05
		}
		g.p[i] = clamp01(g.p[i] * mean / b)
	}
	return g
}

// bitmap is a binary mask.
type bitmap struct {
	w, h int
	b    []bool
}

func newBitmap(w, h int) *bitmap { return &bitmap{w: w, h: h, b: make([]bool, w*h)} }

func (m *bitmap) at(x, y int) bool {
	if x < 0 || y < 0 || x >= m.w || y >= m.h {
		return false
	}
	return m.b[y*m.w+x]
}

func (m *bitmap) count() int {
	n := 0
	for _, v := range m.b {
		if v {
			n++
		}
	}
	return n
}

// adaptiveMask thresholds a grey image against its local mean, which tolerates
// uneven lighting and paper texture. k is the fraction of the local mean below
// which a pixel counts as dark: 0.88 catches pale printed rulings and grey
// guides (structure detection), 0.68 catches only real ink.
func adaptiveMask(g *grayImage, r float64) *bitmap { return adaptiveMaskK(g, r, 0.88) }

// inkMask keeps only strokes that are clearly darker than the paper, which
// excludes light-blue rulings and grey sample guides.
func inkMask(g *grayImage, r float64) *bitmap { return adaptiveMaskK(g, r, 0.68) }

func adaptiveMaskK(g *grayImage, r, k float64) *bitmap {
	win := int(r)
	if win < 1 {
		win = 1
	}
	// A separable box blur gives the local mean without allocating an
	// integral image, which matters a lot on 8 megapixel pages.
	bg := boxBlur(g, win)
	m := newBitmap(g.w, g.h)
	for i, v := range g.p {
		if float64(v) < float64(bg.p[i])*k-0.005 {
			m.b[i] = true
		}
	}
	return m
}

// component is a connected dark region.
type component struct {
	Area       int
	MinX, MinY int
	MaxX, MaxY int
	SumX, SumY float64
	Boundary   int
	// Bounds of ink touching the box edges is filled by callers on demand.
}

func (c component) cx() float64 { return c.SumX / float64(c.Area) }
func (c component) cy() float64 { return c.SumY / float64(c.Area) }
func (c component) w() int      { return c.MaxX - c.MinX + 1 }
func (c component) h() int      { return c.MaxY - c.MinY + 1 }

// components finds 8-connected dark regions.
func components(m *bitmap) []component {
	out, _ := componentsLabeled(m)
	return out
}

// componentsLabeled finds 8-connected dark regions and returns a per-pixel
// component label (-1 for background), which callers use to keep or drop whole
// regions without touching overlapping bounding boxes.
func componentsLabeled(m *bitmap) ([]component, []int32) {
	seen := make([]bool, len(m.b))
	label := make([]int32, len(m.b))
	for i := range label {
		label[i] = -1
	}
	out := []component{}
	queue := make([]int, 0, 4096)
	for i := range m.b {
		if !m.b[i] || seen[i] {
			continue
		}
		id := int32(len(out))
		c := component{MinX: m.w, MinY: m.h, MaxX: -1, MaxY: -1}
		queue = queue[:0]
		queue = append(queue, i)
		seen[i] = true
		for head := 0; head < len(queue); head++ {
			idx := queue[head]
			label[idx] = id
			x, y := idx%m.w, idx/m.w
			c.Area++
			c.SumX += float64(x)
			c.SumY += float64(y)
			if x < c.MinX {
				c.MinX = x
			}
			if y < c.MinY {
				c.MinY = y
			}
			if x > c.MaxX {
				c.MaxX = x
			}
			if y > c.MaxY {
				c.MaxY = y
			}
			edge := x == 0 || y == 0 || x == m.w-1 || y == m.h-1
			for dy := -1; dy <= 1 && !edge; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= m.w || ny >= m.h {
						edge = true
						break
					}
					ni := ny*m.w + nx
					if m.b[ni] && !seen[ni] {
						seen[ni] = true
						queue = append(queue, ni)
					}
				}
			}
			if edge {
				c.Boundary += 4
			} else {
				if !m.b[idx-m.w] {
					c.Boundary++
				}
				if !m.b[idx+m.w] {
					c.Boundary++
				}
				if !m.b[idx-1] {
					c.Boundary++
				}
				if !m.b[idx+1] {
					c.Boundary++
				}
			}
		}
		out = append(out, c)
	}
	return out, label
}

// resizeNRGBA scales an image with a high quality filter.
func resizeNRGBA(src *image.NRGBA, w, h int) *image.NRGBA {
	if w == src.Rect.Dx() && h == src.Rect.Dy() {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return dst
	}
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

// resizeFast is a cheap bilinear resize for intermediate analysis copies where
// speed matters more than the last bit of sharpness.
func resizeFast(src *image.NRGBA, w, h int) *image.NRGBA {
	if w == src.Rect.Dx() && h == src.Rect.Dy() {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return dst
	}
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

// rotateNRGBA rotates around the image centre into a canvas large enough to
// hold the result; corners are filled with paper white.
func rotateNRGBA(src *image.NRGBA, deg float64) *image.NRGBA {
	if math.Abs(deg) < 1e-6 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	r := deg * math.Pi / 180
	c, s := math.Abs(math.Cos(r)), math.Abs(math.Sin(r))
	nw := int(math.Ceil(float64(w)*c + float64(h)*s))
	nh := int(math.Ceil(float64(w)*s + float64(h)*c))
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	// White background so that bilinear edges do not turn black.
	for i := range out.Pix {
		out.Pix[i] = 0xff
	}
	cx, cy := float64(w)/2, float64(h)/2
	ncx, ncy := float64(nw)/2, float64(nh)/2
	// Inverse mapping: dst -> src.
	cos, sin := math.Cos(r), math.Sin(r)
	sp := src.Pix
	for y := 0; y < nh; y++ {
		dy := float64(y) + 0.5 - ncy
		for x := 0; x < nw; x++ {
			dx := float64(x) + 0.5 - ncx
			sx := cos*dx + sin*dy + cx
			sy := -sin*dx + cos*dy + cy
			if sx < 0 || sy < 0 || sx >= float64(w) || sy >= float64(h) {
				continue
			}
			x0, y0 := int(sx), int(sy)
			ax, ay := sx-float64(x0), sy-float64(y0)
			o := (y * out.Stride) + x*4
			var acc [3]float64
			for j := 0; j < 2; j++ {
				yy := clampi(y0+j, 0, h-1)
				wy := 1 - ay
				if j == 1 {
					wy = ay
				}
				for i := 0; i < 2; i++ {
					xx := clampi(x0+i, 0, w-1)
					wx := 1 - ax
					if i == 1 {
						wx = ax
					}
					w8 := wx * wy
					so := yy*src.Stride + xx*4
					acc[0] += float64(sp[so+0]) * w8
					acc[1] += float64(sp[so+1]) * w8
					acc[2] += float64(sp[so+2]) * w8
				}
			}
			out.Pix[o+0] = uint8(acc[0] + 0.5)
			out.Pix[o+1] = uint8(acc[1] + 0.5)
			out.Pix[o+2] = uint8(acc[2] + 0.5)
			out.Pix[o+3] = 255
		}
	}
	return out
}

// median returns the median of the values; it sorts the slice in place.
func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	n := len(v)
	if n%2 == 1 {
		return v[n/2]
	}
	return (v[n/2-1] + v[n/2]) / 2
}

// stdev returns the population standard deviation of v.
func stdev(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	var mean, acc float64
	for _, x := range v {
		mean += x
	}
	mean /= float64(len(v))
	for _, x := range v {
		acc += (x - mean) * (x - mean)
	}
	return math.Sqrt(acc / float64(len(v)))
}

// meanOf returns the arithmetic mean.
func meanOf(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}
