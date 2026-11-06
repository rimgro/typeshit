package profile

import (
	"errors"
	"fmt"
	"image"
	"math"
	"sort"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"typeshit/internal/assets"
	"typeshit/internal/model"
)

// spriteEm is the resolution sprites are stored at inside an atlas: one em is
// spriteEm pixels. It is a compromise between glyph fidelity and atlas size.
const spriteEm = 128.0

// atlasWidth and atlasHeight bound one atlas page in pixels.
const (
	atlasWidth  = 2048
	atlasHeight = 2048
	atlasGutter = 3
	leftBearing = 0.03 // em, baked into every sprite's Box.X
)

// builtSprite is a sprite before it is packed into atlases.
type builtSprite struct {
	char       string
	variant    int
	mask       *image.Alpha // exactly the ink, no padding
	box        model.Rect   // em
	advance    float64
	join       model.Join
	ink        float64
	weight     float64
	confidence float64
}

// CommitScan turns an analysed and annotated scan into a handwriting profile.
// Every repetition of a character on the sheet becomes its own variant, which
// is what makes the generated pages look like one person's hand rather than a
// rubber stamp.
func CommitScan(doc *model.CaptureDoc, scan *image.NRGBA, req model.CommitRequest) (*model.Profile, []*image.NRGBA, error) {
	if doc == nil {
		return nil, nil, errors.New("profile: nil capture doc")
	}
	if scan == nil {
		return nil, nil, errors.New("profile: nil scan")
	}
	labels := make(map[int]string, len(doc.Cells))
	for _, c := range doc.Cells {
		labels[c.ID] = c.Label
	}
	for _, a := range req.Assignments {
		if a.CellID < 0 {
			continue
		}
		labels[a.CellID] = a.Label
	}
	em := doc.Em
	if em < 40 || math.IsNaN(em) {
		em = guideEmFrac * cellH
	}

	g := flattenIllumination(toGray(scan), 40)
	baselines := rowBaselines(doc, em)

	var built []*builtSprite
	perChar := map[string]int{}
	for _, cell := range doc.Cells {
		ch := labels[cell.ID]
		if ch == "" {
			continue
		}
		r := image.Rect(cell.Rect[0], cell.Rect[1], cell.Rect[2], cell.Rect[3])
		r = r.Intersect(image.Rect(0, 0, scan.Rect.Dx(), scan.Rect.Dy()))
		if r.Dx() < 4 || r.Dy() < 4 {
			continue
		}
		baseline := cellBaselineFor(cell, r, doc.Rows, baselines, em)
		sp, ok := spriteFromRegion(g, r, baseline, em, ch)
		if !ok {
			continue
		}
		sp.variant = perChar[ch]
		perChar[ch]++
		built = append(built, sp)
	}
	if len(built) == 0 {
		return nil, nil, errors.New("profile: в разметке нет ни одного подписанного символа")
	}

	// Characters missing from the sheet are grafted from a reference font so
	// the profile can render any text, then every glyph gets enough variants.
	lang := docLang(doc, labels)
	langs := []string{lang}
	grafted := graftMissing(&built, lang, 0.45)
	built = synthesizeVariants(built, 3)

	glyphs, atlases := packAtlases(built)
	style := measureStyle(glyphs, doc, em)
	overrideStyle(&style, req.Style)

	p := &model.Profile{
		ID:      "",
		Name:    req.Name,
		Created: time.Now(),
		Source:  "scan",
		Lang:    langs,
		Em:      spriteEm,
		AtlasW:  atlasWidth,
		AtlasH:  atlasHeight,
		Glyphs:  glyphs,
		Style:   style,
		Meta: map[string]string{
			"capturedEm":  fmt.Sprintf("%.1f", em),
			"slant":       fmt.Sprintf("%.1f", doc.Slant),
			"sheetSource": "scan",
		},
	}
	if len(grafted) > 0 {
		p.Meta["grafted"] = joinStrings(grafted)
		p.Meta["graftedHint"] = "символы, которых не было на листе, взяты из эталонного шрифта"
	}
	if req.Name == "" {
		p.Name = "Мой почерк"
	}
	if len(p.Name) > 60 {
		p.Name = p.Name[:60]
	}
	return p, atlases, nil
}

