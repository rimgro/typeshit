# CONTRACT — handwriting synthesis engine ("typeshit")

Authoritative spec. Every package must compile with `go build ./...` at all times.
Module path: `typeshit`. Go 1.26. Toolchain lives at `/usr/local/go/bin` (add to PATH).

Allowed dependencies: standard library + `golang.org/x/image` (font/sfnt, font/opentype,
math/f64, draw, vector, etc.). Nothing else. No cgo.

## What the product is

A self-hosted analogue of texttoimages.pro: it turns typed text into photorealistic
scans of handwritten pages. The differentiator is that the **handwriting is captured
from a real person** — the user writes on a printed template sheet, photographs it,
the server segments every letter into a sprite, and the renderer recomposes new text
from those sprites with statistical variability, pseudo-cursive joins, ink physics,
paper, and human mistakes.

There are three rendering layers:

1. **glyph sprites** (`internal/profile`) — real ink cut from a sample, normalised to
   em units, several variants per character.
2. **layout** (`internal/layout`) — text → per-glyph placement with randomised
   transforms, links, flaws, pagination.
3. **raster** (`internal/paper`, `internal/ink`, `internal/effects`) — background,
   ink deposition, capture simulation.

## Coordinate systems

- **em units**: `1.0 == font em`. Origin at the pen position *on the baseline*,
  X right, Y **up**. `model.Rect{X,Y,W,H}` → left, bottom-above-baseline, size.
  Descenders are negative Y. `model.Sprite.Advance` is the pen advance after the glyph.
- **px units**: raster pixels of the page, Y **down** (image convention), origin top-left.
  `model.GlyphOp.X/Y` is the baseline pen origin in px for that glyph.
- `GlyphOp.Scale` = **absolute** px per em (so glyph px height ≈ `(Box.H)*Scale`).

Never mix the two in one struct; `Box`/`Advance`/`Join` are em, everything else is px.

## Atlas format

One atlas PNG = `image.NRGBA`, transparent background, white (`255,255,255`) RGB where
ink coverage > 0, and the **alpha channel carries the ink coverage mask**. Sprite `Pix`
is `[x0,y0,x1,y1]` (Y down) inside atlas page `Atlas`; edges must be fully transparent
so bilinear resampling does not bleed.

## Package APIs (do not change these signatures)

### internal/profile (owner: agent A)

```go
type FontOptions struct {
	Lang     string
	Variants int
	Seed     int64
	Style    model.StyleModel
}
type SheetSpec struct {
	Lang  string
	Reps  int
	Page  int
	Pages int
	DPI   int
	Chars []string // chars on this page, row-major
}

func BuildFromFont(id, name string, fontData []byte, opts FontOptions) (*model.Profile, []*image.NRGBA, error)
func SheetPages(lang string, reps int) ([]*image.NRGBA, []SheetSpec, error)
func AnalyzeScan(img image.Image, spec SheetSpec) (*model.CaptureDoc, *image.NRGBA, error)
func CommitScan(doc *model.CaptureDoc, scan *image.NRGBA, req model.CommitRequest) (*model.Profile, []*image.NRGBA, error)
func Charset(lang string) []string
```

### internal/layout (owner: agent B)

```go
func Build(req *model.GenerateRequest, prof *model.Profile, rng *rand.Rand) ([]*model.Page, error)
// rand == math/rand/v2
```

### internal/ink (owner: agent B)

```go
func New(prof *model.Profile, atlases []image.Image, pen model.PenSpec, rng *rand.Rand) *Canvas
func (c *Canvas) DrawPage(dst *image.NRGBA, ops []model.GlyphOp)
func (c *Canvas) Stroke(dst *image.NRGBA, m model.LineMark)
func SpriteMask(sp *model.Sprite, atlas image.Image) *image.Alpha
```

### internal/paper (owner: agent C)

```go
func Render(spec model.PaperSpec, w, h int, dpi float64, rng *rand.Rand) *image.NRGBA
```

### internal/effects (owner: agent C)

```go
func Apply(img *image.NRGBA, spec model.EffectsSpec, rng *rand.Rand) *image.NRGBA
```

### internal/render (owner: lead, already written)

`Engine.Render(req) (*Result, error)` → loads profile from `internal/store`,
`Defaults()`, `layout.Build`, `paper.Render`, `ink.DrawPage/Stroke`,
`effects.Apply`, saves pages. Also `render.PreviewLine(prof, atlases, text, style, w, h) *image.NRGBA`
(lead implements) used for profile previews.

### internal/store (owner: lead, already written)

