---
name: blueprint
description: Change a documentation page by changing its blueprint. Use when code changes documented behavior, when you notice a docs deficiency while reading (a wrong claim, a missing example, caveat, failure case, or link), when a new docs page is needed, when a repo has a `blueprints/` directory, or when you run `drafter check` or `drafter refs`.
---

# Change a page through its blueprint

A blueprint is the durable plan for one docs page. In a repo with a `blueprints/` root, you change the blueprint and never the page prose. The nightly drafter run turns the blueprint change into prose. The [format page](format.md) is the one home of every format rule; this skill says when to act and links the rule.

## Find the blueprint

Find the page's blueprint by the path rule in [Layout](format.md#layout). Read the blueprint whole before you edit it. Write prose fields the way [Write prose fields as blocks](format.md#write-prose-fields-as-blocks) says.

## Pick the case

1. **A code change alters documented behavior.** Update the fact bullet for the behavior. Run `drafter refs <fact-id>` before you correct a fact: it lists every item on every page that places or cites it. Fix each placement the new fact makes wrong. Run `drafter check`, then commit the facts, the blueprints, and the code together. The change is not done until the blueprints match.
2. **You notice a deficiency while reading.** Fix the blueprint, not the page. Choose the row of [the edit catalog](format.md#the-edit-catalog) that matches what you want to do, and make the change it names. Use the item kinds in [Items](format.md#items). When the deficiency is a wrong fact, run `drafter refs <fact-id>` before you correct it, then fix each placement the corrected fact makes wrong. A `failure` item is for a symptom whose handler the facts supply: a step, or a handoff that gives the reader a next action, citing the fact (see the `handler` row in [What `drafter check` checks](format.md#what-drafter-check-checks)). When no fact gives the reader a route, use the "Add a caveat" row of [the edit catalog](format.md#the-edit-catalog) and say in the report that the case has no handler.
3. **A page is missing.** Follow [new-page.md](new-page.md).

If no catalog row fits, stop and say so in your report. Do not invent an edit.

When a request conflicts with a fact, or asks for content a section already carries, make the smaller edit the catalog allows. Say in the report what was asked and why the edit is smaller, naming the fact id or section id.

## Edit

- Item fields and what each kind requires: [Items](format.md#items).
- Place a fact without restating it: [Say what a placement carries](format.md#say-what-a-placement-carries).
- Point at another section: [Refer to a section by id](format.md#refer-to-a-section-by-id).
- Fields only drafter or a person sets, which you never write: [Who sets what](format.md#who-sets-what).
- Fields that sit outside `sections`: [Top-level fields](format.md#top-level-fields).
- Before you add or move a section, read its row in [the edit catalog](format.md#the-edit-catalog). Prefer an item edit when one does the job.

## Check

For each item the edit adds or changes, walk [Check what you add](format.md#check-what-you-add) before `drafter check` and the commit. Run `drafter check` after every blueprint edit and fix each error before you commit. [What `drafter check` checks](format.md#what-drafter-check-checks) lists the checks, the severities, and the exit codes. To check one file, name it: `drafter check <blueprint>`. A run that names a blueprint skips the facts store, so its `store` warnings are not yours to chase.

When a fact changes, `drafter refs` is the lookup: [Find where a fact is used](format.md#find-where-a-fact-is-used).

## Read

Before the first edit, note `git rev-parse HEAD` as the base. Every read uses that base. Run `drafter check <blueprint>`, commit, then read as a separate command: `drafter read --sync --since <base> <blueprint>`. A read of an uncommitted tree prints "nothing changed" and judges nothing. The read judges the change as a code reviewer judges a pull request; [What `drafter read` does with a blueprint](format.md#what-drafter-read-does-with-a-blueprint) says what blocks and what returns as a "Nit:". It prints each finding with its location and whether it blocks.

Fix every blocking finding, and any Nits you choose, in one batch. Run `drafter check <blueprint>`, commit, and read again. Polish goes before a read, never after an accepted one: once a read accepts, make no further edit, Nits included, because an edit after the last read is a change no read saw. Stop at an accept or after the third read. Findings that start "Already present:" are older defects and stay out of the edit.

| The read ends with | Do |
|---|---|
| Exit 0, no blocking finding printed | Accepted. Stop. |
| Exit 0 with a blocking finding printed, or exit 1 with the blueprint read's findings | Fix and read again. |
| Exit 1 from a consistency escalation | Fix the conflict the change caused and read again. |
| Exit 1 escalated on rule `cap`, or exit 3 naming `cycle cap` or `spend limit` | Stop reading, commit, and report the stop to the user. |
| Any other exit 3 | Run `drafter read --sync --resume <run-id> <blueprint>` once. It counts as the same read. |
| Exit 2 naming the key | Skip the reads and say so in the report. |
| Exit 2 for a dirty tree, an unresolved `--since`, or config | Fix the cause and rerun. It isn't a read. |

When blocking findings remain after the third read, add one empty commit (`git commit --allow-empty`) whose message lists each by location, and list them to the user in the final report.

## Page prose is off limits

Never edit a managed page's prose. Your own edit would count as a human edit and the nightly run would have to write it back.

One exception applies. When the project's gate fails your code change because a page still names something the change renamed or removed, make only the page edits the gate demands, and change the blueprint to match, in the same pull request. The last row of [the edit catalog](format.md#the-edit-catalog) is the rule.

## Related

- The root and outline file: [outline.yaml](format.md#outlineyaml).
- Do not edit `drafted.lock`: [drafted.lock](format.md#draftedlock).
