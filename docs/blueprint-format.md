# Blueprint format

A blueprint is the durable plan for one documentation page. It holds everything the page must say
and why, as one YAML file, `<page>.blueprint.yaml`, kept apart from the page. Agents change the
blueprint, never the page prose, and drafter turns the change into prose.

A blueprint starts from the [plan format](plan-format.md) and changes it for a file that lives for
years and is edited in small steps. This page is the format's one home. The JSON Schemas are
[`schema/blueprint.schema.json`](../schema/blueprint.schema.json) and
[`schema/outline.schema.json`](../schema/outline.schema.json). They describe the shape of each
file, and `drafter check` checks what a schema can't.

## Version and stability

Every blueprint and every outline starts with `version: 1`. A later release of the format may add
optional fields. It never removes a field or changes what one means.

## Layout

All blueprints live under one root in the repository, `blueprints/` by default. A page's blueprint
sits at the page's path under that root, with `.md` replaced by `.blueprint.yaml`.

```
README.md
docs/extend/rotate-the-key.md
blueprints/
  outline.yaml
  facts/
  drafted.lock
  README.blueprint.yaml
  docs/extend/rotate-the-key.blueprint.yaml
```

One rule takes a page to its blueprint and back, with no setting and no search: the blueprint of
`docs/extend/rotate-the-key.md` is `blueprints/docs/extend/rotate-the-key.blueprint.yaml`, and the
blueprint of the root `README.md` is `blueprints/README.blueprint.yaml`. The `.blueprint.yaml`
suffix lets an editor attach the schema by glob and lets a tool tell a blueprint from
`outline.yaml`. The facts store stays shared, in `blueprints/facts/` by convention, because many
pages cite one fact.

The settings `blueprints.root`, `blueprints.outline`, and `facts.dir` move these paths for a
project that is mid-migration, and `drafter check` warns while a project departs from the
convention. [Configuration](configuration.md) lists them.

## Write prose fields as blocks

A prose field holds a sentence or more. Write it as a `|` block or inside quotes. A plain YAML
value ends at the first ` #`, so a plain-style prose value that carries a line comment is an
error. A plain value can't start with `[[` either, because YAML reads it as a nested list:

```yaml
why: [[verify]] carries the checks
```

That line is a YAML parse error, and `why: [[verify]]` alone decodes as a list. The decoder
reports a prose field that decodes as a list and tells you to quote the value or use a block:

```yaml
why: |
  [[verify]] carries the checks.
```

## Top-level fields

| Field | Type | Required | Holds |
|---|---|---|---|
| `version` | integer | yes | The format version, `1`. |
| `argument` | string | yes | The page's argument, in prose. |
| `introduction` | mapping | yes | The three parts of the page's opening, as in the plan format. |
| `framing` | mapping | no | The framing record's four descriptive parts. |
| `orderReason` | string | yes | Why the sections run in this order, as a whole. |
| `preconditions` | list of strings | no | State tokens the reader brings to the page. |
| `exemplars` | list | no | Published pages to imitate. |
| `sections` | list | yes | The page's sections, in page order. At least one. |
| `dispositions` | list | no | Facts the blueprint keeps out of the page body. |
| `constraints` | list of strings | no | Rules specific to this page. |
| `review` | string | no | `human` for a page that must never merge unread. |

The page's path, title, type, and job aren't in the blueprint. They live in the page's entry in
`outline.yaml` and in the path rule, so no fact has two homes. A blueprint with a `page` block, a
`friction` field, or a `registerSections` field is an error. The friction a pass meets goes in the
project's friction log. The register sections come from the project's `register.<track>`
settings.

The `introduction` mapping holds `covers`, `priorKnowledge`, `notCovered`, and an optional list
of `facts`, as in the plan format. `dispositions` and `constraints` work as they do there, except
that `constraints` holds only rules specific to the page and never a copy of a register rule.
Each disposition has a `fact`, a `disposition` of `cut` or `subordinated`, a `reason`, and, for
`subordinated`, a `reference` to the link target that now carries the fact.

An `orderReason` explains the order of the sections as a whole. The reason for each section's
place goes in that section's `why`.

### framing

The `framing` mapping carries the framing record's four descriptive parts. Each is a prose field.

