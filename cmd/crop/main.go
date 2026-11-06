// Command crop extracts a region of a PNG and scales it up, for eyeballing
// rendered pages at pixel level.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	xdraw "golang.org/x/image/draw"
)

func main() {
	in := flag.String("in", "", "input png")
	out := flag.String("out", "/tmp/crop.png", "output png")
	x0 := flag.Int("x", 0, "x")
	y0 := flag.Int("y", 0, "y")
	w := flag.Int("w", 600, "width")
	h := flag.Int("h", 400, "height")
	scale := flag.Float64("s", 2, "scale")
	flag.Parse()

	f, err := os.Open(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	img, err := png.Decode(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rect := image.Rect(*x0, *y0, *x0+*w, *y0+*h).Intersect(img.Bounds())
	src := image.NewNRGBA(rect)
	xdraw.Draw(src, src.Bounds(), img, rect.Min, xdraw.Src)

	dw, dh := int(float64(src.Bounds().Dx())*(*scale)), int(float64(src.Bounds().Dy())*(*scale))
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)

	o, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := png.Encode(o, dst); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	o.Close()
	fmt.Println(*out, dw, dh)
}