// rowBaselines maps a row index to the baseline in px, falling back to the
// lattice geometry when the analyser did not provide rows.
func rowBaselines(doc *model.CaptureDoc, em float64) map[int]float64 {
	out := make(map[int]float64, len(doc.Rows))
	for _, r := range doc.Rows {
		out[r.Index] = r.Baseline
	}
	return out
}

// cellBaselineFor resolves the baseline that belongs to one cell.
func cellBaselineFor(cell model.CaptureCell, r image.Rectangle, rows []model.CaptureRow, baselines map[int]float64, em float64) float64 {
	if b, ok := baselines[cell.Row]; ok && b > 0 {
		return b
	}
	// Markup edits may drop the row index; fall back to the row that overlaps
	// the cell vertically.
	best, bestOverlap := 0.0, 0.0
	for _, row := range rows {
		overlap := math.Min(float64(r.Max.Y), row.Bottom) - math.Max(float64(r.Min.Y), row.Top)
		if overlap > bestOverlap {
			bestOverlap, best = overlap, row.Baseline
		}
	}
	if bestOverlap > 0 {
		return best
	}
	// Last resort: assume the glyph sits on 85% of its own height.
	return float64(r.Min.Y) + 0.85*float64(r.Dy())
}

// spriteFromRegion cuts one glyph out of the normalised scan.
func spriteFromRegion(g *grayImage, r image.Rectangle, baseline, em float64, ch string) (*builtSprite, bool) {
	paper := localPaper(g, r)
	if paper < 0.3 {
		return nil, false
	}
	cov := coverageMap(g, r, paper, em)
	if cov.ink < 8 {
		return nil, false
	}
	bb := cov.bounds
	if bb.Empty() || bb.Dx() < 2 || bb.Dy() < 2 {
		return nil, false
	}
	inkW := float64(bb.Dx())
	inkH := float64(bb.Dy())
	if inkW > float64(r.Dx())*1.02 || inkH > em*1.6 {
		// Ink spilled over the cell boundary; keep it but lower confidence.
	}
	box := model.Rect{
		X: leftBearing,
		Y: (baseline - float64(bb.Max.Y)) / em,
		W: inkW / em,
		H: inkH / em,
	}
	advance := inkW/em + 2*leftBearing
	if ref, ok := referenceAdvance(ch); ok {
		if min := ref * 0.85; advance < min {
			advance = min
		}
	}

	// Crop the coverage and resample to the atlas resolution.
	cw, chh := bb.Dx(), bb.Dy()
	crop := make([]float32, cw*chh)
	for y := 0; y < chh; y++ {
		copy(crop[y*cw:(y+1)*cw], cov.cov[(bb.Min.Y-r.Min.Y+y)*cov.w+bb.Min.X-r.Min.X:])
	}
	tw := int(math.Round(box.W * spriteEm))
	th := int(math.Round(box.H * spriteEm))
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	if tw > 512 || th > 512 {
		return nil, false
	}
	mask := resizeCoverage(crop, cw, chh, tw, th)
	// The stored box must match the stored pixels exactly.
	box.W = float64(tw) / spriteEm
	box.H = float64(th) / spriteEm

	// In the resampled mask one em is exactly spriteEm pixels.
	scale := spriteEm / em
	join := computeJoin(mask, (baseline-float64(bb.Min.Y))*scale, spriteEm, advance)

	conf := 0.92
	if float64(bb.Dx()) >= float64(r.Dx())-3 || float64(bb.Dy()) >= float64(r.Dy())-3 {
		conf -= 0.2 // likely clipped by the cell edge
	}
	if cov.mean < 0.45 {
		conf -= 0.15
	}
	if len(ch) > 1 {
		conf -= 0.1
	}

	weightPx := cov.thickness
	return &builtSprite{
		char:       ch,
		mask:       mask,
		box:        box,
		advance:    advance,
		join:       join,
		ink:        cov.mean,
		weight:     weightPx / em,
		confidence: clampF(conf, 0.3, 1),
	}, true
}

// coverage is the per-pixel ink coverage of one cell.
type coverage struct {
	cov       []float32
	w, h      int
	bounds    image.Rectangle // ink bounds in image coordinates
	ink       int             // number of pixels above the threshold
	mean      float64         // mean coverage of kept ink
	thickness float64         // stroke thickness estimate in px
}

