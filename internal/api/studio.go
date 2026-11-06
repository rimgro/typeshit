package api

import (
	"bytes"
	"encoding/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"typeshit/internal/model"
	"typeshit/internal/profile"
	"typeshit/internal/render"
)

const (
	maxUploadBytes = 20 << 20
	templateDPI    = 300
)

type templateInfo struct {
	Spec sheetSpecView `json:"spec"`
	Page int           `json:"page"`
	URL  string        `json:"url"`
	W    int           `json:"w"`
	H    int           `json:"h"`
}

// sheetSpecView is the public JSON shape of profile.SheetSpec (which has no
// json tags of its own).
type sheetSpecView struct {
	Lang  string   `json:"lang"`
	Reps  int      `json:"reps"`
	Page  int      `json:"page"`
	Pages int      `json:"pages"`
	DPI   int      `json:"dpi"`
	Chars []string `json:"chars"`
}

func viewSheetSpec(s profile.SheetSpec) sheetSpecView {
	return sheetSpecView{
		Lang:  s.Lang,
		Reps:  s.Reps,
		Page:  s.Page,
		Pages: s.Pages,
		DPI:   s.DPI,
		Chars: s.Chars,
	}
}

// ---------- printable sample sheets ----------

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	if lang == "" {
		lang = "ru"
	}
	reps := 2
	if v := r.URL.Query().Get("reps"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 3 {
			writeErr(w, http.StatusBadRequest, "bad_reps", "reps must be 1, 2 or 3")
			return
		}
		reps = n
	}
	infos, err := s.templates(lang, reps)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "template_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, infos)
}

func (s *Server) templatesDir() string { return filepath.Join(s.Store.Root, "templates") }

// templates builds (and caches) the sample sheets for a charset/repetition pair.
func (s *Server) templates(lang string, reps int) ([]templateInfo, error) {
	key := lang + "-" + strconv.Itoa(reps)
	s.mu.Lock()
	if infos, ok := s.tmpl[key]; ok {
		s.mu.Unlock()
		return infos, nil
	}
	s.mu.Unlock()

	imgs, specs, err := profile.SheetPages(lang, reps)
	if err != nil {
		return nil, err
	}
	if len(imgs) == 0 {
		return nil, errNoTemplates
	}
	dir := s.templatesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	infos := make([]templateInfo, 0, len(imgs))
	for i, img := range imgs {
		name := key + "-" + strconv.Itoa(i) + ".png"
		p := filepath.Join(dir, name)
		if _, statErr := os.Stat(p); statErr != nil {
			if err := writePNGFile(p, img); err != nil {
				return nil, err
			}
		}
		spec := profile.SheetSpec{Lang: lang, Reps: reps, Page: i, Pages: len(imgs), DPI: templateDPI}
		if i < len(specs) {
			spec = specs[i]
		}
		infos = append(infos, templateInfo{
			Spec: viewSheetSpec(spec),
			Page: i,
			URL:  "/api/v1/template/" + lang + "/" + strconv.Itoa(reps) + "/" + strconv.Itoa(i) + ".png",
			W:    img.Bounds().Dx(),
			H:    img.Bounds().Dy(),
		})
	}
	s.mu.Lock()
	s.tmpl[key] = infos
	s.mu.Unlock()
	return infos, nil
}

func (s *Server) handleTemplatePage(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	reps, err := strconv.Atoi(r.PathValue("reps"))
	if err != nil || reps < 1 || reps > 3 {
		writeErr(w, http.StatusNotFound, "not_found", "unknown template")
		return
	}
	pageName := strings.TrimSuffix(r.PathValue("page"), ".png")
	page, err := strconv.Atoi(pageName)
	if err != nil || page < 0 {
		writeErr(w, http.StatusNotFound, "not_found", "unknown template page")
		return
	}
	infos, err := s.templates(lang, reps)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "template_failed", err.Error())
		return
	}
	if page >= len(infos) {
		writeErr(w, http.StatusNotFound, "not_found", "unknown template page")
		return
	}
	p := filepath.Join(s.templatesDir(), lang+"-"+strconv.Itoa(reps)+"-"+strconv.Itoa(page)+".png")
	f, err := os.Open(p)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "template page not found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeContent(w, r, "template.png", st.ModTime(), f)
}

// ---------- capture (handwriting markup) ----------

func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_upload", "expected a multipart form with a file field")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no_file", "multipart field \"file\" is required")
		return
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_image", "cannot decode the image: "+err.Error())
		return
	}

	spec := profile.SheetSpec{DPI: templateDPI}
	if raw := r.FormValue("spec"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &spec); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_spec", "invalid spec JSON: "+err.Error())
			return
		}
	} else {
		spec.Lang = r.FormValue("lang")
		if v := r.FormValue("reps"); v != "" {
			spec.Reps, _ = strconv.Atoi(v)
		}
		spec.Page, _ = strconv.Atoi(r.FormValue("page"))
		spec.Pages, _ = strconv.Atoi(r.FormValue("pages"))
	}
	if spec.DPI == 0 {
		spec.DPI = templateDPI
	}

	doc, norm, err := profile.AnalyzeScan(img, spec)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "capture_failed", err.Error())
		return
	}
	if doc == nil || norm == nil {
		writeErr(w, http.StatusInternalServerError, "capture_failed", "scan analyser returned nothing")
		return
	}
	id := render.NewID("c")
	doc.ID = id
	doc.Image = "/api/v1/capture/" + id + "/image"
	if b := norm.Bounds(); doc.W == 0 || doc.H == 0 {
		doc.W, doc.H = b.Dx(), b.Dy()
	}
	if err := s.Store.WriteCapture(id, norm); err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	if err := s.Store.PutDoc(doc); err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (s *Server) handleCaptureDoc(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.Store.GetDoc(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no capture with id "+id)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handleCaptureImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	img, err := s.Store.ReadCapture(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no capture with id "+id)
		return
	}
	var buf bytes.Buffer
	if err := writePNGTo(&buf, img); err != nil {
		writeErr(w, http.StatusInternalServerError, "encode", err.Error())
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	servePNG(w, buf.Bytes())
}

