// Package prompts runs a JavaScript prompts module in goja and renders its seat prompts.
//
// A module is a script that runs under an async wrapper and returns an object mapping each seat
// name to a function of one context object that returns the prompt text. The runtime defines no
// require, fetch, process, console, or setTimeout, so a module computes text and does nothing else.
package prompts

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/dop251/goja"
)

// Host is what the loader hands the module: the handoff context's args and resolved pages, as
// raw JSON. Resolved is required, since the chain cannot run without its resolved pages.
type Host struct {
	Args     json.RawMessage
	Resolved json.RawMessage
}

// Module is a loaded prompts module. It is not safe for concurrent use.
type Module struct {
	name  string
	vm    *goja.Runtime
	parse goja.Callable
	seats map[string]goja.Callable
}

var exportMeta = regexp.MustCompile(`(?m)^export const meta`)

// prelude defines the globals a module sees besides args and DRAFTER_HOST.
const prelude = `
var phase = function () {};
var log = function () {};
var parallel = function () {};
var pipeline = function () {};
var agent = function () {
	throw new Error("agent called at load: a prompts module cannot start agents");
};
`

// Load runs src, named name in errors, and returns the module its seat map describes. The
// wrapper adds nothing before src's first line, so line numbers in errors match src.
func Load(name string, src []byte, h Host) (*Module, error) {
	if len(h.Resolved) == 0 || string(h.Resolved) == "null" {
		return nil, fmt.Errorf("load %s: host has no resolved pages: the module needs the chain context (context.resolved)", name)
	}
	args := h.Args
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	host, err := json.Marshal(struct {
		Args     json.RawMessage `json:"args"`
		Resolved json.RawMessage `json:"resolved"`
	}{args, h.Resolved})
	if err != nil {
		return nil, fmt.Errorf("load %s: encode host: %w", name, err)
	}

	vm := goja.New()
	parse, ok := goja.AssertFunction(vm.Get("JSON").ToObject(vm).Get("parse"))
	if !ok {
		return nil, fmt.Errorf("load %s: JSON.parse is not callable", name)
	}
	for _, g := range []struct {
		name string
		text string
	}{{"args", string(args)}, {"DRAFTER_HOST", string(host)}} {
		v, err := parse(goja.Undefined(), vm.ToValue(g.text))
		if err != nil {
			return nil, fmt.Errorf("load %s: parse %s: %w", name, g.name, err)
		}
		if err := vm.Set(g.name, v); err != nil {
			return nil, fmt.Errorf("load %s: define %s: %w", name, g.name, err)
		}
	}
	if _, err := vm.RunString(prelude); err != nil {
		return nil, fmt.Errorf("load %s: define globals: %w", name, err)
	}

	body := exportMeta.ReplaceAllString(string(src), "const meta")
	prog, err := goja.Compile(name, "(async function () {"+body+"\n})()", false)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", name, err)
	}
	res, err := vm.RunProgram(prog)
	if err != nil {
		return nil, fmt.Errorf("load %s: %s", name, describe(err))
	}
	p, ok := res.Export().(*goja.Promise)
	if !ok {
		return nil, fmt.Errorf("load %s: script did not produce a promise", name)
	}
	switch p.State() {
	case goja.PromiseStateRejected:
		return nil, fmt.Errorf("load %s: %s", name, describeValue(p.Result()))
	case goja.PromiseStatePending:
		return nil, fmt.Errorf("load %s: script awaits a promise that never settles", name)
	}

	obj, ok := p.Result().(*goja.Object)
	if !ok {
		return nil, fmt.Errorf("load %s: script returned no seat map", name)
	}
	m := &Module{name: name, vm: vm, parse: parse, seats: map[string]goja.Callable{}}
	for _, k := range obj.Keys() {
		fn, ok := goja.AssertFunction(obj.Get(k))
		if !ok {
			return nil, fmt.Errorf("load %s: seat %q is not a function", name, k)
		}
		m.seats[k] = fn
	}
	return m, nil
}

// Seats returns the module's seat names in sorted order.
func (m *Module) Seats() []string {
	names := make([]string, 0, len(m.seats))
	for k := range m.seats {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

// Render calls the seat's builder with c and returns the prompt. A seat the module does not
// define, a builder that throws, and a builder that returns anything but a string are errors.
func (m *Module) Render(seat string, c Context) (string, error) {
	fn, ok := m.seats[seat]
	if !ok {
		return "", fmt.Errorf("render %s: no seat %q (have %s)", m.name, seat, strings.Join(m.Seats(), ", "))
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("render %s seat %s: encode context: %w", m.name, seat, err)
	}
	arg, err := m.parse(goja.Undefined(), m.vm.ToValue(string(raw)))
	if err != nil {
		return "", fmt.Errorf("render %s seat %s: parse context: %w", m.name, seat, err)
	}
	res, err := fn(goja.Undefined(), arg)
	if err != nil {
		return "", fmt.Errorf("render %s seat %s: %s", m.name, seat, describe(err))
	}
	s, ok := res.Export().(string)
	if !ok {
		return "", fmt.Errorf("render %s seat %s: builder returned %s, not a string", m.name, seat, res.ExportType())
	}
	return s, nil
}

// describe renders a goja failure with its message and, for a JavaScript exception, its line.
func describe(err error) string {
	if ex, ok := errors.AsType[*goja.Exception](err); ok {
		return describeValue(ex.Value())
	}
	return err.Error()
}

// describeValue renders a thrown value as "Name: message at file:line:col".
func describeValue(v goja.Value) string {
	o, ok := v.(*goja.Object)
	if !ok {
		return v.String()
	}
	msg := o.Get("message")
	if msg == nil {
		return v.String()
	}
	out := msg.String()
	if n := o.Get("name"); n != nil {
		out = n.String() + ": " + out
	}
	if st := o.Get("stack"); st != nil {
		for line := range strings.SplitSeq(st.String(), "\n") {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "at ") {
				out += " " + t
				break
			}
		}
	}
	return out
}
