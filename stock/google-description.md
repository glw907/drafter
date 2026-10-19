# The Google standard

drafter's stock prompts write and review pages to the
[Google developer documentation style guide](https://developers.google.com/style). This page
describes that standard, the reader it serves, and where its rules come from. The drafting seat
reads it before it writes a page.

## What the standard is

The Google developer documentation style guide is Google's editorial guide for technical
documentation. It sets voice and tone, grammar, formatting, and word choice for pages that
developers read. It covers the choices a writer makes in every sentence: who the page addresses,
which tense and voice a sentence takes, how a procedure is numbered, and how code appears in
running text.

The guide gives guidelines, not laws. It asks you to follow your project's own style first, then
the guide, and to depart from the guide when a departure makes a page clearer for its readers. A
project that sets its own rules for a page keeps them.

## Who the standard writes for

The guide writes for software developers and other technical practitioners. The reader is
competent and busy. They arrive with a task or a question, often from a search, and they want the
answer without reading the whole page. Write for that reader:

- Address the reader as "you", and tell them what to do in the imperative.
- Put the goal or the condition first, so a reader can skip what doesn't apply to them.
- Use plain words, define each term on first use, and keep one term for one thing.
- Keep a friendly, conversational tone that never wastes the reader's time.

## How drafter uses the standard

The standard reaches the seats as four files that drafter ships in its binary. A prompt names each
one with a `drafter:` token, and drafter inlines the file into the request.

| File | Holds | Read by |
|---|---|---|
| `google-description.md` | This page: the standard, its reader, and its source. | The drafting seat. |
| `google-tells.md` | The patterns the standard rules out, each with a before-and-after pair. | The drafting seat and the editor seat. |
| `google-rubric.md` | Short rules paraphrased from the guide, each linked to the guide's page. | The editor seat, which grades a page against it. |
| `google-corpus.md` | Short excerpts from the guide's own pages, in the guide's voice. | The drafting seat, for voice only. |

None of these files copies the guide. The rubric and the tells paraphrase it in fresh words, and
the corpus quotes a few sentences with attribution. When a rule here and the guide disagree, the
guide governs: follow its link.

## Source and license

The guide is published by Google at <https://developers.google.com/style>. The footer of its pages
reads: "Except as otherwise noted, the content of this page is licensed under the Creative Commons
Attribution 4.0 License, and code samples are licensed under the Apache 2.0 License." The
Creative Commons Attribution 4.0 License is at <https://creativecommons.org/licenses/by/4.0/>.

These files aren't endorsed by Google. Each excerpt in `google-corpus.md` names its source page,
its license, and the date it was retrieved.