// localPaper estimates the paper luminance inside a cell.
func localPaper(g *grayImage, r image.Rectangle) float64 {
	r = r.Intersect(image.Rect(0, 0, g.w, g.h))
	if r.Empty() {
		return 0
	}
	vals := make([]float64, 0, r.Dx()*r.Dy()/9+16)
	for y := r.Min.Y; y < r.Max.Y; y += 3 {
		for x := r.Min.X; x < r.Max.X; x += 3 {
			vals = append(vals, float64(g.p[y*g.w+x]))
		}
	}
	return percentile(vals, 0.86)
}

// coverageMap converts luminance to ink coverage, dropping light ruling and
// grey sample glyphs while keeping even a faint pencil.
func coverageMap(g *grayImage, r image.Rectangle, paper, em float64) *coverage {
	w, h := r.Dx(), r.Dy()
	cov := &coverage{cov: make([]float32, w*h), w: w, h: h}
	den := math.Max(paper*0.78, 0.15)
	for y := 0; y < h; y++ {
		gy := r.Min.Y + y
		row := g.p[gy*g.w : (gy+1)*g.w]
		for x := 0; x < w; x++ {
			v := float64(row[r.Min.X+x])
			c := (paper - v) / den
			cov.cov[y*w+x] = float32(smoothstep(0.30, 0.62, c))
		}
	}
	// Component analysis on the binary mask.
	bin := newBitmap(w, h)
	for i, v := range cov.cov {
		if v > 0.35 {
			bin.b[i] = true
		}
	}
	comps, labelMap := componentsLabeled(bin)
	if len(comps) == 0 {
		return cov
	}
	// Choose the main component and keep the others only when they belong to
	// the same glyph (umlauts, dots, a lifted pen).
	main := 0
	for i, c := range comps {
		if c.Area > comps[main].Area {
			main = i
		}
	}
	keep := make([]bool, len(comps))
	areaThreshold := math.Max(8, float64(w*h)*0.0015)
	for i, c := range comps {
		var meanCov float64
		for y := c.MinY; y <= c.MaxY; y++ {
			for x := c.MinX; x <= c.MaxX; x++ {
				meanCov += float64(cov.cov[y*w+x])
			}
		}
		meanCov /= float64(c.Area)
		// Light grey sample guides and pale rulings are excluded by their
		// coverage; real pen ink is dense.
		if meanCov < 0.42 {
			continue
		}
		if float64(c.Area) < areaThreshold {
			// Keep small marks (umlauts, periods) only near the main glyph.
			if rectGap(c, comps[main]) > 0.22*em {
				continue
			}
		}
		keep[i] = true
	}
	minX, minY, maxX, maxY := w, h, -1, -1
	count := 0
	var sum float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			l := labelMap[idx]
			if l < 0 || !keep[l] {
				cov.cov[idx] = 0
				continue
			}
			v := cov.cov[idx]
			if v <= 0.35 {
				cov.cov[idx] = 0
				continue
			}
			count++
			sum += float64(v)
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
	if maxX < minX {
		return cov
	}
	cov.ink = count
	cov.mean = sum / math.Max(1, float64(count))
	cov.bounds = image.Rect(r.Min.X+minX, r.Min.Y+minY, r.Min.X+maxX+1, r.Min.Y+maxY+1)
	// Stroke thickness ≈ 2·area/perimeter of the kept mask.
	perim := 0
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if cov.cov[y*w+x] <= 0.35 {
				continue
			}
			if x == 0 || y == 0 || x == w-1 || y == h-1 ||
				cov.cov[y*w+x-1] <= 0.35 || cov.cov[y*w+x+1] <= 0.35 ||
				cov.cov[(y-1)*w+x] <= 0.35 || cov.cov[(y+1)*w+x] <= 0.35 {
				perim++
			}
		}
	}
	if perim > 0 {
		cov.thickness = 2 * float64(count) / float64(perim)
	}
	return cov
}

// RectGap returns the distance between a component and the main component's
// bounding box (0 when they overlap).
func rectGap(c, main component) float64 {
	dx := math.Max(0, math.Max(float64(main.MinX-c.MaxX), float64(c.MinX-main.MaxX)))
	dy := math.Max(0, math.Max(float64(main.MinY-c.MaxY), float64(c.MinY-main.MaxY)))
	return math.Hypot(dx, dy)
}

