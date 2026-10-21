# Introductions

Every page opens with an introduction and ends with the closing section its type calls for. This
page gives the questions you answer before you write an introduction, and where the background a
reader lacks goes. It doesn't give wording to fill in. Write each introduction for the page's own readers.

A blueprint plans its introduction in its `:::reader` block (in an older YAML blueprint,
`framing.introPlan`, which `drafter check` requires). The plan answers to what the page's
sections deliver and to the introduction its anatomy requires. [Stock anatomies](anatomies.md) lists those parts. Each rule
below names its source.

## Start from the reader

Before you plan the introduction, answer three questions about the page's readers. Record the
answers in the `:::reader` block (`framing.readers` in an older YAML blueprint).

- **Who arrives at the page?** Name each role that reads it. Google's technical writing course
  starts here: "Begin by identifying your audience's role(s)."
  Source: [Google Technical Writing One, Audience](https://developers.google.com/tech-writing/one/audience).
- **What do they already know?** A page owes its reader only what they lack for the task. Name
  what each reader already brings, so the page can skip it.
  Source: [Google Technical Writing One, Audience](https://developers.google.com/tech-writing/one/audience).
- **Why are they reading?** Name the goal each reader brings to the page. The introduction starts
  from that goal, and the page's scope follows from it.
  Source: [Google Technical Writing One, Documents](https://developers.google.com/tech-writing/one/documents).

A page can serve more than one reader. When readers
arrive with different goals, the introduction names each goal, so each reader learns early
whether the page serves them.
Source: [Google Technical Writing One, Audience](https://developers.google.com/tech-writing/one/audience)
and [Documents](https://developers.google.com/tech-writing/one/documents).

## Say what the page covers

The introduction says what the page covers, what prior knowledge it expects, and what it doesn't
cover. Name what's out of scope when a reader could reasonably expect the page to cover it. Keep
the introduction short: it frames the page and doesn't try to cover everything.
Source: [Google Technical Writing Two, Organizing large documents](https://developers.google.com/tech-writing/two/large-docs)
and [Google Technical Writing One, Documents](https://developers.google.com/tech-writing/one/documents).

The outline entry's `covers` and `outOfScope` lists are the page's scope. When the blueprint's
`introduction` differs from them, the outline entry wins.

## Say whether the reader is studying or working

A tutorial serves a reader who is studying, and a task guide serves a reader who is working. The
introduction tells the reader which kind of page they're on:

- A reader who is studying needs to see early what they will achieve by the end. A tutorial's
  introduction names the result and what the reader can do once they finish.
  Source: [Diátaxis, Tutorials](https://diataxis.fr/tutorials/).
- A reader who is working already knows what they want to achieve, and needs the problem or task
  named plainly so they can tell at once whether the page solves it. A task guide's introduction
  names the task and, when it helps, when a reader performs it.
  Source: [Diátaxis, How-to guides](https://diataxis.fr/how-to-guides/).
- A reader of a concept page is studying too, but to understand rather than to do. Its
  introduction says what the idea is and why it matters.
  Source: [Diátaxis, Explanation](https://diataxis.fr/explanation/).

Source for the split: [Diátaxis, Tutorials and how-to guides](https://diataxis.fr/tutorials-how-to/).

## Let the anatomy give the parts, never the words

A page type's anatomy gives the introduction's parts and the sections that follow, in order.
Source: [The Good Docs Project templates](https://gitlab.com/tgdp/templates/-/tree/v1.6.0).

The anatomy never supplies the introduction's sentences, even where a template offers typical
wording. Two pages of the same type have different readers who know different things, and a page
owes each reader only what they don't already know, so their introductions differ.
Source: [Google Technical Writing One, Audience](https://developers.google.com/tech-writing/one/audience).

## Assume the reader may arrive from anywhere

A reader can reach the page from a search or a link, with no earlier page behind them. The
introduction establishes enough context that the page stands on its own.
Source: [Every Page is Page One](https://everypageispageone.com/the-book/), section 10, "EPPO
Topics Establish their Context."

When the page expects the reader to have read or done something first, the introduction names it
and links to it.
Source: [Google Technical Writing Two, Organizing large documents](https://developers.google.com/tech-writing/two/large-docs).

## Put background where the reader needs it

Background is what a reader must know to follow the page and may not. Give it only to the reader
who lacks it, and in one of two forms:

- A link to the concept page that explains it, when the background is a whole idea the doc set
  covers elsewhere.
- A line in the section that needs it (a `sentence` or `reason` item in an older YAML blueprint),
  citing the facts it rests on, when a sentence or two is enough.

Don't explain what the page's readers already know.
Source: [Google Technical Writing One, Audience](https://developers.google.com/tech-writing/one/audience).

The background you record while planning stays a planning note about what the page rests on. It
is never drafted as content; the lines above are. The format is in drafter's
[blueprint format](https://github.com/glw907/drafter/blob/main/docs/blueprint-format.md).
