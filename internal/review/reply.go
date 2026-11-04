package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// When a turn ends with nothing to deliver, the Stop hook waits for a
// reply sent from snoop, holding the turn open. It marks that in the
// turns folder (one directory per session, one file named for when the
// turn ended and the hook's PID), so snoop can offer a reply and treat the
// turn as ended. It stops waiting the moment a prompt is typed in the
// terminal, which Claude Code would otherwise queue behind the hook.

// Turn is a session whose Stop hook is waiting for a reply.
type Turn struct {
	Session  string    `json:"session_id"`
	Cwd      string    `json:"cwd"`
	Last     string    `json:"last_assistant_message,omitempty"`
	Ended    time.Time `json:"ended"`
	Deadline time.Time `json:"deadline"`
}

// TurnsDir is where waiting turns are marked.
func TurnsDir() string { return filepath.Join(StateDir(), "turns") }

// replyHeader frames a reply so the agent knows who sent it.
const replyHeader = "Message from the user, sent from snoop:\n\n"

// Reply queues a message from the user for a session. It reaches the
// agent through the next hook to run, and at once while a turn waits.
func Reply(inbox, session, text string, now time.Time) (string, error) {
	return Send(inbox, session, replyHeader+strings.TrimSpace(text)+"\n", now)
}

// WaitingTurns lists the sessions whose Stop hook is waiting, by session.
// It deletes markers whose hook has exited and files left long ago.
func WaitingTurns(dir string, now time.Time) (map[string]Turn, error) {
	sessions, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]Turn{}
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
			if !ok || !strings.HasSuffix(n, askExt) {
				continue
			}
			p := filepath.Join(d, n)
			if now.Sub(at) > askLeftover || !alive(pid) {
				os.Remove(p)
				continue
			}
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var t Turn
			if json.Unmarshal(b, &t) != nil || !t.Deadline.After(now) {
				continue
			}
			if old, ok := out[t.Session]; !ok || t.Ended.After(old.Ended) {
				out[t.Session] = t
			}
		}
	}
	return out, nil
}

// awaitReply holds the turn open until a reply arrives, and prints it as
// the reason to keep going. It gives up, printing nothing, when a prompt
// is queued in the terminal, no snoop is listening any more, or the wait
// runs out.
func awaitReply(h hookInput, out io.Writer, o HookOptions) error {
	now := o.Now()
	if o.Turns == "" || o.StopWait <= 0 || !Listening(o.Listening, now) || headless(h.TranscriptPath) {
		return nil
	}
	d, err := sessionDir(o.Turns, h.SessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	t := Turn{Session: h.SessionID, Cwd: h.Cwd, Last: h.Last, Ended: now, Deadline: now.Add(o.StopWait)}
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	marker := filepath.Join(d, fmt.Sprintf("%020d-%d%s", now.UnixNano(), os.Getpid(), askExt))
	if err := writeRenamed(marker, b); err != nil {
		return err
	}
	defer os.Remove(marker)
	w := newQueueWatch(h.TranscriptPath)
	for {
		now := o.Now()
		text, done, err := Claim(o.Inbox, h.SessionID, now)
		if err != nil {
			return err
		}
		if text != "" {
			if err := json.NewEncoder(out).Encode(map[string]string{"decision": "block", "reason": text}); err != nil {
				return err
			}
			done()
			return nil
		}
		if w.queued() || !now.Before(t.Deadline) || !Listening(o.Listening, now) {
			return nil
		}
		o.Sleep(askPoll)
	}
}

// headTail is how much of the transcript's end the hook reads to tell a
// headless run.
const headTail = 64 << 10

// headless reports whether a transcript is from a run with no terminal
// (claude -p, or the Agent SDK), which nobody could type a reply into and
// which a waiting hook would only delay.
func headless(path string) bool {
	if path == "" {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	// The newest record that says, from the end; a turn can end with one
	// record larger than the tail (a prompt snapshot), so failing that, the
	// first records, which always say.
	b := make([]byte, headTail)
	if info, err := f.Stat(); err == nil && info.Size() > headTail {
		n, _ := f.ReadAt(b, info.Size()-headTail)
		if v, ok := entrypoint(b[:n]); ok {
			return v
		}
	}
	n, _ := f.ReadAt(b, 0)
	v, _ := entrypoint(b[:n])
	return v
}

// entrypoint reports whether the last entrypoint in b is a headless one,
// and whether b names one.
func entrypoint(b []byte) (sdk, ok bool) {
	const key = `"entrypoint":"`
	i := bytes.LastIndex(b, []byte(key))
	if i < 0 {
		return false, false
	}
	return bytes.HasPrefix(b[i+len(key):], []byte("sdk")), true
}

// queueWatch notices a prompt queued in the transcript after it started.
type queueWatch struct {
	path string
	off  int64
	rest []byte // a line not yet ended
}

func newQueueWatch(path string) *queueWatch {
	w := &queueWatch{path: path}
	if info, err := os.Stat(path); err == nil {
		w.off = info.Size()
	}
	return w
}

// queued reads what was written since the last call and reports whether
// it queued a prompt.
func (w *queueWatch) queued() bool {
	if w.path == "" {
		return false
	}
	f, err := os.Open(w.path)
	if err != nil {
		return false
	}
	defer f.Close()
	if _, err := f.Seek(w.off, io.SeekStart); err != nil {
		return false
	}
	b, _ := io.ReadAll(io.LimitReader(f, 16<<20))
	w.off += int64(len(b))
	data := append(w.rest, b...)
	// Only whole lines are judged; the rest waits for its newline.
	end := bytes.LastIndexByte(data, '\n') + 1
	w.rest = append([]byte(nil), data[end:]...)
	for _, line := range bytes.Split(data[:end], []byte{'\n'}) {
		if bytes.Contains(line, []byte(`"queue-operation"`)) && bytes.Contains(line, []byte(`"enqueue"`)) {
			return true
		}
	}
	return false
}
