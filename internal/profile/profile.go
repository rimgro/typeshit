// Package profile builds handwriting profiles from reference sheets or fonts.
//
// A profile is a set of glyph sprites plus a statistical style model. The
// interesting path is CommitScan: the user writes on a printed template, the
// server cuts every letter out of the photo and keeps each repetition as a
// separate variant, so generated pages inherit the real hand's variability
// instead of a font's single shape.
package profile

import "typeshit/internal/model"

// FontOptions tunes glyph synthesis from a font.
type FontOptions struct {
	Lang     string
	Variants int   // geometric variants per character
	Seed     int64 // deterministic seed
	Style    model.StyleModel
}

// SheetSpec describes one printable sample-sheet page.
type SheetSpec struct {
	Lang  string
	Reps  int // repetitions per character (variants per glyph)
	Page  int // 0-based page index
	Pages int // total pages
	DPI   int
	// Chars are the characters on this page in row-major order (left to right,
	// top to bottom) so the analyser can assign labels automatically.
	Chars []string
}
