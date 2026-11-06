package effects

import (
	"image"
	"math"
	"math/rand/v2"

	"typeshit/internal/model"
)

// applyScan mimics a flatbed scan: a slight desaturation, a steeper tone curve
// that washes the paper white, faint lamp streaks, sensor speckle and dust.
func applyScan(in *image.NRGBA, spec model.EffectsSpec, rng *rand.Rand) *image.NRGBA {
	out := clone(in)
	if spec.Resample > 0 {
		out = resampleSoft(out, spec.Resample)
	}
	desaturate(out, 0.18)
	toneCurve(out, def(spec.Contrast, 0.18), 0.015)
	lampStreaks(out, rng)
	out = blur(out, 0.25+0.35*def(spec.Blur, 0.2))
	addNoise(out, 1.6+9*def(spec.Noise, 0.3), rng)
	dust(out, 1+rng.IntN(3), rng)
	if spec.Shadow > 0 {
		edgeShadow(out, spec.Shadow)
	}
	if spec.Vignette > 0 {
		vignette(out, spec.Vignette)
	}
	q := spec.JPEG
	if q <= 0 {
		q = 92
	}
	return jpegRoundTrip(out, q)
}

// lampStreaks adds the faint horizontal banding of a CIS scanner lamp.
func lampStreaks(in *image.NRGBA, rng *rand.Rand) {
	w, h := in.Rect.Dx(), in.Rect.Dy()
	rowDelta := make([]int, h)
	for y := range rowDelta {
		rowDelta[y] = int(math.Round((rng.Float64()*2 - 1) * 2))
		if rng.Float64() < 0.004 {
			rowDelta[y] += 3 + rng.IntN(4)
		}
	}
	p := in.Pix
	for y := 0; y < h; y++ {
		d := rowDelta[y]
		if d == 0 {
			continue
		}
		row := y * in.Stride
		for x := 0; x < w; x++ {
			i := row + x*4
			p[i] = clampU8(float64(p[i]) + float64(d))
			p[i+1] = clampU8(float64(p[i+1]) + float64(d))
			p[i+2] = clampU8(float64(p[i+2]) + float64(d))
		}
	}
}
