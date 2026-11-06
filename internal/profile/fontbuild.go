package profile

import (
	"errors"
	"image"
	"math"
	"time"

	"typeshit/internal/model"
)

// fontEm is the em size (in atlas pixels) used for font-derived profiles. It is
// larger than spriteEm because font outlines are crisp and benefit from it.
const fontEm = 160.0

// BuildFromFont rasterises a font into a handwriting profile. It is the
// instant-demo path: no sheet to write, but also no personal character.
func BuildFromFont(id, name string, fontData []byte, opts FontOptions) (*model.Profile, []*image.NRGBA, error) {
	if opts.Lang == "" {
		opts.Lang = "ru"
	}
	if opts.Variants <= 0 {
		opts.Variants = 3
	}
	if opts.Variants > 8 {
		opts.Variants = 8
	}
	face, err := newTypeFace(fontData, fontEm)
	if err != nil {
		return nil, nil, err
	}
	defer face.close()

	built := make([]*builtSprite, 0, len(Charset(opts.Lang))*opts.Variants)
	var slants []float64
	for _, ch := range Charset(opts.Lang) {
		runes := []rune(ch)
		if len(runes) != 1 || !face.has(runes[0]) {
			continue
		}
		rg, ok := face.renderGlyph(runes[0])
		if !ok {
			continue
		}
		sp := spriteFromRaster(rg, fontEm, ch)
		if sp == nil {
			continue
		}
		sp.confidence = 0.85
		built = append(built, sp)
		if v := maskSlant(rg.mask); v != 0 {
			slants = append(slants, v)
		}
	}
	if len(built) == 0 {
		return nil, nil, errors.New("profile: the font contains none of the requested characters")
	}
	built = synthesizeVariants(built, opts.Variants)

	glyphs, atlases := packAtlases(built)
	style := fontStyle(glyphs, slants, opts.Style)
	if name == "" {
		name = "Шрифтовой почерк"
	}
	p := &model.Profile{
		ID:      id,
		Name:    name,
		Created: time.Now(),
		Source:  "font",
		Lang:    []string{opts.Lang},
		Em:      fontEm,
		AtlasW:  atlasWidth,
		AtlasH:  atlasHeight,
		Glyphs:  glyphs,
		Style:   style,
		Meta:    map[string]string{"sheetSource": "font"},
	}
	return p, atlases, nil
}

// fontStyle measures the style of a font-derived profile, then lets explicitly
// requested values win.
func fontStyle(glyphs map[string][]model.Sprite, slants []float64, override model.StyleModel) model.StyleModel {
	st := model.StyleModel{
		SlantSigma:   3,
		Slant:        0,
		ScaleSigma:   0.025,
		RotSigma:     1.2,
		DySigma:      0.015,
		DxSigma:      0.012,
		AdvanceScale: 1,
		WordGap:      0.62,
		LineGap:      1.9,
		InkMean:      0.9,
		InkSigma:     0.05,
		JoinProb:     0,
	}
	var weights, inks []float64
	joinable, total := 0, 0
	for _, group := range glyphs {
		for _, s := range group {
			weights = append(weights, s.Weight)
			inks = append(inks, s.Ink)
			total++
			if s.Join.Kind != "none" {
				joinable++
			}
		}
	}
	if len(weights) > 0 {
		// The glyph outlines already carry the font's lean and weight, so the
		// style describes only how far individual letters deviate from it.
		st.WeightMean = 1
		st.WeightSigma = clampF(0.04+stdev(weights)*3, 0.03, 0.18)
	}
	if len(inks) > 0 {
		st.InkMean = clampF(meanOf(inks), 0.4, 1)
		st.InkSigma = clampF(stdev(inks), 0.02, 0.2)
	}
	if total > 0 {
		st.JoinProb = clampF(0.35+0.65*float64(joinable)/float64(total), 0, 0.95)
	}
	// A connected (cursive) font should be written with tighter line spacing
	// and more variation; a print font keeps its gaps.
	if st.JoinProb > 0.5 {
		st.LineGap = 1.75
		st.WordGap = 0.55
		st.DySigma = 0.02
	}
	st.CharCount = len(glyphs)
	for _, g := range glyphs {
		st.VariantCount += len(g)
	}
	overrideStyle(&st, override)
	return st
}

// overrideStyle applies non-zero fields of the requested style.
func overrideStyle(st *model.StyleModel, o model.StyleModel) {
	set := func(dst *float64, v float64) {
		if v != 0 {
			*dst = v
		}
	}
	set(&st.Slant, o.Slant)
	set(&st.SlantSigma, o.SlantSigma)
	set(&st.ScaleSigma, o.ScaleSigma)
	set(&st.RotSigma, o.RotSigma)
	set(&st.DySigma, o.DySigma)
	set(&st.DxSigma, o.DxSigma)
	set(&st.AdvanceScale, o.AdvanceScale)
	set(&st.WordGap, o.WordGap)
	set(&st.LineGap, o.LineGap)
	set(&st.InkMean, o.InkMean)
	set(&st.InkSigma, o.InkSigma)
	set(&st.WeightMean, o.WeightMean)
	set(&st.WeightSigma, o.WeightSigma)
	set(&st.JoinProb, o.JoinProb)
	set(&st.LiftProb, o.LiftProb)
}

// maskSlant estimates the lean of a rendered glyph from the horizontal drift
// between the top and bottom of its ink. Positive means the top leans right.
func maskSlant(m *image.Alpha) float64 {
	if m == nil {
		return 0
	}
	r := inkBounds(m)
	if r.Empty() || r.Dy() < 8 {
		return 0
	}
	band := r.Dy() / 4
	if band < 2 {
		band = 2
	}
	cx := func(y0, y1 int) (float64, float64) {
		var sum, w float64
		for y := y0; y < y1; y++ {
			row := m.Pix[y*m.Stride:]
			for x := r.Min.X; x < r.Max.X; x++ {
				a := float64(row[x]) / 255
				sum += float64(x) * a
				w += a
			}
		}
		if w == 0 {
			return 0, 0
		}
		return sum / w, w
	}
	topX, topW := cx(r.Min.Y, r.Min.Y+band)
	botX, botW := cx(r.Max.Y-band, r.Max.Y)
	if topW == 0 || botW == 0 {
		return 0
	}
	dy := float64(r.Max.Y - band - (r.Min.Y + band))
	if dy < 4 {
		return 0
	}
	deg := math.Atan2(topX-botX, dy) * 180 / math.Pi
	if math.Abs(deg) > 35 {
		return 0
	}
	return deg
}
