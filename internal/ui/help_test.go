package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestApp(t *testing.T, w, h int) *App {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	os.WriteFile(p, []byte(groupFixture), 0o644)
	a := New(Options{Root: t.TempDir()})
	a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	a.Update(sessionsMsg{})
	a.Update(openMsg{path: p})
	t.Cleanup(func() {
		for a.top() != nil {
			a.pop()
		}
	})
	return a
}

func TestHelpOverlayPerScreen(t *testing.T) {
	a := newTestApp(t, 120, 60)
	cases := []struct{ tab, want string }{
		{"1", "Search and filter"},
		{"2", "expand everything"},
		{"3", "Scroll"},
	}
	for _, c := range cases {
		a.Update(key(c.tab))
		a.Update(key("?"))
		if out := a.View(); !strings.Contains(out, c.want) {
			t.Errorf("tab %s help lacks %q:\n%s", c.tab, c.want, out)
		}
		_, cmd := a.Update(key("q")) // closes help, does not quit
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatal("q quit the app instead of closing help")
			}
		}
		if a.showHelp || a.top() == nil {
			t.Fatal("help still open, or the session was closed")
		}
	}
}

func TestHelpKeysDoNotLeak(t *testing.T) {
	a := newTestApp(t, 120, 40)
	v := a.top()
	before := v.cursor
	a.Update(key("?"))
	a.Update(key("g")) // would move the timeline cursor if it leaked
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if v.cursor != before || a.showHelp || a.top() != v {
		t.Fatalf("cursor %d→%d, help %v, view changed %v", before, v.cursor, a.showHelp, a.top() != v)
	}
}

func TestQuestionMarkTypesWhileSearching(t *testing.T) {
	a := newTestApp(t, 120, 40)
	a.Update(key("/"))
	a.Update(key("?"))
	if a.showHelp || a.top().search.Value() != "?" {
		t.Fatalf("help %v, search %q; want ? typed into the search", a.showHelp, a.top().search.Value())
	}
}

func TestPickerHelp(t *testing.T) {
	a := New(Options{Root: t.TempDir()})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a.Update(sessionsMsg{})
	a.Update(key("?"))
	if !strings.Contains(a.View(), "Session picker") {
		t.Fatal("picker help not shown")
	}
}

func TestFootersFit(t *testing.T) {
	a := newTestApp(t, 100, 30)
	v := a.top()
	check := func(name string) {
		lines := strings.Split(a.View(), "\n")
		if last := lines[len(lines)-1]; strings.Contains(last, "…") || !strings.Contains(last, "help") {
			t.Errorf("%s footer truncated or missing ? help: %q", name, last)
		}
	}
	check("timeline")
	v.focusR = true
	check("detail focus")
	v.focusR = false
	a.Update(key("2"))
	check("tools")
	a.Update(key("3"))
	check("stats")
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	check("picker")
}
