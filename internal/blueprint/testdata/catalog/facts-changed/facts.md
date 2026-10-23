# Facts

- `f:aaaaaa` A widget service holds more than one key at once, and a new key does not invalidate the old. Source: https://example.com/docs/keys, read 2026-10-01. [external: Example]
- `f:bbbbbb` Rotating a key needs the admin role, and the role comes from the owner. Source: `src/rotate.go`. [verified]
- `f:cccccc` The signing check returns the id of the key that signed. Source: `src/sign.go`. [verified]
- `f:dddddd` A key is a 32-byte value. Source: `src/keys.go`. [verified]
- `f:eeeeee` A deleted key stops signing at once. Source: `src/keys.go`. [verified]
