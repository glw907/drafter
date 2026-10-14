package group

const (
	// Mention is only named in this comment line.
	Alpha = 1
	Beta  = Mention + 1
)

var (
	Gamma string
	Delta = "d"
)

type (
	Epsilon struct {
		Inner int
	}
	Zeta int
)

func body() {
	Mention := 1
	_ = Mention
}
