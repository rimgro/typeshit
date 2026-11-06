package effects

import (
	"image"
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"testing"

	"typeshit/internal/model"
	"typeshit/internal/paper"
)

// inkPage builds a notebook page with a few rows of fake handwriting strokes so
// the capture effects can be judged on something that looks like a document.
func inkPage(w, h int, seed uint64) *image.NRGBA {
	rng := rand.New(rand.NewPCG(seed, seed>>3))
	img := paper.Render(model.PaperSpec{Kind: "lined", CellMM: 8, Aging: 0.35, MarginMM: 20, Color: "#fbf8f1"}, w, h, 25.4*float64(w)/210.0, rng)
	dpi := 25.4 * float64(w) / 210.0
	pitch := 8 * dpi / 25.4
	ink := rgb{22, 30, 62}
	for line := 1; line < int(float64(h)/pitch)-2; line++ {
		baseline := float64(line)*pitch + 0.72*pitch
		x := 120.0
		for x < float64(w)-120 {
			wordLen := 30 + rng.Float64()*90
			for x < float64(w)-120 && x < wordLen+float64(line)*0 {
				break
			}
			_ = wordLen
			break
		}
		// scribble a word: a series of loops and diagonal strokes
		for x = 130 + rng.Float64()*20; x < float64(w)-140; x += 6 + rng.Float64()*10 {
			amp := 3.0 + rng.Float64()*9
			top := baseline - 6 - rng.Float64()*14
			drawStroke(img, x, top, x+4+rng.Float64()*8, baseline+rng.Float64()*4, ink, 1.4+rng.Float64()*0.8, amp)
			if rng.Float64() < 0.08 {
				x += 8 + rng.Float64()*10
			}
		}
	}
	return img
}

func drawStroke(img *image.NRGBA, x0, y0, x1, y1 float64, col rgb, width, amp float64) {
	n := int(math.Hypot(x1-x0, y1-y0)) + 2
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x := x0 + (x1-x0)*t
		y := y0 + (y1-y0)*t + math.Sin(t*math.Pi*2)*amp
		for oy := -int(width); oy <= int(width); oy++ {
			for ox := -int(width); ox <= int(width); ox++ {
				px := int(x) + ox
				py := int(y) + oy
				if px < 0 || py < 0 || px >= img.Rect.Dx() || py >= img.Rect.Dy() {
					continue
				}
				d := math.Hypot(float64(ox), float64(oy))
				if d > width {
					continue
				}
				a := 1 - d/(width+0.5)
				j := py*img.Stride + px*4
				img.Pix[j] = clampU8(float64(img.Pix[j])*(1-a) + float64(col.r)*a)
				img.Pix[j+1] = clampU8(float64(img.Pix[j+1])*(1-a) + float64(col.g)*a)
				img.Pix[j+2] = clampU8(float64(img.Pix[j+2])*(1-a) + float64(col.b)*a)
			}
		}
	}
}

func TestVisualTemp(t *testing.T) {
	if os.Getenv("TS_VISUAL") == "" {
		t.Skip("set TS_VISUAL=1")
	}
	base := inkPage(1240, 1754, 11)
	save := func(name string, img *image.NRGBA) {
		f, err := os.Create("/tmp/fx_" + name + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	save("base", base)
	save("scan", Apply(base, model.EffectsSpec{Kind: "scan"}, rand.New(rand.NewPCG(3, 4))))
	save("phone", Apply(base, model.EffectsSpec{Kind: "phone"}, rand.New(rand.NewPCG(3, 4))))
	save("fax", Apply(base, model.EffectsSpec{Kind: "fax"}, rand.New(rand.NewPCG(3, 4))))
}
