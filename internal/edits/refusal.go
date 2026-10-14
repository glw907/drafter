package edits

import "strings"

// Refusal is the error Apply returns for an answer that breaks the edit
// contract. Problems go back to the model verbatim.
type Refusal struct {
	Problems []string
}

func (r *Refusal) Error() string {
	return "edit contract refused the answer: " + strings.Join(r.Problems, "; ")
}
