package layout

import (
	"math/rand/v2"
	"unicode"

	"typeshit/internal/model"
)

// flawPlan is the resolved probability set for human mistakes. All values are
// 0..1 and are rolled once per word / per page as documented in FlawsSpec.
type flawPlan struct {
	crossOut    float64
	correction  float64
	overwrite   float64
	smudge      float64
	blot        float64
	stain       float64
	marginNote  float64
	skipWord    float64
	underline   float64
	crease      float64
	pressureDot float64
	wobble      float64
}

func clamp01(v float64) float64 { return clampF(v, 0, 1) }

func newFlawPlan(f model.FlawsSpec, _ *rand.Rand) *flawPlan {
	return &flawPlan{
		crossOut:    clamp01(f.CrossOut),
		correction:  clamp01(f.Correction),
		overwrite:   clamp01(f.Overwrite),
		smudge:      clamp01(f.Smudge),
		blot:        clamp01(f.Blot),
		stain:       clamp01(f.Stain),
		marginNote:  clamp01(f.MarginNote),
		skipWord:    clamp01(f.SkipWord),
		underline:   clamp01(f.Underline),
		crease:      clamp01(f.Crease),
		pressureDot: clamp01(f.PressureDot),
		wobble:      clamp01(f.LineWobble),
	}
}

// lookalikes are the substitutions people actually make when they mis-write a
// Cyrillic word.
var lookalikes = map[rune][]rune{
	'о': {'а', 'е'},
	'а': {'о', 'е'},
	'е': {'и', 'я'},
	'и': {'е', 'й'},
	'т': {'п', 'н'},
	'п': {'т', 'б'},
	'б': {'в', 'п'},
	'в': {'б', 'ф'},
	'н': {'м', 'п'},
	'м': {'н', 'л'},
	'ш': {'щ', 'с'},
	'щ': {'ш', 'ц'},
	'с': {'з', 'ш'},
	'з': {'с', 'ж'},
	'к': {'н', 'т'},
	'л': {'м', 'и'},
	'р': {'п', 'с'},
	'у': {'и', 'ч'},
	'я': {'е', 'ю'},
	'ю': {'я', 'у'},
}

// typoOf returns a plausible misspelling of word. It always differs from the
// input when the word is long enough to mutate.
func typoOf(word string, rng *rand.Rand) string {
	rs := []rune(word)
	if len(rs) < 2 {
		return word + "ъ"
	}
	lowers := make([]int, 0, len(rs))
	for i, r := range rs {
		if unicode.IsLetter(r) && unicode.IsLower(r) {
			lowers = append(lowers, i)
		}
	}
	if len(lowers) == 0 {
		return word + "ь"
	}
	switch rng.IntN(4) {
	case 0: // duplicate a letter
		i := lowers[rng.IntN(len(lowers))]
		out := append([]rune{}, rs[:i+1]...)
		out = append(out, rs[i:]...)
		return string(out)
	case 1: // swap two neighbours
		i := lowers[rng.IntN(len(lowers))]
		if i+1 < len(rs) && rs[i] != rs[i+1] {
			out := append([]rune{}, rs...)
			out[i], out[i+1] = out[i+1], out[i]
			return string(out)
		}
	case 2: // drop a letter
		i := lowers[rng.IntN(len(lowers))]
		if len(rs) > 3 {
			out := append([]rune{}, rs[:i]...)
			out = append(out, rs[i+1:]...)
			return string(out)
		}
	default: // substitute a lookalike
		i := lowers[rng.IntN(len(lowers))]
		if alts, ok := lookalikes[rs[i]]; ok && len(alts) > 0 {
			out := append([]rune{}, rs...)
			out[i] = alts[rng.IntN(len(alts))]
			return string(out)
		}
	}
	// Fallback: append a soft sign, a classic Russian slip.
	return word + "ь"
}

// strikeWord draws two or three thick overlapping lines through the word and
// marks its glyphs as struck.
func (b *builder) strikeWord(span wordSpan) {
	if b.page == nil || span.ops[1] <= span.ops[0] {
		return
	}
	em := span.emPx
	y := span.baseline - 0.24*em
	slope := b.rng.NormFloat64() * 0.06 * em
	w := clampF(0.036*em*(0.8+b.rng.Float64()*0.5), 0.6, 9)
	b.page.Lines = append(b.page.Lines, model.LineMark{
		X0: span.x0 - 0.08*em, Y0: y, X1: span.x1 + 0.06*em, Y1: y + slope,
		Width: w, Ink: clampF(0.85+b.rng.NormFloat64()*0.06, 0.4, 1), Kind: "strike",
	})
	if b.rng.Float64() < 0.55 {
		y2 := y + 0.09*em
		b.page.Lines = append(b.page.Lines, model.LineMark{
			X0: span.x0 + 0.05*em, Y0: y2, X1: span.x1 - 0.02*em, Y1: y2 + slope*1.4,
			Width: w * 0.85, Ink: 0.75, Kind: "strike",
		})
	}
	if b.rng.Float64() < 0.35 {
		// A diagonal cross line, the way people really cancel a word.
		b.page.Lines = append(b.page.Lines, model.LineMark{
			X0: span.x0, Y0: span.baseline - 0.55*em, X1: span.x1, Y1: span.baseline + 0.02*em,
			Width: w * 0.7, Ink: 0.7, Kind: "strike",
		})
	}
	for i := span.ops[0]; i < span.ops[1]; i++ {
		b.page.Ops[i].Strike = true
	}
}

