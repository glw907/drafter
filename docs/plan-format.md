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
`use`, `reason`, and each entry of `constraints` and `friction`.

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
the repository root. The `title` field is the page's H1. The `type` field is the page type, as
free text. Write the page type exactly as the project's outline names it: a project might use
`concept`, `task guide`, `symptom row`, or `tutorial milestone`. The command checks only that
`type` is present and not empty.

### The opening

The `introduction` mapping holds the opening. It follows the three parts that Google's developer documentation style
guide asks for in a page's opening. All three are required prose. The `covers` field says what
the page covers. The `priorKnowledge` field says what the reader should already know. The
`notCovered` field says what the page leaves out.

### sections

Each entry in `sections` has a `heading` (the section heading, verbatim), `opens` (the section's
first sentence, as prose), and `items`. A section needs at least one item.

A section can carry a `level`, `2` or `3`, which defaults to `2`. The list stays flat, in page
order: write a level-3 section as the next entry after its parent, with `level: 3`. Any other
value is an error. A same-page `#anchor` can name a level-3 heading, since the link check reads
every section `heading`.

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
| `link` | `target` | A link the section must carry. |
| `handoff` | `text` | The section's closing turn toward what comes next. |
| `reason` | `text` | A "why" that must survive drafting. |
| `step` | `text`, and optionally `requires`, `produces`, `canFail` | One action the reader takes. |
| `failure` | `symptom`, `handledBy`, and optionally `from`, `cause` | A failure that a step can cause, or a free-standing symptom row. |
| `code` | `source`, `text` | A code block the section shows. The `source` is `path` or `path#Symbol`, and `text` says what the block shows. |
| `figure` | `text`, and optionally `source` | A figure the section needs. The `text` says what it must show, and the optional `source` says where it comes from. |

The `target` of a link is one of `path`, `path#anchor`, `#anchor`, or an `http://` or `https://`
URL. A path resolves from the directory of the page, as the page's own link would.

A step's `requires` and `produces` are lists of state tokens, such as `new-key` or `deployed`.
The `canFail` field is `true` or `false`, and it defaults to `false`. A failure's `from` is the
`id` of a step. Its `handledBy` is the `id` of the item that tells the reader what to do. A
failure with no `from` is a free-standing symptom row, for a page that lists symptoms, causes, and
fixes with no steps. The optional `cause` says why the symptom happens.

The path part of a code item's `source` resolves from the directory of the page, as a link target
does, and must exist. The `#Symbol` part isn't checked. A figure's `source` isn't checked.

### What the plan leaves out

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
item's kind, a `level` other than `2` or `3`, and a plain-style prose value with a line
comment. Later checks run only on a plan
that decodes without errors.

### ids

Every `id` is unique. A failure's `from`, when present, names a step. A failure's `handledBy` names an item that
exists. No fact is both a `fact` item and a disposition.

### reader-state

In page order, every step's `requires` tokens must be produced by an earlier step or be listed
in `preconditions`. This check proves the steps are consistent. It doesn't prove they are
complete: a page with no deploy step passes unless some step requires `deployed`. A person or a
reviewing model still judges whether a step is missing.

### recovery

Every step with `canFail: true` must be the `from` of at least one `failure` item. A failure
item whose handler doesn't exist is an `ids` error.

### links

For every `link` item, every subordinated `reference`, and the path part of every `code` item's
`source`:

- An `http://` or `https://` target isn't checked.
- A path must exist under the repository root. A directory passes, but an anchor on a directory
  fails.
- An anchor must match the slug of a heading in the target file. A same-page `#anchor` must
  match the slug of the page's `title` or of a section `heading`.
- A target that the outline lists but that isn't on disk yet is a warning, and its anchor isn't
  checked. Any other missing target is an error.

A path that begins with `/` resolves from the repository root. A path that leaves the repository
is an error.

Slugs follow GitHub's rule as `github-slugger` 2.0.0 implements it. A repeated heading in one
file takes the suffix `-1`, then `-2`. The headings come from a CommonMark parse of the target
file, including setext headings, and never from fenced code. A heading's text keeps the text of
inline code and drops link URLs and other markup. A leading front matter block is ignored. A
heading written as raw HTML isn't an anchor in this version of the check.

In YAML, quote a same-page target such as `"#grace-period"`. Unquoted, the `#` starts a comment.

### facts

This check runs only when you pass `--facts`. Without it, each `fact` item and each `sentence`
item's `facts` list gets a warning that names its line, and no fact is checked.

Every `fact` item's `fact` field, every id in a `sentence` item's `facts` list, and every
disposition's `fact` field must name exactly one bullet in the facts directory. The bullet behind
a `fact` item or a `facts` entry must also be citable.

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

```json
{ "pages": [{ "path": "docs/guides/revoke-a-key.md" }] }
```

## Output and exit codes

`drafter check-plan [--facts <dir>] [--outline <file>] [--root <dir>] [--json] <plan>...` prints
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
