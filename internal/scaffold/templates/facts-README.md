# Facts

This directory is the project's facts store. Every other Markdown file here holds facts, one
bullet each, and a blueprint places or cites a fact by its id.

```markdown
- `f:k3x9a2` A key is 32 bytes long. Source: `src/keys.go:12` (`KeyBytes`). [verified]
```

- The bullet opens with its id as a code span: `f:` plus six base36 characters.
- The claim follows the id.
- `Source:` follows the claim. A pointer is a code span, one of `path:lines`, `path#name`, or a
  bare `path`. A second code span in parentheses right after the pointer is the anchor: a word the
  cited lines should show.
- One tag ends the bullet, as `[tag]` or `[tag: note]`. The tags are `verified`, `docs-drift`,
  `external`, `vendor`, `candidate`, and `rejected`.
- Put harvest notes under a `## Harvest record` heading and file history under `## Provenance`.
  Bullets there are not facts.

The whole grammar is in the
[blueprint format](https://github.com/glw907/drafter/blob/main/docs/blueprint-format.md#the-facts-store).
