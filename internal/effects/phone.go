package effects

import (
	"image"
	"math"
	"math/rand/v2"

	"typeshit/internal/model"
)

// applyPhone mimics a photo of the page taken with a phone: the sheet is
// slightly rotated and keystoned, the desk shows in the corners, light falls
// unevenly, the lens vignettes and the whole frame went through the camera's
// JPEG encoder.
func applyPhone(in *image.NRGBA, spec model.EffectsSpec, rng *rand.Rand) *image.NRGBA {
	src := in
	if spec.Resample > 0 {
		src = resampleSoft(src, spec.Resample)
	}
	theta := spec.RotateDeg
	if theta == 0 {
		theta = 0.4 + rng.Float64()*1.2
		if rng.Float64() < 0.5 {
			theta = -theta
		}
	}
	inset := 0.018 + rng.Float64()*0.022
	out := tiltAndRotate(src, theta, def(spec.Perspective, 0.22), def(spec.Shadow, 0.35), inset)
	out = blur(out, 0.18+0.4*def(spec.Blur, 0.3))
	lightGradient(out, rng)
	vignette(out, def(spec.Vignette, 0.35))
	addNoise(out, 2+12*def(spec.Noise, 0.4), rng)
	q := spec.JPEG
	if q <= 0 {
		q = 78
	}
	return jpegRoundTrip(out, q)
}

// tiltAndRotate places the page inside the frame at a slight angle and with a
// hand-held keystone, leaving a strip of desk visible around the sheet.
func tiltAndRotate(in *image.NRGBA, thetaDeg, persp, shadow, inset float64) *image.NRGBA {
	w, h := in.Rect.Dx(), in.Rect.Dy()
	if w < 4 || h < 4 {
		return clone(in)
	}
	if inset <= 0 {
		inset = 0.02
	}
	rad := thetaDeg * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	cx, cy := float64(w)/2, float64(h)/2

	ix := inset * float64(w)
	iy := 0.7 * inset * float64(h)
	k := persp * 0.05
	quad := [4]pt{
		{ix + k*float64(w)*0.55, iy + k*float64(h)*0.55},             // TL
		{float64(w) - ix - k*float64(w)*0.30, iy + k*float64(h)*0.1}, // TR
		{float64(w) - ix, float64(h) - iy},                           // BR
		{ix + k*float64(w)*0.15, float64(h) - iy - k*float64(h)*0.2}, // BL
	}
	for i := range quad {
		quad[i] = rotatePoint(quad[i], cx, cy, cos, sin)
	}

	srcQuad := [4]pt{{0, 0}, {float64(w), 0}, {float64(w), float64(h)}, {0, float64(h)}}
	m, ok := homographyFromQuads(srcQuad, quad)
	if !ok {
		return clone(in)
	}
	inv, ok := m.invert()
	if !ok {
		return clone(in)
	}

	desk := mixColour(borderColour(in), rgb{96, 86, 74}, 0.72)
	desk = mixColour(desk, rgb{40, 36, 32}, 0.15)
	shadowPx := math.Max(5, float64(w+h)*0.003)
	return warpPage(in, inv, warpOptions{
		OffsetX:   0,
		OffsetY:   0,
		OutW:      w,
		OutH:      h,
		Desk:      desk,
		ShadowPx:  shadowPx,
		ShadowAmt: 0.25 + 0.5*clampF(shadow, 0, 1),
		DeskGrain: 9,
		DeskSeed:  0x51ED,
	})
}

// lightGradient pours a soft directional light over the photo, slightly warm
// on the bright side and cool in the shade.
func lightGradient(in *image.NRGBA, rng *rand.Rand) {
	w, h := in.Rect.Dx(), in.Rect.Dy()
	strength := 0.05 + rng.Float64()*0.05
	angle := rng.Float64() * math.Pi * 2
	dx, dy := math.Cos(angle), math.Sin(angle)
	xs := make([]float64, w)
	ys := make([]float64, h)
	for x := 0; x < w; x++ {
		xs[x] = (float64(x)/float64(w) - 0.5) * 2 * dx
	}
	for y := 0; y < h; y++ {
		ys[y] = (float64(y)/float64(h) - 0.5) * 2 * dy
	}
	p := in.Pix
	for y := 0; y < h; y++ {
		row := y * in.Stride
		for x := 0; x < w; x++ {
			f := 1 - strength*(xs[x]+ys[y])
			i := row + x*4
			p[i] = clampU8(float64(p[i]) * f * 1.03)
			p[i+1] = clampU8(float64(p[i+1]) * f)
			p[i+2] = clampU8(float64(p[i+2]) * f * 0.97)
		}
	}
}
