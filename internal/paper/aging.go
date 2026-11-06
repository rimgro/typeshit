package paper

import (
	"math"
	"math/rand/v2"
)

// hash2 is a small integer hash used for the per-pixel grain. It is stable
// across runs (no seed from the global RNG) so a given seed reproduces the
// same sheet byte for byte.
func hash2(x, y, seed uint32) uint32 {
	h := x*0x9E3779B1 ^ y*0x85EBCA77 ^ seed*0xC2B2AE3D
	h ^= h >> 15
	h *= 0x2545F491
	h ^= h >> 13
	return h
}

// addGrain lays down the paper fibre: a low frequency mottling plus a
// high frequency tooth, both scaled by the aging amount.
func addGrain(c *canvas, aging float64, rng *rand.Rand) {
	w, h := c.w, c.h
	const cell = 26.0
	gw := int(float64(w)/cell) + 2
	gh := int(float64(h)/cell) + 2
	grid := make([]float64, gw*gh)
	for i := range grid {
		grid[i] = rng.Float64()*2 - 1
	}
	// Paper fibres mostly run along the sheet, so each row gets a bias.
	rowBias := make([]int, h)
	for y := range rowBias {
		rowBias[y] = int(math.Round((rng.Float64()*2 - 1) * 2))
	}
	lowAmp := 0.8 + 3.6*aging
	fibAmp := 0.7 + 3.2*aging
	seed := rng.Uint32()
	pix := c.img.Pix
	stride := c.img.Stride
	for y := 0; y < h; y++ {
		gy := float64(y) / cell
		iy := int(gy)
		fy := gy - float64(iy)
		r0 := iy * gw
		r1 := (iy + 1) * gw
		rb := float64(rowBias[y])
		base := y * stride
		for x := 0; x < w; x++ {
			gx := float64(x) / cell
			ix := int(gx)
			fx := gx - float64(ix)
			v := (grid[r0+ix]*(1-fx)+grid[r0+ix+1]*fx)*(1-fy) +
				(grid[r1+ix]*(1-fx)+grid[r1+ix+1]*fx)*fy
			fib := (float64(hash2(uint32(x), uint32(y), seed)&15) - 7.5) / 7.5
			d := int(lowAmp*v + fibAmp*fib + rb)
			if d == 0 {
				continue
			}
			i := base + x*4
			pix[i] = clampInt(int(pix[i]) + d)
			pix[i+1] = clampInt(int(pix[i+1]) + d)
			pix[i+2] = clampInt(int(pix[i+2]) + d)
		}
	}
	// Micro speckles: stray inclusions in the pulp.
	n := int(float64(w*h) * 0.00004 * (0.4 + aging))
	for i := 0; i < n; i++ {
		c.add(rng.IntN(w), rng.IntN(h), -8-rng.IntN(14))
	}
}

// addOxidation yellows the sheet, strongest near the edges, where a page
// actually ages first.
func addOxidation(c *canvas, aging float64, base rgb) {
	w, h := c.w, c.h
	edgeX := make([]float64, w)
	edgeY := make([]float64, h)
	const span = 0.16 // fraction of the smaller dimension
	ref := math.Min(float64(w), float64(h))
	bx := math.Max(1, span*ref)
	for x := 0; x < w; x++ {
		d := math.Min(float64(x), float64(w-1-x)) / bx
		edgeX[x] = clamp01(1 - d)
	}
	for y := 0; y < h; y++ {
		d := math.Min(float64(y), float64(h-1-y)) / bx
		edgeY[y] = clamp01(1 - d)
	}
	pix := c.img.Pix
	stride := c.img.Stride
	// Precompute the tint delta for every opacity step (1/64 resolution).
	const steps = 64
	var tintR, tintG, tintB [steps + 1]float64
	for i := 0; i <= steps; i++ {
		a := float64(i) / steps
		tintR[i] = (float64(agingTint.r) - float64(base.r)) * a
		tintG[i] = (float64(agingTint.g) - float64(base.g)) * a
		tintB[i] = (float64(agingTint.b) - float64(base.b)) * a
	}
	for y := 0; y < h; y++ {
		ey := edgeY[y]
		base := y * stride
		for x := 0; x < w; x++ {
			e := math.Max(ey, edgeX[x])
			a := aging * (0.05 + 0.55*e*e)
			i := base + x*4
			if a > 0.001 {
				si := int(a * steps)
				if si > steps {
					si = steps
				}
				pix[i] = clampInt(int(float64(pix[i]) + tintR[si] + 0.5))
				pix[i+1] = clampInt(int(float64(pix[i+1]) + tintG[si] + 0.5))
				pix[i+2] = clampInt(int(float64(pix[i+2]) + tintB[si] + 0.5))
			}
			// Subtle loss of brightness towards the cut edge.
			if d := int(aging * 26 * e * e); d > 0 {
				pix[i] = clampInt(int(pix[i]) - d)
				pix[i+1] = clampInt(int(pix[i+1]) - d)
				pix[i+2] = clampInt(int(pix[i+2]) - d)
			}
		}
	}
}

