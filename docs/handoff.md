# The handoff

A handoff tells drafter everything about one planned page. Any planner can write one. drafter
decodes every handoff into the same structure, whether it arrives as a handoff file or inside a
planner's run file.

Commit the plan and the framing record before you run drafter. drafter starts only on a clean
worktree.

## The handoff file

A handoff file is a JSON object that describes one page. `drafter page` takes one or more files.

```json
{
  "version": 1,
  "project": "/work/widget",
  "page": {
    "path": "docs/widget/install.md",
    "brief": "docs/internal/briefs/widget/install.json",
    "type": "task-guide"
  },
  "plan": "docs/internal/briefs/widget/install.plan.yaml",
  "framing": "docs/internal/briefs/widget/install.framing.md",
  "inputs": {
    "job": "Show a new user how to install Widget.",
    "pageType": "task guide",
    "registerSections": ["## Task guides"],
    "exemplarSources": ["/work/exemplars/guide.md"],
    "claimInventory": [
      {
        "claim": "Widget installs with one command.",
        "disposition": "carried",
        "factId": "f:abc123",
        "section": "Install"
      }
    ],
    "factIds": ["f:abc123"]
  },
  "context": {
    "args": {},
    "resolved": {"index": "docs/widget/README.md"}
  },
  "checksum": "sha256:..."
}
```

| Field | Holds |
|---|---|
| `version` | The format version. The only version is `1`. |
| `project` | The project root, as an absolute path. drafter loads `drafter.toml` and `drafter.local.toml` from it. |
| `page` | The page's path and its brief's path, both relative to the project, and its type: `task-guide`, `concept`, or `reference`. |
| `plan` | The plan's path, relative to the project. |
| `framing` | The framing record's path, relative to the project. Leave it out when the planner wrote none. |
| `inputs` | The page inputs, with the plan's and the framing's dispositions applied. |
| `context` | Optional. An object that drafter passes to the prompts module unchanged. |
| `checksum` | `sha256:` and the hex SHA-256 of the plan file's bytes followed by the framing file's bytes. |

### The inputs

| Field | Holds |
|---|---|
| `job` | The page's job, in a sentence. |
| `pageType` | Optional. The page type as the planner names it. |
| `registerSections` | The register sections the seats read. |
| `exemplarSources` | The files that serve as exemplars. |
| `claimInventory` | The page's claims. Each entry has a `claim` and a `disposition`, and may have a `factId`, a `reason`, and a `section`. |
| `factIds` | Optional. The IDs of the facts the page cites, for a project with a facts store. |

### The checksum

The checksum pins the plan and the framing the planner checked. A plan edited after the handoff
was written fails the check, and so does one edited between a stop and a resume.

To compute it, run:

```sh
cat docs/internal/briefs/widget/install.plan.yaml \
    docs/internal/briefs/widget/install.framing.md | sha256sum
```

Put `sha256:` before the hex digest. When the page has no framing record, hash the plan alone.

## A planner's run file

A planner that runs as a Claude Code Workflow can skip the handoff file. The Workflow runtime
saves each run's return value in a run file, and `drafter page --workflow-run <run-file>` reads the
handoff from it. No model retypes the handoff, and nothing lands in the worktree.

The run file is Claude Code's own format, so drafter reads only `.result.handoff`. That object
holds a version, the planner's `args`, the `resolved` pages and index, the `worktree` path, and a
`pages` entry for each page with its `page`, `plan`, `framing`, and `inputs`. drafter decodes it
strictly against its own versioned structure, and it refuses an unknown version or field and names
the run file in the error.

drafter converts each entry to a handoff. The worktree becomes `project`. The `args` and the
resolved pages and index become `context`. Because no one typed a checksum, drafter computes it
from the files on disk when it converts and records it, and a resume then checks the plan and the
framing exactly as it does for a handoff file.

drafter refuses a run file that:

- has no `result`, which means the run hasn't finished;
- has a `result.handoff` that is missing or null;
- lists no pages;
- carries a resolved page with `figure: true` or a non-empty `rework`, because drafter doesn't
  draft figures or rework.

## What drafter refuses

drafter decodes each handoff strictly. Every refusal names the file.

- An unknown version or an unknown field.
- A `project` that isn't an absolute path.
- A `plan` or `framing` that is an absolute path. Both must be relative to the project.
- A `page.path`, `page.brief`, `plan`, or `framing` that resolves outside `project`.
- A checksum that doesn't match the plan and framing on disk. The error prints both digests.
- A set of handoff files that name more than one project, because one run works one worktree.

### Paths that must stay inside the project

The paths `page.path`, `page.brief`, `plan`, and `framing` must resolve inside `project`, after
`..` segments are applied. `exemplarSources` is exempt, because exemplars usually live in a
shared directory outside the project.
