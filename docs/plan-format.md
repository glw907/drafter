# Plan format

A page plan is one YAML file, `<page>.plan.yaml`, that tells a drafter what a documentation page
must say and in what order. The `drafter check-plan` command validates the file offline and
reports each problem with its line number and a fix.

The JSON Schema for the format is [`schema/plan.schema.json`](../schema/plan.schema.json). The
schema describes the file's shape. The command also checks what a schema can't: that ids are
unique, that steps run in a consistent order, and that links and facts exist.

## Version and stability

Every plan starts with `version: 1`. A later release of the format may add optional fields. It
never removes a field or changes what one means, so a plan you write today stays valid.

## Write prose fields as blocks

Several fields hold prose, meaning a sentence or more. Write each prose field as a `|` block or
inside quotes. In YAML, a plain value ends at the first ` #`, so this line loses its second half
without any error from the YAML parser:

```yaml
text: Push the new key first # then delete the old one
```

The value reads as `Push the new key first`. `drafter check-plan` refuses a plain-style prose
value that carries a line comment, and tells you to use a block or quotes. Write the value like
this instead:

```yaml
text: |
  Push the new key first # then delete the old one
```

A `|` block keeps the text exactly, including any `#`. The prose fields are `argument`,
`covers`, `priorKnowledge`, `notCovered`, `orderReason`, `opens`, `text`, `symptom`, `cause`,
`use`, `reason`, the page `title`, each section `heading`, and each entry of `constraints` and
`friction`.

## Top-level fields

| Field | Type | Required | Holds |
|---|---|---|---|
| `version` | integer | yes | The format version, `1`. |
| `page` | mapping | yes | The page the plan is for. |
| `argument` | string | yes | The page's argument, in prose. |
| `introduction` | mapping | yes | The three parts of the page's opening. |
| `orderReason` | string | yes | Why the sections run in this order. |
| `preconditions` | list of strings | no | State tokens the reader brings to the page. |
| `sections` | list | yes | The page's sections, in page order. At least one. |
| `dispositions` | list | no | Facts the plan keeps out of the page body. |
| `constraints` | list of strings | no | Drafting constraints. |
| `friction` | list of strings | no | Places where the format got in the planner's way. |

A field that isn't in the format is an error, and so is a repeated key, an empty file, and a
second YAML document in the same file.

### page

The `page` mapping has three required fields. The `path` field is the page's path relative to
the repository root, so a path that is absolute or climbs out of the root with `..` is an error.
The `title` field is the page's H1. The `type` field is the page type, as free text. Write the page type exactly as the project's outline names it: a project might use
`concept`, `task guide`, `symptom row`, or `tutorial milestone`. The command checks only that
`type` is present and not empty.

### The opening

The `introduction` mapping holds the opening. It follows the three parts that Google's developer documentation style
guide asks for in a page's opening. All three are required prose. The `covers` field says what
the page covers. The `priorKnowledge` field says what the reader should already know. The
`notCovered` field says what the page leaves out.

The opening can also carry `facts`, an optional list of fact ids that back it. The ids cite
facts and don't place them, the same as a sentence's `facts`.

### sections

Each entry in `sections` has a `heading` (the section heading, verbatim), `opens` (the section's
first sentence, as prose), and `items`. A section needs at least one item, with one exception: a
level-2 section whose next entry is a level-3 section may leave `items` empty. The heading then
stands alone above its subsections. Any other empty `items` is an error.

A section can carry a `level`, `2` or `3`, which defaults to `2`. The list stays flat, in page
order: write a level-3 section as the next entry after its parent, with `level: 3`. Any other
value is an error. A same-page `#anchor` can name a level-3 heading, since the link check reads
every section `heading`.

A section can also carry an `anchor`, a plain string. Set it when other pages or the plan's own
links depend on the heading's anchor. The check compares it with the slug of the `heading`, taken
the same way as for a same-page `#anchor`, and reports a mismatch as an error that names both
values. A heading that repeats an earlier one takes the `-1` suffix, so its `anchor` has to carry
the suffix too.

Within a section, list the items in the order the drafted prose will present them.

### preconditions

The `preconditions` list holds the state tokens the reader brings to the page, and a "Before you
begin" section describes the same state in prose. Anything the reader must do there is a `step`
that produces its token, not a precondition.

### dispositions

Each entry in `dispositions` records a fact that the page body doesn't carry. The `fact` field is
a fact id. The `disposition` field is `subordinated` or `cut`. A subordinated fact moves to
another page, so it needs a `reference`, the link target that now carries it. Every entry has a
`reason`, as prose.

