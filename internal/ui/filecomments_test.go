package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/review"
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

// A session run in a subdirectory: the Diff tab's paths are relative to
// the repository and the Files tab's to the session's directory, and a
// comment from either is the same file's.
func TestCommentPathsAgreeAcrossTabs(t *testing.T) {
	v := newTestView(t)
	v.sess.CWD = "/repo/app"
	v.rev.diff = &review.Diff{Root: "/repo"}
	if a, b := v.diffAbs("app/main.go"), v.filesAbs("main.go"); a != b {
		t.Fatalf("diff %q, files %q", a, b)
	}
	v.addComment(review.Comment{Path: v.diffAbs("app/main.go")}, "from Diff")
	if v.fileComment(v.filesAbs("main.go")) == nil {
		t.Error("the Files tab doesn't see the Diff tab's comment")
	}
	if got := v.sentPath(v.diffAbs("app/main.go")); got != "main.go" {
		t.Errorf("sent as %q", got)
	}
	if got := v.sentPath("/elsewhere/x.go"); got != "/elsewhere/x.go" {
		t.Errorf("a file outside the session's directory sent as %q", got)
	}
}

// Two edits in the same second have the same label but are different
// changes, each with its own comment.
func TestCommentsOnChangesInOneSecond(t *testing.T) {
	v := newTestView(t)
	at := time.Date(2026, 10, 24, 10, 0, 1, 0, time.UTC)
	edit := func(id, old, nw string) fileTouch {
		in, _ := json.Marshal(map[string]string{"file_path": "/w/a.go", "old_string": old, "new_string": nw})
		return fileTouch{op: opEdit, ev: &parse.Event{Kind: parse.KindTool, Time: at, Tool: &parse.ToolCall{ID: id, Name: "Edit", Input: in}}}
	}
	f := &fileEntry{path: "a.go", touches: []fileTouch{edit("t1", "a", "b"), edit("t2", "c", "d")}}
	v.detail.Height = 5
	v.detail.SetContent(strings.Repeat("x\n", 40))
	v.fMarks = []int{3, 10}
	comment := func(text string) {
		v.commentOnChange(f)
		v.rev.input.SetValue(text)
		v.typeComment(key("enter"))
	}
	comment("first")
	v.detail.SetYOffset(10)
	v.commentOnChange(f)
	if got := v.rev.input.Value(); got != "" {
		t.Fatalf("the second change opened %q", got)
	}
	v.typeComment(key("esc"))
	comment("second")
	if len(v.rev.comments) != 2 || v.rev.comments[0].Lines[1].Text != "b" || v.rev.comments[1].Lines[1].Text != "d" {
		t.Errorf("comments = %+v", v.rev.comments)
	}
	v.detail.SetYOffset(0)
	v.commentOnChange(f)
	if got := v.rev.input.Value(); got != "first" {
		t.Errorf("back on the first change: %q", got)
	}
}
