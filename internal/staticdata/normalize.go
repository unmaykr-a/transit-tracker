package staticdata

import (
	"regexp"
	"strings"
	"unicode"
)

var foldMap = map[rune]string{
	'ä': "a", 'á': "a", 'à': "a", 'â': "a", 'å': "a", 'ā': "a",
	'ö': "o", 'ó': "o", 'ò': "o", 'ô': "o", 'õ': "o", 'ø': "o",
	'ü': "u", 'ú': "u", 'ù': "u", 'û': "u", 'ū': "u",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e", 'ē': "e",
	'í': "i", 'ì': "i", 'î': "i", 'ï': "i", 'ī': "i",
	'š': "s", 'ž': "z", 'č': "c", 'ç': "c", 'ņ': "n", 'ñ': "n", 'ļ': "l", 'ķ': "k", 'ģ': "g",
	'ß': "ss",
}

// Fold lowercases, strips diacritics and collapses punctuation to single spaces,
// so "Ülemiste" and "ulemiste" (or "Balti jaam" and "balti-jaam") compare equal.
func Fold(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		switch {
		case foldMap[r] != "":
			b.WriteString(foldMap[r])
			space = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// English annotations the operator appends to some names: "(train station)", "(bus station)", "(port)".
var annotation = regexp.MustCompile(`\s*\((?:[a-z]+ )?(?:station|port|terminal)\)$|\s*\(ajutine\)$`)

// cleanName drops markers that are not part of a stop's name ("Bussijaam (bus station)" → "Bussijaam").
func cleanName(s string) string {
	return strings.TrimSpace(annotation.ReplaceAllString(strings.TrimSpace(s), ""))
}
