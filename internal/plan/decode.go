package plan

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const checkDecode = "decode"

// kinds lists, per item kind, the fields it requires and the fields it may carry besides id
// and kind. An item carrying any other field is refused.
var kinds = map[string]struct{ required, optional []string }{
	"fact":     {required: []string{"fact"}, optional: []string{"use"}},
	"sentence": {required: []string{"mode", "text"}, optional: []string{"facts"}},
	"link":     {required: []string{"target"}, optional: []string{"text"}},
	"handoff":  {required: []string{"text"}},
	"reason":   {required: []string{"text"}, optional: []string{"facts"}},
	"step":     {required: []string{"text"}, optional: []string{"requires", "produces", "canFail", "optional"}},
	"failure":  {required: []string{"symptom", "handledBy"}, optional: []string{"from", "cause", "facts"}},
	"code":     {required: []string{"text"}, optional: []string{"source", "facts"}},
	"figure":   {required: []string{"text"}, optional: []string{"source"}},
}

var (
	idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	lineInErr = regexp.MustCompile(`line (\d+)`)
)

const quotedStyles = yaml.DoubleQuotedStyle | yaml.SingleQuotedStyle | yaml.LiteralStyle | yaml.FoldedStyle

type decoder struct {
	file     string
	findings []Finding
}

func (d *decoder) add(line int, msg, fix string) {
	d.findings = append(d.findings, Finding{
		File: d.file, Line: line, Severity: SeverityError, Check: checkDecode, Message: msg, Fix: fix,
	})
}

// Decode parses data as a plan and reports every schema fault it finds, each at its line.
// It walks the yaml.Node tree itself because the library's own decoding drops a duplicate key,
// ignores an unknown field, accepts an empty document, skips a second document, and cuts a
// plain-style value at a line comment, all without an error. The returned plan is nil when the
// file is not one YAML mapping; otherwise it is as far decoded as the faults allow, and a
// caller runs the plan checks only when HasError reports false.
func Decode(file string, data []byte) (*Plan, []Finding) {
	p := new(Plan)
	ok, findings := DecodeDocument(file, data, p, func(r *Reporter) { r.d.validate(p) })
	if !ok {
		return nil, findings
	}
	return p, findings
}

// Reporter lets the validate function passed to DecodeDocument add findings to the decode.
type Reporter struct{ d *decoder }

// Add records a decode error at line.
func (r *Reporter) Add(line int, msg, fix string) { r.d.add(line, msg, fix) }

// Faulted reports whether a finding already sits at line, so a rule that needs a well-formed
// value can skip one the walk already refused.
func (r *Reporter) Faulted(line int) bool { return r.d.faultAt(line) }

// ValidID reports whether s is a lowercase token usable as an item or section id.
func ValidID(s string) bool { return idPattern.MatchString(s) }

// DecodeDocument parses data as one YAML mapping into v, a pointer to a struct whose fields
// carry the yaml and plan tags that Plan's do, and reports every fault the walk finds, each at
// its line. validate, when not nil, runs after the walk to apply the rules that need the whole
// value. It returns false when the file is not one YAML mapping, and v is then untouched.
func DecodeDocument(file string, data []byte, v any, validate func(*Reporter)) (bool, []Finding) {
	d := &decoder{file: file}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			d.add(1, "the file holds no YAML document", "write the file as a YAML mapping that starts with version: 1")
		} else {
			d.add(errLine(err), "malformed YAML: "+strings.TrimPrefix(err.Error(), "yaml: "), "fix the syntax at this line")
		}
		return false, d.findings
	}
	var extra yaml.Node
	switch err := dec.Decode(&extra); {
	case err == nil:
		line := extra.Line
		if len(extra.Content) > 0 {
			line = extra.Content[0].Line
		}
		d.add(line, "the file holds a second YAML document", "keep one document per file and delete everything from the second --- on")
		return false, d.findings
	case !errors.Is(err, io.EOF):
		d.add(errLine(err), "malformed YAML: "+strings.TrimPrefix(err.Error(), "yaml: "), "fix the syntax at this line")
		return false, d.findings
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		line := 1
		if len(doc.Content) > 0 {
			line = doc.Content[0].Line
		}
		d.add(line, "the file must be a YAML mapping", "write key: value lines, starting with version: 1")
		return false, d.findings
	}
	d.structure(doc.Content[0], reflect.ValueOf(v).Elem(), "")
	if validate != nil {
		validate(&Reporter{d})
	}
	slices.SortStableFunc(d.findings, func(a, b Finding) int { return cmp.Compare(a.Line, b.Line) })
	return true, d.findings
}

