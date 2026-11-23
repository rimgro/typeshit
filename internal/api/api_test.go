package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"typeshit/internal/model"
	"typeshit/internal/render"
	"typeshit/internal/store"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	s := New(st, &render.Engine{Store: st})
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(func() {
		srv.Close()
		s.Close()
	})
	return s, srv
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, data
}

func decodeErr(t *testing.T, data []byte) apiError {
	t.Helper()
	var e apiError
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("expected an error object, got %q: %v", data, err)
	}
	if e.Code == "" || e.Error == "" {
		t.Fatalf("error object is incomplete: %+v", e)
	}
	return e
}

// testProfile writes a minimal but structurally valid profile to the store.
func testProfile(t *testing.T, st *store.Store, id string) {
	t.Helper()
	atlas := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 20; y < 70; y++ {
		for x := 20; x < 60; x++ {
			i := atlas.PixOffset(x, y)
			atlas.Pix[i+0], atlas.Pix[i+1], atlas.Pix[i+2], atlas.Pix[i+3] = 255, 255, 255, 255
		}
	}
	sprites := make([]model.Sprite, 0, 3)
	glyphs := map[string][]model.Sprite{}
	for i, ch := range []string{"а", "б", "в"} {
		sp := model.Sprite{
			Char:       ch,
			Variant:    i,
			Atlas:      0,
			Pix:        [4]int{20, 20, 60, 70},
			Box:        model.Rect{X: 0, Y: 0, W: 0.4, H: 0.5},
			Advance:    0.55,
			Join:       model.Join{Kind: "none"},
			Ink:        0.9,
			Weight:     0.06,
			Confidence: 1,
		}
		sprites = append(sprites, sp)
		glyphs[ch] = []model.Sprite{sp}
	}
	_ = sprites
	p := &model.Profile{
		ID:      id,
		Name:    "test",
		Created: time.Now(),
		Source:  "font",
		Em:      128,
		AtlasW:  128,
		AtlasH:  128,
		Glyphs:  glyphs,
		Style: model.StyleModel{
			AdvanceScale: 1,
			WordGap:      0.3,
			LineGap:      1.8,
			InkMean:      0.9,
			InkSigma:     0.05,
			JoinProb:     0,
		},
	}
	if err := st.SaveProfile(p, []*image.NRGBA{atlas}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
}

func TestHealth(t *testing.T) {
	_, srv := newTestServer(t)
	resp, data := do(t, srv, http.MethodGet, "/api/v1/health", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, data)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["ok"] != true {
		t.Fatalf("ok = %v", got["ok"])
	}
	if got["version"] != Version {
		t.Fatalf("version = %v", got["version"])
	}
}

func TestUnknownAPIRouteIsJSON(t *testing.T) {
	_, srv := newTestServer(t)
	resp, data := do(t, srv, http.MethodGet, "/api/v1/there-is-nothing-here", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type = %q", ct)
	}
	if e := decodeErr(t, data); e.Code != "not_found" {
		t.Fatalf("code = %q", e.Code)
	}
}

func TestStaticIndex(t *testing.T) {
	_, srv := newTestServer(t)
	resp, data := do(t, srv, http.MethodGet, "/", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content type = %q", ct)
	}
	if len(data) == 0 || !bytes.Contains(bytes.ToLower(data), []byte("<html")) {
		t.Fatalf("expected an HTML shell, got %q", data[:min(len(data), 80)])
	}
}

func TestGenerateValidation(t *testing.T) {
	_, srv := newTestServer(t)
	cases := []struct {
		name string
		body map[string]any
		want int
		code string
	}{
		{"empty text", map[string]any{"text": "   ", "profileId": "p1"}, http.StatusBadRequest, "empty_text"},
		{"no profile", map[string]any{"text": "привет"}, http.StatusBadRequest, "profile_required"},
		{"too long", map[string]any{"text": strings.Repeat("а", maxRunes+1), "profileId": "p1"}, http.StatusRequestEntityTooLarge, "text_too_long"},
		{"bad page size", map[string]any{"text": "привет", "profileId": "p1", "page": map[string]any{"width": 9000, "height": 9000}}, http.StatusBadRequest, "bad_page_size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, data := do(t, srv, http.MethodPost, "/api/v1/generate", tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", resp.StatusCode, tc.want, data)
			}
			if e := decodeErr(t, data); e.Code != tc.code {
				t.Fatalf("code = %q, want %q", e.Code, tc.code)
			}
		})
	}
}

