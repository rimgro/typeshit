package profile

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"

	"typeshit/internal/model"
)

// dotChars are glyphs whose ink is legitimately tiny; speck filtering must not
// remove them.
var dotChars = map[string]bool{
	".": true, ",": true, ":": true, ";": true, "'": true, "\"": true, "-": true,
	"ё": true, "й": true, "Ё": true, "Й": true, "!": true, "?": true,
}

// xHeightChars have neither ascenders nor descenders, so their ink height is
// the writer's x-height.
var xHeightChars = map[string]bool{
	"а": true, "о": true, "е": true, "и": true, "н": true, "с": true, "м": true,
	"х": true, "ъ": true, "ь": true, "ы": true, "э": true, "ю": true, "я": true,
	"ш": true, "щ": true,
	"a": true, "c": true, "e": true, "m": true, "n": true, "o": true, "r": true,
	"s": true, "u": true, "v": true, "w": true, "x": true, "z": true,
}

// descenderChars sink below the baseline, so they cannot be used to measure it.
var descenderChars = map[string]bool{
	"у": true, "ф": true, "д": true, "ц": true, "щ": true, "з": true,
	"g": true, "j": true, "p": true, "q": true, "y": true, ",": true, ";": true, "/": true,
}

// analyzer holds intermediate state for one scan.
type analyzer struct {
	rgb  *image.NRGBA
	g    *grayImage // normalised canonical luminance
	cov  *image.NRGBA
	doc  *model.CaptureDoc
	note []string
}

// AnalyzeScan normalises a photo or scan of a sample sheet into canonical A4
// coordinates and proposes glyph cells.
//
// When spec describes a printed template (spec.Pages > 0) the known lattice is
// projected through the detected registration marks, which is far more robust
// than guessing the grid from ink. With spec.Pages == 0 the image is treated as
// a free-form handwriting sample and segmented bottom-up.
func AnalyzeScan(img image.Image, spec SheetSpec) (*model.CaptureDoc, *image.NRGBA, error) {
	if img == nil {
		return nil, nil, errors.New("profile: nil image")
	}
	b := img.Bounds()
	if b.Dx() < 64 || b.Dy() < 64 {
		return nil, nil, errors.New("profile: image too small")
	}

	work := limitSize(toNRGBA(img), 2600)
	// Deskew on a small copy: the angle does not need full resolution.
	skewCopy := work
	if maxInt(work.Rect.Dx(), work.Rect.Dy()) > 1000 {
		k := 1000.0 / float64(maxInt(work.Rect.Dx(), work.Rect.Dy()))
		skewCopy = resizeFast(work, int(float64(work.Rect.Dx())*k+0.5), int(float64(work.Rect.Dy())*k+0.5))
	}
	skew := estimateSkew(flattenIllumination(toGray(skewCopy), maxInt(skewCopy.Rect.Dx()/14, 8)))
	work = rotateNRGBA(work, -skew)

	a := &analyzer{note: []string{}}
	var srcToCanon homography
	if spec.Pages > 0 {
		l := layoutFor(spec.Lang, spec.Reps, spec.Page)
		srcToCanon = templateTransform(work, l, a)
	} else {
		srcToCanon = photoTransform(work)
	}

	canon := image.NewNRGBA(image.Rect(0, 0, sheetW, sheetH))
	fillRect(canon, canon.Bounds(), colPaper)
	// srcToCanon maps canonical coordinates to the (deskewed) photo, which is
	// exactly what the resampler needs to read the photo into the canonical
	// frame.
	if _, err := srcToCanon.inv(); err == nil {
		warpRGBA(work, canon, srcToCanon)
	} else {
		a.note = append(a.note, "Не удалось вычислить трансформацию, страница не выровнена.")
		copyFit(canon, work)
	}

	a.rgb = canon
	// The masks below compare each pixel with its local mean, which already
	// removes gradients, so no extra illumination pass is needed here.
	a.g = toGray(canon)
	doc := &model.CaptureDoc{W: sheetW, H: sheetH, Notes: a.note}
	a.doc = doc

	mask := inkMask(a.g, 14)
	if spec.Pages > 0 {
		l := layoutFor(spec.Lang, spec.Reps, spec.Page)
		a.buildTemplateCells(l, spec, mask)
	} else {
		a.buildFreeformCells(mask)
	}
	doc.Slant = a.measureSlant()
	doc.Em = a.estimateEm(mask)
	if doc.Em < 40 {
		doc.Em = guideEmFrac * cellH
		doc.Notes = append(doc.Notes, "Не удалось точно оценить размер почерка, взята высота по шаблону.")
	}
	return doc, canon, nil
}

