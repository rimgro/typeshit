package profile

import (
	"image"
	"math"
)

// Corner bits of a component bounding box.
const (
	cornerTL = 1 << 0
	cornerTR = 1 << 1
	cornerBR = 1 << 2
	cornerBL = 1 << 3
)

// findMarks locates the four solid registration L-marks printed at the sheet
// corners. It returns the outer corner of each mark in image coordinates,
// ordered top-left, top-right, bottom-right, bottom-left.
//
// The L shape is identified by its corner signature: three bbox corners carry
// ink and the one pointing into the page is empty. That rejects text, blots and
// stray strokes far more reliably than a size/aspect test alone.
func findMarks(g *grayImage) ([4]image.Point, bool) {
	var out [4]image.Point
	mask := adaptiveMask(g, float64(maxInt(g.w/40, 6)))
	comps := components(mask)
	maxDim := float64(maxInt(g.w, g.h))
	best := [4]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(1)}
	found := [4]bool{}
	// The empty corner expected for each image corner: TL mark -> BR empty.
	emptyFor := [4]uint8{cornerBR, cornerBL, cornerTL, cornerTR}
	corners := [4][2]float64{{0, 0}, {float64(g.w), 0}, {float64(g.w), float64(g.h)}, {0, float64(g.h)}}
	for _, c := range comps {
		w, h := float64(c.w()), float64(c.h())
		side := math.Max(w, h)
		if float64(c.Area) < 120 || math.Min(w, h) < 8 {
			continue
		}
		if side/math.Max(1, math.Min(w, h)) > 2.2 {
			continue
		}
		fill := float64(c.Area) / (w * h)
		if fill < 0.08 || fill > 0.5 {
			continue
		}
		if side < maxDim*0.012 || side > maxDim*0.07 {
			continue
		}
		sig, ok := cornerSignature(mask, c)
		if !ok {
			continue
		}
		for k := 0; k < 4; k++ {
			if sig != (cornerTL|cornerTR|cornerBR|cornerBL)&^emptyFor[k] {
				continue
			}
			// Anchor: the bbox corner closest to the image corner.
			ax, ay := float64(c.MinX), float64(c.MinY)
			if k == 1 || k == 2 {
				ax = float64(c.MaxX)
			}
			if k >= 2 {
				ay = float64(c.MaxY)
			}
			d := dist(ax, ay, corners[k][0], corners[k][1])
			if d > maxDim*0.45 {
				continue
			}
			if d < best[k] {
				best[k] = d
				out[k] = image.Pt(int(ax), int(ay))
				found[k] = true
			}
		}
	}
	for _, f := range found {
		if !f {
			return out, false
		}
	}
	// Reject quads whose aspect is nowhere near a page.
	dTop := dist(float64(out[0].X), float64(out[0].Y), float64(out[1].X), float64(out[1].Y))
	dLeft := dist(float64(out[0].X), float64(out[0].Y), float64(out[3].X), float64(out[3].Y))
	if dLeft == 0 {
		return out, false
	}
	if r := dTop / dLeft; r < 0.4 || r > 2.5 {
		return out, false
	}
	return out, true
}

// cornerSignature reports which bbox corners of a component carry ink.
func cornerSignature(m *bitmap, c component) (uint8, bool) {
	w, h := c.w(), c.h()
	k := minInt(w, h) / 8
	if k < 2 {
		k = 2
	}
	probe := func(x0, y0 int) bool {
		on, total := 0, 0
		for y := y0; y < y0+k; y++ {
			for x := x0; x < x0+k; x++ {
				total++
				if m.at(x, y) {
					on++
				}
			}
		}
		return total > 0 && float64(on)/float64(total) > 0.55
	}
	var sig uint8
	if probe(c.MinX, c.MinY) {
		sig |= cornerTL
	}
	if probe(c.MaxX-k+1, c.MinY) {
		sig |= cornerTR
	}
	if probe(c.MaxX-k+1, c.MaxY-k+1) {
		sig |= cornerBR
	}
	if probe(c.MinX, c.MaxY-k+1) {
		sig |= cornerBL
	}
	if sig == 0 || sig == cornerTL|cornerTR|cornerBR|cornerBL {
		return 0, false
	}
	return sig, true
}