// scribbleOver draws a small zigzag over one glyph, like a letter written over
// twice.
func (b *builder) scribbleOver(op model.GlyphOp) {
	if b.page == nil || op.Sprite == nil {
		return
	}
	em := op.Scale
	box := op.Sprite.Box
	cx := op.X + (box.X+box.W/2)*em
	cy := op.Y - (box.Y+box.H/2)*em
	hh := box.H * em * 0.45
	ww := box.W * em * 0.55
	steps := 4
	var prevX, prevY float64
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := cx - ww + 2*ww*t
		y := cy + hh*0.6*float64((i%2)*2-1) + b.rng.NormFloat64()*0.5
		if i > 0 {
			b.page.Lines = append(b.page.Lines, model.LineMark{
				X0: prevX, Y0: prevY, X1: x, Y1: y,
				Width: clampF(0.05*em, 0.5, 10), Ink: 0.9, Kind: "scribble",
			})
		}
		prevX, prevY = x, y
	}
}

// caretMark returns a chevron insertion mark: tip at (x,y), half-width w, and
// height h.
func caretMark(x, y, w, stroke, ink float64) model.LineMark {
	return model.LineMark{X0: x, Y0: y, X1: w, Y1: w * 0.9, Width: stroke, Ink: ink, Kind: "caret"}
}

// emitCorrection writes the correct word above the line, connected by a caret.
// It never pushes the following lines: handwriting corrections are squeezed
// into the gap that the ruling already leaves.
func (b *builder) emitCorrection(text string, x float64, ctx lineCtx) {
	if b.page == nil {
		return
	}
	emPx := ctx.emPx * 0.80
	if emPx < 4 {
		emPx = 4
	}
	above := ctx.baseline - 0.60*b.p.linePitch
	// Stay above the previous line's descenders.
	minY := b.p.firstBaseline - 0.2*b.p.linePitch
	if above < minY {
		above = minY
	}
	w := b.measure(text) * emPx
	startX := x
	right := b.colLeft() + b.p.colW - 2
	if startX+w > right {
		startX = right - w
	}
	if startX < b.colLeft()+2 {
		startX = b.colLeft() + 2
	}
	b.emitRun(text, startX, runOpts{
		emPx:     emPx,
		baseline: above,
		slant:    ctx.slant,
		rotSig:   b.p.rotSigma * 0.7,
		scaleSig: b.p.scaleSigma * 0.7,
		inkMean:  clampF(ctx.ink-0.04, 0.3, 1),
		inkSig:   b.p.inkSigma,
		weight:   b.p.weightMean * 0.95,
		alpha:    0.95,
		links:    false,
	})
}

// emitPageFlaws places page-level accidents (smudges, blots, stains, creases).
func (b *builder) emitPageFlaws(p *model.Page) {
	w := float64(p.Width)
	h := float64(p.Height)
	rects := b.lines[p.Index]

	pick := func() (float64, float64) {
		if len(rects) == 0 {
			return w * (0.2 + 0.6*b.rng.Float64()), h * (0.2 + 0.6*b.rng.Float64())
		}
		r := rects[b.rng.IntN(len(rects))]
		// Bias towards the edges of the written line so the middle of the text
		// stays readable.
		frac := 0.08 + 0.22*b.rng.Float64()
		if b.rng.IntN(2) == 0 {
			return r.x0 + (r.x1-r.x0)*frac, (r.y0+r.y1)/2 + b.rng.NormFloat64()*4
		}
		return r.x1 - (r.x1-r.x0)*frac, (r.y0+r.y1)/2 + b.rng.NormFloat64()*4
	}

	if b.flaws.smudge > 0 && b.rng.Float64() < b.flaws.smudge {
		x, y := pick()
		p.Flaws = append(p.Flaws, model.Flaw{
			Kind: "smudge", X: x, Y: y,
			R:      w * (0.03 + 0.05*b.rng.Float64()),
			Amount: 0.25 + 0.5*b.rng.Float64(),
			Rotate: b.rng.Float64() * 180,
		})
	}
	if b.flaws.blot > 0 && b.rng.Float64() < b.flaws.blot {
		x, y := pick()
		p.Flaws = append(p.Flaws, model.Flaw{
			Kind: "blot", X: x, Y: y,
			R:      1.2 + 3.5*b.rng.Float64(),
			Amount: 0.65 + 0.35*b.rng.Float64(),
			Rotate: b.rng.Float64() * 360,
		})
	}
	if b.flaws.stain > 0 && b.rng.Float64() < b.flaws.stain {
		p.Flaws = append(p.Flaws, model.Flaw{
			Kind: "stain",
			X:    w * (0.15 + 0.7*b.rng.Float64()), Y: h * (0.1 + 0.8*b.rng.Float64()),
			R:      w * (0.05 + 0.08*b.rng.Float64()),
			Amount: 0.15 + 0.25*b.rng.Float64(),
			Rotate: b.rng.Float64() * 360,
		})
	}
	if b.flaws.crease > 0 && b.rng.Float64() < b.flaws.crease {
		vertical := b.rng.IntN(2) == 0
		amt := 0.2 + 0.35*b.rng.Float64()
		if vertical {
			x := w * (0.3 + 0.4*b.rng.Float64())
			p.Flaws = append(p.Flaws, model.Flaw{Kind: "crease", X: x, Y: h / 2, R: h / 2, Amount: amt, Rotate: 90})
		} else {
			y := h * (0.3 + 0.4*b.rng.Float64())
			p.Flaws = append(p.Flaws, model.Flaw{Kind: "crease", X: w / 2, Y: y, R: w / 2, Amount: amt, Rotate: 0})
		}
	}
}
