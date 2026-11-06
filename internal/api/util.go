package api

import (
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

// errNoTemplates is returned when the sheet renderer produces nothing.
var errNoTemplates = errors.New("no template pages were rendered")

func writePNGFile(path string, img image.Image) error {
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

func writePNGTo(w io.Writer, img image.Image) error {
	return png.Encode(w, img)
}