// orientQuad resolves how the sheet lies in the photo (0/90/180/270 degrees of
// rotation) by scoring how much printed header ink lands in the canonical
// header band. The page header is asymmetric, which makes this reliable even
// though the lattice itself is centred.
func orientQuad(g *grayImage, quad [4]image.Point, l *sheetLayout) ([4][2]float64, int) {
	qs := [4][2]float64{}
	for i, p := range quad {
		qs[i] = [2]float64{float64(p.X), float64(p.Y)}
	}
	// A camera can hand the corners back in either winding. A mirrored
	// homography would flip every letter, so normalise the winding first.
	if quadSignedArea(qs) < 0 {
		qs = [4][2]float64{qs[0], qs[3], qs[2], qs[1]}
	}
	canon := [4][2]float64{l.Marks[0], l.Marks[1], l.Marks[2], l.Marks[3]}
	bestScore := math.Inf(-1)
	best := 0
	var bestQuad [4][2]float64
	for k := 0; k < 4; k++ {
		cand := [4][2]float64{qs[k%4], qs[(k+1)%4], qs[(k+2)%4], qs[(k+3)%4]}
		h, err := quadToQuad(canon, cand)
		if err != nil || !saneTransform(h, g.w, g.h) {
			continue
		}
		sc := headerScore(g, h, cand)
		// Prefer the orientation we already have when the scores are close:
		// a filled sheet is dense everywhere, so the title band is a weak
		// signal and must not flip an upright page.
		if k == 0 {
			sc += 0.004
		}
		if sc > bestScore {
			bestScore, best, bestQuad = sc, k, cand
		}
	}
	if bestScore == math.Inf(-1) {
		return qs, 0
	}
	return bestQuad, best
}

// quadSignedArea returns the shoelace area of a quad in image coordinates.
func quadSignedArea(q [4][2]float64) float64 {
	sum := 0.0
	for i := range q {
		j := (i + 1) % 4
		sum += q[i][0]*q[j][1] - q[j][0]*q[i][1]
	}
	return sum / 2
}

// headerScore measures the dark-pixel density in the canonical title band
// minus the density in the footer band, using the given canonical→photo map.
// Samples that land outside the page (on the desk around it, which is usually
// much darker than paper) are ignored, otherwise a rotated candidate would win
// simply because it maps the margins onto the desk.
func headerScore(g *grayImage, h homography, page [4][2]float64) float64 {
	// The bands are kept inside the page margins on purpose: the lattice is
	// full of the writer's ink, which is denser than any printed chrome and
	// would otherwise win the vote for a 180-degree flipped page.
	headerInk := bandInk(g, h, page, 70, 40, sheetW-70, 200)
	footerInk := bandInk(g, h, page, 70, sheetH-120, sheetW-70, sheetH-30)
	return headerInk - footerInk*0.6
}

// bandInk samples a canonical rectangle and returns the fraction of dark pixels.
func bandInk(g *grayImage, h homography, page [4][2]float64, x0, y0, x1, y1 float64) float64 {
	const n = 24
	dark := 0
	total := 0
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			cx := x0 + (x1-x0)*(float64(i)+0.5)/n
			cy := y0 + (y1-y0)*(float64(j)+0.5)/n
			px, py := h.apply(cx, cy)
			if px < 0 || py < 0 || px >= float64(g.w) || py >= float64(g.h) {
				continue
			}
			if !pointInQuad(page, px, py) {
				continue
			}
			total++
			if g.at(int(px), int(py)) < 0.62 {
				dark++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(dark) / float64(total)
}

// pointInQuad reports whether (x,y) lies inside the convex quad (TL,TR,BR,BL).
func pointInQuad(q [4][2]float64, x, y float64) bool {
	sign := 0.0
	for i := range q {
		j := (i + 1) % 4
		cross := (q[j][0]-q[i][0])*(y-q[i][1]) - (q[j][1]-q[i][1])*(x-q[i][0])
		if cross == 0 {
			continue
		}
		if sign == 0 {
			sign = cross
		} else if (cross > 0) != (sign > 0) {
			return false
		}
	}
	return true
}

// saneTransform rejects homographies that would fold or wildly rescale the page.
func saneTransform(h homography, w, hgt int) bool {
	area := 0.0
	prevX, prevY := 0.0, 0.0
	pts := [4][2]float64{{0, 0}, {sheetW, 0}, {sheetW, sheetH}, {0, sheetH}}
	for i, p := range pts {
		x, y := h.apply(p[0], p[1])
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			return false
		}
		if x < -0.25*float64(w) || y < -0.25*float64(hgt) || x > 1.25*float64(w) || y > 1.25*float64(hgt) {
			return false
		}
		if i > 0 {
			area += prevX*y - x*prevY
		}
		prevX, prevY = x, y
	}
	area += prevX*pts[0][1] - pts[0][0]*prevY
	area = math.Abs(area) / 2
	imgArea := float64(w) * float64(hgt)
	return area > imgArea*0.15 && area < imgArea*4
}

// detectionScale is the long side of the downscaled copy used for structural
// detection. It is plenty for 40 px marks and keeps the analyser fast.
const detectionScale = 1200

