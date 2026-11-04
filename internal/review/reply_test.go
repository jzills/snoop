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

// replyOpts is a listening snoop with a fake clock, for a Stop hook that
// waits wait. Sleep advances the clock, keeps snoop listening, and then
// runs during, which stands in for snoop or the terminal meanwhile.
func replyOpts(t *testing.T, wait time.Duration, during func(o HookOptions, now time.Time)) (HookOptions, string, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	now := t0
	transcript := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(transcript, []byte(`{"type":"user","entrypoint":"cli"}`+"\n"), 0o600)
	o := HookOptions{Inbox: filepath.Join(dir, "inbox"), Turns: filepath.Join(dir, "turns"),
		Listening: filepath.Join(dir, "listening"), StopWait: wait, Now: func() time.Time { return now }}
	Listen(o.Listening, t0)
	o.Sleep = func(d time.Duration) {
		now = now.Add(d)
		Listen(o.Listening, now)
		if during != nil {
			during(o, now)
		}
	}
	return o, transcript, &now
}

func stopInput(transcript string) string {
	return `{"session_id":"s1","hook_event_name":"Stop","cwd":"/w","transcript_path":` + quote(transcript) +
		`,"last_assistant_message":"Done. Shall I push?"}`
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestStopWaitsForAReply(t *testing.T) {
	var seen map[string]Turn
	sent := false
	o, tr, now := replyOpts(t, time.Minute, func(o HookOptions, now time.Time) {
		if seen == nil {
			seen, _ = WaitingTurns(o.Turns, now)
		}
		if now.Sub(t0) >= 3*time.Second && !sent {
			Reply(o.Inbox, "s1", "  yes, push it  ", now)
			sent = true
		}
	})
	var out bytes.Buffer
	if err := Hook(strings.NewReader(stopInput(tr)), &out, o); err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if m["decision"] != "block" || m["reason"] != "Message from the user, sent from snoop:\n\nyes, push it\n" {
		t.Errorf("printed %q", out.String())
	}
	if d := now.Sub(t0); d < 3*time.Second || d > 4*time.Second {
		t.Errorf("replied after %v", d)
	}
	if turn, ok := seen["s1"]; !ok || turn.Cwd != "/w" || turn.Last != "Done. Shall I push?" || !turn.Ended.Equal(t0) {
		t.Errorf("while waiting, WaitingTurns = %+v", seen)
	}
	if ts, _ := WaitingTurns(o.Turns, *now); len(ts) != 0 {
		t.Errorf("after the reply, WaitingTurns = %+v", ts)
	}
}

// A prompt typed in the terminal is queued behind the hook, so the hook
// steps aside as soon as the transcript shows it.
func TestStopGivesWayToATypedPrompt(t *testing.T) {
	o, tr, now := replyOpts(t, time.Minute, nil)
	o.Sleep = func(d time.Duration) {
		*now = now.Add(d)
		Listen(o.Listening, *now)
		if now.Sub(t0) == 2*time.Second {
			f, _ := os.OpenFile(tr, os.O_APPEND|os.O_WRONLY, 0)
			// Half a line first: it counts only once it ends.
			f.WriteString(`{"type":"queue-operation","operation":"enq`)
			f.Close()
		}
		if now.Sub(t0) == 2500*time.Millisecond {
			f, _ := os.OpenFile(tr, os.O_APPEND|os.O_WRONLY, 0)
			f.WriteString(`ueue","content":"run the tests"}` + "\n")
			f.Close()
		}
	}
	var out bytes.Buffer
	if err := Hook(strings.NewReader(stopInput(tr)), &out, o); err != nil || out.Len() != 0 {
		t.Errorf("printed %q, %v", out.String(), err)
	}
	if d := now.Sub(t0); d < 2500*time.Millisecond || d > 2700*time.Millisecond {
		t.Errorf("gave way after %v", d)
	}
}

// A prompt typed while the turn's last reply was written is already in the
// transcript when the hook starts.
func TestStopGivesWayToAPromptAlreadyQueued(t *testing.T) {
	o, tr, now := replyOpts(t, time.Minute, nil)
	f, _ := os.OpenFile(tr, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"queue-operation","operation":"enqueue","content":"first"}` + "\n" +
		`{"type":"queue-operation","operation":"dequeue"}` + "\n" +
		`{"type":"assistant","message":{"content":"done"}}` + "\n" +
		`{"type":"queue-operation","operation":"enqueue","content":"next"}` + "\n")
	f.Close()
	var out bytes.Buffer
	if err := Hook(strings.NewReader(stopInput(tr)), &out, o); err != nil || out.Len() != 0 {
		t.Errorf("printed %q, %v", out.String(), err)
	}
	if !now.Equal(t0) {
		t.Errorf("waited %v", now.Sub(t0))
	}
	// One taken off the queue doesn't count.
	if waitingPrompt([]byte(`{"operation":"enqueue"}` + "\n" + `{"operation":"dequeue"}` + "\n")) {
		t.Error("a dequeued prompt counts as waiting")
	}
}

func TestStopGivesUp(t *testing.T) {
	o, tr, now := replyOpts(t, 10*time.Second, nil)
	var out bytes.Buffer
	if err := Hook(strings.NewReader(stopInput(tr)), &out, o); err != nil || out.Len() != 0 {
		t.Errorf("printed %q, %v", out.String(), err)
	}
	if d := now.Sub(t0); d != 10*time.Second {
		t.Errorf("gave up after %v", d)
	}
	if ts, _ := WaitingTurns(o.Turns, *now); len(ts) != 0 {
		t.Errorf("WaitingTurns = %+v", ts)
	}
}

// Once no snoop is listening, nobody can reply.
func TestStopStopsWhenSnoopLeaves(t *testing.T) {
	o, tr, now := replyOpts(t, time.Minute, nil)
	o.Sleep = func(d time.Duration) { *now = now.Add(d) }
	var out bytes.Buffer
	Hook(strings.NewReader(stopInput(tr)), &out, o)
	if d := now.Sub(t0); d > listenFresh+time.Second {
		t.Errorf("waited %v with nobody listening", d)
	}
}

func TestStopDoesntWait(t *testing.T) {
	cases := map[string]func(o *HookOptions, transcript string){
		"no snoop listening":    func(o *HookOptions, _ string) { os.Remove(o.Listening) },
		"an older install":      func(o *HookOptions, _ string) { o.StopWait = 0 },
		"a headless run":        func(_ *HookOptions, tr string) { os.WriteFile(tr, []byte(`{"entrypoint":"sdk-cli"}`+"\n"), 0o600) },
		"no turns folder given": func(o *HookOptions, _ string) { o.Turns = "" },
		"no transcript":         func(_ *HookOptions, tr string) { os.Remove(tr) },
		"comments to deliver":   func(o *HookOptions, _ string) { Send(o.Inbox, "s1", "fix it", t0) },
	}
	for name, change := range cases {
		o, tr, now := replyOpts(t, time.Minute, nil)
		change(&o, tr)
		var out bytes.Buffer
		if err := Hook(strings.NewReader(stopInput(tr)), &out, o); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if !now.Equal(t0) {
			t.Errorf("%s: waited %v", name, now.Sub(t0))
		}
	}
}

func TestWaitingTurnsDropsDeadHooks(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "s1")
	os.MkdirAll(d, 0o700)
	b, _ := json.Marshal(Turn{Session: "s1", Ended: t0, Deadline: t0.Add(time.Minute)})
	p := filepath.Join(d, "00000000000000000001-42.json")
	os.WriteFile(p, b, 0o600)
	was := alive
	alive = func(int) bool { return false }
	defer func() { alive = was }()
	if ts, _ := WaitingTurns(dir, t0); len(ts) != 0 {
		t.Errorf("WaitingTurns kept a turn whose hook exited: %+v", ts)
	}
	if _, err := os.Stat(p); err == nil {
		t.Error("the marker was kept")
	}
	if ts, err := WaitingTurns(filepath.Join(dir, "none"), t0); ts != nil || err != nil {
		t.Errorf("missing folder: %v, %v", ts, err)
	}
}

func TestHeadless(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) string {
		p := filepath.Join(dir, "t.jsonl")
		os.WriteFile(p, []byte(s), 0o600)
		return p
	}
	cases := map[string]bool{
		`{"entrypoint":"cli"}`:                                     false,
		`{"entrypoint":"sdk-cli"}`:                                 true,
		`{"entrypoint":"sdk-ts"}`:                                  true,
		`{"entrypoint":"sdk-cli"}` + "\n" + `{"entrypoint":"cli"}`: false,
		`{"type":"user"}`:                                          false,
	}
	for s, want := range cases {
		if got := headless(write(s)); got != want {
			t.Errorf("headless(%s) = %v", s, got)
		}
	}
	// A turn that ends with a record bigger than the tail.
	big := `{"entrypoint":"sdk-cli"}` + "\n" + `{"type":"attachment","snapshot":"` + strings.Repeat("x", 2*headTail) + `"}` + "\n"
	if !headless(write(big)) {
		t.Error("a headless run ending with a big record isn't headless")
	}
	if !headless(filepath.Join(dir, "missing")) || !headless("") {
		t.Error("a run with no transcript to read waits")
	}
}