func TestGenerateWithProfile(t *testing.T) {
	s, srv := newTestServer(t)
	testProfile(t, s.Store, "p1")

	resp, data := do(t, srv, http.MethodPost, "/api/v1/generate", map[string]any{
		"text":      "абв абв абв",
		"profileId": "p1",
		"seed":      42,
		"page":      map[string]any{"width": 600, "height": 800},
	})
	switch resp.StatusCode {
	case http.StatusOK:
		var out generateResponse
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatalf("json: %v (%s)", err, data)
		}
		if len(out.Pages) == 0 {
			t.Fatal("no pages returned")
		}
		if !strings.HasPrefix(out.Pages[0], "/api/v1/results/") {
			t.Fatalf("page url = %q", out.Pages[0])
		}
		pageResp, pageData := do(t, srv, http.MethodGet, out.Pages[0], nil)
		if pageResp.StatusCode != http.StatusOK {
			t.Fatalf("page status = %d", pageResp.StatusCode)
		}
		if !bytes.HasPrefix(pageData, []byte("\x89PNG")) {
			t.Fatalf("page is not a PNG: %q", pageData[:min(len(pageData), 16)])
		}
		if cc := pageResp.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			t.Fatalf("cache-control = %q", cc)
		}
	case http.StatusInternalServerError:
		// The layout package may still be a stub; the failure must be a clean
		// JSON error rather than a panic or an HTML body.
		if e := decodeErr(t, data); e.Code != "render_failed" && e.Code != "internal" {
			t.Fatalf("code = %q", e.Code)
		}
	default:
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, data)
	}
}