// templateTransform computes canonical→photo coordinates for a printed sheet.
func templateTransform(work *image.NRGBA, l *sheetLayout, a *analyzer) homography {
	small, g, scale := detectionCopy(work)
	canon := [4][2]float64{l.Marks[0], l.Marks[1], l.Marks[2], l.Marks[3]}

	if quadSmall, ok := findMarks(g); ok {
		oriented, rotation := orientQuad(g, quadSmall, l)
		if rotation == 0 {
			a.note = append(a.note, "Страница выровнена по печатным меткам.")
		} else {
			a.note = append(a.note, "Фото было повёрнуто, страница выровнена автоматически.")
		}
		var quad [4][2]float64
		for i, p := range oriented {
			quad[i] = [2]float64{p[0] / scale, p[1] / scale}
		}
		if h, err := quadToQuad(canon, quad); err == nil && saneTransform(h, work.Rect.Dx(), work.Rect.Dy()) {
			return h
		}
	}

	if h, ok := gridBBoxTransform(small, g, l, scale); ok {
		a.note = append(a.note, "Метки не найдены: выравнивание по границам сетки. Проверьте разметку.")
		return h
	}

	a.note = append(a.note, "Шаблон не распознан: страница вписана целиком. Разметку придётся поправить вручную.")
	return photoTransform(work)
}

// detectionCopy returns a downscaled copy of the photo, its flattened greyscale
// and the scale factor (small = work * scale).
func detectionCopy(work *image.NRGBA) (*image.NRGBA, *grayImage, float64) {
	long := maxInt(work.Rect.Dx(), work.Rect.Dy())
	if long <= detectionScale {
		g := flattenIllumination(toGray(work), maxInt(work.Rect.Dx()/20, 8))
		return work, g, 1
	}
	scale := float64(detectionScale) / float64(long)
	small := resizeFast(work, int(float64(work.Rect.Dx())*scale+0.5), int(float64(work.Rect.Dy())*scale+0.5))
	g := flattenIllumination(toGray(small), maxInt(small.Rect.Dx()/20, 8))
	return small, g, scale
}

// scaleHomography post-multiplies a transform with a uniform scale.
func scaleHomography(h homography, a float64) homography {
	return homography{{a, 0, 0}, {0, a, 0}, {0, 0, 1}}.mul(h)
}

// gridBBoxTransform falls back to fitting the printed lattice bounding box.
func gridBBoxTransform(small *image.NRGBA, g *grayImage, l *sheetLayout, scale float64) (homography, bool) {
	toWork := func(h homography) (homography, bool) { return scaleHomography(h, 1/scale), true }
	if h, ok := gridBBoxBlue(small, l); ok {
		return toWork(h)
	}
	return gridBBoxDark(g, l, scale, toWork)
}

// gridBBoxBlue finds the light blue rulings and fits the lattice to their box.
func gridBBoxBlue(small *image.NRGBA, l *sheetLayout) (homography, bool) {
	b := small.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	count := 0
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		row := small.Pix[y*small.Stride:]
		for x := b.Min.X; x < b.Max.X; x += 2 {
			o := x * 4
			r, bb := int(row[o]), int(row[o+2])
			if bb-r < 8 || bb < 120 || r > 245 {
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
			count++
		}
	}
	if count < 300 {
		return identity, false
	}
	return fitRects(l.Grid, image.Rect(minX, minY, maxX, maxY))
}

// gridBBoxDark fits the whole dark content area (monochrome photos).
func gridBBoxDark(g *grayImage, l *sheetLayout, scale float64, toWork func(homography) (homography, bool)) (homography, bool) {
	mask := adaptiveMask(g, float64(maxInt(g.w/40, 6)))
	bb := inkBoundsIn(mask, maskRect(mask), 400)
	if bb.Empty() || bb.Dx() < 100 || bb.Dy() < 100 {
		return identity, false
	}
	content := l.Grid.Inset(-(markOffset + markArm))
	h, ok := fitRects(content, bb)
	if !ok {
		return identity, false
	}
	return toWork(h)
}

// fitRects builds the axis-aligned scale+offset map from src to dst.
func fitRects(src, dst image.Rectangle) (homography, bool) {
	if src.Dx() <= 0 || src.Dy() <= 0 || dst.Dx() <= 0 || dst.Dy() <= 0 {
		return identity, false
	}
	sx := float64(dst.Dx()) / float64(src.Dx())
	sy := float64(dst.Dy()) / float64(src.Dy())
	return affine(sx, sy, 0, 0, float64(dst.Min.X)-float64(src.Min.X)*sx, float64(dst.Min.Y)-float64(src.Min.Y)*sy), true
}

// photoTransform places a free-form photo centred inside the canonical page and
// returns the canonical→photo map (the inverse of the placement).
func photoTransform(work *image.NRGBA) homography {
	sw, sh := float64(work.Rect.Dx()), float64(work.Rect.Dy())
	s := math.Min(float64(sheetW)/sw, float64(sheetH)/sh) // canonical px per photo px
	nw, nh := sw*s, sh*s
	ox := (float64(sheetW) - nw) / 2
	oy := (float64(sheetH) - nh) / 2
	return affine(1/s, 1/s, 0, 0, -ox/s, -oy/s)
}
