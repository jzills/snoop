package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
)

const treeFixture = `{"type":"user","uuid":"u1","cwd":"/repo","timestamp":"2026-10-24T10:00:00Z","message":{"content":"go"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/repo/internal/ui/app.go","old_string":"a","new_string":"b\nc"}},{"type":"tool_use","id":"t2","name":"Edit","input":{"file_path":"/repo/internal/ui/board.go","old_string":"x","new_string":"y"}},{"type":"tool_use","id":"t3","name":"Read","input":{"file_path":"/repo/docs/notes.md"}},{"type":"tool_use","id":"t4","name":"Write","input":{"file_path":"/repo/main.go","content":"package main\n"}},{"type":"tool_use","id":"t5","name":"Read","input":{"file_path":"/etc/hosts"}}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"},{"type":"tool_result","tool_use_id":"t2","content":"ok"},{"type":"tool_result","tool_use_id":"t3","content":"ok"},{"type":"tool_result","tool_use_id":"t4","content":"ok"},{"type":"tool_result","tool_use_id":"t5","content":"ok"}]}}
`

func treeFiles(t *testing.T) []*fileEntry {
	t.Helper()
	s, _ := parse.Parse(strings.NewReader(treeFixture))
	return buildFiles([]fileSource{{events: s.Events}}, "/repo", func(string) bool { return true })
}

func TestBuildFileTree(t *testing.T) {
	tree := buildFileTree(treeFiles(t))
	var names []string
	for _, n := range tree {
		names = append(names, n.name)
	}
	// Folders first, changed before read-only; internal/ui/ is merged into
	// one row, as is /etc/.
	if got := strings.Join(names, " "); got != "internal/ui/ /etc/ docs/ main.go" {
		t.Fatalf("top level = %q", got)
	}
	ui := tree[0]
	if ui.key != "internal/ui/" || len(ui.children) != 2 || ui.children[0].name != "app.go" || ui.children[0].depth != 1 {
		t.Errorf("internal/ui/ = %+v", ui)
	}
	if ui.sum.edits != 2 || ui.sum.added != 3 || ui.sum.removed != 2 {
		t.Errorf("internal/ui/ totals = %+v", ui.sum)
	}
	if !tree[1].sum.readOnly() || tree[1].children[0].file.path != "/etc/hosts" {
		t.Errorf("/etc/ = %+v", tree[1])
	}
}

func openTreeView(t *testing.T) *view {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	os.WriteFile(p, []byte(treeFixture), 0o644)
	v, err := openView(p, p, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.close)
	v.update(key("4"), 140, 30)
	v.render(140, 30)
	return v
}

func rowNames(v *view) string {
	var names []string
	for _, n := range v.fRows {
		names = append(names, n.name)
	}
	return strings.Join(names, " ")
}

func TestFilesTreeToggle(t *testing.T) {
	v := openTreeView(t)
	if got := rowNames(v); got != "internal/ui/app.go internal/ui/board.go main.go docs/notes.md /etc/hosts" {
		t.Fatalf("flat rows = %q", got)
	}
	v.update(key("down"), 140, 30) // internal/ui/board.go
	v.update(key("T"), 140, 30)
	// Changed folders start open, read-only ones closed.
	if got := rowNames(v); got != "internal/ui/ app.go board.go /etc/ docs/ main.go" {
		t.Fatalf("tree rows = %q", got)
	}
	if f := v.selectedFile(); f == nil || f.path != "internal/ui/board.go" {
		t.Errorf("selection moved to %+v", v.selectedFileRow())
	}
	out := ansi.Strip(v.render(140, 30))
	for _, want := range []string{"▿ internal/ui/", "▹ docs/", "T flat list"} {
		if !strings.Contains(out, want) {
			t.Errorf("tree lacks %q:\n%s", want, out)
		}
	}
	v.update(key("T"), 140, 30)
	if f := v.selectedFile(); f == nil || f.path != "internal/ui/board.go" || len(v.fRows) != 5 {
		t.Errorf("back to flat: %q, selected %+v", rowNames(v), f)
	}
}

func TestFilesTreeExpandCollapse(t *testing.T) {
	v := openTreeView(t)
	v.update(key("T"), 140, 30)
	v.update(key("down"), 140, 30) // app.go
	v.update(key("left"), 140, 30) // to its folder
	if n := v.selectedFileRow(); n.key != "internal/ui/" {
		t.Fatalf("left selected %q", n.key)
	}
	v.update(key("left"), 140, 30) // closes it
	if got := rowNames(v); got != "internal/ui/ /etc/ docs/ main.go" {
		t.Errorf("after closing = %q", got)
	}
	v.update(key("down"), 140, 30) // /etc/
	v.update(key(" "), 140, 30)
	if got := rowNames(v); got != "internal/ui/ /etc/ hosts docs/ main.go" {
		t.Errorf("after space = %q", got)
	}
	out := ansi.Strip(v.render(140, 30))
	if !strings.Contains(out, "/etc/hosts") && !strings.Contains(out, "hosts") {
		t.Errorf("folder detail lacks its files:\n%s", out)
	}
	v.update(key("E"), 140, 30)
	if len(v.fRows) != 8 {
		t.Errorf("E opened %q", rowNames(v))
	}
	v.update(key("E"), 140, 30)
	if got := rowNames(v); got != "internal/ui/ app.go board.go /etc/ docs/ main.go" {
		t.Errorf("second E should reset: %q", got)
	}
}

func TestFilesTreeSearchOpensFolders(t *testing.T) {
	v := openTreeView(t)
	v.update(key("T"), 140, 30)
	v.search.SetValue("notes")
	v.refresh()
	v.render(140, 30)
	if got := rowNames(v); got != "docs/ notes.md" {
		t.Errorf("search rows = %q", got)
	}
}

func TestFilesTreeCarriesToTheNextSession(t *testing.T) {
	a := New(Options{})
	v := openTreeView(t)
	a.push(v)
	v.update(key("T"), 140, 30)
	w := openTreeView(t)
	a.push(w)
	w.render(140, 30)
	if !w.prefs.filesTree || rowNames(w) != "internal/ui/ app.go board.go /etc/ docs/ main.go" {
		t.Errorf("the next session view should open in the tree: %q", rowNames(w))
	}
}
