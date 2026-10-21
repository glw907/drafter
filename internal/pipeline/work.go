package pipeline

import (
	"encoding/json"

	"github.com/glw907/drafter/internal/batch"
	"github.com/glw907/drafter/internal/blueprint"
	"github.com/glw907/drafter/internal/edits"
)

// The steps of a page, in the order a clean page takes them. PageState.Step holds one.
const (
	stPlan         = "plan"
	stBpCheck      = "bpcheck"
	stBpRead       = "bpread"
	stConsistency  = "consistency"
	stDraft        = "draft"
	stGateFix      = "gatefix"
	stCoverage     = "coverage"
	stRound1       = "round1"
	stRedraft      = "redraft"
	stRound2       = "round2"
	stReader1      = "reader1"
	stReader2      = "reader2"
	stFinalRedraft = "finalredraft"
	stScoped       = "scoped"
	stDone         = "done"
	// A section redraft's own steps: the section exchange in place of the draft, and the coherence
	// read and its fix exchange at the point the page would otherwise accept.
	stSection      = "section"
	stCoherence    = "coherence"
	stCoherenceFix = "coherencefix"
	// stEdit asks the edit pass's turns: its one edit, then any gate-fix turn after it.
	stEdit = "edit"
	// stSmall asks the one scoped fix of small findings that a fix verdict in round 2, or a miss
	// in the coverage check after the redraft, opens in place of an escalation.
	stSmall = "small"
)

// Stage tokens in a custom_id, the spec's stage numbers.
const (
	s1 = "s1"
	s2 = "s2"
	s3 = "s3"
	s4 = "s4"
	s5 = "s5"
	s6 = "s6"
	// s6b is the small-finding fix, which runs after round 2 or in its place.
	s6b = "s6b"
	s7  = "s7"
	s8  = "s8"
	// s2e is the edit pass, which follows the draft's gate.
	s2e = "s2e"
)

// Exchange kinds. A draft exchange is the gate-fix conversation after the draft; a redraft
// exchange answers round 1's findings; a final exchange answers the final reader's. A section
// exchange redrafts a section redraft's changed sections, and a coherence exchange answers the
// coherence read's findings. A small exchange answers the open small findings alone.
const (
	exDraft     = "draft"
	exRedraft   = "redraft"
	exFinal     = "final"
	exSection   = "section"
	exCoherence = "coherence"
	exSmall     = "small"
	// exEdit is the edit pass: one exchange on the edit seat, opened once after the draft's gate
	// holds, whose later turns answer a red gate.
	exEdit = "edit"
)

// exchange is one edit-contract conversation: the turns so far, every numbered finding it
// raised, the resolutions that stand, and the text of the next user turn.
type exchange struct {
	Kind        string             `json:"kind"`
	History     []batch.Message    `json:"history,omitempty"`
	Findings    []edits.Finding    `json:"findings,omitempty"`
	Resolutions []edits.Resolution `json:"resolutions,omitempty"`
	Turn        string             `json:"turn"`
	// GateFix says a red gate has reopened the exchange. Every later turn of it answers gate
	// findings or a refusal of them, so it stays set, and no region bound applies to those turns.
	GateFix bool `json:"gate_fix,omitempty"`
}

// seat names the seat that answers the exchange's turns: the edit seat for the edit pass, and the
// draft seat for every other exchange.
func (ex *exchange) seat() string {
	if ex.Kind == exEdit {
		return "edit"
	}
	return "draft"
}

// coverageState is where a coverage check stands: its phase (initial, redraft, final, coherence,
// or small), the mechanical misses, and whether the model's read of the claims is in.
type coverageState struct {
	Phase    string       `json:"phase"`
	MechDone bool         `json:"mech_done,omitempty"`
	Misses   []missRecord `json:"misses,omitempty"`
	ReadIn   bool         `json:"read_in,omitempty"`
}

// missRecord is a coverage miss as the page keeps it.
type missRecord struct {
	Item    string `json:"item"`
	Section string `json:"section"`
	Kind    string `json:"kind"`
	Reason  string `json:"reason"`
}

// held says where a page waits on a gate that was red with nothing of its own.
type held struct {
	Kind    string `json:"kind"`
	OnGreen string `json:"on_green"`
}

