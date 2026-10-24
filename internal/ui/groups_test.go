package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/tail"
)

const groupFixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-24T10:00:00Z","message":{"content":"go"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"ls"}},{"type":"tool_use","id":"b2","name":"Bash","input":{"command":"false"}},{"type":"tool_use","id":"r1","name":"Read","input":{"file_path":"a.go"}},{"type":"tool_use","id":"s1","name":"Skill","input":{"skill":"review"}},{"type":"tool_use","id":"m1","name":"mcp__gh__pr","input":{}},{"type":"tool_use","id":"g1","name":"Agent","input":{"subagent_type":"Explore","description":"look"}}]}}
{"type":"user","uuid":"r","timestamp":"2026-10-24T10:00:03Z","message":{"content":[{"type":"tool_result","tool_use_id":"b1","content":"ok"},{"type":"tool_result","tool_use_id":"b2","content":"no","is_error":true}]}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:04Z","message":{"id":"m2","content":[{"type":"text","text":"done"}]}}
`

func newGroupView(t *testing.T) *view {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	os.WriteFile(p, []byte(groupFixture), 0o644)
	v, err := openView(p, p, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.close)
	v.tab = tabTools
	return v
}

func labels(v *view) []string {
	var out []string
	for _, g := range v.gRows {
		out = append(out, g.label)
	}
	return out
}

func TestBuildGroups(t *testing.T) {
	v := newGroupView(t)
	if len(v.groups) != 4 {
		t.Fatalf("roots = %d, want 4", len(v.groups))
	}
	builtin := v.groups[0]
	if builtin.label != "Built-in tools" || builtin.n != 3 || builtin.errs != 1 {
		t.Errorf("builtin = %+v", builtin)
	}
	if bash := builtin.children[0]; bash.label != "Bash" || bash.n != 2 || len(bash.children) != 2 {
		t.Errorf("first name = %q (%d), want Bash (2) sorted first", bash.label, bash.n)
	}
	want := map[int]string{1: "review", 2: "gh·pr", 3: "Explore"}
	for i, name := range want {
		if got := v.groups[i].children[0].label; got != name {
			t.Errorf("group %d name = %q, want %q", i, got, name)
		}
	}
	// Default view: kinds open, names closed.
	got := labels(v)
	if len(got) != 9 || got[0] != "Built-in tools" || got[1] != "Bash" || got[2] != "Read" || got[3] != "Skills" {
		t.Errorf("rows = %q", got)
	}
}

func TestGroupKeys(t *testing.T) {
	v := newGroupView(t)
	v.update(key("j"), 150, 30) // Bash
	v.update(key(" "), 150, 30) // expand Bash
	if got := labels(v); len(got) != 11 || got[2] != "ls" || got[3] != "false" {
		t.Fatalf("after expand: %q", got)
	}
	v.update(key("j"), 150, 30)                      // first call
	v.update(tea.KeyMsg{Type: tea.KeyLeft}, 150, 30) // to parent
	if g := v.selectedGroup(); g.label != "Bash" {
		t.Fatalf("left on a call selected %q, want its group", g.label)
	}
	v.update(tea.KeyMsg{Type: tea.KeyLeft}, 150, 30) // collapse
	if len(v.gRows) != 9 {
		t.Fatalf("collapse left %d rows", len(v.gRows))
	}
	v.update(key("E"), 150, 30)
	if len(v.gRows) != 4+5+6 {
		t.Fatalf("expand all = %d rows, want 15", len(v.gRows))
	}
	v.update(key("E"), 150, 30)
	if len(v.gRows) != 9 {
		t.Fatalf("second E should restore defaults, got %d rows", len(v.gRows))
	}
}

func TestEnterOnCallJumpsToTimeline(t *testing.T) {
	v := newGroupView(t)
	v.kind = 3 // skills only: would hide the Bash call
	v.refresh()
	v.tab = tabTools
	v.gOpen["k0/Bash"] = true
	v.flattenGroups("c/b2")
	v.update(tea.KeyMsg{Type: tea.KeyEnter}, 150, 30)
	if v.tab != tabTimeline {
		t.Fatal("enter on a call did not switch to the timeline")
	}
	if e := v.selected(); e == nil || e.Tool == nil || e.Tool.ID != "b2" {
		t.Fatalf("timeline selection = %+v, want call b2", e)
	}
	if v.kind != 0 || v.follow {
		t.Errorf("kind = %d follow = %v; want filter cleared, follow off", v.kind, v.follow)
	}
}

func TestGroupSearch(t *testing.T) {
	v := newGroupView(t)
	v.search.SetValue("false")
	v.refresh()
	got := labels(v)
	if len(got) != 3 || got[0] != "Built-in tools" || got[1] != "Bash" || got[2] != "false" {
		t.Fatalf("search rows = %q, want the matching call with its groups open", got)
	}
}

func TestGroupSelectionSurvivesLiveUpdate(t *testing.T) {
	v := newGroupView(t)
	v.flattenGroups("k3") // Subagents
	v.onTail(tail.Update{Lines: [][]byte{[]byte(`{"type":"assistant","uuid":"a9","message":{"id":"m9","content":[{"type":"tool_use","id":"b9","name":"Bash","input":{"command":"pwd"}}]}}`)}})
	if g := v.selectedGroup(); g == nil || g.key != "k3" {
		t.Fatalf("selection moved to %+v after a live update", g)
	}
	if v.groups[0].children[0].n != 3 {
		t.Errorf("Bash count = %d, want 3 after the new call", v.groups[0].children[0].n)
	}
}

func TestExpandAllDuringSearchKeepsState(t *testing.T) {
	v := newGroupView(t)
	v.setOpen(v.groups[0].children[0], true) // Bash, opened by hand
	v.setOpen(v.groups[0], false)            // Built-in, closed by hand
	v.search.SetValue("review")
	v.refresh()
	v.update(key("E"), 150, 30)
	if !v.gOpen["k0/Bash"] {
		t.Error("E during search wiped a group opened by hand")
	}
	if !v.gOpen["k1/review"] {
		t.Error("E during search did not expand the matching groups")
	}
	v.search.SetValue("")
	v.refresh()
	if v.savedOpen(v.groups[0]) {
		t.Error("E during search opened a group that didn't match, overriding its saved state")
	}
}
