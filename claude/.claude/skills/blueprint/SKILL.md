---
name: blueprint
description: Change a documentation page by changing its blueprint. Use when code changes documented behavior, when you notice a docs deficiency while reading (a wrong claim, a missing example, caveat, failure case, or link), when a new docs page is needed, when a repo has a `blueprints/` directory, or when you run `drafter check` or `drafter refs`.
---

# Change a page through its blueprint

A blueprint is the durable plan for one docs page. In a repo with a `blueprints/` root, you change the blueprint and never the page prose. The nightly drafter run turns the blueprint change into prose. The [format page](format.md) is the one home of every format rule; this skill says when to act and links the rule.

## Find the blueprint

The blueprint of `docs/a/b.md` is `blueprints/docs/a/b.blueprint.yaml`. See [Layout](format.md#layout). Read the blueprint whole before you edit it. Write prose fields the way [Write prose fields as blocks](format.md#write-prose-fields-as-blocks) says.

## Pick the case

1. **A code change alters documented behavior.** Update the fact bullet for the behavior. Run `drafter refs <fact-id>` before you correct a fact: it lists every item on every page that places or cites it. Fix each placement the new fact makes wrong. Run `drafter check`, then commit the facts, the blueprints, and the code together. The change is not done until the blueprints match.
2. **You notice a deficiency while reading.** Fix the blueprint, not the page. Choose the row of [the edit catalog](format.md#the-edit-catalog) that matches what you want to do, and make the change it names. Use the item kinds in [Items](format.md#items).
3. **A page is missing.** Follow [new-page.md](new-page.md).

If no catalog row fits, stop and say so in your report. Do not invent an edit.

## Edit

- Item fields and what each kind requires: [Items](format.md#items).
- Place a fact without restating it: [Say what a placement carries](format.md#say-what-a-placement-carries).
- Point at another section: [Refer to a section by id](format.md#refer-to-a-section-by-id).
- Fields only drafter or a person sets, which you never write: [Who sets what](format.md#who-sets-what).
- Fields that sit outside `sections`: [Top-level fields](format.md#top-level-fields).
- Adding or moving a section redrafts the whole page; the catalog row says so. Prefer an item edit when one does the job.

## Check

Run `drafter check` after every blueprint edit and fix each error before you commit. [What `drafter check` checks](format.md#what-drafter-check-checks) lists the checks, the severities, and the exit codes. To check one file, name it: `drafter check <blueprint>`.

When a fact changes, `drafter refs` is the lookup: [Find where a fact is used](format.md#find-where-a-fact-is-used).

## Page prose is off limits

Never edit a managed page's prose. Your own edit would count as a human edit and the nightly run would have to write it back.

One exception applies. When the project's gate fails your code change because a page still names something the change renamed or removed, make only the page edits the gate demands, and change the blueprint to match, in the same pull request. The last row of [the edit catalog](format.md#the-edit-catalog) is the rule.

## Related

- The root and outline file: [outline.yaml](format.md#outlineyaml).
- Do not edit `drafted.lock`: [drafted.lock](format.md#draftedlock).