// resizeCoverage resamples a coverage buffer with bilinear interpolation.
func resizeCoverage(src []float32, sw, sh, dw, dh int) *image.Alpha {
	out := image.NewAlpha(image.Rect(0, 0, dw, dh))
	if dw == 0 || dh == 0 {
		return out
	}
	if dw == sw && dh == sh {
		for y := 0; y < dh; y++ {
			row := out.Pix[y*out.Stride:]
			for x := 0; x < dw; x++ {
				row[x] = uint8(clampF(float64(src[y*sw+x]), 0, 1)*255 + 0.5)
			}
		}
		return out
	}
	sx := float64(sw) / float64(dw)
	sy := float64(sh) / float64(dh)
	for y := 0; y < dh; y++ {
		fy := (float64(y)+0.5)*sy - 0.5
		y0 := int(math.Floor(fy))
		ay := fy - float64(y0)
		y0 = clampi(y0, 0, sh-1)
		y1 := clampi(y0+1, 0, sh-1)
		row := out.Pix[y*out.Stride:]
		for x := 0; x < dw; x++ {
			fx := (float64(x)+0.5)*sx - 0.5
			x0 := int(math.Floor(fx))
			ax := fx - float64(x0)
			x0 = clampi(x0, 0, sw-1)
			x1 := clampi(x0+1, 0, sw-1)
			v00 := float64(src[y0*sw+x0])
			v10 := float64(src[y0*sw+x1])
			v01 := float64(src[y1*sw+x0])
			v11 := float64(src[y1*sw+x1])
			v := (v00*(1-ax)+v10*ax)*(1-ay) + (v01*(1-ax)+v11*ax)*ay
			row[x] = uint8(clampF(v, 0, 1)*255 + 0.5)
		}
	}
	return out
}

// computeJoin finds the connection anchors and the join style of a glyph.
//
// The sprite is cropped to its ink, so both edge columns always contain
// something. A real cursive connection is recognisable because the edge ink is
// a thin stroke tip (a small vertical extent) rather than the side of a letter
// body, which spans most of the glyph height.
func computeJoin(m *image.Alpha, baselineRow, emPx, advancEm float64) model.Join {
	j := model.Join{Entry: model.Point{X: 0, Y: 0.02}, Exit: model.Point{X: advancEm, Y: 0.02}, Kind: "none"}
	if m == nil || m.Rect.Dx() < 3 || m.Rect.Dy() < 3 {
		return j
	}
	w, h := m.Rect.Dx(), m.Rect.Dy()
	entryY, leftTouch := edgeAnchor(m, 0, w)
	exitY, rightTouch := edgeAnchor(m, w, w)
	toEm := func(y float64) float64 { return (baselineRow - y) / emPx }
	ey := toEm(entryY)
	xy := toEm(exitY)
	j.Entry.Y = clampF(ey, -0.2, 0.95)
	j.Exit.Y = clampF(xy, -0.2, 0.95)
	if !leftTouch {
		j.Entry.Y = 0.03
	}
	if !rightTouch {
		j.Exit.Y = 0.03
	}
	switch {
	case !leftTouch && !rightTouch:
		j.Kind = "none"
	case rightTouch && j.Exit.Y > 0.5:
		j.Kind = "up"
	case leftTouch && rightTouch && j.Entry.Y > 0.22 && j.Exit.Y > 0.22:
		j.Kind = "high"
	default:
		j.Kind = "base"
	}
	_ = h
	return j
}

