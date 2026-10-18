package doc

// Mode picks a speed.
const (
	ModeFast = "fast"
	ModeSlow = "slow"
)

// Runner runs jobs.
type Runner struct{}

// Run starts the runner.
func (r *Runner) Run(job string) error {
	return nil
}

// Stop halts the runner.
func Stop() {}
