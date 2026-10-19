# Patterns the Google standard rules out

Each section on this page names a pattern that the
[Google developer documentation style guide](https://developers.google.com/style) rules out, and
gives one sentence before and after the fix. The examples are written for this page; none is taken
from the guide. The drafting seat avoids these patterns, and the editor seat reports each one it
finds by the section's heading.

## Passive voice that hides the actor

A passive sentence can leave the reader unsure who acts: the reader, the tool, or a server. Make
the actor the subject.

Before: The configuration file is read and the cache is cleared.

After: The tool reads the configuration file and clears the cache.

## Future tense for present behavior

A tool's ordinary behavior happens now, every time it runs. Use the present tense, and keep
"will" for something that happens later.

Before: When you run the command, it will print the version number.

After: When you run the command, it prints the version number.

## The writer in the first person

"We" and "let's" put the writer in the reader's task. Address the reader as "you", or give the
instruction in the imperative.

Before: Let's create a project directory before we install the package.

After: Before you install the package, create a project directory.

## The instruction before its condition

A reader who meets the instruction first acts before learning whether it applies. Put the goal or
the condition first.

Before: Run the export command if you want to keep a copy of the report.

After: To keep a copy of the report, run the export command.

## Words that date the page

"Now", "new", "currently", and "latest" anchor a page to the day it was written, and they turn
false with the next release. Describe the product as it is.

Before: The tool now supports a new JSON output format.

After: The tool can print its output as JSON.

## Filler that claims the task is easy

"Simply", "just", and "easily" add no information, and they talk down to a reader who finds the
step hard. Cut them.

Before: Simply run the installer, and the setup is easily done.

After: Run the installer.

## Polite padding in an instruction

"Please" and "kindly" slow an instruction down without making it clearer. Give the instruction
directly.

Before: Please make sure to save your changes before you close the editor.

After: Save your changes before you close the editor.

## Link text that says nothing

"Click here" and "this link" don't tell the reader where a link goes, and they read badly out of
context. Make the link text say what the reader finds.

Before: `To read about the flags, click [here](flags.md).`

After: `For a list of every flag, see [Command flags](flags.md).`

## A heading built on an -ing verb

A task heading starts with a bare verb, the same form as the instruction it heads. A gerund reads
as a topic, not a task.

Before: `## Configuring the output directory`

After: `## Configure the output directory`

## A procedure introduced by a fragment

A procedure opens with a complete sentence that gives the reader context, not a fragment that the
numbered steps complete.

Before: To reset the counter:

After: To reset the counter, follow these steps:

## An abbreviation with no definition

A reader who doesn't know an abbreviation can't look it up in the sentence. Spell it out on first
use, and use the short form after that.

Before: Set the TTL for each cache entry.

After: Set the time to live (TTL) for each cache entry.