type cellsBody struct {
	Cells []model.CaptureCell `json:"cells"`
}

func (s *Server) handleCaptureCells(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.Store.GetDoc(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no capture with id "+id)
		return
	}
	var body cellsBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.Cells) > 2000 {
		writeErr(w, http.StatusBadRequest, "too_many_cells", "at most 2000 cells are allowed")
		return
	}
	seen := make(map[int]bool, len(body.Cells))
	cells := make([]model.CaptureCell, 0, len(body.Cells))
	for _, c := range body.Cells {
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		if len([]rune(c.Label)) > 3 {
			writeErr(w, http.StatusBadRequest, "bad_label", "a cell label must be a single character")
			return
		}
		if doc.W > 0 && doc.H > 0 {
			c.Rect = clampRect(c.Rect, doc.W, doc.H)
			if c.Rect[2]-c.Rect[0] < 2 || c.Rect[3]-c.Rect[1] < 2 {
				continue
			}
		}
		cells = append(cells, c)
	}
	doc.Cells = cells
	if err := s.Store.PutDoc(doc); err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func clampRect(rect [4]int, w, h int) [4]int {
	if rect[0] > rect[2] {
		rect[0], rect[2] = rect[2], rect[0]
	}
	if rect[1] > rect[3] {
		rect[1], rect[3] = rect[3], rect[1]
	}
	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	rect[0] = clamp(rect[0], 0, w)
	rect[2] = clamp(rect[2], 0, w)
	rect[1] = clamp(rect[1], 0, h)
	rect[3] = clamp(rect[3], 0, h)
	return rect
}

func (s *Server) handleCaptureCommit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.Store.GetDoc(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no capture with id "+id)
		return
	}
	scan, err := s.Store.ReadCapture(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "the capture image is missing")
		return
	}
	var req model.CommitRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	// The stored markup and the commit payload must agree: apply assignments
	// to the document first, then hand the merged state to the cutter.
	byID := make(map[int]int, len(doc.Cells))
	for i := range doc.Cells {
		byID[doc.Cells[i].ID] = i
	}
	for _, a := range req.Assignments {
		if i, ok := byID[a.CellID]; ok {
			doc.Cells[i].Label = a.Label
		}
	}
	if err := s.Store.PutDoc(doc); err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}

	p, atlases, err := profile.CommitScan(doc, scan, req)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "commit_failed", err.Error())
		return
	}
	if p == nil {
		writeErr(w, http.StatusInternalServerError, "commit_failed", "the cutter returned no profile")
		return
	}
	p.ID = render.NewID("p")
	if strings.TrimSpace(req.Name) != "" {
		p.Name = strings.TrimSpace(req.Name)
	}
	if p.Name == "" {
		p.Name = "Мой почерк"
	}
	if n := []rune(p.Name); len(n) > 80 {
		p.Name = string(n[:80])
	}
	p.Created = time.Now()
	if len(p.Lang) == 0 {
		p.Lang = []string{"ru"}
	}
	if p.Source == "" {
		p.Source = "scan"
	}
	if p.Meta == nil {
		p.Meta = map[string]string{}
	}
	p.Meta["captureId"] = id

	// Appending another sample sheet to an existing handwriting: stack the new
	// variants on top of the old ones and repack the atlases.
	if mergeID := strings.TrimSpace(req.MergeInto); mergeID != "" {
		base, baseAtlas, err := s.Store.LoadProfile(mergeID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not_found", "no profile with id "+mergeID)
			return
		}
		merged, pages, err := profile.Merge([]*model.Profile{base, p}, [][]image.Image{baseAtlas, toImages(atlases)})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "merge_failed", err.Error())
			return
		}
		merged.ID = base.ID
		merged.Name = base.Name
		merged.Created = base.Created
		merged.Preview = ""
		if err := s.persistProfile(merged, pages); err != nil {
			writeErr(w, http.StatusInternalServerError, "io", err.Error())
			return
		}
		s.invalidatePreview(merged.ID)
		writeJSON(w, http.StatusCreated, summarize(merged))
		return
	}

	if err := s.persistProfile(p, atlases); err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, summarize(p))
}

// toImages adapts NRGBA pages to the image.Image list the engine expects.
func toImages(pages []*image.NRGBA) []image.Image {
	out := make([]image.Image, len(pages))
	for i := range pages {
		out[i] = pages[i]
	}
	return out
}
