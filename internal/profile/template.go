package profile

import (
	"fmt"
	"image"
	"image/color"
)

// Canonical sheet geometry. A sample sheet is A4 at 300 dpi so that a photo of
// a printed sheet can be mapped back to exact pixel coordinates.
const (
	sheetDPI       = 300
	sheetW         = 2480 // A4 width at 300 dpi
	sheetH         = 3508 // A4 height at 300 dpi
	sheetMargin    = 80
	sheetHeaderH   = 240
	sheetFooterH   = 70
	cellW          = 240
	cellH          = 300
	maxCols        = 9
	maxRows        = 10
	cellBaseline   = 0.75 // baseline position inside a cell (from the top)
	cellXHeight    = 0.46 // fallback x-height guide, relative to the guide em
	guideEmFrac    = 0.90 // guide em relative to the cell height
	markOffset     = 30   // registration mark distance from the grid corner
	markArm        = 80
	markThick      = 9
	sheetEmPerXH   = 0.48 // assumed x-height / em ratio when sizing a captured profile
	minXHeightCell = 5    // minimum x-only glyphs needed to size the capture
)

// sheetCell is one writing cell on a sample sheet.
type sheetCell struct {
	Row, Col int
	Rect     image.Rectangle
	Char     string // "" for unused trailing cells
}

// sheetLayout is the deterministic geometry shared by the printable sheet and
// the analyser, so a scan can be mapped back without guessing.
type sheetLayout struct {
	Lang     string
	Reps     int
	Page     int
	Pages    int
	Cols     int
	Rows     int
	Grid     image.Rectangle
	Em       float64 // guide em in px
	Baseline float64 // baseline y offset from a cell's top edge
	Cells    []sheetCell
	// Marks are the outer corners of the four registration L-marks in
	// canonical order: top-left, top-right, bottom-right, bottom-left.
	Marks [4][2]float64
}

// layoutFor builds the deterministic sheet layout for one page.
func layoutFor(lang string, reps, page int) *sheetLayout {
	if reps < 1 {
		reps = 1
	}
	if reps > maxCols {
		reps = maxCols
	}
	chars := Charset(lang)
	// Keep full repetitions of a character inside one row: a row is `reps`
	// cells wide per character, so the lattice width must divide by reps.
	cols := maxCols - maxCols%reps
	if cols < reps {
		cols = reps
	}
	rows := maxRows
	perPage := cols * rows / reps
	pages := (len(chars) + perPage - 1) / perPage
	if pages < 1 {
		pages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= pages {
		page = pages - 1
	}

	l := &sheetLayout{Lang: lang, Reps: reps, Page: page, Pages: pages, Cols: cols, Rows: rows}
	gw, gh := cols*cellW, rows*cellH
	availH := sheetH - 2*sheetMargin - sheetHeaderH - sheetFooterH
	gx := (sheetW - gw) / 2
	gy := sheetMargin + sheetHeaderH + (availH-gh)/2
	l.Grid = image.Rect(gx, gy, gx+gw, gy+gh)
	l.Baseline = cellBaseline * cellH
	l.Em = guideEmFrac * cellH

	// Fill the lattice row-major; every character occupies `reps` consecutive
	// cells so repeated samples become real glyph variants.
	idx := page * perPage
	seq := make([]string, 0, cols*rows)
	for ; idx < (page+1)*perPage && idx < len(chars); idx++ {
		for k := 0; k < reps; k++ {
			seq = append(seq, chars[idx])
		}
	}
	for len(seq) < cols*rows {
		seq = append(seq, "")
	}
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x0 := gx + c*cellW
			y0 := gy + r*cellH
			l.Cells = append(l.Cells, sheetCell{
				Row:  r,
				Col:  c,
				Rect: image.Rect(x0, y0, x0+cellW, y0+cellH),
				Char: seq[r*cols+c],
			})
		}
	}
	g := l.Grid
	l.Marks = [4][2]float64{
		{float64(g.Min.X - markOffset), float64(g.Min.Y - markOffset)},
		{float64(g.Max.X + markOffset), float64(g.Min.Y - markOffset)},
		{float64(g.Max.X + markOffset), float64(g.Max.Y + markOffset)},
		{float64(g.Min.X - markOffset), float64(g.Max.Y + markOffset)},
	}
	return l
}

// cellCentre returns the px centre of a cell.
func (c sheetCell) centre() (float64, float64) {
	return float64(c.Rect.Min.X+c.Rect.Max.X) / 2, float64(c.Rect.Min.Y+c.Rect.Max.Y) / 2
}