// edgeAnchor inspects one side of a glyph (side = 0 left, 1 right) and returns
// the coverage-weighted y where the ink leaves the glyph plus whether that
// looks like a cursive connection.
//
// The sprite is cropped to its ink, so the outermost columns always have some
// ink. A connection is instead recognised a little inward: a single short run
// of ink (a tail) connects, while a round bowl shows two runs or a tall run and
// a letter body shows one tall run.
func edgeAnchor(m *image.Alpha, side, w int) (float64, bool) {
	h := m.Rect.Dy()
	// Anchor: the outermost two columns.
	ax0, ax1 := 0, minInt(2, w)
	if side == 1 {
		ax0, ax1 = maxInt(0, w-2), w
	}
	var sum, weight float64
	for y := 0; y < h; y++ {
		row := m.Pix[y*m.Stride:]
		for x := ax0; x < ax1; x++ {
			a := float64(row[x]) / 255
			if a > 0.25 {
				sum += float64(y) * a
				weight += a
			}
		}
	}
	if weight == 0 {
		return 0, false
	}
	edgeY := sum / weight

	// Decision: a strip about a tenth of the width in from the edge.
	deep := w / 8
	if deep < 2 {
		deep = 2
	}
	if side == 0 {
		ax0, ax1 = deep, 2*deep
	} else {
		ax0, ax1 = w-2*deep, w-deep
	}
	if ax0 < 0 {
		ax0 = 0
	}
	if ax1 > w {
		ax1 = w
	}
	if ax1 <= ax0 {
		return edgeY, false
	}
	rows := make([]bool, h)
	for y := 0; y < h; y++ {
		row := m.Pix[y*m.Stride:]
		for x := ax0; x < ax1; x++ {
			if row[x] > 64 {
				rows[y] = true
				break
			}
		}
	}
	runs, extent := 0, 0
	inRun := false
	for _, on := range rows {
		if on {
			extent++
			if !inRun {
				runs++
			}
		}
		inRun = on
	}
	if runs != 1 || float64(extent)/float64(h) > 0.30 {
		return edgeY, false
	}
	return edgeY, true
}

// synthesizeVariants pads each character with geometric variants until it has
// at least want instances.
func synthesizeVariants(built []*builtSprite, want int) []*builtSprite {
	byChar := map[string][]*builtSprite{}
	for _, s := range built {
		byChar[s.char] = append(byChar[s.char], s)
	}
	chars := make([]string, 0, len(byChar))
	for c := range byChar {
		chars = append(chars, c)
	}
	sort.Strings(chars)
	out := make([]*builtSprite, 0, len(built)+len(chars)*want)
	for _, ch := range chars {
		group := byChar[ch]
		sort.Slice(group, func(i, j int) bool { return group[i].variant < group[j].variant })
		out = append(out, group...)
		srcs := group
		k := 0
		for len(group) < want {
			src := srcs[k%len(srcs)]
			k++
			rot := []float64{-1.4, 1.2, -0.7, 1.7}[k%4]
			scale := []float64{1.02, 0.98, 1.035, 0.975}[k%4]
			thick := []int{1, -1, 0, 1}[k%4]
			nv := &builtSprite{
				char:       ch,
				variant:    len(group),
				mask:       perturbMask(src.mask, scale, rot, thick),
				box:        src.box,
				advance:    src.advance * scale,
				join:       src.join,
				ink:        src.ink,
				weight:     src.weight * scale,
				confidence: clampF(src.confidence*0.75, 0.25, 0.7),
			}
			// The perturbed mask keeps the ink box, but resampling may add a
			// pixel; keep the stored box consistent with the pixels.
			nv.box.W = float64(nv.mask.Rect.Dx()) / spriteEm
			nv.box.H = float64(nv.mask.Rect.Dy()) / spriteEm
			nv.box.X = src.box.X
			nv.box.Y = src.box.Y
			group = append(group, nv)
			out = append(out, nv)
		}
	}
	return out
}

// perturbMask applies a tiny rotation/scale and stroke thickness change.
func perturbMask(src *image.Alpha, scale, rotDeg float64, thick int) *image.Alpha {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	nw := int(math.Round(float64(w)*scale)) + 2
	nh := int(math.Round(float64(h)*scale)) + 2
	if nw < 2 {
		nw = 2
	}
	if nh < 2 {
		nh = 2
	}
	dst := image.NewAlpha(image.Rect(0, 0, nw, nh))
	r := rotDeg * math.Pi / 180
	cos, sin := math.Cos(r), math.Sin(r)
	cx, cy := float64(w)/2, float64(h)/2
	ncx, ncy := float64(nw)/2, float64(nh)/2
	for y := 0; y < nh; y++ {
		dy := float64(y) + 0.5 - ncy
		for x := 0; x < nw; x++ {
			dx := float64(x) + 0.5 - ncx
			sx := (cos*dx+sin*dy)/scale + cx
			sy := (-sin*dx+cos*dy)/scale + cy
			if sx < 0 || sy < 0 || sx >= float64(w) || sy >= float64(h) {
				continue
			}
			v := bilinearAlpha(src, sx, sy)
			dst.Pix[y*dst.Stride+x] = uint8(v + 0.5)
		}
	}
	if thick != 0 {
		dst = morphAlpha(dst, thick)
	}
	return trimAlpha(dst)
}

