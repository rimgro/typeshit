package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/font/sfnt"

	"typeshit/internal/assets"
	"typeshit/internal/model"
	"typeshit/internal/profile"
	"typeshit/internal/render"
)

const (
	previewCacheMax = 128
	previewW        = 1200
	previewH        = 210
)

type fontInfo struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Slant    float64 `json:"slant"`
	Cursive  bool    `json:"cursive"`
	Cyrillic bool    `json:"cyrillic"`
	Builtin  bool    `json:"builtin"`
}

type profileSummary struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Created  time.Time         `json:"created"`
	Source   string            `json:"source"`
	Lang     []string          `json:"lang"`
	Chars    int               `json:"chars"`
	Variants int               `json:"variants"`
	Preview  string            `json:"preview"`
	Style    model.StyleModel  `json:"style"`
	Meta     map[string]string `json:"meta,omitempty"`
}

type createProfileBody struct {
	Name     string `json:"name"`
	FontID   string `json:"fontId"`
	Variants int    `json:"variants"`
	Lang     string `json:"lang"`
}

// ---------- fonts ----------

func (s *Server) handleFonts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.fontList())
}

func (s *Server) fontList() []fontInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fontsOK {
		return s.fonts
	}
	out := make([]fontInfo, 0, len(assets.FontList))
	for _, f := range assets.FontList {
		info := fontInfo{ID: f.ID, Name: f.Name, Slant: f.Slant, Cursive: f.Cursive, Builtin: true}
		if data, ok := assets.FontData(f.ID); ok {
			info.Cyrillic = fontHasCyrillic(data)
		}
		out = append(out, info)
	}
	s.fonts = out
	s.fontsOK = true
	return out
}

// fontHasCyrillic reports whether the font can draw the Russian alphabet.
func fontHasCyrillic(data []byte) bool {
	f, err := sfnt.Parse(data)
	if err != nil {
		return false
	}
	var buf sfnt.Buffer
	for _, r := range []rune{'а', 'я', 'ё', 'Ж'} {
		idx, err := f.GlyphIndex(&buf, r)
		if err != nil || idx == 0 {
			return false
		}
	}
	return true
}

// ---------- profiles ----------

func (s *Server) profileDir(id string) string {
	return filepath.Join(s.Store.Root, "profiles", id)
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.Store.ListProfiles()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	out := make([]profileSummary, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, summarize(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func summarize(p *model.Profile) profileSummary {
	variants := 0
	for _, sprites := range p.Glyphs {
		variants += len(sprites)
	}
	return profileSummary{
		ID:       p.ID,
		Name:     p.Name,
		Created:  p.Created,
		Source:   p.Source,
		Lang:     p.Lang,
		Chars:    len(p.Glyphs),
		Variants: variants,
		Preview:  "/api/v1/profiles/" + p.ID + "/preview",
		Style:    p.Style,
		Meta:     p.Meta,
	}
}

// loadProfileMeta reads only profile.json (atlases stay on disk).
func (s *Server) loadProfileMeta(id string) (*model.Profile, error) {
	b, err := os.ReadFile(filepath.Join(s.profileDir(id), "profile.json"))
	if err != nil {
		return nil, err
	}
	var p model.Profile
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeErr(w, http.StatusNotFound, "not_found", "unknown profile")
		return
	}
	p, _, err := s.Store.LoadProfile(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no profile with id "+id)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeErr(w, http.StatusNotFound, "not_found", "unknown profile")
		return
	}
	if _, err := s.loadProfileMeta(id); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no profile with id "+id)
		return
	}
	if err := s.Store.DeleteProfile(id); err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAtlas(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")
	if !validID(id) || name == "" || strings.ContainsAny(name, "/\\") {
		writeErr(w, http.StatusNotFound, "not_found", "unknown atlas")
		return
	}
	p, err := s.loadProfileMeta(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no profile with id "+id)
		return
	}
	allowed := false
	for _, a := range p.Atlases {
		if a == name {
			allowed = true
			break
		}
	}
	if !allowed {
		writeErr(w, http.StatusNotFound, "not_found", "atlas page not found")
		return
	}
	pth := filepath.Join(s.profileDir(id), name)
	f, err := os.Open(pth)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "atlas page not found")
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

func (s *Server) handleProfilePreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeErr(w, http.StatusNotFound, "not_found", "unknown profile")
		return
	}
	text := strings.TrimSpace(r.URL.Query().Get("text"))
	if len([]rune(text)) > 600 {
		text = string([]rune(text)[:600])
	}
	seed := int64(1)
	if text == "" {
		seed = 7
	}
	cacheKey := id + "|" + text + "|" + itoa(int(seed))

	s.mu.Lock()
	if data, ok := s.previews[cacheKey]; ok {
		s.mu.Unlock()
		servePNG(w, data)
		return
	}
	s.mu.Unlock()

	prof, atlases, err := s.Store.LoadProfile(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no profile with id "+id)
		return
	}
	img := render.PreviewLine(prof, atlases, text, previewW, previewH, seed)
	if img == nil {
		writeErr(w, http.StatusInternalServerError, "preview_failed", "preview renderer returned no image")
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		writeErr(w, http.StatusInternalServerError, "encode", err.Error())
		return
	}
	data := buf.Bytes()
	s.cachePreview(cacheKey, data)

	w.Header().Set("Cache-Control", "public, max-age=3600")
	servePNG(w, data)
}

