// Package store persists profiles, capture sessions, jobs and results on disk.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"typeshit/internal/model"
)

// ErrNotFound is returned when an id does not exist on disk.
var ErrNotFound = errors.New("not found")

// Store is a directory backed data store.
type Store struct {
	Root string
}

// Open creates the directory layout if needed.
func Open(root string) (*Store, error) {
	s := &Store{Root: root}
	for _, d := range []string{"profiles", "captures", "results", "jobs", "paper"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) profileDir(id string) string { return filepath.Join(s.Root, "profiles", id) }

// SaveProfile writes profile.json and the atlas pages.
func (s *Store) SaveProfile(p *model.Profile, atlases []*image.NRGBA) error {
	dir := s.profileDir(p.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p.Atlases = p.Atlases[:0]
	for i, a := range atlases {
		name := fmt.Sprintf("atlas-%02d.png", i)
		if err := writePNG(filepath.Join(dir, name), a); err != nil {
			return err
		}
		p.Atlases = append(p.Atlases, name)
	}
	return s.writeJSON(filepath.Join(dir, "profile.json"), p)
}

// LoadProfile reads the profile plus its decoded atlas pages.
func (s *Store) LoadProfile(id string) (*model.Profile, []image.Image, error) {
	dir := s.profileDir(id)
	var p model.Profile
	if err := readJSON(filepath.Join(dir, "profile.json"), &p); err != nil {
		return nil, nil, err
	}
	atlas := make([]image.Image, 0, len(p.Atlases))
	for _, name := range p.Atlases {
		img, err := readPNG(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, err
		}
		atlas = append(atlas, img)
	}
	return &p, atlas, nil
}

// ListProfiles returns profiles sorted newest first.
func (s *Store) ListProfiles() ([]*model.Profile, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "profiles"))
	if err != nil {
		return nil, err
	}
	out := []*model.Profile{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var p model.Profile
		if err := readJSON(filepath.Join(s.Root, "profiles", e.Name(), "profile.json"), &p); err != nil {
			continue
		}
		out = append(out, &p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// DeleteProfile removes a profile directory.
func (s *Store) DeleteProfile(id string) error {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return ErrNotFound
	}
	return os.RemoveAll(s.profileDir(id))
}

// WriteCapture stores a normalised capture scan.
func (s *Store) WriteCapture(id string, img *image.NRGBA) error {
	return writePNG(filepath.Join(s.Root, "captures", id+".png"), img)
}

// ReadCapture loads a normalised capture scan.
func (s *Store) ReadCapture(id string) (*image.NRGBA, error) {
	return readPNG(filepath.Join(s.Root, "captures", id+".png"))
}

// PutDoc stores capture markup state.
func (s *Store) PutDoc(doc *model.CaptureDoc) error {
	return s.writeJSON(filepath.Join(s.Root, "captures", doc.ID+".json"), doc)
}

// GetDoc loads capture markup state.
func (s *Store) GetDoc(id string) (*model.CaptureDoc, error) {
	var doc model.CaptureDoc
	if err := readJSON(filepath.Join(s.Root, "captures", id+".json"), &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// NewResultDir allocates a directory for a render result.
func (s *Store) NewResultDir(id string) (string, error) {
	dir := filepath.Join(s.Root, "results", id)
	return dir, os.MkdirAll(dir, 0o755)
}

// ResultPath maps a file name inside a result.
func (s *Store) ResultPath(id, name string) string {
	return filepath.Join(s.Root, "results", id, name)
}

// SavePage stores one rendered page.
func (s *Store) SavePage(resultID string, index int, img *image.NRGBA) (string, error) {
	name := fmt.Sprintf("page-%03d.png", index+1)
	return name, writePNG(s.ResultPath(resultID, name), img)
}

// PutJob stores mutable job state.
func (s *Store) PutJob(j *model.Job) error {
	j.Updated = time.Now()
	return s.writeJSON(filepath.Join(s.Root, "jobs", j.ID+".json"), j)
}

// GetJob loads job state.
func (s *Store) GetJob(id string) (*model.Job, error) {
	var j model.Job
	if err := readJSON(filepath.Join(s.Root, "jobs", id+".json"), &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return json.Unmarshal(b, v)
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readPNG(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	if n, ok := img.(*image.NRGBA); ok {
		return n, nil
	}
	b := img.Bounds()
	n := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			n.Set(x, y, img.At(x, y))
		}
	}
	return n, nil
}