func bilinearAlpha(src *image.Alpha, x, y float64) float64 {
	x0, y0 := int(math.Floor(x-0.5)), int(math.Floor(y-0.5))
	ax, ay := x-0.5-float64(x0), y-0.5-float64(y0)
	w, h := src.Rect.Dx(), src.Rect.Dy()
	at := func(xx, yy int) float64 {
		xx = clampi(xx, 0, w-1)
		yy = clampi(yy, 0, h-1)
		return float64(src.Pix[yy*src.Stride+xx])
	}
	return (at(x0, y0)*(1-ax)+at(x0+1, y0)*ax)*(1-ay) + (at(x0, y0+1)*(1-ax)+at(x0+1, y0+1)*ax)*ay
}

// morphAlpha dilates (thick > 0) or erodes (thick < 0) the mask.
func morphAlpha(src *image.Alpha, thick int) *image.Alpha {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewAlpha(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := dst.Pix[y*dst.Stride:]
		for x := 0; x < w; x++ {
			if thick > 0 {
				var m uint8
				for dy := -thick; dy <= thick; dy++ {
					for dx := -thick; dx <= thick; dx++ {
						v := atAlpha(src, x+dx, y+dy)
						if v > m {
							m = v
						}
					}
				}
				row[x] = m
			} else {
				m := uint8(255)
				for dy := thick; dy <= -thick; dy++ {
					for dx := thick; dx <= -thick; dx++ {
						v := atAlpha(src, x+dx, y+dy)
						if v < m {
							m = v
						}
					}
				}
				row[x] = m
			}
		}
	}
	return dst
}

func atAlpha(a *image.Alpha, x, y int) uint8 {
	if x < 0 || y < 0 || x >= a.Rect.Dx() || y >= a.Rect.Dy() {
		return 0
	}
	return a.Pix[y*a.Stride+x]
}

// trimAlpha crops an alpha image to its ink bounds.
func trimAlpha(a *image.Alpha) *image.Alpha {
	r := inkBounds(a)
	if r.Empty() {
		return a
	}
	return cropAlpha(a, r)
}

// atlasPacker packs sprites into fixed size atlas pages.
type atlasPacker struct {
	pages []*image.NRGBA
	x, y  int
	shelf int
}

func (p *atlasPacker) place(m *image.Alpha) (page int, rect [4]int) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	if p.x+w+atlasGutter*2 > atlasWidth {
		p.x = atlasGutter
		p.y += p.shelf + atlasGutter
		p.shelf = 0
	}
	if p.y+h+atlasGutter*2 > atlasHeight {
		p.pages = append(p.pages, image.NewNRGBA(image.Rect(0, 0, atlasWidth, atlasHeight)))
		p.x, p.y, p.shelf = atlasGutter, atlasGutter, 0
	}
	if len(p.pages) == 0 {
		p.pages = append(p.pages, image.NewNRGBA(image.Rect(0, 0, atlasWidth, atlasHeight)))
		p.x, p.y, p.shelf = atlasGutter, atlasGutter, 0
	}
	pageIdx := len(p.pages) - 1
	dst := p.pages[pageIdx]
	for yy := 0; yy < h; yy++ {
		drow := dst.Pix[(p.y+yy)*dst.Stride:]
		mrow := m.Pix[yy*m.Stride:]
		for xx := 0; xx < w; xx++ {
			o := (p.x + xx) * 4
			drow[o+0], drow[o+1], drow[o+2] = 255, 255, 255
			drow[o+3] = mrow[xx]
		}
	}
	rect = [4]int{p.x, p.y, p.x + w, p.y + h}
	if h > p.shelf {
		p.shelf = h
	}
	p.x += w + atlasGutter
	return pageIdx, rect
}

