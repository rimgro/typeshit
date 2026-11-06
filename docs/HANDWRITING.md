# How handwriting capture works

This package is the part that makes the output a *person's* handwriting instead
of a font. Nothing here is neural: it is a deterministic raster pipeline that
cuts real ink out of a photograph, normalises it, and hands the renderer several
real variants per character plus a measured variability model.

## The pipeline

```
print SheetPages(lang, reps)          A4 @300dpi, 2-4 pages, one character per cell
        │                             3 corner registration L-marks, lattice, dashed
        │                             baseline + x-height, faint sample glyph in the
        │                             first cell of each group
        ▼
user writes on paper with a pen, photographs it (portrait, page fills frame)
        ▼
AnalyzeScan(photo, spec)
        │  downscale → grayscale → illumination flattening → adaptive threshold
        │  deskew (projection-variance search on dark-pixel row sums)
        │  find the 4 registration marks by their L corner signature
        │  resolve page orientation from the printed header band
        │  homography canonical → photo, inverse warp into 2480×3508
        │  project the known lattice onto the scan (no grid guessing)
        │  refine per-row baselines from the ink that sits on them
        │  estimate em from the x-height of letters without ascenders
        │  estimate the writing slant from block structure tensors
        ▼
CaptureDoc  (cells with rect + guessed label + score, rows with baselines)
        ▼
[human markup in the browser: fix labels, move/merge/split cells]
        ▼
CommitScan(doc, scan, commit)
        │  per cell: local paper level → coverage map → drop ruling/specks
        │  crop ink, resample to 128 px/em, measure Box/Advance/Join/Ink/Weight
        │  each repetition of a character becomes a variant
        │  graft characters that were not on the sheet from a reference font
        │  synthesise extra variants (rotate/scale/thickness) up to 3 per glyph
        │  pack 2048×2048 atlases, measure the StyleModel
        ▼
model.Profile + atlas pages
```

`AnalyzeScan` with `spec.Pages == 0` treats the image as a free-form sample: it
segments ink bottom-up (components → text lines → tokens → letter-sized slices)
and leaves every label empty for the user to fill in.

## Sprite geometry contract (important for the renderer)

* `Sprite.Box` is the **ink bounding box** in em units relative to the pen origin
  on the baseline: `Box.X` = left edge, `Box.Y` = bottom edge above the baseline
  (negative for descenders), `Y` axis points **up**.
* `Sprite.Pix` is the atlas rectangle of the same ink, and the relation
  `Box.W == (Pix[2]-Pix[0]) / Profile.Em` holds exactly (to rounding). The
  renderer should therefore draw the atlas rectangle scaled by
  `op.Scale / Profile.Em` at
  `x = op.X + Box.X*op.Scale`, `y = op.Y - (Box.Y+Box.H)*op.Scale`.
* Atlas pages are `image.NRGBA`: transparent background, **white RGB** and the
  ink coverage in the **alpha** channel. Sample the alpha channel only; tint
  with the pen colour.
* `Sprite.Advance` is the pen advance after the glyph (em), `Join.Entry/Exit`
  are the cursive connection anchors relative to the same origin, and
  `Join.Kind` is `none | base | high | up`.
* Sprites are stored at `Profile.Em` pixels per em: 128 for captured profiles,
  160 for font-derived ones.

## What the Variability model means

`Profile.Style` is measured, not guessed:

| field | source |
|---|---|
| `Slant` | dominant lean of near-vertical strokes, degrees (positive = right) |
| `DxSigma` | spread of advances between variants of the same character, relative |
| `ScaleSigma` | spread of glyph heights between variants, relative |
| `DySigma` | spread of glyph baseline offsets, em |
| `InkMean/InkSigma` | coverage of the ink and its spread |
| `WeightMean/WeightSigma` | stroke thickness (2·area/perimeter) in em |
| `JoinProb` | share of glyphs whose ink touches both side edges (cursive) |
| `LineGap` | measured baseline pitch divided by em |
| `SlantSigma`, `RotSigma` | derived from the scale spread |

`BuildFromFont` produces the same structure from a font so the demo path and the
captured path feed the renderer identically.

## Practical notes

* A full Russian sheet with `reps=2` is 3 pages (80 cells each), `reps=3` is 4
  pages, `reps=1` is 2 pages: writing the whole alphabet is the slow part.
* If a character is missing from the sheet, it is grafted from the Caveat
  reference font and listed in `Profile.Meta["grafted"]`; those sprites carry a
  low `Confidence` so the renderer can avoid them.
* Photos are expected to be portrait with the whole sheet in frame. Wrong
  orientation is corrected automatically via the header band, but a heavily
  perspectived photo loses accuracy at the far edge.
* `AnalyzeScan` returns `CaptureDoc.ID` and `CaptureDoc.Image` empty: the API
  layer owns ids and URLs. `CaptureCell.ID` values are stable and are what
  `CommitRequest.Assignments` refers to.
* Cursive connectivity is a heuristic (a thin single-run tail at the glyph
  edge); print-like samples get few joins, connected hands more. The layout can
  always force joins through the request's style override.

## Measured cost (this machine, 12 threads)

| step | time |
|---|---|
| `SheetPages("ru", 2)` (3 pages) | ~0.75 s |
| `BuildFromFont` (94 chars × 3 variants, one 600 KB atlas) | ~0.35 s |
| `AnalyzeScan` of a written A4 sheet | ~1.9 s |
| `CommitScan` (80 cells + grafting + atlases) | ~0.6 s |

Peak memory for a capture is roughly 200 MB because several full-page
luminance buffers are alive at the same time; captures are therefore processed
one at a time.
