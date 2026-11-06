// Package effects simulates how a rendered page was captured: a flatbed scan,
// a phone photo taken under a desk lamp, or a fax.
package effects

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"math/rand/v2"
	"strings"

	xdraw "golang.org/x/image/draw"

	"typeshit/internal/model"
)

type rgb struct{ r, g, b uint8 }

// Apply post-processes a rendered page according to spec.Kind.
//
// Zero-valued numeric fields mean "use the default for this kind" (JSON omits
// them anyway); put any positive value to override. Kind "none" or unknown
// returns the input untouched, the same pointer.
func Apply(img *image.NRGBA, spec model.EffectsSpec, rng *rand.Rand) *image.NRGBA {
	if img == nil {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	if rng == nil {
		rng = rand.New(rand.NewPCG(1, 2))
	}
	switch strings.ToLower(strings.TrimSpace(spec.Kind)) {
	case "scan":
		return applyScan(img, spec, rng)
	case "phone", "photo":
		return applyPhone(img, spec, rng)
	case "fax", "copy":
		return applyFax(img, spec, rng)
	default:
		return img
	}
}

// def returns v when it is positive, otherwise the kind default.
func def(v, fallback float64) float64 {
	if v > 0 {
		return v
	}
	return fallback
}

func clampU8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clone(in *image.NRGBA) *image.NRGBA {
	w, h := in.Rect.Dx(), in.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+w*4], in.Pix[y*in.Stride:y*in.Stride+w*4])
	}
	return out
}

// resampleSoft downsamples and upscales again, mimicking a cheap phone camera
// pipeline that throws away real detail.
func resampleSoft(in *image.NRGBA, amount float64) *image.NRGBA {
	w, h := in.Rect.Dx(), in.Rect.Dy()
	f := 1 / (1 + 2*clampF(amount, 0, 1))
	sw, sh := int(float64(w)*f+0.5), int(float64(h)*f+0.5)
	if sw < 8 || sh < 8 {
		return in
	}
	small := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	xdraw.CatmullRom.Scale(small, small.Rect, in, in.Rect, xdraw.Src, nil)
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.BiLinear.Scale(out, out.Rect, small, small.Rect, xdraw.Src, nil)
	return out
}

// blur is a cheap 3x3 box blur mixed with the original by amount (0..1).
func blur(in *image.NRGBA, amount float64) *image.NRGBA {
	k := clampF(amount, 0, 1)
	if k <= 0.001 {
		return in
	}
	w, h := in.Rect.Dx(), in.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	sp := in.Pix
	dp := out.Pix
	ss, ds := in.Stride, out.Stride
	for y := 0; y < h; y++ {
		y0 := y - 1
		if y0 < 0 {
			y0 = 0
		}
		y1 := y + 1
		if y1 >= h {
			y1 = h - 1
		}
		for x := 0; x < w; x++ {
			x0 := x - 1
			if x0 < 0 {
				x0 = 0
			}
			x1 := x + 1
			if x1 >= w {
				x1 = w - 1
			}
			var r, g, b int
			for yy := y0; yy <= y1; yy++ {
				row := yy * ss
				for xx := x0; xx <= x1; xx++ {
					i := row + xx*4
					r += int(sp[i])
					g += int(sp[i+1])
					b += int(sp[i+2])
				}
			}
			n := float64((x1 - x0 + 1) * (y1 - y0 + 1))
			i := y*ds + x*4
			dp[i] = clampU8(float64(sp[i])*(1-k) + float64(r)/n*k)
			dp[i+1] = clampU8(float64(sp[i+1])*(1-k) + float64(g)/n*k)
			dp[i+2] = clampU8(float64(sp[i+2])*(1-k) + float64(b)/n*k)
			dp[i+3] = 0xFF
		}
	}
	return out
}

