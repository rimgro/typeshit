package profile

import (
	"errors"
	"image"

	"typeshit/internal/model"
)

// Merge combines several captured profiles (one per sample-sheet page) into a
// single handwriting profile: variants of the same character are stacked, so a
// three-sheet capture ends up with three real variants per letter.
//
// parts[0] wins for identity (id, name, source, meta) and provides the base
// style; dispersion values are averaged so the variability of every sheet
// contributes.
func Merge(parts []*model.Profile, atlases [][]image.Image) (*model.Profile, []*image.NRGBA, error) {
	if len(parts) == 0 {
		return nil, nil, errors.New("profile: nothing to merge")
	}
	if len(parts) == 1 {
		out := make([]*image.NRGBA, 0, len(atlases[0]))
		for _, a := range atlases[0] {
			out = append(out, toNRGBA(a))
		}
		return parts[0], out, nil
	}

	var built []*builtSprite
	for pi, p := range parts {
		if p == nil || pi >= len(atlases) {
			continue
		}
		for ch, sprites := range p.Glyphs {
			for _, sp := range sprites {
				if sp.Atlas < 0 || sp.Atlas >= len(atlases[pi]) {
					continue
				}
				mask := spriteAlpha(atlases[pi][sp.Atlas], sp)
				if mask.Rect.Empty() {
					continue
				}
				built = append(built, &builtSprite{
					char:       ch,
					variant:    sp.Variant,
					mask:       mask,
					box:        sp.Box,
					advance:    sp.Advance,
					join:       sp.Join,
					ink:        sp.Ink,
					weight:     sp.Weight,
					confidence: sp.Confidence,
				})
			}
		}
	}
	if len(built) == 0 {
		return nil, nil, errors.New("profile: no sprites to merge")
	}
	// Drop fallback (font-grafted) sprites for characters that at least one
	// sheet captured for real. Grafted sprites carry a low confidence.
	best := make(map[string]float64, len(built))
	for _, s := range built {
		if s.confidence > best[s.char] {
			best[s.char] = s.confidence
		}
	}
	kept := built[:0]
	for _, s := range built {
		if best[s.char] >= 0.6 && s.confidence < 0.6 {
			continue
		}
		kept = append(kept, s)
	}
	built = kept
	// Re-number variants per character so indices stay dense.
	next := make(map[string]int, len(parts[0].Glyphs))
	for _, s := range built {
		s.variant = next[s.char]
		next[s.char]++
	}
	glyphs, pages := packAtlases(built)

	out := *parts[0]
	out.Glyphs = glyphs
	out.Atlases = nil
	out.Style.CharCount = len(glyphs)
	out.Style.VariantCount = 0
	for _, g := range glyphs {
		out.Style.VariantCount += len(g)
	}
	var ink, join, lg float64
	n := 0
	for _, p := range parts {
		if p == nil {
			continue
		}
		ink += p.Style.InkMean
		join += p.Style.JoinProb
		lg += p.Style.LineGap
		n++
	}
	if n > 0 {
		out.Style.InkMean = ink / float64(n)
		out.Style.JoinProb = clampF(join/float64(n), 0.55, 0.95)
		out.Style.LineGap = lg / float64(n)
	}
	out.Style.WeightMean = 1
	if out.Meta == nil {
		out.Meta = map[string]string{}
	}
	out.Meta["merged"] = itoa(len(parts))
	// Characters that are still missing after the merge are the grafted ones.
	var missing []string
	for _, ch := range Charset("ru") {
		if best[ch] > 0 && best[ch] < 0.6 {
			missing = append(missing, ch)
		}
	}
	if len(missing) == 0 {
		delete(out.Meta, "grafted")
		delete(out.Meta, "graftedHint")
	} else {
		out.Meta["grafted"] = joinStrings(missing)
	}
	return &out, pages, nil
}

// spriteAlpha extracts a sprite's coverage mask from an atlas page.
func spriteAlpha(atlas image.Image, sp model.Sprite) *image.Alpha {
	b := atlas.Bounds()
	r := image.Rect(sp.Pix[0], sp.Pix[1], sp.Pix[2], sp.Pix[3]).Intersect(b)
	if r.Empty() {
		return image.NewAlpha(image.Rect(0, 0, 0, 0))
	}
	out := image.NewAlpha(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			_, _, _, a := atlas.At(x, y).RGBA()
			out.Pix[(y-r.Min.Y)*out.Stride+(x-r.Min.X)] = uint8(a >> 8)
		}
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
