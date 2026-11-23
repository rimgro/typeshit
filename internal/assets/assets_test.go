package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirEmpty(t *testing.T) {
	n, err := LoadDir(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("missing dir: %v", err)
	}
	if n != 0 || len(External()) != 0 {
		t.Fatalf("expected empty catalogue, got %d", n)
	}
	if len(AllFonts()) != len(FontList) {
		t.Fatalf("AllFonts = %d, want %d", len(AllFonts()), len(FontList))
	}
}

func TestLoadDirCatalog(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha.ttf", "beta.otf", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	catalog := `{"fonts":[
		{"id":"alpha","name":"Alpha Script","category":"handwriting","scripts":["Cyrillic"],"languages":["ru"]},
		{"id":"beta","name":"Beta","category":"cursive","slant":7,"cursive":true}
	]}`
	if err := os.WriteFile(filepath.Join(dir, CatalogFile), []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if n != 2 {
		t.Fatalf("loaded %d fonts, want 2 (txt must be skipped)", n)
	}

	list := External()
	if len(list) != 2 || list[0].ID != "alpha" || list[1].ID != "beta" {
		t.Fatalf("catalogue = %+v", list)
	}
	alpha := list[0]
	if alpha.Name != "Alpha Script" || alpha.Category != "handwriting" || alpha.Source != "dir:"+filepath.Base(dir) {
		t.Fatalf("alpha = %+v", alpha)
	}
	if !list[1].Cursive || list[1].Slant != 7 {
		t.Fatalf("beta metadata = %+v", list[1])
	}

	if _, ok := Lookup("beta"); !ok {
		t.Fatal("Lookup missed external font")
	}
	if _, ok := Lookup("caveat"); !ok {
		t.Fatal("Lookup missed builtin font")
	}
	if data, ok := FontData("alpha"); !ok || len(data) == 0 {
		t.Fatal("FontData did not read the external file")
	}
	if len(AllFonts()) != len(FontList)+2 {
		t.Fatalf("AllFonts = %d", len(AllFonts()))
	}

	// Builtin ids win over external ones with the same id.
	if err := os.WriteFile(filepath.Join(dir, "caveat.ttf"), []byte("shadow"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	data, ok := FontData("caveat")
	if !ok || string(data) == "shadow" {
		t.Fatal("external font shadowed the builtin one")
	}

	t.Cleanup(func() { LoadDir("") })
}

func TestSanitizeID(t *testing.T) {
	cases := map[string]string{
		"Abram Font4You": "abram-font4you",
		"viktoria-s":     "viktoria-s",
		"--evil/../id--": "evil-id",
	}
	for in, want := range cases {
		if got := sanitizeID(in); got != want {
			t.Fatalf("sanitizeID(%q) = %q, want %q", in, got, want)
		}
	}
}