func errLine(err error) int {
	if m := lineInErr.FindStringSubmatch(err.Error()); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 1
}

type fieldSpec struct {
	index                  int
	name                   string
	required, prose, empty bool
}

func fieldsOf(t reflect.Type) []fieldSpec {
	var out []fieldSpec
	for i := range t.NumField() {
		name := t.Field(i).Tag.Get("yaml")
		if name == "" || name == "-" {
			continue
		}
		opts := strings.Split(t.Field(i).Tag.Get("plan"), ",")
		out = append(out, fieldSpec{index: i, name: name, required: slices.Contains(opts, "required"), prose: slices.Contains(opts, "prose"), empty: slices.Contains(opts, "empty")})
	}
	return out
}

func at(where, msg string) string {
	if where == "" {
		return msg
	}
	return where + ": " + msg
}

func (d *decoder) structure(n *yaml.Node, v reflect.Value, where string) {
	if n.Kind != yaml.MappingNode {
		d.add(n.Line, at(where, "must be a mapping"), "write it as key: value lines")
		return
	}
	specs := fieldsOf(v.Type())
	lines := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			d.add(key.Line, at(where, "a key must be a plain word"), "rename the key")
			continue
		}
		if first, dup := lines[key.Value]; dup {
			d.add(key.Line, at(where, fmt.Sprintf("duplicate key %q (first at line %d)", key.Value, first)), "delete one of the two")
			continue
		}
		lines[key.Value] = key.Line
		j := slices.IndexFunc(specs, func(s fieldSpec) bool { return s.name == key.Value })
		if j < 0 {
			names := make([]string, len(specs))
			for k, s := range specs {
				names[k] = s.name
			}
			d.add(key.Line, at(where, fmt.Sprintf("unknown field %q", key.Value)), "remove it or fix the spelling; the fields here are "+strings.Join(names, ", "))
			delete(lines, key.Value)
			continue
		}
		d.value(val, v.Field(specs[j].index), specs[j], join(where, key.Value))
	}
	for _, s := range specs {
		if _, ok := lines[s.name]; s.required && !ok {
			d.add(n.Line, at(where, fmt.Sprintf("missing required field %q", s.name)), fmt.Sprintf("add %s:", s.name))
		}
	}
	v.FieldByName("Line").SetInt(int64(n.Line))
	v.FieldByName("Lines").Set(reflect.ValueOf(lines))
}

func join(where, name string) string {
	if where == "" {
		return name
	}
	return where + "." + name
}

func (d *decoder) value(n *yaml.Node, f reflect.Value, s fieldSpec, where string) {
	if n.Kind == yaml.AliasNode {
		d.add(n.Line, at(where, "aliases are not supported"), "write the value out")
		return
	}
	switch f.Kind() {
	case reflect.String:
		d.str(n, f, s, where)
	case reflect.Int:
		var i int
		if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!int" || n.Decode(&i) != nil {
			d.add(n.Line, at(where, "must be an integer"), "write a whole number")
			return
		}
		f.SetInt(int64(i))
	case reflect.Bool:
		var b bool
		if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!bool" || n.Decode(&b) != nil {
			d.add(n.Line, at(where, "must be true or false"), "write true or false")
			return
		}
		f.SetBool(b)
	case reflect.Struct:
		d.structure(n, f, where)
	case reflect.Slice:
		d.list(n, f, s, where)
	}
}

func (d *decoder) str(n *yaml.Node, f reflect.Value, s fieldSpec, where string) {
	if n.Kind == yaml.SequenceNode && s.prose {
		d.add(n.Line, at(where, "must be a string, and this value decodes as a list; a value that starts with [[ is a YAML list"), "quote the value or write it as a | block")
		return
	}
	if n.Kind != yaml.ScalarNode || n.ShortTag() == "!!null" {
		d.add(n.Line, at(where, "must be a string"), "write a value, or remove the field")
		return
	}
	if strings.TrimSpace(n.Value) == "" {
		d.add(n.Line, at(where, "is empty"), "write a value, or remove the field")
		return
	}
	if s.prose && n.Style&quotedStyles == 0 && n.LineComment != "" {
		d.add(n.Line, at(where, "a plain-style prose value carries a line comment, and YAML cuts the value at the #"), "write the value as a | block or put it in quotes")
	}
	f.SetString(n.Value)
}

