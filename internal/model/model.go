// Package model contains the shared data contracts of the handwriting engine.
//
// Coordinate systems
//
//	"em" units: 1.0 == font em size. Origin is the pen position on the
//	baseline, X grows to the right, Y grows upwards. Glyph boxes are
//	expressed relative to that origin (descenders are negative).
//	"px" units: raster pixels of the rendered page, Y grows downwards.
package model

import "time"

// Point is a 2D point in em units.
type Point struct{ X, Y float64 }

// Rect is an axis-aligned box in em units with Y pointing up: X is the left
// offset from the pen origin, Y is the bottom edge above the baseline.
type Rect struct{ X, Y, W, H float64 }

// Join describes how a glyph connects to the next one for pseudo-cursive
// writing.
type Join struct {
	Entry Point `json:"entry"`
	Exit  Point `json:"exit"`
	// Kind tells the renderer how to draw the link:
	//   "none"  - glyph is isolated (uppercase, most print styles)
	//   "base"  - link at baseline height
	//   "high"  - link around x-height (cursive o/л/м)
	//   "up"    - link sweeps up to the next glyph (cursive в/б/у)
	Kind string `json:"kind"`
}

// Sprite is one concrete instance of a character cut out of a handwriting
// sample (or rendered from a fallback font). A profile keeps several variants
// per character; the renderer samples among them.
type Sprite struct {
	Char    string  `json:"char"`
	Variant int     `json:"variant"`
	Atlas   int     `json:"atlas"`   // atlas page index
	Pix     [4]int  `json:"pix"`     // x0,y0,x1,y1 inside the atlas (Y down)
	Box     Rect    `json:"box"`     // em units, relative to baseline origin
	Advance float64 `json:"advance"` // em units
	Join    Join    `json:"join"`
	Ink     float64 `json:"ink"`    // measured mean darkness 0..1
	Weight  float64 `json:"weight"` // measured stroke weight, em units
	// Confidence is 0..1 and comes from auto-segmentation quality. Low
	// confidence sprites are still usable but are penalised when sampling.
	Confidence float64 `json:"confidence"`
}

// StyleModel is the variability fingerprint measured on the sample. It drives
// how much the synthesised page drifts from the ideal.
type StyleModel struct {
	Slant        float64 `json:"slant"`        // degrees, positive leans right
	SlantSigma   float64 `json:"slantSigma"`   // degrees
	ScaleSigma   float64 `json:"scaleSigma"`   // relative (0.05 == 5%)
	RotSigma     float64 `json:"rotSigma"`     // degrees per glyph
	DySigma      float64 `json:"dySigma"`      // em, baseline jitter
	DxSigma      float64 `json:"dxSigma"`      // em, letter spacing jitter
	AdvanceScale float64 `json:"advanceScale"` // multiplier on sprite advances
	WordGap      float64 `json:"wordGap"`      // em
	LineGap      float64 `json:"lineGap"`      // em
	InkMean      float64 `json:"inkMean"`
	InkSigma     float64 `json:"inkSigma"`
	WeightMean   float64 `json:"weightMean"`
	WeightSigma  float64 `json:"weightSigma"`
	JoinProb     float64 `json:"joinProb"` // chance to draw the cursive link
	LiftProb     float64 `json:"liftProb"` // chance to break inside a word
	CharCount    int     `json:"charCount"`
	VariantCount int     `json:"variantCount"`
}

// Profile is a complete handwriting model: sprites plus measured style.
type Profile struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Created time.Time           `json:"created"`
	Source  string              `json:"source"` // "scan" | "font" | "manual"
	Lang    []string            `json:"lang"`
	Em      float64             `json:"em"` // atlas pixels per em
	AtlasW  int                 `json:"atlasW"`
	AtlasH  int                 `json:"atlasH"`
	Atlases []string            `json:"atlases"` // file names inside the profile dir
	Glyphs  map[string][]Sprite `json:"glyphs"`
	Style   StyleModel          `json:"style"`
	Meta    map[string]string   `json:"meta"`
	Preview string              `json:"preview"` // relative path to a test-line PNG
}

// Variants returns all sprites for ch, or nil.
func (p *Profile) Variants(ch string) []Sprite { return p.Glyphs[ch] }

