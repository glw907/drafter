// Package config loads drafter's three configuration layers, the environment, and the flags,
// resolves each setting with the source it came from, and refuses what the layers may not hold.
package config

import "errors"

// Config holds every resolved setting. Every path is absolute after Load.
type Config struct {
	Prices          Prices
	BillingCycleDay int
	WorkspaceID     string
	KeyEnv          string
	KeyCommand      string
	APIBaseURL      string
	PageCap         float64 // US dollars
	CycleCeiling    float64 // US dollars
	Mode            string  // batch or sync
	StateDir        string
	RetentionDays   int
	Output          string // human or json
	PagesInFlight   int
	Retries         Retries
	Gate            Gate
	Prompts         string
	Seats           []string // the seats that run; draft is always among them
	Seat            map[string]Seat
	Facts           Facts
	Register        map[string][]string
	InlineFiles     []string
	Root            string // the project root, empty outside a project
}

// Retries holds the retry budgets.
type Retries struct {
	GateFix      int
	EditContract int
}

// Gate holds the gate settings.
type Gate struct {
	Command          string
	IndexLinkCommand string
	Protocol         string
	Lane             string
	SectionPattern   string
	FailurePattern   string
}

// Seat holds one seat's settings.
type Seat struct {
	Agent     string // an agent definition file, optional
	Model     string
	Effort    string
	MaxTokens int64
	Inputs    []string // command templates
}

// Prices is the dated price table.
type Prices struct {
	Date   string
	Source string
	// BatchDiscount is the fraction taken off every rate in batch mode, from 0 up to but not
	// including 1. Unset means 0, so a missing value overstates cost and never understates it.
	BatchDiscount float64
	Models        map[string]Price
}

// Price holds one model's rates in US dollars per million tokens.
type Price struct {
	Input        float64
	Output       float64
	CacheWrite5m float64
	CacheWrite1h float64
	CacheRead    float64
	LongContext  *Tier // the second tier, when the model has one
}

// Tier is a model's long-context rate tier. It applies to a request whose total input (input,
// cache-read, and cache-write tokens) is over Threshold, and its rates replace all five.
type Tier struct {
	Threshold    int64
	Input        float64
	Output       float64
	CacheWrite5m float64
	CacheWrite1h float64
	CacheRead    float64
}

// Facts holds the facts-store settings.
type Facts struct {
	Dir          string
	BulletFormat string
}

// Flags holds the only two settings that have command-line flags.
type Flags struct {
	Sync bool
	JSON bool
}

// Source is one setting's resolved value and where it came from.
type Source struct {
	Key   string
	Value string
	From  string   // the layer, variable, or flag that set it, or "default" or "unset"
	Sets  []string // for a cap, every source that set it as "source: value", lowest layer first
}

// Load reads the layers for the project at root and returns the resolved configuration.
func Load(root string, flags Flags) (*Config, error) {
	return nil, errors.New("config: not implemented")
}

// Sources returns one row per setting. It never carries a key.
func (c *Config) Sources() []Source {
	return nil
}
