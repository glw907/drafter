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
| `introPlan` | How the introduction opens and what it promises. Required: `drafter check` reports a blueprint with no `framing`, or with no `introPlan`, as an `error`. |

A part cites a fact as a backticked id. A backticked token counts as a citation when the facts
store holds a bullet with that id, or when it starts with the prefix, through the first colon,
that every id in the store shares. A mistyped id therefore reaches the facts check as a missing
fact. The narration of how the planning session moved against an older plan belongs in the commit
message, not here.

The decoder doesn't require `introPlan`, so a converted blueprint still decodes. `drafter convert`
lists `framing.introPlan` among the choices it leaves to you, and `drafter check` reports it as an
error until you fill it in. For
what a good plan for the introduction answers, see the
[introductions reference](../stock/introductions.md).

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
| `tracks` | list | no | The reader tracks that group the pages. |

Each entry of `types` has a `name` and an `anatomy`. The `anatomy` is the shape the type follows,
and `drafter check` requires one. A page's `type` must be a declared type.

### Anatomies

An anatomy is either a stock anatomy or an entry in a register. The stock anatomies are `Concept
page`, `Task guide`, `Tutorial`, `Reference entry`, and `Troubleshooting page`. The
[anatomies reference](../stock/anatomies.md) describes each one. Any other name must be an entry
in the register of the track that holds the type's pages. An entry is a heading, or a list item
that opens with a bold name, under the register's `## The page anatomies` section. With no
`tracks`, an outline can't name a register, so `drafter check` warns about an anatomy that isn't a
stock name. With `tracks`, it reports the same anatomy as an error.

### tracks

A track groups the pages one kind of reader uses. An outline with no `tracks`, or with `tracks: []`,
skips the track checks, and an anatomy that isn't a stock name warns instead of failing. Each entry of `tracks` has these fields.

| Field | Required | Holds |
|---|---|---|
| `name` | yes | The track's name. Each name appears once. |
| `reader` | yes | The reader profile, in prose: who reads these pages and what they bring. |
| `standard` | yes | The published style standard the track follows. `google` is the only value. |
| `paths` | yes | A list of at least one glob over page paths. |
| `register` | no | The repository path of a register file that adds anatomy entries. |

A glob matches a page path segment by segment, as Go's `path.Match` does, except that a `**`
segment matches zero or more segments. `docs/**` matches every page under `docs`,
and `docs/**/*.md` matches `docs/a.md` and `docs/a/b.md`.

With `tracks` declared, `drafter check` reports these states.

| State | Severity |
|---|---|
| A managed page matches no track, or matches two. | error |
| A track names a `standard` other than `google`. | error |
| A track's `register` file doesn't exist. | error |
| A glob in `paths` is malformed. | error |
| A track's globs match no managed page. | warning |

A missing field, an empty value, `paths: []`, and a repeated track `name` stop the command, as a
malformed outline does. These checks run once for the whole outline, whichever blueprints you name.

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

`drafter seed <blueprint>...` gives an existing page the same entry without a conversion, so the
page's first run keeps its prose. Run `drafter read` on the blueprint first and commit the page and
the blueprint. A blueprint qualifies when its entry holds an `accepted` id equal to the blueprint as
committed at `HEAD`, holds no `page` id, and neither file has uncommitted changes. Seed checks every
named blueprint before it writes anything, and the first one that doesn't qualify stops the call with
the page and the fix. The entry it writes keeps `accepted`.