// packAtlases writes sprites into atlases and produces the profile glyph map.
func packAtlases(built []*builtSprite) (map[string][]model.Sprite, []*image.NRGBA) {
	glyphs := make(map[string][]model.Sprite, len(built))
	order := append([]*builtSprite(nil), built...)
	// Tall sprites first keeps the shelves tight.
	sort.SliceStable(order, func(i, j int) bool {
		return order[i].mask.Rect.Dy() > order[j].mask.Rect.Dy()
	})
	packer := &atlasPacker{}
	for _, s := range order {
		page, rect := packer.place(s.mask)
		glyphs[s.char] = append(glyphs[s.char], model.Sprite{
			Char:       s.char,
			Variant:    s.variant,
			Atlas:      page,
			Pix:        rect,
			Box:        s.box,
			Advance:    s.advance,
			Join:       s.join,
			Ink:        s.ink,
			Weight:     s.weight,
			Confidence: s.confidence,
		})
	}
	for _, list := range glyphs {
		sort.Slice(list, func(i, j int) bool { return list[i].Variant < list[j].Variant })
	}
	pages := packer.pages
	if len(pages) == 0 {
		pages = append(pages, image.NewNRGBA(image.Rect(0, 0, atlasWidth, atlasHeight)))
	}
	return glyphs, pages
}

// measureStyle derives the variability model from the captured variants.
func measureStyle(glyphs map[string][]model.Sprite, doc *model.CaptureDoc, em float64) model.StyleModel {
	st := model.StyleModel{
		// The sprites already carry the writer's own lean, so the mean shear
		// must stay at zero; only the dispersion is applied by the layout.
		Slant:        0,
		SlantSigma:   3.0,
		AdvanceScale: 1,
		WordGap:      0.58,
		LineGap:      1.4,
		RotSigma:     1.2,
	}
	if st.LineGap < 1 {
		st.LineGap = 1
	}
	// Baseline pitch: the distance between consecutive detected rows.
	if len(doc.Rows) >= 2 {
		var pitches []float64
		for i := 1; i < len(doc.Rows); i++ {
			d := doc.Rows[i].Baseline - doc.Rows[i-1].Baseline
			if d > 0 {
				pitches = append(pitches, d)
			}
		}
		if len(pitches) > 0 {
			pitch := median(pitches)
			st.LineGap = clampF(pitch/em, 1.1, 3.2)
		}
	}

	var advRel, scaleRel, dyAbs, inks, weights []float64
	joinable, total := 0, 0
	for _, group := range glyphs {
		if len(group) < 2 {
			total++
			continue
		}
		var advs, hs, ys []float64
		for _, s := range group {
			advs = append(advs, s.Advance)
			hs = append(hs, s.Box.H)
			ys = append(ys, s.Box.Y)
			inks = append(inks, s.Ink)
			weights = append(weights, s.Weight)
			total++
			if s.Join.Kind != "none" {
				joinable++
			}
		}
		if mean := meanOf(advs); mean > 0.01 {
			advRel = append(advRel, stdev(advs)/mean)
		}
		if mean := meanOf(hs); mean > 0.01 {
			scaleRel = append(scaleRel, stdev(hs)/mean)
		}
		dyAbs = append(dyAbs, stdev(ys))
	}
	st.DxSigma = clampF(meanOf(advRel), 0.01, 0.12)
	st.ScaleSigma = clampF(meanOf(scaleRel), 0.01, 0.12)
	st.DySigma = clampF(meanOf(dyAbs), 0.005, 0.06)
	st.RotSigma = clampF(1.2+st.ScaleSigma*40, 0.6, 4)
	st.SlantSigma = clampF(2.5+st.ScaleSigma*30, 1, 8)
	st.InkMean = clampF(meanOf(inks), 0.35, 1)
	st.InkSigma = clampF(stdev(inks), 0.02, 0.25)
	// WeightMean is a multiplier for the ink renderer (1 == as measured),
	// while Sprite.Weight keeps the physical stroke thickness in em units.
	st.WeightMean = 1
	st.WeightSigma = clampF(0.04+stdev(weights)*3, 0.03, 0.18)
	if total > 0 {
		// The share of glyphs whose ink ends in a connection-like tail. It is a
		// conservative prior: the renderer only joins two glyphs when both of
		// them have a join kind, and this probability gates that further.
		st.JoinProb = clampF(0.55+0.45*float64(joinable)/float64(total), 0, 0.95)
	}
	st.CharCount = len(glyphs)
	for _, g := range glyphs {
		st.VariantCount += len(g)
	}
	return st
}

