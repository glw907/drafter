# Rotate the key

This page shows how to rotate a key without downtime.

## Before you begin

The key lives for `90` days. Read the [key reference](../reference/keys.md#key-lifetime) and the
[service docs](https://example.com/docs) first.

## Rotate the key

Create the new key, then deploy it. The old key keeps working for
**24 hours** after rotation.

```bash
# not a heading
make deploy
```

#### Unplanned detail

The deploy reads the key from the environment.

The health endpoint reports the new key ID.

The old key keeps working until you *revoke* it. Deploying first means a failed deploy leaves
the old key working. When the deploy is done, [revoke the old key](#revoke-the-old-key).

## Revoke the old key

Revoke the old key with the `revoke` command. Revocation cannot be undone. Next, read the key
reference.
