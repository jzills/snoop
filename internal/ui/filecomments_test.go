package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
)

// In the Files tab, c comments on the change at the top of the detail
// pane, quoting it, and S sends it with the Diff tab's comments.
func TestCommentOnAChange(t *testing.T) {
	a, v, inbox := diffApp(t, true)
	press(a, "4")
	a.View()
	if n := v.selectedFileRow(); n == nil || !strings.HasSuffix(n.key, "a.go") {
		t.Fatalf("selected %+v", n)
	}
	a.View()
	press(a, "c")
	if !v.rev.typing || !strings.Contains(v.rev.input.Placeholder, "the Edit at") {
		t.Fatalf("typing %v, placeholder %q", v.rev.typing, v.rev.input.Placeholder)
	}
	for _, k := range "keep 1" {
		a.Update(key(string(k)))
	}
	press(a, "enter")
	if len(v.rev.comments) != 1 {
		t.Fatalf("comments = %+v", v.rev.comments)
	}
	c := v.rev.comments[0]
	if !strings.HasPrefix(c.At, "the Edit at ") || len(c.Lines) != 2 || c.Lines[0].Text != "1" || c.Lines[1].Text != "2" {
		t.Errorf("comment = %+v", c)
	}
	out := ansi.Strip(a.View())
	for _, want := range []string{"●1", "1 comment"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
	// c again edits the same change's comment.
	press(a, "c")
	if v.rev.input.Value() != "keep 1" {
		t.Errorf("c on the same change = %q", v.rev.input.Value())
	}
	press(a, "esc")

	press(a, "C")
	for _, k := range "whole" {
		a.Update(key(string(k)))
	}
	press(a, "enter", "S")
	es, _ := os.ReadDir(inbox)
	if len(es) != 1 {
		t.Fatalf("inbox has %d files", len(es))
	}
	b, _ := os.ReadFile(inbox + "/" + es[0].Name())
	for _, want := range []string{"a.go (the Edit at ", "    -1\n    +2\nkeep 1", "a.go (whole file)\nwhole"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("sent text lacks %q:\n%s", want, b)
		}
	}
}

func TestChangeLinesAreCapped(t *testing.T) {
	in, _ := json.Marshal(map[string]string{"file_path": "/w/a.go", "content": strings.Repeat("line\n", 50)})
	ls := changeLines(&parse.ToolCall{Name: "Write", Input: in})
	if len(ls) != maxQuoted+1 || ls[maxQuoted].Text != "… 20 more lines" {
		t.Errorf("%d lines, last %+v", len(ls), ls[len(ls)-1])
	}
}