func (s *Server) cachePreview(key string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.previews[key]; !ok {
		s.previewO = append(s.previewO, key)
	}
	s.previews[key] = data
	for len(s.previewO) > previewCacheMax {
		old := s.previewO[0]
		s.previewO = s.previewO[1:]
		delete(s.previews, old)
	}
}

// invalidatePreview drops cached previews of a profile after it changed.
func (s *Server) invalidatePreview(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.previews {
		if strings.HasPrefix(key, id+"|") {
			delete(s.previews, key)
		}
	}
}

// handleCreateProfile builds a profile from one of the bundled fonts.
func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var body createProfileBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.FontID == "" {
		body.FontID = assets.FontList[0].ID
	}
	data, ok := assets.FontData(body.FontID)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown_font", "unknown fontId "+body.FontID)
		return
	}
	if body.Variants <= 0 {
		body.Variants = 3
	}
	if body.Variants > 6 {
		body.Variants = 6
	}
	if body.Lang == "" {
		body.Lang = "ru"
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Быстрый почерк"
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}

	p, atlases, err := profile.BuildFromFont(render.NewID("p"), name, data, profile.FontOptions{
		Lang:     body.Lang,
		Variants: body.Variants,
		Seed:     time.Now().UnixNano(),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "profile_failed", err.Error())
		return
	}
	if p == nil {
		writeErr(w, http.StatusInternalServerError, "profile_failed", "profile builder returned nothing")
		return
	}
	p.ID = render.NewID("p")
	p.Name = name
	p.Created = time.Now()
	p.Lang = []string{body.Lang}
	if p.Source == "" {
		p.Source = "font"
	}
	if p.Meta == nil {
		p.Meta = map[string]string{}
	}
	p.Meta["fontId"] = body.FontID
	if err := s.persistProfile(p, atlases); err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, summarize(p))
}

// persistProfile renders the preview thumbnail, then writes the profile.
func (s *Server) persistProfile(p *model.Profile, atlases []*image.NRGBA) error {
	if p.Preview == "" {
		p.Preview = "preview.png"
	}
	dir := s.profileDir(p.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if len(atlases) > 0 {
		imgs := make([]image.Image, len(atlases))
		for i := range atlases {
			imgs[i] = atlases[i]
		}
		if prev := render.PreviewLine(p, imgs, "", previewW, previewH, 7); prev != nil {
			f, err := os.Create(filepath.Join(dir, p.Preview))
			if err == nil {
				_ = png.Encode(f, prev)
				_ = f.Close()
			}
		}
	}
	return s.Store.SaveProfile(p, atlases)
}
