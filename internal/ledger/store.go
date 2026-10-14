package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/glw907/drafter/internal/batch"
)

const (
	version     = 1
	typeReserve = "reserve"
	typeSettle  = "settle"
)

// line is one ledger line. Readers ignore fields they do not know.
type line struct {
	V    int       `json:"v"`
	Type string    `json:"type"`
	Time time.Time `json:"time"`
	Entry
	Intent   string       `json:"intent,omitempty"`
	Requests int          `json:"requests,omitempty"`
	Worst    float64      `json:"worst,omitempty"`
	Key      string       `json:"key,omitempty"`
	Usage    *batch.Usage `json:"usage,omitempty"`
	Cost     float64      `json:"cost,omitempty"`
	Kind     string       `json:"kind,omitempty"`
}

type settleRec struct {
	time time.Time
	cost float64
}

// intent is what the ledger holds about one submission unit.
type intent struct {
	res      Reservation
	reserved bool
	entry    Entry
	settles  []settleRec
	count    int // settle lines other than void
	void     bool
}

func (it *intent) open() bool {
	return it.reserved && !it.void && it.count < it.res.Requests
}

// settled is the sum of the intent's settle lines at or after from.
func (it *intent) settled(from time.Time) float64 {
	var sum float64
	for _, s := range it.settles {
		if !s.time.Before(from) {
			sum += s.cost
		}
	}
	return sum
}

type state struct {
	intents map[string]*intent
	order   []string
	keys    map[string]bool
	settles []line
	tornAt  int64 // offset of the fragment the file ends in, when tornLen > 0
	tornLen int   // bytes in that fragment, which holds no newline
}

// pageTotal is the page's settled spend plus its open intents at their worst case.
func (s *state) pageTotal(runID, page string) float64 {
	var sum float64
	for _, id := range s.order {
		it := s.intents[id]
		if it.entry.RunID != runID || it.entry.Page != page {
			continue
		}
		if it.open() {
			sum += it.res.Worst
		} else {
			sum += it.settled(time.Time{})
		}
	}
	return sum
}

// cycleTotal is the settled spend dated at or after start plus every open intent, whenever it
// was reserved.
func (s *state) cycleTotal(start time.Time) float64 {
	var sum float64
	for _, id := range s.order {
		it := s.intents[id]
		if it.open() {
			sum += it.res.Worst
		} else {
			sum += it.settled(start)
		}
	}
	return sum
}

func (l *Ledger) path() string     { return filepath.Join(l.dir, "ledger.jsonl") }
func (l *Ledger) lockPath() string { return filepath.Join(l.dir, "ledger.lock") }

func (l *Ledger) warn(format string, a ...any) {
	w := l.Warn
	if w == nil {
		w = os.Stderr
	}
	_, _ = fmt.Fprintf(w, format+"\n", a...)
}

// lock takes the blocking exclusive lock, creating the state directory and lock file on first
// use, and returns the function that releases it.
func (l *Ledger) lock() (func(), error) {
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	f, err := os.OpenFile(l.lockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open ledger lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock %s: %w", l.lockPath(), err)
	}
	return func() { _ = f.Close() }, nil
}

// rlock takes the shared lock for a read, so a read waits out a writer's check and append. It
// never creates the state directory: with no directory there is no ledger, and ok is false. A
// directory without ledger.lock gets one, so a writer that starts next locks the same file.
// Call it only from the public read methods; Reserve and Settle already hold the exclusive lock,
// and flock treats a second descriptor on the file as a separate holder that would deadlock.
func (l *Ledger) rlock() (unlock func(), ok bool, err error) {
	f, err := os.OpenFile(l.lockPath(), os.O_CREATE|os.O_RDONLY, 0o600)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("open ledger lock %s: %w", l.lockPath(), err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		_ = f.Close()
		return nil, false, fmt.Errorf("lock %s: %w", l.lockPath(), err)
	}
	return func() { _ = f.Close() }, true, nil
}

