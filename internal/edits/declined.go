package edits

// DeclinedFinding is a blocking finding the model declined, with its reason.
type DeclinedFinding struct {
	Finding Finding
	Reason  string
}

// Declined returns each blocking finding in fs that rs resolves declined,
// in fs order, for that seat's next read. A later resolution of a finding
// in rs overrides an earlier one.
func Declined(fs []Finding, rs []Resolution) []DeclinedFinding {
	byFinding := make(map[int]Resolution, len(rs))
	for _, r := range rs {
		byFinding[r.Finding] = r
	}
	var out []DeclinedFinding
	for _, f := range fs {
		r, ok := byFinding[f.N]
		if f.Blocking && ok && r.Status == StatusDeclined {
			out = append(out, DeclinedFinding{Finding: f, Reason: r.Reason})
		}
	}
	return out
}
