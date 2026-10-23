package plan

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// CheckExhaustive names the check that warns on a claim of a complete set.
const CheckExhaustive = "exhaustive"

var (
	codeSpan       = regexp.MustCompile("`[^`]*`")
	exhaustiveWord = map[string]bool{"only": true, "every": true, "all": true, "no": true, "none": true, "never": true, "always": true}
	countWord      = map[string]bool{
		"two": true, "three": true, "four": true, "five": true, "six": true, "seven": true,
		"eight": true, "nine": true, "ten": true, "eleven": true, "twelve": true,
	}
)

// ExhaustiveClaim returns the first phrase in text that claims a complete set, or "" when there is
// none. It is one of the words only, every, all, no, none, never, and always, or a count spelled
// from two to twelve before a plural noun. A digit count is no claim of a complete set: it is as
// often a version or a measurement. Words match whole and without case, and a code span is skipped.
func ExhaustiveClaim(text string) string {
	words := strings.FieldsFunc(codeSpan.ReplaceAllString(text, " "), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '\''
	})
	for i, w := range words {
		w = strings.ToLower(w)
		if exhaustiveWord[w] {
			return w
		}
		if i+1 == len(words) || !countWord[w] {
			continue
		}
		if next := strings.ToLower(words[i+1]); len(next) > 2 && strings.HasSuffix(next, "s") && !strings.HasSuffix(next, "ss") {
			return w + " " + next
		}
	}
	return ""
}

// ExhaustiveFinding is the warning for a line whose text holds claim, a phrase ExhaustiveClaim
// returned.
func ExhaustiveFinding(file string, line int, claim string) Finding {
	return Finding{
		File: file, Line: line, Severity: SeverityWarning, Check: CheckExhaustive,
		Message: fmt.Sprintf("%q claims a complete set, and a draft may repeat the claim as fact", claim),
		Fix:     "check that the source says the set is complete, or give examples instead",
	}
}
