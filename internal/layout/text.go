package layout

import (
	"strings"
	"unicode"
)

// Russian and Latin vowels are the allowed hyphenation points in this
// heuristic; we do not ship a dictionary, we only need plausible breaks.
const vowels = "аеёиоуыэюяАЕЁИОУЫЭЮЯaeiouyAEIOUY"

// part is one word of a source line with the pen gap that precedes it.
type part struct {
	text string
	gap  float64 // em
}

// splitParagraph splits a source line into words, dropping any run of spaces.
func splitParagraph(line string) []string {
	return strings.Fields(line)
}

// measure returns the nominal width of s in em units using per-rune mean
// advances. It is deliberately independent of the concrete variant so that
// wrapping stays stable while the renderer still picks variants freely.
func (b *builder) measure(s string) float64 {
	r := b.p
	var w float64
	for _, ch := range s {
		if ch == ' ' {
			w += r.wordGap
			continue
		}
		if a, ok := r.meanAdv[ch]; ok {
			w += a
			continue
		}
		if alt, ok := r.alt[ch]; ok {
			if a, ok := r.meanAdv[alt]; ok {
				w += a
				continue
			}
		}
		if a, ok := r.meanAdv[unicode.ToLower(ch)]; ok {
			w += a
			continue
		}
		if a, ok := r.meanAdv[unicode.ToUpper(ch)]; ok {
			w += a
			continue
		}
		w += 0.36
	}
	return w * r.advScale
}

// isPunctBreak reports whether a space before this rune should be dropped.
func tightBefore(ch rune) bool {
	switch ch {
	case '.', ',', ';', ':', '!', '?', ')', ']', '}', '»', '%':
		return true
	}
	return false
}

// tightAfter reports whether a space after this rune should be dropped.
func tightAfter(ch rune) bool {
	switch ch {
	case '(', '[', '{', '«', '№':
		return true
	}
	return false
}

// gapFor computes the word gap in em between two adjacent words, honouring
// basic Russian punctuation spacing.
func (b *builder) gapFor(prev, next string) float64 {
	g := b.p.wordGap
	if prev != "" {
		last, _ := lastRune(prev)
		if tightAfter(last) {
			return g * 0.18
		}
	}
	if next != "" {
		first, _ := firstRune(next)
		if tightBefore(first) {
			// A stray space before punctuation is swallowed entirely.
			return 0
		}
		if first == '—' || first == '–' {
			return g * 1.1
		}
	}
	if prev != "" {
		last, _ := lastRune(prev)
		if last == '—' || last == '–' {
			return g * 1.1
		}
	}
	return g
}

func firstRune(s string) (rune, bool) {
	for _, r := range s {
		return r, true
	}
	return 0, false
}

func lastRune(s string) (rune, bool) {
	var r rune
	ok := false
	for _, c := range s {
		r, ok = c, true
	}
	return r, ok
}

// hyphenate splits word so that head+"-" fits into avail em. It prefers a
// break after a vowel, then after a hyphen already in the word, then a hard
// rune break. Every returned head ends with "-".
func (b *builder) hyphenate(word string, avail float64) (head, tail string, ok bool) {
	rs := []rune(word)
	if len(rs) < 5 || avail < 1.2 {
		return "", "", false
	}
	best := -1
	// Preferred: after a vowel, at least 2 runes on each side.
	for i := 1; i < len(rs)-1; i++ {
		if !strings.ContainsRune(vowels, rs[i]) {
			continue
		}
		if i+1 < 2 || len(rs)-(i+1) < 2 {
			continue
		}
		h := string(rs[:i+1]) + "-"
		if b.measure(h) <= avail-0.05 {
			best = i + 1
		}
	}
	// Compound words: keep the existing hyphen.
	for i := 1; i < len(rs)-1; i++ {
		if rs[i] != '-' {
			continue
		}
		if b.measure(string(rs[:i+1])) <= avail-0.05 && i+1 > 1 && len(rs)-(i+1) >= 1 {
			return string(rs[:i+1]), string(rs[i+1:]), true
		}
	}
	if best > 0 {
		return string(rs[:best]) + "-", string(rs[best:]), true
	}
	// Last resort: hard break with a hyphen when it helps at all.
	for i := len(rs) - 1; i >= 2; i-- {
		h := string(rs[:i]) + "-"
		if b.measure(h) <= avail-0.05 {
			return h, string(rs[i:]), true
		}
	}
	return "", "", false
}

// hardBreak cuts word at the last rune boundary that fits in avail without a
// hyphen. Used for pathological unbreakable tokens.
func (b *builder) hardBreak(word string, avail float64) (head, tail string) {
	rs := []rune(word)
	var width float64
	cut := 0
	for i, r := range rs {
		w := b.p.advScale * runeAdvance(b.p.meanAdv, r)
		if width+w > avail && i > 0 {
			break
		}
		width += w
		cut = i + 1
	}
	if cut == 0 || cut >= len(rs) {
		return "", word
	}
	return string(rs[:cut]), string(rs[cut:])
}

func runeAdvance(mean map[rune]float64, r rune) float64 {
	if a, ok := mean[r]; ok {
		return a
	}
	return 0.36
}