| Field | Holds |
|---|---|
| `readers` | Who arrives, from where, and why. |
| `background` | The background the page rests on. |
| `placeInDocSet` | Where the page sits in the doc set. |
| `introPlan` | How the introduction opens and what it promises. |

A part cites a fact as a backticked id. A backticked token counts as a citation when the facts
store holds a bullet with that id, or when it starts with the prefix, through the first colon,
that every id in the store shares. A mistyped id therefore reaches the facts check as a missing
fact. The narration of how the planning session moved against an older plan belongs in the commit
message, not here.

### exemplars

Each entry names a published page to imitate. The `source` and `take` fields are required. The
`slice` field names the exemplar corpus, and `manifestType` gives the exemplar's type in its
manifest. The `take` field says what to take from the page and what to leave.

### sections

| Field | Type | Required | Holds |
|---|---|---|---|
| `id` | string | yes | A stable id. Headings change; section-scoped work keys on the id. |
| `heading` | string | yes | The section's heading, verbatim. |
| `level` | integer | no | `2` or `3`. The default is `2`. |
| `anchor` | string | no | The heading's slug, when the blueprint pins it. |
| `why` | string | by check | Why the section is here and sits where it does. |
| `opens` | string | yes | The section's first sentence. |
| `items` | list | yes | The section's items. Empty only above a level-3 section. |
| `styleNotes` | list | no | Style-only human edits, each with `old` and `new` wording. |

The decoder doesn't require `why`, so a converted blueprint decodes. `drafter check` reports a
section with no `why` as an error. Section ids and item ids share one namespace, unique across
the blueprint, and follow the item id pattern: lowercase letters, digits, `-`, and `_`, starting
with a letter or digit.

## Items

Each item has an `id` and a `kind`, and the kind decides which other fields it takes. Any item
may also carry `origin`, whose only value is `human`. The kinds are those of the plan format,
plus `example`.

| Kind | Required | Optional |
|---|---|---|
| `fact` | `fact` | `use` |
| `sentence` | `mode`, `text` | `facts` |
| `example` | `text` | `facts` |
| `link` | `target` | `text` |
| `handoff` | `text` | |
| `reason` | `text` | `facts` |
| `step` | `text` | `requires`, `produces`, `canFail`, `optional` |
| `failure` | `symptom`, `handledBy` | `from`, `cause`, `facts` |
| `code` | `text` | `source`, `facts` |
| `figure` | `text` | `source` |

The plan format's [item reference](plan-format.md#items) describes the kinds it shares. An
`example` item is a worked example in prose: its `text` says what the example shows, and its
optional `facts` list names the facts it rests on. The `mode` of a `sentence` is `verbatim` or
`claim`.

### Say what a placement carries

A `fact` item places a fact. Its `use`, when present, says which part of the fact this placement
carries, and it starts with `Only` or `Omit`. It never restates the fact, because drafter puts
the fact's own text in front of the drafter, and a restatement goes stale when the fact is
corrected. Leave `use` out to place the whole fact.

```yaml
- id: needs-admin
  kind: fact
  fact: "f:abc123"
  use: |
    Only the role requirement.
```

### Refer to a section by id

A prose field names another section as `[[<section-id>]]`, never by its heading, so a heading
rename breaks nothing. `drafter check` resolves each reference against the blueprint's section
ids and reports an unknown id. Quote the value or use a `|` block, because a plain value can't
start with `[[`.

### Who sets what

Some fields are privileged. A person or agent edits a blueprint's content, and drafter alone
writes the marks that record a human edit.

| Field | Set by | Meaning |
|---|---|---|
| `origin: human` | drafter's write-back | The item carries a person's edit and is graded as a checklist. It clears when a redraft carrying it is accepted. |
| `styleNotes` | drafter's write-back | The old and new wording of a style-only edit. It stays until the register's owner rules on it. |
| `review: human` | a person | The page never merges in an autonomous run. |
| `managed: false` | a person, in `outline.yaml` | The page has no blueprint and drafter never drafts it. |
| every `drafted.lock` field | drafter | What drafter last wrote and what the blueprint read last accepted. |

## outline.yaml