A fact is either placed in a section as a `fact` item or listed here, never both.

## Items

Every item has an `id` and a `kind`. The `id` is a lowercase token (letters, digits, `-`, and
`_`) and is unique across the whole plan. The `kind` decides which other fields the item takes.
An item can't carry a field that its kind doesn't list.

| Kind | Fields | Pins down |
|---|---|---|
| `fact` | `fact`, and optionally `use` | A fact id placed in the section. The item id is separate, so one fact can sit in two sections. The `use` field says which claim in the fact this placement backs. |
| `sentence` | `mode`, `text`, and optionally `facts` | A load-bearing sentence. The `mode` is `verbatim` or `claim`. The `facts` field lists the fact ids the sentence rests on. |
| `link` | `target`, and optionally `text` | A link the section must carry. The `text` field says what the link text should read. |
| `handoff` | `text` | The section's closing turn toward what comes next. |
| `reason` | `text`, and optionally `facts` | A "why" that must survive drafting. The `facts` field lists the fact ids it rests on. |
| `step` | `text`, and optionally `requires`, `produces`, `canFail`, `optional` | One action the reader takes. An `optional` step is one the reader may skip. |
| `failure` | `symptom`, `handledBy`, and optionally `from`, `cause`, `facts` | A failure that a step can cause, or a free-standing symptom row. The `facts` field lists the fact ids it rests on. |
| `code` | `text`, and optionally `source`, `facts` | A code block the section shows. The `text` says what the block shows. The `source` is `path`, `path#Symbol`, or an `http://` or `https://` URL. Omit it for an illustrative snippet with no model file. |
| `figure` | `text`, and optionally `source` | A figure the section needs. The `text` says what it must show, and the optional `source` says where it comes from. |

The `target` of a link is one of `path`, `path#anchor`, `#anchor`, or an `http://` or `https://`
URL. A path resolves from the directory of the page, as the page's own link would.

A step's `requires` and `produces` are lists of state tokens, such as `new-key` or `deployed`.
The `canFail` field is `true` or `false`, and it defaults to `false`. The `optional` field works
the same way. A failure's `from` is the `id` of a step. Its `handledBy` is the `id` of the item
that tells the reader what to do. A failure with no `from` is a free-standing symptom row, for a
page that lists symptoms, causes, and fixes with no steps. The optional `cause` says why the
symptom happens. The optional `facts` list names the fact ids the symptom and its cause rest on.

The path part of a code item's `source` resolves from the directory of the page, as a link target
does, and must exist. The `#Symbol` part isn't checked, and neither is a URL. A figure's `source`
isn't checked.

### Using the kinds

Each kind has a job. These conventions keep plans consistent from one page to the next.

**Order.** List the items in the order the page presents them. A step's expected result is a
`sentence` item right after the step. The fact behind a step is a `fact` item right after it. The
fact behind a `reason` is a `fact` item beside it, and the `reason` can also list the id in its
`facts`.

**Facts.** A `fact` item places a fact. A `facts` list cites one, on a sentence, a code item, a
reason, a failure, or the introduction. A citation and a placement are separate, so a citation needs no item
of its own, and a fact can be placed anywhere on the page: a sentence can cite a fact placed in a
later section. A plan can't cite a fact that its `dispositions` list leaves out of the page.

A `use` that restricts a placement starts with "only" or "omit", so a reader can tell it from a
description of the fact:

```yaml
use: |
  Only the overlap window: both keys sign during it.
```

A `use` that takes only the correct half of a partly stale fact is intended. File the stale half
as documentation drift as well. Quote a fact id in YAML when it holds a colon, as in `"f:abc123"`.

An inference that follows from a fact but isn't the fact is a `claim` sentence whose `facts` lists
what it follows from. A fact whose home is another page is `subordinated` to that page, not
`cut`. When that page is planned but not written, list it in an outline, and the link is only a
warning until the page exists.

**Failures.** A failure renders where its item sits. Its `from` only links it to a step for the
reader-state walk, so a closing "Resolve" section made of failures is valid. A failure with two
causes is two failures. A failure whose state comes from a step on a route the reader may have
skipped is free-standing, and its `cause` names the state. The same goes for a failure caused by
state the reader lacks, such as a missing role. An ordered diagnostic list is a run of
free-standing failures in page order, after a `verbatim` sentence that tells the reader to work
through them in order.

A `handledBy` can name any kind of item, anywhere on the page, before or after the failure. A
handler on another page is a `link` item.

