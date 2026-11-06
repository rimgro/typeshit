package ink

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"typeshit/internal/model"
)

// drawTestGlyph paints a soft blob into the atlas: a stand-in for a real
// handwriting sprite, with a stem and a bowl so edges are exercised.
func drawTestGlyph(atlas *image.NRGBA, x0, y0, w, h int) {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx := float64(x)/float64(w) - 0.5
			fy := float64(y)/float64(h) - 0.5
			d := math.Sqrt(fx*fx + fy*fy)
			cov := (0.46 - d) * 12
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			i := (y0+y)*atlas.Stride + (x0+x)*4
			atlas.Pix[i] = 255
			atlas.Pix[i+1] = 255
			atlas.Pix[i+2] = 255
			atlas.Pix[i+3] = uint8(cov * 255)
		}
	}
}

func testProfile() (*model.Profile, []image.Image) {
	atlas := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	drawTestGlyph(atlas, 8, 8, 100, 100)
	prof := &model.Profile{
		ID: "t", Em: 100,
		Glyphs: map[string][]model.Sprite{},
		Style:  model.StyleModel{InkMean: 0.94, WeightMean: 1},
	}
	for i, ch := range []string{"а", "б", "в", "г"} {
		prof.Glyphs[ch] = []model.Sprite{{
			Char: ch, Variant: 0, Atlas: 0,
			Pix:     [4]int{8, 8, 108, 108},
			Box:     model.Rect{X: 0.02, Y: 0, W: 0.5, H: 0.5},
			Advance: 0.55, Ink: 0.94, Weight: 1, Confidence: 0.9,
		}}
		_ = i
	}
	return prof, []image.Image{atlas}
}

func whitePage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 250
		img.Pix[i+1] = 248
		img.Pix[i+2] = 242
		img.Pix[i+3] = 255
	}
	return img
}

func inkAmount(img *image.NRGBA) float64 {
	var sum float64
	for y := 0; y < img.Rect.Dy(); y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < img.Rect.Dx(); x++ {
			v := (int(row[x*4]) + int(row[x*4+1]) + int(row[x*4+2])) / 3
			if v < 200 {
				sum += float64(200 - v)
			}
		}
	}
	return sum
}

func TestSpriteMaskHasTransparentBorder(t *testing.T) {
	prof, atlases := testProfile()
	sp := &prof.Glyphs["а"][0]
	m := SpriteMask(sp, atlases[0])
	if m.Rect.Dx() != 102 || m.Rect.Dy() != 102 {
		t.Fatalf("mask size %dx%d, want 102x102", m.Rect.Dx(), m.Rect.Dy())
	}
	for x := 0; x < m.Rect.Dx(); x++ {
		if m.Pix[x] != 0 || m.Pix[(m.Rect.Dy()-1)*m.Stride+x] != 0 {
			t.Fatalf("top/bottom border is not transparent at x=%d", x)
		}
	}
	for y := 0; y < m.Rect.Dy(); y++ {
		if m.Pix[y*m.Stride] != 0 || m.Pix[y*m.Stride+m.Rect.Dx()-1] != 0 {
			t.Fatalf("left/right border is not transparent at y=%d", y)
		}
	}
}

func TestDrawPagePutsInk(t *testing.T) {
	prof, atlases := testProfile()
	img := whitePage(300, 200)
	c := New(prof, atlases, model.PenSpec{Kind: "ballpoint", Color: "#101830", InkDensity: 1, Pressure: 0.6}, rand.New(rand.NewPCG(1, 2)))
	ops := []model.GlyphOp{
		{Sprite: &prof.Glyphs["а"][0], X: 40, Y: 100, Scale: 60, Ink: 0.95, Weight: 1, Alpha: 1, Seed: 11},
		{Sprite: &prof.Glyphs["б"][0], X: 90, Y: 100, Scale: 60, Rotate: 4, Slant: 8, Ink: 0.9, Weight: 1, Alpha: 1, Seed: 12},
	}
	c.DrawPage(img, ops)
	if inkAmount(img) < 1000 {
		t.Fatalf("expected visible ink, got %f", inkAmount(img))
	}
	// The ink must be dark blue, not black and not the paper colour.
	r, g, b, _ := img.At(56, 85).RGBA()
	if r>>8 > 200 && g>>8 > 200 {
		t.Fatalf("pixel at glyph centre stayed paper-coloured: %d %d %d", r>>8, g>>8, b>>8)
	}
	if int(b>>8) <= int(r>>8) {
		t.Fatalf("ballpoint ink should be cooler than the paper: r=%d b=%d", r>>8, b>>8)
	}
}