`outline.yaml` sits at the top of the blueprint root and lists every page of the doc set.

| Field | Type | Required | Holds |
|---|---|---|---|
| `version` | integer | yes | `1`. |
| `types` | list | yes | The page types the doc set declares. |
| `pages` | list | yes | Every page, in order. |
| `crossLinks` | list | no | Links one page should carry to another. |

Each entry of `types` has a `name` and an `anatomy`. The `anatomy` names the register entry the
type follows, such as `Task guide`, and `drafter check` requires one. A page's `type` must be a
declared type.

Each entry of `pages` has these fields.

| Field | Holds |
|---|---|
| `path` | The page's path from the repository root. Always required. |
| `slug` | The short name that `crossLinks` and other tools use. |
| `title` | The page's H1. |
| `type` | A declared type. |
| `job` | What the reader accomplishes with the page. |
| `covers` | What the page covers. |
| `outOfScope` | What the page leaves to other pages. |
| `figure` | `true` for a page that carries a figure. Defaults to `false`. |
| `managed` | `false` for a page drafter never drafts. Defaults to `true`. |

A managed page needs `slug`, `title`, `type`, and `job`. A page with `managed: false` needs only
`path`, which covers a historical record whose text must not change. Each entry of `crossLinks`
has `from`, `to`, and `why`, naming pages by slug or path.

## drafted.lock

`drafted.lock` records what drafter last wrote, in the spirit of a package lockfile. Only drafter
writes it. It sits beside `outline.yaml`, apart from the blueprints, so a redraft never changes a
blueprint.

```yaml
version: 1
pages:
  docs/extend/rotate-the-key.md:
    page: 6f1ed002ab5595859014ebf0951522d9a1b2c3d4
    blueprint: 8d4c0f9b1a2e3d4c5b6a79881726354433221100
    accepted: 8d4c0f9b1a2e3d4c5b6a79881726354433221100
    sections:
      _intro: sha256:3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1b
      rotate: sha256:5feceb66ffc86f38d952786c6d696c79c2dbc239dd4e91b46729d73a27fb57e9
```

Each entry is keyed on the page path and holds these fields.

| Field | Holds |
|---|---|
| `page` | The git blob id of the page at drafter's last write. |
| `blueprint` | The git blob id of the blueprint at drafter's last write. |
| `accepted` | The blob id of the blueprint that the blueprint read last accepted. |
| `sections` | One hash per section id, and `_intro` for the text above the first section. |

Hashes follow three rules.

- A page or blueprint hash is the file's git blob id, so `git cat-file blob <id>` retrieves the
  text drafter last wrote. The lock has no commit field, because an entry committed with its page
  can't name that commit.
- A section hash is `sha256:` and the hex digest of the section's text. The text is the region
  below the section's heading up to the next section's heading, so a level-3 child's text is the
  child's own. A section with no matching heading on the page has no key, and a missing key reads
  as changed.
- The entries come out sorted by page path, so a diff of the lock stays small.

An entry with only `accepted` set belongs to a page that isn't drafted yet. A missing
`drafted.lock` is valid and reads as no entries.

`drafter convert` seeds a page's entry with the page as it stands, at no cost, and records the
blueprint it wrote. The reshape that follows is a blueprint change, so a converted page gets
redrafted once, the first time drafting runs on it.

## What `drafter check` checks

`drafter check [--root dir] [--json] [<blueprint>...]` validates every blueprint under the
blueprint root, or the ones you name, with no network call. It prints each finding as
`file:line: severity: check: message (fix)`. The exit status is 0 when no finding is an error, 1
when any is, and 2 when the command can't run. It reports every file's findings and continues
past a blueprint that fails to decode. `--json` prints the findings as one array.

The command can't run, and exits 2, in these states:

- The blueprint root doesn't exist.
- `outline.yaml` doesn't exist or doesn't decode.
- `drafted.lock` exists but doesn't parse. A missing lock is valid.
- The facts directory is set but holds no facts file.
- A file you name isn't a `.blueprint.yaml` file at its path-rule location.

The checks that run on each blueprint use the same logic as `drafter check-plan`, applied to the
blueprint's view: the blueprint joined to its outline entry, with each framing citation counted
as an example item.

