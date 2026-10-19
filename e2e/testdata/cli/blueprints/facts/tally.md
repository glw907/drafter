# Tally facts

- `tally-state-file` Tally keeps its count in a file named .tally in the current directory. Source: `main.go:12-13` (`stateFile`). [verified]
- `tally-count-cmd` The command `tally count` adds each item you name on the command line to the total. Source: `main.go:33-39` (`runCount`). [verified]
- `tally-by-flag` The flag --by sets the amount each item adds, and the default is 1. Source: `main.go:33-36` (`by`). [verified]
- `tally-total-cmd` The command `tally total` prints the saved total. Source: `main.go:23-24` (`readTotal`). [verified]
- `tally-reset-cmd` The command `tally reset` deletes the count file. Source: `main.go:25-26` (`stateFile`). [verified]
- `tally-count-args` The command `tally count` counts the arguments left after it reads its flags, and it never reads their names. Source: `main.go:36-38` (`NArg`). [verified]
- `tally-count-creates` The command `tally count` creates the .tally file the first time it runs. Source: `main.go:39` (`WriteFile`). [verified]
- `tally-count-quiet` The command `tally count` prints nothing. Source: `main.go:33-40` (`runCount`). [verified]
- `tally-by-int` The flag --by takes a whole number. Source: `main.go:36` (`Int`). [verified]
- `tally-total-empty` The command `tally total` prints 0 when the .tally file does not exist. Source: `main.go:42-48` (`readTotal`). [verified]
