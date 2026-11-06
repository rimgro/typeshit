package render

import (
	"image"
	"math"
	"math/rand/v2"
	"strings"

	"typeshit/internal/effects"
	"typeshit/internal/ink"
	"typeshit/internal/layout"
	"typeshit/internal/model"
	"typeshit/internal/paper"
)

// PreviewLine renders a short sample of text with a profile, without touching
// the store. Used for profile thumbnails in the UI.
//
// The page is intentionally small (w×h px) and clean: it is meant to show the
// character shapes, not the paper.
func PreviewLine(prof *model.Profile, atlases []image.Image, text string, w, h int, seed int64) *image.NRGBA {
	if w <= 0 {
		w = 900
	}
	if h <= 0 {
		h = 120
	}
	if text == "" {
		text = "Съешь же ещё этих мягких французских булок, да выпей чаю. 0123456789"
	}
	text = clipText(text, 72)
	req := &model.GenerateRequest{
		Text:      text,
		ProfileID: prof.ID,
		Seed:      seed,
		Page: model.LayoutSpec{
			Width:      w,
			Height:     h,
			MarginSide: 24,
			MarginTop:  20,
			MarginBot:  18,
			Align:      "left",
			LineHeight: 0,
		},
		Paper: model.PaperSpec{Kind: "blank", Color: "#ffffff", MarginSide: "none"},
		Pen:   model.PenSpec{Kind: "ballpoint", Color: "#16203c", InkDensity: 1, Pressure: 0.55},
		Style: model.StyleOverrides{Variability: 1, Scale: previewScale(w, h)},
	}
	Defaults(req, prof)
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)>>17))
	pages, err := layout.Build(req, prof, rng)
	if err != nil || len(pages) == 0 {
		return image.NewNRGBA(image.Rect(0, 0, w, h))
	}
	bg := paper.Render(req.Paper, w, h, 25.4*float64(w)/210.0, rng)
	c := ink.New(prof, atlases, req.Pen, rng)
	c.DrawPage(bg, pages[0].Ops)
	for _, m := range pages[0].Lines {
		c.Stroke(bg, m)
	}
	return effects.Apply(bg, model.EffectsSpec{Kind: "none"}, rng)
}

// previewScale picks an em size that keeps roughly one and a half lines of
// text readable inside a small thumbnail.
func previewScale(w, h int) float64 {
	byWidth := float64(w) / 26.0
	byHeight := float64(h) / 3.1
	return math.Round(math.Min(byWidth, byHeight))
}

// clipText truncates a sample to at most n runes on a word boundary.
func clipText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:—-") + "…"
}
