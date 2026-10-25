package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
)

const (
	bPrompt    = `{"type":"user","uuid":"p1","timestamp":"2026-10-25T10:00:00Z","cwd":"/src/api","message":{"role":"user","content":"fix the flaky test"}}`
	bBash      = `{"type":"assistant","uuid":"a1","timestamp":"2026-10-25T10:00:05Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}`
	bBashDone  = `{"type":"user","uuid":"r1","timestamp":"2026-10-25T10:00:07Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`
	bAgent     = `{"type":"assistant","uuid":"a2","timestamp":"2026-10-25T10:00:05Z","message":{"id":"m2","content":[{"type":"tool_use","id":"t2","name":"Agent","input":{"subagent_type":"Explore","description":"map the parser"}}]}}`
	bTurnEnd   = `{"type":"system","subtype":"turn_duration","uuid":"d1","timestamp":"2026-10-25T10:00:09Z","durationMs":9000}`
	bInterrupt = `{"type":"user","uuid":"p2","timestamp":"2026-10-25T10:00:06Z","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}`
)

var bStart = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)

func sessOf(ls ...string) *parse.Session {
	s := parse.NewSession()
	for _, l := range ls {
		s.Feed([]byte(l))
	}
	return s
}

func TestRowState(t *testing.T) {
	stall := 30 * time.Second
	for _, tc := range []struct {
		name   string
		lines  []string
		last   time.Duration // since bStart
		now    time.Duration
		want   rowStatus
		detail string
	}{
		{"no prompts", nil, 0, time.Minute, rowIdle, "no prompts yet"},
		{"replying", []string{bPrompt}, 0, 10 * time.Second, rowWorking, "replying"},
		{"running a tool", []string{bPrompt, bBash}, 5 * time.Second, 20 * time.Second, rowWorking, "go test"},
		{"tool past the stall", []string{bPrompt, bBash}, 5 * time.Second, time.Minute, rowWaiting, "waiting 55s on Bash"},
		{"a waiting tool stays waiting however quiet", []string{bPrompt, bBash}, 5 * time.Second, 25 * time.Minute, rowWaiting, "on Bash"},
		{"subagents are never waiting", []string{bPrompt, bAgent}, 5 * time.Second, 5 * time.Minute, rowWorking, "Explore"},
		{"turn ended", []string{bPrompt, bBash, bBashDone, bTurnEnd}, 9 * time.Second, time.Minute, rowYourTurn, "turn ended"},
		{"interrupted", []string{bPrompt, bBash, bInterrupt}, 6 * time.Second, time.Minute, rowYourTurn, "interrupted"},
		{"quiet without a waiting tool", []string{bPrompt}, 0, 11 * time.Minute, rowIdle, "no activity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, detail := rowState(sessOf(tc.lines...), "", bStart.Add(tc.last), bStart.Add(tc.now), stall)
			if st != tc.want || !strings.Contains(detail, tc.detail) {
				t.Errorf("state %v %q, want %v containing %q", st, detail, tc.want, tc.detail)
			}
		})
	}
}

func TestForkedSkillRowIsWorking(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	skill := `{"type":"assistant","uuid":"a3","timestamp":"2026-10-25T10:00:05Z","message":{"id":"m3","content":[{"type":"tool_use","id":"t3","name":"Skill","input":{"skill":"review"}}]}}`
	s := sessOf(bPrompt, skill)
	if st, _ := rowState(s, p, bStart, bStart.Add(time.Minute), 30*time.Second); st != rowWaiting {
		t.Fatalf("a Skill with no subagent = %v, want waiting", st)
	}
	writeSubagent(t, p, "f1")
	if st, d := rowState(s, p, bStart, bStart.Add(time.Minute), 30*time.Second); st != rowWorking {
		t.Errorf("a forked Skill = %v %q, want working", st, d)
	}
}

// newTestBoard seeds two live sessions and one old one under a temporary
// root and loads the board the way the app does.
func newTestBoard(t *testing.T) (*board, string) {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "-src-api")
	os.MkdirAll(proj, 0o755)
	now := time.Now()
	write := func(name string, at time.Time, ls ...string) {
		p := filepath.Join(proj, name)
		os.WriteFile(p, []byte(strings.Join(ls, "\n")+"\n"), 0o644)
		os.Chtimes(p, at, at)
	}
	write("busy.jsonl", now.Add(-5*time.Second), bPrompt, bBash)
	write("done.jsonl", now.Add(-time.Minute), bPrompt, bBash, bBashDone, bTurnEnd)
	write("old.jsonl", now.Add(-2*time.Hour), bPrompt)
	b := newBoard(root)
	loadBoard(t, b, now)
	return b, proj
}

