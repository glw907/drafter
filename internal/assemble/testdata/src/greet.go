// Package greet is a synthetic fixture.
package greet

import (
	"fmt"
	"strings"
)

// Greeting is mentioned here, before any declaration of Shout.

const Prefix = "hello"

var Suffix = "!"

type Greeter struct {
	Name string
}

func New(name string) *Greeter {
	return &Greeter{Name: name}
}

func (g *Greeter) Hello() string {
	return fmt.Sprintf("%s, %s%s", Prefix, g.Name, Suffix)
}

func Shout(s string) string {
	return strings.ToUpper(s)
}

// line 31
// line 32
// line 33
// line 34
// line 35
// line 36
// line 37
// line 38
// line 39
// line 40
// line 41
// line 42
// line 43
// line 44
// line 45
// line 46
// line 47
// line 48
// line 49
// line 50