func TestDrawPageDeterministic(t *testing.T) {
	prof, atlases := testProfile()
	pen := model.PenSpec{Kind: "ballpoint", Color: "#101830", InkDensity: 1, Pressure: 0.6}
	ops := []model.GlyphOp{
		{Sprite: &prof.Glyphs["а"][0], X: 40, Y: 100, Scale: 60, Ink: 0.95, Weight: 1, Alpha: 1, Seed: 7},
		{Sprite: &prof.Glyphs["в"][0], X: 80, Y: 100, Scale: 60, Ink: 0.95, Weight: 1, Alpha: 1, Seed: 8},
	}
	a := whitePage(200, 200)
	b := whitePage(200, 200)
	New(prof, atlases, pen, rand.New(rand.NewPCG(3, 4))).DrawPage(a, ops)
	New(prof, atlases, pen, rand.New(rand.NewPCG(9, 10))).DrawPage(b, ops)
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("rendering is not deterministic at byte %d", i)
		}
	}
}

func TestPenKindsDiffer(t *testing.T) {
	prof, atlases := testProfile()
	ops := []model.GlyphOp{
		{Sprite: &prof.Glyphs["а"][0], X: 40, Y: 100, Scale: 60, Ink: 0.95, Weight: 1, Alpha: 1, Seed: 5},
	}
	amount := func(kind string) float64 {
		img := whitePage(200, 200)
		New(prof, atlases, model.PenSpec{Kind: kind, Color: "#101830", InkDensity: 1, Pressure: 0.6}, rand.New(rand.NewPCG(1, 1))).DrawPage(img, ops)
		return inkAmount(img)
	}
	gel := amount("gel")
	ball := amount("ballpoint")
	pencil := amount("pencil")
	if !(gel > pencil) {
		t.Fatalf("gel (%f) should be denser than pencil (%f)", gel, pencil)
	}
	if !(ball > pencil) {
		t.Fatalf("ballpoint (%f) should be denser than pencil (%f)", ball, pencil)
	}
	if math.Abs(gel-ball) < 1 {
		t.Fatalf("gel and ballpoint should differ, both %f", gel)
	}
}

func TestRotationKeepsInkArea(t *testing.T) {
	prof, atlases := testProfile()
	area := func(rot float64) float64 {
		img := whitePage(240, 240)
		ops := []model.GlyphOp{{Sprite: &prof.Glyphs["а"][0], X: 100, Y: 140, Scale: 80, Rotate: rot, Ink: 1, Weight: 1, Alpha: 1, Seed: 3}}
		New(prof, atlases, model.PenSpec{Kind: "gel", Color: "#101830", InkDensity: 1, Pressure: 0.6}, rand.New(rand.NewPCG(2, 2))).DrawPage(img, ops)
		return inkAmount(img)
	}
	a := area(0)
	b := area(11)
	if a <= 0 || b <= 0 {
		t.Fatal("no ink")
	}
	if ratio := b / a; ratio < 0.8 || ratio > 1.2 {
		t.Fatalf("rotation changed ink area by %.2fx", ratio)
	}
}

func TestDrawPageHandlesEdges(t *testing.T) {
	prof, atlases := testProfile()
	img := whitePage(50, 50)
	c := New(prof, atlases, model.PenSpec{Kind: "ballpoint"}, rand.New(rand.NewPCG(1, 1)))
	ops := []model.GlyphOp{
		{Sprite: &prof.Glyphs["а"][0], X: -80, Y: -60, Scale: 90, Ink: 1, Weight: 1, Alpha: 1, Seed: 1},
		{Sprite: &prof.Glyphs["а"][0], X: 500, Y: 500, Scale: 90, Ink: 1, Weight: 1, Alpha: 1, Seed: 2},
		{Sprite: nil, X: 10, Y: 10, Scale: 10},
		{Sprite: &prof.Glyphs["а"][0], X: 25, Y: 25, Scale: 0, Ink: 1, Weight: 1, Alpha: 1, Seed: 3},
	}
	c.DrawPage(img, ops)
	empty := whitePage(50, 50)
	c.DrawPage(empty, nil)
	c.DrawPage(nil, ops)
	if inkAmount(empty) != 0 {
		t.Fatal("empty op list must not draw")
	}
}

func TestStrokeKinds(t *testing.T) {
	prof, atlases := testProfile()
	for _, kind := range []string{"underline", "strike", "caret", "dot", "scribble"} {
		img := whitePage(200, 120)
		c := New(prof, atlases, model.PenSpec{Kind: "ballpoint", Color: "#101830", InkDensity: 1, Pressure: 0.6}, rand.New(rand.NewPCG(1, 1)))
		c.Stroke(img, model.LineMark{X0: 20, Y0: 60, X1: 160, Y1: 62, Width: 3, Ink: 0.9, Kind: kind})
		if inkAmount(img) <= 0 {
			t.Fatalf("stroke kind %q drew nothing", kind)
		}
	}
}

func TestLinkDraws(t *testing.T) {
	prof, atlases := testProfile()
	img := whitePage(200, 120)
	c := New(prof, atlases, model.PenSpec{Kind: "ballpoint", Color: "#101830", InkDensity: 1, Pressure: 0.6}, rand.New(rand.NewPCG(1, 1)))
	op := model.GlyphOp{
		Sprite: &prof.Glyphs["а"][0], X: 80, Y: 80, Scale: 50, Ink: 0.9, Weight: 1, Alpha: 1, Seed: 4,
		Link: &model.LinkOp{X0: 20, Y0: 70, X1: 70, Y1: 68, Ctrl: model.Point{X: 45, Y: 55}, Width: 2, Ink: 0.9, Kind: "high"},
	}
	c.DrawPage(img, []model.GlyphOp{op})
	if inkAmount(img) <= 0 {
		t.Fatal("link not drawn")
	}
}