**Handoffs and links.** A `handoff` says what the closing turn must do, and the drafter writes
the sentence. A closing sentence that must be copied word for word is a `verbatim` sentence. A
pointer to another page is a `link` item. Links that the introduction needs go in the first
section. A section that collects further reading is a normal section of links, and the drafter
drops any link the body already carries.

**Tokens.** When two routes satisfy one requirement, give each route's last step the same
`produces` token, and have the later step require that token. When a step needs many tokens, have
the last step of the earlier block produce one summary token, such as `access-configured`, and
require that. A token names a state, never an action.

**Opens and the opening.** A fact that backs a section's `opens` sentence is a `fact` item in that
section, and its `use` names the opening. A fact that backs the introduction goes in the
introduction's `facts`. A figure for the introduction sits in the first section. Omit a figure's
`source` when the figure has no source token, since an empty value is refused.

**Claims with no fact.** A claim the page needs but can't back yet goes in `friction`, the list a
run surfaces to whoever owns the facts. A claim from outside the repository needs a fact. File a
vendor behavior as a `[vendor]` fact before the plan places it, and until then note the gap in
`friction` and give the reader a `link` to the vendor's page.

### What the plan leaves out

The format has no kind for a table or a form. Describe a list or table in an item's `use` or
`text`, or in a constraint. A negative, such as "this page doesn't say X", is a constraint. The
plan has no field for rework input from an earlier draft.

The plan carries no exemplars and no cross-links from the outline. The chain supplies both from
the page inputs, so don't copy them into `constraints`.

### Example

```yaml
version: 1
page:
  path: docs/guides/rotate-the-api-key.md
  title: Rotate the API key
  type: task-guide
argument: |
  Rotate a key by publishing the new key before you retire the old one.
introduction:
  covers: |
    How to rotate an API key without interrupting publishing.
  priorKnowledge: |
    You can publish a site.
  notCovered: |
    Revoking a key after the grace period.
orderReason: |
  The new key must be live before the old key retires, so creation comes first.
preconditions:
  - key-exists
sections:
  - heading: Rotate the key
    opens: |
      Create the new key, then publish with it.
    items:
      - id: s-create
        kind: step
        text: |
          Create a new key.
        requires: [key-exists]
        produces: [new-key]
      - id: s-publish
        kind: step
        text: |
          Publish the site with the new key.
        requires: [new-key]
        produces: [published]
        canFail: true
      - id: x-publish
        kind: failure
        from: s-publish
        symptom: |
          The publish fails while the health check still passes.
        handledBy: r-publish
      - id: r-publish
        kind: reason
        text: |
          A passing health check shows only that the service is up, so read the publish log.
      - id: f-overlap
        kind: fact
        fact: key-overlap
        use: |
          Only the overlap window: both keys sign during it.
dispositions:
  - fact: key-grace
    disposition: subordinated
    reference: ../reference/keys.md#grace-period
    reason: |
      The reference page owns the grace period.
```

## What `check-plan` checks

Each finding names one of these checks.

### decode

The decode check reads the YAML and refuses anything outside the format: malformed YAML, an empty
file, a second document, an unknown or repeated field, a missing required field, a value of the
wrong type, an unknown `kind`, `mode`, or `disposition`, a field that doesn't belong to the
item's kind, a `level` other than `2` or `3`, an empty `items` list that the level rule doesn't
allow, a `page.path` that is absolute or climbs out of the root, and a plain-style prose value
with a line comment. Later checks run only on a plan
that decodes without errors.

### ids

Every `id` is unique. A failure's `from`, when present, names a step. A failure's `handledBy` names an item that
exists. No fact is both a `fact` item and a disposition, and no `facts` list, on a sentence, a code
item, a reason, a failure, or the introduction, names a fact that a disposition leaves out. This check runs
with or without `--facts`.

### reader-state

In page order, every step's `requires` tokens must be produced by an earlier step or be listed
in `preconditions`. This check proves the steps are consistent. It doesn't prove they are
complete: a page with no deploy step passes unless some step requires `deployed`. A person or a
reviewing model still judges whether a step is missing.

A step that is `optional` produces its tokens only for readers who take it. When a step that isn't
optional requires a token that only optional steps produce earlier on the page, and no
precondition supplies it, the check gives a warning. Make a required step produce the token, or
list it in `preconditions`.

### recovery

Every step with `canFail: true` must be the `from` of at least one `failure` item. A failure
item whose handler doesn't exist is an `ids` error. A failure whose `from` names a step that
isn't `canFail` gets a warning, since the step is not expected to fail: mark the step `canFail`, or
drop `from` to make the failure free-standing.

