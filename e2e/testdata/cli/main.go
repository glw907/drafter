// Command tally keeps a running count of items in a file.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// stateFile is where tally keeps its count, in the current directory.
const stateFile = ".tally"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tally <count|total|reset>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "count":
		runCount(os.Args[2:])
	case "total":
		fmt.Println(readTotal())
	case "reset":
		os.Remove(stateFile)
	default:
		fmt.Fprintf(os.Stderr, "tally: unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

// runCount adds the items to the total. The --by flag sets how much each item adds.
func runCount(args []string) {
	fs := flag.NewFlagSet("count", flag.ExitOnError)
	by := fs.Int("by", 1, "the amount each item adds")
	fs.Parse(args)
	total := readTotal() + *by*fs.NArg()
	os.WriteFile(stateFile, []byte(strconv.Itoa(total)), 0o644)
}

// readTotal returns the saved total, or 0 when tally has not counted yet.
func readTotal() int {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return n
}