func TestFlawsDraw(t *testing.T) {
	prof, atlases := testProfile()
	for _, f := range []model.Flaw{
		{Kind: "smudge", X: 60, Y: 60, R: 25, Amount: 0.8, Rotate: 20},
		{Kind: "blot", X: 100, Y: 80, R: 5, Amount: 1},
		{Kind: "stain", X: 100, Y: 100, R: 40, Amount: 0.6},
		{Kind: "crease", X: 100, Y: 60, R: 80, Amount: 0.5},
	} {
		img := whitePage(200, 200)
		c := New(prof, atlases, model.PenSpec{Kind: "ballpoint"}, rand.New(rand.NewPCG(1, 1)))
		c.DrawFlaws(img, []model.Flaw{f})
		if changedPixels(img) == 0 {
			t.Fatalf("flaw %q drew nothing", f.Kind)
		}
	}
}

// changedPixels counts pixels that no longer equal the paper colour.
func changedPixels(img *image.NRGBA) int {
	n := 0
	for y := 0; y < img.Rect.Dy(); y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < img.Rect.Dx(); x++ {
			if row[x*4] != 250 || row[x*4+1] != 248 || row[x*4+2] != 242 {
				n++
			}
		}
	}
	return n
}

func TestSpriteMaskGenericImage(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			atlas.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 128})
		}
	}
	sp := &model.Sprite{Pix: [4]int{2, 2, 12, 12}}
	m := SpriteMask(sp, atlas)
	if m.Rect.Dx() != 12 {
		t.Fatalf("mask width %d", m.Rect.Dx())
	}
	if m.Pix[m.Stride+1] != 128 {
		t.Fatalf("alpha not copied: %d", m.Pix[m.Stride+1])
	}
}

func TestLargePagePerformance(t *testing.T) {
	prof, atlases := testProfile()
	const glyphs = 3000
	ops := make([]model.GlyphOp, 0, glyphs)
	chars := []string{"а", "б", "в", "г"}
	rng := rand.New(rand.NewPCG(1, 1))
	for i := 0; i < glyphs; i++ {
		ch := chars[i%len(chars)]
		ops = append(ops, model.GlyphOp{
			Sprite: &prof.Glyphs[ch][0],
			X:      float64(80 + (i%70)*14),
			Y:      float64(100 + (i/70)*55),
			Scale:  34 + rng.Float64()*2,
			Rotate: rng.NormFloat64() * 0.8,
			Slant:  6 + rng.NormFloat64(),
			Ink:    0.9 + rng.NormFloat64()*0.05,
			Weight: 1 + rng.NormFloat64()*0.08,
			Alpha:  1,
			Seed:   rng.Int64(),
		})
	}
	img := whitePage(1240, 1754)
	c := New(prof, atlases, model.PenSpec{Kind: "ballpoint", Color: "#101830", InkDensity: 1, Pressure: 0.6}, rng)
	start := time.Now()
	c.DrawPage(img, ops)
	elapsed := time.Since(start)
	if inkAmount(img) <= 0 {
		t.Fatal("no ink on the page")
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("dense page took %v, want <= 1.5s", elapsed)
	}
	t.Logf("3000 glyphs in %v", elapsed)
}

func TestDrawIntoSubImage(t *testing.T) {
	prof, atlases := testProfile()
	parent := whitePage(100, 100)
	sub := parent.SubImage(image.Rect(20, 20, 80, 80)).(*image.NRGBA)
	c := New(prof, atlases, model.PenSpec{Kind: "ballpoint", Color: "#101830", InkDensity: 1, Pressure: 0.6}, rand.New(rand.NewPCG(1, 1)))
	op := model.GlyphOp{Sprite: &prof.Glyphs["а"][0], X: 40, Y: 50, Scale: 40, Ink: 1, Weight: 1, Alpha: 1, Seed: 3}
	c.DrawPage(sub, []model.GlyphOp{op})
	c.Stroke(sub, model.LineMark{X0: 25, Y0: 30, X1: 70, Y1: 30, Width: 2, Ink: 0.9})
	c.DrawFlaws(sub, []model.Flaw{{Kind: "blot", X: 50, Y: 50, R: 4, Amount: 1}})
	if inkAmount(sub) <= 0 {
		t.Fatal("sub-image received no ink")
	}
	// Outside the sub-image the parent must stay pristine.
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x >= 20 && x < 80 && y >= 20 && y < 80 {
				continue
			}
			i := y*parent.Stride + x*4
			if parent.Pix[i] != 250 || parent.Pix[i+1] != 248 || parent.Pix[i+2] != 242 {
				t.Fatalf("ink leaked outside the sub-image at %d,%d", x, y)
			}
		}
	}
}
