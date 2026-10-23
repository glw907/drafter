---
page: Fix a widget
type: symptom row
track: operate
exemplar: guides/widget-troubleshooting
---

# Fix a widget

:::reader
**Who reads it.** An operator who runs a widget farm.

- A widget stopped spinning. :fact[ab12cd]
:::

:::constraints
- Name no widget model. :fact[ef34gh]
:::

## Read the widget log

*Shows the reader where the widget log lives.*

- Every widget writes one log line per spin. :fact[ab12cd]
- The log rotates daily, so yesterday's spins sit in a dated file. :fact[ef34gh] :fact[ab12cd]
- Code: a sample of the log's lines. :source[/src/widget/log.sh#sample] :fact[ij56kl]

```sh
- not a bullet :fact[zz99zz]
## not a heading
```

### `widget.stalled`

*Helps the reader whose widget stopped.*

1. Restart the widget with `widgetctl restart`. :fact[ij56kl]

:::row
**Symptom.** The widget does not spin.
**Cause.** The spindle is dry.
**Fix.** Oil the spindle. :fact[ij56kl]
:::

## See also

- See [Widget reference](../reference/widget.md).
- Figure: the spindle, drawn from above. :source[/docs/img/spindle.svg]