// limitSize downscales an image so its longest side is at most maxDim.
func limitSize(img *image.NRGBA, maxDim int) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= maxDim && h <= maxDim {
		return img
	}
	s := float64(maxDim) / float64(maxInt(w, h))
	return resizeNRGBA(img, int(float64(w)*s+0.5), int(float64(h)*s+0.5))
}

// toNRGBA converts any image to NRGBA over a white background.
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	fillRect(out, out.Bounds(), color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Over)
	return out
}

// copyFit scales src into dst, preserving aspect and centring on white.
func copyFit(dst, src *image.NRGBA) {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := dst.Rect.Dx(), dst.Rect.Dy()
	s := math.Min(float64(dw)/float64(sw), float64(dh)/float64(sh))
	nw, nh := int(float64(sw)*s+0.5), int(float64(sh)*s+0.5)
	scaled := resizeNRGBA(src, nw, nh)
	ox, oy := (dw-nw)/2, (dh-nh)/2
	for y := 0; y < nh; y++ {
		copy(dst.Pix[(oy+y)*dst.Stride+ox*4:(oy+y)*dst.Stride+ox*4+nw*4],
			scaled.Pix[y*scaled.Stride:y*scaled.Stride+nw*4])
	}
}

// estimateSkew finds the rotation angle (degrees) that maximises the variance
// of the horizontal projection of dark pixels, using only dark pixel centres
// so no image rotation is needed per candidate.
func estimateSkew(g *grayImage) float64 {
	small := g
	if maxInt(g.w, g.h) > 800 {
		s := 800.0 / float64(maxInt(g.w, g.h))
		small = resizeGray(g, int(float64(g.w)*s+0.5), int(float64(g.h)*s+0.5))
	}
	mask := adaptiveMask(small, float64(maxInt(small.w/40, 4)))
	pts := make([]float64, 0, 4096)
	cx, cy := float64(small.w)/2, float64(small.h)/2
	step := 1
	if mask.count() > 120000 {
		step = 2
	}
	for y := 0; y < small.h; y += step {
		for x := 0; x < small.w; x += step {
			if mask.at(x, y) {
				pts = append(pts, float64(x)-cx, float64(y)-cy)
			}
		}
	}
	if len(pts) < 64 {
		return 0
	}
	score := func(deg float64) float64 {
		r := deg * math.Pi / 180
		s, c := math.Sin(r), math.Cos(r)
		bins := make([]int32, small.h+2)
		for i := 0; i < len(pts); i += 2 {
			x, y := pts[i], pts[i+1]
			yy := int(-s*x + c*y + cy + 1)
			if yy < 0 {
				yy = 0
			}
			if yy >= len(bins) {
				yy = len(bins) - 1
			}
			bins[yy]++
		}
		var acc float64
		for _, v := range bins {
			acc += float64(v) * float64(v)
		}
		return acc
	}
	best, bestScore := 0.0, -1.0
	for deg := -8.0; deg <= 8.0001; deg += 0.5 {
		if sc := score(deg); sc > bestScore {
			bestScore, best = sc, deg
		}
	}
	for deg := best - 0.4; deg <= best+0.4; deg += 0.1 {
		if sc := score(deg); sc > bestScore {
			bestScore, best = sc, deg
		}
	}
	return best
}

// resizeGray resizes a luminance image with bilinear sampling.
func resizeGray(g *grayImage, w, h int) *grayImage {
	out := newGray(w, h)
	if w <= 0 || h <= 0 {
		return out
	}
	sx := float64(g.w) / float64(w)
	sy := float64(g.h) / float64(h)
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*sy - 0.5
		y0 := int(math.Floor(fy))
		ay := fy - float64(y0)
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*sx - 0.5
			x0 := int(math.Floor(fx))
			ax := fx - float64(x0)
			v00 := float64(g.at(x0, y0))
			v10 := float64(g.at(x0+1, y0))
			v01 := float64(g.at(x0, y0+1))
			v11 := float64(g.at(x0+1, y0+1))
			out.p[y*w+x] = float32((v00*(1-ax)+v10*ax)*(1-ay) + (v01*(1-ax)+v11*ax)*ay)
		}
	}
	return out
}

