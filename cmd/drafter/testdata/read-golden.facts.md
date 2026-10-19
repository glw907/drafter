# Tally facts

- `tally-state-file` Tally keeps its count in a file named .tally in the current directory. Source: `main.go:12-13` (`stateFile`). [verified]
- `tally-count-cmd` The command `tally count` adds one for each item you name on the command line. Source: `main.go:33-39` (`runCount`). [verified]
- `tally-by-flag` The flag --by sets the amount each item adds, and the default is 1. Source: `main.go:33-36` (`by`). [verified]
- `tally-total-cmd` The command `tally total` prints the saved total. Source: `main.go:23-24` (`readTotal`). [verified]
- `tally-reset-cmd` The command `tally reset` deletes the count file. Source: `main.go:25-26` (`stateFile`). [verified]
