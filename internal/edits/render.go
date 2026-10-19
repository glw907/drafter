package edits

import (
	"fmt"
	"strings"
)

// Instructions returns the contract as the model reads it, to append to
// every redraft and gate-fix turn.
func Instructions() string {
	return `Answer with exact edits and one resolution per numbered finding.

Each edit names a file (page or brief), an old span, and its replacement. Edits apply in the order you give them, and each old span must occur exactly once in the file as the earlier edits in your answer left it. Old and new must differ.

Resolve every numbered finding exactly once, by its number: status edited with the indexes of the edits that address it, or status declined with a reason. Edit indexes are 0-based and point into the edits array of this same answer: the first edit is 0. Every edit must be named by some resolution. A blocking finding resolved edited must name at least one edit. Decline a blocking finding only when it is factually wrong about the page.

The brief must still be valid JSON after your edits. An answer that breaks any of these rules is refused with the problems listed, and you answer again; resolutions from earlier answers stand unless you resolve the same finding again.`
}

// BoundInstructions returns the sentence a bounded turn appends after
// Instructions. Instructions allows declining a blocking finding only when
// it is wrong about the page, and a finding located outside the sections a
// bound allows has no edit that could fix it.
func BoundInstructions() string {
	return "This answer may change only the sections the redraft names, so resolve a finding located outside them as declined, with the reason that it lies outside the sections this redraft may change."
}

// Render returns the numbered findings text a redraft answers.
func Render(fs []Finding) string {
	var b strings.Builder
	for i, f := range fs {
		if i > 0 {
			b.WriteString("\n")
		}
		kind := "advisory"
		if f.Blocking {
			kind = "blocking"
		}
		fmt.Fprintf(&b, "Finding %d (%s, %s)\n", f.N, f.Seat, kind)
		fmt.Fprintf(&b, "Location: %s\n", f.Location)
		fmt.Fprintf(&b, "Text: %s\n", f.Text)
		fmt.Fprintf(&b, "Rewrite: %s\n", f.Rewrite)
	}
	return b.String()
}