// work is the pipeline's working memory for one page. It is saved in the page's state after
// every step, so a resume continues from it.
type work struct {
	// Got holds the accepted answers of the current step's seats, so a seat that failed can be
	// asked again alone.
	Got  map[string]string `json:"got,omitempty"`
	Ex   *exchange         `json:"ex,omitempty"`
	Cov  coverageState     `json:"cov"`
	Held *held             `json:"held,omitempty"`
	// Baseline is the git object ID of the round-1 snapshot.
	Baseline string `json:"baseline,omitempty"`
	// Round1 holds each seat's round-1 read as the seat answered it.
	Round1 map[string]string `json:"round1,omitempty"`
	// CovFindings are the stage-3 misses as numbered findings, carried to the redraft.
	CovFindings []edits.Finding `json:"cov_findings,omitempty"`
	// Reader is the final reader's conversation and whether this is the retest.
	Reader readerState `json:"reader"`
	// Cited is the final reader's findings as the final redraft and scoped reads cite them.
	Cited string `json:"cited,omitempty"`
	// Budgets counts the retries each failure row has spent, keyed by row.
	Budgets map[string]int `json:"budgets,omitempty"`
	// Gates counts the gate runs, for log names.
	Gates int `json:"gates"`
	// IndexPre is each file the index-link command changed, as it was before the page's first
	// run of the command: its content, or absent.
	IndexPre map[string]*string `json:"index_pre,omitempty"`
	// IndexPost is the hash each of those files had right after the page's last run of the
	// command, so a restore can tell whether another page has changed the file since.
	IndexPost map[string]string `json:"index_post,omitempty"`
	Rec       record            `json:"rec"`
	// PendingGate names the exchange kind of a write whose gate has not finished, so a resume
	// that finds the files written runs the gate and never asks the model again.
	PendingGate string `json:"pending_gate,omitempty"`
	// Sent holds the turns of the request most recently built for each seat, which an answer
	// extends into a conversation.
	Sent map[string][]batch.Message `json:"sent,omitempty"`
	// DraftHist is the draft seat's conversation after the draft, the start of a gate-fix exchange.
	DraftHist []batch.Message `json:"draft_hist,omitempty"`
	// Accepted is the git blob id of the blueprint a blueprint page's read accepted, as it was at
	// the run's start commit.
	Accepted string `json:"accepted,omitempty"`
	// Since is the commit a blueprint read compared against and Scope the page's comparison. A
	// resume reads both from here, so it needs no revision of its own. Both are empty for an
	// ordinary read, which is how a state from before they existed resumes: as a whole read.
	Since string           `json:"since,omitempty"`
	Scope *blueprint.Scope `json:"scope,omitempty"`
	// Change is the change block the page's read sends after the blueprint, built once at the start
	// of the run so a resume sends the same bytes. Demoted counts the conflicts the consistency
	// rule reported as a warning, already present or unplaced.
	Change  string `json:"change,omitempty"`
	Demoted int    `json:"demoted,omitempty"`
	// Draft says the page drafts after its blueprint read, and SkipRead that the read is skipped.
	// A state saved before drafting existed decodes both as false: a read, not a draft.
	Draft    bool `json:"draft,omitempty"`
	SkipRead bool `json:"skip_read,omitempty"`
	// Edit says the page gets the edit pass after its draft's gate holds, and Edited that the pass
	// has been opened. Narrow says the page runs without --judge: it takes no blueprint read, no
	// review round, no reader, no coverage read, and no coherence read, and is accepted on the
	// gate after the edit pass. A state saved before they existed decodes all three as false.
	Edit   bool `json:"edit,omitempty"`
	Edited bool `json:"edited,omitempty"`
	Narrow bool `json:"narrow,omitempty"`
	// Section says the page is a section redraft, decided once when it reached its draft path,
	// since the page changes after the first write. CoherenceRetest says the coherence read is the
	// retest after its fix.
	Section         bool `json:"section,omitempty"`
	CoherenceRetest bool `json:"coherence_retest,omitempty"`
	// WholeBlocked says the blueprint read judged the whole blueprint and returned fix. Bar is the
	// rendered bar a bar read's blueprint seat gets as its findings, and BarFrom the commit of the
	// blocked read it came from.
	WholeBlocked bool   `json:"whole_blocked,omitempty"`
	Bar          string `json:"bar,omitempty"`
	BarFrom      string `json:"bar_from,omitempty"`
	// ReadScope and ReadChange are the blueprint read's own scope and change block, set when the
	// read judges something other than the draft's Scope and Change. readScope and readChange
	// say which pair a read uses.
	ReadScope  *blueprint.Scope `json:"read_scope,omitempty"`
	ReadChange string           `json:"read_change,omitempty"`
}

// readScope is the scope the blueprint read judges: ReadScope when the page has one, else the
// draft's Scope.
func (w *work) readScope() *blueprint.Scope {
	if w.ReadScope != nil {
		return w.ReadScope
	}
	return w.Scope
}

// readChange is the change block the blueprint read judges, paired with readScope: a page with a
// read scope of its own reads its ReadChange, empty when that scope is not scoped, and never the
// draft's Change.
func (w *work) readChange() string {
	if w.ReadScope != nil {
		return w.ReadChange
	}
	return w.Change
}

// readerState is the final reader's two-turn conversation.
type readerState struct {
	History []batch.Message `json:"history,omitempty"`
	Retest  bool            `json:"retest,omitempty"`
}

func (w *work) marshal() (json.RawMessage, error) {
	return json.Marshal(w)
}

func (w *work) spend(row string) int {
	if w.Budgets == nil {
		w.Budgets = map[string]int{}
	}
	w.Budgets[row]++
	return w.Budgets[row]
}
