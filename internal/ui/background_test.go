package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A turn that launches a code review in the background and ends.
const (
	bgSkill  = `{"type":"assistant","uuid":"a5","timestamp":"2026-10-25T10:00:02Z","message":{"id":"m5","content":[{"type":"tool_use","id":"s1","name":"Skill","input":{"skill":"code-review"}}]}}`
	bgForked = `{"type":"user","uuid":"r5","timestamp":"2026-10-25T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"s1","content":"Launched in the background."}]},"toolUseResult":{"status":"forked","background":true,"agentId":"cr1","commandName":"code-review","success":true}}`
	bgEnd    = `{"type":"system","subtype":"turn_duration","uuid":"d5","timestamp":"2026-10-25T10:00:06Z","durationMs":6000}`
	bgNote   = `{"type":"user","uuid":"n5","timestamp":"2026-10-25T10:04:00Z","message":{"role":"user","content":"<task-notification>\n<tool-use-id>s1</tool-use-id>\n<status>completed</status>\n<summary>Review finished</summary>\n</task-notification>"}}`
	bgReply  = `{"type":"assistant","uuid":"a6","timestamp":"2026-10-25T10:04:05Z","message":{"id":"m6","content":[{"type":"text","text":"The review found two issues."}]}}`
	bgEnd2   = `{"type":"system","subtype":"turn_duration","uuid":"d6","timestamp":"2026-10-25T10:04:09Z","durationMs":9000}`
	bgShell  = `{"type":"assistant","uuid":"a7","timestamp":"2026-10-25T10:00:02Z","message":{"id":"m7","content":[{"type":"tool_use","id":"b9","name":"Bash","input":{"command":"npm run dev","run_in_background":true}}]}}`
	bgShellR = `{"type":"user","uuid":"r7","timestamp":"2026-10-25T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b9","content":"Started."}]},"toolUseResult":{"backgroundTaskId":"bg1","stdout":"","stderr":""}}`
)

func TestBackgroundWorkKeepsTheSessionWorking(t *testing.T) {
	s := sessOf(bPrompt, bgSkill, bgForked, bgEnd)
	st, detail := rowState(s, "", bStart.Add(time.Minute), bStart.Add(2*time.Minute), 30*time.Second)
	if st != rowWorking || !strings.Contains(detail, "in background") || !strings.Contains(detail, "code-review") {
		t.Errorf("state %v %q, want working with code-review in background", st, detail)
	}
}

func TestBackgroundWorkGoesIdleWhenQuiet(t *testing.T) {
	s := sessOf(bPrompt, bgSkill, bgForked, bgEnd)
	if st, _ := rowState(s, "", bStart, bStart.Add(11*time.Minute), 30*time.Second); st == rowWorking {
		t.Error("still working after 10 quiet minutes")
	}
}

func TestABackgroundShellDoesNotKeepTheSessionWorking(t *testing.T) {
	s := sessOf(bPrompt, bgShell, bgShellR, bgEnd)
	if st, _ := rowState(s, "", bStart.Add(6*time.Second), bStart.Add(time.Minute), 30*time.Second); st != rowYourTurn {
		t.Errorf("state %v, want your turn: a dev server may run forever", st)
	}
}

func TestABackgroundCallRowRunsUntilItReports(t *testing.T) {
	a := newClockApp(t, time.Minute, bPrompt, bgSkill, bgForked, bgEnd)
	row := ""
	for _, l := range screenLines(a) {
		if strings.Contains(l, "code-review") && strings.Contains(l, "Skill") {
			row = l
		}
	}
	if row == "" || strings.Contains(row, glyphOK) || !strings.ContainsAny(row, spinFrames) {
		t.Errorf("launch row %q, want a spinner and no ✓", row)
	}
	a.top().onTail(lines(bgNote))
	for _, l := range screenLines(a) {
		if strings.Contains(l, "code-review") && strings.Contains(l, "Skill") && !strings.Contains(l, glyphOK) {
			t.Errorf("launch row %q, want ✓ once it reported", l)
		}
	}
}

func TestTheWorkRowShowsTheBackgroundSubagent(t *testing.T) {
	a := newClockApp(t, time.Minute, bPrompt, bgSkill, bgForked, bgEnd)
	v := a.top()
	writeSubagent(t, v.path, "cr1")
	sub := filepath.Join(strings.TrimSuffix(v.path, ".jsonl"), "subagents", "agent-cr1.jsonl")
	os.WriteFile(sub, []byte(bPrompt+"\n"+bBash+"\n"), 0o644)
	a.Update(tickMsg{})
	row := lastRow(a)
	if !strings.Contains(row, "1 in background") || !strings.Contains(row, "code-review") || !strings.Contains(row, "go test") {
		t.Errorf("last row %q, want the background review and its running Bash", row)
	}
}

func TestTheTurnAlertWaitsForBackgroundWork(t *testing.T) {
	a := newClockApp(t, 5*time.Second, bPrompt, bgSkill, bgForked)
	v := a.top()
	for _, al := range v.onTail(lines(bgEnd)) {
		if al.kind == alertTurn {
			t.Fatal("turn alert while the review runs in the background")
		}
	}
	v.onTail(lines(bgNote, bgReply))
	got := false
	for _, al := range v.onTail(lines(bgEnd2)) {
		got = got || al.kind == alertTurn
	}
	if !got {
		t.Error("no turn alert once Claude finished with the review")
	}
}

func TestABackgroundSubagentViewWorksUntilItReports(t *testing.T) {
	a := newClockApp(t, time.Minute, bPrompt, bgSkill, bgForked, bgEnd)
	parent := a.top().path
	writeSubagent(t, parent, "cr1")
	sub := filepath.Join(strings.TrimSuffix(parent, ".jsonl"), "subagents", "agent-cr1.jsonl")
	os.WriteFile(sub, []byte(bPrompt+"\n"), 0o644)
	v, err := openView(sub, parent, "code-review", false)
	if err != nil {
		t.Fatal(err)
	}
	a.push(v)
	if top := screenLines(a)[0]; !strings.Contains(top, "working") {
		t.Errorf("top bar %q, want working while the review runs", top)
	}
}