// estimateSlant measures the dominant lean of near-vertical strokes.
//
// Block-wise structure tensors give the local gradient direction; the histogram
// of those directions peaks at the writing slant because a hand's vertical
// strokes all lean by roughly the same angle. Positive values lean right, in
// degrees.
func estimateSlant(mask *bitmap) float64 {
	if mask.count() < 200 {
		return 0
	}
	const block = 8
	const bins = 180 // 1 degree per bin over (-90, 90]
	hist := make([]float64, bins)
	step := 1
	if mask.w*mask.h > 2000000 {
		step = 2
	}
	grad := func(x, y int) (float64, float64) {
		gx := b2f(mask.at(x+1, y-1)) + 2*b2f(mask.at(x+1, y)) + b2f(mask.at(x+1, y+1)) -
			b2f(mask.at(x-1, y-1)) - 2*b2f(mask.at(x-1, y)) - b2f(mask.at(x-1, y+1))
		gy := b2f(mask.at(x-1, y+1)) + 2*b2f(mask.at(x, y+1)) + b2f(mask.at(x+1, y+1)) -
			b2f(mask.at(x-1, y-1)) - 2*b2f(mask.at(x, y-1)) - b2f(mask.at(x+1, y-1))
		return gx, gy
	}
	for by := 1; by+block < mask.h-1; by += block * step {
		for bx := 1; bx+block < mask.w-1; bx += block * step {
			var gxx, gyy, gxy float64
			ink := 0
			for y := by; y < by+block; y++ {
				for x := bx; x < bx+block; x++ {
					if !mask.at(x, y) {
						continue
					}
					ink++
					gx, gy := grad(x, y)
					gxx += gx * gx
					gyy += gy * gy
					gxy += gx * gy
				}
			}
			trace := gxx + gyy
			if ink < 6 || trace < 8 {
				continue
			}
			// Coherence: a block with mixed stroke directions is noise.
			coh := math.Hypot(gxx-gyy, 2*gxy) / trace
			if coh < 0.35 {
				continue
			}
			theta := 0.5 * math.Atan2(2*gxy, gxx-gyy)
			deg := theta * 180 / math.Pi
			bin := int(math.Floor(deg)) + 90
			if bin < 0 {
				bin = 0
			}
			if bin >= bins {
				bin = bins - 1
			}
			hist[bin] += trace
		}
	}
	// Smooth the histogram so a single noisy bin does not win.
	sm := make([]float64, bins)
	for i := range hist {
		var sum float64
		for d := -2; d <= 2; d++ {
			j := i + d
			if j < 0 || j >= bins {
				continue
			}
			sum += hist[j]
		}
		sm[i] = sum
	}
	best, bestV := 0, 0.0
	total := 0.0
	for i, v := range sm {
		total += v
		if v > bestV {
			best, bestV = i, v
		}
	}
	if total <= 0 {
		return 0
	}
	var sum, weight float64
	for d := -4; d <= 4; d++ {
		j := best + d
		if j < 0 || j >= bins {
			continue
		}
		sum += float64(j-90) * sm[j]
		weight += sm[j]
	}
	if weight == 0 {
		return 0
	}
	deg := sum / weight
	if deg > 45 || deg < -45 {
		// Horizontal strokes dominate: the sample is upright print writing.
		return 0
	}
	return deg
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
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

// measureSlant estimates the writing lean on a downscaled copy: the angle does
// not need full page resolution and this keeps the analyser fast.
func (a *analyzer) measureSlant() float64 {
	small := a.g
	if maxInt(a.g.w, a.g.h) > 1400 {
		s := 1400.0 / float64(maxInt(a.g.w, a.g.h))
		small = resizeGray(a.g, int(float64(a.g.w)*s+0.5), int(float64(a.g.h)*s+0.5))
	}
	return estimateSlant(inkMask(small, 8))
}

// estimateEm derives px-per-em from the x-height of the captured glyphs.
func (a *analyzer) estimateEm(mask *bitmap) float64 {
	var heights []float64
	for _, cell := range a.doc.Cells {
		if cell.Label == "" || !xHeightChars[cell.Label] {
			continue
		}
		r := image.Rect(cell.Rect[0], cell.Rect[1], cell.Rect[2], cell.Rect[3])
		bb := inkBoundsIn(mask, r, 2)
		if bb.Empty() {
			continue
		}
		heights = append(heights, float64(bb.Dy()))
	}
	if len(heights) < minXHeightCell {
		return 0
	}
	return median(heights) / sheetEmPerXH
}

// buildTemplateCells maps the deterministic lattice through the detected
// transform and scores each cell by its ink.
func (a *analyzer) buildTemplateCells(l *sheetLayout, spec SheetSpec, mask *bitmap) {
	a.doc.Rows = make([]model.CaptureRow, 0, l.Rows)
	for r := 0; r < l.Rows; r++ {
		top := float64(l.Grid.Min.Y + r*cellH)
		base := top + l.Baseline
		row := model.CaptureRow{Index: r, Top: top, Baseline: base, Bottom: top + cellH}
		row.Label = rowLabel(l, r)
		// Refine the baseline from the ink that sits on it: most letters touch
		// the baseline, so the median bottom edge is a good estimate.
		var bottoms []float64
		for _, c := range l.Cells {
			if c.Row != r || c.Char == "" || descenderChars[c.Char] || dotChars[c.Char] {
				continue
			}
			bb := inkBoundsFast(mask, c.Rect.Inset(6))
			if bb.Empty() {
				continue
			}
			bottoms = append(bottoms, float64(bb.Max.Y))
		}
		if len(bottoms) >= 3 {
			row.Baseline = median(bottoms)
		}
		a.doc.Rows = append(a.doc.Rows, row)
	}

	for i, c := range l.Cells {
		if i >= len(spec.Chars) && c.Char == "" {
			continue
		}
		label := c.Char
		if i < len(spec.Chars) && spec.Chars[i] != "" {
			label = spec.Chars[i]
		}
		bb := inkBoundsFast(mask, c.Rect.Inset(6))
		cell := model.CaptureCell{
			ID:   len(a.doc.Cells),
			Rect: [4]int{c.Rect.Min.X, c.Rect.Min.Y, c.Rect.Max.X, c.Rect.Max.Y},
			Row:  c.Row,
			Col:  c.Col,
		}
		if bb.Empty() || bb.Dx() < 3 || bb.Dy() < 3 {
			cell.Label = ""
			cell.Score = 0
		} else {
			cell.Label = label
			cell.Score = cellScore(bb, c.Rect)
		}
		a.doc.Cells = append(a.doc.Cells, cell)
	}
}

// rowLabel renders the characters of a lattice row for the UI.
func rowLabel(l *sheetLayout, row int) string {
	out := ""
	seen := map[string]bool{}
	for _, c := range l.Cells {
		if c.Row != row || c.Char == "" || seen[c.Char] {
			continue
		}
		seen[c.Char] = true
		out += c.Char
	}
	return out
}

// cellScore rates how confidently a cell holds one glyph: ink coverage and
// compactness relative to the cell.
func cellScore(bb image.Rectangle, cell image.Rectangle) float64 {
	if cell.Dx() <= 0 || cell.Dy() <= 0 {
		return 0
	}
	wRatio := float64(bb.Dx()) / float64(cell.Dx())
	hRatio := float64(bb.Dy()) / float64(cell.Dy())
	score := 0.5
	if wRatio > 0.05 && wRatio < 1.05 {
		score += 0.2
	}
	if hRatio > 0.05 && hRatio < 1.05 {
		score += 0.2
	}
	if wRatio > 0.95 || hRatio > 0.95 {
		score -= 0.3 // likely clipped by the cell edge
	}
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

// buildFreeformCells segments an arbitrary handwriting sample bottom-up.
func (a *analyzer) buildFreeformCells(mask *bitmap) {
	comps := components(mask)
	keep := make([]component, 0, len(comps))
	pageArea := float64(mask.w * mask.h)
	for _, c := range comps {
		if c.Area < 12 {
			continue
		}
		if float64(c.Area) > pageArea*0.2 {
			continue
		}
		if c.w() > mask.w*3/4 && c.h() > mask.h*3/4 {
			continue
		}
		keep = append(keep, c)
	}
	if len(keep) == 0 {
		a.note = append(a.note, "На фото не найдено рукописного текста.")
		return
	}
	// Group components into text lines by vertical overlap.
	sort.Slice(keep, func(i, j int) bool { return keep[i].MinY < keep[j].MinY })
	type line struct {
		comps []component
		top   int
		bot   int
	}
	var lines []*line
	for _, c := range keep {
		placed := false
		for _, ln := range lines {
			overlap := minInt(ln.bot, c.MaxY) - maxInt(ln.top, c.MinY)
			if overlap > 0 && overlap*2 > (c.MaxY-c.MinY) {
				ln.comps = append(ln.comps, c)
				ln.top = minInt(ln.top, c.MinY)
				ln.bot = maxInt(ln.bot, c.MaxY)
				placed = true
				break
			}
		}
		if !placed {
			lines = append(lines, &line{comps: []component{c}, top: c.MinY, bot: c.MaxY})
		}
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].top < lines[j].top })

	for li, ln := range lines {
		sort.Slice(ln.comps, func(i, j int) bool { return ln.comps[i].MinX < ln.comps[j].MinX })
		// Split into tokens on wide horizontal gaps.
		var tokens [][]component
		var cur []component
		for _, c := range ln.comps {
			if len(cur) > 0 {
				prev := cur[len(cur)-1]
				gap := c.MinX - prev.MaxX
				med := medianComponentWidth(cur)
				if float64(gap) > med*0.55+4 {
					tokens = append(tokens, cur)
					cur = nil
				}
			}
			cur = append(cur, c)
		}
		if len(cur) > 0 {
			tokens = append(tokens, cur)
		}
		var bottoms []float64
		for _, c := range ln.comps {
			bottoms = append(bottoms, float64(c.MaxY))
		}
		baseline := float64(ln.bot)
		if len(bottoms) > 0 {
			baseline = percentile(bottoms, 0.6)
		}
		a.doc.Rows = append(a.doc.Rows, model.CaptureRow{
			Index:    li,
			Top:      float64(ln.top),
			Baseline: baseline,
			Bottom:   float64(ln.bot),
		})
		for _, tok := range tokens {
			x0, y0, x1, y1 := tok[0].MinX, tok[0].MinY, tok[0].MaxX, tok[0].MaxY
			for _, c := range tok[1:] {
				x0 = minInt(x0, c.MinX)
				y0 = minInt(y0, c.MinY)
				x1 = maxInt(x1, c.MaxX)
				y1 = maxInt(y1, c.MaxY)
			}
			tokenRect := image.Rect(x0, y0, x1, y1)
			// Cursive words arrive as one blob; roughly split them on the
			// deepest troughs of the vertical ink profile so the user gets a
			// usable starting point instead of one giant cell.
			for _, r := range splitToken(mask, tokenRect) {
				pad := int(math.Max(2, float64(r.Dy())*0.12))
				r = image.Rect(r.Min.X-pad, r.Min.Y-pad, r.Max.X+pad, r.Max.Y+pad).Intersect(maskRect(mask))
				if r.Dx() < 3 || r.Dy() < 3 {
					continue
				}
				a.doc.Cells = append(a.doc.Cells, model.CaptureCell{
					ID:    len(a.doc.Cells),
					Rect:  [4]int{r.Min.X, r.Min.Y, r.Max.X, r.Max.Y},
					Row:   li,
					Score: 0.4,
				})
			}
		}
	}
	a.note = append(a.note, "Разметка найдена автоматически: подпишите каждую область символом.")
}

