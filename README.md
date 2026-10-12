# drafter

drafter runs documentation drafting and review as batched calls to the Claude API. A planner,
such as a Claude Code session, writes a page plan in drafter's YAML format, and drafter validates
it with `drafter check-plan`. Later releases draft and review each planned page, gate it with the
project's own checks, and record every finding and its resolution. The tool is in early
development.
