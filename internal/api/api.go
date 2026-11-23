// Package api exposes the HTTP JSON API and serves the web UI.
package api

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"typeshit/internal/render"
	"typeshit/internal/store"
	"typeshit/internal/webui"
)

// Version is reported by /api/v1/health.
const Version = "0.1.0"

// Server wires the API handlers to the rendering engine.
type Server struct {
	Store  *store.Store
	Engine *render.Engine

	// sem bounds concurrent renders so a burst of requests cannot exhaust RAM.
	sem chan struct{}

	jobs      chan *queuedJob
	workerWG  sync.WaitGroup
	closeOnce sync.Once

	mu       sync.Mutex
	exportMu sync.Mutex
	previews map[string][]byte
	previewO []string
	fonts    []fontInfo
	fontsOK  bool
	tmpl     map[string][]templateInfo
}

// New builds a server and starts the asynchronous job workers.
func New(st *store.Store, eng *render.Engine) *Server {
	s := &Server{
		Store:    st,
		Engine:   eng,
		sem:      make(chan struct{}, max(1, runtime.NumCPU())),
		jobs:     make(chan *queuedJob, jobQueueSize),
		previews: map[string][]byte{},
		tmpl:     map[string][]templateInfo{},
	}
	s.startWorkers(jobWorkers)
	return s
}

// Handler returns the root mux: /api/v1/* JSON, everything else the web UI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/fonts", s.handleFonts)
	mux.HandleFunc("GET /api/v1/fonts/{id}/file", s.handleFontFile)
	mux.HandleFunc("POST /api/v1/import", s.handleImport)
	mux.HandleFunc("GET /api/v1/profiles", s.handleProfiles)
	mux.HandleFunc("POST /api/v1/profiles", s.handleCreateProfile)
	mux.HandleFunc("GET /api/v1/profiles/{id}", s.handleProfile)
	mux.HandleFunc("DELETE /api/v1/profiles/{id}", s.handleDeleteProfile)
	mux.HandleFunc("GET /api/v1/profiles/{id}/atlas/{name}", s.handleAtlas)
	mux.HandleFunc("GET /api/v1/profiles/{id}/preview", s.handleProfilePreview)

	mux.HandleFunc("GET /api/v1/template", s.handleTemplates)
	mux.HandleFunc("GET /api/v1/template/{lang}/{reps}/{page}", s.handleTemplatePage)

	mux.HandleFunc("POST /api/v1/capture", s.handleCapture)
	mux.HandleFunc("GET /api/v1/capture/{id}", s.handleCaptureDoc)
	mux.HandleFunc("GET /api/v1/capture/{id}/image", s.handleCaptureImage)
	mux.HandleFunc("POST /api/v1/capture/{id}/cells", s.handleCaptureCells)
	mux.HandleFunc("POST /api/v1/capture/{id}/commit", s.handleCaptureCommit)

	mux.HandleFunc("POST /api/v1/generate", s.handleGenerate)
	mux.HandleFunc("GET /api/v1/jobs/{id}", s.handleJob)
	mux.HandleFunc("GET /api/v1/results/{id}/{file}", s.handleResultFile)
	mux.HandleFunc("GET /api/v1/results/{id}/export.pdf", s.handleExportPDF)
	mux.HandleFunc("GET /api/v1/results/{id}/export.zip", s.handleExportZIP)

	mux.HandleFunc("GET /api/v1/openapi.json", s.handleOpenAPI)
	mux.HandleFunc("/api/v1", handleAPINotFound)
	mux.HandleFunc("/api/v1/", handleAPINotFound)

	mux.Handle("/", s.staticHandler())
	return recoverMW(logMW(mux))
}

// Close stops the job workers. Pending queued jobs are dropped.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.jobs)
		s.workerWG.Wait()
	})
}

func (s *Server) acquire() { s.sem <- struct{}{} }
func (s *Server) release() { <-s.sem }

// ---------- helpers ----------

const maxBodyBytes = 32 << 20

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeErr(w http.ResponseWriter, code int, slug, msg string) {
	writeJSON(w, code, apiError{Error: msg, Code: slug})
}

func handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, "not_found", "unknown endpoint "+r.URL.Path)
}

// decodeJSON reads a JSON body with a hard size limit.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		msg := "invalid JSON body: " + err.Error()
		if _, ok := err.(*http.MaxBytesError); ok {
			writeErr(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			return false
		}
		writeErr(w, http.StatusBadRequest, "bad_json", msg)
		return false
	}
	return true
}

func servePNG(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ---------- middleware ----------

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func logMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		log.Printf("%s %s %d %dB %s", r.Method, r.URL.Path, sw.status, sw.bytes, time.Since(start).Round(time.Millisecond))
	})
}

func recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				if strings.HasPrefix(r.URL.Path, "/api/") {
					writeErr(w, http.StatusInternalServerError, "internal", "internal server error")
					return
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---------- static web UI ----------

func (s *Server) staticHandler() http.Handler {
	fsys := webui.FS()
	fileServer := http.FileServer(http.FS(fsys))
	index, indexErr := fs.ReadFile(fsys, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if st, err := fs.Stat(fsys, name); err == nil && !st.IsDir() {
			if strings.HasSuffix(name, ".html") {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: unknown paths render the shell so client routes work.
		if indexErr != nil {
			http.Error(w, "web UI is not built", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(index)
	})
}
