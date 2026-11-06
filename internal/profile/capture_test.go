package profile

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"typeshit/internal/assets"

	"typeshit/internal/model"
)

// bundledFont exposes the embedded fonts to tests.
func bundledFont(id string) ([]byte, bool) { return assets.FontData(id) }

// testFace loads a bundled font for simulating a human writer.
func testFace(t *testing.T, size float64) *typeFace {
	t.Helper()
	data, ok := bundledFont("caveat")
	if !ok {
		t.Fatal("bundled font missing")
	}
	f, err := newTypeFace(data, size)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.close)
	return f
}

// writeSheet plays the human: it writes every cell's character onto a rendered
// template with per-cell jitter, exactly like someone filling in the sheet.
func writeSheet(t *testing.T, img *image.NRGBA, l *sheetLayout, jitterSeed int64, ink color.NRGBA) *image.NRGBA {
	t.Helper()
	rng := rand.New(rand.NewPCG(uint64(jitterSeed), 11))
	scales := []float64{0.96, 1.0, 1.035}
	faces := make([]*typeFace, len(scales))
	for i, s := range scales {
		faces[i] = testFace(t, l.Em*s)
	}
	for i, c := range l.Cells {
		if c.Char == "" {
			continue
		}
		r := []rune(c.Char)[0]
		f := faces[i%len(faces)]
		if !f.has(r) {
			continue
		}
		g, ok := f.renderGlyph(r)
		if !ok {
			continue
		}
		adv := f.measure(c.Char)
		cx, _ := c.centre()
		dx := (rng.Float64()*2 - 1) * 5
		dy := (rng.Float64()*2 - 1) * 6
		x := cx - adv/2 + dx
		baseline := l.guideBaseline(c) + dy
		if isUpper(c.Char) {
			x += 2
		}
		blitMask(img, g.mask, int(x-g.originX+0.5), int(baseline-g.baselineY+0.5), ink)
	}
	return img
}

func TestCharset(t *testing.T) {
	ru := Charset("ru")
	if len(ru) < 90 {
		t.Fatalf("ru charset too small: %d", len(ru))
	}
	seen := map[string]bool{}
	for _, c := range ru {
		if seen[c] {
			t.Fatalf("duplicate %q", c)
		}
		seen[c] = true
	}
	for _, need := range []string{"ё", "й", "ъ", "я", "Ё", "0", "9", "?", ","} {
		if !seen[need] {
			t.Fatalf("charset missing %q", need)
		}
	}
	short := Charset("ru-short")
	if len(short) >= len(ru) {
		t.Fatalf("ru-short should be shorter")
	}
	lat := Charset("latin")
	if !contains(lat, "q") || contains(lat, "я") {
		t.Fatalf("latin charset wrong")
	}
}

func contains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}

func TestSheetPages(t *testing.T) {
	a, sa, err := SheetPages("ru", 2)
	if err != nil {
		t.Fatal(err)
	}
	b, sb, err := SheetPages("ru", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) == 0 || len(a) != len(sa) {
		t.Fatalf("pages=%d specs=%d", len(a), len(sa))
	}
	if len(a) != len(b) {
		t.Fatal("page count not deterministic")
	}
	for i := range a {
		if a[i].Rect != b[i].Rect {
			t.Fatalf("page %d size differs", i)
		}
		if len(sa[i].Chars) != len(sb[i].Chars) {
			t.Fatalf("page %d spec differs", i)
		}
		if sa[i].Pages != len(a) {
			t.Fatalf("page %d reports Pages=%d, want %d", i, sa[i].Pages, len(a))
		}
		if a[i].Bounds().Dx() != sheetW || a[i].Bounds().Dy() != sheetH {
			t.Fatalf("page %d wrong size", i)
		}
		if !sheetHasInk(a[i]) {
			t.Fatalf("page %d is blank", i)
		}
	}
	if len(a) < 2 || len(a) > 6 {
		t.Fatalf("unexpected page count %d", len(a))
	}
	// Every charset character must appear at least `reps` times overall.
	counts := map[string]int{}
	for _, sp := range sa {
		for _, c := range sp.Chars {
			if c != "" {
				counts[c]++
			}
		}
	}
	for _, c := range Charset("ru") {
		if counts[c] < 2 {
			t.Fatalf("char %q appears %d times", c, counts[c])
		}
	}
}

// sheetHasInk reports whether the sheet contains solid black marks.
func sheetHasInk(img *image.NRGBA) bool {
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 40 && img.Pix[i+3] == 255 {
			n++
		}
		if n > 2000 {
			return true
		}
	}
	return false
}