// Chars returns every character the profile can draw, sorted.
func (p *Profile) Chars() []string {
	out := make([]string, 0, len(p.Glyphs))
	for k := range p.Glyphs {
		out = append(out, k)
	}
	return out
}

// ---------- Generation request ----------

// StyleOverrides are optional per-request deviations from the profile style.
// nil means "keep what the profile measured".
type StyleOverrides struct {
	Scale       float64  `json:"scale"`       // 0 => auto (fit page)
	Slant       *float64 `json:"slant"`       // absolute degrees
	LineGap     *float64 `json:"lineGap"`     // em
	WordGap     *float64 `json:"wordGap"`     // em
	Variability float64  `json:"variability"` // 0..2 multiplier for sigmas, 1 = measured
	Join        *float64 `json:"join"`        // 0..1 cursive link probability
}

// PaperSpec selects and tints the page background.
type PaperSpec struct {
	Kind        string  `json:"kind"`        // lined | grid | dot | blank | custom
	Color       string  `json:"color"`       // hex, paper tint
	RulingColor string  `json:"rulingColor"` // hex
	CellMM      float64 `json:"cellMm"`      // ruling pitch in mm
	MarginMM    float64 `json:"marginMm"`    // red margin line offset, 0 = none
	Aging       float64 `json:"aging"`       // 0..1 yellowing / wear
	Texture     string  `json:"texture"`     // asset id of a scanned paper, optional
	MarginSide  string  `json:"marginSide"`  // left | right | none
}

// PenSpec drives the ink renderer.
type PenSpec struct {
	Color      string  `json:"color"`      // hex ink colour
	Kind       string  `json:"kind"`       // ballpoint | gel | pencil | fountain
	Width      float64 `json:"width"`      // em fraction, 0 = from profile
	InkDensity float64 `json:"inkDensity"` // 0..1
	Pressure   float64 `json:"pressure"`   // 0..1 modulates width/opacity
	Bleed      float64 `json:"bleed"`      // 0..1 paper absorption halo
	Sharp      bool    `json:"sharp"`      // hard edges instead of anti-aliased?
}

// EffectsSpec is the "how was this page captured" model.
type EffectsSpec struct {
	Kind        string  `json:"kind"` // none | scan | phone | fax
	Noise       float64 `json:"noise"`
	Blur        float64 `json:"blur"`
	JPEG        int     `json:"jpeg"` // 0 => disabled
	Vignette    float64 `json:"vignette"`
	Shadow      float64 `json:"shadow"`
	RotateDeg   float64 `json:"rotateDeg"`
	Perspective float64 `json:"perspective"`
	Contrast    float64 `json:"contrast"`
	Deskew      bool    `json:"deskew"`
	Resample    float64 `json:"resample"` // > 0: downscale then upscale, softens
}

// FlawsSpec adds human imperfection: corrections, blots, smudges.
type FlawsSpec struct {
	LineWobble  float64 `json:"lineWobble"`  // 0..1 baseline wave amplitude
	CrossOut    float64 `json:"crossOut"`    // per-word chance to be struck through
	Correction  float64 `json:"correction"`  // per-word chance to be fixed above the line
	Overwrite   float64 `json:"overwrite"`   // chance a letter is scribbled over
	Smudge      float64 `json:"smudge"`      // chance of a smeared patch per page
	Blot        float64 `json:"blot"`        // chance of an ink blob per page
	Stain       float64 `json:"stain"`       // chance of a coffee/water mark per page
	MarginNote  float64 `json:"marginNote"`  // chance of a note in the margin
	SkipWord    float64 `json:"skipWord"`    // chance to drop a word (and fix it)
	Underline   float64 `json:"underline"`   // chance a line gets underlined
	Crease      float64 `json:"crease"`      // fold line strength
	PressureDot float64 `json:"pressureDot"` // pen-down blobs at stroke starts
}

// LayoutSpec controls flow of text over the page.
type LayoutSpec struct {
	Columns    int     `json:"columns"`
	LineHeight float64 `json:"lineHeight"` // em, 0 = from profile lineGap
	Indent     float64 `json:"indent"`     // em, first-line indent
	Align      string  `json:"align"`      // left | center | justify
	Width      int     `json:"width"`      // page px
	Height     int     `json:"height"`     // page px
	MarginTop  float64 `json:"marginTop"`  // px
	MarginSide float64 `json:"marginSide"` // px
	MarginBot  float64 `json:"marginBot"`  // px
	SkipLines  int     `json:"skipLines"`  // empty lines before text
	PageNumber bool    `json:"pageNumber"`
}

