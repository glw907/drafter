# Edge cases

## Tags

- `wrapped-tag` The claim, then its source. Source: the key policy. [external: a qualifier that
  wraps onto a second line]
- `code-bracket` A bullet with brackets in code, `keys[0]` and `['a', 'b']`. Source: the key
  policy. [verified]
- `quoted-bracket` A bullet quoting "[...path]" in a catch-all route. Source: the key policy.
  [verified]
- `rejected-fact` A claim that turned out false. Source: the key policy. [rejected: it was a
  guess]
- `cand-excluded` A claim kept out of the pages. Source: the key policy. [candidate: excluded]
- `cand-plain` A claim not yet traced. Source: a page only. [candidate]
- `drift-fact` The code says one thing and a page another. Source: the key policy. [docs-drift:
  page says "two hours"]
- `no-tag` A bullet that forgot its tag. Source: the key policy.
- `bad-tag` A bullet with an unknown tag. Source: the key policy. [maybe: unsure]
- `space-tag` A bullet whose qualifier takes a space. Source: the key policy. [verified checked]
- `two-tags` A bullet with two tags. [verified] [external: both]
- `dup-id` The first bullet with a shared id. Source: the key policy. [verified]
- `dup-id` The second bullet with the shared id. Source: the key policy. [verified]
