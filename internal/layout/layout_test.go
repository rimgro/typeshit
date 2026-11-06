package layout

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"
	"unicode"

	"typeshit/internal/model"
)

// testProfile builds a synthetic profile: it does not depend on the capture
// package, so layout can be tested in isolation.
func testProfile(variants int, cursive bool) *model.Profile {
	p := &model.Profile{
		ID:   "test",
		Name: "test",
		Em:   100,
		Style: model.StyleModel{
			Slant:        7,
			SlantSigma:   1.1,
			ScaleSigma:   0.03,
			RotSigma:     0.8,
			DySigma:      0.015,
			DxSigma:      0.03,
			AdvanceScale: 1,
			WordGap:      0.56,
			LineGap:      1.45,
			InkMean:      0.94,
			InkSigma:     0.05,
			WeightMean:   1,
			WeightSigma:  0.08,
			JoinProb:     0.9,
			LiftProb:     0.03,
		},
		Glyphs: map[string][]model.Sprite{},
	}
	if variants < 1 {
		variants = 1
	}
	add := func(ch string, box model.Rect, adv float64, join model.Join) {
		list := make([]model.Sprite, variants)
		for v := 0; v < variants; v++ {
			b := box
			a := adv
			if v > 0 {
				b.W *= 1 + 0.03*float64(v-1)
				a *= 1 + 0.02*float64(v-1)
			}
			list[v] = model.Sprite{
				Char: ch, Variant: v, Atlas: 0,
				Pix: [4]int{0, 0, int(b.W * 100), int(b.H * 100)},
				Box: b, Advance: a, Join: join,
				Ink: 0.92, Weight: 0.08, Confidence: 0.9,
			}
		}
		p.Glyphs[ch] = list
	}
	none := model.Join{Kind: "none"}
	low := func(r rune) model.Join {
		if !cursive {
			return none
		}
		return model.Join{Entry: model.Point{X: 0.02, Y: 0.34}, Exit: model.Point{X: 0.5, Y: 0.34}, Kind: "high"}
	}
	lowDesc := func(r rune) model.Join {
		if !cursive {
			return none
		}
		return model.Join{Entry: model.Point{X: 0.02, Y: 0.34}, Exit: model.Point{X: 0.48, Y: 0.34}, Kind: "up"}
	}

	for _, r := range "абвгдежзийклмнопрстуфхцчшщъыьэюя" {
		switch r {
		case 'у', 'р', 'д', 'ф', 'ц', 'щ':
			add(string(r), model.Rect{X: 0.02, Y: -0.22, W: 0.52, H: 0.72}, 0.58, lowDesc(r))
		case 'б', 'в':
			add(string(r), model.Rect{X: 0.02, Y: 0, W: 0.52, H: 0.86}, 0.58, lowDesc(r))
		default:
			add(string(r), model.Rect{X: 0.02, Y: 0, W: 0.5, H: 0.52}, 0.56, low(r))
		}
	}
	add("ё", model.Rect{X: 0.02, Y: 0, W: 0.5, H: 0.56}, 0.56, low('ё'))
	for _, r := range "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ" {
		add(string(r), model.Rect{X: 0.02, Y: 0, W: 0.62, H: 0.74}, 0.68, none)
	}
	for r := '0'; r <= '9'; r++ {
		add(string(r), model.Rect{X: 0.03, Y: 0, W: 0.44, H: 0.7}, 0.5, none)
	}
	for _, r := range ".,;:!?-—()«»\"'/%*+=" {
		add(string(r), model.Rect{X: 0.02, Y: 0, W: 0.2, H: 0.2}, 0.3, none)
	}
	for _, r := range "abcdefghijklmnopqrstuvwxyz" {
		add(string(r), model.Rect{X: 0.02, Y: 0, W: 0.45, H: 0.5}, 0.52, none)
	}
	for _, r := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		add(string(r), model.Rect{X: 0.02, Y: 0, W: 0.55, H: 0.7}, 0.62, none)
	}
	return p
}

