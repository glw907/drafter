package e2e

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// tallyMarkdownBlueprint is the fixture page's blueprint in the Markdown format.
const tallyMarkdownBlueprint = `---
page: Count items with tally
type: task guide
track: cli-docs
---

# Count items with tally

:::reader
**Who reads it.** A developer with tally installed who wants a running count.
:::

## Before you begin

*Tells the reader where tally keeps the count.*

- Tally keeps its count in a file named .tally in the current directory. :fact[tally-state-file]
- Run every tally command in the same directory. :fact[tally-state-file]

## Add items

*Shows the reader how to add items.*

1. Run ` + "`tally count`" + ` with the items you want to add. :fact[tally-count-cmd]

## Read the total

*Shows the reader how to read the total.*

- The command ` + "`tally total`" + ` prints the saved total. :fact[tally-total-cmd]
`

// TestDefaultPageDraftsFromAMarkdownBlueprint names the page, finds its .blueprint.md, and sends
// the draft and the edit with the blueprint's text verbatim. It holds once the blueprint check
// decodes Markdown blueprints; until then the check reads the file as YAML and the page escalates,
// which this test reports as a skip.
func TestDefaultPageDraftsFromAMarkdownBlueprint(t *testing.T) {
	e := newDraftEnv(t)
	const md = "blueprints/docs/count-items.blueprint.md"
	e.write(md, tallyMarkdownBlueprint)
	e.commit("a Markdown blueprint", md)
	r := e.pageDefault(draftPage)
	if r.code != 0 {
		t.Fatalf("exit %d, sent %v\n%s%s", r.code, e.stub.sent(), r.stdout, r.stderr)
	}
	acceptedPage(t, e.summary(r))
	if got, want := e.stub.sent(), []string{"draft", "edit"}; !slices.Equal(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	quoted, err := json.Marshal(tallyMarkdownBlueprint)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Trim(string(quoted), `"`)
	for i, body := range e.stub.sentBodies() {
		if !strings.Contains(body, text) {
			t.Errorf("request %d lacks the Markdown blueprint's text verbatim", i)
		}
	}
}
