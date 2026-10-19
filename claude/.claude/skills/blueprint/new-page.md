# Add a new page

Use this procedure when a docs page does not exist yet. You write two things: the page's entry in `outline.yaml` and its blueprint. You write no page prose, and you do not run a drafting gate. The nightly run drafts the page. Read the [format page](format.md) sections linked below before you write.

## 1. Settle the page's job and type

Write down the job: what the reader accomplishes with the page. Pick its page type from the types `outline.yaml` declares. Read the anatomy the type names in [anatomies.md](anatomies.md) for its introduction and ending, or, when the outline's track for the page names a register, that register's entry for the anatomy. The entry's fields and what each requires are in [outline.yaml](format.md#outlineyaml).

Set `covers` and `outOfScope` from the neighbors: read the outline whole, so you know each page's job and what another page owns. Keep out of this page whatever a neighbor owns.

## 2. Trace each claim to a source

Every command, step, transcript, figure, warning, success signal, and assertion the page makes is a claim. Give each claim one disposition:

- **Carried.** A fact bullet in the facts store already covers it exactly. Cite the fact id.
- **Filed.** You traced it to code, config, or a vendor doc and filed a new fact bullet with a valid tag and its source; see [What `drafter check` checks](format.md#what-drafter-check-checks). Never file a fact tagged `candidate` or `docs-drift`.
- **Cut.** The claim does not survive the page. Record the reason in a `dispositions` entry.
- **Re-pointed.** Another page owns it. Link to that page.

A fact whose only source is another docs page gets retraced to code, config, or a vendor doc first. When that fails, do not cite it. Edit fact bullets with the Edit tool, never a shell append. `drafter check` reports a fact with no valid tag; see [What `drafter check` checks](format.md#what-drafter-check-checks).

## 3. Write the plan into the blueprint

Argue the order of the sections. Google's outline guidance is the source: structure the page so it introduces information when it is most relevant to the reader. Do not copy the order of `covers`; that list is an inventory.

Fill every required field that [Top-level fields](format.md#top-level-fields) and [sections](format.md#sections) list:

- The `orderReason` explains the order as a whole.
- A section's `opens` is the one sentence a reader takes from it, decided now. The ending section the page type requires is one of the sections.
- A placement for every fact id, a `dispositions` entry for every fact the page does not carry, and a link item to the page or entry that states a subordinated fact. Open that target and confirm it states the fact. When it does not, say so in your report.

Use the item kinds in [Items](format.md#items). Place facts as [Say what a placement carries](format.md#say-what-a-placement-carries) says.

## 4. Frame the introduction

Decide the introduction's framing from the reader before any prose exists. Introductions follow no fixed pattern; reason this page's framing fresh, never from another page's record. Read [introductions.md](introductions.md), and the introduction rules of the register only when the page's track names one. Then fill the `framing` mapping, whose four parts are in [framing](format.md#framing):

1. **Readers.** Work out who arrives, from where (which pages link here, which index group lists it, a search, a command's output), what each is looking for, what each knows, and what each lacks. Name each reader. When a reader is likely to arrive in the wrong place, name them and the page they belong on.
2. **Background.** Name the general model the page sits in, where each outside system fits when the page touches it, and why the thing the page covers exists. Back every point with a fact id. A point you cannot back stays out of the record, and you say so in your report.
3. **Place in the doc set.** The page's group and neighbors, what links in and why, what sibling pages own. Word a shared idea fresh instead of repeating a neighbor's opening.
4. **Intro plan.** The introduction as two or three paragraphs. It opens on a statement about the subject from the reader's situation, never an imperative and never a sentence about the page. It names each reader's reason, so every reader learns early whether the page answers it. It keeps the three introduction parts and, on a task guide, the one-line contract that names what the reader accomplishes. Name the fact ids each paragraph draws on.

Cite a fact in the framing as [framing](format.md#framing) describes. Put narration of how your planning moved in the commit message, never in the blueprint.

## 5. Check and commit

Run `drafter check`. Fix every error: [What `drafter check` checks](format.md#what-drafter-check-checks) lists them. Commit the outline entry, the blueprint, and any new fact bullets together. The nightly run drafts the page from there.