// addWear draws the discrete accidents: foxing spots, worn corners,
// fingerprints and (on badly aged sheets) a coffee ring.
func addWear(c *canvas, aging float64, rng *rand.Rand) {
	w, h := float64(c.w), float64(c.h)
	ref := math.Min(w, h)

	// Foxing: rusty age spots.
	n := 1 + int(aging*10)
	for i := 0; i < n; i++ {
		cx := rng.Float64() * w
		cy := rng.Float64() * h
		r := 5 + rng.Float64()*ref*0.035
		a := (0.06 + rng.Float64()*0.14) * aging
		softBlob(c, cx, cy, r, foxTint, a)
	}

	// Worn / darkened corners.
	cornerR := ref * (0.06 + 0.06*aging)
	corners := [4][2]float64{{0, 0}, {w, 0}, {0, h}, {w, h}}
	for _, p := range corners {
		softBlob(c, p[0], p[1], cornerR*(0.7+rng.Float64()*0.5), wornTint, 0.10*aging)
	}

	// Fingerprints: a couple of faint whorls at the edges.
	fp := 1 + int(aging*2)
	for i := 0; i < fp; i++ {
		cx := rng.Float64() * w
		cy := rng.Float64() * h
		if rng.Float64() < 0.6 {
			if rng.Float64() < 0.5 {
				cx = ref * 0.06
			} else {
				cx = w - ref*0.06
			}
		}
		r := ref * (0.03 + rng.Float64()*0.05)
		whorl(c, cx, cy, r, mix(stainTint, wornTint, 0.5), 0.05*aging)
	}

	// Coffee / tea ring.
	if aging >= 0.3 {
		cx := w * (0.15 + rng.Float64()*0.7)
		cy := h * (0.1 + rng.Float64()*0.8)
		r := ref * (0.08 + rng.Float64()*0.10)
		amount := 0.6 + rng.Float64()*0.4
		ring(c, cx, cy, r, 3+rng.Float64()*4, stainTint, 0.09*aging*amount)
		softBlob(c, cx, cy, r*0.92, stainTint, 0.018*aging*amount)
	}

	// Weaker water stains.
	if aging > 0.55 {
		m := 1 + rng.IntN(2)
		for i := 0; i < m; i++ {
			softBlob(c, rng.Float64()*w, rng.Float64()*h, ref*(0.05+rng.Float64()*0.12), stainTint, 0.03*aging)
		}
	}
}

// softBlob paints a radial falloff disc.
func softBlob(c *canvas, cx, cy, r float64, col rgb, a float64) {
	if a <= 0 || r <= 0 {
		return
	}
	x0 := int(cx - r)
	x1 := int(cx + r)
	y0 := int(cy - r)
	y1 := int(cy + r)
	invR2 := 1 / (r * r)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			d2 := (dx*dx + dy*dy) * invR2
			if d2 >= 1 {
				continue
			}
			f := (1 - d2) * (1 - d2)
			c.blend(x, y, col, a*f)
		}
	}
}

// ring paints an annulus with an irregular inner edge (a dried coffee ring).
func ring(c *canvas, cx, cy, r, width float64, col rgb, a float64) {
	if a <= 0 || r <= 0 {
		return
	}
	outer := r + width
	x0 := int(cx - outer)
	x1 := int(cx + outer)
	y0 := int(cy - outer)
	y1 := int(cy + outer)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			d := math.Hypot(dx, dy)
			// Slightly broken rim.
			th := math.Atan2(dy, dx)
			wob := math.Sin(th*3.0)*width*0.45 + math.Sin(th*7.0+1.3)*width*0.25
			inner := r + wob*0.5
			outerE := r + width + wob
			if d < inner-0.6 || d > outerE+0.6 {
				continue
			}
			f := 1.0
			if d < inner {
				f = 1 - (inner-d)/0.6
			} else if d > outerE {
				f = 1 - (d-outerE)/0.6
			}
			c.blend(x, y, col, a*f)
		}
	}
}

// whorl paints concentric fingerprint ridges.
func whorl(c *canvas, cx, cy, r float64, col rgb, a float64) {
	if a <= 0 {
		return
	}
	x0 := int(cx - r)
	x1 := int(cx + r)
	y0 := int(cy - r)
	y1 := int(cy + r)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			d := math.Hypot(dx, dy)
			if d > r {
				continue
			}
			f := 0.5 + 0.5*math.Sin(d*1.6)
			f *= 1 - d/r
			c.blend(x, y, col, a*f)
		}
	}
}