// splitToken cuts a wide ink blob into roughly letter-sized pieces at the
// lightest columns around the expected boundaries.
func splitToken(mask *bitmap, r image.Rectangle) []image.Rectangle {
	r = r.Intersect(maskRect(mask))
	if r.Empty() {
		return nil
	}
	h := float64(r.Dy())
	if h < 4 {
		return []image.Rectangle{r}
	}
	n := int(math.Round(float64(r.Dx()) / (0.62 * h)))
	if n <= 1 || float64(r.Dx())/h < 0.95 {
		return []image.Rectangle{r}
	}
	profile := make([]int, r.Dx())
	for x := r.Min.X; x < r.Max.X; x++ {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			if mask.at(x, y) {
				profile[x-r.Min.X]++
			}
		}
	}
	bounds := []int{r.Min.X}
	step := float64(r.Dx()) / float64(n)
	for i := 1; i < n; i++ {
		want := r.Min.X + int(math.Round(step*float64(i)))
		search := int(step*0.45) + 2
		best, bestV := want, profile[clampi(want-r.Min.X, 0, len(profile)-1)]
		for x := want - search; x <= want+search; x++ {
			if x <= r.Min.X+1 || x >= r.Max.X-1 {
				continue
			}
			if v := profile[x-r.Min.X]; v < bestV {
				best, bestV = x, v
			}
		}
		bounds = append(bounds, best)
	}
	bounds = append(bounds, r.Max.X)
	out := make([]image.Rectangle, 0, n)
	for i := 0; i < len(bounds)-1; i++ {
		sub := image.Rect(bounds[i], r.Min.Y, bounds[i+1], r.Max.Y).Intersect(r)
		if sub.Dx() < 3 {
			continue
		}
		// Tighten vertically to the ink inside the slice.
		bb := inkBoundsIn(mask, sub, 6)
		if bb.Empty() {
			continue
		}
		out = append(out, bb)
	}
	return out
}

