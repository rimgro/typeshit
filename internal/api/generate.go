package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"typeshit/internal/model"
	"typeshit/internal/render"
)

const (
	jobWorkers   = 2
	jobQueueSize = 64
	maxRunes     = 120000
	maxPagePx    = 4000
	maxPages     = 40
)

// generateBody is the POST /api/v1/generate payload.
type generateBody struct {
	model.GenerateRequest
	Async bool `json:"async"`
}

// generateResponse is returned for synchronous renders.
type generateResponse struct {
	ID        string   `json:"id"`
	Pages     []string `json:"pages"`
	Width     int      `json:"width"`
	Height    int      `json:"height"`
	Millis    int64    `json:"millis"`
	PageCount int      `json:"pageCount"`
}

// jobView is model.Job with API URLs instead of bare file names.
type jobView struct {
	model.Job
	Pages     []string `json:"pages"`
	PageFiles []string `json:"pageFiles"`
}

type queuedJob struct {
	job *model.Job
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	profiles, _ := s.Store.ListProfiles()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"version":  Version,
		"profiles": len(profiles),
	})
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	var body generateBody
	if !decodeJSON(w, r, &body) {
		return
	}
	req := &body.GenerateRequest
	if code, slug, msg := validateGenerate(req); code != 0 {
		writeErr(w, code, slug, msg)
		return
	}
	async := body.Async || r.URL.Query().Get("async") == "1" || r.URL.Query().Get("async") == "true"
	if async {
		job, err := s.enqueue(req)
		if err != nil {
			writeErr(w, http.StatusTooManyRequests, "queue_full", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"jobId": job.ID, "status": job.Status})
		return
	}

	res, err := s.renderSync(req)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "render_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, generateResponse{
		ID:        res.ID,
		Pages:     pageURLs(res.ID, res.Names),
		Width:     res.Width,
		Height:    res.Height,
		Millis:    res.Millis,
		PageCount: len(res.Pages),
	})
}

// renderSync runs one render under the global concurrency semaphore.
func (s *Server) renderSync(req *model.GenerateRequest) (*render.Result, error) {
	s.acquire()
	defer s.release()

	res, err := s.Engine.Render(req)
	if err != nil {
		return nil, err
	}
	if len(res.Pages) == 0 {
		return nil, fmt.Errorf("nothing to render: empty text")
	}
	if len(res.Pages) > maxPages {
		// Keep the store clean; the request simply asked for too much.
		_ = os.RemoveAll(filepath.Join(s.Store.Root, "results", res.ID))
		return nil, fmt.Errorf("document needs %d pages, the limit is %d (shorten the text or enlarge the page)", len(res.Pages), maxPages)
	}
	return res, nil
}

// validateGenerate returns a non-zero status code when the request is invalid.
func validateGenerate(req *model.GenerateRequest) (code int, slug, msg string) {
	if strings.TrimSpace(req.Text) == "" {
		return http.StatusBadRequest, "empty_text", "text is empty"
	}
	if n := utf8.RuneCountInString(req.Text); n > maxRunes {
		return http.StatusRequestEntityTooLarge, "text_too_long",
			fmt.Sprintf("text has %d runes, the limit is %d", n, maxRunes)
	}
	if req.ProfileID == "" {
		return http.StatusBadRequest, "profile_required", "profileId is required, pick or create a handwriting profile"
	}
	if req.Page.Width < 0 || req.Page.Width > maxPagePx || req.Page.Height < 0 || req.Page.Height > maxPagePx {
		return http.StatusBadRequest, "bad_page_size",
			fmt.Sprintf("page width and height must be between 1 and %d px", maxPagePx)
	}
	if req.Page.Columns < 0 || req.Page.Columns > 4 {
		return http.StatusBadRequest, "bad_columns", "columns must be between 1 and 4"
	}
	if req.Title != "" && utf8.RuneCountInString(req.Title) > 500 {
		return http.StatusRequestEntityTooLarge, "title_too_long", "title is too long"
	}
	return 0, "", ""
}

// ---------- async jobs ----------

func (s *Server) startWorkers(n int) {
	for i := 0; i < n; i++ {
		s.workerWG.Add(1)
		go func() {
			defer s.workerWG.Done()
			for q := range s.jobs {
				s.runJob(q.job)
			}
		}()
	}
}

func (s *Server) enqueue(req *model.GenerateRequest) (*model.Job, error) {
	job := &model.Job{
		ID:      render.NewID("j"),
		Status:  model.JobQueued,
		Created: time.Now(),
		Updated: time.Now(),
		Request: *req,
	}
	if err := s.Store.PutJob(job); err != nil {
		return nil, err
	}
	// The worker owns the queued job, so hand it a snapshot: the caller keeps
	// the original and reads its fields while the worker runs.
	snapshot := *job
	select {
	case s.jobs <- &queuedJob{job: &snapshot}:
		return job, nil
	default:
		job.Status = model.JobError
		job.Error = "queue is full, try again later"
		_ = s.Store.PutJob(job)
		return nil, fmt.Errorf("queue is full, try again later")
	}
}

func (s *Server) runJob(job *model.Job) {
	job.Status = model.JobRunning
	_ = s.Store.PutJob(job)

	start := time.Now()
	res, err := s.renderSync(&job.Request)
	if err != nil {
		job.Status = model.JobError
		job.Error = err.Error()
		job.Millis = time.Since(start).Milliseconds()
		_ = s.Store.PutJob(job)
		return
	}
	job.Status = model.JobDone
	job.Pages = res.Names
	job.Width = res.Width
	job.Height = res.Height
	job.Millis = time.Since(start).Milliseconds()
	if err := s.Store.PutJob(job); err != nil {
		return
	}
	_ = s.writeResultRef(job.ID, res.ID)
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.Store.GetJob(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "job_not_found", "no job with id "+id)
		return
	}
	view := jobView{Job: *job, Pages: []string{}, PageFiles: job.Pages}
	if job.Status == model.JobDone {
		if resultID, ok := s.readResultRef(id); ok {
			view.Pages = pageURLs(resultID, job.Pages)
		}
	}
	writeJSON(w, http.StatusOK, view)
}

// Result directories are named by the render engine, so the job -> result link
// is kept in a sidecar file to survive restarts.
func (s *Server) resultRefPath(jobID string) string {
	return filepath.Join(s.Store.Root, "jobs", jobID+".result")
}

func (s *Server) writeResultRef(jobID, resultID string) error {
	return os.WriteFile(s.resultRefPath(jobID), []byte(resultID), 0o644)
}

func (s *Server) readResultRef(jobID string) (string, bool) {
	b, err := os.ReadFile(s.resultRefPath(jobID))
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(string(b))
	if id == "" {
		return "", false
	}
	return id, true
}

func pageURLs(resultID string, names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, "/api/v1/results/"+resultID+"/"+n)
	}
	return out
}

// handleResultFile serves rendered pages and caches nothing itself: the files
// are immutable once written.
func (s *Server) handleResultFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("file")
	if !validID(id) || !validPageFile(name) {
		writeErr(w, http.StatusNotFound, "not_found", "unknown result file")
		return
	}
	p := s.Store.ResultPath(id, name)
	f, err := os.Open(p)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "result not found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func validID(id string) bool {
	if id == "" || len(id) > 40 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func validPageFile(name string) bool {
	if !strings.HasPrefix(name, "page-") || !strings.HasSuffix(name, ".png") {
		return false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(name, "page-"), ".png")
	if mid == "" {
		return false
	}
	for _, c := range mid {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
