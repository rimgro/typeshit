const common = `Project: /home/rimgro/Projects/typeshit (Go module \`typeshit\`, Go 1.26 at /usr/local/go/bin — add it to PATH).
FIRST read docs/CONTRACT.md and internal/model/model.go completely. They are authoritative.
Hard rules:
- Only create/edit files inside the paths assigned to you below. Never edit internal/model, internal/store, internal/render, internal/assets, cmd/, go.mod.
- Public signatures in the contract stay exactly as given.
- Dependencies: stdlib + golang.org/x/image only. No cgo, no network.
- \`go build ./...\` must keep passing; \`go vet ./...\` clean; ship *_test.go for your package.
- Deterministic for a given seed (math/rand/v2 + rand.NewPCG).
- Hot pixel loops write into img.Pix directly, never At/Set.
- Comments in English. Final report: files changed, what works, what is approximate.`;

const tasks = [
{
key: "profile",
agent: "worker",
task: `${common}

YOU OWN: internal/profile/** and an optional new internal/glyph/** helper package.
GOAL: the handwriting CAPTURE subsystem — the core differentiator. It must capture a real person's handwriting from a photographed template sheet, keeping several real variants per glyph. It is NOT a font renderer, although BuildFromFont provides the instant-demo path.

1) Charset(lang): "ru" = lowercase абвгдеёжзийклмнопрстуфхцчшщъыьэюя + uppercase + digits 0-9 + punctuation . , ; : ! ? - ( ) « » " ' / \\ * + = ; "ru-short" = lowercase+uppercase+digits; anything else = latin+digits+punct.

2) SheetPages(lang, reps): printable A4 @300dpi (2480x3508) sheets. Per page: light-blue lattice, reps columns x up to 12 rows of cells (~230x250 px); each cell shows the target character faintly (light grey, drawn via golang.org/x/image/font/opentype from an embedded fallback font), a dashed baseline and dashed x-height line, solid black corner registration L-marks, Russian header with instructions and page number, machine-readable footer (lang/reps/page). One page = up to 12 distinct characters, each repeated in all columns. spec.Chars must list characters row-major including repeats so the analyser can auto-label. Full charset spans multiple pages.

3) AnalyzeScan(img, spec) — phone photo -> normalised analysable scan:
   - downscale so max dimension <= 2400 px, grayscale f32/uint8
   - illumination flattening: divide by a box-blurred copy (radius ~ w/12), rescale
   - adaptive threshold via integral image (window ~ w/24)
   - deskew: brute force rotation in [-8,8] deg, 0.25 deg steps, maximise the variance of the horizontal projection of dark pixels; rotate with golang.org/x/image/draw
   - perspective: find the four black registration L-marks (largest dark components near the corners); if found, build the homography to canonical A4 and warp it yourself (inverse map + bilinear); else keep the deskewed image
   - grid detection: projection peaks of ruling/ink, snap the expected rows x cols lattice; per row compute Baseline from the dashed baseline pixels (fallback lattice bottom minus 12%), Top, Bottom
   - fill model.CaptureDoc{Cells, Rows, Em (px per em from cell height + guide x-height), Slant (mean angle of near-vertical ink strokes via column runs + PCA), Notes in Russian}. Auto-label row-major from spec.Chars only for cells that contain ink; empty cells get Label "" and Score 0. When spec.Pages == 0 (manual mode) leave labels empty and segment ink with connected components + projection splitting, Score from compactness.
   - return the normalised image as *image.NRGBA so the API can store it. Robust to crooked, unevenly lit, shadowed photos. Never panic.

4) CommitScan(doc, scan, req): cut sprites. For each labelled cell: coverage mask from darkness relative to the local background (median of the cell border), drop specks far from the cell centre (keep dots of . , ё), trim to ink bbox, map to em units using the row Baseline and doc.Em, compute Box, Advance (ink width + half the mean side bearing), Weight (ink area / skeleton length), Ink (mean coverage), Join{Entry,Exit,Kind} from ink touching the left/right bbox borders: x-height band -> "high", baseline only -> "base", sweeping up -> "up", none -> "none". Each repetition of the same char becomes a separate Variant — this is the real variability. Skip empty/too-small cells.
   Pack sprites into atlas pages (2048 wide, transparent bg, white RGB + coverage in alpha exactly as the contract says). Fill Profile{Glyphs, Em, Source:"scan"|"manual"}.
   Measure StyleModel from the data: median advance/WordGap, x-height, slant, and dispersion across repetitions (ScaleSigma, RotSigma, DySigma, DxSigma, InkMean/InkSigma, WeightMean/WeightSigma, JoinProb, LineGap = row pitch/Em). Clamp to sane ranges. If a glyph has < 3 variants, synthesise extra geometric variants (rotate ±1.5°, scale 0.97..1.03, ±1px thickness via min/max filter, tiny shear) marked Confidence 0.7.

5) BuildFromFont(id, name, fontData, opts): render the charset at em = 160 px with sfnt+opentype, extract coverage alpha, trim, compute Box/Advance/Weight/Ink, Entry/Exit and Join kind heuristics, produce opts.Variants (default 3) geometric variants so pages are not a rubber stamp, pack atlases, Source="font", plausible Style defaults from font metrics (+ assets.FontList slant if opts.Style is zero).

6) Tests (internal/profile/*_test.go): (a) round-trip: SheetPages("ru",2) -> simulate a human by drawing each cell's char into the correct cell with a bundled TTF plus random jitter/rotation/scale -> AnalyzeScan(spec) -> CommitScan -> assert every charset char of that page has >=1 variant, valid atlas bounds, Em within 5%, Style.InkSigma > 0. (b) template determinism/page count. (c) AnalyzeScan on a synthetically rotated (3°) vignetted noised sheet. (d) Charset sanity (no dupes, contains ё/й).

Quality bar: BuildFromFont alone must give a good demo; a captured sheet must give a believable personal handwriting. No magic number without a comment explaining units.`,
},
{
key: "layout",
agent: "worker",
task: `${common}

YOU OWN: internal/layout/** and internal/ink/**.
GOAL: text -> per-glyph placed ops with human variability + a physical ink rasteriser. Do NOT depend on internal/profile (written in parallel); in tests build model.Profile by hand with synthetic sprites whose atlas you generate yourself.

A) layout.Build(req, prof, rng) ([]*model.Page, error)
- Inputs: req.Text (paragraphs by \\n), req.Page (px size, margins, columns, indent, align left|center|justify, lineHeight em, skipLines, page number), req.Style (Scale, Slant, LineGap, WordGap, Variability 0..2, Join), req.Flaws, req.Title, req.Paper, prof.Style, prof.Glyphs.
- em px: fit the column width using the profile's median advance + word gap; honour Style.Scale > 0. Line pitch = LineGap*em, clamped so lines never overlap (>= 1.05 * max sprite height).
- For paper kind lined/grid: align the first baseline to the ruling pitch (CellMM mm -> px = mm*dpi/25.4, dpi = 25.4*page.Width/210) so text sits ON the printed lines; small random phase per page. paper.Render uses the same lattice (line 0 at CellMM*dpi/25.4), so match it exactly.
- Greedy wrap; hyphenate long words at a vowel + '-' (Russian-aware); paragraph indent; blank lines; columns top-to-bottom then left-to-right; page numbers small at the bottom centre.
- Per glyph: variant weighted by Confidence, never the same Variant twice for one char in a row and prefer unseen in the last 3; Advance = sprite.Advance*AdvanceScale*(1+N(0,DxSigma)) plus rare tight/wide letters (5%, ±4%). Baseline = line baseline + N(0,DySigma)*em + wobble.
- Long-range drift: per-line baseline random walk (amplitude from Flaws.LineWobble, default small), per-line slant and size drift, slow per-page sinusoid (period 5-9 lines) so pages read human.
- Transform per glyph: Rotate = N(0,RotSigma), Slant = Style.Slant + N(0,SlantSigma) + drift, Scale jitter, Ink = clamp(InkMean + N(0,InkSigma)), Alpha 1. Uppercase/punctuation jitter slightly less.
- Joins: when prev and cur sprites have Join.Kind != "none", same word, rng < JoinProb -> op.Link cubic from prev.Exit to cur.Entry with overshooting control point ("high" above the midpoint, "base" below), jittered anchors. Word boundaries break joins; inside a word break with LiftProb.
- Flaws (0..1 probabilities, default 0): crossOut (word struck through with 2-3 thick overlapping lines + ops marked Strike), correction (crossed-out word + corrected word above at 0.72 size with caret/arrow, push the following line down if it collides), overwrite (letter scribbled over with a tight zigzag), skipWord (word omitted then re-written above after a caret), underline, margin notes (short Russian note vertically in the margin, slight rotation), pressure dots at stroke starts. Page-level Flaw entries: smudge, blot, stain, crease at plausible positions that do not hide the text centre.
- Punctuation: no space before . , ; : ! ? ) » ; no space after ( « ; spaced em dash; hyphen breaks. Missing glyph -> similar char fallback (case, е/ё, ъ/ь) or skip; never panic.
- Title: centred top, 1.25x, underline when Flaws.Underline is high.
- Ops in painting order, inside the page (a few px of margin bleed is fine).

B) internal/ink: physical ink rasteriser.
- SpriteMask: sprite sub-image + 1px transparent margin, antialiased alpha copy.
- DrawPage(dst, ops): affine transform (scale to op.Scale/em, rotate, shear for Slant, tiny random warp), bilinear sampling, then coverage modulation:
  * ballpoint: 0.80..1.0 coverage, dry-patch value noise, uneven edges, occasional starvation streaks along fast strokes.
  * gel: near-uniform, tiny halo, dense.
  * pencil: grain multiply (0.35..1.0 high-frequency), low density, edge roughness.
  * fountain: darker at turns (mask minus blurred/max-filtered mask), wet halo (blurred low alpha), occasional feathering.
  * pressure: width modulation (separable max/min filter thickening/thinning) + opacity; every glyph gets its own coverage and 1-2 accidental pen skips.
  * colour = pen.Color with per-glyph darkness; alpha = coverage*opacity; composite source-over directly into dst.Pix.
  * cache transformed masks keyed by rounded (scale 0.5 px, rotate 1°, slant 1°) + variant, cap ~4000 entries. Target <= 1.5 s for a dense A4 page.
- Stroke(dst, m): variable-width polyline with pressure profile (thin ends, thicker middle), tremor, ink pooling at the ends, dry tail. Used for underlines, strikes, links, carets, margin notes.

C) Tests: layout determinism, wrapping/margins/columns/pagination, hyphenation, punctuation spacing, nothing off-page, joins only inside words, flaw probabilities roughly right. Ink: black sprite on white gives the expected coverage, rotation stable, ballpoint != pencil, empty input safe, and a timing assert that a synthetic 3000-glyph page renders in under ~1.5 s (generous margin).`,
},
{
key: "paper",
agent: "worker",
task: `${common}

YOU OWN: internal/paper/** and internal/effects/**.
GOAL: the page must look like a photographed sheet, not like a PNG. Pure raster code; only internal/model may be imported.

A) paper.Render(spec, w, h, dpi, rng) *image.NRGBA
- Paper base: spec.Color, fibre texture (hash-grid value noise, 2 octaves), micro speckles, subtle brightness gradient.
- Kinds: "lined" horizontal ruling at spec.CellMM mm pitch (px = mm*dpi/25.4, line 0 exactly at that y), slight waviness, opacity variation, faint printed double edge; "grid" (клетка); "dot"; "blank"; unknown -> lined.
- Margin vertical red-ish line at spec.MarginMM from spec.MarginSide (left/right/none), Soviet-notebook look.
- spec.Aging 0..1: yellowing, uneven oxidation, darker edges, foxing spots, worn corners, faint fingerprints, coffee ring at 0.3+. At 0 the page is clean and must be exactly the flat colour outside the ruling.
- Fast: incremental integer noise into Pix, no per-pixel allocation, A4 well under 200 ms.

B) effects.Apply(img, spec, rng) *image.NRGBA
- "none": return img untouched.
- "scan": desaturate slightly, gentle contrast S-curve, paper whitening, faint horizontal streaks, speckle noise, very slight blur, 1-3 dust specks, thin dark edge when Shadow > 0.
- "phone": rotate by spec.RotateDeg (default random 0.4..1.6) with bilinear sampling on an expanded canvas, perspective tilt, directional light gradient (warm/cool), corner vignette, drop shadow under the page edges, depth-varying slight blur, sensor noise, then a real JPEG round-trip at spec.JPEG (default ~78) with ringing/blocking.
- "fax": heavy contrast clamp, binarise-ish, salt noise, scanlines.
- spec.Resample > 0: downscale then upscale (cheap phone camera softness).
- Ink must stay clearly darker than paper (assert in a test). Never return nil. Deterministic per seed.

C) Tests: dimensions preserved, determinism, exact ruling pitch, ink contrast preserved through phone/fax, aging 0 vs 1 differ, A4 paper render under 500 ms.`,
},
{
key: "api",
agent: "worker",
task: `${common}

YOU OWN: internal/api/** (rewrite api.go, add files) — nothing else.
GOAL: the HTTP surface for the web UI and for autonomous agents. Implement exactly the endpoint table in docs/CONTRACT.md. Read internal/render/render.go, internal/store/store.go, internal/model/*.go, internal/assets/assets.go first. internal/profile is written in parallel — call it through the contract signatures and degrade gracefully (500 + JSON error) when it errors.

- Go 1.22+ pattern routing via http.ServeMux.
- JSON in/out; errors {"error":"...","code":"slug"}; panic-recover middleware; request log to stderr (method, path, status, duration).
- POST /api/v1/generate: decode model.GenerateRequest, limits (text <= 120000 runes, page.Width/Height <= 4000 and > 0, pages <= 40, body <= 32 MB), sync by default -> {id,pages:[url],width,height,millis}; async when "async":true or ?async=1 -> {"jobId"} with a 2-worker pool + queue 64 persisting model.Job; GET /api/v1/jobs/{id} returns the job with page URLs.
- GET /api/v1/results/{id}/{file} serves PNG with immutable cache headers; /export.pdf and /export.zip generated on demand and cached next to the pages. Write both writers yourself with stdlib only: PDF 1.4, one page per rendered page, each image embedded DCTDecode (re-encode JPEG q85); ZIP via archive/zip with the PNGs + README.txt.
- Studio: GET /api/v1/template?lang&reps -> [{spec,page,url}] and caches template PNGs under <data>/templates/{lang}-{reps}-{page}.png; POST /api/v1/capture (multipart "file" + optional "spec" JSON) -> profile.AnalyzeScan, store scan+doc, return doc; GET capture/{id}, GET capture/{id}/image; POST capture/{id}/cells (bulk replace markup); POST capture/{id}/commit -> profile.CommitScan, set ID, Name, Created, Lang, persist via store.SaveProfile, and write a preview test-line PNG with render.PreviewLine into the profile dir (fall back to a blank PNG if it returns nil).
- POST /api/v1/profiles from a bundled font: {name, fontId, variants, lang} using assets.FontData; GET /api/v1/fonts: assets.FontList + "cyrillic": bool computed once via the font cmap for а/я/ё (cached).
- GET /api/v1/profiles/{id}, DELETE, GET atlas image, GET preview?text= via render.PreviewLine.
- GET /api/v1/openapi.json: a compact but valid OpenAPI 3.1 document for the above.
- Static: internal/webui.FS() at "/", SPA fallback to index.html for unknown non-API paths, no directory listing.
- Bound concurrent renders with a semaphore of runtime.NumCPU(); mutex around shared caches; server timeouts.
- Tests (httptest, -race safe): health, unknown route returns JSON 404, limit rejection, generate with a hand-built profile stored via store.SaveProfile under t.TempDir() (do not rely on internal/profile), async job lifecycle, zip/pdf magic bytes, static serving. Skip pipeline-dependent assertions when render returns an error/blank page while the other packages are still stubs.`,
},
{
key: "webui",
agent: "worker",
task: `${common}

YOU OWN: internal/webui/static/** only (index.html, handwriting.html, help.html, app.css, *.js). No build step, no CDN, no frameworks, system font stack only.
GOAL: a polished, fast, mobile-friendly Russian UI for the product against /api/v1/... (see docs/CONTRACT.md). Keep JS in separate files and verify each with \`node --check\`.

1) index.html — генератор:
   - Left column: auto-growing textarea, char counter, "вставить пример", title field.
   - Profile picker: cards with preview thumbnails (/api/v1/profiles/{id}/preview?text=...), a "создать свой почерк" card -> handwriting.html, and a "быстрый демо-профиль" button that POSTs /api/v1/profiles with the first /api/v1/fonts entry (variants 3) when no profile exists.
   - Collapsible control groups with real Russian labels: Бумага (kind: тетрадь в линейку/клетка/точки/чистая, cellMm, tint, margin side/width, aging), Ручка (шариковая/гелевая/карандаш/перьевая, colour swatches, width, density, pressure, bleed), Стиль (scale, slant, lineGap, wordGap, variability 0..2, cursive join 0..1), Ошибки и помарки (crossOut, correction, overwrite, smudge, blot, stain, marginNote, underline, crease, lineWobble, note that 0 = чисто), Эффекты (none/scan/phone/fax, noise, blur, jpeg, vignette, shadow, rotate, perspective, resample).
   - Buttons: "Сгенерировать" (POST /generate, busy state with elapsed ms), download PNG/PDF/ZIP links, seed field + dice button.
   - Output: page thumbnails grid, click -> fullscreen viewer with zoom/pan and prev/next, page count, render time.
   - Cheap auto-preview: debounce 600 ms, first page only at ~700x990, shown in a preview pane.
2) handwriting.html — студия почерка (key screen):
   - Step 1: charset (ru / ru-short / latin) + reps (1..3), download template pages from /api/v1/template as a gallery with a print button + print stylesheet.
   - Step 2: upload photo/scan (drag&drop, file input, capture attribute for mobile) -> POST multipart /api/v1/capture with the chosen spec.
   - Step 3: markup canvas (HiDPI, pan with drag/space, wheel/pinch zoom, crisp): each detected cell as a rounded rect with its label. Click a cell then press a key to set the label (Cyrillic/Latin/digits/punctuation from e.key), Backspace clears, Escape deselects; drag to move; corner handles resize; double-click deletes; toolbar "+" or shift-drag adds a manual cell; arrow keys nudge; detected row baselines drawn as draggable lines. Side panel groups cells by character with counts and jump-to; a "проверить" report lists missing charset characters. Local undo/redo.
   - Step 4: "Сохранить ячейки" (POST /cells), name field, "Создать почерк" (POST /commit) -> new profile card with a test line (/preview?text=...) and a link back to the generator with it preselected.
3) help.html — how to write the sample (pen, light, no shadows, fill the box), how to photograph (top-down, no flash, page fills frame), curl examples for agents, endpoint list.
4) Design: paper-ish background (#f6f2ea), white cards, one accent (#1b2a4a), 8px spacing scale, 12px radii, 1px borders, accessible focus states, dark mode via prefers-color-scheme. Responsive: controls collapse under 900px. Text or inline SVG buttons, no emoji-only.
5) Robustness: no unhandled promise rejections, toast on every failed fetch using the server's {"error"}, skeletons while loading, disable buttons while pending, remember settings in localStorage.

Verify: \`node --check\` every .js, no broken references, and if the server builds, run it on 127.0.0.1:8099, curl the pages and API, fix what is obviously broken.`,
},
];

const res = await runs.all(tasks.map((t) => ({ key: t.key, agent: t.agent, task: t.task, async: false })));
const out = {};
for (const r of res) out[r.key] = String(r.output ?? "").slice(0, 3000);
return out;
