// Package render orchestrates a generation request into page images.
package render

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"math/rand/v2"
	"time"

	"typeshit/internal/effects"
	"typeshit/internal/ink"
	"typeshit/internal/layout"
	"typeshit/internal/model"
	"typeshit/internal/paper"
	"typeshit/internal/store"
)

// Engine renders pages and persists them through the store.
type Engine struct {
	Store *store.Store
}

// Result is the output of a render job.
type Result struct {
	ID     string
	Pages  []*image.NRGBA
	Names  []string
	Width  int
	Height int
	Millis int64
}

// Defaults fills sensible values for any missing request field.
func Defaults(req *model.GenerateRequest, prof *model.Profile) {
	p := &req.Page
	if p.Width <= 0 {
		p.Width = 1240
	}
	if p.Height <= 0 {
		p.Height = 1754
	}
	if p.MarginTop <= 0 {
		p.MarginTop = 90
	}
	if p.MarginSide <= 0 {
		p.MarginSide = 110
	}
	if p.MarginBot <= 0 {
		p.MarginBot = 90
	}
	if p.LineHeight <= 0 {
		p.LineHeight = 0
	}
	if p.Columns <= 0 {
		p.Columns = 1
	}
	if p.Align == "" {
		p.Align = "left"
	}
	if req.Paper.Kind == "" {
		req.Paper.Kind = "lined"
	}
	if req.Paper.Color == "" {
		req.Paper.Color = "#fbf8f1"
	}
	if req.Paper.RulingColor == "" {
		req.Paper.RulingColor = "#9fb6cf"
	}
	if req.Paper.CellMM <= 0 {
		req.Paper.CellMM = 8
	}
	if req.Paper.MarginSide == "" {
		req.Paper.MarginSide = "left"
	}
	if req.Paper.MarginMM <= 0 {
		req.Paper.MarginMM = 20
	}
	if req.Pen.Kind == "" {
		req.Pen.Kind = "ballpoint"
	}
	if req.Pen.Color == "" {
		req.Pen.Color = "#141a3a"
	}
	if req.Pen.InkDensity <= 0 {
		req.Pen.InkDensity = 1
	}
	if req.Pen.Pressure <= 0 {
		req.Pen.Pressure = 0.55
	}
	if req.Effects.Kind == "" {
		req.Effects.Kind = "none"
	}
	if req.Style.Variability <= 0 {
		req.Style.Variability = 1
	}
	if req.Seed == 0 {
		var b [8]byte
		_, _ = crand.Read(b[:])
		for i := 0; i < 8; i++ {
			req.Seed = req.Seed<<8 | int64(b[i])
		}
	}
}

// Render runs the full pipeline for a request.
func (e *Engine) Render(req *model.GenerateRequest) (*Result, error) {
	start := time.Now()
	prof, atlases, err := e.Store.LoadProfile(req.ProfileID)
	if err != nil {
		return nil, fmt.Errorf("profile %q: %w", req.ProfileID, err)
	}
	Defaults(req, prof)

	rng := rand.New(rand.NewPCG(uint64(req.Seed), uint64(req.Seed)>>32))

	pages, err := layout.Build(req, prof, rng)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("nothing to render: empty text")
	}

	res := &Result{Width: req.Page.Width, Height: req.Page.Height}
	res.ID = NewID("r")
	if _, err := e.Store.NewResultDir(res.ID); err != nil {
		return nil, err
	}

	canvas := ink.New(prof, atlases, req.Pen, rng)
	dpi := 25.4 * float64(req.Page.Width) / 210.0

	for i, page := range pages {
		bg := paper.Render(req.Paper, page.Width, page.Height, dpi, rng)
		canvas.DrawFlaws(bg, page.Flaws)
		canvas.DrawPage(bg, page.Ops)
		for _, m := range page.Lines {
			canvas.Stroke(bg, m)
		}
		out := effects.Apply(bg, req.Effects, rng)
		name, err := e.Store.SavePage(res.ID, i, out)
		if err != nil {
			return nil, err
		}
		res.Pages = append(res.Pages, out)
		res.Names = append(res.Names, name)
	}
	res.Millis = time.Since(start).Milliseconds()
	return res, nil
}

// NewID returns a short random identifier with a prefix.
func NewID(prefix string) string {
	var b [6]byte
	_, _ = crand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}
