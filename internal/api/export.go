package api

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// resultPageFiles lists the rendered PNG files of a result, sorted.
func (s *Server) resultPageFiles(id string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(s.Store.ResultPath(id, ""), "page-*.png"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, os.ErrNotExist
	}
	return matches, nil
}

func (s *Server) handleExportPDF(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeErr(w, http.StatusNotFound, "not_found", "unknown result")
		return
	}
	target := s.Store.ResultPath(id, "export.pdf")
	s.exportMu.Lock()
	if _, err := os.Stat(target); err != nil {
		if err := s.buildPDF(id, target); err != nil {
			s.exportMu.Unlock()
			writeErr(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
	}
	s.exportMu.Unlock()
	s.serveFile(w, r, target, "application/pdf", "handwriting.pdf")
}

func (s *Server) handleExportZIP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeErr(w, http.StatusNotFound, "not_found", "unknown result")
		return
	}
	target := s.Store.ResultPath(id, "export.zip")
	s.exportMu.Lock()
	if _, err := os.Stat(target); err != nil {
		if err := s.buildZIP(id, target); err != nil {
			s.exportMu.Unlock()
			writeErr(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
	}
	s.exportMu.Unlock()
	s.serveFile(w, r, target, "application/zip", "handwriting.zip")
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, path, contentType, download string) {
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "file not found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "io", err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+download+`"`)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, download, st.ModTime(), f)
}

func (s *Server) buildZIP(id, target string) error {
	files, err := s.resultPageFiles(id)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.Base(p))
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	readme, err := zw.Create("README.txt")
	if err != nil {
		return err
	}
	if _, err := readme.Write([]byte(zipReadme)); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return writeFileAtomic(target, buf.Bytes())
}

const zipReadme = "Рукописные страницы, сгенерированные typeshit.\n" +
	"Каждый файл page-NNN.png — одна страница документа.\n"

func (s *Server) buildPDF(id, target string) error {
	files, err := s.resultPageFiles(id)
	if err != nil {
		return err
	}
	jpegs := make([][]byte, 0, len(files))
	sizes := make([][2]int, 0, len(files))
	for _, p := range files {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return err
		}
		b := img.Bounds()
		jpegs = append(jpegs, buf.Bytes())
		sizes = append(sizes, [2]int{b.Dx(), b.Dy()})
	}
	return writeFileAtomic(target, buildPDF(jpegs, sizes))
}

// buildPDF writes a minimal PDF 1.4 document with one page per JPEG image
// (DCTDecode keeps the file small: a 40 page scan stays a few megabytes).
func buildPDF(jpegs [][]byte, sizes [][2]int) []byte {
	n := len(jpegs)
	objs := make([][]byte, 0, 2+3*n)

	var kids strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", 3+i*3)
	}
	objs = append(objs, []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	objs = append(objs, []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), n)))

	for i := 0; i < n; i++ {
		pageID := 3 + i*3
		contentID := pageID + 1
		imageID := pageID + 2
		w, h := pageSize(sizes[i])

		objs = append(objs, []byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] "+
				"/Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
			w, h, imageID, contentID)))

		content := fmt.Sprintf("q\n%.2f 0 0 %.2f 0 0 cm\n/Im0 Do\nQ\n", w, h)
		objs = append(objs, []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)))

		header := fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d "+
			"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n",
			sizes[i][0], sizes[i][1], len(jpegs[i]))
		imgObj := make([]byte, 0, len(header)+len(jpegs[i])+12)
		imgObj = append(imgObj, header...)
		imgObj = append(imgObj, jpegs[i]...)
		imgObj = append(imgObj, "\nendstream"...)
		objs = append(objs, imgObj)
	}

	var buf bytes.Buffer
	buf.Grow(1 << 20)
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objs)+1)
	for i, o := range objs {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", i+1)
		buf.Write(o)
		buf.WriteString("\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objs)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return buf.Bytes()
}

// pageSize returns the page box in PostScript points, keeping A4 proportions.
func pageSize(px [2]int) (float64, float64) {
	const a4w, a4h = 595.28, 841.89
	if px[0] > px[1] {
		return a4h, a4w
	}
	return a4w, a4h
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