func TestRoundTripCapture(t *testing.T) {
	pages, specs, err := SheetPages("ru", 2)
	if err != nil {
		t.Fatal(err)
	}
	page := writeSheet(t, pages[0], layoutFor("ru", 2, 0), 42, color.NRGBA{R: 18, G: 22, B: 40, A: 255})
	start := time.Now()
	doc, norm, err := AnalyzeScan(page, specs[0])
	if err != nil {
		t.Fatal(err)
	}
	analyzeMs := time.Since(start).Milliseconds()
	if norm.Bounds().Dx() != sheetW || norm.Bounds().Dy() != sheetH {
		t.Fatalf("normalised size %v", norm.Bounds())
	}
	if len(doc.Cells) == 0 {
		t.Fatal("no cells detected")
	}
	labelled := 0
	for _, c := range doc.Cells {
		if c.Label != "" {
			labelled++
		}
	}
	if labelled < 20 {
		t.Fatalf("only %d labelled cells out of %d", labelled, len(doc.Cells))
	}
	start = time.Now()
	prof, atlases, err := CommitScan(doc, norm, model.CommitRequest{Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("analyze=%dms commit=%dms", analyzeMs, time.Since(start).Milliseconds())
	if len(atlases) == 0 {
		t.Fatal("no atlas pages")
	}
	want := 0
	for _, ch := range specs[0].Chars {
		if ch == "" {
			continue
		}
		if len(prof.Glyphs[ch]) == 0 {
			t.Errorf("no sprite for %q", ch)
		}
		want++
	}
	for ch, list := range prof.Glyphs {
		if len(list) < 3 {
			t.Errorf("%q has %d variants, want >= 3", ch, len(list))
		}
		for _, s := range list {
			if s.Atlas < 0 || s.Atlas >= len(atlases) {
				t.Fatalf("%q atlas index %d out of %d", ch, s.Atlas, len(atlases))
			}
			b := atlases[s.Atlas].Bounds()
			r := image.Rect(s.Pix[0], s.Pix[1], s.Pix[2], s.Pix[3])
			if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > b.Max.X || r.Max.Y > b.Max.Y {
				t.Fatalf("%q sprite %v outside atlas %v", ch, r, b)
			}
			if s.Box.W <= 0 || s.Box.H <= 0 || s.Advance <= 0 {
				t.Fatalf("%q bad metrics %+v", ch, s)
			}
			if math.Abs(s.Box.W*prof.Em-float64(r.Dx())) > 1.5 {
				t.Fatalf("%q box width %f does not match pix width %d", ch, s.Box.W*prof.Em, r.Dx())
			}
		}
	}
	if prof.Style.InkSigma <= 0 {
		t.Errorf("ink variability not measured")
	}
	if prof.Style.VariantCount < 2*prof.Style.CharCount {
		t.Errorf("too few variants: %d for %d chars", prof.Style.VariantCount, prof.Style.CharCount)
	}
	if math.Abs(prof.Style.LineGap-float64(cellH)/doc.Em) > 0.6 {
		t.Errorf("line gap %f out of range (em=%f)", prof.Style.LineGap, doc.Em)
	}
	if len(prof.Meta["grafted"]) > 0 {
		t.Logf("grafted: %s", prof.Meta["grafted"])
	}
	t.Logf("chars=%d variants=%d em=%.1f slant=%.1f style=%+v", prof.Style.CharCount, prof.Style.VariantCount, doc.Em, doc.Slant, prof.Style)
}

func TestAnalyzeRotatedNoisy(t *testing.T) {
	pages, specs, err := SheetPages("ru", 1)
	if err != nil {
		t.Fatal(err)
	}
	sheet := writeSheet(t, pages[0], layoutFor("ru", 1, 0), 7, color.NRGBA{R: 20, G: 20, B: 30, A: 255})
	sheet = applyTestNoise(rotateNRGBA(sheet, 3.2), 0.02, 9)
	doc, norm, err := AnalyzeScan(sheet, specs[0])
	if err != nil {
		t.Fatal(err)
	}
	labelled := 0
	for _, c := range doc.Cells {
		if c.Label != "" {
			labelled++
		}
	}
	if labelled < 40 {
		t.Fatalf("only %d labelled cells after rotation/noise (of %d)", labelled, len(doc.Cells))
	}
	prof, _, err := CommitScan(doc, norm, model.CommitRequest{Name: "rot"})
	if err != nil {
		t.Fatal(err)
	}
	if prof.Style.CharCount < 40 {
		t.Fatalf("captured only %d chars", prof.Style.CharCount)
	}
	if doc.Slant < -20 || doc.Slant > 20 {
		t.Fatalf("implausible slant %f", doc.Slant)
	}
}

func TestAnalyzeFreeform(t *testing.T) {
	// A blank piece of paper with a single line of handwriting.
	img := image.NewNRGBA(image.Rect(0, 0, 2000, 1400))
	fillRect(img, img.Bounds(), color.NRGBA{R: 250, G: 249, B: 245, A: 255})
	face := testFace(t, 120)
	text := "Контрольная работа"
	x := 120.0
	for _, r := range text {
		g, ok := face.renderGlyph(r)
		if !ok {
			continue
		}
		blitMask(img, g.mask, int(x-g.originX), int(700-g.baselineY), color.NRGBA{R: 20, G: 20, B: 40, A: 255})
		x += face.measure(string(r))
	}
	doc, norm, err := AnalyzeScan(img, SheetSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells) < 8 {
		t.Fatalf("freeform segmentation found %d cells", len(doc.Cells))
	}
	doc.Cells[0].Label = "К"
	prof, _, err := CommitScan(doc, norm, model.CommitRequest{Name: "free"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prof.Glyphs["К"]) == 0 {
		t.Fatal("no sprite for the labelled cell")
	}
	if doc.Em < 40 {
		t.Fatalf("bad em estimate %f", doc.Em)
	}
}

// applyTestNoise adds gaussian grain, a light gradient and a vignette.
func applyTestNoise(src *image.NRGBA, sigma float64, seed int64) *image.NRGBA {
	rng := rand.New(rand.NewPCG(uint64(seed), 3))
	out := image.NewNRGBA(src.Bounds())
	w := float64(src.Rect.Dx())
	for y := 0; y < src.Rect.Dy(); y++ {
		for x := 0; x < src.Rect.Dx(); x++ {
			o := y*src.Stride + x*4
			v := float64(src.Pix[o]) / 255
			n := rng.NormFloat64() * sigma
			// Slight horizontal light falloff, like a phone photo.
			v = v*(1-0.12*float64(x)/w) + n
			c := uint8(clampF(v, 0, 1)*255 + 0.5)
			out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = c, c, c, 255
		}
	}
	return out
}

func TestEstimateSlant(t *testing.T) {
	// Vertical bars sheared by k lean left in image coordinates (y grows down),
	// so the measured lean must be negative for positive k.
	for _, k := range []float64{-0.25, 0, 0.2} {
		m := newBitmap(400, 400)
		for bar := 0; bar < 8; bar++ {
			x0 := 40 + bar*45
			for y := 20; y < 380; y++ {
				xx := int(float64(x0) + k*float64(y-20))
				for dx := 0; dx < 6; dx++ {
					if xx+dx >= 0 && xx+dx < 400 {
						m.b[y*m.w+xx+dx] = true
					}
				}
			}
		}
		got := estimateSlant(m)
		want := -math.Atan(k) * 180 / math.Pi
		if math.Abs(got-want) > 3 {
			t.Fatalf("k=%.2f: slant %.1f, want %.1f", k, got, want)
		}
	}
}

func TestAnalyzeWithoutMarks(t *testing.T) {
	pages, specs, err := SheetPages("ru", 1)
	if err != nil {
		t.Fatal(err)
	}
	l := layoutFor("ru", 1, 0)
	page := writeSheet(t, pages[0], l, 3, color.NRGBA{R: 15, G: 18, B: 35, A: 255})
	// Crop away the registration marks, as a tight phone photo would.
	crop := l.Grid.Inset(-20).Add(image.Pt(0, 0))
	sub := image.NewNRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	for y := 0; y < crop.Dy(); y++ {
		copy(sub.Pix[y*sub.Stride:y*sub.Stride+crop.Dx()*4],
			page.Pix[(crop.Min.Y+y)*page.Stride+crop.Min.X*4:(crop.Min.Y+y)*page.Stride+crop.Max.X*4])
	}
	doc, norm, err := AnalyzeScan(sub, specs[0])
	if err != nil {
		t.Fatal(err)
	}
	labelled := 0
	for _, c := range doc.Cells {
		if c.Label != "" {
			labelled++
		}
	}
	if labelled < 20 {
		t.Fatalf("only %d labelled cells without marks: %v", labelled, doc.Notes)
	}
	if _, _, err := CommitScan(doc, norm, model.CommitRequest{Name: "nomarks"}); err != nil {
		t.Fatal(err)
	}
}
