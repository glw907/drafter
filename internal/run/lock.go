package run

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Lock is a held, non-blocking flock on a file.
type Lock struct {
	f *os.File
}

// TryLock takes an exclusive flock on path, creating the file if needed,
// and writes the holder's PID into it. A busy lock returns an error naming
// the PID in the file. Because flock treats each descriptor independently,
// a second TryLock on the same path in one process is refused too.
func TryLock(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		holder := "unknown"
		if b, rerr := os.ReadFile(path); rerr == nil && strings.TrimSpace(string(b)) != "" {
			holder = strings.TrimSpace(string(b))
		}
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("lock %s is held by pid %s", path, holder)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	if err := f.Truncate(0); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write lock %s: %w", path, err)
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write lock %s: %w", path, err)
	}
	return &Lock{f: f}, nil
}

// Release unlocks and closes the lock. It leaves the file in place.
func (l *Lock) Release() error {
	// Closing the descriptor drops the flock.
	return l.f.Close()
}