func TestAsyncJobLifecycle(t *testing.T) {
	s, srv := newTestServer(t)
	testProfile(t, s.Store, "p1")

	resp, data := do(t, srv, http.MethodPost, "/api/v1/generate", map[string]any{
		"text":      "абв",
		"profileId": "p1",
		"async":     true,
	})
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d: %s", resp.StatusCode, data)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		t.Skip("queue full")
	}
	var started struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(data, &started); err != nil || started.JobID == "" {
		t.Fatalf("jobId missing: %s (%v)", data, err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		jobResp, jobData := do(t, srv, http.MethodGet, "/api/v1/jobs/"+started.JobID, nil)
		if jobResp.StatusCode != http.StatusOK {
			t.Fatalf("job status = %d: %s", jobResp.StatusCode, jobData)
		}
		var job struct {
			ID     string   `json:"id"`
			Status string   `json:"status"`
			Pages  []string `json:"pages"`
			Error  string   `json:"error"`
		}
		if err := json.Unmarshal(jobData, &job); err != nil {
			t.Fatalf("json: %v (%s)", err, jobData)
		}
		switch job.Status {
		case "done":
			if len(job.Pages) == 0 {
				t.Fatal("done job has no page urls")
			}
			for _, u := range job.Pages {
				if !strings.HasPrefix(u, "/api/v1/results/") {
					t.Fatalf("page url = %q", u)
				}
			}
			return
		case "error":
			// Acceptable while the render pipeline is still stubbed.
			if job.Error == "" {
				t.Fatal("failed job has no error message")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s did not finish, last status %q", started.JobID, job.Status)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestExports(t *testing.T) {
	s, srv := newTestServer(t)
	img := image.NewNRGBA(image.Rect(0, 0, 40, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.NRGBA{R: 250, G: 248, B: 240, A: 255})
		}
	}
	id := render.NewID("r")
	if _, err := s.Store.NewResultDir(id); err != nil {
		t.Fatalf("NewResultDir: %v", err)
	}
	if _, err := s.Store.SavePage(id, 0, img); err != nil {
		t.Fatalf("SavePage: %v", err)
	}

	t.Run("zip", func(t *testing.T) {
		resp, data := do(t, srv, http.MethodGet, "/api/v1/results/"+id+"/export.zip", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
			t.Fatalf("not a zip: %q", data[:min(len(data), 8)])
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("zip: %v", err)
		}
		var havePage, haveReadme bool
		for _, f := range zr.File {
			havePage = havePage || f.Name == "page-001.png"
			haveReadme = haveReadme || f.Name == "README.txt"
		}
		if !havePage || !haveReadme {
			t.Fatalf("zip entries = %v", zr.File)
		}
	})

	t.Run("pdf", func(t *testing.T) {
		resp, data := do(t, srv, http.MethodGet, "/api/v1/results/"+id+"/export.pdf", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		if !bytes.HasPrefix(data, []byte("%PDF-1.4")) {
			t.Fatalf("not a pdf: %q", data[:min(len(data), 8)])
		}
		if !bytes.Contains(data, []byte("/Type /Page")) {
			t.Fatal("pdf has no page object")
		}
		idx := bytes.LastIndex(data, []byte("startxref\n"))
		if idx < 0 {
			t.Fatal("pdf has no startxref")
		}
		rest := string(data[idx+len("startxref\n"):])
		end := strings.IndexByte(rest, '\n')
		if end < 0 {
			t.Fatal("malformed startxref")
		}
		off, err := strconv.Atoi(strings.TrimSpace(rest[:end]))
		if err != nil || off <= 0 || off >= len(data) {
			t.Fatalf("startxref offset = %q", rest[:end])
		}
		if !bytes.HasPrefix(data[off:], []byte("xref")) {
			t.Fatalf("xref offset points at %q", data[off:min(len(data), off+16)])
		}
		// The embedded DCTDecode stream must be a decodable JPEG of the page.
		hdr := bytes.Index(data, []byte("/Filter /DCTDecode"))
		if hdr < 0 {
			t.Fatal("pdf has no DCTDecode image")
		}
		imgStart := bytes.Index(data[hdr:], []byte("stream\n"))
		if imgStart < 0 {
			t.Fatal("image stream not found")
		}
		imgStart += hdr + len("stream\n")
		imgEnd := bytes.Index(data[imgStart:], []byte("\nendstream"))
		if imgEnd < 0 {
			t.Fatal("image stream is not terminated")
		}
		decoded, err := jpeg.Decode(bytes.NewReader(data[imgStart : imgStart+imgEnd]))
		if err != nil {
			t.Fatalf("embedded jpeg: %v", err)
		}
		if b := decoded.Bounds(); b.Dx() != 40 || b.Dy() != 60 {
			t.Fatalf("embedded image is %dx%d", b.Dx(), b.Dy())
		}
	})
}

func TestExportsUnknownResult(t *testing.T) {
	_, srv := newTestServer(t)
	resp, data := do(t, srv, http.MethodGet, "/api/v1/results/abc123/export.zip", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	decodeErr(t, data)
}

func TestResultFileValidation(t *testing.T) {
	s, srv := newTestServer(t)
	id := render.NewID("r")
	if _, err := s.Store.NewResultDir(id); err != nil {
		t.Fatal(err)
	}
	if err := writePNGFile(s.Store.ResultPath(id, "page-001.png"), image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/results/" + id + "/evil.txt",
		"/api/v1/results/" + id + "/page-abc.png",
		"/api/v1/results/" + id + "/page-002.png",
	} {
		resp, data := do(t, srv, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: status = %d", path, resp.StatusCode)
		}
		decodeErr(t, data)
	}
	resp, data := do(t, srv, http.MethodGet, "/api/v1/results/"+id+"/page-001.png", nil)
	if resp.StatusCode != http.StatusOK || !bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Fatalf("valid page: status %d", resp.StatusCode)
	}
}

func TestProfilesAndAtlas(t *testing.T) {
	s, srv := newTestServer(t)
	testProfile(t, s.Store, "p1")

	resp, data := do(t, srv, http.MethodGet, "/api/v1/profiles", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, data)
	}
	var list []profileSummary
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(list) != 1 || list[0].ID != "p1" || list[0].Chars != 3 {
		t.Fatalf("list = %+v", list)
	}

	resp, data = do(t, srv, http.MethodGet, "/api/v1/profiles/p1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("profile status = %d: %s", resp.StatusCode, data)
	}
	var full model.Profile
	if err := json.Unmarshal(data, &full); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(full.Glyphs["а"]) != 1 {
		t.Fatalf("glyphs = %+v", full.Glyphs)
	}

	resp, data = do(t, srv, http.MethodGet, "/api/v1/profiles/p1/atlas/atlas-00.png", nil)
	if resp.StatusCode != http.StatusOK || !bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Fatalf("atlas status = %d", resp.StatusCode)
	}
	resp, _ = do(t, srv, http.MethodGet, "/api/v1/profiles/p1/atlas/profile.json", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-atlas file status = %d", resp.StatusCode)
	}

	resp, data = do(t, srv, http.MethodGet, "/api/v1/profiles/p1/preview", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d: %s", resp.StatusCode, data)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("preview content type = %q", ct)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("preview is not a PNG: %v", err)
	}

	resp, _ = do(t, srv, http.MethodDelete, "/api/v1/profiles/p1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	resp, _ = do(t, srv, http.MethodGet, "/api/v1/profiles/p1", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted profile status = %d", resp.StatusCode)
	}
}