// readShared reads the ledger under the shared lock; an absent state directory reads as empty.
func (l *Ledger) readShared() (*state, error) {
	unlock, ok, err := l.rlock()
	if err != nil {
		return nil, err
	}
	if ok {
		defer unlock()
	}
	return l.read()
}

// read parses the whole ledger. An absent file reads as empty. A final line with no newline is a
// torn write: it is skipped with a warning, and the next writer truncates it away. Any other line
// that does not parse is an error. A legacy torn marker line parses and is ignored like any
// unknown type.
func (l *Ledger) read() (*state, error) {
	st := &state{intents: map[string]*intent{}, keys: map[string]bool{}}
	data, err := os.ReadFile(l.path())
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ledger: %w", err)
	}
	var tail []byte
	end := bytes.LastIndexByte(data, '\n') + 1
	if end < len(data) {
		data, tail = data[:end], data[end:]
	}
	var raw [][]byte
	if len(data) > 0 {
		raw = bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	}
	for i, b := range raw {
		ln, err := parseLine(b)
		if err != nil {
			return nil, fmt.Errorf("ledger %s line %d: %w", l.path(), i+1, err)
		}
		st.add(ln)
	}
	if len(tail) > 0 {
		st.tornAt, st.tornLen = int64(end), len(tail)
		l.warn("ledger %s line %d: skipping torn final line of %d bytes", l.path(), len(raw)+1, len(tail))
	}
	return st, nil
}

func parseLine(b []byte) (*line, error) {
	var ln line
	if err := json.Unmarshal(b, &ln); err != nil {
		return nil, err
	}
	if ln.V != version {
		return nil, fmt.Errorf("unsupported line version %d", ln.V)
	}
	if ln.Type == "" {
		return nil, errors.New("line has no type")
	}
	return &ln, nil
}

func (st *state) get(id string, e Entry) *intent {
	it, ok := st.intents[id]
	if !ok {
		it = &intent{entry: e}
		st.intents[id] = it
		st.order = append(st.order, id)
	}
	return it
}

func (st *state) add(ln *line) {
	switch ln.Type {
	case typeReserve:
		it := st.get(ln.Intent, ln.Entry)
		it.reserved = true
		it.entry = ln.Entry
		it.res = Reservation{Entry: ln.Entry, Intent: ln.Intent, Requests: ln.Requests, Worst: ln.Worst}
	case typeSettle:
		// A crash can leave a settle line complete but unterminated; the retry then appends the
		// same key again. The first line wins.
		if st.keys[ln.Key] {
			return
		}
		it := st.get(ln.Intent, ln.Entry)
		it.settles = append(it.settles, settleRec{ln.Time, ln.Cost})
		if ln.Kind == "void" {
			it.void = true
		} else {
			it.count++
		}
		st.keys[ln.Key] = true
		st.settles = append(st.settles, *ln)
	}
}

// append writes the lines as one write on an O_APPEND descriptor and syncs. After a torn tail it
// first truncates the file to the end of its last complete line and syncs, so a crash at any
// point leaves the fragment, the truncated file, or the truncated file with a torn new line, each
// of which the next call reads and repairs. The append must go on an O_APPEND descriptor: truncate
// leaves the file offset where it was, and a plain write would land past the new end and leave
// NUL bytes. The caller holds the exclusive lock, so st is current.
func (l *Ledger) append(st *state, lns ...line) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, ln := range lns {
		if err := enc.Encode(ln); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(l.path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open ledger: %w", err)
	}
	if st.tornLen > 0 {
		if err := f.Truncate(st.tornAt); err != nil {
			_ = f.Close()
			return fmt.Errorf("truncate torn tail of ledger: %w", err)
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return fmt.Errorf("sync ledger: %w", err)
		}
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		return fmt.Errorf("append to ledger: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync ledger: %w", err)
	}
	return f.Close()
}

func (l *Ledger) stamp() time.Time { return l.Now().UTC() }
