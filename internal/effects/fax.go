package effects

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"

	"typeshit/internal/model"
)

// applyFax crushes the tones, binarises the result almost completely, adds
// salt noise and scanlines: the classic 1990s fax of a handwritten page.
func applyFax(in *image.NRGBA, spec model.EffectsSpec, rng *rand.Rand) *image.NRGBA {
	out := clone(in)
	if spec.Resample > 0 {
		out = resampleSoft(out, spec.Resample)
	}
	desaturate(out, 1)

	contrast := def(spec.Contrast, 0.65)
	p := out.Pix
	for i := 0; i+3 < len(p); i += 4 {
		c := color.NRGBA{p[i], p[i+1], p[i+2], 255}
		l := luminance(c) / 255
		l = clampF((l-0.52)*(1+2.2*clampF(contrast, 0, 1))+0.54, 0, 1)
		s := smoothstep(0.30, 0.62, l)
		l = 0.28*l + 0.72*s
		v := clampU8(l * 255)
		p[i], p[i+1], p[i+2] = v, v, v
	}

	out = blur(out, 0.12+0.25*def(spec.Blur, 0.1))

	// Salt: isolated white specks where the line dropped out.
	w, h := out.Rect.Dx(), out.Rect.Dy()
	salt := int(float64(w*h) * 0.0012 * (0.4 + def(spec.Noise, 0.5)))
	for i := 0; i < salt; i++ {
		x := rng.IntN(w)
		y := rng.IntN(h)
		j := y*out.Stride + x*4
		out.Pix[j], out.Pix[j+1], out.Pix[j+2] = 0xFF, 0xFF, 0xFF
	}
	// Scanlines: the fax head advances one row at a time.
	for y := 0; y < h; y += 3 {
		row := y * out.Stride
		for x := 0; x < w; x++ {
			j := row + x*4
			out.Pix[j] = clampU8(float64(out.Pix[j]) * 0.965)
			out.Pix[j+1] = clampU8(float64(out.Pix[j+1]) * 0.965)
			out.Pix[j+2] = clampU8(float64(out.Pix[j+2]) * 0.965)
		}
	}
	addNoise(out, 1.5+5*def(spec.Noise, 0.5), rng)
	if spec.Vignette > 0 {
		vignette(out, spec.Vignette)
	}
	q := spec.JPEG
	if q <= 0 {
		q = 65
	}
	return jpegRoundTrip(out, q)
}

// smoothstep maps x from [lo,hi] to [0,1] with an S-curve.
func smoothstep(lo, hi, x float64) float64 {
	if hi <= lo {
		return 0
	}
	t := clampF((x-lo)/(hi-lo), 0, 1)
	return t * t * (3 - 2*t)
}

var _ = math.Abs
