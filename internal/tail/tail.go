// Package tail follows a growing transcript file and notices new sessions
// appearing in the same project folder.
package tail

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Update struct {
	Lines      [][]byte // complete new lines from the followed file
	NewSession string   // path of a newly created transcript, when watching for them
	Reset      bool     // the file shrank (truncated or replaced); Lines hold it from the start
	Err        error
}

type Tailer struct {
	path    string
	off     int64
	watcher *fsnotify.Watcher
	tick    *time.Ticker
	known   map[string]bool
	follow  bool
	done    chan struct{}
}

// ErrClosed is reported by Wait once the tailer has been closed.
var ErrClosed = errors.New("tail: closed")

// Open follows path from byte offset off. With followNew, transcripts
// created later in the same folder are reported as NewSession.
func Open(path string, off int64, followNew bool) (*Tailer, error) {
	t := &Tailer{path: path, off: off, follow: followNew, known: map[string]bool{}, done: make(chan struct{})}
	if w, err := fsnotify.NewWatcher(); err == nil {
		if w.Add(filepath.Dir(path)) == nil {
			t.watcher = w
		} else {
			w.Close()
		}
	}
	// Polling backs up fsnotify, which misses events on some filesystems
	// (e.g. Windows drives under WSL).
	t.tick = time.NewTicker(time.Second)
	for _, p := range t.transcripts() {
		t.known[p] = true
	}
	return t, nil
}

func (t *Tailer) Close() {
	close(t.done)
	t.tick.Stop()
	if t.watcher != nil {
		t.watcher.Close()
	}
}

func (t *Tailer) transcripts() []string {
	ps, _ := filepath.Glob(filepath.Join(filepath.Dir(t.path), "*.jsonl"))
	return ps
}

// Wait blocks until there is something new to report.
func (t *Tailer) Wait() Update {
	var events chan fsnotify.Event
	var errs chan error
	if t.watcher != nil {
		events, errs = t.watcher.Events, t.watcher.Errors
	}
	for {
		select {
		case <-t.done:
			return Update{Err: ErrClosed}
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if ev.Name == t.path && ev.Has(fsnotify.Write) {
				if u, ok := t.read(); ok {
					return u
				}
			}
			if t.follow && ev.Has(fsnotify.Create) && strings.HasSuffix(ev.Name, ".jsonl") && !t.known[ev.Name] {
				t.known[ev.Name] = true
				return Update{NewSession: ev.Name}
			}
		case <-errs:
			// Fall back to polling only.
			events, errs = nil, nil
		case <-t.tick.C:
			if u, ok := t.read(); ok {
				return u
			}
			if t.follow {
				for _, p := range t.transcripts() {
					if !t.known[p] {
						t.known[p] = true
						return Update{NewSession: p}
					}
				}
			}
		}
	}
}

func (t *Tailer) read() (Update, bool) {
	f, err := os.Open(t.path)
	if err != nil {
		return Update{Err: err}, true
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Update{}, false
	}
	reset := st.Size() < t.off
	if reset {
		t.off = 0
	}
	if st.Size() <= t.off && !reset {
		return Update{}, false
	}
	buf := make([]byte, st.Size()-t.off)
	n, err := f.ReadAt(buf, t.off)
	if err != nil && err != io.EOF {
		return Update{Err: err}, true
	}
	buf = buf[:n]
	end := bytes.LastIndexByte(buf, '\n')
	if end < 0 {
		return Update{Reset: reset}, reset
	}
	t.off += int64(end + 1)
	var lines [][]byte
	for _, l := range bytes.Split(buf[:end], []byte{'\n'}) {
		if len(l) > 0 {
			lines = append(lines, l)
		}
	}
	return Update{Lines: lines, Reset: reset}, len(lines) > 0 || reset
}
