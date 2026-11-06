package paper

import (
	"image"
	"image/png"
	"math/rand/v2"
	"os"
	"testing"

	"typeshit/internal/model"
)

func TestVisualTemp(t *testing.T) {
	if os.Getenv("TS_VISUAL") == "" {
		t.Skip("set TS_VISUAL=1")
	}
	w, h := 1240, 1754
	dpi := 25.4 * float64(w) / 210.0
	cases := []struct {
		name string
		spec model.PaperSpec
	}{
		{"lined_clean", model.PaperSpec{Kind: "lined", CellMM: 8, Aging: 0, MarginMM: 20, Color: "#fbf8f1"}},
		{"lined_aged", model.PaperSpec{Kind: "lined", CellMM: 8, Aging: 0.75, MarginMM: 20}},
		{"grid", model.PaperSpec{Kind: "grid", CellMM: 5, Aging: 0.3, MarginMM: 20}},
		{"dot", model.PaperSpec{Kind: "dot", CellMM: 5, Aging: 0.1}},
	}
	for _, c := range cases {
		img := Render(c.spec, w, h, dpi, rand.New(rand.NewPCG(7, 9)))
		f, err := os.Create("/tmp/paper_" + c.name + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	_ = image.Rect
}