| Check | What it requires | Severity |
|---|---|---|
| `decode` | The file decodes against the schema, and the ids are well formed and unique. | error |
| `ids` | Every `from` and `handledBy` names an item, and no fact is both placed and disposed. | error |
| `reader-state` | A step requires only state that an earlier step or `preconditions` produces. | error, or warning for state only optional steps produce |
| `recovery` | Every step marked `canFail` has a `failure` item that names it in `from`. | error, or warning for a failure after a step that can't fail |
| `links` | Link targets and code sources exist, anchors match a heading, and each cross-link in `outline.yaml` has a `link` item. | error, or warning for a page that's planned but not written |
| `facts` | Every fact a blueprint places or cites is in exactly one facts file and is tagged `verified`, `external`, or `vendor`. Framing citations count. | error, or warning when no facts directory is set |
| `blueprints` | Every managed page in `outline.yaml` has one blueprint, and every blueprint is at a path-rule location and names a managed page the outline lists. | error, or warning for a managed page with no blueprint until `blueprints.required` is `true` |
| `refs` | Every `[[id]]` in a prose field names a section id. | error |
| `why` | Every section has a non-empty `why`. | error |
| `use` | A `use` that's present starts with `Only` or `Omit`. | error |
| `types` | The page's `type` is one `types` declares, and the type's `anatomy` isn't empty. | error |
| `lock` | An entry with a `page` id names a page on disk, and an entry with only an `accepted` id names a page the outline lists. | error |
| `convention` | `blueprints.root` is `blueprints`, and `facts.dir` is `<root>/facts`. | warning |

A prose field that carries a `[[id]]` is `argument`, `orderReason`, the `introduction` parts, the
`framing` parts, a section's `why` and `opens`, an item's `text`, `use`, `symptom`, and `cause`,
and `constraints`. A finding on a framing citation points at the framing field's line. A typo'd id
reports "in no facts file", and an id on two bullets reports "on 2 bullets".

When you name blueprints, `drafter check` checks only those files and the lock entries for their
pages. The pairing of pages with blueprints and the `convention` warnings come from a run without
names.

### Find where a fact is used

`drafter refs <fact-id> [--root dir] [--json]` lists every blueprint item, on every page, that
places or cites the fact. It prints one `file:line: item-id (kind)` per line. It covers `fact`
items, every `facts` list, the introduction's facts, dispositions, and framing citations.

- A framing citation has the item id `_framing-<part>` and the kind `example`.
- The introduction's `facts` list has the item id `introduction` and the kind `introduction`.
- A disposition has the item id `dispositions` and the kind `disposition`, at the line of its
  `fact`.

The exit status is 0 when the command lists at least one place and 1 when it lists none. The
message for none says whether the facts store holds the id, which tells a mistyped id from a fact
no page uses yet. The status is 2 when the command can't run, for the same reasons as `drafter
check`.

## The edit catalog

The catalog lists the edits a live agent makes most often and the blueprint change each takes.
Every row maps to the item kinds above, and a test holds the format to it.

| The agent wants to | It changes | Item kinds |
|---|---|---|
| Record changed behavior | The fact bullet, then each placement `drafter refs` lists whose `use` no longer fits. | `fact` |
| Add an example | An `example` item, or a `code` item for a code block, in the section. | `example`, `code` |
| Correct a claim | The fact behind it, or the `sentence` item when the claim is an inference. | `sentence` |
| Add a failure case | A `failure` item with its handler, and `canFail` on the step that causes it. | `failure`, `step`, `handoff` |
| Add a caveat | A `sentence` item with `mode: claim`, citing the facts it rests on. | `sentence` |
| Add a link | A `link` item in the section that needs it. | `link` |
| Remove a claim | The item goes, or a `dispositions` entry with `cut` and a reason. | any |
| Add or move a section | The section list, which redrafts the whole page. | |
| Start a new page | An `outline.yaml` entry and a whole blueprint. | |
| Follow a rename the gate demands | The fact bullets, the blueprint, and the page edit the gate names. | |

The last row is the one prose exception. A project's gate can fail a code change whose pages
still name something the change renamed or removed. In that pull request an agent may make only
the page edits the gate demands, and changes the blueprint to match.
