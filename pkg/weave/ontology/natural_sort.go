package ontology

import (
	"strings"
	"unicode"
)

// naturalSortPadWidth is the zero-pad width applied to each numeric run
// inside an ontology local-name when computing a sort key. Four digits
// covers any sane ontology: CRM tops at E89/P195, and four leaves headroom
// up to 9999. Bumping this requires regenerating every stored
// natural_sort_key, so prefer to live with four.
const naturalSortPadWidth = 4

// naturalSortKey turns an ontology local-name into a lexically-sortable key
// that yields natural numeric order. It tokenises the input into alternating
// runs of digits and non-digits, zero-pads each digit run to
// naturalSortPadWidth, and concatenates.
func naturalSortKey(localName string) string {
	if localName == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(localName) + naturalSortPadWidth)

	runes := []rune(localName)
	i := 0
	for i < len(runes) {
		if unicode.IsDigit(runes[i]) {
			j := i
			for j < len(runes) && unicode.IsDigit(runes[j]) {
				j++
			}
			run := string(runes[i:j])
			if pad := naturalSortPadWidth - (j - i); pad > 0 {
				b.WriteString(strings.Repeat("0", pad))
			}
			b.WriteString(run)
			i = j
			continue
		}
		j := i
		for j < len(runes) && !unicode.IsDigit(runes[j]) {
			j++
		}
		b.WriteString(string(runes[i:j]))
		i = j
	}
	return b.String()
}
