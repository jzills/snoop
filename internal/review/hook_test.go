package review

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		if err := Hook(strings.NewReader(in), &out, inboxOpts(dir)); err != nil || out.Len() != 0 {
			t.Errorf("%s with nothing waiting: %q, %v", c.event, out.String(), err)
		}
		Send(dir, "s1", "fix it", t0)
		if err := Hook(strings.NewReader(in), &out, inboxOpts(dir)); err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(out.Bytes(), &m); err != nil || !c.check(m) {
			t.Errorf("%s printed %s", c.event, out.String())
		}
		out.Reset()
		if Hook(strings.NewReader(in), &out, inboxOpts(dir)); out.Len() != 0 {
			t.Errorf("%s delivered twice", c.event)
		}
	}
}

func TestHookOtherEvent(t *testing.T) {
	dir := t.TempDir()
	Send(dir, "s1", "wait", t0)
	var out bytes.Buffer
	Hook(strings.NewReader(`{"session_id":"s1","hook_event_name":"PreToolUse"}`), &out, inboxOpts(dir))
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
		if err := Hook(strings.NewReader(in), &out, inboxOpts(t.TempDir())); err == nil || out.Len() != 0 {
			t.Errorf("%q: %v, %q", in, err, out.String())
		}
	}
}

func inboxOpts(dir string) HookOptions {
	return HookOptions{Inbox: dir, Now: func() time.Time { return t0 }}
}

// askOpts is a listening snoop with a fake clock: Sleep advances it and
// then runs during, which stands in for snoop answering meanwhile.
func askOpts(t *testing.T, wait time.Duration, during func(dir string, now time.Time)) (HookOptions, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	now := t0
	listening := filepath.Join(dir, "listening")
	Listen(listening, t0)
	o := HookOptions{Inbox: filepath.Join(dir, "inbox"), Asks: filepath.Join(dir, "asks"), Listening: listening, AskWait: wait,
		Now: func() time.Time { return now },
		Sleep: func(d time.Duration) {
			now = now.Add(d)
			if during != nil {
				during(filepath.Join(dir, "asks"), now)
			}
		}}
	return o, &now
}

const askInput = `{"session_id":"s1","hook_event_name":"PermissionRequest","cwd":"/w","agent_id":"a1","agent_type":"Explore","tool_name":"Bash","tool_input":{"command":"npm test"}}`

// answerAt answers the first pending request once the clock reaches at.
func answerAt(at time.Time, a Answer) func(string, time.Time) {
	done := false
	return func(dir string, now time.Time) {
		if done || now.Before(at) {
			return
		}
		rs, _ := Pending(dir, now)
		if len(rs) > 0 {
			Respond(dir, rs[0], a)
			done = true
		}
	}
}

func decision(t *testing.T, out []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output %q: %v", out, err)
	}
	h := m["hookSpecificOutput"].(map[string]any)
	if h["hookEventName"] != "PermissionRequest" {
		t.Errorf("hookEventName = %v", h["hookEventName"])
	}
	return h["decision"].(map[string]any)
}

func TestHookNotListeningReturnsAtOnce(t *testing.T) {
	dir := t.TempDir()
	slept := false
	o := HookOptions{Inbox: dir, Asks: filepath.Join(dir, "asks"), Listening: filepath.Join(dir, "listening"), AskWait: time.Minute,
		Now: func() time.Time { return t0 }, Sleep: func(time.Duration) { slept = true }}
	var out bytes.Buffer
	if err := Hook(strings.NewReader(askInput), &out, o); err != nil || out.Len() != 0 || slept {
		t.Errorf("out %q, err %v, slept %v", out.String(), err, slept)
	}
	if _, err := os.Stat(o.Asks); err == nil {
		t.Error("wrote a request with no snoop listening")
	}
}

func TestHookAllows(t *testing.T) {
	o, _ := askOpts(t, time.Minute, answerAt(t0, Answer{Behavior: "allow"}))
	var out bytes.Buffer
	if err := Hook(strings.NewReader(askInput), &out, o); err != nil {
		t.Fatal(err)
	}
	d := decision(t, out.Bytes())
	if _, ok := d["message"]; d["behavior"] != "allow" || ok {
		t.Errorf("decision = %v", d)
	}
	if es, _ := os.ReadDir(filepath.Join(o.Asks, "s1")); len(es) != 0 {
		t.Errorf("left %d files", len(es))
	}
}

func TestHookDeniesWithMessage(t *testing.T) {
	o, _ := askOpts(t, time.Minute, answerAt(t0.Add(2*time.Second), Answer{Behavior: "deny", Message: "use make test"}))
	var out bytes.Buffer
	Hook(strings.NewReader(askInput), &out, o)
	if d := decision(t, out.Bytes()); d["behavior"] != "deny" || d["message"] != "use make test" {
		t.Errorf("decision = %v", d)
	}
}

func TestHookTimesOutSilently(t *testing.T) {
	o, now := askOpts(t, 15*time.Second, nil)
	var out bytes.Buffer
	if err := Hook(strings.NewReader(askInput), &out, o); err != nil || out.Len() != 0 {
		t.Errorf("out %q, err %v", out.String(), err)
	}
	if w := now.Sub(t0); w < 15*time.Second || w > 15200*time.Millisecond {
		t.Errorf("waited %v", w)
	}
	if es, _ := os.ReadDir(filepath.Join(o.Asks, "s1")); len(es) != 0 {
		t.Errorf("left %d files", len(es))
	}
}

func TestHookWaitsBrieflyForAClaimedAnswer(t *testing.T) {
	// snoop claims the request just before the deadline but writes its
	// answer a second after it.
	var claimed Request
	during := func(dir string, now time.Time) {
		switch {
		case claimed.ID == "" && !now.Before(t0.Add(14900*time.Millisecond)):
			rs, _ := Pending(dir, now)
			if len(rs) == 1 {
				claimed = rs[0]
				base := filepath.Join(dir, "s1", claimed.ID)
				os.Rename(base+askExt, base+claimedExt)
			}
		case claimed.ID != "" && !now.Before(t0.Add(16*time.Second)):
			b, _ := json.Marshal(Answer{Behavior: "allow"})
			writeRenamed(filepath.Join(dir, "s1", claimed.ID+answerExt), b)
		}
	}
	o, _ := askOpts(t, 15*time.Second, during)
	var out bytes.Buffer
	Hook(strings.NewReader(askInput), &out, o)
	if out.Len() == 0 || decision(t, out.Bytes())["behavior"] != "allow" {
		t.Errorf("out %q", out.String())
	}
}

func TestHookRequestFields(t *testing.T) {
	var got []Request
	o, _ := askOpts(t, time.Second, func(dir string, now time.Time) {
		if got == nil {
			got, _ = Pending(dir, now)
		}
	})
	var out bytes.Buffer
	Hook(strings.NewReader(askInput), &out, o)
	if len(got) != 1 {
		t.Fatalf("pending = %v", got)
	}
	r := got[0]
	if r.Session != "s1" || r.Cwd != "/w" || r.AgentID != "a1" || r.AgentType != "Explore" || r.Tool != "Bash" ||
		string(r.Input) != `{"command":"npm test"}` || !r.Asked.Equal(t0) || !r.Deadline.Equal(t0.Add(time.Second)) {
		t.Errorf("request = %+v", r)
	}
}
