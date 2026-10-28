package review

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestHookOutput(t *testing.T) {
	cases := []struct {
		event string
		check func(map[string]any) bool
	}{
		{"PostToolUse", func(m map[string]any) bool {
			h := m["hookSpecificOutput"].(map[string]any)
			return h["hookEventName"] == "PostToolUse" && h["additionalContext"] == "fix it\n"
		}},
		{"UserPromptSubmit", func(m map[string]any) bool {
			h := m["hookSpecificOutput"].(map[string]any)
			return h["hookEventName"] == "UserPromptSubmit" && h["additionalContext"] == "fix it\n"
		}},
		{"Stop", func(m map[string]any) bool { return m["decision"] == "block" && m["reason"] == "fix it\n" }},
	}
	for _, c := range cases {
		dir := t.TempDir()
		in := `{"session_id":"s1","hook_event_name":"` + c.event + `","stop_hook_active":false}`
		var out bytes.Buffer
		if err := Hook(strings.NewReader(in), &out, dir, t0); err != nil || out.Len() != 0 {
			t.Errorf("%s with nothing waiting: %q, %v", c.event, out.String(), err)
		}
		Send(dir, "s1", "fix it", t0)
		if err := Hook(strings.NewReader(in), &out, dir, t0); err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(out.Bytes(), &m); err != nil || !c.check(m) {
			t.Errorf("%s printed %s", c.event, out.String())
		}
		out.Reset()
		if Hook(strings.NewReader(in), &out, dir, t0); out.Len() != 0 {
			t.Errorf("%s delivered twice", c.event)
		}
	}
}

func TestHookOtherEvent(t *testing.T) {
	dir := t.TempDir()
	Send(dir, "s1", "wait", t0)
	var out bytes.Buffer
	Hook(strings.NewReader(`{"session_id":"s1","hook_event_name":"PreToolUse"}`), &out, dir, t0)
	if out.Len() != 0 {
		t.Errorf("printed %q", out.String())
	}
	if text, _, _ := Claim(dir, "s1", t0); text != "wait\n" {
		t.Error("comment taken by an event that can't deliver it")
	}
}

func TestHookBadInput(t *testing.T) {
	var out bytes.Buffer
	for _, in := range []string{"garbage", `{"hook_event_name":"Stop"}`} {
		if err := Hook(strings.NewReader(in), &out, t.TempDir(), t0); err == nil || out.Len() != 0 {
			t.Errorf("%q: %v, %q", in, err, out.String())
		}
	}
}
