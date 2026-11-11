package parse

import (
	"testing"
	"time"
)

const (
	bgPrompt = `{"type":"user","uuid":"p1","timestamp":"2026-10-25T10:00:00Z","message":{"role":"user","content":"review the branch"}}`
	bgSkill  = `{"type":"assistant","uuid":"a1","timestamp":"2026-10-25T10:00:02Z","message":{"id":"m1","content":[{"type":"tool_use","id":"s1","name":"Skill","input":{"skill":"code-review"}}]}}`
	bgForked = `{"type":"user","uuid":"r1","timestamp":"2026-10-25T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"s1","content":"Launched in the background."}]},"toolUseResult":{"status":"forked","background":true,"agentId":"cr1","commandName":"code-review","success":true}}`
	bgAgent  = `{"type":"assistant","uuid":"a2","timestamp":"2026-10-25T10:00:04Z","message":{"id":"m2","content":[{"type":"tool_use","id":"g1","name":"Agent","input":{"subagent_type":"Explore","description":"map it","run_in_background":true}}]}}`
	bgAsync  = `{"type":"user","uuid":"r2","timestamp":"2026-10-25T10:00:05Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"g1","content":"Async agent launched."}]},"toolUseResult":{"status":"async_launched","isAsync":true,"agentId":"ex1"}}`
	bgEnd    = `{"type":"system","subtype":"turn_duration","uuid":"d1","timestamp":"2026-10-25T10:00:06Z","durationMs":6000}`
	bgNote   = `{"type":"user","uuid":"n1","timestamp":"2026-10-25T10:04:00Z","message":{"role":"user","content":"<task-notification>\n<task-id>cr1</task-id>\n<tool-use-id>s1</tool-use-id>\n<status>completed</status>\n<summary>Review finished</summary>\n</task-notification>"}}`
	bgReply  = `{"type":"assistant","uuid":"a3","timestamp":"2026-10-25T10:04:05Z","message":{"id":"m3","content":[{"type":"text","text":"The review found two issues."}]}}`
	fgDone   = `{"type":"user","uuid":"r3","timestamp":"2026-10-25T10:00:09Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"g1","content":"mapped"}]},"toolUseResult":{"status":"completed","agentId":"ex1"}}`
)

func feedAll(ls ...string) *Session {
	s := NewSession()
	for _, l := range ls {
		s.Feed([]byte(l))
	}
	return s
}

func TestBackgroundLaunchesAreRunning(t *testing.T) {
	s := feedAll(bgPrompt, bgSkill, bgForked, bgAgent, bgAsync)
	run := s.RunningBackground()
	if len(run) != 2 {
		t.Fatalf("%d running in the background, want 2", len(run))
	}
	for _, e := range run {
		if !e.Tool.Background || !e.Tool.Running() {
			t.Errorf("%s: background %v running %v", e.Tool.Name, e.Tool.Background, e.Tool.Running())
		}
	}
}

func TestAForegroundAgentIsNotBackground(t *testing.T) {
	agent := `{"type":"assistant","uuid":"a2","timestamp":"2026-10-25T10:00:04Z","message":{"id":"m2","content":[{"type":"tool_use","id":"g1","name":"Agent","input":{"subagent_type":"Explore","description":"map it"}}]}}`
	s := feedAll(bgPrompt, agent, fgDone)
	if e := s.Events[1]; e.Tool.Background || e.Tool.Running() {
		t.Errorf("foreground agent: background %v running %v", e.Tool.Background, e.Tool.Running())
	}
	if n := len(s.RunningBackground()); n != 0 {
		t.Errorf("%d running in the background, want 0", n)
	}
}

func TestATaskNotificationEndsItsCall(t *testing.T) {
	s := feedAll(bgPrompt, bgSkill, bgForked, bgAgent, bgAsync, bgEnd, bgNote)
	skill := s.Events[1].Tool
	if skill.Running() || !skill.BackgroundDone {
		t.Fatalf("the notified call is still running")
	}
	if want := time.Date(2026, 10, 25, 10, 4, 0, 0, time.UTC); !skill.BackgroundEnd.Equal(want) {
		t.Errorf("ended %v, want %v", skill.BackgroundEnd, want)
	}
	if skill.BackgroundStatus != "completed" {
		t.Errorf("status %q, want completed", skill.BackgroundStatus)
	}
	if run := s.RunningBackground(); len(run) != 1 || run[0].Tool.Name != "Agent" {
		t.Errorf("running %v, want only the Agent", run)
	}
}

func TestATaskNotificationReopensTheTurn(t *testing.T) {
	s := feedAll(bgPrompt, bgSkill, bgForked, bgEnd)
	if !s.Turns[0].Done {
		t.Fatal("the turn isn't done before the notification")
	}
	s.Feed([]byte(bgNote))
	s.Feed([]byte(bgReply))
	if len(s.Turns) != 1 {
		t.Fatalf("%d turns, want the notification in the same turn", len(s.Turns))
	}
	if s.Turns[0].Done {
		t.Error("the turn is still done while Claude works on the notification")
	}
	s.Feed([]byte(`{"type":"system","subtype":"turn_duration","uuid":"d2","timestamp":"2026-10-25T10:04:09Z","durationMs":9000}`))
	if !s.Turns[0].Done {
		t.Error("the turn didn't end again")
	}
}

func TestANotificationInATextBlockEndsItsCall(t *testing.T) {
	note := `{"type":"user","uuid":"n2","timestamp":"2026-10-25T10:04:00Z","message":{"role":"user","content":[{"type":"text","text":"<task-notification><tool-use-id>g1</tool-use-id><status>failed</status><summary>Agent failed</summary></task-notification>"}]}}`
	s := feedAll(bgPrompt, bgAgent, bgAsync, note)
	if g := s.Events[1].Tool; g.Running() || g.BackgroundStatus != "failed" {
		t.Errorf("running %v status %q, want ended and failed", g.Running(), g.BackgroundStatus)
	}
}
