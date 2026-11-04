package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/review"
)

// diffTreeApp is a session on its Diff tab with files in folders, the
// session having changed all but docs/notes.md.
func diffTreeApp(t *testing.T) (*App, *view) {
	t.Helper()
	a, v, _ := diffApp(t, true)
	press(a, "5")
	file := func(p string, add, del int) *review.File {
		return &review.File{Path: p, Added: add, Removed: del, Hunks: []review.Hunk{{Header: "@@", Lines: []review.Line{{Kind: '+', New: 1, Text: "x"}}}}}
	}
	files := []*review.File{file("internal/ui/app.go", 3, 1), file("internal/ui/board.go", 2, 0),
		file("internal/review/hook.go", 5, 5), file("main.go", 1, 0), file("docs/notes.md", 4, 0)}
	v.rev.diff = &review.Diff{Root: "/r", Files: files}
	v.rev.ours = map[string]bool{"internal/ui/app.go": true, "internal/ui/board.go": true, "internal/review/hook.go": true, "main.go": true}
	v.rev.cursor = 0
	v.layoutDiff("")
	return a, v
}

func diffNames(v *view) string {
	var names []string
	for _, n := range v.rev.rows {
		names = append(names, n.name)
	}
	return strings.Join(names, " ")
}

func TestDiffTree(t *testing.T) {
	a, v := diffTreeApp(t)
	press(a, "T", "g")
	// Folders first, the session's own before others; "internal/" holds
	// two folders, so it isn't merged.
	if got := diffNames(v); got != "internal/ review/ hook.go ui/ app.go board.go docs/ notes.md main.go" {
		t.Fatalf("tree = %q", got)
	}
	out := ansi.Strip(a.View())
	for _, want := range []string{"▿ internal/", "+10", "−6"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
	// The folder's pane lists its files.
	if !strings.Contains(out, "3 files") || !strings.Contains(out, "review/hook.go") {
		t.Errorf("folder pane:\n%s", out)
	}
	press(a, "h") // closes internal/
	if got := diffNames(v); got != "internal/ docs/ notes.md main.go" {
		t.Errorf("after closing = %q", got)
	}
	press(a, " ")      // opens it again
	press(a, "j", "j") // hook.go
	press(a, "h")      // its folder
	if n := v.selectedDiffRow(); n == nil || n.key != "internal/review/" {
		t.Errorf("left on a file selected %+v", n)
	}
	press(a, "j", "tab") // into hook.go's lines
	if !v.focusR || v.diffFile() == nil || v.diffFile().Path != "internal/review/hook.go" {
		t.Errorf("focus %v on %+v", v.focusR, v.diffFile())
	}
	press(a, "tab", "T")
	if got := diffNames(v); !strings.HasPrefix(got, "internal/ui/app.go") {
		t.Errorf("flat list = %q", got)
	}
	if n := v.selectedDiffRow(); n.key != "internal/review/hook.go" {
		t.Errorf("T lost the selection: %q", n.key)
	}
}

func TestDiffTreeExpandAll(t *testing.T) {
	a, v := diffTreeApp(t)
	press(a, "T", "g", "E") // on internal/, all open: closes it and its folders
	if got := diffNames(v); got != "internal/ docs/ notes.md main.go" {
		t.Errorf("E on internal/ = %q", got)
	}
	press(a, "E") // opens them again
	if got := diffNames(v); got != "internal/ review/ hook.go ui/ app.go board.go docs/ notes.md main.go" {
		t.Errorf("second E = %q", got)
	}
	// On a file, E closes every folder and keeps the cursor on the file's.
	press(a, "j", "j") // hook.go
	press(a, "E")
	if got := diffNames(v); got != "internal/ docs/ main.go" {
		t.Errorf("E on a file = %q", got)
	}
	if n := v.selectedDiffRow(); n.key != "internal/" {
		t.Errorf("selected %q", n.key)
	}
}

// A refresh keeps the selected folder, and comments count on folders.
func TestDiffTreeRefreshAndComments(t *testing.T) {
	a, v := diffTreeApp(t)
	press(a, "T", "g", "j", "j", "j") // ui/
	v.rev.comments = []pending{{Comment: review.Comment{Path: v.diffAbs("internal/ui/app.go"), Text: "why"}, id: 1}}
	v.onDiff(diffMsg{seq: v.rev.seq, d: &review.Diff{Root: "/r", Files: v.rev.diff.Files}, ours: v.rev.ours})
	if n := v.selectedDiffRow(); n == nil || n.key != "internal/ui/" {
		t.Errorf("after a refresh, selected %+v", n)
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "●1") {
		t.Errorf("no comment count:\n%s", out)
	}
}