func maskRect(m *bitmap) image.Rectangle { return image.Rect(0, 0, m.w, m.h) }

// medianComponentWidth returns the median width of components.
func medianComponentWidth(cs []component) float64 {
	v := make([]float64, 0, len(cs))
	for _, c := range cs {
		v = append(v, float64(c.w()))
	}
	if len(v) == 0 {
		return 0
	}
	return median(v)
}

// percentile returns the q-quantile (0..1) of v; it sorts v in place.
func percentile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	idx := int(q * float64(len(v)-1))
	return v[idx]
}

// inkBoundsFast returns the bounding box of dark pixels inside r. It is used
// for cell scoring where a stray speck is harmless; CommitScan does the strict
// component filtering when it actually cuts glyphs.
func inkBoundsFast(m *bitmap, r image.Rectangle) image.Rectangle {
	r = r.Intersect(maskRect(m))
	if r.Empty() {
		return image.Rectangle{}
	}
	minX, minY := r.Max.X, r.Max.Y
	maxX, maxY := r.Min.X-1, r.Min.Y-1
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := m.b[y*m.w:]
		for x := r.Min.X; x < r.Max.X; x++ {
			if !row[x] {
				continue
			}
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
	if maxX < minX || maxY < minY {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

// inkBoundsIn returns the ink bounding box inside r, ignoring isolated specks
// smaller than minArea.
func inkBoundsIn(m *bitmap, r image.Rectangle, minArea int) image.Rectangle {
	r = r.Intersect(maskRect(m))
	if r.Empty() {
		return image.Rectangle{}
	}
	sub := newBitmap(r.Dx(), r.Dy())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := m.b[y*m.w:]
		drow := sub.b[(y-r.Min.Y)*sub.w:]
		copy(drow, row[r.Min.X:r.Max.X])
	}
	for _, c := range components(sub) {
		if c.Area < minArea {
			for y := c.MinY; y <= c.MaxY; y++ {
				for x := c.MinX; x <= c.MaxX; x++ {
					sub.b[y*sub.w+x] = false
				}
			}
		}
	}
	minX, minY := sub.w, sub.h
	maxX, maxY := -1, -1
	for y := 0; y < sub.h; y++ {
		for x := 0; x < sub.w; x++ {
			if sub.b[y*sub.w+x] {
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
	if maxX < minX {
		return image.Rectangle{}
	}
	return image.Rect(r.Min.X+minX, r.Min.Y+minY, r.Min.X+maxX+1, r.Min.Y+maxY+1)
}
