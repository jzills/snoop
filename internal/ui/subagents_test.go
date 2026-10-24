package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const inlineFixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-24T10:00:00Z","message":{"content":"go"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Agent","input":{"description":"helper"}}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:09Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"done"}]},"toolUseResult":{"agentId":"sub1"}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:10Z","message":{"id":"m2","content":[{"type":"text","text":"All done."}]}}
`

const inlineSub = `{"type":"user","uuid":"s0","timestamp":"2026-10-24T10:00:02Z","message":{"content":"find the zebra"}}
{"type":"assistant","uuid":"s1","timestamp":"2026-10-24T10:00:03Z","message":{"id":"x1","content":[{"type":"tool_use","id":"st1","name":"Grep","input":{"pattern":"stripes"}}]}}
`

// newInlineView opens a session with one subagent and returns the view and
// the subagent transcript's path.
func newInlineView(t *testing.T) (*view, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	os.WriteFile(p, []byte(inlineFixture), 0o644)
	subDir := filepath.Join(dir, "s", "subagents")
	os.MkdirAll(subDir, 0o755)
	sp := filepath.Join(subDir, "agent-sub1.jsonl")
	os.WriteFile(sp, []byte(inlineSub), 0o644)
	v, err := openView(p, p, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.close)
	return v, sp
}

func selectAgent(t *testing.T, v *view) {
	t.Helper()
	for i, e := range v.shown {
		if agentID(e) == "sub1" {
			v.cursor, v.follow = i, false
			return
		}
	}
	t.Fatal("no Agent row")
}

func TestExpandSubagentInline(t *testing.T) {
	v, _ := newInlineView(t)
	n := len(v.shown)
	selectAgent(t, v)
	v.update(key("x"), 140, 30)
	if len(v.shown) != n+2 {
		t.Fatalf("shown = %d, want %d", len(v.shown), n+2)
	}
	sub := v.shown[v.cursor+1]
	if sub.Text != "find the zebra" || v.depth[sub] != 1 {
		t.Errorf("after the call: %q at depth %d", sub.Text, v.depth[sub])
	}
	out := ansi.Strip(v.timeline(140, 20))
	if !strings.Contains(out, "▾") || !strings.Contains(out, "│ ") {
		t.Errorf("expanded rows lack ▾ / │:\n%s", out)
	}
	// n skips the subagent's own prompt.
	v.cursor = 0
	v.jumpPrompt(1)
	if v.cursor != 0 {
		t.Errorf("n moved to a subagent prompt (row %d)", v.cursor)
	}
	selectAgent(t, v)
	v.update(key("x"), 140, 30)
	if len(v.shown) != n {
		t.Errorf("collapse left %d rows, want %d", len(v.shown), n)
	}
}

func TestSearchMarksCollapsedSubagent(t *testing.T) {
	v, _ := newInlineView(t)
	v.search.SetValue("stripes")
	v.refresh()
	if len(v.shown) != 1 || agentID(v.shown[0]) != "sub1" || !v.marked[v.shown[0]] {
		t.Fatalf("shown = %d rows, marked = %v", len(v.shown), v.marked)
	}
	if out := ansi.Strip(v.timeline(140, 5)); !strings.Contains(out, "●") {
		t.Errorf("no ● on the collapsed call:\n%s", out)
	}
	// Expanded, the match itself is listed and the mark goes.
	v.expanded["sub1"] = true
	v.refresh()
	if len(v.shown) != 2 || v.shown[1].Tool == nil || v.shown[1].Tool.Name != "Grep" || v.marked[v.shown[0]] {
		t.Errorf("expanded search = %d rows", len(v.shown))
	}
}

func TestExpandedSubagentGrowsLive(t *testing.T) {
	v, sp := newInlineView(t)
	v.expanded["sub1"] = true
	v.refresh()
	n := len(v.shown)
	if v.pollSubs() {
		t.Error("poll reported changes with nothing new")
	}
	f, _ := os.OpenFile(sp, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"assistant","uuid":"s2","timestamp":"2026-10-24T10:00:04Z","message":{"id":"x2","content":[{"type":"text","text":"found it"}]}}` + "\n")
	f.Close()
	if !v.pollSubs() {
		t.Fatal("poll missed the new line")
	}
	v.refresh()
	if len(v.shown) != n+1 {
		t.Errorf("shown = %d, want %d", len(v.shown), n+1)
	}
}

func TestToggleAll(t *testing.T) {
	v, _ := newInlineView(t)
	v.update(key("X"), 140, 30)
	if !v.expanded["sub1"] {
		t.Fatal("X did not expand")
	}
	v.update(key("X"), 140, 30)
	if v.expanded["sub1"] {
		t.Error("second X did not collapse")
	}
}
