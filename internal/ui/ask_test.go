package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jzills/snoop/internal/review"
)

func TestAskSummary(t *testing.T) {
	cases := []struct {
		r    review.Request
		want string
	}{
		{review.Request{Tool: "Bash", Input: json.RawMessage(`{"command":"npm test\n  --race"}`)}, "Bash: npm test --race"},
		{review.Request{Tool: "Edit", Input: json.RawMessage(`{"file_path":"/w/a.go","old_string":"x"}`)}, "Edit: /w/a.go"},
		{review.Request{Tool: "Write", Input: json.RawMessage(`{"file_path":"b.md"}`)}, "Write: b.md"},
		{review.Request{Tool: "WebFetch", Input: json.RawMessage(`{"url":"https://x.dev"}`)}, "WebFetch: https://x.dev"},
		{review.Request{Tool: "mcp__github__create_issue", Input: json.RawMessage(`{}`)}, "github·create_issue"},
		{review.Request{Tool: "Glob", Input: json.RawMessage(`{}`)}, "Glob"},
		{review.Request{Tool: "Bash", Input: json.RawMessage(`{"command":"ls"}`), AgentID: "a1", AgentType: "Explore"}, "Bash: ls via Explore"},
		{review.Request{Tool: "Bash", Input: json.RawMessage(`{"command":"ls"}`), AgentID: "a1"}, "Bash: ls via subagent"},
	}
	for _, c := range cases {
		if got := askSummary(c.r); got != c.want {
			t.Errorf("askSummary(%s) = %q, want %q", c.r.Tool, got, c.want)
		}
	}
}

// askApp is an app whose requests and listening file live in a temporary
// folder.
func askApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	a := New(Options{Root: t.TempDir(), AsksDir: filepath.Join(dir, "asks"), ListeningFile: filepath.Join(dir, "listening")})
	return a, dir
}

// runAsks runs the commands a tick starts for requests, feeding their
// messages back, as the runtime would.
func runAsks(a *App, now time.Time) {
	for _, c := range a.askCmds(now) {
		if m := c(); m != nil {
			a.Update(m)
		}
	}
}

func askFor(t *testing.T, dir, session, command string, at time.Time) review.Request {
	t.Helper()
	r, err := review.Ask(filepath.Join(dir, "asks"), review.Request{Session: session, Tool: "Bash",
		Input: json.RawMessage(`{"command":"` + command + `"}`), Asked: at, Deadline: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestListensOnlyWithABoardOrSession(t *testing.T) {
	a, dir := askApp(t)
	runAsks(a, time.Now())
	if _, err := os.Stat(filepath.Join(dir, "listening")); err == nil {
		t.Error("listening from the picker")
	}
	a.board = newBoard(a.opts.Root)
	runAsks(a, time.Now())
	if !review.Listening(filepath.Join(dir, "listening"), time.Now()) {
		t.Error("not listening with the board open")
	}
}

func TestAsksArriveOnTick(t *testing.T) {
	a, dir := askApp(t)
	a.board = newBoard(a.opts.Root)
	now := time.Now()
	askFor(t, dir, "s1", "npm test", now.Add(-time.Second))
	askFor(t, dir, "s1", "make", now)
	runAsks(a, now)
	if rs := a.asks["s1"]; len(rs) != 2 || askSummary(rs[0]) != "Bash: npm test" {
		t.Errorf("asks = %+v", a.asks)
	}
}
