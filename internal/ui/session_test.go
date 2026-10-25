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
	"github.com/jzills/snoop/internal/stats"
	"github.com/jzills/snoop/internal/store"
	"github.com/jzills/snoop/internal/tail"
)

const fixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-24T10:00:00Z","message":{"content":"run the tests"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"` + "a\\nb\\nc\\nd\\ne\\nf\\ng\\nh\\ni\\nj\\nk\\nl\\nm\\nn\\no\\np\\nq\\nr\\ns\\nt" + `"}]}}
{"type":"assistant","uuid":"a2","message":{"id":"m2","content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"x"}}]}}
{"type":"user","uuid":"r2","timestamp":"2026-10-24T10:00:03Z","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":"ok"}]}}
{"type":"assistant","uuid":"a3","timestamp":"2026-10-24T10:00:04Z","message":{"id":"m3","content":[{"type":"text","text":"All done."}]}}
`

func newTestView(t *testing.T) *view {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := openView(p, p, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.close)
	return v
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestSearchDoesNotMatchEveryFinishedTool(t *testing.T) {
	v := newTestView(t)
	v.search.SetValue("done")
	v.refresh()
	if len(v.shown) != 1 || v.shown[0].Kind != parse.KindText {
		t.Fatalf("search 'done' matched %d events, want only the reply", len(v.shown))
	}
}

func TestMoveOnEmptyListKeepsFollow(t *testing.T) {
	v := newTestView(t)
	v.kind = len(kindFilters) - 1 // errors: none in the fixture
	v.refresh()
	v.update(key("j"), 120, 30)
	if !v.follow {
		t.Fatal("pressing j on an empty list turned follow off")
	}
}

func TestToolDurIgnoresMissingTimestamp(t *testing.T) {
	v := newTestView(t)
	read := v.sess.Tool("t2")
	var e *parse.Event
	for _, x := range v.sess.Events {
		if x.Tool == read {
			e = x
		}
	}
	if d := e.Duration(); d != 0 {
		t.Fatalf("duration with missing start time = %v, want 0", d)
	}
	if row := renderRow(e, false, 120); strings.Contains(row, "h") && strings.Contains(row, "m") && strings.Contains(row, "0000") {
		t.Fatalf("row shows a bogus duration: %q", row)
	}
	bash := v.sess.Events[1]
	if d := bash.Duration(); d != time.Second {
		t.Fatalf("bash duration = %v, want 1s", d)
	}
}

func TestDetailScrollSurvivesRefresh(t *testing.T) {
	v := newTestView(t)
	v.follow = false
	v.cursor = 1 // the Bash call with a 20-line result
	v.detailPane(60, 5)
	v.detail.ScrollDown(4)
	before := v.detail.YOffset
	if before == 0 {
		t.Fatal("detail did not scroll")
	}
	// A live update and a search keystroke both refresh the view.
	v.onTail(tail.Update{Lines: [][]byte{[]byte(`{"type":"system","content":"tick"}`)}})
	v.refresh()
	v.detailPane(60, 5)
	if v.detail.YOffset != before {
		t.Fatalf("detail scroll reset to %d after refresh, want %d", v.detail.YOffset, before)
	}
	// Selecting another event starts at the top.
	v.cursor = 0
	v.detailPane(60, 5)
	if v.detail.YOffset != 0 {
		t.Fatalf("new selection kept offset %d", v.detail.YOffset)
	}
}

func TestMixSegments(t *testing.T) {
	cases := []struct {
		by   [4]int
		w    int
		want [4]int
	}{
		{[4]int{1, 1, 1, 0}, 10, [4]int{4, 3, 3, 0}},
		{[4]int{100, 1, 0, 0}, 10, [4]int{9, 1, 0, 0}},
		{[4]int{0, 0, 0, 0}, 10, [4]int{}},
	}
	for _, c := range cases {
		got := mixSegments(c.by, c.w)
		sum := 0
		for _, s := range got {
			sum += s
		}
		if got != c.want || (sum != c.w && sum != 0) {
			t.Errorf("mixSegments(%v, %d) = %v, want %v", c.by, c.w, got, c.want)
		}
	}
}

func TestOpenErrorKeepsScreen(t *testing.T) {
	a := New(Options{Root: t.TempDir()})
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	a.Update(sessionsMsg{})
	openNow(a, openMsg{path: filepath.Join(t.TempDir(), "missing.jsonl")})
	out := a.View()
	if !strings.Contains(out, "snoop") || !strings.Contains(out, glyphErr) {
		t.Fatalf("error should show in the footer over the picker:\n%s", out)
	}
	a.Update(key("j"))
	if strings.Contains(a.View(), glyphErr) {
		t.Fatal("error not cleared by the next key")
	}
}

func TestPickerEscClearsAppliedSearch(t *testing.T) {
	a := New(Options{Root: t.TempDir()})
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	a.Update(sessionsMsg{})
	a.Update(key("/"))
	a.Update(key("f"))
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.picker.input.Value() != "f" {
		t.Fatalf("search = %q", a.picker.input.Value())
	}
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.picker.input.Value() != "" {
		t.Fatal("esc did not clear the applied search")
	}
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("esc quit instead of clearing the search")
		}
	}
}

func TestModelOrderIsStable(t *testing.T) {
	s := parse.NewSession()
	s.Models = map[string]int{"model-b": 5, "model-a": 5, "model-c": 5, "model-d": 1}
	first := renderStats(s, stats.Stats{}, 100)
	for i := 0; i < 20; i++ {
		if renderStats(s, stats.Stats{}, 100) != first {
			t.Fatal("model order changed between renders")
		}
	}
	if a, b := strings.Index(first, "model-a"), strings.Index(first, "model-b"); a > b {
		t.Fatal("ties not sorted by name")
	}
}

func TestSubagentStatusRefreshes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	os.WriteFile(p, []byte(`{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:00Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Agent","input":{"description":"explore"}}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:01Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"launched"}]},"toolUseResult":{"agentId":"x1"}}
`), 0o644)
	sub := filepath.Join(dir, "s", "subagents")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "agent-x1.jsonl"), nil, 0o644)
	meta := filepath.Join(sub, "agent-x1.meta.json")
	os.WriteFile(meta, []byte(`{"agentType":"Explore","finished":false}`), 0o644)

	v, err := openView(p, p, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.close()
	v.follow, v.cursor = false, 0
	if out := v.detailPane(80, 30); !strings.Contains(out, "running") {
		t.Fatalf("want running:\n%s", out)
	}
	os.WriteFile(meta, []byte(`{"agentType":"Explore","finished":true}`), 0o644)
	if out := v.detailPane(80, 30); strings.Contains(out, "finished") {
		t.Fatal("meta.json re-read on a plain redraw; it should be cached")
	}
	// A finishing background agent writes a notification to the parent.
	v.onTail(tail.Update{Lines: [][]byte{[]byte(`{"type":"system","content":"agent done"}`)}})
	if out := v.detailPane(80, 30); !strings.Contains(out, "finished") {
		t.Fatalf("status not refreshed:\n%s", out)
	}
}

func TestTruncationReloadsSession(t *testing.T) {
	v := newTestView(t)
	v.onTail(tail.Update{Reset: true, Lines: [][]byte{[]byte(`{"type":"user","uuid":"n1","message":{"content":"fresh start"}}`)}})
	if len(v.sess.Events) != 1 || v.sess.Events[0].Text != "fresh start" {
		t.Fatalf("events after reset = %d", len(v.sess.Events))
	}
}

func TestScrollOffset(t *testing.T) {
	cases := []struct{ offset, cursor, n, h, want int }{
		{0, 0, 100, 10, 0},
		{0, 15, 100, 10, 6},  // cursor below the window
		{20, 5, 100, 10, 5},  // cursor above the window
		{40, 44, 45, 10, 35}, // list shrank: fill the window
		{40, 2, 3, 10, 0},    // fewer rows than the window
	}
	for _, c := range cases {
		if got := scrollOffset(c.offset, c.cursor, c.n, c.h); got != c.want {
			t.Errorf("scrollOffset(%d, %d, %d, %d) = %d, want %d", c.offset, c.cursor, c.n, c.h, got, c.want)
		}
	}
}

func TestSearchShowsAllMatches(t *testing.T) {
	v := newTestView(t)
	v.offset = 4 // scrolled down before searching
	v.search.SetValue("t")
	v.refresh()
	out := v.timeline(80, 10)
	rows := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			rows++
		}
	}
	if rows != len(v.shown) || rows < 2 {
		t.Fatalf("drew %d rows for %d matches", rows, len(v.shown))
	}
}

func TestPickerShowsFullOldDates(t *testing.T) {
	old := time.Now().AddDate(0, -3, 0)
	p := picker{all: []store.SessionInfo{{ID: "x", Title: "old session", ModTime: old}}, loaded: true}
	p.refilter()
	if out := p.view(120, 10); !strings.Contains(out, old.Format("Jan 02")) {
		t.Fatalf("old date cut off:\n%s", out)
	}
}

func TestOpenRunsInTheBackground(t *testing.T) {
	a := newTestApp(t, 120, 30)
	for a.top() != nil {
		a.pop()
	}
	p := filepath.Join(t.TempDir(), "s.jsonl")
	os.WriteFile(p, []byte(fixture), 0o644)
	_, cmd := a.Update(openMsg{path: p})
	if a.top() != nil || cmd == nil {
		t.Fatal("open parsed the session inside Update")
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "opening s.jsonl") {
		t.Errorf("no opening progress in the footer:\n%s", out)
	}
	a.Update(cmd())
	if a.top() == nil || a.loading != "" {
		t.Fatalf("the parsed session was not shown (loading %q)", a.loading)
	}
}

func TestEscCancelsAnOpen(t *testing.T) {
	a := newTestApp(t, 120, 30)
	for a.top() != nil {
		a.pop()
	}
	p := filepath.Join(t.TempDir(), "s.jsonl")
	os.WriteFile(p, []byte(fixture), 0o644)
	_, cmd := a.Update(openMsg{path: p})
	_, quit := a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if quit != nil {
		t.Fatal("esc while opening did more than cancel")
	}
	a.Update(cmd())
	if a.top() != nil {
		t.Error("a cancelled open was shown")
	}
}

func TestOnlyTheLatestOpenIsShown(t *testing.T) {
	a := newTestApp(t, 120, 30)
	for a.top() != nil {
		a.pop()
	}
	dir := t.TempDir()
	p1, p2 := filepath.Join(dir, "one.jsonl"), filepath.Join(dir, "two.jsonl")
	os.WriteFile(p1, []byte(fixture), 0o644)
	os.WriteFile(p2, []byte(fixture), 0o644)
	_, c1 := a.Update(openMsg{path: p1})
	_, c2 := a.Update(openMsg{path: p2})
	a.Update(c2())
	a.Update(c1())
	if len(a.stack) != 1 || a.top().path != p2 {
		t.Errorf("stack = %d views, top %v", len(a.stack), a.top())
	}
}