func testRequest(text string, seed int64) *model.GenerateRequest {
	return &model.GenerateRequest{
		Text: text,
		Seed: seed,
		Page: model.LayoutSpec{
			Width: 1240, Height: 1754,
			MarginSide: 110, MarginTop: 90, MarginBot: 90,
			Columns: 1, Align: "left",
		},
		Paper: model.PaperSpec{Kind: "lined", CellMM: 8},
		Pen:   model.PenSpec{Kind: "ballpoint", Color: "#16203c"},
		Style: model.StyleOverrides{Variability: 1},
	}
}

func build(t *testing.T, req *model.GenerateRequest, prof *model.Profile) []*model.Page {
	t.Helper()
	rng := rand.New(rand.NewPCG(uint64(req.Seed), uint64(req.Seed)>>7))
	pages, err := Build(req, prof, rng)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return pages
}

const sampleText = `Контрольная работа по теме «Производные и дифференциалы».

Съешь же ещё этих мягких французских булок, да выпей чаю. Скорость изменения функции характеризует её производная, а дифференциал описывает главную линейную часть приращения. На практике важно не путать эти понятия, особенно при решении прикладных задач.`

func TestBuildDeterministic(t *testing.T) {
	prof := testProfile(3, true)
	a := build(t, testRequest(sampleText, 42), prof)
	b := build(t, testRequest(sampleText, 42), prof)
	if len(a) != len(b) {
		t.Fatalf("page count differs: %d vs %d", len(a), len(b))
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatal("same seed produced different layout")
	}
}

func TestBuildDifferentSeedsDiffer(t *testing.T) {
	prof := testProfile(3, true)
	a := build(t, testRequest(sampleText, 1), prof)
	b := build(t, testRequest(sampleText, 2), prof)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) == string(jb) {
		t.Fatal("different seeds produced identical layout")
	}
}

func TestOpsStayOnPage(t *testing.T) {
	prof := testProfile(2, true)
	req := testRequest(sampleText, 7)
	pages := build(t, req, prof)
	for _, p := range pages {
		if p.Width != req.Page.Width || p.Height != req.Page.Height {
			t.Fatalf("page size mismatch: %d×%d", p.Width, p.Height)
		}
		for _, op := range p.Ops {
			if op.Sprite == nil {
				t.Fatal("nil sprite in op")
			}
			if op.X < 0 || op.X > float64(p.Width) {
				t.Fatalf("op X off page: %f", op.X)
			}
			if op.Y < 0 || op.Y > float64(p.Height) {
				t.Fatalf("op Y off page: %f", op.Y)
			}
			if op.Scale <= 0 {
				t.Fatalf("bad scale %f", op.Scale)
			}
			if op.Ink <= 0 || op.Ink > 1 {
				t.Fatalf("bad ink %f", op.Ink)
			}
		}
	}
}

func TestWrappingRespectsColumn(t *testing.T) {
	prof := testProfile(2, false)
	req := testRequest(strings.Repeat("Слово ", 200), 3)
	pages := build(t, req, prof)
	right := float64(req.Page.Width) - req.Page.MarginSide + 4
	for _, p := range pages {
		for _, op := range p.Ops {
			extent := op.X + op.Sprite.Box.W*op.Scale + 1
			if extent > right {
				t.Fatalf("glyph extends to %f > %f", extent, right)
			}
		}
	}
}

func TestPagination(t *testing.T) {
	prof := testProfile(2, false)
	short := build(t, testRequest("Одна строка.", 1), prof)
	if len(short) != 1 {
		t.Fatalf("short text should fit one page, got %d", len(short))
	}
	long := build(t, testRequest(strings.Repeat(sampleText+"\n\n", 12), 1), prof)
	if len(long) < 2 {
		t.Fatalf("long text should paginate, got %d pages", len(long))
	}
	for i, p := range long {
		if p.Index != i {
			t.Fatalf("page index %d != %d", p.Index, i)
		}
	}
}

