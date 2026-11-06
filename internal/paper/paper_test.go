package paper

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"typeshit/internal/model"
)

const (
	testW = 620
	testH = 877
)

func testDPI(w int) float64 { return 25.4 * float64(w) / 210.0 }

func renderTest(spec model.PaperSpec, seed uint64) *image.NRGBA {
	return Render(spec, testW, testH, testDPI(testW), rand.New(rand.NewPCG(seed, seed+1)))
}

func pixelAt(img *image.NRGBA, x, y int) rgb {
	i := y*img.Stride + x*4
	return rgb{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}
}

func TestRenderDimensions(t *testing.T) {
	for _, spec := range []model.PaperSpec{
		{Kind: "lined", CellMM: 8},
		{Kind: "grid", CellMM: 5},
		{Kind: "dot", CellMM: 5},
		{Kind: "blank"},
		{Kind: "unknown-kind"},
		{},
	} {
		img := renderTest(spec, 1)
		if img.Rect.Dx() != testW || img.Rect.Dy() != testH {
			t.Fatalf("kind %q: got %v, want %dx%d", spec.Kind, img.Rect, testW, testH)
		}
		if len(img.Pix) != testW*testH*4 {
			t.Fatalf("kind %q: pix length %d", spec.Kind, len(img.Pix))
		}
	}
}

// At Aging == 0 a blank sheet must be exactly the base colour, otherwise the
// clean mode stops being predictable.
func TestCleanPageIsFlat(t *testing.T) {
	spec := model.PaperSpec{Kind: "blank", Color: "#fbf8f1"}
	img := renderTest(spec, 3)
	want := rgb{0xFB, 0xF8, 0xF1}
	for y := 0; y < testH; y += 7 {
		for x := 0; x < testW; x += 7 {
			if got := pixelAt(img, x, y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want flat %v", x, y, got, want)
			}
		}
	}
}

