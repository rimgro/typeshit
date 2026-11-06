// Package assets embeds bundled fonts used to synthesise fallback profiles.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed fonts/*.ttf
var fonts embed.FS

// Fonts lists the bundled font file names and their display metadata.
type Font struct {
	ID      string
	Name    string
	File    string
	Slant   float64 // degrees
	Cursive bool
}

// FontList is the catalogue of bundled fonts.
var FontList = []Font{
	{ID: "caveat", Name: "Caveat (быстрый)", File: "fonts/caveat.ttf", Slant: 8, Cursive: false},
	{ID: "badscript", Name: "Bad Script (пропись)", File: "fonts/badscript.ttf", Slant: 6, Cursive: true},
	{ID: "marckscript", Name: "Marck Script (каллиграфия)", File: "fonts/marckscript.ttf", Slant: 12, Cursive: true},
	{ID: "neucha", Name: "Neucha (печатный)", File: "fonts/neucha.ttf", Slant: 4, Cursive: false},
	{ID: "pangolin", Name: "Pangolin (аккуратный)", File: "fonts/pangolin.ttf", Slant: 3, Cursive: false},
	{ID: "amaticsc", Name: "Amatic SC (узкий)", File: "fonts/amaticsc.ttf", Slant: 2, Cursive: false},
	{ID: "caveatbrush", Name: "Caveat Brush (маркер)", File: "fonts/caveatbrush.ttf", Slant: 7, Cursive: false},
	{ID: "lobster", Name: "Lobster (росчерк)", File: "fonts/lobster.ttf", Slant: 10, Cursive: true},
	{ID: "gloria", Name: "Gloria (печатный, крупный)", File: "fonts/gloria.ttf", Slant: 1, Cursive: false},
}

// FontData returns the TTF bytes for a bundled font id.
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
	return nil, false
}

// FontFS exposes the raw embedded font filesystem.
func FontFS() fs.FS { return fonts }
