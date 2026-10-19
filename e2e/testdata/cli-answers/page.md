# Count items with tally

Tally counts the items you name.

## Before you begin

Tally keeps its count in a file named .tally in the current directory.

Run every tally command in the same directory.

To start from zero, run `tally reset` first.

The command `tally reset` deletes the count file.

## Add items

Pick the items you want to count.

Run `tally count` with the items you want to add.

The command `tally count` adds each item you name on the command line to the total.

The command `tally count` creates the .tally file the first time it runs.
## Read the total

Run `tally total` in the same directory.

The command `tally total` prints the saved total.

The command `tally total` prints 0 when the .tally file does not exist.

If `tally total` prints 0 although you counted items, you are in the wrong directory.

Run `tally total` in the directory where you ran `tally count`.

The file is named .tally.

## See also

For the rest of tally, see [the tally readme](../README.md).
