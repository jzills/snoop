package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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

// boardApp is an app on the live board of newTestBoard, with requests in
// a temporary folder.
func boardApp(t *testing.T) (*App, string) {
	t.Helper()
	a, dir := askApp(t)
	b, _ := newTestBoard(t)
	a.board = b
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return a, dir
}

// answerFile reads the answer written for a request, or "".
func answerFile(dir string, r review.Request) string {
	b, _ := os.ReadFile(filepath.Join(dir, "asks", r.Session, r.ID+".answer"))
	return strings.TrimSpace(string(b))
}

// pressAsk sends a key and runs the command it returns, feeding the message
// back.
func pressAsk(a *App, k string) {
	_, cmd := a.Update(key(k))
	if cmd != nil {
		if m := cmd(); m != nil {
			if _, ok := m.(answeredMsg); ok {
				a.Update(m)
			}
		}
	}
}

func TestRowAskingComesFirst(t *testing.T) {
	a, dir := boardApp(t)
	askFor(t, dir, "done", "npm test", time.Now())
	runAsks(a, time.Now())
	out := ansi.Strip(a.View())
	if i, j := strings.Index(out, "? asking"), strings.Index(out, "◌ waiting"); i < 0 || j < 0 || i > j {
		t.Errorf("asking should sort above waiting:\n%s", out)
	}
	for _, want := range []string{"Bash: npm test", "2 waiting on you"} {
		if !strings.Contains(out, want) {
			t.Errorf("board lacks %q:\n%s", want, out)
		}
	}
}

func TestBoardShowsTheRequestInFull(t *testing.T) {
	a, dir := boardApp(t)
	r, _ := review.Ask(filepath.Join(dir, "asks"), review.Request{Session: "done", Cwd: "/w/api", Tool: "Bash",
		Input: json.RawMessage(`{"command":"rm -rf dist","description":"Clean the build"}`), Asked: time.Now(), Deadline: time.Now().Add(time.Minute)})
	if r.ID == "" {
		t.Fatal("not asked")
	}
	runAsks(a, time.Now())
	out := ansi.Strip(a.View())
	for _, want := range []string{"Request", "$ rm -rf dist", "Clean the build", "in /w/api", "A allow once"} {
		if !strings.Contains(out, want) {
			t.Errorf("board lacks %q:\n%s", want, out)
		}
	}
}

func TestAnswersTheOldestRequest(t *testing.T) {
	a, dir := boardApp(t)
	now := time.Now()
	old := askFor(t, dir, "done", "first", now.Add(-time.Second))
	newer := askFor(t, dir, "done", "second", now)
	runAsks(a, now)
	if out := ansi.Strip(a.View()); !strings.Contains(out, "(1 of 2)") {
		t.Errorf("board lacks (1 of 2):\n%s", out)
	}
	pressAsk(a, "A")
	if got := answerFile(dir, old); got != `{"behavior":"allow"}` {
		t.Errorf("older answer = %q", got)
	}
	if got := answerFile(dir, newer); got != "" {
		t.Errorf("newer answered too: %q", got)
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "allowed Bash: first") {
		t.Errorf("footer after A:\n%s", out)
	}
}

func TestDenyWithReason(t *testing.T) {
	a, dir := boardApp(t)
	r := askFor(t, dir, "done", "npm test", time.Now())
	runAsks(a, time.Now())
	pressAsk(a, "D")
	pressAsk(a, "q") // typed into the reason, not quitting
	a.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	for _, k := range "use make" {
		pressAsk(a, string(k))
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "deny reason") {
		t.Errorf("no reason input:\n%s", out)
	}
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.Update(cmd())
	if got := answerFile(dir, r); got != `{"behavior":"deny","message":"use make"}` {
		t.Errorf("answer = %q", got)
	}

	r2 := askFor(t, dir, "done", "ls", time.Now())
	runAsks(a, time.Now())
	pressAsk(a, "D")
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := answerFile(dir, r2); got != "" || a.ans.typing {
		t.Errorf("esc still answered: %q", got)
	}
}