func TestHyphenation(t *testing.T) {
	prof := testProfile(1, false)
	req := testRequest(strings.Repeat("электрофицированный ", 40), 5)
	req.Page.Columns = 3
	pages := build(t, req, prof)
	found := false
	for _, p := range pages {
		for _, op := range p.Ops {
			if op.Sprite.Char == "-" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected at least one hyphenated line")
	}
}

func TestPunctuationSpacing(t *testing.T) {
	prof := testProfile(1, false)
	reqA := testRequest("слово, слово", 9)
	reqB := testRequest("слово , слово", 9)
	a := build(t, reqA, prof)
	b := build(t, reqB, prof)
	width := func(pages []*model.Page) float64 {
		var max float64
		for _, p := range pages {
			for _, op := range p.Ops {
				if x := op.X + op.Sprite.Box.W*op.Scale; x > max {
					max = x
				}
			}
		}
		return max
	}
	wa, wb := width(a), width(b)
	if math.Abs(wa-wb) > 0.5 {
		t.Fatalf("space before a comma should not change the width: %f vs %f", wa, wb)
	}
}

func TestJoinsOnlyInsideWords(t *testing.T) {
	prof := testProfile(2, true)
	req := testRequest("аб вг", 11)
	req.Style.Join = ptr(1.0)
	pages := build(t, req, prof)
	links := 0
	for _, p := range pages {
		for _, op := range p.Ops {
			if op.Link != nil {
				links++
			}
		}
	}
	if links != 2 {
		t.Fatalf("expected 2 intra-word links, got %d", links)
	}
}

func TestNoJoinsWhenProfileIsNotCursive(t *testing.T) {
	prof := testProfile(2, false)
	req := testRequest("абвгд", 11)
	req.Style.Join = ptr(1.0)
	for _, p := range build(t, req, prof) {
		for _, op := range p.Ops {
			if op.Link != nil {
				t.Fatal("print profile must not emit links")
			}
		}
	}
}

func TestFlaws(t *testing.T) {
	prof := testProfile(2, true)
	req := testRequest(sampleText, 13)
	req.Flaws.CrossOut = 1
	req.Flaws.Correction = 0
	req.Flaws.SkipWord = 0
	pages := build(t, req, prof)
	strikes := 0
	for _, p := range pages {
		for _, m := range p.Lines {
			if m.Kind == "strike" {
				strikes++
			}
		}
	}
	if strikes < 2 {
		t.Fatalf("crossOut=1 should strike words, got %d marks", strikes)
	}

	clean := testRequest(sampleText, 13)
	clean.Flaws = model.FlawsSpec{}
	for _, p := range build(t, clean, prof) {
		for _, m := range p.Lines {
			if m.Kind == "strike" {
				t.Fatal("clean request must not strike words")
			}
		}
		if len(p.Flaws) != 0 {
			t.Fatal("clean request must not add page flaws")
		}
	}
}

func TestCorrectionWritesReplacement(t *testing.T) {
	prof := testProfile(2, true)
	req := testRequest("правильное слово", 17)
	req.Flaws.Correction = 1.0
	pages := build(t, req, prof)
	if len(pages) != 1 {
		t.Fatalf("expected one page, got %d", len(pages))
	}
	carets := 0
	for _, m := range pages[0].Lines {
		if m.Kind == "caret" {
			carets++
		}
	}
	if carets == 0 {
		t.Fatal("correction should leave a caret mark")
	}
	// The corrected run is drawn smaller and above the baseline; assert that
	// at least some ops sit clearly above the first line baseline.
	lineY := 0.0
	for _, op := range pages[0].Ops {
		if op.Y > lineY && op.Y-op.Scale*0.7 < 200 {
			lineY = op.Y
		}
	}
	above := 0
	for _, op := range pages[0].Ops {
		if op.Y < lineY-8 {
			above++
		}
	}
	if above == 0 {
		t.Fatal("expected replacement text above the line")
	}
}

func TestMarginNoteRotated(t *testing.T) {
	prof := testProfile(2, true)
	req := testRequest(sampleText, 21)
	req.Flaws.MarginNote = 1.0
	pages := build(t, req, prof)
	rotated := 0
	for _, op := range pages[0].Ops {
		if op.Rotate < -80 {
			rotated++
		}
	}
	if rotated == 0 {
		t.Fatal("margin note should be rotated")
	}
}

func TestColumns(t *testing.T) {
	prof := testProfile(2, false)
	req := testRequest(strings.Repeat(sampleText+"\n", 6), 23)
	req.Page.Columns = 2
	pages := build(t, req, prof)
	if len(pages) == 0 {
		t.Fatal("no pages")
	}
	mid := float64(req.Page.Width) / 2
	left, right := 0, 0
	for _, p := range pages {
		for _, op := range p.Ops {
			if op.X < mid {
				left++
			} else {
				right++
			}
		}
	}
	if left == 0 || right == 0 {
		t.Fatalf("two columns expected, got left=%d right=%d", left, right)
	}
}

func TestTitleAndPageNumbers(t *testing.T) {
	prof := testProfile(2, false)
	req := testRequest(sampleText, 31)
	req.Title = "Конспект"
	req.Page.PageNumber = true
	pages := build(t, req, prof)
	if len(pages) == 0 {
		t.Fatal("no pages")
	}
	// Page number sits in the bottom margin.
	bottom := float64(req.Page.Height) - float64(req.Page.MarginBot)
	nums := 0
	for _, op := range pages[len(pages)-1].Ops {
		if op.Y > bottom && unicode.IsDigit(firstRuneOf(op.Sprite.Char)) {
			nums++
		}
	}
	if nums == 0 {
		t.Fatal("expected a page number in the bottom margin")
	}
}

func TestLinedBaselineOnRuling(t *testing.T) {
	prof := testProfile(2, false)
	req := testRequest(sampleText, 37)
	pages := build(t, req, prof)
	dpi := 25.4 * float64(req.Page.Width) / 210.0
	cell := req.Paper.CellMM * dpi / 25.4
	if len(pages) == 0 {
		t.Fatal("no pages")
	}
	// Every glyph must sit close to a printed ruling; the drift is allowed to
	// move the text a little, but never off the line.
	var offs []float64
	for _, op := range pages[0].Ops {
		rem := math.Mod(op.Y, cell)
		off := math.Min(rem, cell-rem)
		offs = append(offs, off)
	}
	if len(offs) == 0 {
		t.Fatal("no ops")
	}
	sort.Float64s(offs)
	p95 := offs[int(float64(len(offs))*0.95)]
	if p95 > cell*0.15 {
		t.Fatalf("95%% of baselines are %.2f px off the ruling (cell %.2f)", p95, cell)
	}
	if offs[len(offs)-1] > cell*0.35 {
		t.Fatalf("baseline drifted %.2f px off the ruling (cell %.2f)", offs[len(offs)-1], cell)
	}
}

func TestEmptyAndWeirdInput(t *testing.T) {
	prof := testProfile(1, true)
	for _, text := range []string{"", "   ", "\n\n", "😀🙈", "Ω≈ç√", strings.Repeat("Ж", 400)} {
		req := testRequest(text, 3)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic on %q: %v", text, r)
				}
			}()
			_, _ = Build(req, prof, rand.New(rand.NewPCG(1, 2)))
		}()
	}
}

