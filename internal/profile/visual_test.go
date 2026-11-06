package profile

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"testing"

	"typeshit/internal/model"
	"typeshit/internal/render"
)

// TestVisualRoundTrip is a manual inspection aid, not an assertion suite.
// Run with TS_VISUAL=1 go test ./internal/profile -run TestVisualRoundTrip
// and look at the PNGs it drops in /tmp.
func TestVisualRoundTrip(t *testing.T) {
	if os.Getenv("TS_VISUAL") == "" {
		t.Skip("set TS_VISUAL=1 to dump visual artefacts")
	}
	pages, specs, err := SheetPages("ru", 2)
	if err != nil {
		t.Fatal(err)
	}
	spec := specs[0]
	var photos []*image.NRGBA
	for i := range pages {
		l := layoutFor("ru", 2, i)
		blank := image.NewNRGBA(pages[i].Bounds())
		copy(blank.Pix, pages[i].Pix)
		written := writeSheet(t, blank, l, int64(2024+i*17), color.NRGBA{R: 24, G: 30, B: 62, A: 255})
		dump(t, fmt.Sprintf("/tmp/hw_template_%d.png", i), pages[i])
		photo := photoEffect(written, 3.1-float64(i)*0.4, int64(7+i))
		photos = append(photos, photo)
		dump(t, fmt.Sprintf("/tmp/hw_photo_%d.png", i), photo)
	}

	doc, scan, err := AnalyzeScan(photos[0], spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cells=%d rows=%d em=%.1f notes=%v", len(doc.Cells), len(doc.Rows), doc.Em, doc.Notes)
	dump(t, "/tmp/hw_analyzed.png", scan)
	dump(t, "/tmp/hw_overlay.png", overlay(scan, doc))

	prof, atlases, err := CommitScan(doc, scan, model.CommitRequest{Name: "visual"})
	if err != nil {
		t.Fatal(err)
	}
	prof.ID = "visual"
	var imgs []image.Image
	for _, a := range atlases {
		imgs = append(imgs, a)
	}
	preview := render.PreviewLine(prof, imgs, "Съешь же ещё этих мягких французских булок, да выпей чаю.", 1100, 210, 5)
	dump(t, "/tmp/hw_comitted.png", preview)
}

func dump(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

// photoEffect makes a template look like a phone photo: rotated, vignetted,
// noised.
func photoEffect(src *image.NRGBA, deg float64, seed int64) *image.NRGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	rad := float64(deg) * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	cx, cy := float64(w)/2, float64(h)/2
	rng := rand.New(rand.NewPCG(uint64(seed), 3))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			sx := cx + dx*cos + dy*sin
			sy := cy - dx*sin + dy*cos
			if sx < 0 || sy < 0 || sx >= float64(w)-1 || sy >= float64(h)-1 {
				dst.Set(x, y, color.NRGBA{90, 82, 74, 255})
				continue
			}
			c := bilin(src, sx, sy)
			// vignette + uneven lamp light
			v := 1 - 0.16*math.Hypot(dx/cx, dy/cy)
			if sx > float64(w)*0.7 {
				v *= 0.94
			}
			n := float64(rng.IntN(13)) - 6
			dst.Set(x, y, color.NRGBA{
				clampU8(float64(c.R)*v + n),
				clampU8(float64(c.G)*v + n*0.9),
				clampU8(float64(c.B)*v + n*0.8),
				255,
			})
		}
	}
	return dst
}

func bilin(img *image.NRGBA, x, y float64) color.NRGBA {
	x0, y0 := int(x), int(y)
	fx, fy := x-float64(x0), y-float64(y0)
	at := func(xx, yy int) color.NRGBA {
		i := img.PixOffset(xx, yy)
		return color.NRGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], 255}
	}
	a, b := at(x0, y0), at(x0+1, y0)
	c, d := at(x0, y0+1), at(x0+1, y0+1)
	mix := func(v0, v1, v2, v3 uint8) uint8 {
		return clampU8((1-fx)*(1-fy)*float64(v0) + fx*(1-fy)*float64(v1) +
			(1-fx)*fy*float64(v2) + fx*fy*float64(v3))
	}
	return color.NRGBA{mix(a.R, b.R, c.R, d.R), mix(a.G, b.G, c.G, d.G), mix(a.B, b.B, c.B, d.B), 255}
}

func clampU8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// overlay draws the detected cells and baselines on top of the normalised scan
// so the markup UI behaviour can be eyeballed.
func overlay(scan *image.NRGBA, doc *model.CaptureDoc) *image.NRGBA {
	out := image.NewNRGBA(scan.Bounds())
	copy(out.Pix, scan.Pix)
	b := out.Bounds()
	put := func(x, y int, c color.NRGBA) {
		if x >= b.Min.X && y >= b.Min.Y && x < b.Max.X && y < b.Max.Y {
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	rect := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for x := x0; x <= x1; x++ {
			put(x, y0, c)
			put(x, y1, c)
		}
		for y := y0; y <= y1; y++ {
			put(x0, y, c)
			put(x1, y, c)
		}
	}
	green := color.NRGBA{0, 170, 60, 255}
	red := color.NRGBA{220, 30, 40, 255}
	for _, r := range doc.Rows {
		y := int(r.Baseline)
		for x := b.Min.X + 4; x < b.Max.X-4; x += 4 {
			put(x, y, color.NRGBA{40, 90, 220, 255})
		}
	}
	for _, c := range doc.Cells {
		col := green
		if c.Label == "" {
			col = red
		}
		rect(c.Rect[0], c.Rect[1], c.Rect[2], c.Rect[3], col)
	}
	return out
}