// The printed ruling must land exactly on the lattice the layout engine uses:
// line k at y = k*CellMM*dpi/25.4.
func TestRulingPitch(t *testing.T) {
	const cellMM = 8
	img := renderTest(model.PaperSpec{Kind: "lined", CellMM: cellMM, MarginMM: 0}, 5)
	pitch := cellMM * testDPI(testW) / 25.4
	x := testW / 2
	base := rgb{0xFB, 0xF8, 0xF1}

	var rows []int
	for y := 0; y < testH; y++ {
		c := pixelAt(img, x, y)
		if absInt(int(c.r)-int(base.r)) > 2 {
			rows = append(rows, y)
		}
	}
	if len(rows) < 3 {
		t.Fatalf("found only %d ruling rows", len(rows))
	}
	// Group consecutive rows into line centres.
	var centres []float64
	start := rows[0]
	prev := rows[0]
	for _, y := range rows[1:] {
		if y-prev > 2 {
			centres = append(centres, float64(start+prev)/2)
			start = y
		}
		prev = y
	}
	centres = append(centres, float64(start+prev)/2)
	for i, c := range centres {
		want := float64(i+1) * pitch
		if math.Abs(c-want) > 2.0 {
			t.Fatalf("ruling %d at y=%.1f, want ~%.1f", i, c, want)
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestMarginLine(t *testing.T) {
	spec := model.PaperSpec{Kind: "blank", CellMM: 8, MarginMM: 20, MarginSide: "left"}
	img := renderTest(spec, 7)
	dpi := testDPI(testW)
	wantX := int(20 * dpi / 25.4)
	found := -1
	for x := wantX - 3; x <= wantX+3; x++ {
		if c := pixelAt(img, x, testH/2); c.r < 240 || c.g < 220 || c.b < 220 {
			found = x
			break
		}
	}
	if found < 0 {
		t.Fatalf("no margin line near x=%d", wantX)
	}

	right := renderTest(model.PaperSpec{Kind: "blank", MarginMM: 20, MarginSide: "right"}, 7)
	if c := pixelAt(right, testW-wantX, testH/2); c.r > 245 && c.g > 230 {
		t.Fatalf("right margin line missing at x=%d (got %v)", testW-wantX, c)
	}

	none := renderTest(model.PaperSpec{Kind: "blank", MarginMM: 20, MarginSide: "none"}, 7)
	for x := 0; x < testW; x++ {
		if c := pixelAt(none, x, testH/2); c != (rgb{0xFB, 0xF8, 0xF1}) {
			t.Fatalf("MarginSide=none painted a line at x=%d", x)
		}
	}
}

func TestDeterministic(t *testing.T) {
	spec := model.PaperSpec{Kind: "lined", CellMM: 8, Aging: 0.6, MarginMM: 20}
	var prev []byte
	for i := 0; i < 2; i++ {
		img := renderTest(spec, 42)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		if i == 1 && !bytes.Equal(prev, buf.Bytes()) {
			t.Fatal("two renders with the same seed differ")
		}
		prev = buf.Bytes()
	}
}

// Aging must change the sheet, and more aging must add more variance.
func TestAgingAddsWear(t *testing.T) {
	clean := renderTest(model.PaperSpec{Kind: "blank"}, 11)
	aged := renderTest(model.PaperSpec{Kind: "blank", Aging: 1}, 11)

	spread := func(img *image.NRGBA) int {
		minR, maxR := 255, 0
		for y := 0; y < testH; y += 3 {
			for x := 0; x < testW; x += 3 {
				c := pixelAt(img, x, y)
				if int(c.r) < minR {
					minR = int(c.r)
				}
				if int(c.r) > maxR {
					maxR = int(c.r)
				}
			}
		}
		return maxR - minR
	}
	if spread(clean) != 0 {
		t.Fatalf("clean sheet has texture (spread %d)", spread(clean))
	}
	if spread(aged) < 12 {
		t.Fatalf("aged sheet looks almost flat (spread %d)", spread(aged))
	}
}

func TestGridAndDotKinds(t *testing.T) {
	grid := renderTest(model.PaperSpec{Kind: "grid", CellMM: 5, MarginMM: 0}, 13)
	// A vertical ruling line must exist away from the horizontal ones.
	pitch := 5 * testDPI(testW) / 25.4
	x := int(3 * pitch)
	if c := pixelAt(grid, x, int(pitch*1.5)); c.b == 0xFB && c.r == 0xFB {
		t.Fatalf("grid has no vertical ruling at x=%d", x)
	}

	dot := renderTest(model.PaperSpec{Kind: "dot", CellMM: 5}, 13)
	blank := renderTest(model.PaperSpec{Kind: "blank", CellMM: 5}, 13)
	if bytes.Equal(dot.Pix, blank.Pix) {
		t.Fatal("dot kind renders like a blank sheet")
	}
}

func TestRenderPerformance(t *testing.T) {
	if raceEnabled {
		t.Skip("wall-clock assertion is meaningless under -race")
	}
	start := time.Now()
	Render(model.PaperSpec{Kind: "lined", CellMM: 8, Aging: 0.5, MarginMM: 20}, 1240, 1754, 150, rand.New(rand.NewPCG(1, 2)))
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("A4 paper render took %v, budget 500ms", d)
	}
}

func TestZeroSize(t *testing.T) {
	for _, sz := range [][2]int{{0, 0}, {-5, 10}, {10, -5}} {
		img := Render(model.PaperSpec{Kind: "lined"}, sz[0], sz[1], 0, nil)
		if img == nil || img.Rect.Dx() < 1 || img.Rect.Dy() < 1 {
			t.Fatalf("size %v produced %v", sz, img.Rect)
		}
	}
}

func TestColourParsing(t *testing.T) {
	cases := []struct {
		in   string
		def  rgb
		want rgb
	}{
		{"#102030", rgb{9, 9, 9}, rgb{16, 32, 48}},
		{"102030", rgb{9, 9, 9}, rgb{16, 32, 48}},
		{"#fff", rgb{9, 9, 9}, rgb{255, 255, 255}},
		{"", rgb{9, 9, 9}, rgb{9, 9, 9}},
		{"#zzz", rgb{9, 9, 9}, rgb{9, 9, 9}},
		{"nonsense", rgb{9, 9, 9}, rgb{9, 9, 9}},
	}
	for _, c := range cases {
		if got := parseHex(c.in, c.def); got != c.want {
			t.Fatalf("parseHex(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
