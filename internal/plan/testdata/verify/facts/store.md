# Facts

These bullets exercise the verifier. Each id names its case.

## Fails

- `f:moved1` The anchor sits far from the cited line. Source: `src/long.go:3` (`farWord`). [verified]
- `f:missng` The anchor is nowhere in the file. Source: `src/long.go:3` (`nowhereWord`). [verified]
- `f:dupe01` The first bullet of a repeated id. Source: `src/long.go:3`. [verified]
- `f:dupe01` The second bullet of a repeated id. Source: `src/long.go:3`. [verified]
- `f:over01` The cited line is two past the last newline. Source: `src/long.go:32`. [verified]
- `f:nopath` The path does not exist. Source: `src/gone.go:3`. [verified]
- `f:ambig1` The base name is in several directories. Source: `same.go:1`. [verified]
- `f:nosrc1` The bullet lacks a source. [verified]
- `f:badtag` The tag is outside the vocabulary. Source: `src/long.go:3`. [maybe]
- `f:nodecl` That Go declaration is missing. Source: `src/doc.go#Missing`. [verified]
- `f:tsout1` The anchor is in another function. Source: `src/ts/lib.ts#exported` (`tokenInOtherFunction`). [verified]
- `f:goout1` The anchor is in another Go function. Source: `src/doc.go#Stop` (`job`). [verified]
- `f:backwd` The range runs backward. Source: `src/long.go:9-5`. [verified]
- `f:dirptr` A line pointer names a directory. Source: `src/pkg:1`. [verified]
- `f:badfile` The line pointer is past the end of a short file. Source: `src/notes.md:9-10`. [verified]
- A claim without an id. Source: `src/long.go:3`. [verified]

## Passes

- `f:edge01` The line after the trailing newline counts. Source: `src/long.go:31`. [verified]
- `f:substr` An anchor word inside a longer word. Source: `src/long.go:3` (`Widget`). [verified]
- `f:near01` An anchor near the cited lines. Source: `src/long.go:3` (`nearWord`). [verified]
- `f:uniqbn` A slashless pointer reaches the one file with that name. Source: `solo.go:1`. [verified]
- `f:gometh` A Go method. Source: `src/doc.go#Run` (`job`). [verified]
- `f:gogrp1` A name in a grouped const block. Source: `src/doc.go#ModeSlow` (`slow`). [verified]
- `f:tsfunc` An exported TypeScript function. Source: `src/ts/lib.ts#exported` (`trim`). [verified]
- `f:tsind1` An indented const inside a callback. Source: `src/ts/lib.ts#cases` (`cases`). [verified]
- `f:tsglob` A declare global block. Source: `src/ts/lib.ts#global` (`appState`). [verified]
- `f:bareok` A bare directory path passes, and a URL and a command are not pointers. Source: `src/pkg/`, https://example.com/page, run `go test ./...`. [verified]
- `f:multi1` Several pointers in one bullet. Source: `src/long.go:3,12-13` (`nearWord`), `src/doc.go#Stop`. [verified]
- `f:parens1` A path in parentheses is commentary. Source: `src/long.go:3` (see also `src/gone.go:3`). [verified]

## Skipped

- `f:dotted` A dotted symbol. Source: `src/ts/lib.ts#Window.appState`. [verified]
- `f:mdhead` A heading pointer into Markdown. Source: `src/notes.md#intro`. [verified]
- `f:route1` A route path with brackets. Source: `src/routes/[token]/+page.svelte:1-15`. [verified]

## Warns

- `f:barewn` A bare path that does not exist. Source: `src/gone/`. [verified]

## Provenance

- `f:prov01` A provenance bullet may not carry an id.
- A provenance bullet without an id is fine.

## Harvest record

- A harvest bullet without an id is fine.
- `harvested-page.md` opens with a file name, which is no id.