// Page is the final geometry of one page, produced by layout.
type Page struct {
	Index  int
	Width  int
	Height int
	Ops    []GlyphOp
	Lines  []LineMark
	// Flaws holds page-level decorations chosen by layout (smudges, stains).
	Flaws []Flaw
}

// GlyphOp is a single sprite draw with its own randomised transform.
type GlyphOp struct {
	Sprite *Sprite
	X, Y   float64 // px, baseline pen origin
	Scale  float64 // absolute px per em
	Rotate float64 // degrees
	Slant  float64 // degrees (shear)
	Ink    float64 // 0..1 relative ink amount
	Weight float64 // stroke weight multiplier
	Alpha  float64
	Seed   int64
	// Link (may be zero) draws a connecting stroke from the previous glyph.
	Link *LinkOp
	// Strike marks the glyph as part of a struck-out word.
	Strike bool
}

// LinkOp is a pseudo-cursive connector between two pen positions.
type LinkOp struct {
	X0, Y0 float64 // px
	X1, Y1 float64
	Ctrl   Point // px control point
	Width  float64
	Ink    float64
	Kind   string
}

// LineMark is a pen stroke that is not a glyph (underline, strikethrough).
type LineMark struct {
	X0, Y0, X1, Y1 float64
	Width          float64
	Ink            float64
	Kind           string // underline | strike | arrow | bracket
}

// Flaw is a page-level ink-paper accident.
type Flaw struct {
	Kind   string // smudge | blot | stain | crease | fingerprint
	X, Y   float64
	R      float64
	Amount float64
	Rotate float64
}

// GenerateRequest is the API payload for a rendering job.
type GenerateRequest struct {
	Text      string         `json:"text"`
	ProfileID string         `json:"profileId"`
	Seed      int64          `json:"seed"`
	Page      LayoutSpec     `json:"page"`
	Paper     PaperSpec      `json:"paper"`
	Pen       PenSpec        `json:"pen"`
	Style     StyleOverrides `json:"style"`
	Effects   EffectsSpec    `json:"effects"`
	Flaws     FlawsSpec      `json:"flaws"`
	Title     string         `json:"title"`
	// Format is "png" (default), "jpeg" or "pdf".
	Format string `json:"format"`
}

// ---------- Capture (handwriting markup) ----------

// CaptureCell is one candidate region on a scanned sample sheet.
type CaptureCell struct {
	ID     int     `json:"id"`
	Rect   [4]int  `json:"rect"`  // x0,y0,x1,y1 px in the normalised scan
	Label  string  `json:"label"` // assigned character, "" = unused
	Row    int     `json:"row"`   // row index from template grid detection
	Col    int     `json:"col"`
	Score  float64 `json:"score"`  // segmentation confidence
	Manual bool    `json:"manual"` // user-created or user-edited
}

// CaptureRow is a detected ruled line; Baseline is the px y of the baseline.
type CaptureRow struct {
	Index    int     `json:"index"`
	Top      float64 `json:"top"`
	Baseline float64 `json:"baseline"`
	Bottom   float64 `json:"bottom"`
	Label    string  `json:"label"`
}

// CaptureDoc is an analysed sample sheet, sent to the browser for markup.
type CaptureDoc struct {
	ID    string        `json:"id"`
	Image string        `json:"image"` // normalised scan PNG
	W     int           `json:"w"`
	H     int           `json:"h"`
	Cells []CaptureCell `json:"cells"`
	Rows  []CaptureRow  `json:"rows"`
	Em    float64       `json:"em"` // estimated px per em
	Slant float64       `json:"slant"`
	Notes []string      `json:"notes"`
}

// Assignment marks one cell of a CaptureDoc as a character.
type Assignment struct {
	CellID int    `json:"cellId"`
	Label  string `json:"label"`
}

// CommitRequest finishes a manual markup session.
type CommitRequest struct {
	Name        string       `json:"name"`
	Assignments []Assignment `json:"assignments"`
	Style       StyleModel   `json:"style"`
	// MergeInto appends the captured glyphs to an existing profile instead of
	// creating a new one (used to combine several sample-sheet pages).
	MergeInto string `json:"mergeInto"`
}
