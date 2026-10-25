package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
)

func TestLineDelta(t *testing.T) {
	cases := []struct {
		old, nw    string
		add, remov int
	}{
		{"a\nb\nc", "a\nB\nc", 1, 1},
		{"a\nc", "a\nb\nc", 1, 0},
		{"", "x\ny\n", 2, 0},
		{"same", "same", 0, 0},
	}
	for _, c := range cases {
		if a, r := lineDelta(c.old, c.nw); a != c.add || r != c.remov {
			t.Errorf("lineDelta(%q, %q) = +%d -%d, want +%d -%d", c.old, c.nw, a, r, c.add, c.remov)
		}
	}
}

const filesFixture = `{"type":"user","uuid":"u1","cwd":"/repo","timestamp":"2026-10-24T10:00:00Z","message":{"content":"go"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/repo/a.go"}},{"type":"tool_use","id":"t2","name":"Edit","input":{"file_path":"/repo/b.go","old_string":"x\ny","new_string":"x\nz\nw"}},{"type":"tool_use","id":"t3","name":"Write","input":{"file_path":"/etc/c.conf","content":"1\n2\n3\n"}},{"type":"tool_use","id":"t4","name":"Edit","input":{"file_path":"/repo/b.go","old_string":"q","new_string":"r"}},{"type":"tool_use","id":"t5","name":"Agent","input":{"description":"helper"}}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"},{"type":"tool_result","tool_use_id":"t2","content":"ok"},{"type":"tool_result","tool_use_id":"t3","content":"ok"},{"type":"tool_result","tool_use_id":"t4","content":"no match","is_error":true}]}}
{"type":"user","uuid":"r2","timestamp":"2026-10-24T10:00:09Z","message":{"content":[{"type":"tool_result","tool_use_id":"t5","content":"done"}]},"toolUseResult":{"agentId":"sub1"}}
`

const subFixture = `{"type":"assistant","uuid":"s1","timestamp":"2026-10-24T10:00:05Z","message":{"id":"x1","content":[{"type":"tool_use","id":"st1","name":"Edit","input":{"file_path":"/repo/a.go","old_string":"k","new_string":"K"}}]}}
`

func TestBuildFiles(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(filesFixture))
	sub, _ := parse.Parse(strings.NewReader(subFixture))
	agent := s.Events[len(s.Events)-1]
	files := buildFiles([]fileSource{{events: s.Events}, {events: sub.Events, agent: "helper", via: agent}}, "/repo", func(string) bool { return true })

	var paths []string
	for _, f := range files {
		paths = append(paths, f.path)
	}
	// All three were changed; a.go's subagent edit is the most recent.
	if got := strings.Join(paths, " "); got != "a.go b.go /etc/c.conf" {
		t.Fatalf("paths = %q", got)
	}
	a, b, c := files[0], files[1], files[2]
	if a.reads != 1 || a.edits != 1 || len(a.touches) != 2 || a.touches[1].agent != "helper" || a.touches[1].via != agent {
		t.Errorf("a.go = %+v", a)
	}
	if b.edits != 1 || b.errs != 1 || b.added != 2 || b.removed != 1 {
		t.Errorf("b.go = %+v (failed edit must not count lines)", b)
	}
	if c.writes != 1 || c.added != 3 {
		t.Errorf("c.conf = %+v", c)
	}
}

func TestFilesReadOnlyLast(t *testing.T) {
	in := `{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:09Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"new-read.go"}}]}}
{"type":"assistant","uuid":"a0","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m0","content":[{"type":"tool_use","id":"t0","name":"Write","input":{"file_path":"old-write.go","content":"x"}}]}}
`
	s, _ := parse.Parse(strings.NewReader(in))
	files := buildFiles([]fileSource{{events: s.Events}}, "", func(string) bool { return true })
	if files[0].path != "old-write.go" || !files[1].readOnly() {
		t.Errorf("order = %s, %s", files[0].path, files[1].path)
	}
}

func TestFilesTabIncludesSubagents(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	os.WriteFile(p, []byte(filesFixture), 0o644)
	subDir := filepath.Join(dir, "s", "subagents")
	os.MkdirAll(subDir, 0o755)
	os.WriteFile(filepath.Join(subDir, "agent-sub1.jsonl"), []byte(subFixture), 0o644)
	os.WriteFile(filepath.Join(subDir, "agent-sub1.meta.json"), []byte(`{"description":"helper","agentType":"Explore"}`), 0o644)

	v, err := openView(p, p, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.close()
	v.update(key("4"), 140, 30)
	out := ansi.Strip(v.render(140, 30))
	for _, want := range []string{"4 Files", "3 files", "a.go", "⤷ helper", "Bash aren't tracked"} {
		if !strings.Contains(out, want) {
			t.Errorf("Files tab lacks %q:\n%s", want, out)
		}
	}
	// enter on a subagent's change shows the Agent call that ran it.
	v.update(key("enter"), 140, 30)
	if sel := v.selected(); v.tab != tabTimeline || sel == nil || sel.Tool == nil || sel.Tool.Name != "Agent" {
		t.Errorf("enter selected %+v on tab %d", sel, v.tab)
	}
}

func TestFilesSearchFiltersPaths(t *testing.T) {
	v := newTestView(t)
	v.tab = tabFiles
	v.search.SetValue("nomatch")
	v.refresh()
	v.render(120, 30)
	if len(v.files) != 0 {
		t.Errorf("files = %d, want 0", len(v.files))
	}
}

func TestFitLeftKeepsFileName(t *testing.T) {
	if got := fitLeft("docs/superpowers/plans/report.md", 12); got != "…s/report.md" {
		t.Errorf("fitLeft = %q", got)
	}
	if got := fitLeft("a.go", 12); got != "a.go" {
		t.Errorf("short path changed: %q", got)
	}
}

func TestFileDetailKeepsHistoryLinesApart(t *testing.T) {
	// An edit followed by a read: reads have no blank line before them.
	in := `{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"m.go","old_string":"a","new_string":"b"}}]}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:02Z","message":{"id":"m2","content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"m.go"}}]}}
`
	s, _ := parse.Parse(strings.NewReader(in))
	files := buildFiles([]fileSource{{events: s.Events}}, "", func(string) bool { return true })
	out, _ := renderFileDetail(files[0], 60)
	for _, l := range strings.Split(ansi.Strip(out), "\n") {
		if strings.HasPrefix(l, "+ ") && strings.ContainsAny(l, glyphOK+glyphErr+glyphPending) {
			t.Errorf("diff line runs into the next entry: %q", l)
		}
	}
}

func TestDiffOfNewContentHasNoDeletion(t *testing.T) {
	got := ansi.Strip(diff("", "package x\n", 80))
	if got != "+ package x" {
		t.Errorf("diff = %q, want only the added line", got)
	}
}
