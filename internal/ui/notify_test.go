package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/notify"
	"github.com/jzills/snoop/internal/parse"
)

type sent struct{ notes []notify.Note }

func (s *sent) Send(n notify.Note) error { s.notes = append(s.notes, n); return nil }
func (s *sent) Name() string             { return "test" }

func noteApp(t *testing.T) (*App, *sent) {
	t.Helper()
	s := &sent{}
	a := New(Options{Notifier: s, CacheDir: t.TempDir(), BellOn: map[string]bool{alertTurn: true, alertStall: true}})
	return a, s
}

func turnAlert(path, id string) *alert {
	return &alert{kind: alertTurn, text: "✓ your turn", note: "Your turn", path: path, title: "api · Fix tests",
		ev: &parse.Event{UUID: id}}
}

func runNote(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if b, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range b {
			runNote(c)
		}
	}
}

func TestNotificationNamesTheSession(t *testing.T) {
	a, s := noteApp(t)
	now := time.Now()
	runNote(a.notifyCmd([]*alert{{kind: alertCompact}, turnAlert("/a.jsonl", "e1")}, now))
	if len(s.notes) != 1 || s.notes[0] != (notify.Note{Title: "api · Fix tests", Body: "Your turn"}) {
		t.Fatalf("notes = %+v", s.notes)
	}
	// Kinds not chosen never notify.
	runNote(a.notifyCmd([]*alert{{kind: alertCompact, path: "/b.jsonl"}}, now))
	if len(s.notes) != 1 {
		t.Errorf("a compaction notified: %+v", s.notes)
	}
}

func TestNotificationsAreThrottledPerSessionAndKind(t *testing.T) {
	a, s := noteApp(t)
	now := time.Now()
	runNote(a.notifyCmd([]*alert{turnAlert("/a.jsonl", "e1")}, now))
	runNote(a.notifyCmd([]*alert{turnAlert("/a.jsonl", "e2")}, now.Add(20*time.Second)))
	runNote(a.notifyCmd([]*alert{turnAlert("/b.jsonl", "e3")}, now.Add(20*time.Second)))
	runNote(a.notifyCmd([]*alert{turnAlert("/a.jsonl", "e4")}, now.Add(2*time.Minute)))
	if len(s.notes) != 3 {
		t.Errorf("sent %d notes, want 3 (the second turn of /a.jsonl within a minute is dropped)", len(s.notes))
	}
}

func TestNoNotificationWhileFocused(t *testing.T) {
	a, s := noteApp(t)
	a.Update(tea.FocusMsg{})
	runNote(a.notifyCmd([]*alert{turnAlert("/a.jsonl", "e1")}, time.Now()))
	if len(s.notes) != 0 {
		t.Errorf("notified while snoop had focus: %+v", s.notes)
	}
	a.Update(tea.BlurMsg{})
	runNote(a.notifyCmd([]*alert{turnAlert("/a.jsonl", "e2")}, time.Now()))
	if len(s.notes) != 1 {
		t.Errorf("no notification after focus moved away: %+v", s.notes)
	}
}

func TestTwoSnoopsSendOneNotification(t *testing.T) {
	cache := t.TempDir()
	s1, s2 := &sent{}, &sent{}
	on := map[string]bool{alertTurn: true}
	a1 := New(Options{Notifier: s1, CacheDir: cache, BellOn: on})
	a2 := New(Options{Notifier: s2, CacheDir: cache, BellOn: on})
	now := time.Now()
	runNote(a1.notifyCmd([]*alert{turnAlert("/a.jsonl", "e1")}, now))
	runNote(a2.notifyCmd([]*alert{turnAlert("/a.jsonl", "e1")}, now))
	if len(s1.notes)+len(s2.notes) != 1 {
		t.Errorf("two snoops sent %d notes for one event", len(s1.notes)+len(s2.notes))
	}
	// A different session's event still goes out from the second.
	runNote(a2.notifyCmd([]*alert{turnAlert("/b.jsonl", "e9")}, now))
	if len(s2.notes) != 1 {
		t.Errorf("second snoop's own session did not notify")
	}
}

func TestEverySessionsAlertNotifies(t *testing.T) {
	a, s := noteApp(t)
	stall := func(path, id string) *alert {
		return &alert{kind: alertStall, note: "Waiting", path: path, title: path, ev: &parse.Event{UUID: id}}
	}
	// One board tick: two sessions stall, one of them twice.
	runNote(a.notifyCmd([]*alert{stall("/a.jsonl", "e1"), stall("/b.jsonl", "e2"), stall("/a.jsonl", "e3")}, time.Now()))
	if len(s.notes) != 2 || s.notes[0].Title != "/b.jsonl" || s.notes[1].Title != "/a.jsonl" {
		t.Errorf("notes = %+v, want one each for /b.jsonl and /a.jsonl", s.notes)
	}
}

func TestBoardAlertsCarryTheSession(t *testing.T) {
	b, proj := newTestBoard(t)
	appendLines(t, filepath.Join(proj, "busy.jsonl"), bBashDone, bTurnEnd)
	as := b.poll(time.Now(), 0, func(string) bool { return false })
	if len(as) != 1 {
		t.Fatalf("alerts = %+v", as)
	}
	a := as[0]
	if a.title != "api · fix the flaky test" || !strings.HasSuffix(a.path, "busy.jsonl") || !strings.HasPrefix(a.note, "Your turn") {
		t.Errorf("alert title %q path %q note %q", a.title, a.path, a.note)
	}
}

func appendLines(t *testing.T, path string, ls ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(strings.Join(ls, "\n") + "\n")
}

func TestNoteTitle(t *testing.T) {
	if got := noteTitle("api", "fix\n the   tests"); got != "api · fix the tests" {
		t.Errorf("noteTitle = %q", got)
	}
	long := strings.Repeat("x", 80)
	if got := noteTitle("", long); len([]rune(got)) != 60 || !strings.HasSuffix(got, "…") {
		t.Errorf("long title = %q", got)
	}
}
