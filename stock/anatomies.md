# Stock anatomies

An anatomy is the shape a page type follows: its sections in order, what its introduction
covers, and how it closes. Each type in `outline.yaml` names one in its `anatomy` field. drafter
ships the five anatomies on this page, and a type can name any of them with no other setup.

Each anatomy paraphrases one template from [The Good Docs Project](https://thegooddocsproject.dev/)
and links to it. The templates come from release v1.6.0. A template gives the sections and their
order. It never supplies the wording of a page's introduction. Write each introduction for the
page's own readers, as [Introductions](introductions.md) describes.

The blueprint read checks a blueprint's introduction and closing section against its anatomy. The
other parts of an anatomy guide the plan and the prose, and no read returns a finding under them.

A section marked optional appears only when the page has material for it. Heading text is yours to
write; the names below describe each section's job, not its heading.

In a blueprint, a figure or a code sample is a `Figure:` or `Code:` bullet in the reader block or
the section it supports, as [the blueprint format](../docs/blueprint-format.md#markdown-blueprints)
describes.

## Concept page

A concept page explains an idea so the reader understands it: what it is, why it matters, and how
it relates to what they already know. It serves a reader who is studying, not one in the middle of
a task. Template:
[Concept](https://gitlab.com/tgdp/templates/-/blob/v1.6.0/concept/template_concept.md) (v1.6.0).

The introduction comes first and has no heading of its own. It has these parts:

1. A summary that names the concept, says why it matters to the reader, and says what the page
   covers.
2. A definition of the concept. When the page relies on more terms, define each one here as a list
   item.
3. Optionally, a figure that shows the concept, with a title that says what it shows.

The sections follow in this order:

1. Background (optional): the history or context that explains why the concept works the way it
   does.
2. Use cases: the situations where the concept applies and what the reader gains from it.
3. Comparison (optional): how the concept compares with alternatives, often as a table.

Closing section (optional): related resources. It links to the task guides, other concepts, and
outside pages a reader goes to next.

## Task guide

A task guide walks a reader through one task they already want to do, as a numbered procedure. It
serves a reader who is working: they know their goal and need the steps. Template:
[How-to](https://gitlab.com/tgdp/templates/-/blob/v1.6.0/how-to/template_how-to.md) (v1.6.0).

The introduction is an overview with these parts:

1. What task the guide covers, in one sentence.
2. Optionally, when and why a reader performs the task.

The sections follow in this order:

1. Prerequisites (optional): what the reader must have or have done before the first step.
2. The task: its numbered steps, each starting with a verb. A step can add a short explanation, a
   code sample or screenshot, and the result the reader sees. A step with parts uses substeps.
3. Subtasks (optional): a large task splits into subtasks, each with its own steps.

Closing section: see also. It links to related task guides, concepts, reference entries, and
troubleshooting.

## Tutorial

A tutorial teaches by doing: the reader completes a guided exercise and learns along the way. It
serves a reader who is studying, and it leads them to a working result. Template:
[Tutorial](https://gitlab.com/tgdp/templates/-/blob/v1.6.0/tutorial/template_tutorial.md) (v1.6.0).

The introduction is an overview with these parts:

1. What the reader builds or does in the tutorial.
2. Who the tutorial is for, and the concepts it assumes they already know.
3. What the reader can do once they finish, as a list of learning objectives.

The sections follow in this order:

1. Background (optional): what the product or feature is and why a reader uses it.
2. Prerequisites: the knowledge, software, environment, and access the reader needs first.
3. The tasks: one section per task, each opening with what the reader does first, then numbered
   steps that start with a verb. A step can add an explanation, a code sample or screenshot, and
   the result the reader sees.
4. Summary: what the reader learned, as a short list.

Closing section: next steps. It links to related tutorials, task guides, or other pages a reader
can take on next.

## Reference entry

A reference entry describes something exactly, so a reader can look up one detail: settings,
fields, commands, or other structured facts. A reader consults it while they work; it doesn't
teach or walk through a task. Template:
[Reference](https://gitlab.com/tgdp/templates/-/blob/v1.6.0/reference/template_reference.md)
(v1.6.0).

The introduction is a description with these parts:

1. What the entry describes and its scope.
2. How it relates to other pages, when it complements one, such as the task guide it supports.

The sections follow in this order:

1. The structured entries: tables, lists, or other consistent formats, in the same order as the
   thing they describe. A table names each item, describes it, and gives an example where one
   helps.
2. Commands (optional): each command's name, what it does, its arguments and whether each is
   required, and an example.

Closing section: none. The reader came for one detail, so the entry ends with its last item.

## Troubleshooting page

A troubleshooting page helps a reader who hit a problem find its cause and fix it. It's organized
by symptom. The symptom is what the reader sees, and it's all they know on arrival. Template:
[Troubleshooting](https://gitlab.com/tgdp/templates/-/blob/v1.6.0/troubleshooting/template_troubleshooting.md)
(v1.6.0).

The introduction is one or two sentences that give the page's scope: the whole product, or one
feature or task.

The sections follow in this order, one group per symptom:

1. The symptom: what the reader sees, such as an error message's text or a slow response.
2. For each cause of the symptom, the cause, then the solution or workaround, then what the reader
   sees once it works.

Closing section: more information. It links to other pages that can help a reader solve the
problem.