func (d *decoder) list(n *yaml.Node, f reflect.Value, s fieldSpec, where string) {
	if n.Kind != yaml.SequenceNode {
		d.add(n.Line, at(where, "must be a list"), "write one - entry per line")
		return
	}
	if s.required && !s.empty && len(n.Content) == 0 {
		d.add(n.Line, at(where, "must hold at least one entry"), "add an entry")
	}
	for i, c := range n.Content {
		elem := reflect.New(f.Type().Elem()).Elem()
		d.value(c, elem, s, fmt.Sprintf("%s[%d]", where, i))
		f.Set(reflect.Append(f, elem))
	}
}

func (d *decoder) faultAt(line int) bool {
	return slices.ContainsFunc(d.findings, func(f Finding) bool { return f.Line == line })
}

// validate checks the rules that need a whole decoded value: the version, the page path, each
// item's kind and the fields that kind allows, and each disposition.
func (d *decoder) validate(p *Plan) {
	if line, ok := p.Lines["version"]; ok && p.Version != Version && !d.faultAt(line) {
		d.add(line, fmt.Sprintf("unsupported version %d", p.Version), "set version: 1")
	}
	if line, ok := p.Page.Lines["path"]; ok && !filepath.IsLocal(p.Page.Path) && !d.faultAt(line) {
		d.add(line, fmt.Sprintf("page: path %q is not a path inside the repository", p.Page.Path),
			"write the path relative to the repository root, with no leading / and no .. that climbs out")
	}
	for i := range p.Sections {
		s := &p.Sections[i]
		if line, ok := s.Lines["level"]; !ok {
			s.Level = 2
		} else if s.Level != 2 && s.Level != 3 && !d.faultAt(line) {
			d.add(line, fmt.Sprintf("sections[%d]: level %d is not a heading level the plan supports", i, s.Level), "use level: 2 or level: 3, or omit it for 2")
		}
	}
	for i := range p.Sections {
		s := &p.Sections[i]
		parentOfSubsections := s.Level == 2 && i+1 < len(p.Sections) && p.Sections[i+1].Level == 3
		if line, ok := s.Lines["items"]; ok && len(s.Items) == 0 && !parentOfSubsections && !d.faultAt(line) {
			d.add(line, fmt.Sprintf("sections[%d].items: must hold at least one entry unless the next section is level 3", i), "add an entry, or leave items empty only above a level-3 section")
		}
		for j := range s.Items {
			d.item(&s.Items[j], fmt.Sprintf("sections[%d].items[%d]", i, j))
		}
	}
	for i := range p.Dispositions {
		x := &p.Dispositions[i]
		where := fmt.Sprintf("dispositions[%d]", i)
		switch x.Disposition {
		case "":
		case "cut":
		case "subordinated":
			if _, ok := x.Lines["reference"]; !ok {
				d.add(x.Line, at(where, "a subordinated fact needs a reference"), "add reference: with the link target that now carries the fact")
			}
		default:
			d.add(x.Lines["disposition"], at(where, fmt.Sprintf("unknown disposition %q", x.Disposition)), "use subordinated or cut")
		}
	}
}

func (d *decoder) item(it *Item, where string) {
	if line, ok := it.Lines["id"]; ok && !idPattern.MatchString(it.ID) {
		d.add(line, at(where, fmt.Sprintf("id %q is not a lowercase token", it.ID)), "use lowercase letters, digits, - and _")
	}
	kind, ok := it.Lines["kind"]
	if !ok {
		return
	}
	spec, known := kinds[it.Kind]
	if !known {
		names := slices.Sorted(maps.Keys(kinds))
		d.add(kind, at(where, fmt.Sprintf("unknown kind %q", it.Kind)), "use one of "+strings.Join(names, ", "))
		return
	}
	allowed := slices.Concat([]string{"id", "kind"}, spec.required, spec.optional)
	for name, line := range it.Lines {
		if !slices.Contains(allowed, name) {
			d.add(line, at(where, fmt.Sprintf("field %q does not belong to kind %q", name, it.Kind)), "remove it; a "+it.Kind+" item takes "+strings.Join(allowed, ", "))
		}
	}
	for _, name := range spec.required {
		if _, ok := it.Lines[name]; !ok {
			d.add(it.Line, at(where, fmt.Sprintf("kind %q needs the field %q", it.Kind, name)), fmt.Sprintf("add %s:", name))
		}
	}
	if line, ok := it.Lines["mode"]; ok && it.Mode != "verbatim" && it.Mode != "claim" {
		d.add(line, at(where, fmt.Sprintf("unknown mode %q", it.Mode)), "use verbatim or claim")
	}
}
