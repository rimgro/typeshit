package api

import (
	"regexp"
	"strings"
)

// Markdown import: the generator writes letters, not markup, so a .md document
// is reduced to the words a person would actually copy out by hand. Headings
// stay as their text, lists lose their bullets, links keep the anchor text,
// images and rules disappear.

var (
	mdFence      = regexp.MustCompile("^\\s*(```|~~~)")
	mdHeading    = regexp.MustCompile(`^\s{0,3}#{1,6}\s+`)
	mdSetext     = regexp.MustCompile(`^\s{0,3}(=+|-{3,})\s*$`)
	mdRule       = regexp.MustCompile(`^\s{0,3}((\*\s*){3,}|(-\s*){3,}|(_\s*){3,})$`)
	mdBullet     = regexp.MustCompile(`^\s{0,3}[-*+]\s+(\[[ xX]\]\s+)?`)
	mdOrdered    = regexp.MustCompile(`^\s{0,3}\d{1,3}[.)]\s+`)
	mdQuote      = regexp.MustCompile(`^\s{0,3}>\s?`)
	mdTableSep   = regexp.MustCompile(`^\s*\|?(\s*:?-{2,}:?\s*\|)+\s*:?-{2,}:?\s*\|?\s*$`)
	mdImage      = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdLink       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdRefLink    = regexp.MustCompile(`\[([^\]]*)\]\[[^\]]*\]`)
	mdAutolink   = regexp.MustCompile(`<((?:https?|mailto):[^>\s]+)>`)
	mdRefDef     = regexp.MustCompile(`^\s{0,3}\[[^\]]+\]:\s*\S+.*$`)
	mdHTMLTag    = regexp.MustCompile(`</?[A-Za-z][^>]*>`)
	mdComment    = regexp.MustCompile(`<!--(?s:.*?)-->`)
	mdEscapeChar = regexp.MustCompile(`\\([\\` + "`" + `*_{}\[\]()#+\-.!>~|])`)
)

// markdownText reduces CommonMark-ish source to plain prose. It is deliberately
// tolerant: anything it does not recognise survives as literal text.
func markdownText(src string) string {
	var out []string
	inFence := false
	src = mdComment.ReplaceAllString(strings.ReplaceAll(src, "\r\n", "\n"), "")
	for _, line := range strings.Split(src, "\n") {
		if mdFence.MatchString(line) {
			inFence = !inFence
			out = append(out, "")
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}
		if mdRefDef.MatchString(line) || mdRule.MatchString(line) || mdTableSep.MatchString(line) {
			out = append(out, "")
			continue
		}
		line = mdHeading.ReplaceAllString(line, "")
		line = mdSetext.ReplaceAllString(line, "")
		line = mdQuote.ReplaceAllString(line, "")
		line = mdBullet.ReplaceAllString(line, "")
		line = mdOrdered.ReplaceAllString(line, "")
		if strings.HasPrefix(strings.TrimSpace(line), "|") && strings.HasSuffix(strings.TrimSpace(line), "|") {
			cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
			for i := range cells {
				cells[i] = strings.TrimSpace(cells[i])
			}
			line = strings.Join(cells, " ")
		}
		line = inlineMarkdown(line)
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// inlineMarkdown strips the character-level markup from a single line.
func inlineMarkdown(line string) string {
	line = mdImage.ReplaceAllString(line, "")
	line = mdLink.ReplaceAllString(line, "$1")
	line = mdRefLink.ReplaceAllString(line, "$1")
	line = mdAutolink.ReplaceAllString(line, "$1")
	line = mdHTMLTag.ReplaceAllString(line, "")
	line = mdEscapeChar.ReplaceAllString(line, "$1")
	for _, pair := range [][2]string{
		{"**", ""}, {"__", ""}, {"~~", ""}, {"`", ""},
	} {
		line = strings.ReplaceAll(line, pair[0], pair[1])
	}
	// A lone * or _ is emphasis markup, but it also occurs in real text
	// (snake_case, 2*2); only drop it when it wraps a word.
	for _, mark := range []byte{'*', '_'} {
		line = stripEmphasis(line, mark)
	}
	return line
}

// stripEmphasis removes *word* / _word_ runs while leaving snake_case and
// arithmetic alone: a mark only opens emphasis at a word boundary and only
// closes it before a boundary.
func stripEmphasis(line string, mark byte) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] != mark {
			b.WriteByte(line[i])
			i++
			continue
		}
		if i+1 < len(line) && isBoundaryBefore(line, i) && !isBoundaryAfter(line, i+1) {
			j := strings.IndexByte(line[i+1:], mark)
			if j > 0 {
				j += i + 1
				inner := line[i+1 : j]
				if !strings.ContainsAny(inner, "_*") && isBoundaryAfter(line, j+1) {
					b.WriteString(inner)
					i = j + 1
					continue
				}
			}
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

func isBoundaryBefore(line string, i int) bool {
	if i == 0 {
		return true
	}
	return strings.IndexByte(" \t([{\"'>-—–/", line[i-1]) >= 0
}

func isBoundaryAfter(line string, i int) bool {
	if i >= len(line) {
		return true
	}
	return strings.IndexByte(" \t)]}\"'.,;:!?—–/", line[i]) >= 0
}