`Open(root)`, `SaveProfile(p, atlases)`, `LoadProfile(id) (*model.Profile, []image.Image, error)`,
`ListProfiles()`, `DeleteProfile(id)`, `WriteCapture/ReadCapture`, `PutDoc/GetDoc`,
`NewResultDir`, `ResultPath`, `SavePage`, `PutJob/GetJob`.

## HTTP API (owner: agent D) — exact paths

All JSON under `/api/v1`. Errors: `{"error":"message","code":"slug"}` with 4xx/5xx.
Everything else is served from the embedded web UI (`internal/webui.FS()`), with SPA
fallback to `index.html`.

| Method | Path | Body / Query | Response |
|---|---|---|---|
| GET | `/api/v1/health` | – | `{"ok":true,"version":"..."}` |
| GET | `/api/v1/fonts` | – | `[{"id","name","slant","cursive","cyrillic":bool}]` |
| GET | `/api/v1/profiles` | – | `[{profile summary}]` |
| POST | `/api/v1/profiles` | `{"name","fontId","variants"}` | `{profile}` (built from font) |
| GET | `/api/v1/profiles/{id}` | – | `{profile}` |
| DELETE | `/api/v1/profiles/{id}` | – | `{"ok":true}` |
| GET | `/api/v1/profiles/{id}/atlas/{name}` | – | image/png |
| GET | `/api/v1/profiles/{id}/preview` | `text` | image/png test line |
| GET | `/api/v1/template` | `lang=ru&reps=2` | `[{spec,image}]` (JSON, image = path) |
| GET | `/api/v1/template/{lang}/{reps}/{page}.png` | – | image/png |
| POST | `/api/v1/capture` | multipart `file`, optional `spec` JSON `SheetSpec` | `{doc}` (`CaptureDoc`) |
| GET | `/api/v1/capture/{id}` | – | `{doc}` |
| GET | `/api/v1/capture/{id}/image` | – | image/png normalised scan |
| POST | `/api/v1/capture/{id}/cells` | `{"cells":[{id,rect,label,manual}]}` | `{doc}` (bulk replace markup) |
| POST | `/api/v1/capture/{id}/commit` | `model.CommitRequest` | `{profile}` |
| POST | `/api/v1/generate` | `model.GenerateRequest` (+ `"async":bool`) | `{id,pages:[url],width,height,millis}` or `{jobId}` |
| GET | `/api/v1/jobs/{id}` | – | `model.Job` (plus `pages` urls) |
| GET | `/api/v1/results/{id}/{file}` | – | image/png |
| GET | `/api/v1/results/{id}/export.pdf` | – | application/pdf |
| GET | `/api/v1/results/{id}/export.zip` | – | application/zip |
| GET | `/api/v1/openapi.json` | – | small OpenAPI 3.1 doc |

Limits: text ≤ 120 000 runes, pages ≤ 40, request body ≤ 32 MB, upload ≤ 20 MB.
Concurrent renders bounded (semaphore of `runtime.NumCPU()`), async jobs in a worker
pool of 2 with queue 64. Result files: `data/results/<id>/page-001.png`; PDF/ZIP are
generated on demand and cached next to the pages.

## Web UI (owner: agent E) — `internal/webui/static/`

Vanilla HTML/CSS/JS, **no build step, no CDN, no external fonts** (system font stack).
Russian UI. Three views:

1. `index.html` — generator: text area, profile picker, pen/paper/style/flaws controls,
   live preview (debounced `POST /api/v1/generate`, `scale` low), download PNG/PDF/ZIP.
2. `handwriting.html` — handwriting studio: download template, upload photo, canvas
   markup (pan/zoom, click a cell → type the letter, drag/resize/delete/add cells,
   set rows/baselines), commit → profile, live test-line preview.
3. `help.html` — how to photograph a sample well + API usage snippet.

Design: light "paper" theme, generous spacing, no gradients-for-gradients, mobile ok.

## Quality bar

- Deterministic: identical `seed` + request → identical bytes.
- Performance: A4 @ 1240×1754 ≤ 1.5 s/page, 40 pages ≤ 40 s.
- Realism is the acceptance criterion: pages must look like a photographed notebook
  page, letters must not be a repeating rubber stamp.
- Each package ships `*_test.go` covering its core math/behaviour.
- `go vet ./...` clean.

## Style rules

- No panics on user input; return errors.
- No global mutable state; pass `*rand.Rand` for determinism.
- Preallocate slices in hot loops; avoid per-pixel `Set()` on NRGBA, write to `Pix`.
- Comments in English, UI strings in Russian.
