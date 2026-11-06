package effects

import (
	"bytes"
	"image"
	"image/png"
	"math/rand/v2"
	"testing"
	"time"

	"typeshit/internal/model"
)

// testPage builds a white sheet with a solid black bar in the middle, which
// stands in for handwriting when checking that an effect keeps the ink dark.
func testPage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i+3 < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 250, 248, 243, 255
	}
	for y := h * 45 / 100; y < h*55/100; y++ {
		for x := w * 35 / 100; x < w*65/100; x++ {
			i := y*img.Stride + x*4
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = 25, 30, 62
		}
	}
	return img
}

func meanLum(img *image.NRGBA, x0, y0, x1, y1 int) float64 {
	var sum float64
	var n int
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x < 0 || y < 0 || x >= img.Rect.Dx() || y >= img.Rect.Dy() {
				continue
			}
			i := y*img.Stride + x*4
			sum += 0.299*float64(img.Pix[i]) + 0.587*float64(img.Pix[i+1]) + 0.114*float64(img.Pix[i+2])
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func TestNoneReturnsInput(t *testing.T) {
	img := testPage(80, 100)
	if got := Apply(img, model.EffectsSpec{Kind: "none"}, nil); got != img {
		t.Fatal("kind none must return the input untouched")
	}
	if got := Apply(img, model.EffectsSpec{Kind: "gaussian-blur-9000"}, nil); got != img {
		t.Fatal("unknown kind must return the input untouched")
	}
	if got := Apply(img, model.EffectsSpec{}, nil); got != img {
		t.Fatal("empty kind must return the input untouched")
	}
}

func TestNilInput(t *testing.T) {
	for _, kind := range []string{"scan", "phone", "fax", "none"} {
		got := Apply(nil, model.EffectsSpec{Kind: kind}, nil)
		if got == nil {
			t.Fatalf("kind %q returned nil", kind)
		}
	}
}

func TestTinyInput(t *testing.T) {
	for _, sz := range [][2]int{{1, 1}, {2, 2}, {3, 7}, {7, 3}} {
		img := testPage(sz[0], sz[1])
		for _, kind := range []string{"scan", "phone", "fax"} {
			got := Apply(img, model.EffectsSpec{Kind: kind, Resample: 0.5}, rand.New(rand.NewPCG(1, 1)))
			if got == nil || got.Rect.Dx() != sz[0] || got.Rect.Dy() != sz[1] {
				t.Fatalf("kind %q size %v: got %v", kind, sz, got.Rect)
			}
		}
	}
}

func TestDimensionsPreserved(t *testing.T) {
	img := testPage(320, 452)
	for _, kind := range []string{"scan", "phone", "fax"} {
		for _, resample := range []float64{0, 0.7} {
			got := Apply(img, model.EffectsSpec{Kind: kind, Resample: resample}, rand.New(rand.NewPCG(9, 9)))
			if got.Rect.Dx() != 320 || got.Rect.Dy() != 452 {
				t.Fatalf("kind %q resample %.1f: got %v, want 320x452", kind, resample, got.Rect)
			}
		}
	}
}

func TestDeterministic(t *testing.T) {
	img := testPage(240, 320)
	for _, kind := range []string{"scan", "phone", "fax"} {
		var first []byte
		for i := 0; i < 2; i++ {
			got := Apply(img, model.EffectsSpec{Kind: kind}, rand.New(rand.NewPCG(77, 33)))
			var buf bytes.Buffer
			if err := png.Encode(&buf, got); err != nil {
				t.Fatal(err)
			}
			if i == 1 && !bytes.Equal(first, buf.Bytes()) {
				t.Fatalf("kind %q is not deterministic for a fixed seed", kind)
			}
			first = buf.Bytes()
		}
	}
}

// The ink has to survive the capture simulation: a black bar stays black and
// the paper stays bright.
func TestInkStaysDark(t *testing.T) {
	img := testPage(620, 877)
	// Sample well inside the bar and well inside the paper.
	bar := func(g *image.NRGBA) float64 { return meanLum(g, 620*40/100, 877*48/100, 620*60/100, 877*52/100) }
	sheet := func(g *image.NRGBA) float64 { return meanLum(g, 620*15/100, 877*48/100, 620*25/100, 877*52/100) }

	for _, kind := range []string{"scan", "phone", "fax"} {
		got := Apply(img, model.EffectsSpec{Kind: kind}, rand.New(rand.NewPCG(5, 6)))
		b := bar(got)
		s := sheet(got)
		if b > 115 {
			t.Fatalf("kind %q washed the ink out: bar luminance %.0f", kind, b)
		}
		if s < 140 {
			t.Fatalf("kind %q darkened the paper too much: %.0f", kind, s)
		}
		if s-b < 40 {
			t.Fatalf("kind %q: ink/paper contrast collapsed (%.0f vs %.0f)", kind, b, s)
		}
	}
}

// A phone photo shows the desk around the tilted sheet.
func TestPhoneShowsDesk(t *testing.T) {
	img := testPage(620, 877)
	got := Apply(img, model.EffectsSpec{Kind: "phone"}, rand.New(rand.NewPCG(21, 22)))
	corner := meanLum(got, 0, 0, 12, 12)
	paper := meanLum(got, 300, 700, 340, 750)
	if corner >= paper-30 {
		t.Fatalf("corner (%.0f) is not darker than the paper (%.0f)", corner, paper)
	}
	if corner > 150 {
		t.Fatalf("desk corner looks like paper: %.0f", corner)
	}
}

func TestPhoneRotates(t *testing.T) {
	img := testPage(400, 560)
	a := Apply(img, model.EffectsSpec{Kind: "phone", RotateDeg: 2}, rand.New(rand.NewPCG(1, 1)))
	b := Apply(img, model.EffectsSpec{Kind: "phone", RotateDeg: -2}, rand.New(rand.NewPCG(1, 1)))
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("rotateDeg has no effect")
	}
}

