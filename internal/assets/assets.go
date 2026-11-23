// Package assets embeds bundled fonts used to synthesise fallback profiles and
// can additionally expose an external font catalogue loaded from disk.
package assets

import (
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

//go:embed fonts/*.ttf
var fonts embed.FS

// Font lists the bundled font file names and their display metadata.
type Font struct {
	ID      string
	Name    string
	File    string
	Slant   float64 // degrees
	Cursive bool
	// Category is a free-form tag shown in the UI ("handwriting", "cursive", ...).
	Category string
	// Source names the catalogue the font came from: "builtin" or a dir label.
	Source string
}

// FontList is the catalogue of bundled fonts.
var FontList = []Font{
	{ID: "caveat", Name: "Caveat (быстрый)", File: "fonts/caveat.ttf", Slant: 8, Cursive: false, Category: "handwriting", Source: "builtin"},
	{ID: "badscript", Name: "Bad Script (пропись)", File: "fonts/badscript.ttf", Slant: 6, Cursive: true, Category: "cursive", Source: "builtin"},
	{ID: "marckscript", Name: "Marck Script (каллиграфия)", File: "fonts/marckscript.ttf", Slant: 12, Cursive: true, Category: "cursive", Source: "builtin"},
	{ID: "neucha", Name: "Neucha (печатный)", File: "fonts/neucha.ttf", Slant: 4, Cursive: false, Category: "handwriting", Source: "builtin"},
	{ID: "pangolin", Name: "Pangolin (аккуратный)", File: "fonts/pangolin.ttf", Slant: 3, Cursive: false, Category: "handwriting", Source: "builtin"},
	{ID: "amaticsc", Name: "Amatic SC (узкий)", File: "fonts/amaticsc.ttf", Slant: 2, Cursive: false, Category: "handwriting", Source: "builtin"},
	{ID: "caveatbrush", Name: "Caveat Brush (маркер)", File: "fonts/caveatbrush.ttf", Slant: 7, Cursive: false, Category: "handwriting", Source: "builtin"},
	{ID: "lobster", Name: "Lobster (росчерк)", File: "fonts/lobster.ttf", Slant: 10, Cursive: true, Category: "cursive", Source: "builtin"},
	{ID: "gloria", Name: "Gloria (печатный, крупный)", File: "fonts/gloria.ttf", Slant: 1, Cursive: false, Category: "handwriting", Source: "builtin"},
}

// catalogEntry is one record of an external catalog.json (optional).
type catalogEntry struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Source   string   `json:"source"`
	Slant    float64  `json:"slant"`
	Cursive  bool     `json:"cursive"`
	Scripts  []string `json:"scripts"`
}

// external is the catalogue loaded from a directory on disk (debug/parity fonts).
type external struct {
	mu    sync.RWMutex
	list  []Font
	paths map[string]string // id -> absolute ttf/otf path
}

var ext = &external{paths: map[string]string{}}

// CatalogFile is the optional metadata file looked up inside a font directory.
const CatalogFile = "catalog.json"

// LoadDir scans dir for *.ttf/*.otf and replaces the external catalogue.
// A missing directory is not an error: the catalogue just becomes empty.
// Metadata is taken from an optional catalog.json in the same directory:
//
//	{"fonts":[{"id":"alexander","name":"Alexander","category":"handwriting"}]}
//
// Files without a catalogue record keep their file name as the display name.
func LoadDir(dir string) (int, error) {
	if dir == "" {
		reset()
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			reset()
			return 0, nil
		}
		return 0, err
	}

	meta := readCatalog(filepath.Join(dir, CatalogFile))

	list := make([]Font, 0, len(entries))
	paths := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch strings.ToLower(filepath.Ext(name)) {
		case ".ttf", ".otf":
		default:
			continue
		}
		id := strings.TrimSuffix(name, filepath.Ext(name))
		f := Font{
			ID:       sanitizeID(id),
			Name:     strings.TrimSpace(meta[id].Name),
			File:     name,
			Slant:    meta[id].Slant,
			Cursive:  meta[id].Cursive,
			Category: strings.TrimSpace(meta[id].Category),
			Source:   strings.TrimSpace(meta[id].Source),
		}
		if f.Name == "" {
			f.Name = id
		}
		if f.Category == "" {
			f.Category = "external"
		}
		if f.Source == "" {
			f.Source = "dir:" + filepath.Base(dir)
		}
		if f.ID == "" {
			continue
		}
		list = append(list, f)
		paths[f.ID] = filepath.Join(dir, name)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	ext.mu.Lock()
	ext.list, ext.paths = list, paths
	ext.mu.Unlock()
	return len(list), nil
}

func reset() {
	ext.mu.Lock()
	ext.list, ext.paths = nil, map[string]string{}
	ext.mu.Unlock()
}

func readCatalog(path string) map[string]catalogEntry {
	out := map[string]catalogEntry{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var doc struct {
		Fonts []struct {
			catalogEntry
			ID string `json:"id"`
		} `json:"fonts"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return out
	}
	for _, f := range doc.Fonts {
		if f.ID == "" {
			continue
		}
		out[f.ID] = f.catalogEntry
	}
	return out
}

// sanitizeID keeps ids URL-safe: they end up in /api/v1/profiles bodies.
func sanitizeID(id string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
			dash = false
		case r == '-' || r == ' ' || r == '.':
			if !dash {
				b.WriteByte('-')
				dash = true
			}
		default:
			if !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// External returns the fonts discovered by the last LoadDir call.
func External() []Font {
	ext.mu.RLock()
	defer ext.mu.RUnlock()
	return append([]Font(nil), ext.list...)
}

// AllFonts returns the builtin catalogue followed by the external one.
func AllFonts() []Font {
	out := make([]Font, 0, len(FontList)+len(External()))
	out = append(out, FontList...)
	out = append(out, External()...)
	return out
}

// Lookup returns font metadata by id from either catalogue.
func Lookup(id string) (Font, bool) {
	for _, f := range FontList {
		if f.ID == id {
			return f, true
		}
	}
	ext.mu.RLock()
	defer ext.mu.RUnlock()
	for _, f := range ext.list {
		if f.ID == id {
			return f, true
		}
	}
	return Font{}, false
}

// FontData returns the TTF bytes for a bundled or externally loaded font id.
func FontData(id string) ([]byte, bool) {
	for _, f := range FontList {
		if f.ID == id {
			b, err := fonts.ReadFile(f.File)
			if err != nil {
				return nil, false
			}
			return b, true
		}
	}
	ext.mu.RLock()
	path, ok := ext.paths[id]
	ext.mu.RUnlock()
	if !ok {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return b, true
}

// FontFS exposes the raw embedded font filesystem.
func FontFS() fs.FS { return fonts }