func TestBuildRejectsEmptyProfile(t *testing.T) {
	prof := &model.Profile{Glyphs: map[string][]model.Sprite{}}
	_, err := Build(testRequest("тест", 1), prof, rand.New(rand.NewPCG(1, 2)))
	if err == nil {
		t.Fatal("expected an error for a profile without glyphs")
	}
}

func TestJitterDamping(t *testing.T) {
	prof := testProfile(3, true)
	req := testRequest(strings.Repeat("Иван иванов ИВАН 1234., ", 20), 41)
	pages := build(t, req, prof)
	var lower, upper float64
	var nLower, nUpper int
	for _, p := range pages {
		for _, op := range p.Ops {
			r := firstRuneOf(op.Sprite.Char)
			if unicode.IsLower(r) {
				lower += math.Abs(op.Rotate)
				nLower++
			} else if unicode.IsUpper(r) {
				upper += math.Abs(op.Rotate)
				nUpper++
			}
		}
	}
	if nLower == 0 || nUpper == 0 {
		t.Fatal("profile lacks cases")
	}
	if lower/float64(nLower) <= upper/float64(nUpper) {
		t.Fatal("lowercase should be jittered more than uppercase")
	}
}

func ptr(f float64) *float64 { return &f }

func firstRuneOf(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

func TestJustifyStretchesAndStaysInColumn(t *testing.T) {
	prof := testProfile(1, false)
	req := testRequest(strings.Repeat(sampleText+"\n", 4), 51)
	req.Page.Columns = 2
	req.Page.Align = "justify"
	p, err := resolveParams(req, prof)
	if err != nil {
		t.Fatal(err)
	}
	pages := build(t, req, prof)
	filled := 0
	for _, pg := range pages {
		for _, op := range pg.Ops {
			if op.Y > p.maxBaseline {
				continue // page number
			}
			in := false
			for c := 0; c < p.cols; c++ {
				left := p.x0 + float64(c)*(p.colW+p.colGap)
				if op.X >= left-1 && op.X <= left+p.colW+2 {
					in = true
					break
				}
			}
			if !in {
				t.Fatalf("glyph %q at x=%.1f fell outside every column (colW=%.1f)", op.Sprite.Char, op.X, p.colW)
			}
			right := p.x0 + p.colW
			if op.X > right*0.9 {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Fatal("justified text never reached the right edge of the column")
	}
}

func TestCenterAlign(t *testing.T) {
	prof := testProfile(1, false)
	req := testRequest("Короткая строка", 53)
	req.Page.Align = "center"
	req.Page.Indent = 0.01 // disable the paragraph indent
	p, err := resolveParams(req, prof)
	if err != nil {
		t.Fatal(err)
	}
	pages := build(t, req, prof)
	minX, maxX := math.Inf(1), math.Inf(-1)
	for _, op := range pages[0].Ops {
		if op.X < minX {
			minX = op.X
		}
		w := op.Sprite.Box.W * op.Scale
		if op.X+w > maxX {
			maxX = op.X + w
		}
	}
	mid := minX + (maxX-minX)/2
	want := p.x0 + p.colW/2
	if math.Abs(mid-want) > 0.04*p.colW {
		t.Fatalf("centred line mid %.1f, column centre %.1f", mid, want)
	}
}

func TestMultiLinePageFill(t *testing.T) {
	prof := testProfile(2, false)
	req := testRequest(strings.Repeat("Строка текста для проверки заполнения страницы. ", 400), 57)
	p, err := resolveParams(req, prof)
	if err != nil {
		t.Fatal(err)
	}
	pages := build(t, req, prof)
	if len(pages) < 2 {
		t.Fatalf("expected several pages, got %d", len(pages))
	}
	for i, pg := range pages {
		if i == len(pages)-1 {
			continue
		}
		maxY := 0.0
		for _, op := range pg.Ops {
			if op.Y > maxY {
				maxY = op.Y
			}
		}
		if maxY < p.maxBaseline-p.linePitch*1.5 {
			t.Fatalf("page %d is underfilled: last baseline %.1f, limit %.1f", i, maxY, p.maxBaseline)
		}
	}
}
