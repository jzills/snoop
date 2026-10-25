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

// run feeds a command's messages back into the app until it settles.
func run(a *App, cmd tea.Cmd) {
	for i := 0; cmd != nil && i < 1000; i++ {
		msg := cmd()
		if msg == nil {
			return
		}
		_, cmd = a.Update(msg)
	}
}

func TestPickerSummary(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-repo")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte(fixture), 0o644)
	os.WriteFile(filepath.Join(proj, "s2.jsonl"), []byte(groupFixture), 0o644)
	infos, _ := store.List(root)

	a := New(Options{Root: root, CacheDir: filepath.Join(t.TempDir(), "cache")})
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	a.Update(sessionsMsg{infos: infos})
	_, cmd := a.Update(key("s"))
	if !a.picker.summary || cmd == nil {
		t.Fatal("s did not open the summary and start a scan")
	}
	run(a, cmd)
	if a.picker.scanning || len(a.picker.sums) != 2 {
		t.Fatalf("scanning = %v, sums = %d", a.picker.scanning, len(a.picker.sums))
	}
	out := ansi.Strip(a.View())
	for _, want := range []string{"Summary of 2 sessions", "Sessions    2", "Tools", "Bash"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(a.opts.CacheDir, "summaries.json")); err != nil {
		t.Errorf("cache not written: %v", err)
	}
	// esc goes back to the list rather than quitting.
	if _, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEsc}); a.picker.summary || cmd != nil {
		t.Errorf("esc: summary = %v, cmd = %v", a.picker.summary, cmd)
	}
}

func TestQuittingMidScanKeepsTheCache(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-repo")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte(fixture), 0o644)
	os.WriteFile(filepath.Join(proj, "s2.jsonl"), []byte(groupFixture), 0o644)
	infos, _ := store.List(root)
	cache := filepath.Join(t.TempDir(), "cache")

	a := New(Options{Root: root, CacheDir: cache})
	a.Update(sessionsMsg{infos: infos})
	_, cmd := a.Update(key("s"))
	// Take one result, then quit before the scan finishes.
	_, cmd = a.Update(cmd())
	a.Update(cmd())
	if a.picker.scanned != 1 || !a.picker.scanning {
		t.Fatalf("scanned %d, scanning %v", a.picker.scanned, a.picker.scanning)
	}
	a.Close()
	b, err := os.ReadFile(filepath.Join(cache, "summaries.json"))
	if err != nil || !strings.Contains(string(b), "s1.jsonl") && !strings.Contains(string(b), "s2.jsonl") {
		t.Errorf("cache after quitting mid-scan: %v %.200s", err, b)
	}
}
