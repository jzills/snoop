package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/store"
)

func typeText(a *App, s string) {
	for _, r := range s {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func newFindApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "-src-api")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "tests.jsonl"), []byte(bPrompt+"\n"+bBash+"\n"+bBashDone+"\n"), 0o644)
	os.WriteFile(filepath.Join(proj, "other.jsonl"), []byte(fixture), 0o644)
	infos, _ := store.List(root)
	a := New(Options{Root: root})
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.Update(sessionsMsg{infos: infos})
	return a
}

func TestFindAcrossSessions(t *testing.T) {
	a := newFindApp(t)
	a.Update(key("f"))
	if !a.picker.find || !a.picker.filtering {
		t.Fatal("f did not start a find")
	}
	if !a.picker.scanning {
		t.Fatal("f did not start indexing the sessions")
	}
	run(a, startScan(a.picker.all, a.picker.cache))
	typeText(a, "go test")
	if len(a.picker.shown) != 1 {
		t.Fatalf("shown = %d sessions, want the one that ran go test", len(a.picker.shown))
	}
	out := ansi.Strip(a.View())
	for _, want := range []string{"$ go test ./...", "1 matching"} {
		if !strings.Contains(out, want) {
			t.Errorf("find view lacks %q:\n%s", want, out)
		}
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter}) // keep the results
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a result did nothing")
	}
	msg, ok := cmd().(openMsg)
	if !ok || !strings.HasSuffix(msg.path, "tests.jsonl") || msg.search != "go test" {
		t.Fatalf("enter = %+v, want tests.jsonl opened with the search", msg)
	}
	a.Update(msg)
	v := a.top()
	if v == nil {
		t.Fatal("the session did not open")
	}
	defer a.pop()
	if v.search.Value() != "go test" || len(v.shown) == 0 || v.shown[0].Tool == nil || v.shown[0].Tool.Name != "Bash" {
		t.Errorf("opened with search %q showing %d events", v.search.Value(), len(v.shown))
	}
}

func TestEscLeavesFind(t *testing.T) {
	a := newFindApp(t)
	a.Update(key("f"))
	typeText(a, "zzz")
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.picker.find || a.picker.input.Value() != "" || len(a.picker.shown) != 2 {
		t.Errorf("after esc: find %v, query %q, shown %d", a.picker.find, a.picker.input.Value(), len(a.picker.shown))
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "Claude Code sessions") {
		t.Errorf("esc did not return to the list:\n%s", out)
	}
}

func TestExcerptShowsTheMatch(t *testing.T) {
	long := "wc -l internal/*/*.go cmd/*/*.go && git log --oneline | head -20; sed -n 1,40p internal/parse/parse.go"
	got := excerpt(long, "parse.go", 40)
	if !strings.HasPrefix(got, "…") || !strings.Contains(ansi.Truncate(got, 40, ""), "parse.go") {
		t.Errorf("excerpt = %q, want the match within 40 cells", got)
	}
	if got := excerpt("short parse.go", "parse.go", 40); got != "short parse.go" {
		t.Errorf("short text changed: %q", got)
	}
	if got := excerpt(long, "wc", 40); got != long {
		t.Errorf("a match already in view moved: %q", got)
	}
}

func TestEscAfterAnEmptyFindDoesNotQuit(t *testing.T) {
	a := newFindApp(t)
	a.Update(key("f"))
	a.Update(tea.KeyMsg{Type: tea.KeyEnter}) // nothing typed
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("esc quit instead of leaving find")
		}
	}
	if a.picker.find {
		t.Error("esc did not leave find")
	}
}