// guideBaseline returns the baseline y of a cell in px.
func (l *sheetLayout) guideBaseline(c sheetCell) float64 {
	return float64(c.Rect.Min.Y) + l.Baseline
}

// sheetColours keeps the printed sheet quiet so the ink is unambiguous.
var (
	colPaper      = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colGrid       = color.NRGBA{R: 186, G: 208, B: 229, A: 255}
	colGridDark   = color.NRGBA{R: 150, G: 182, B: 212, A: 255}
	colBaseline   = color.NRGBA{R: 168, G: 190, B: 214, A: 255}
	colXHeight    = color.NRGBA{R: 214, G: 226, B: 238, A: 255}
	colGuide      = color.NRGBA{R: 205, G: 205, B: 205, A: 255}
	colGuideFaint = color.NRGBA{R: 226, G: 226, B: 226, A: 255}
	colInk        = color.NRGBA{R: 20, G: 20, B: 24, A: 255}
	colText       = color.NRGBA{R: 58, G: 62, B: 70, A: 255}
	colTextFaint  = color.NRGBA{R: 150, G: 154, B: 162, A: 255}
)

// SheetPages renders the printable sample sheets for a language. Each page
// carries up to maxRows distinct characters repeated `reps` times; a full
// Russian charset needs 2-4 pages depending on reps.
func SheetPages(lang string, reps int) ([]*image.NRGBA, []SheetSpec, error) {
	if lang == "" {
		lang = "ru"
	}
	first := layoutFor(lang, reps, 0)
	pages := first.Pages
	out := make([]*image.NRGBA, 0, pages)
	specs := make([]SheetSpec, 0, pages)
	guide := uiFace(first.Em)
	defer guide.close()
	// A second, small face for sheet chrome.
	head := uiFace(46)
	defer head.close()
	small := uiFace(24)
	defer small.close()

	for p := 0; p < pages; p++ {
		l := first
		if p > 0 {
			l = layoutFor(lang, reps, p)
		}
		img := drawSheet(l, guide, head, small)
		chars := make([]string, len(l.Cells))
		for i, c := range l.Cells {
			chars[i] = c.Char
		}
		out = append(out, img)
		specs = append(specs, SheetSpec{
			Lang:  l.Lang,
			Reps:  l.Reps,
			Page:  l.Page,
			Pages: l.Pages,
			DPI:   sheetDPI,
			Chars: chars,
		})
	}
	return out, specs, nil
}

// repsHint explains how many repetitions a row asks for.
func repsHint(reps int) string {
	switch reps {
	case 1:
		return "Пропишите каждую букву один раз."
	case 2:
		return "Пропишите каждую букву два раза подряд."
	case 3:
		return "Пропишите каждую букву три раза подряд."
	}
	return fmt.Sprintf("Пропишите каждую букву %d раз подряд.", reps)
}