// referenceAdvance returns the natural advance of a character in em units,
// measured on a bundled reference font. It stops punctuation from collapsing.
var (
	refOnce sync.Once
	refFont *sfnt.Font
	refAdv  map[string]float64
)

func referenceAdvance(ch string) (float64, bool) {
	refOnce.Do(func() {
		refAdv = map[string]float64{}
		data, ok := assets.FontData("caveat")
		if !ok {
			return
		}
		f, err := sfnt.Parse(data)
		if err != nil {
			return
		}
		refFont = f
		upem := float64(f.UnitsPerEm())
		var buf sfnt.Buffer
		for _, lang := range []string{"ru", "latin"} {
			for _, c := range Charset(lang) {
				gi, err := f.GlyphIndex(&buf, []rune(c)[0])
				if err != nil || gi == 0 {
					continue
				}
				adv, err := f.GlyphAdvance(&buf, gi, fixed.Int26_6(f.UnitsPerEm()*64), font.HintingNone)
				if err != nil {
					continue
				}
				refAdv[c] = float64(adv) / 64 / upem
			}
		}
	})
	v, ok := refAdv[ch]
	return v, ok
}

// graftMissing fills characters that the sample sheet did not cover with
// sprites rendered from a reference font, so the profile can render any text.
func graftMissing(built *[]*builtSprite, lang string, confidence float64) []string {
	have := map[string]bool{}
	for _, s := range *built {
		have[s.char] = true
	}
	data, ok := assets.FontData("caveat")
	if !ok {
		return nil
	}
	face, err := newTypeFace(data, spriteEm)
	if err != nil {
		return nil
	}
	defer face.close()
	var grafted []string
	for _, ch := range Charset(lang) {
		if have[ch] {
			continue
		}
		r := []rune(ch)
		if len(r) != 1 || !face.has(r[0]) {
			continue
		}
		rg, ok := face.renderGlyph(r[0])
		if !ok {
			continue
		}
		sp := spriteFromRaster(rg, spriteEm, ch)
		if sp == nil {
			continue
		}
		sp.confidence = confidence
		sp.variant = 0
		*built = append(*built, sp)
		grafted = append(grafted, ch)
		have[ch] = true
	}
	return grafted
}

// spriteFromRaster converts a font glyph into a built sprite.
func spriteFromRaster(rg *rasterGlyph, em float64, ch string) *builtSprite {
	if rg == nil || rg.ink.Empty() {
		return nil
	}
	mask := cropAlpha(rg.mask, rg.ink)
	box := model.Rect{
		X: (float64(rg.ink.Min.X) - rg.originX) / em,
		Y: (rg.baselineY - float64(rg.ink.Max.Y)) / em,
		W: float64(rg.ink.Dx()) / em,
		H: float64(rg.ink.Dy()) / em,
	}
	adv := rg.advance / em
	if adv < box.W {
		adv = box.W
	}
	join := computeJoin(mask, rg.baselineY-float64(rg.ink.Min.Y), em*float64(mask.Rect.Dy())/float64(rg.ink.Dy()), adv)
	return &builtSprite{
		char:       ch,
		mask:       mask,
		box:        box,
		advance:    adv,
		join:       join,
		ink:        rg.density,
		weight:     rg.weight / em,
		confidence: 0.9,
	}
}

// docLang guesses the language of a capture session from the labels.
func docLang(doc *model.CaptureDoc, labels map[int]string) string {
	cyr, lat := 0, 0
	for _, c := range doc.Cells {
		ch := labels[c.ID]
		if ch == "" {
			continue
		}
		for _, r := range ch {
			if r >= 'а' && r <= 'я' || r == 'ё' {
				cyr++
			} else if r >= 'a' && r <= 'z' {
				lat++
			}
		}
	}
	if lat > cyr {
		return "latin"
	}
	return "ru"
}

func joinStrings(v []string) string {
	out := ""
	for i, s := range v {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

func smoothstep(a, b, x float64) float64 {
	if b == a {
		return 0
	}
	t := clampF((x-a)/(b-a), 0, 1)
	return t * t * (3 - 2*t)
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
