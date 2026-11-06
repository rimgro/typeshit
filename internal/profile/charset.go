package profile

import "strings"

const (
	ruLower = "абвгдеёжзийклмнопрстуфхцчшщъыьэюя"
	ruUpper = "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ"
	digits  = "0123456789"
	latinLo = "abcdefghijklmnopqrstuvwxyz"
	latinUp = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// punct covers what Russian school work actually uses.
	punct = ".,;:!?-()«»\"'/\\*+="
)

// Charset returns the characters a sample sheet asks the user to write, in
// the order they appear on the sheet (and therefore in spec.Chars).
//
// "ru"       – full Russian school alphabet, digits and punctuation;
// "ru-short" – letters and digits only (fewer pages to write);
// anything else – Latin letters, digits and punctuation.
func Charset(lang string) []string {
	var sb strings.Builder
	switch lang {
	case "ru":
		sb.WriteString(ruLower + ruUpper + digits + punct)
	case "ru-short":
		sb.WriteString(ruLower + ruUpper + digits)
	case "latin":
		sb.WriteString(latinLo + latinUp + digits + punct)
	default:
		sb.WriteString(latinLo + latinUp + digits + punct)
	}
	out := make([]string, 0, len(sb.String()))
	seen := make(map[rune]bool)
	for _, r := range sb.String() {
		if seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, string(r))
	}
	return out
}

// isUpper reports whether s starts with an uppercase letter (any script).
func isUpper(s string) bool {
	for _, r := range s {
		return strings.ToUpper(string(r)) == string(r) && strings.ToLower(string(r)) != string(r)
	}
	return false
}

// lowerRune returns the lowercase form of the first rune.
func lowerRune(s string) string { return strings.ToLower(s) }