`drafter page` writes a page's entry when it accepts the page. The entry holds the page as written,
the blueprint as committed at the run's start, and the section hashes of the written page. The
entry's `accepted` id changes only when the page's blueprint read covered the whole blueprint:
then `accepted` becomes that blueprint's id. A scoped or skipped read leaves it alone. See
[What `drafter page` does with a blueprint](#what-drafter-page-does-with-a-blueprint).

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
| `types` | The page's `type` is one `types` declares, and the type's `anatomy` isn't empty. Each declared type's `anatomy` is a stock name or an entry in its track's register. | error, or warning when the outline declares no `tracks` |
| `tracks` | Each managed page matches exactly one track, and each track names a known `standard`, an existing `register`, and well-formed globs. | error, or warning for a track that matches no managed page |
| `framing` | The blueprint has a `framing` with an `introPlan`. | error |
| `lock` | An entry with a `page` id names a page on disk, and an entry with only an `accepted` id names a page the outline lists. | error |
| `convention` | `blueprints.root` is `blueprints`, and `facts.dir` is `<root>/facts`. | warning |
| `handler` | A `failure`'s handler gives the reader a next action. A `handledBy` that names only a `fact` item, a `sentence` with `mode: verbatim`, or a `link` or `handoff` that sends the reader back to the section holding the failure's `from` step warns. The message carries the fix: hand off to the step or section that gives the reader a next action. | warning |
| `store` | Every fact bullet in the facts store has an id no other bullet has, a `Source:`, and one valid tag, and each source pointer names something that exists. See [The facts store](#the-facts-store). | error, or warning for a bare path that doesn't exist |

A prose field that carries a `[[id]]` is `argument`, `orderReason`, the `introduction` parts, the
`framing` parts, a section's `why` and `opens`, an item's `text`, `use`, `symptom`, and `cause`,
and `constraints`. A finding on a framing citation points at the framing field's line. A typo'd id
reports "in no facts file", and an id on two bullets reports "on 2 bullets".

When you name blueprints, `drafter check` checks only those files and the lock entries for their
pages. The pairing of pages with blueprints, the `convention` warnings, and the `store` check come
from a run without names.

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

## The facts store

The facts store is a directory of Markdown files, `blueprints/facts` unless `facts.dir` says
otherwise. Every file in it except `README.md` is a facts file. Each fact is one bullet, and a
blueprint places or cites a fact by its id.

```markdown
- `f:k3x9a2` A key is 32 bytes long. Source: `src/keys.go:12` (`KeyBytes`), `src/keys.go#KeyBytes`. [verified]
```

### The bullet grammar

A bullet starts with `- `, runs over the non-blank lines that follow, and ends at a blank line,
the next bullet, or a `## ` heading.

- The bullet opens with its id as a code span. Any id works, and `f:` plus six base36
  characters, such as `f:k3x9a2`, is the recommended shape. An id appears on one bullet in the
  whole store.
- The claim follows the id.
- `Source:` follows the claim and says where the claim comes from. A pointer is a code span. An
  anchor is a second code span in parentheses right after the pointer, and it quotes or names
  what the pointer should show.
- One status tag ends the bullet, in the colon form `[tag: note]` or bare `[tag]`. The tags are
  `verified`, `docs-drift`, `external`, `vendor`, `candidate`, and `rejected`. A blueprint can
  cite only a `verified`, `external`, or `vendor` fact.
- A bullet under a `## Harvest record` or `## Provenance` heading isn't a fact. It needs no
  source or tag, and it can't carry an id.

### The `Source:` grammar

The pointers are the code spans between `Source:` and the status tag that sit outside
parentheses and brackets and aren't URLs. A span in parentheses is commentary, and so is a span
that isn't path-shaped. A slashless span counts as a path when the file exists at the project
root, or when it carries a `:lines` or `#name` and has a dot.

| Form | Verified as | Fails, as an error, when |
|---|---|---|
| `path:lines` | The path from the project root. A path with no slash that isn't at the root resolves to the one file with that name in the repository. | No file or more than one file matches, the target is a directory, a cited line is outside the file, or an anchor word is absent from the cited lines plus 10 on each side |
| `path#name`, with a path ending `.go`, `.ts`, `.tsx`, `.mts`, `.cts`, `.js`, `.jsx`, `.mjs`, or `.cjs` and a `name` of one identifier | The path and the declaration of `name` in it | The file is missing, no declaration matches, or an anchor word is absent from the declaration |
| `path`, with a slash, or a name that exists at the root | The path exists | Never. A path that doesn't exist is a warning, and its anchor isn't checked |

`lines` is a list such as `12`, `12-20`, or `3,12-20`. A line is in range when it is at most
the number of lines `split("\n")` gives for the file, so a file that ends in a newline has one
more line than `wc -l` counts. An anchor word is a match of `[A-Za-z_][A-Za-z0-9_]{2,}`. A
word is present when the text contains it, even inside a longer word.

The bare path is a warning because it names things that are not always files of the project: a
package, a path inside a dependency, or a path a scaffold makes. Nothing separates those from a
stale path, so the check reports the path and leaves the call to you.

A declaration is one of these.

- In Go, a `func`, method, `type`, `var`, or `const` at column 0, or a name inside a grouped
  `const`, `var`, or `type` block.
- In TypeScript and JavaScript, a `function`, `class`, `interface`, `type`, `enum`, `const`,
  `let`, `var`, `namespace`, or `module` at column 0, with any `export`, `default`, `declare`,
  `abstract`, or `async`, and `declare global` for the name `global`. When no column-0 line
  matches, an indented `const`, `let`, `var`, or `function` counts, such as a `const` inside a
  test callback.

The declaration runs from its line to the line before the next column-0 declaration, or to the
end of the file.

### What verification skips

A pointer is skipped, and counted as skipped, rather than checked in these cases.

- A `#name` with a dot, such as `Class.method`.
- A `#name` on any file that isn't one of the extensions above, such as `.md`, `.json`, or `.svelte`.
- A pointer whose path has a character outside letters, digits, `_`, `.`, `/`, and `-`. Route
  paths such as `src/routes/(site)/[id]/+page.svelte` and `@scope` package paths fall here.

### Where it runs

`drafter check` with no blueprint named verifies the whole store, adds the findings under the
check name `store`, and prints `facts store: <n> pointers checked, <k> skipped` on stderr before
the totals. Each finding is on the facts file and the line of its bullet. A run that names
blueprints doesn't verify the store.

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

## Check what you add

The blueprint read (`blueprintPrompt`) checks each item a change adds or alters against these
lines. Which lines block in a change read, and which come back as "Nit:", is set out in [what the read does](#what-drafter-read-does-with-a-blueprint).

- An `example` appears only for a recorded fact, and it never invents output (drafter's own rule: an `example`'s `facts` names what it rests on, see [Items](#items)). See [what the read does](#what-drafter-read-does-with-a-blueprint).
- A `failure` follows the diagnosis that tells the reader which failure they have, its handler is a next action, and its symptom differs from every existing row (the last clause is drafter's own rule). See [what the read does](#what-drafter-read-does-with-a-blueprint), the Red Hat peer review guide's [Structure checklist](https://redhat-documentation.github.io/peer-review/#_structure) on logical order and troubleshooting steps, and the "Add a failure case" row of [the edit catalog](#the-edit-catalog).
- A `heading` names what the section delivers. See Google's [Headings and titles](https://developers.google.com/style/headings).
- A `use` scopes a placed fact and never restates it. See [what the read does](#what-drafter-read-does-with-a-blueprint) and [Say what a placement carries](#say-what-a-placement-carries).
- In a task section, the steps (the actions the reader takes) come before the explanation. See the Red Hat [Structure checklist](https://redhat-documentation.github.io/peer-review/#_structure) on mixed module types and logical order.
- A new item agrees with its section's facts, and the consistency read reports a conflict only when both sides cannot be true together. A difference of scope is not one. The agreement check is drafter's own code: [`ConsistencyPrompt`](../internal/assemble/consistency.go#L25).

## What `drafter read` does with a blueprint

`drafter read` runs the checks above on the blueprint as committed at the run's start, then sends
one batch holding the consistency read, when the project has a facts store, and the blueprint
read. The blueprint read grades the blueprint as a plan: order, pace, and the introduction, the
completeness of steps and failures, what a reader can know at each step, recovery before and
after diagnosis, a `use` that restates its fact, durable fields that narrate planning history,
and a human-origin item the blueprint would bury. A blocking finding sets the blueprint aside.
When the run ends with every blueprint decided, the accepted blueprints' blob ids go into the
`accepted` field of `drafted.lock`.

With `--since <rev>`, the read judges the change as a code reviewer judges a pull request. It
still reads the whole blueprint, and it also receives the diff of the blueprint, the changed fact
bullets, and the outline entry when it changed. A finding blocks only for a defect the change
adds, alters, or causes. Every other defect is reported as non-blocking, with a finding that
starts "Already present:". The consistency read blocks only on a conflict that touches a fact the
change touched, and reports any other conflict as already present. In a change read, a conflict that names no fact
and no page is recorded as a warning starting "Unplaced:" and never blocks. A blueprint with no earlier
version at `<rev>` is read whole. A change read writes no `drafted.lock` entry when it accepts.

Within the defects a change adds, the change read blocks on a defect that makes the page wrong, or
leaves a reader stuck or without a state a later step needs: a claim past its fact, invented
output, a handler that loops or dead-ends, a state no step supplies, a missing step. Order, pace,
heading form, field placement, and a `use` that restates its fact come back as non-blocking
findings that start "Nit:", and a Nit never makes the verdict "fix". A whole read keeps the
blocking rule above for those.

House register rules apply in neither blueprint read. The page's type sets the introduction and
closing section it requires, and nothing else in the register is graded here; the editor seat
applies the register to prose.

`drafter read` prints each finding under its page with its location and whether it blocks, and
prints the record's warnings and any escalation detail. `--json` carries the same as fields of the
page.

## What `drafter page` does with a blueprint

`drafter page` drafts a page from its blueprint. Name the page (`docs/x.md`) or its blueprint
(`blueprints/docs/x.blueprint.yaml`). An argument that ends in `.blueprint.yaml` is a blueprint,
one that ends in `.md` is a page, and anything else is a [handoff file](handoff.md). Give one kind
of argument, not a mix, and don't combine blueprints with `--workflow-run`.

A run starts on a clean worktree and reads every input from the commit it starts at: the blueprint,
the outline, the facts, the page, and its brief. For each page it first decides what to run, by
comparing the blueprint with the entry in `drafted.lock`.

| The page | The run |
|---|---|
| Has no entry, or an entry with only `accepted`, or no committed page | Drafts the page whole. |
| Matches its entry: blueprint, facts, and outline entry all unchanged | Isn't run. The command prints `up to date with drafted.lock`, spends nothing, and needs no API key, Vale, or `drafter` on `PATH`. |
| Changed in its structure: a section added, removed, or reordered, or a page-level field changed | Drafts the page whole, after a read of the change. |
| Changed in some sections, and has no brief at the start commit | Drafts the page whole, because a section redraft can't cover sentences the brief doesn't list. |
| Changed in some sections, and a changed fact is cited by a sentence in the page's preamble | Drafts the page whole. |
| Changed in some sections | Redrafts those sections. |

A changed fact bullet counts for every section that cites it, in the blueprint or in the page's
brief. The redraft may edit only the changed sections. drafter refuses an edit outside them or on
a heading. The review reads are narrowed to the same sections. A read of the whole page for
coherence follows, and its findings, like a gate fix, can change any section. drafter declines a
finding whose fix lies outside the changed sections, and gives that as the reason.

When the entry's `accepted` id equals the blueprint at the start commit, the blueprint read is
skipped. That happens after a change to a fact alone.

To tell what changed, drafter needs the outline entry and the facts the earlier draft saw. The
lock names no commit. drafter walks the history of `drafted.lock` to the commit that introduced
the page's current entry, and reads the outline entry and the facts from that commit's parent. A
`drafter read` that rewrites only `accepted` doesn't move the commit. The walk stops at a lock it
can't decode.

On the stock prompts module, a page whose outline declares tracks must match exactly one, and the
track must name the `google` standard. A project module is not checked.

### What the run writes

A run writes the page and its brief, and, when it ends with every page decided, the lock. It never
commits. Commit the page, the brief, and `drafted.lock` together, with nothing committed between
them and the run's start. If a fact or outline edit is committed on its own after the run starts,
the next run may not see it. drafter warns and names the files when it finds one.

A run that stopped writes no lock entry, because its pages aren't final. Resume it with
`drafter page --resume <id>` and the same arguments, and the resume writes the entries of the
pages accepted before and after the stop. If you discard a stopped run's pages instead, run again.
If the lock can't be written after the run finished, the command prints the run's summary, then the
error and the command that resumes the run. The resume submits no request and writes the lock.

## The brief

A brief is the file that maps a page's sentences to the facts they rest on. `drafter page` writes
it beside the page's blueprint: `blueprints/docs/guide.brief.json` for `docs/guide.md`, unless
`blueprints.brief_path` names another place (see [Configuration](configuration.md)). Commit it
with the page.

```json
{
  "sentences": [
    {"text": "Rotating a key needs the admin role.", "id": "f:bbbbbb"},
    {"text": "The check returns the id of the key that signed.", "id": ["f:cccccc", "f:aaaaaa"]},
    {"text": "Here is how the rotation works.", "id": "no-claim"}
  ]
}
```

The brief is a JSON object with a `sentences` array, one entry for each sentence of the page's
prose, in page order. Other fields are ignored. Each entry has these fields:

| Field | Value |
|---|---|
| `text` | The sentence as the page has it. Emphasis and code marks don't need to match. |
| `id` | The bare string `no-claim`, one fact id, or a non-empty list of fact ids. |

A fact id is any string without whitespace or a backtick. A list never holds `no-claim`.

### What `drafter gate` checks in a brief

The stock gate runs the provenance check on a page that has a blueprint, after the blueprint
checks. A page with a blueprint and no brief prints `no brief for <page>; provenance check
skipped`. The check reads the brief, the page, the facts store, and the blueprint's `fact` items,
and every defect it finds is an error. It prints each as `file:line: error: provenance: message
(fix)`. A defect in a sentence cites the brief, at the line that holds the sentence, and a defect
in page text cites the page.

The check denies by default. It reports:

- **A shape defect.** The brief isn't valid JSON, isn't an object, or has no `sentences` array.
  The check stops there.
- **An unclassified sentence.** The `text` is empty, the `id` is missing or isn't a string or a
  list of strings, the list is empty, a list holds `no-claim`, or an id has whitespace or a
  backtick.
- **An id that doesn't resolve.** Each id must name exactly one bullet in the facts store, and the
  bullet's tag must be `verified`, `external`, or `vendor`. A `candidate`, `rejected`, or
  `docs-drift` bullet, or a bullet whose tag doesn't parse, can't be cited. When the project sets
  no `facts.dir`, every cited id is a defect.
- **A token no cited fact holds.** The check pulls these from a cited sentence: dates, versions,
  numerals, paths and URLs, commands, flags, and code names. Each must appear in at least one cited
  bullet. A numeral or version must appear in the bullet's claim, the text before its `Source:`
  part. Any other token may also appear in the bullet's source pointers.
- **A `no-claim` sentence that states a token.** A sentence that names a command or a number cites
  the fact that states it.
- **A sentence that isn't on the page, or page text that isn't a sentence.** The page's prose is
  the page less its front matter, fenced code, HTML comments, `details` and `summary` tags,
  images, and heading lines. Each brief sentence must occur in it, and nothing but Markdown
  punctuation may be left over. An empty `sentences` array on a page with prose reports the prose
  as unclassified page text.
- **A fact item no sentence cites.** Each `fact` item of the blueprint must be cited by at least
  one sentence. `drafter page` checks that the sentence sits in the item's section.

A command is a code span that begins with a known command-line tool and has at least one more word.
The known tools are `git`, `go`, `make`, `npm`, `npx`, `pnpm`, `yarn`, `node`, `curl`, `docker`,
`python`, `pip`, and `cargo`, and the first word of every code span of two or more words in a fact's
claim. A project's own tool is read as a command once a fact states it.

When the check passes, the gate prints `provenance: <page>: N sentences (cited X, no-claim Y)`,
after the other output. The gate holds that line back when the gate fails, so a failure in the
store alone isn't read as a defect in the page.

The check reads tokens, not meaning. A sentence that contradicts its fact with the fact's own
tokens passes, and so do a spelled-out number, a unit, a short flag, a bare filename in prose, and
a snippet in a code span. A reviewer who reads the brief beside the page owns those.