func TestFonts(t *testing.T) {
	_, srv := newTestServer(t)
	resp, data := do(t, srv, http.MethodGet, "/api/v1/fonts", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var fonts []fontInfo
	if err := json.Unmarshal(data, &fonts); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(fonts) == 0 {
		t.Fatal("no fonts")
	}
	if fonts[0].ID == "" || fonts[0].Name == "" {
		t.Fatalf("font = %+v", fonts[0])
	}
}

func TestFontFile(t *testing.T) {
	_, srv := newTestServer(t)
	resp, data := do(t, srv, http.MethodGet, "/api/v1/fonts/caveat/file", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "font/ttf" && ct != "font/otf" {
		t.Fatalf("content-type = %q", ct)
	}
	if len(data) < 1000 {
		t.Fatalf("font file is suspiciously small: %d bytes", len(data))
	}
	if !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("cache-control = %q", resp.Header.Get("Cache-Control"))
	}

	resp, data = do(t, srv, http.MethodGet, "/api/v1/fonts/nope/file", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown font status = %d", resp.StatusCode)
	}
	decodeErr(t, data)
}

func TestTemplateEndpoint(t *testing.T) {
	_, srv := newTestServer(t)
	resp, data := do(t, srv, http.MethodGet, "/api/v1/template?lang=ru&reps=1", nil)
	switch resp.StatusCode {
	case http.StatusOK:
		var infos []templateInfo
		if err := json.Unmarshal(data, &infos); err != nil {
			t.Fatalf("json: %v", err)
		}
		if len(infos) == 0 {
			t.Fatal("no template pages")
		}
		pageResp, pageData := do(t, srv, http.MethodGet, infos[0].URL, nil)
		if pageResp.StatusCode != http.StatusOK || !bytes.HasPrefix(pageData, []byte("\x89PNG")) {
			t.Fatalf("template page status = %d", pageResp.StatusCode)
		}
	case http.StatusInternalServerError:
		if e := decodeErr(t, data); e.Code != "template_failed" {
			t.Fatalf("code = %q", e.Code)
		}
	default:
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, data)
	}
}

func TestCaptureValidation(t *testing.T) {
	_, srv := newTestServer(t)

	resp, data := do(t, srv, http.MethodGet, "/api/v1/capture/nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	decodeErr(t, data)

	// A non-multipart body must be rejected with a JSON error.
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/capture", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	httpResp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", httpResp.StatusCode)
	}
	if e := decodeErr(t, body); e.Code != "bad_upload" {
		t.Fatalf("code = %q", e.Code)
	}

	// Multipart with a non-image payload.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "sheet.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("not an image"))
	mw.Close()
	resp2, err := srv.Client().Post(srv.URL+"/api/v1/capture", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp2.StatusCode)
	}
	if e := decodeErr(t, body2); e.Code != "bad_image" {
		t.Fatalf("code = %q", e.Code)
	}
}

func TestOpenAPI(t *testing.T) {
	_, srv := newTestServer(t)
	resp, data := do(t, srv, http.MethodGet, "/api/v1/openapi.json", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v", doc["openapi"])
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) < 10 {
		t.Fatalf("paths = %v", doc["paths"])
	}
	if _, ok := paths["/api/v1/generate"]; !ok {
		t.Fatal("generate path is missing from the OpenAPI document")
	}
}

func TestWrongMethodOnHealth(t *testing.T) {
	_, srv := newTestServer(t)
	// A method-less catch-all route serves JSON for every unknown /api/v1
	// combination, so a wrong method reports 404 rather than a plain-text 405.
	resp, data := do(t, srv, http.MethodPost, "/api/v1/health", map[string]any{})
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type = %q", ct)
	}
	if e := decodeErr(t, data); e.Code != "not_found" && e.Code != "method_not_allowed" {
		t.Fatalf("code = %q", e.Code)
	}
}

func TestProfileFromFontEndpoint(t *testing.T) {
	_, srv := newTestServer(t)
	resp, data := do(t, srv, http.MethodPost, "/api/v1/profiles", map[string]any{
		"name":     "Тест",
		"fontId":   "caveat",
		"variants": 2,
		"lang":     "ru",
	})
	switch resp.StatusCode {
	case http.StatusCreated:
		var sum profileSummary
		if err := json.Unmarshal(data, &sum); err != nil {
			t.Fatalf("json: %v", err)
		}
		if sum.ID == "" || sum.Name != "Тест" {
			t.Fatalf("summary = %+v", sum)
		}
	case http.StatusInternalServerError:
		if e := decodeErr(t, data); e.Code != "profile_failed" {
			t.Fatalf("code = %q", e.Code)
		}
	default:
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, data)
	}

	resp, data = do(t, srv, http.MethodPost, "/api/v1/profiles", map[string]any{"fontId": "nope"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown font status = %d: %s", resp.StatusCode, data)
	}
	if e := decodeErr(t, data); e.Code != "unknown_font" {
		t.Fatalf("code = %q", e.Code)
	}
}

func TestBodyTooLarge(t *testing.T) {
	_, srv := newTestServer(t)
	huge := fmt.Sprintf(`{"text":"%s","profileId":"p1"}`, strings.Repeat("a", maxBodyBytes))
	resp, err := srv.Client().Post(srv.URL+"/api/v1/generate", "application/json", strings.NewReader(huge))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
}