// vignette darkens the corners radially. amount is 0..1.
func vignette(in *image.NRGBA, amount float64) *image.NRGBA {
	a := clampF(amount, 0, 1)
	if a <= 0 {
		return in
	}
	w, h := in.Rect.Dx(), in.Rect.Dy()
	xs := make([]float64, w)
	ys := make([]float64, h)
	for x := 0; x < w; x++ {
		d := (float64(x)/float64(w) - 0.5) * 2
		xs[x] = d * d
	}
	for y := 0; y < h; y++ {
		d := (float64(y)/float64(h) - 0.5) * 2
		ys[y] = d * d
	}
	p := in.Pix
	for y := 0; y < h; y++ {
		row := y * in.Stride
		for x := 0; x < w; x++ {
			f := 1 - a*0.55*(xs[x]+ys[y])*0.5
			i := row + x*4
			p[i] = clampU8(float64(p[i]) * f)
			p[i+1] = clampU8(float64(p[i+1]) * f)
			p[i+2] = clampU8(float64(p[i+2]) * f)
		}
	}
	return in
}

// hashNoise is a deterministic 0..1 value for a pixel and seed.
func hashNoise(x, y int, seed uint32) float64 {
	h := uint32(x)*0x9E3779B1 ^ uint32(y)*0x85EBCA77 ^ seed
	h ^= h >> 15
	h *= 0x2545F491
	h ^= h >> 13
	h *= 0x27D4EB2F
	h ^= h >> 16
	return float64(h&0xFFFF) / 65535.0
}

// addNoise adds monochrome sensor noise of the given amplitude (0..255).
func addNoise(in *image.NRGBA, amp float64, rng *rand.Rand) *image.NRGBA {
	if amp <= 0 {
		return in
	}
	seed := rng.Uint32()
	w, h := in.Rect.Dx(), in.Rect.Dy()
	p := in.Pix
	for y := 0; y < h; y++ {
		row := y * in.Stride
		for x := 0; x < w; x++ {
			d := (hashNoise(x, y, seed) - 0.5) * 2 * amp
			i := row + x*4
			p[i] = clampU8(float64(p[i]) + d)
			p[i+1] = clampU8(float64(p[i+1]) + d*0.92)
			p[i+2] = clampU8(float64(p[i+2]) + d*1.08)
		}
	}
	return in
}

// jpegRoundTrip encodes and decodes the image, leaving real compression
// artefacts and 8x8 block structure behind.
func jpegRoundTrip(in *image.NRGBA, quality int) *image.NRGBA {
	if quality <= 0 {
		return in
	}
	q := int(clampF(float64(quality), 1, 100))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, in, &jpeg.Options{Quality: q}); err != nil {
		return in
	}
	dec, err := jpeg.Decode(&buf)
	if err != nil {
		return in
	}
	out := image.NewNRGBA(in.Rect)
	xdraw.Draw(out, out.Rect, dec, dec.Bounds().Min, xdraw.Src)
	return out
}

// toneCurve applies a gentle S-curve to the luminance while keeping colour.
// amount 0..1.
func toneCurve(in *image.NRGBA, amount, contrastBias float64) {
	var lut [256]uint8
	for i := 0; i < 256; i++ {
		v := float64(i) / 255
		// Lift the paper towards white and steepen the midtones.
		v = (v - 0.04) / 0.88
		v = clampF(v, 0, 1)
		v = v + amount*(v-0.5)*(1-math.Abs(2*v-1))
		v = clampF(v+contrastBias, 0, 1)
		lut[i] = clampU8(v * 255)
	}
	p := in.Pix
	for i := 0; i+3 < len(p); i += 4 {
		l := (299*int(p[i]) + 587*int(p[i+1]) + 114*int(p[i+2])) / 1000
		d := int(lut[l]) - l
		p[i] = clampU8(float64(p[i]) + float64(d))
		p[i+1] = clampU8(float64(p[i+1]) + float64(d))
		p[i+2] = clampU8(float64(p[i+2]) + float64(d))
	}
}

// desaturate pulls every pixel towards its luminance. amount 0..1.
func desaturate(in *image.NRGBA, amount float64) {
	k := clampF(amount, 0, 1)
	if k <= 0 {
		return
	}
	p := in.Pix
	for i := 0; i+3 < len(p); i += 4 {
		l := 0.299*float64(p[i]) + 0.587*float64(p[i+1]) + 0.114*float64(p[i+2])
		p[i] = clampU8(float64(p[i])*(1-k) + l*k)
		p[i+1] = clampU8(float64(p[i+1])*(1-k) + l*k)
		p[i+2] = clampU8(float64(p[i+2])*(1-k) + l*k)
	}
}

