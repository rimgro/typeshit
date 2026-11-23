package api

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// maxImportBytes caps the size of an imported document.
const maxImportBytes = 24 << 20

// importResult is the payload returned by /api/v1/import.
type importResult struct {
	Text  string `json:"text"`
	Chars int    `json:"chars"`
	Name  string `json:"name"`
}

// handleImport turns an uploaded .docx / .txt / .md into plain text so agents and the
// browser can feed documents straight into the generator. Markdown is reduced to the
// prose a human would copy out by hand.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_body", "ожидался multipart с полем file")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no_file", "поле file обязательно")
		return
	}
	defer file.Close()
	if hdr.Size > maxImportBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "документ больше 24 МБ")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read_failed", err.Error())
		return
	}

	name := filepath.Base(hdr.Filename)
	ext := strings.ToLower(filepath.Ext(name))
	var text string
	switch ext {
	case ".docx":
		text, err = docxText(data)
	case ".txt", "", ".md":
		if !utf8.Valid(data) {
			err = errors.New("файл не в UTF-8")
		} else {
			text = string(data)
			if ext == ".md" {
				text = markdownText(text)
			}
		}
	default:
		writeErr(w, http.StatusBadRequest, "unsupported", "поддерживаются .docx, .txt и .md")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "parse_failed", err.Error())
		return
	}
	text = normalizeImportedText(text)
	if strings.TrimSpace(text) == "" {
		writeErr(w, http.StatusBadRequest, "empty", "в документе не нашлось текста")
		return
	}
	writeJSON(w, http.StatusOK, importResult{Text: text, Chars: len([]rune(text)), Name: name})
}

// docxText extracts the visible text of a .docx package. A .docx is a zip with
// word/document.xml; paragraphs are <w:p>, runs are <w:t>. No fonts, tables
// layout or images are imported, only the words in reading order.
func docxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("это не похоже на .docx")
	}
	var doc *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			doc = f
			break
		}
	}
	if doc == nil {
		return "", errors.New("в .docx нет word/document.xml")
	}
	rc, err := doc.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var out strings.Builder
	dec := xml.NewDecoder(io.LimitReader(rc, maxImportBytes))
	inText := false
	// Tables repeat cells; a tab keeps cells apart so numbering stays readable.
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.New("не удалось разобрать document.xml")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "br", "cr":
				out.WriteString("\n")
			case "tab":
				out.WriteString("\t")
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				out.WriteString("\n\n")
			case "tc":
				out.WriteString("\t")
			}
		case xml.CharData:
			if inText {
				out.Write(t)
			}
		}
	}
	return out.String(), nil
}

// normalizeImportedText cleans the extracted text: CRLF, stray tabs, runs of
// blank lines and non-breaking spaces.
func normalizeImportedText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\u200b", "")
	s = strings.ReplaceAll(s, "\t\n", "\n")
	s = strings.ReplaceAll(s, "\n\t", "\n")
	lines := strings.Split(s, "\n")
	var clean []string
	blank := 0
	for _, ln := range lines {
		ln = strings.TrimRight(ln, " \t")
		if strings.TrimSpace(ln) == "" {
			blank++
			if blank > 1 {
				continue
			}
			clean = append(clean, "")
			continue
		}
		blank = 0
		clean = append(clean, strings.TrimLeft(ln, " "))
	}
	return strings.TrimSpace(strings.Join(clean, "\n"))
}