func TestFaxDegradesPaper(t *testing.T) {
	img := testPage(320, 452)
	got := Apply(img, model.EffectsSpec{Kind: "fax"}, rand.New(rand.NewPCG(4, 4)))
	white, mid, ink, total := 0, 0, 0, 0
	for y := 0; y < got.Rect.Dy(); y += 2 {
		for x := 0; x < got.Rect.Dx(); x += 2 {
			i := y*got.Stride + x*4
			v := int(got.Pix[i])
			switch {
			case v > 200:
				white++
			case v < 80:
				ink++
			default:
				mid++
			}
			total++
		}
	}
	if frac := float64(white) / float64(total); frac < 0.6 {
		t.Fatalf("fax paper is not white enough: %.2f", frac)
	}
	if float64(ink)/float64(total) < 0.005 {
		t.Fatalf("fax lost the ink: %.3f dark pixels", float64(ink)/float64(total))
	}
	if float64(mid)/float64(total) > 0.2 {
		t.Fatalf("fax tones were not crushed: %.2f midtones", float64(mid)/float64(total))
	}
	for i := 0; i+3 < len(got.Pix); i += 4 {
		if got.Pix[i] != got.Pix[i+1] || got.Pix[i+1] != got.Pix[i+2] {
			t.Fatal("fax output is not grayscale")
		}
	}
}

func TestResampleSoftensEdges(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			i := y*img.Stride + x*4
			v := uint8(0)
			if x >= 100 {
				v = 255
			}
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = v, v, v, 255
		}
	}
	got := resampleSoft(img, 0.8)
	mid := meanLum(got, 96, 40, 105, 60)
	if mid < 20 || mid > 235 {
		t.Fatalf("edge was not softened: mid luminance %.0f", mid)
	}
}

func TestScanWhitensPaper(t *testing.T) {
	img := testPage(300, 400)
	got := Apply(img, model.EffectsSpec{Kind: "scan"}, rand.New(rand.NewPCG(2, 2)))
	before := meanLum(img, 10, 10, 90, 60)
	after := meanLum(got, 10, 10, 90, 60)
	if after < before {
		t.Fatalf("scan darkened the paper: %.0f -> %.0f", before, after)
	}
}

func TestSpeed(t *testing.T) {
	if raceEnabled {
		t.Skip("wall-clock assertion is meaningless under -race")
	}
	img := testPage(1240, 1754)
	for _, kind := range []string{"scan", "phone", "fax"} {
		start := time.Now()
		Apply(img, model.EffectsSpec{Kind: kind}, rand.New(rand.NewPCG(1, 2)))
		if d := time.Since(start); d > 1500*time.Millisecond {
			t.Fatalf("kind %q took %v on A4", kind, d)
		}
	}
}

func TestWarpHomographyRoundTrip(t *testing.T) {
	src := [4]pt{{0, 0}, {100, 0}, {100, 200}, {0, 200}}
	dst := [4]pt{{10, 5}, {95, 0}, {110, 195}, {5, 210}}
	h, ok := homographyFromQuads(src, dst)
	if !ok {
		t.Fatal("homography solve failed")
	}
	inv, ok := h.invert()
	if !ok {
		t.Fatal("homography is not invertible")
	}
	for i, p := range src {
		ux, uy := h.apply(p.x, p.y)
		if absF(ux-dst[i].x) > 1e-6 || absF(uy-dst[i].y) > 1e-6 {
			t.Fatalf("corner %d mapped to (%.3f,%.3f), want (%.3f,%.3f)", i, ux, uy, dst[i].x, dst[i].y)
		}
		bx, by := inv.apply(ux, uy)
		if absF(bx-p.x) > 1e-6 || absF(by-p.y) > 1e-6 {
			t.Fatalf("corner %d did not round-trip: (%.3f,%.3f)", i, bx, by)
		}
	}
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