// toGray flattens the image to luminance.
func toGray(in *image.NRGBA) {
	p := in.Pix
	for i := 0; i+3 < len(p); i += 4 {
		v := clampU8(0.299*float64(p[i]) + 0.587*float64(p[i+1]) + 0.114*float64(p[i+2]))
		p[i], p[i+1], p[i+2] = v, v, v
	}
}

// borderColour estimates the average colour of a thin frame around the image.
func borderColour(in *image.NRGBA) rgb {
	w, h := in.Rect.Dx(), in.Rect.Dy()
	if w == 0 || h == 0 {
		return rgb{200, 200, 200}
	}
	t := 1 + h/200
	var r, g, b, n int
	add := func(x, y int) {
		i := y*in.Stride + x*4
		r += int(in.Pix[i])
		g += int(in.Pix[i+1])
		b += int(in.Pix[i+2])
		n++
	}
	for x := 0; x < w; x += 2 {
		for k := 0; k < t; k++ {
			add(x, k)
			add(x, h-1-k)
		}
	}
	for y := 0; y < h; y += 2 {
		for k := 0; k < t; k++ {
			add(k, y)
			add(w-1-k, y)
		}
	}
	if n == 0 {
		return rgb{200, 200, 200}
	}
	return rgb{clampU8(float64(r) / float64(n)), clampU8(float64(g) / float64(n)), clampU8(float64(b) / float64(n))}
}

// mixColour blends a towards b by t.
func mixColour(a, b rgb, t float64) rgb {
	t = clampF(t, 0, 1)
	return rgb{
		clampU8(float64(a.r)*(1-t) + float64(b.r)*t),
		clampU8(float64(a.g)*(1-t) + float64(b.g)*t),
		clampU8(float64(a.b)*(1-t) + float64(b.b)*t),
	}
}

// edgeShadow darkens the border of the frame, as if the page lifted off the
// scanner glass.
func edgeShadow(in *image.NRGBA, amount float64) {
	a := clampF(amount, 0, 1)
	if a <= 0 {
		return
	}
	w, h := in.Rect.Dx(), in.Rect.Dy()
	band := math.Max(4, float64(h)*0.02)
	p := in.Pix
	for y := 0; y < h; y++ {
		dy := math.Min(float64(y), float64(h-1-y))
		row := y * in.Stride
		for x := 0; x < w; x++ {
			dx := math.Min(float64(x), float64(w-1-x))
			d := math.Min(dx, dy)
			sh := 0.0
			switch {
			case d < 1:
				sh = 1
			case d < band:
				sh = 1 - d/band
			}
			if sh <= 0 {
				continue
			}
			f := 1 - 0.45*a*sh*sh
			i := row + x*4
			p[i] = clampU8(float64(p[i]) * f)
			p[i+1] = clampU8(float64(p[i+1]) * f)
			p[i+2] = clampU8(float64(p[i+2]) * f)
		}
	}
}

// dust sprinkles a few dark specks (scanner dirt, hair, crumbs).
func dust(in *image.NRGBA, count int, rng *rand.Rand) {
	w, h := in.Rect.Dx(), in.Rect.Dy()
	for i := 0; i < count; i++ {
		cx := rng.IntN(w)
		cy := rng.IntN(h)
		r := 0.6 + rng.Float64()*1.6
		shade := uint8(40 + rng.IntN(90))
		for y := int(float64(cy) - r - 1); y <= int(float64(cy)+r+1); y++ {
			for x := int(float64(cx) - r - 1); x <= int(float64(cx)+r+1); x++ {
				if x < 0 || y < 0 || x >= w || y >= h {
					continue
				}
				d := math.Hypot(float64(x)+0.5-float64(cx), float64(y)+0.5-float64(cy))
				if d > r {
					continue
				}
				al := 0.85 * (1 - d/(r+0.5))
				j := y*in.Stride + x*4
				in.Pix[j] = clampU8(float64(in.Pix[j])*(1-al) + float64(shade)*al)
				in.Pix[j+1] = clampU8(float64(in.Pix[j+1])*(1-al) + float64(shade)*al)
				in.Pix[j+2] = clampU8(float64(in.Pix[j+2])*(1-al) + float64(shade)*al)
			}
		}
	}
}

// luminance returns the perceptual brightness of a colour.
func luminance(c color.NRGBA) float64 {
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}