func TestLateAnswerSaysAlreadyAnswered(t *testing.T) {
	a, dir := boardApp(t)
	r := askFor(t, dir, "done", "npm test", time.Now())
	runAsks(a, time.Now())
	os.Remove(filepath.Join(dir, "asks", "done", r.ID+".json")) // given up on meanwhile
	pressAsk(a, "A")
	if out := ansi.Strip(a.View()); !strings.Contains(out, "already answered") {
		t.Errorf("footer:\n%s", out)
	}
}

func TestAInSessionView(t *testing.T) {
	a, dir := askApp(t)
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	p := filepath.Join(t.TempDir(), "s1.jsonl")
	os.WriteFile(p, []byte(fixture), 0o644)
	openNow(a, openMsg{path: p})
	t.Cleanup(func() {
		for a.top() != nil {
			a.pop()
		}
	})
	r := askFor(t, dir, "s1", "npm test", time.Now())
	runAsks(a, time.Now())
	if out := ansi.Strip(a.View()); !strings.Contains(out, "? Bash: npm test") || !strings.Contains(out, "A allow once") {
		t.Errorf("session footer lacks the request:\n%s", out)
	}
	// In the search, A is a letter.
	pressAsk(a, "/")
	pressAsk(a, "A")
	if got := answerFile(dir, r); got != "" {
		t.Errorf("A in the search answered: %q", got)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	pressAsk(a, "A")
	if got := answerFile(dir, r); got != `{"behavior":"allow"}` {
		t.Errorf("answer = %q", got)
	}
}

func TestNoAskNoEffect(t *testing.T) {
	a, dir := boardApp(t)
	runAsks(a, time.Now())
	pressAsk(a, "A")
	pressAsk(a, "D")
	if a.ans.typing {
		t.Error("D opened the reason with nothing asked")
	}
	if es, _ := os.ReadDir(filepath.Join(dir, "asks")); len(es) != 0 {
		t.Errorf("wrote %d entries", len(es))
	}
}

func TestAskAlertOnBoard(t *testing.T) {
	a, dir := boardApp(t)
	askFor(t, dir, "done", "npm test", time.Now())
	runAsks(a, time.Now())
	b := a.board
	if b.alert == nil || !strings.HasPrefix(b.alert.text, "api: ? Bash: npm test") || b.alert.kind != alertAsk {
		t.Fatalf("alert = %+v", b.alert)
	}
	b.alert = nil
	runAsks(a, time.Now())
	if b.alert != nil {
		t.Errorf("the same request alerted again: %+v", b.alert)
	}
}

// A session whose last Bash call has no result yet.
const askFixture = `{"type":"user","uuid":"u1","cwd":"/w/api","timestamp":"2026-10-24T10:00:00Z","message":{"content":"run the tests"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"npm test"}}]}}
`

func TestAskAlertInSessionView(t *testing.T) {
	a, dir := askApp(t)
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	p := filepath.Join(t.TempDir(), "s1.jsonl")
	os.WriteFile(p, []byte(askFixture), 0o644)
	openNow(a, openMsg{path: p})
	t.Cleanup(func() {
		for a.top() != nil {
			a.pop()
		}
	})
	askFor(t, dir, "s1", "npm test", time.Now())
	runAsks(a, time.Now())
	v := a.top()
	if v.alert == nil || v.alert.text != "? Bash: npm test" || v.alert.ev == nil || v.alert.ev.Tool.ID != "t1" {
		t.Fatalf("alert = %+v", v.alert)
	}
	v.cursor = 0
	a.Update(key("!"))
	if sel := v.selected(); sel == nil || sel.Tool == nil || sel.Tool.ID != "t1" {
		t.Errorf("! selected %+v", sel)
	}
}

func TestAskNotifies(t *testing.T) {
	s := &sent{}
	a, dir := askApp(t)
	a.opts.Notifier, a.opts.CacheDir, a.opts.BellOn = s, t.TempDir(), map[string]bool{alertAsk: true}
	b, _ := newTestBoard(t)
	a.board = b
	askFor(t, dir, "done", "npm test", time.Now())
	for _, c := range a.askCmds(time.Now()) {
		if m, ok := c().(asksMsg); ok {
			_, cmd := a.Update(m)
			runNote(cmd)
		}
	}
	if len(s.notes) != 1 || s.notes[0].Body != "Bash wants to run npm test" || s.notes[0].Title != "api · fix the flaky test" {
		t.Errorf("notes = %+v", s.notes)
	}
}
