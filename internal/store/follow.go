package store

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jzills/snoop/internal/parse"
)

// Follower keeps a parsed transcript current by reading only what was
// added since the last poll.
type Follower struct {
	Path string
	Sess *parse.Session
	Off  int64 // bytes parsed so far
}

// Track parses a whole transcript and returns a Follower for it.
func Track(path string) (*Follower, error) {
	s, off, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Follower{Path: path, Sess: s, Off: off}, nil
}

// Poll reads complete lines added since the last poll. A file that shrank
// was replaced or truncated, so it is parsed again from the start into a
// new Sess and reset is true.
func (f *Follower) Poll() (changed, reset bool) {
	st, err := os.Stat(f.Path)
	if err != nil {
		return false, false
	}
	if st.Size() < f.Off {
		s, off, err := Load(f.Path)
		if err != nil {
			return false, false
		}
		f.Sess, f.Off = s, off
		return true, true
	}
	if st.Size() == f.Off {
		return false, false
	}
	fh, err := os.Open(f.Path)
	if err != nil {
		return false, false
	}
	defer fh.Close()
	if _, err := fh.Seek(f.Off, 0); err != nil {
		return false, false
	}
	n, _ := f.Sess.Read(fh)
	f.Off += n
	return n > 0, false
}

// Activity is a transcript and when it, or any of its subagents, was last
// written.
type Activity struct {
	Path string
	Last time.Time
}

// subagentHorizon is how far back a quiet transcript may have started a
// subagent that is still writing.
const subagentHorizon = 24 * time.Hour

// Recent lists the top-level transcripts under root written since since,
// counting writes to their subagents' transcripts. It only stats files.
func Recent(root string, since time.Time) []Activity {
	paths, _ := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	var out []Activity
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		last := st.ModTime()
		if last.Before(since) && last.After(since.Add(-subagentHorizon)) {
			if sub := subagentWrite(p); sub.After(last) {
				last = sub
			}
		}
		if !last.Before(since) {
			out = append(out, Activity{p, last})
		}
	}
	return out
}

// subagentWrite is the newest write to any of a session's subagent
// transcripts, or the zero time.
func subagentWrite(sessionPath string) time.Time {
	var last time.Time
	subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(sessionPath, ".jsonl"), "subagents", "agent-*.jsonl"))
	for _, sp := range subs {
		if st, err := os.Stat(sp); err == nil && st.ModTime().After(last) {
			last = st.ModTime()
		}
	}
	return last
}

// SubagentWrittenSince reports whether any of a session's subagents wrote
// at or after t.
func SubagentWrittenSince(sessionPath string, t time.Time) bool {
	return !subagentWrite(sessionPath).Before(t)
}
