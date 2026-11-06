// Package layout turns text into positioned glyph operations with human
// variability: per-glyph transforms, cursive links, drift and mistakes.
package layout

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"unicode"

	"typeshit/internal/model"
)

const (
	defaultWordGap  = 0.56 // em
	defaultLineGap  = 1.45 // em, ruling pitch / em
	defaultIndent   = 2.2  // em
	minEm           = 8.0
	maxEm           = 420.0
	targetCharsLine = 48.0 // used to derive em on blank paper
)

func orF(v, def float64) float64 {
	if v > 0 {
		return v
	}
	return def
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

// params is the resolved, request-specific typographic state.
type params struct {
	em        float64
	linePitch float64

	slant, slantSigma, rotSigma, scaleSigma float64
	dySigma, dxSigma                        float64
	advScale, wordGap, joinProb, liftProb   float64
	inkMean, inkSigma                       float64
	weightMean, weightSigma                 float64
	variability                             float64

	ascent, descent, maxH float64 // em
	meanAdv               map[rune]float64
	alt                   map[rune]rune

	x0, x1        float64 // content left/right px
	colW, colGap  float64
	cols          int
	indentEm      float64
	lined         bool
	cellPx        float64
	firstBaseline float64
	maxBaseline   float64
	pageW, pageH  int
	marginSide    float64
}

type driftState struct {
	baseline float64 // px, current walking offset
	slant    float64 // deg
	scale    float64 // relative
	phase    float64
}

type lineCtx struct {
	baseline float64 // effective baseline (already includes drift)
	slant    float64 // effective slant in degrees
	scale    float64 // effective scale multiplier
	ink      float64 // base ink amount
	emPx     float64
}

type lineRect struct{ x0, y0, x1, y1 float64 }

// builder is the mutable state of one Build call.
type builder struct {
	req  *model.GenerateRequest
	prof *model.Profile
	rng  *rand.Rand
	p    *params

	pages []*model.Page
	page  *model.Page

	col       int
	y         float64 // baseline of the next line
	pageLine  int     // lines emitted on the current page
	lineIndex int     // lines emitted overall

	drift     driftState
	recent    map[rune][]int
	lines     map[int][]lineRect
	flaws     *flawPlan
	freshPage bool // the next emitted line starts a page/column
}

// Build paginates the request text and returns the pages to rasterise.
func Build(req *model.GenerateRequest, prof *model.Profile, rng *rand.Rand) ([]*model.Page, error) {
	if req == nil || prof == nil || rng == nil {
		return nil, errors.New("layout: nil request, profile or rng")
	}
	if len(prof.Glyphs) == 0 {
		return nil, errors.New("layout: profile has no glyphs")
	}
	p, err := resolveParams(req, prof)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Text) == "" && strings.TrimSpace(req.Title) == "" {
		return nil, nil
	}

	b := &builder{
		req:    req,
		prof:   prof,
		rng:    rng,
		p:      p,
		y:      p.firstBaseline,
		recent: map[rune][]int{},
		lines:  map[int][]lineRect{},
		drift:  driftState{phase: rng.Float64() * 6.283},
	}
	b.y += float64(maxInt(req.Page.SkipLines, 0)) * p.linePitch
	b.flaws = newFlawPlan(req.Flaws, rng)

	if strings.TrimSpace(req.Title) != "" {
		b.emitTitle(strings.TrimSpace(req.Title))
	}

	for _, srcLine := range strings.Split(strings.ReplaceAll(req.Text, "\r\n", "\n"), "\n") {
		words := splitParagraph(srcLine)
		if len(words) == 0 {
			b.skipLine()
			continue
		}
		b.flowParagraph(words)
	}

	if b.page != nil {
		b.finishPage(b.page)
	}
	if len(b.pages) == 0 {
		return nil, nil
	}
	return b.pages, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------- parameter resolution ----------

func resolveParams(req *model.GenerateRequest, prof *model.Profile) (*params, error) {
	p := &params{alt: buildAlt()}
	st := prof.Style

	// Page geometry.
	p.pageW = req.Page.Width
	if p.pageW <= 0 {
		p.pageW = 1240
	}
	p.pageH = req.Page.Height
	if p.pageH <= 0 {
		p.pageH = 1754
	}
	p.cols = maxInt(req.Page.Columns, 1)
	if p.cols > 4 {
		p.cols = 4
	}
	p.marginSide = req.Page.MarginSide
	if p.marginSide <= 0 {
		p.marginSide = 110
	}
	marginTop := req.Page.MarginTop
	if marginTop <= 0 {
		marginTop = 90
	}
	marginBot := req.Page.MarginBot
	if marginBot <= 0 {
		marginBot = 90
	}
	p.x0 = p.marginSide
	p.x1 = float64(p.pageW) - p.marginSide
	if p.x1-p.x0 < 80 {
		p.x0, p.x1 = 20, float64(p.pageW)-20
	}

	// Glyph statistics.
	p.meanAdv = make(map[rune]float64, len(prof.Glyphs))
	var advSum float64
	var advCount int
	p.ascent, p.descent, p.maxH = 0, 0, 0
	for key, list := range prof.Glyphs {
		if len(list) == 0 {
			continue
		}
		ch, _ := firstRune(key)
		var sum float64
		for i := range list {
			sp := &list[i]
			a := spriteAdvance(sp)
			sum += a
			advSum += a
			advCount++
			top := sp.Box.Y + sp.Box.H
			if top > p.ascent {
				p.ascent = top
			}
			if sp.Box.Y < p.descent {
				p.descent = sp.Box.Y
			}
			if sp.Box.H > p.maxH {
				p.maxH = sp.Box.H
			}
		}
		p.meanAdv[ch] = sum / float64(len(list))
	}
	if advCount == 0 {
		return nil, errors.New("layout: profile has no usable sprites")
	}
	globalMean := advSum / float64(advCount)
	if globalMean < 0.18 || globalMean > 1.6 {
		globalMean = clampF(globalMean, 0.18, 1.6)
	}
	p.ascent = clampF(p.ascent, 0.5, 1.7)
	if p.descent > -0.04 {
		p.descent = -0.2
	}
	p.descent = clampF(-p.descent, 0.05, 0.7)
	if p.maxH < 0.25 {
		p.maxH = 0.8
	}
	p.maxH = clampF(p.maxH, 0.3, 1.8)

	// Style knobs with measured-else-default semantics.
	p.variability = req.Style.Variability
	if p.variability <= 0 {
		p.variability = 1
	}
	p.variability = clampF(p.variability, 0, 3)
	v := p.variability

	p.slant = st.Slant
	if req.Style.Slant != nil {
		p.slant = *req.Style.Slant
	}
	p.slant = clampF(p.slant, -25, 35)
	p.slantSigma = clampF(orF(st.SlantSigma, 1.1)*v, 0, 6)
	p.rotSigma = clampF(orF(st.RotSigma, 0.9)*v, 0, 6)
	p.scaleSigma = clampF(orF(st.ScaleSigma, 0.035)*v, 0, 0.16)
	p.dySigma = clampF(orF(st.DySigma, 0.018)*v, 0, 0.08)
	p.dxSigma = clampF(orF(st.DxSigma, 0.032)*v, 0, 0.14)
	p.advScale = clampF(orF(st.AdvanceScale, 1), 0.7, 1.4)
	p.weightMean = clampF(orF(st.WeightMean, 1), 0.4, 2)
	p.weightSigma = clampF(orF(st.WeightSigma, 0.08)*v, 0, 0.4)
	p.inkMean = clampF(orF(st.InkMean, 0.94), 0.3, 1)
	p.inkSigma = clampF(orF(st.InkSigma, 0.05)*v, 0, 0.3)
	p.linePitch = 0 // resolved below

	p.wordGap = clampF(orF(st.WordGap, defaultWordGap), 0.2, 1.4)
	if req.Style.WordGap != nil {
		p.wordGap = clampF(*req.Style.WordGap, 0.05, 3)
	}
	p.joinProb = clampF(orF(st.JoinProb, 0.85), 0, 1)
	if req.Style.Join != nil {
		p.joinProb = clampF(*req.Style.Join, 0, 1)
	}
	p.liftProb = clampF(orF(st.LiftProb, 0.04), 0, 0.5)

	// Writing size.
	lineGapRatio := clampF(orF(st.LineGap, defaultLineGap), 1.12, 3.6)
	p.lined = req.Paper.Kind == "lined" || req.Paper.Kind == "grid"
	cellMM := orF(req.Paper.CellMM, 8)
	if cellMM < 3 {
		cellMM = 3
	}
	dpi := 25.4 * float64(p.pageW) / 210.0
	p.cellPx = cellMM * dpi / 25.4

	em := 0.0
	for iter := 0; iter < 2; iter++ {
		gap := 0.0
		if p.cols > 1 {
			gap = 1.5 * em
			if gap < 18 {
				gap = 18
			}
		}
		contentW := p.x1 - p.x0 - float64(p.cols-1)*gap
		colW := contentW / float64(p.cols)
		if colW < 60 {
			colW = 60
		}
		if p.lined {
			// On ruled paper the glyph extent (ascender..descender) should fill
			// most of one ruling pitch, like real handwriting does.
			em = p.cellPx / (p.maxH * 1.04)
		} else {
			em = colW / (targetCharsLine * clampF(globalMean, 0.25, 0.8))
			// Respect the page height: keep at least 8 lines on a real page.
			// Very short canvases (profile previews) are exempt, otherwise the
			// text would shrink to a few pixels.
			maxLinesEm := (float64(p.pageH) - marginTop - marginBot) / (8 * lineGapRatio)
			if em > maxLinesEm && maxLinesEm > 12 {
				em = maxLinesEm
			}
		}
	}
	if req.Style.Scale >= 4 { // absolute px per em
		em = req.Style.Scale
	} else if req.Style.Scale > 0 { // multiplier of the automatic size
		em *= req.Style.Scale
	}
	em = clampF(em, minEm, maxEm)
	if p.lined {
		// Keep every line on a ruling.
		em = math.Min(em, p.cellPx/(p.maxH*1.08))
		em = clampF(em, minEm, maxEm)
	}
	p.em = em

	colGap := 1.5 * em
	if colGap < 18 {
		colGap = 18
	}
	p.colGap = colGap
	p.colW = (p.x1 - p.x0 - float64(p.cols-1)*colGap) / float64(p.cols)

	// Respect the printed margin rule: nobody writes across it.
	if req.Paper.MarginMM > 0 {
		mmPx := req.Paper.MarginMM * dpi / 25.4
		pad := 0.35 * p.em
		switch strings.ToLower(req.Paper.MarginSide) {
		case "left":
			p.x0 = math.Max(p.x0, mmPx+pad)
		case "right":
			p.x1 = math.Min(p.x1, float64(p.pageW)-mmPx-pad)
		case "both":
			p.x0 = math.Max(p.x0, mmPx+pad)
			p.x1 = math.Min(p.x1, float64(p.pageW)-mmPx-pad)
		}
		if p.x1-p.x0 > 60 {
			p.colW = (p.x1 - p.x0 - float64(p.cols-1)*colGap) / float64(p.cols)
		} else {
			p.x0, p.x1 = p.marginSide, float64(p.pageW)-p.marginSide
			p.colW = (p.x1 - p.x0 - float64(p.cols-1)*colGap) / float64(p.cols)
		}
	}

	// Ruled paper: the pitch is fixed by the ruling, so the writing size is
	// derived from the glyph extent instead of the style line gap.
	p.linePitch = lineGapRatio * em
	if p.lined {
		p.linePitch = p.cellPx
		if p.maxH*em > p.cellPx*0.98 {
			em = p.cellPx / (p.maxH * 1.04)
			p.em = em
		}
	}
	if req.Page.LineHeight > 0 {
		p.linePitch = req.Page.LineHeight * em
		if p.lined {
			n := math.Round(p.linePitch / p.cellPx)
			if n < 1 {
				n = 1
			}
			p.linePitch = n * p.cellPx
		}
	}
	minPitch := p.maxH * em * 1.06
	if p.lined && minPitch > p.cellPx {
		// Shrink the glyphs rather than leaving the rulings.
		em = p.cellPx / (p.maxH * 1.06)
		p.em = em
		p.linePitch = p.cellPx
		minPitch = p.maxH * em * 1.06
	}
	if p.linePitch < minPitch {
		p.linePitch = minPitch
	}

	p.indentEm = defaultIndent
	if req.Page.Indent > 0 {
		p.indentEm = req.Page.Indent
	}

	// Vertical placement.
	if p.lined {
		k := int(math.Ceil((marginTop + p.ascent*em*0.65) / p.cellPx))
		if k < 1 {
			k = 1
		}
		p.firstBaseline = float64(k) * p.cellPx
		p.maxBaseline = float64(p.pageH) - marginBot - p.descent*em*0.45
	} else {
		p.firstBaseline = marginTop + p.ascent*em*0.9
		p.maxBaseline = float64(p.pageH) - marginBot - p.descent*em*0.5
	}
	if p.maxBaseline < p.firstBaseline+p.linePitch {
		p.maxBaseline = p.firstBaseline + p.linePitch
	}
	return p, nil
}

func spriteAdvance(sp *model.Sprite) float64 {
	a := sp.Advance
	if a <= 0 {
		a = sp.Box.W + 0.08
	}
	if sp.Join.Kind != "none" && sp.Join.Kind != "" {
		// Joined sprites already contain the link ink; keep a hair of extra
		// room so links do not swallow the next glyph.
		a += 0.01
	}
	return clampF(a, 0.12, 3)
}

func buildAlt() map[rune]rune {
	pairs := [][2]rune{
		{'ё', 'е'}, {'е', 'ё'}, {'й', 'и'}, {'ъ', 'ь'}, {'ь', 'ъ'},
		{'o', 'о'}, {'о', 'o'}, {'a', 'а'}, {'а', 'a'}, {'e', 'е'},
		{'с', 'c'}, {'c', 'с'}, {'p', 'р'}, {'р', 'p'}, {'x', 'х'},
		{'х', 'x'}, {'y', 'у'}, {'у', 'y'}, {'k', 'к'}, {'к', 'k'},
		{'m', 'м'}, {'м', 'm'}, {'T', 'Т'}, {'Н', 'H'}, {'В', 'B'},
		{'С', 'C'}, {'М', 'M'}, {'А', 'A'}, {'К', 'K'}, {'Е', 'E'},
		{'О', 'O'}, {'Р', 'P'}, {'Х', 'X'}, {'—', '-'}, {'–', '-'},
		{'‘', '\''}, {'’', '\''}, {'“', '"'}, {'”', '"'}, {'„', '"'},
		{'…', '.'}, {'·', '.'}, {'\u00a0', ' '},
	}
	m := make(map[rune]rune, len(pairs))
	for _, p := range pairs {
		m[p[0]] = p[1]
	}
	return m
}

// ---------- vertical flow ----------

func (b *builder) colLeft() float64 {
	return b.p.x0 + float64(b.col)*(b.p.colW+b.p.colGap)
}

func (b *builder) ensurePage() {
	if b.page != nil {
		return
	}
	b.page = &model.Page{
		Index:  len(b.pages),
		Width:  b.p.pageW,
		Height: b.p.pageH,
	}
	b.pages = append(b.pages, b.page)
	b.pageLine = 0
	b.freshPage = true
}

func (b *builder) skipLine() {
	b.y += b.p.linePitch
	b.pageLine++
	if b.y > b.p.maxBaseline+0.5 {
		b.nextColumn()
	}
}

func (b *builder) nextColumn() {
	b.col++
	if b.col >= b.p.cols {
		b.col = 0
		b.y = b.p.firstBaseline
		if b.page != nil {
			b.finishPage(b.page)
			b.page = nil
		}
		b.freshPage = true
		return
	}
	b.y = b.p.firstBaseline
	b.freshPage = true
}

func (b *builder) advanceLine() {
	b.y += b.p.linePitch
	b.pageLine++
	b.lineIndex++

	// Human drift: an AR(1) random walk plus a slow sine. On ruled paper the
	// text has to stay on the printed lines, so the amplitude is much smaller
	// than on blank sheets.
	amp := b.p.linePitch * 0.05
	driftCap := 0.16
	if b.p.lined {
		amp *= 0.55
		driftCap = 0.07
	}
	if b.flaws != nil {
		amp *= 1 + 0.8*b.flaws.wobble
	}
	b.drift.baseline = b.drift.baseline*0.74 + b.rng.NormFloat64()*amp*0.45
	b.drift.baseline = clampF(b.drift.baseline, -b.p.linePitch*driftCap, b.p.linePitch*driftCap)
	b.drift.slant = b.drift.slant*0.8 + b.rng.NormFloat64()*b.p.slantSigma*0.06
	b.drift.slant = clampF(b.drift.slant, -3.5, 3.5)
	b.drift.scale = b.drift.scale*0.85 + b.rng.NormFloat64()*b.p.scaleSigma*0.02
	b.drift.scale = clampF(b.drift.scale, -0.05, 0.05)

	if b.y > b.p.maxBaseline+0.5 {
		b.nextColumn()
	}
}

// lineContext computes the per-line typographic jitter.
func (b *builder) lineContext() lineCtx {
	n := float64(b.lineIndex)
	wave := math.Sin(n*0.72+b.drift.phase) + 0.45*math.Sin(n*0.23+b.drift.phase*1.7)
	baseline := b.y + b.drift.baseline + wave*b.p.linePitch*0.012*(1+1.5*b.flaws.wobble)
	return lineCtx{
		baseline: baseline,
		slant:    clampF(b.p.slant+b.drift.slant+wave*0.35, -30, 40),
		scale:    clampF(1+b.drift.scale+wave*0.006, 0.9, 1.1),
		ink:      b.p.inkMean,
		emPx:     b.p.em,
	}
}

// ---------- horizontal flow ----------

// availableEm is the usable line width in em. It keeps a small safety margin:
// wrapping is measured with mean advances, but the emitted advances carry
// jitter, so a tight fit would occasionally poke past the right margin.
func (b *builder) availableEm(indentEm float64) float64 {
	return (b.p.colW - indentEm*b.p.em - 0.45*b.p.em) / b.p.em
}

func (b *builder) flowParagraph(words []string) {
	indent := b.p.indentEm
	first := true
	var parts []part
	var width float64

	for len(words) > 0 {
		avail := b.availableEm(indentFor(first, indent))
		w := words[0]
		ww := b.measure(w)

		if len(parts) == 0 {
			if ww > avail {
				if head, tail, ok := b.hyphenate(w, avail); ok && head != "" && tail != "" {
					b.flushLine([]part{{text: head}}, indentFor(first, indent), false)
					first = false
					words[0] = tail
					continue
				}
				head, tail := b.hardBreak(w, avail)
				if head != "" && tail != "" {
					b.flushLine([]part{{text: head}}, indentFor(first, indent), false)
					first = false
					words[0] = tail
					continue
				}
			}
			parts = append(parts, part{text: w})
			width = ww
			words = words[1:]
			continue
		}

		gap := b.gapFor(parts[len(parts)-1].text, w)
		if width+gap+ww <= avail {
			parts = append(parts, part{text: w, gap: gap})
			width += gap + ww
			words = words[1:]
			continue
		}

		b.flushLine(parts, indentFor(first, indent), false)
		first = false
		parts = nil
		width = 0
	}
	if len(parts) > 0 {
		b.flushLine(parts, indentFor(first, indent), true)
	}
}

func indentFor(first bool, indent float64) float64 {
	if first {
		return indent
	}
	return 0
}

// flushLine aligns and emits one line of words.
func (b *builder) flushLine(parts []part, indentEm float64, last bool) {
	if len(parts) == 0 {
		return
	}
	if b.y > b.p.maxBaseline+0.5 {
		b.nextColumn()
	}
	b.ensurePage()

	total := 0.0
	for i, pt := range parts {
		if i > 0 {
			total += pt.gap
		}
		total += b.measure(pt.text)
	}
	lineW := total * b.p.em
	colLeft := b.colLeft()
	colW := b.p.colW
	align := strings.ToLower(b.req.Page.Align)

	// A line that opens a page or a column is never indented: the sentence
	// simply continues from the previous page.
	if b.freshPage {
		indentEm = 0
	}
	startX := colLeft + indentEm*b.p.em
	extraPx := 0.0 // extra space added to each inter-word gap, pixels
	nGaps := len(parts) - 1
	switch align {
	case "center":
		startX = colLeft + (colW-lineW)/2
	case "justify":
		if !last && nGaps > 0 {
			extraPx = (colW - indentEm*b.p.em - lineW) / float64(nGaps)
			if extraPx < 0 {
				extraPx = 0
			}
			if extraPx > 1.2*b.p.em {
				// Handwriting is barely ever justified hard: keep the gaps
				// believable and let the line stay short instead.
				extraPx = 1.2 * b.p.em
			}
		}
	}
	startX = clampF(startX, colLeft-2, colLeft+colW)
	ctx := b.lineContext()

	penX := startX
	lineStart := penX
	for i, pt := range parts {
		if i > 0 {
			penX += pt.gap*b.p.em + extraPx
		}
		penX = b.placeWord(pt.text, penX, ctx)
	}
	lineEnd := penX

	b.lines[b.page.Index] = append(b.lines[b.page.Index], lineRect{
		x0: lineStart,
		y0: ctx.baseline - b.p.ascent*b.p.em*ctx.scale,
		x1: lineEnd,
		y1: ctx.baseline + b.p.descent*b.p.em*ctx.scale,
	})

	if b.flaws.underline > 0 && b.rng.Float64() < b.flaws.underline*0.25 {
		y := ctx.baseline + 0.13*b.p.em
		b.page.Lines = append(b.page.Lines, model.LineMark{
			X0: lineStart, Y0: y, X1: lineEnd, Y1: y + b.rng.NormFloat64()*0.6,
			Width: 0.045 * b.p.em, Ink: 0.85, Kind: "underline",
		})
	}
	b.freshPage = false
	b.advanceLine()
}

// placeWord emits one word and returns the pen position after it.
func (b *builder) placeWord(text string, penX float64, ctx lineCtx) float64 {
	start := len(b.page.Ops)

	skip := b.flaws.skipWord > 0 && b.rng.Float64() < b.flaws.skipWord
	makeTypo := !skip && b.flaws.correction > 0 && b.rng.Float64() < b.flaws.correction
	crossOut := !skip && !makeTypo && b.flaws.crossOut > 0 && b.rng.Float64() < b.flaws.crossOut

	emitText := text
	if makeTypo {
		emitText = typoOf(text, b.rng)
		if emitText == text {
			emitText = text + "ь"
		}
	}

	opts := runOpts{
		emPx:     ctx.emPx,
		mul:      ctx.scale,
		baseline: ctx.baseline,
		slant:    ctx.slant,
		rotSig:   b.p.rotSigma,
		scaleSig: b.p.scaleSigma,
		inkMean:  ctx.ink,
		inkSig:   b.p.inkSigma,
		weight:   b.p.weightMean,
		alpha:    1,
		links:    true,
		joinProb: b.p.joinProb,
	}

	var endX float64
	if skip {
		// The word is left out; it is written above the line after a caret.
		endX = penX
	} else {
		endX = b.emitRun(emitText, penX, opts)
	}

	span := wordSpan{x0: penX, x1: endX, baseline: ctx.baseline, emPx: ctx.emPx, ops: [2]int{start, len(b.page.Ops)}}

	switch {
	case skip:
		b.page.Lines = append(b.page.Lines, caretMark(penX, ctx.baseline-0.12*ctx.emPx, 0.3*ctx.emPx, 0.05*ctx.emPx, 0.9))
		b.emitCorrection(text, penX+0.2*ctx.emPx, ctx)
	case makeTypo:
		b.strikeWord(span)
		b.page.Lines = append(b.page.Lines, caretMark(penX+0.15*ctx.emPx, ctx.baseline-0.3*ctx.emPx, 0.26*ctx.emPx, 0.045*ctx.emPx, 0.85))
		b.emitCorrection(text, penX+0.1*ctx.emPx, ctx)
	case crossOut:
		b.strikeWord(span)
	}

	if !skip && b.flaws.overwrite > 0 {
		for i := span.ops[0]; i < span.ops[1]; i++ {
			if b.rng.Float64() < b.flaws.overwrite*0.02 {
				b.scribbleOver(b.page.Ops[i])
			}
		}
	}
	if !skip && span.ops[1] > span.ops[0] && b.flaws.pressureDot > 0 && b.rng.Float64() < b.flaws.pressureDot*0.4 {
		op := b.page.Ops[span.ops[0]]
		b.page.Lines = append(b.page.Lines, model.LineMark{
			X0: op.X, Y0: op.Y, X1: op.X + 0.4, Y1: op.Y + 0.3,
			Width: 0.06 * ctx.emPx, Ink: 0.95, Kind: "dot",
		})
	}
	// Clamp stray ops to the page so a wide glyph never escapes the raster.
	for i := span.ops[0]; i < span.ops[1]; i++ {
		op := &b.page.Ops[i]
		op.X = clampF(op.X, 1, float64(b.p.pageW)-1)
		op.Y = clampF(op.Y, 1, float64(b.p.pageH)-1)
	}
	return endX
}

type wordSpan struct {
	x0, x1   float64
	baseline float64
	emPx     float64
	ops      [2]int
}

// runOpts is the per-run typographic state.
type runOpts struct {
	emPx     float64 // px per em for this run
	mul      float64 // line-level scale multiplier
	baseline float64
	slant    float64
	rotSig   float64
	scaleSig float64
	inkMean  float64
	inkSig   float64
	rot      float64 // fixed extra rotation, degrees (margin notes)
	weight   float64
	alpha    float64
	links    bool
	joinProb float64
}

// glyphPoint maps an em-space point of a placed glyph to page pixels. It is the
// same transform the ink renderer applies (scale, shear, rotate about the pen
// origin on the baseline).
func glyphPoint(op *model.GlyphOp, p model.Point) (float64, float64) {
	s := op.Scale
	k := math.Tan(op.Slant * math.Pi / 180)
	th := op.Rotate * math.Pi / 180
	x1 := p.X * s
	y1 := p.Y * s
	x2 := x1 + y1*k
	y2 := y1
	cos, sin := math.Cos(th), math.Sin(th)
	x3 := x2*cos - y2*sin
	y3 := x2*sin + y2*cos
	return op.X + x3, op.Y - y3
}

// emitRun places every rune of text starting at pen x and returns the end pen
// position. Spaces advance the pen.
func (b *builder) emitRun(text string, x float64, o runOpts) float64 {
	if o.emPx <= 0 {
		o.emPx = b.p.em
	}
	if o.mul <= 0 {
		o.mul = 1
	}
	if o.alpha <= 0 {
		o.alpha = 1
	}
	if o.weight <= 0 {
		o.weight = 1
	}
	if o.inkMean <= 0 {
		o.inkMean = b.p.inkMean
	}
	prevIdx := -1
	var prevSp *model.Sprite
	for _, ch := range text {
		if ch == ' ' || ch == '\u00a0' {
			x += b.p.wordGap * o.emPx * (1 + b.rng.NormFloat64()*0.08)
			prevIdx = -1
			prevSp = nil
			continue
		}
		list := b.spritesFor(ch)
		if len(list) == 0 {
			x += 0.36 * o.emPx
			prevIdx = -1
			prevSp = nil
			continue
		}
		idx := b.pickVariant(ch, list)
		sp := &list[idx]
		damp := b.jitterDamp(ch)

		scale := o.emPx * o.mul * (1 + b.rng.NormFloat64()*o.scaleSig)
		rot := o.rot + b.rng.NormFloat64()*o.rotSig*damp
		slant := o.slant + b.rng.NormFloat64()*b.p.slantSigma*damp*0.5
		dy := b.rng.NormFloat64() * b.p.dySigma * o.emPx * damp

		ink := clampF(o.inkMean+b.rng.NormFloat64()*o.inkSig, 0.25, 1)
		weight := clampF(o.weight*(1+b.rng.NormFloat64()*b.p.weightSigma), 0.4, 2.2)

		op := model.GlyphOp{
			Sprite: sp,
			X:      x,
			Y:      o.baseline + dy,
			Scale:  scale,
			Rotate: rot,
			Slant:  slant,
			Ink:    ink,
			Weight: weight,
			Alpha:  o.alpha,
			Seed:   b.rng.Int64(),
		}
		if o.links && prevIdx >= 0 {
			if lnk := b.linkFor(&b.page.Ops[prevIdx], prevSp, &op, o); lnk != nil {
				op.Link = lnk
			}
		}
		b.page.Ops = append(b.page.Ops, op)
		prevIdx = len(b.page.Ops) - 1
		prevSp = sp

		adv := b.p.meanAdv[ch]
		if adv <= 0 {
			adv = spriteAdvance(sp)
		}
		adv *= b.p.advScale * (1 + b.rng.NormFloat64()*b.p.dxSigma)
		if b.rng.Float64() < 0.05 {
			adv *= 1 + b.rng.NormFloat64()*0.04
		}
		x += adv * o.emPx
	}
	return x
}

// jitterDamp returns a multiplier for the jitter of this character: capitals
// and punctuation are written more steadily than lowercase letters.
func (b *builder) jitterDamp(ch rune) float64 {
	switch {
	case unicode.IsUpper(ch):
		return 0.65
	case unicode.IsDigit(ch) || unicode.IsPunct(ch):
		return 0.5
	}
	return 1
}

func (b *builder) spritesFor(ch rune) []model.Sprite {
	if list, ok := b.prof.Glyphs[string(ch)]; ok && len(list) > 0 {
		return list
	}
	if alt, ok := b.p.alt[ch]; ok {
		if list, ok := b.prof.Glyphs[string(alt)]; ok && len(list) > 0 {
			return list
		}
	}
	if lo := unicode.ToLower(ch); lo != ch {
		if list, ok := b.prof.Glyphs[string(lo)]; ok && len(list) > 0 {
			return list
		}
	}
	if up := unicode.ToUpper(ch); up != ch {
		if list, ok := b.prof.Glyphs[string(up)]; ok && len(list) > 0 {
			return list
		}
	}
	return nil
}

// pickVariant chooses a sprite variant weighted by confidence and penalises
// recent repeats so the same letter never looks stamped twice in a row.
func (b *builder) pickVariant(ch rune, list []model.Sprite) int {
	if len(list) == 1 {
		return 0
	}
	recent := b.recent[ch]
	var weights [32]float64
	if len(list) > len(weights) {
		return b.rng.IntN(len(list))
	}
	total := 0.0
	for i := range list {
		c := list[i].Confidence
		if c <= 0 {
			c = 0.6
		}
		wi := c * c
		for _, r := range recent {
			if r == i {
				wi *= 0.12
				break
			}
		}
		if i < len(weights) {
			weights[i] = wi
		}
		total += wi
	}
	if total <= 0 {
		return b.rng.IntN(len(list))
	}
	x := b.rng.Float64() * total
	idx := len(list) - 1
	for i := range list {
		wi := 0.0
		if i < len(weights) {
			wi = weights[i]
		}
		x -= wi
		if x <= 0 {
			idx = i
			break
		}
	}
	recent = append(recent, idx)
	if len(recent) > 3 {
		recent = recent[len(recent)-3:]
	}
	b.recent[ch] = recent
	return idx
}

// linkFor builds the cursive connector from the previous glyph's exit anchor to
// the current glyph's entry anchor.
func (b *builder) linkFor(prevOp *model.GlyphOp, prevSp *model.Sprite, op *model.GlyphOp, o runOpts) *model.LinkOp {
	if prevSp == nil || prevOp.Sprite == nil {
		return nil
	}
	if prevSp.Join.Kind == "" || prevSp.Join.Kind == "none" {
		return nil
	}
	if b.rng.Float64() > o.joinProb || b.rng.Float64() < b.p.liftProb {
		return nil
	}
	x0, y0 := glyphPoint(prevOp, prevSp.Join.Exit)
	x1, y1 := glyphPoint(op, op.Sprite.Join.Entry)
	if math.Abs(x1-x0) > 2.2*op.Scale {
		return nil
	}
	scale := op.Scale
	cx := (x0 + x1) / 2
	cy := (y0 + y1) / 2
	switch prevSp.Join.Kind {
	case "high":
		cy -= 0.17 * scale
	case "up":
		cy -= 0.33 * scale
	case "base":
		cy += 0.04 * scale
	default:
		cy -= 0.1 * scale
	}
	return &model.LinkOp{
		X0: x0, Y0: y0, X1: x1, Y1: y1,
		Ctrl:  model.Point{X: cx + b.rng.NormFloat64()*0.02*scale, Y: cy + b.rng.NormFloat64()*0.03*scale},
		Width: clampF(0.055*scale*(0.85+b.rng.Float64()*0.4), 0.45, 9),
		Ink:   clampF(o.inkMean+b.rng.NormFloat64()*0.05, 0.3, 1),
		Kind:  prevSp.Join.Kind,
	}
}

// emitTitle draws the page title centred above the text body.
func (b *builder) emitTitle(title string) {
	if b.y > b.p.maxBaseline {
		return
	}
	b.ensurePage()
	emPx := b.p.em * 1.25
	lineW := b.measure(title) * emPx
	startX := b.colLeft() + (b.p.colW-lineW)/2
	startX = clampF(startX, b.colLeft(), b.colLeft()+b.p.colW)
	ctx := b.lineContext()
	endX := b.emitRun(title, startX, runOpts{
		emPx:     emPx,
		baseline: ctx.baseline,
		slant:    ctx.slant * 0.6,
		rotSig:   b.p.rotSigma * 0.5,
		scaleSig: b.p.scaleSigma * 0.4,
		inkMean:  clampF(ctx.ink+0.03, 0.3, 1),
		inkSig:   b.p.inkSigma * 0.6,
		weight:   b.p.weightMean * 1.05,
		alpha:    1,
		links:    false,
	})
	if b.flaws.underline > 0.15 {
		y := ctx.baseline + 0.16*emPx
		b.page.Lines = append(b.page.Lines, model.LineMark{
			X0: startX, Y0: y, X1: endX, Y1: y + b.rng.NormFloat64()*0.8,
			Width: 0.05 * emPx, Ink: 0.9, Kind: "underline",
		})
	}
	b.y += b.p.linePitch * 1.2
}

// finishPage adds page numbers, margin notes and page-level paper accidents.
func (b *builder) finishPage(p *model.Page) {
	if b.req.Page.PageNumber && len(b.page.Ops) > 0 {
		b.emitPageNumber(p)
	}
	if b.flaws.marginNote > 0 && b.rng.Float64() < b.flaws.marginNote && len(p.Ops) > 0 {
		b.emitMarginNote(p)
	}
	b.emitPageFlaws(p)
}

func (b *builder) emitPageNumber(p *model.Page) {
	txt := fmt.Sprintf("%d", p.Index+1)
	emPx := b.p.em * 0.62
	w := b.measure(txt) * emPx
	x := float64(p.Width)/2 - w/2
	y := float64(p.Height) - b.p.marginSide*0.45
	if y < float64(p.Height)-20 {
		y = float64(p.Height) - 28
	}
	b.page = p
	b.emitRun(txt, x, runOpts{
		emPx: emPx, baseline: y, slant: b.p.slant, rotSig: 0.5,
		scaleSig: 0.02, inkMean: 0.85, inkSig: 0.04, weight: b.p.weightMean, alpha: 0.9,
	})
}

var marginNotes = []string{
	"см. §2", "не забыть", "проверить!", "сдать 12.11", "???",
	"выучить", "переписать", "тут ошибка", "спросить",
}

func (b *builder) emitMarginNote(p *model.Page) {
	note := marginNotes[b.rng.IntN(len(marginNotes))]
	x := b.p.marginSide * 0.42
	if x < 12 {
		x = 12
	}
	emPx := b.p.em * 0.5
	y := float64(p.Height)*0.25 + b.rng.Float64()*float64(p.Height)*0.25
	b.page = p
	first := true
	for _, ch := range note {
		list := b.spritesFor(ch)
		if len(list) == 0 || ch == ' ' {
			y += 0.3 * emPx
			continue
		}
		idx := b.pickVariant(ch, list)
		sp := &list[idx]
		op := model.GlyphOp{
			Sprite: sp,
			X:      x,
			Y:      y,
			Scale:  emPx,
			Rotate: -90 + b.rng.NormFloat64()*1.5,
			Slant:  b.p.slant * 0.4,
			Ink:    clampF(0.8+b.rng.NormFloat64()*0.05, 0.3, 1),
			Weight: b.p.weightMean,
			Alpha:  0.92,
			Seed:   b.rng.Int64(),
		}
		p.Ops = append(p.Ops, op)
		y += (b.p.meanAdv[ch] + 0.04) * emPx
		first = false
	}
	_ = first
}