// loadBoard runs one discovery and every load it starts, synchronously.
func loadBoard(t *testing.T, b *board, now time.Time) {
	t.Helper()
	msg := b.scanCmd(now)().(boardScanMsg)
	for _, cmd := range b.onScan(msg) {
		b.onLoaded(cmd().(boardLoadedMsg))
	}
}

func TestBoardListsOnlyLiveSessions(t *testing.T) {
	b, _ := newTestBoard(t)
	if len(b.rows) != 2 {
		t.Fatalf("rows = %d, want busy and done (old is outside the window)", len(b.rows))
	}
	out := ansi.Strip(b.view(120, 20, time.Now(), 30*time.Second))
	// busy.jsonl's Bash call has had no result since 2026, so it waits on you.
	for _, want := range []string{"Live", "2 sessions", "1 waiting on you", "your turn", "on Bash", "api", "fix the flaky test"} {
		if !strings.Contains(out, want) {
			t.Errorf("board view lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "◌ waiting") > strings.Index(out, "✓ your turn") {
		t.Errorf("waiting should sort above your turn:\n%s", out)
	}
}

func TestBoardAlertsOnNewLinesOnly(t *testing.T) {
	b, proj := newTestBoard(t)
	never := func(string) bool { return false }
	if as := b.poll(time.Now(), 0, never); len(as) != 0 {
		t.Fatalf("alerts for lines already there: %+v", as)
	}
	f, _ := os.OpenFile(filepath.Join(proj, "busy.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(bBashDone + "\n" + bTurnEnd + "\n")
	f.Close()
	as := b.poll(time.Now(), 0, never)
	if len(as) != 1 || as[0].kind != alertTurn || !strings.HasPrefix(as[0].text, "api: ") {
		t.Fatalf("alerts = %+v, want one project-prefixed turn alert", as)
	}
	// A session also open in a view raises nothing on the board.
	f, _ = os.OpenFile(filepath.Join(proj, "busy.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(strings.ReplaceAll(bPrompt, "p1", "p3") + "\n" + strings.ReplaceAll(bTurnEnd, "d1", "d3") + "\n")
	f.Close()
	open := func(p string) bool { return strings.HasSuffix(p, "busy.jsonl") }
	if as := b.poll(time.Now(), 0, open); len(as) != 0 {
		t.Errorf("open session alerted on the board: %+v", as)
	}
}

func TestBoardNoStallAlertForCallsPendingAtOpen(t *testing.T) {
	b, _ := newTestBoard(t)
	never := func(string) bool { return false }
	// busy.jsonl's Bash call started at 10:00:05 on 2026-10-25, long ago.
	as := b.poll(time.Now(), 30*time.Second, never)
	if len(as) != 0 {
		t.Fatalf("calls already pending when the board opened alerted: %+v", as)
	}
}

func TestBoardDropsSessionsOutsideTheWindow(t *testing.T) {
	b, proj := newTestBoard(t)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(proj, "done.jsonl"), old, old)
	loadBoard(t, b, time.Now())
	if len(b.rows) != 1 {
		t.Errorf("rows = %d after done.jsonl went quiet, want 1", len(b.rows))
	}
}

func TestBoardEnterOpensTheSelectedSession(t *testing.T) {
	b, _ := newTestBoard(t)
	b.view(120, 20, time.Now(), 30*time.Second) // orders the rows
	cmd := b.update(tea.KeyMsg{Type: tea.KeyEnter}, 20)
	if cmd == nil {
		t.Fatal("enter did nothing")
	}
	msg, ok := cmd().(openMsg)
	if !ok || !strings.HasSuffix(msg.path, "busy.jsonl") {
		t.Errorf("enter opened %+v, want the top row (busy.jsonl, waiting)", msg)
	}
}

func TestLiveBoardFromThePicker(t *testing.T) {
	b, _ := newTestBoard(t)
	a := New(Options{Root: b.root})
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.Update(sessionsMsg{infos: []store.SessionInfo{}})
	_, cmd := a.Update(key("w"))
	run(a, cmd)
	if a.board == nil {
		t.Fatal("w did not open the live board")
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "Live") {
		t.Errorf("view after w:\n%s", out)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.board != nil {
		t.Error("esc did not leave the board")
	}
}

func TestEscFromASessionReturnsToTheBoard(t *testing.T) {
	b, _ := newTestBoard(t)
	a := New(Options{Root: b.root})
	a.board = b
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.View()
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	openNow(a, cmd().(openMsg)) // what the open returns waits on the tail
	if a.top() == nil {
		t.Fatal("enter did not open a session")
	}
	t.Cleanup(func() {
		for a.top() != nil {
			a.pop()
		}
	})
	_, cmd = a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.top() != nil || a.board == nil || cmd != nil {
		t.Fatalf("after esc: top %v, board %v, cmd %v", a.top(), a.board != nil, cmd != nil)
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "Live") {
		t.Errorf("esc did not show the board again:\n%s", out)
	}
}