### links

For every `link` item, every subordinated `reference`, and the path part of every `code` item's
`source`:

- An `http://` or `https://` target isn't checked. A `code` item with an `http://` or `https://`
  `source`, or with no `source`, isn't checked either.
- A path must exist under the repository root. A directory passes, but an anchor on a directory
  fails.
- An anchor must match the slug of a heading in the target file. A same-page `#anchor` must
  match the slug of the page's `title` or of a section `heading`.
- A target that the outline lists but that isn't on disk yet is a warning, and its anchor isn't
  checked. Any other missing target is an error.

A target of just `#` names nothing and is an error.

A path that begins with `/` resolves from the repository root. A path that leaves the repository
is an error.

Slugs follow GitHub's rule as `github-slugger` 2.0.0 implements it. A repeated heading in one
file takes the suffix `-1`, then `-2`. The headings come from a CommonMark parse of the target
file, including setext headings, and never from fenced code. A heading's text keeps the text of
inline code and drops link URLs and other markup. A leading front matter block is ignored. A
heading written as raw HTML isn't an anchor in this version of the check.

In YAML, quote a same-page target such as `"#grace-period"`. Unquoted, the `#` starts a comment.

A section's `anchor`, when present, must equal the slug of its `heading`, computed in page order
with the same repeated-heading suffixes. A mismatch is an error that names both values.

### facts

This check runs only when you pass `--facts`. Without it, each `fact` item and each `facts` list
(on a sentence, a code item, a reason, a failure, or the introduction) gets a warning that names its line,
and no fact is checked.

Every `fact` item's `fact` field, every id in any `facts` list, and every disposition's `fact`
field must name exactly one bullet in the facts directory. The bullet behind a `fact` item or a
`facts` entry must also be citable.

## Facts files

The facts directory holds Markdown files. Every `.md` file except `README.md` is a facts file.
Within a file:

- A bullet starts with `- ` and continues over the following non-blank lines. Drafter joins the
  lines with single spaces, so a bullet can wrap.
- A bullet starts with its id in a code span, such as `` `signing-key-ttl` ``.
- A bullet ends with one status tag in square brackets: `[verified]`, `[external]`, `[vendor]`,
  `[candidate]`, `[rejected]`, or `[docs-drift]`. A tag can carry a reason after a colon, as in
  `[rejected: the draft was withdrawn]`. The tag is the last thing on the bullet.
- Brackets inside code spans and quoted spans aren't tags.
- Bullets under a `## Harvest record` or `## Provenance` heading aren't facts.

A page can cite a fact tagged `verified`, `external`, or `vendor`. It can't cite a `candidate`,
`rejected`, or `docs-drift` fact, a bullet with no tag, or a bullet with a tag outside the
vocabulary.

## Outline file

The `--outline` file is JSON with a `pages` array. Each entry is an object with a `path`, relative
to the repository root. Drafter reads nothing else from the file. A page listed there is a page
that is planned, so a link to it isn't an error before the page exists.

Pass `--outline` more than once to read several files. A page counts as planned when any of them
lists it, and a file that can't be loaded is reported by its path.

```json
{ "pages": [{ "path": "docs/guides/revoke-a-key.md" }] }
```

## Output and exit codes

`drafter check-plan [--facts <dir>] [--outline <file>]... [--root <dir>] [--json] <plan>...` prints
each finding to standard output as `file:line: severity: check: message (fix)`, and a count to
standard error. With `--json`, it prints the same findings as one JSON array. Each object has
the fields `file`, `line`, `severity`, `check`, `message`, and `fix`.

The `--root` flag defaults to the top level of the git work tree that holds the first plan.

| State | Report | Exit |
|---|---|---|
| No findings | The count. | 0 |
| Warnings only | Each warning and the count. | 0 |
| Any error | Every finding and the count. | 1 |
| Empty file, malformed YAML, or a second document | A `decode` finding at its line. | 1 |
| Several plans | Every plan is checked and the count covers all of them. The exit is the worst result. | 0 or 1 |
| No plan argument or an unknown flag | The usage text. | 2 |
| A plan file that is missing or unreadable | The path and the error. | 2 |
| A `--facts` directory that is missing or holds no facts file | The path and the error. | 2 |
| An `--outline` file that is missing, isn't JSON, or has no `pages` array | The path and the error. | 2 |
| No `--root` and a plan outside a git work tree | The error. | 2 |