// drawSheet paints one sample sheet page.
func drawSheet(l *sheetLayout, guide, head, small *typeFace) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, sheetW, sheetH))
	fillRect(img, img.Bounds(), colPaper)

	if head != nil {
		head.draw(img, float64(sheetMargin), float64(sheetMargin)+46, "Пропишите каждую букву в своей клетке", colText)
	}
	if small != nil {
		small.draw(img, float64(sheetMargin), float64(sheetMargin)+86,
			fmt.Sprintf("Ручка: чёрная или синяя. %s Старайтесь не выходить за клетку.", repsHint(l.Reps)), colText)
		small.draw(img, float64(sheetMargin), float64(sheetMargin)+120,
			"Не старайтесь писать идеально ровно — наоборот, пишите так, как пишете обычно.", colTextFaint)
	}

	// Lattice.
	for r := 0; r < l.Rows; r++ {
		for c := 0; c < l.Cols; c++ {
			x0 := l.Grid.Min.X + c*cellW
			y0 := l.Grid.Min.Y + r*cellH
			cell := image.Rect(x0, y0, x0+cellW, y0+cellH)
			frame := colGrid
			if c == 0 {
				frame = colGridDark
			}
			drawLine(img, float64(cell.Min.X), float64(cell.Min.Y), float64(cell.Max.X), float64(cell.Min.Y), frame)
			drawLine(img, float64(cell.Min.X), float64(cell.Min.Y), float64(cell.Min.X), float64(cell.Max.Y), frame)
			drawLine(img, float64(cell.Min.X), float64(cell.Max.Y), float64(cell.Max.X), float64(cell.Max.Y), frame)
			drawLine(img, float64(cell.Max.X), float64(cell.Min.Y), float64(cell.Max.X), float64(cell.Max.Y), frame)
			drawLine(img, float64(cell.Min.X), float64(cell.Min.Y)+1, float64(cell.Max.X), float64(cell.Min.Y)+1, frame)
		}
	}

	// The x-height guide follows the reference font so the user sees how big
	// lowercase letters should be.
	xHeightPx := cellXHeight * l.Em
	if guide != nil {
		if xh := guide.xHeight(); xh > 0 {
			xHeightPx = xh
		}
	}
	// Per-cell guides: dashed baseline and x-height, plus a faint sample glyph
	// in the first cell of each character group (so repeats stay natural).
	for _, c := range l.Cells {
		if c.Char == "" {
			continue
		}
		base := l.guideBaseline(c)
		xh := base - xHeightPx
		drawDashedLine(img, float64(c.Rect.Min.X)+6, base, float64(c.Rect.Max.X)-6, base, 10, 7, colBaseline)
		drawDashedLine(img, float64(c.Rect.Min.X)+6, xh, float64(c.Rect.Max.X)-6, xh, 6, 9, colXHeight)
		if c.Col%l.Reps != 0 || guide == nil {
			continue
		}
		r := []rune(c.Char)[0]
		if !guide.has(r) {
			continue
		}
		g, ok := guide.renderGlyph(r)
		if !ok {
			continue
		}
		adv := guide.measure(c.Char)
		cx, _ := c.centre()
		col := colGuide
		if c.Col != 0 {
			col = colGuideFaint
		}
		drawMaskTinted(img, g, cx-adv/2, base, col)
	}

	// Registration marks: solid L shapes outside the lattice corners.
	g := l.Grid
	marks := [4][4]int{
		{g.Min.X - markOffset, g.Min.Y - markOffset, 1, 1},   // TL arms right/down
		{g.Max.X + markOffset, g.Min.Y - markOffset, -1, 1},  // TR arms left/down
		{g.Max.X + markOffset, g.Max.Y + markOffset, -1, -1}, // BR arms left/up
		{g.Min.X - markOffset, g.Max.Y + markOffset, 1, -1},  // BL arms right/up
	}
	for _, m := range marks {
		x, y, sx, sy := m[0], m[1], m[2], m[3]
		// Arms extend towards the page centre and the thickness grows on the
		// same inward side, so the anchor stays the bbox corner nearest the
		// page corner. Detection relies on that.
		hx0, hx1 := sortedInts(x, x+sx*markArm)
		hy0, hy1 := sortedInts(y, y+sy*markThick)
		fillRect(img, image.Rect(hx0, hy0, hx1, hy1), colInk)
		vx0, vx1 := sortedInts(x, x+sx*markThick)
		vy0, vy1 := sortedInts(y, y+sy*markArm)
		fillRect(img, image.Rect(vx0, vy0, vx1, vy1), colInk)
	}

	// Footer: machine readable page tag + human hint.
	if small != nil {
		tag := fmt.Sprintf("TYPESHIT %s reps=%d page=%d/%d", l.Lang, l.Reps, l.Page+1, l.Pages)
		small.draw(img, float64(sheetMargin), float64(sheetH-sheetFooterH/2), tag, colTextFaint)
	}
	return img
}

// drawMaskTinted draws a rasterised glyph so that its pen origin lands on x
// and its baseline on baseline, using the mask's own baked-in geometry.
func drawMaskTinted(dst *image.NRGBA, g *rasterGlyph, x, baseline float64, c color.NRGBA) {
	blitMask(dst, g.mask, int(x-g.originX+0.5), int(baseline-g.baselineY+0.5), c)
}

// blitMask composites an alpha mask onto dst at (x,y) over the destination.
func blitMask(dst *image.NRGBA, mask *image.Alpha, x, y int, c color.NRGBA) {
	mb := mask.Bounds()
	db := dst.Bounds()
	for my := mb.Min.Y; my < mb.Max.Y; my++ {
		ty := y + (my - mb.Min.Y)
		if ty < db.Min.Y || ty >= db.Max.Y {
			continue
		}
		mrow := mask.Pix[my*mask.Stride:]
		drow := dst.Pix[ty*dst.Stride:]
		for mx := mb.Min.X; mx < mb.Max.X; mx++ {
			tx := x + (mx - mb.Min.X)
			if tx < db.Min.X || tx >= db.Max.X {
				continue
			}
			a := float64(mrow[mx]) / 255
			if a == 0 {
				continue
			}
			o := tx * 4
			drow[o+0] = uint8(float64(drow[o+0])*(1-a) + float64(c.R)*a)
			drow[o+1] = uint8(float64(drow[o+1])*(1-a) + float64(c.G)*a)
			drow[o+2] = uint8(float64(drow[o+2])*(1-a) + float64(c.B)*a)
			drow[o+3] = 255
		}
	}
}
