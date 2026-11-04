package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/review"
)

// replyBoard is an app on a live board with one session whose turn has
// ended but isn't recorded yet, as while its Stop hook waits, and one done.
func replyBoard(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	root := t.TempDir()
	proj := filepath.Join(root, "-src-api")
	os.MkdirAll(proj, 0o755)
	now := time.Now()
	reply := `{"type":"assistant","uuid":"a9","timestamp":"2026-10-25T10:00:09Z","message":{"id":"m9","content":[{"type":"text","text":"Done. Shall I push?"}]}}`
	for name, ls := range map[string][]string{
		"ending.jsonl": {bPrompt, bBash, bBashDone, reply},
		"done.jsonl":   {bPrompt, bBash, bBashDone, bTurnEnd},
	} {
		p := filepath.Join(proj, name)
		os.WriteFile(p, []byte(strings.Join(ls, "\n")+"\n"), 0o644)
		os.Chtimes(p, now.Add(-10*time.Second), now.Add(-10*time.Second))
	}
	a := New(Options{Root: root, AsksDir: filepath.Join(dir, "asks"), ListeningFile: filepath.Join(dir, "listening"),
		InboxDir: filepath.Join(dir, "inbox"), TurnsDir: filepath.Join(dir, "turns")})
	a.board = newBoard(root)
	loadBoard(t, a.board, now)
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return a, dir
}

// waitTurn marks a session's turn as waiting for a reply, as its Stop hook
// does, under this process's PID so it counts as alive.
func waitTurn(t *testing.T, dir, session string, ended time.Time) {
	t.Helper()
	d := filepath.Join(dir, "turns", session)
	os.MkdirAll(d, 0o700)
	b, _ := json.Marshal(review.Turn{Session: session, Cwd: "/w/api", Last: "Done. Shall I push?", Ended: ended, Deadline: ended.Add(time.Minute)})
	if err := os.WriteFile(filepath.Join(d, fmt.Sprintf("%020d-%d.json", ended.UnixNano(), os.Getpid())), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// inboxText is everything waiting in a session's inbox.
func inboxText(dir, session string) string {
	es, _ := os.ReadDir(filepath.Join(dir, "inbox", session))
	var out []string
	for _, e := range es {
		b, _ := os.ReadFile(filepath.Join(dir, "inbox", session, e.Name()))
		out = append(out, string(b))
	}
	return strings.Join(out, "|")
}

// typeReply presses R, types text and presses enter, running the command.
func typeReply(a *App, text string) {
	pressAsk(a, "R")
	for _, k := range text {
		pressAsk(a, string(k))
	}
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		a.Update(cmd())
	}
}

// selectRow moves the board's cursor to a session.
func selectRow(t *testing.T, a *App, id string) {
	t.Helper()
	a.View()
	for i, r := range a.board.order {
		if r.info.ID == id {
			a.board.cursor = i
			return
		}
	}
	t.Fatalf("no row %s", id)
}

func TestReplyToAWaitingTurn(t *testing.T) {
	a, dir := replyBoard(t)
	if out := ansi.Strip(a.View()); strings.Count(out, "your turn") != 1 {
		t.Fatalf("before the hook waits, only done has ended:\n%s", out)
	}
	waitTurn(t, dir, "ending", time.Now().Add(-2*time.Second))
	runAsks(a, time.Now())
	b := a.board
	if b.alert == nil || b.alert.kind != alertTurn || !strings.Contains(b.alert.note, "Shall I push?") {
		t.Fatalf("alert = %+v", b.alert)
	}
	selectRow(t, a, "ending")
	out := ansi.Strip(a.View())
	for _, want := range []string{"turn ended", "R replies", "R reply"} {
		if !strings.Contains(out, want) {
			t.Errorf("board lacks %q:\n%s", want, out)
		}
	}
	typeReply(a, "yes, push")
	if got := inboxText(dir, "ending"); got != "Message from the user, sent from snoop:\n\nyes, push\n" {
		t.Errorf("inbox = %q", got)
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "sent; the agent has it now") {
		t.Errorf("footer:\n%s", out)
	}

	// The same turn doesn't alert again.
	b.alert = nil
	runAsks(a, time.Now())
	if b.alert != nil {
		t.Errorf("alerted again: %+v", b.alert)
	}
}

// A reply to a session that isn't waiting is queued for its next hook.
func TestReplyQueues(t *testing.T) {
	a, dir := replyBoard(t)
	runAsks(a, time.Now())
	selectRow(t, a, "done")
	typeReply(a, "run the linter too")
	if got := inboxText(dir, "done"); !strings.HasSuffix(got, "run the linter too\n") {
		t.Errorf("inbox = %q", got)
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "after its current tool call, or with your next prompt") {
		t.Errorf("footer:\n%s", out)
	}
}

func TestReplyCancelled(t *testing.T) {
	a, dir := replyBoard(t)
	selectRow(t, a, "done")
	typeReply(a, "   ")
	pressAsk(a, "R")
	pressAsk(a, "q") // typed, not quitting
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.ans.typing || inboxText(dir, "done") != "" {
		t.Errorf("an empty or cancelled reply was sent: %q", inboxText(dir, "done"))
	}
}

func TestReplyFooterInSessionView(t *testing.T) {
	a, dir := replyBoard(t)
	p := ""
	for _, r := range a.board.rows {
		if r.info.ID == "ending" {
			p = r.f.Path
		}
	}
	openNow(a, openMsg{path: p})
	t.Cleanup(func() {
		for a.top() != nil {
			a.pop()
		}
	})
	waitTurn(t, dir, "ending", time.Now())
	runAsks(a, time.Now())
	v := a.top()
	if v.alert == nil || v.alert.kind != alertTurn {
		t.Errorf("alert = %+v", v.alert)
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "✓ your turn   R reply   Done. Shall I push?") {
		t.Errorf("footer:\n%s", out)
	}
	typeReply(a, "go")
	if got := inboxText(dir, "ending"); !strings.HasSuffix(got, "\n\ngo\n") {
		t.Errorf("inbox = %q", got)
	}
}

// Once a turn's end was alerted while its hook waited, its recording
// later doesn't alert again.
func TestTurnRecordedAfterTheWaitDoesntAlert(t *testing.T) {
	w := newWatch("")
	s := sessOf(bPrompt, bBash, bBashDone)
	before := w.snap(s)
	w.endedEarly = s.Turns[0]
	s.Turns[0].Done = true
	for _, al := range w.alerts(s, before) {
		if al.kind == alertTurn {
			t.Errorf("alerted again: %+v", al)
		}
	}
	other := &parse.Turn{Done: true, Events: s.Turns[0].Events}
	s.Turns = append(s.Turns, other)
	found := false
	for _, al := range w.alerts(s, before) {
		found = found || al.kind == alertTurn
	}
	if !found {
		t.Error("a later turn didn't alert")
	}
}
