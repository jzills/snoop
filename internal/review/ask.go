package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Permission requests wait in the asks folder while snoop shows them: one
// directory per session, one file per request, named for when it was
// asked and the PID of the hook waiting on it. snoop answers by claiming
// the request (renaming it to .claimed) and writing an .answer next to it,
// which the hook reads. Every file is written under a temporary name and
// renamed into place.

const (
	askExt    = ".json"
	answerExt = ".answer"
	// Any file in the asks folder older than this was left by a hook that
	// died; it outlasts the longest wait.
	askLeftover = 40 * time.Minute
	// How recently a snoop must have written the listening file to count.
	listenFresh = 5 * time.Second
)

// ErrAnswered is returned for a request already answered, given up on, or
// whose hook has exited.
var ErrAnswered = errors.New("already answered")

// Request is a tool call waiting for permission.
type Request struct {
	ID        string          `json:"-"`
	Session   string          `json:"session_id"`
	Cwd       string          `json:"cwd"`
	AgentID   string          `json:"agent_id,omitempty"`
	AgentType string          `json:"agent_type,omitempty"`
	Tool      string          `json:"tool_name"`
	Input     json.RawMessage `json:"tool_input"`
	Asked     time.Time       `json:"asked"`
	Deadline  time.Time       `json:"deadline"`
}

// Answer is how snoop answers a request: allow, or deny with an optional
// message for the agent.
type Answer struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}

// AsksDir is where requests wait.
func AsksDir() string { return filepath.Join(StateDir(), "asks") }

// ListeningFile is written every few seconds by each snoop that can show
// requests.
func ListeningFile() string { return filepath.Join(StateDir(), "listening") }

// Ask writes a request for its session and returns it with its ID set.
func Ask(dir string, r Request) (Request, error) {
	d, err := sessionDir(dir, r.Session)
	if err != nil {
		return r, err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return r, err
	}
	r.ID = fmt.Sprintf("%020d-%d", r.Asked.UnixNano(), os.Getpid())
	b, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	return r, writeRenamed(filepath.Join(d, r.ID+askExt), b)
}

// Pending lists the requests still waiting, across sessions, oldest first.
// It deletes requests whose hook has exited and files left long ago.
func Pending(dir string, now time.Time) ([]Request, error) {
	sessions, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Request
	for _, s := range sessions {
		if !s.IsDir() {
			continue
		}
		d := filepath.Join(dir, s.Name())
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			n := e.Name()
			at, pid, ok := askName(n)
			if !ok {
				continue
			}
			p := filepath.Join(d, n)
			if now.Sub(at) > askLeftover {
				os.Remove(p)
				continue
			}
			if !strings.HasSuffix(n, askExt) {
				continue
			}
			if !alive(pid) {
				os.Remove(p)
				continue
			}
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var r Request
			if json.Unmarshal(b, &r) != nil || !r.Deadline.After(now) {
				continue
			}
			r.ID = strings.TrimSuffix(n, askExt)
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Asked.Before(out[j].Asked) })
	return out, nil
}

// Respond answers a request. Only the first answer counts; later ones,
// and answers to requests given up on, return ErrAnswered.
func Respond(dir string, r Request, a Answer) error {
	d, err := sessionDir(dir, r.Session)
	if err != nil {
		return err
	}
	if _, pid, ok := askName(r.ID); !ok || !alive(pid) {
		return ErrAnswered
	}
	base := filepath.Join(d, r.ID)
	if err := os.Rename(base+askExt, base+claimedExt); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrAnswered
		}
		return err
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return writeRenamed(base+answerExt, append(b, '\n'))
}

// askName reads the time and the hook's PID from a request's file name.
func askName(name string) (at time.Time, pid int, ok bool) {
	ns, rest, ok := strings.Cut(name, "-")
	if !ok {
		return at, 0, false
	}
	if i := strings.IndexByte(rest, '.'); i >= 0 {
		rest = rest[:i]
	}
	n, err1 := strconv.ParseInt(ns, 10, 64)
	pid, err2 := strconv.Atoi(rest)
	if err1 != nil || err2 != nil {
		return at, 0, false
	}
	return time.Unix(0, n), pid, true
}

// Listen marks snoop as able to show requests.
func Listen(path string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeRenamed(path, fmt.Appendf(nil, "%d %d\n", os.Getpid(), now.UnixNano()))
}

// Listening reports whether a snoop wrote the listening file recently.
func Listening(path string, now time.Time) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var pid int
	var ns int64
	if _, err := fmt.Sscanf(string(b), "%d %d", &pid, &ns); err != nil {
		return false
	}
	return now.Sub(time.Unix(0, ns)) <= listenFresh
}

// writeRenamed writes a file under a temporary name, then renames it into
// place, so it is never read half-written.
func writeRenamed(path string, b []byte) error {
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
