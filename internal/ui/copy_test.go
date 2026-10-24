package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
)

func TestCopyText(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(filesFixture))
	byID := func(id string) *parse.Event {
		for _, e := range s.Events {
			if e.Tool != nil && e.Tool.ID == id {
				return e
			}
		}
		t.Fatalf("no %s", id)
		return nil
	}
	cases := map[string]string{
		"t1": "ok",                      // Read: the result
		"t2": "- x\n- y\n+ x\n+ z\n+ w", // Edit: the diff
		"t3": "1\n2\n3\n",               // Write: the content
	}
	for id, want := range cases {
		if got := copyText(byID(id)); got != want {
			t.Errorf("%s: copied %q, want %q", id, got, want)
		}
	}
	if got := copyText(s.Events[0]); got != "go" {
		t.Errorf("prompt copied %q", got)
	}
}

func TestCopyFlashClearsOnKey(t *testing.T) {
	a := newTestApp(t, 120, 40)
	_, cmd := a.Update(key("y"))
	v := a.top()
	if cmd == nil || !strings.Contains(ansi.Strip(v.footer(120)), "copied") {
		t.Fatalf("y gave cmd %v, footer %q", cmd, ansi.Strip(v.footer(120)))
	}
	a.Update(key("k"))
	if v.flash != "" {
		t.Error("flash survived a key")
	}
}

func TestExportViewWritesShownEvents(t *testing.T) {
	t.Chdir(t.TempDir())
	v, _ := newInlineView(t)
	v.expanded["sub1"] = true
	v.refresh()
	v.exportView(time.Date(2026, 10, 24, 9, 5, 0, 0, time.Local))
	if !strings.Contains(ansi.Strip(v.flash), "exported 5 events") {
		t.Fatalf("flash = %q", ansi.Strip(v.flash))
	}
	b, err := os.ReadFile(v.sess.ID[:min(8, len(v.sess.ID))] + "-20261024-0905.md")
	if err != nil {
		t.Fatal(err)
	}
	if md := string(b); !strings.Contains(md, "> ## find the zebra") || !strings.Contains(md, "All done.") {
		t.Errorf("export lacks the subagent prompt or reply:\n%s", md)
	}
}
