package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func askAt(t *testing.T, dir, session string, at time.Time) Request {
	t.Helper()
	r, err := Ask(dir, Request{Session: session, Tool: "Bash", Input: json.RawMessage(`{"command":"ls"}`),
		Asked: at, Deadline: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAskAndPending(t *testing.T) {
	dir := t.TempDir()
	askAt(t, dir, "s1", t0.Add(time.Second))
	askAt(t, dir, "s2", t0)
	askAt(t, dir, "s1", t0.Add(2*time.Second))
	got, err := Pending(dir, t0.Add(3*time.Second))
	if err != nil || len(got) != 3 {
		t.Fatalf("Pending = %d, %v", len(got), err)
	}
	for i, r := range got {
		if r.ID == "" || r.Tool != "Bash" || string(r.Input) != `{"command":"ls"}` {
			t.Errorf("request %d = %+v", i, r)
		}
		if i > 0 && r.Asked.Before(got[i-1].Asked) {
			t.Errorf("not oldest first: %v", got)
		}
	}
	if got[0].Session != "s2" {
		t.Errorf("oldest is %s", got[0].Session)
	}
}

func TestPendingMissingDir(t *testing.T) {
	got, err := Pending(filepath.Join(t.TempDir(), "none"), t0)
	if err != nil || got != nil {
		t.Errorf("Pending = %v, %v", got, err)
	}
}

func TestPendingSkipsExpiredAndClaimed(t *testing.T) {
	dir := t.TempDir()
	askAt(t, dir, "s1", t0.Add(-2*time.Minute)) // its deadline has passed
	r := askAt(t, dir, "s1", t0)
	if err := Respond(dir, r, Answer{Behavior: "allow"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := Pending(dir, t0.Add(time.Second)); len(got) != 0 {
		t.Errorf("Pending = %+v", got)
	}
}

func TestPendingCleansLeftovers(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "s1")
	os.MkdirAll(d, 0o700)
	name := func(at time.Time, ext string) string {
		return filepath.Join(d, fmt.Sprintf("%020d-%d%s", at.UnixNano(), os.Getpid(), ext))
	}
	old, recent := t0.Add(-41*time.Minute), t0.Add(-39*time.Minute)
	for _, ext := range []string{".json", ".claimed", ".answer"} {
		os.WriteFile(name(old, ext), []byte("{}"), 0o600)
		os.WriteFile(name(recent, ext), []byte("{}"), 0o600)
	}
	Pending(dir, t0)
	for _, ext := range []string{".json", ".claimed", ".answer"} {
		if _, err := os.Stat(name(old, ext)); err == nil {
			t.Errorf("old %s kept", ext)
		}
		if _, err := os.Stat(name(recent, ext)); err != nil {
			t.Errorf("recent %s deleted", ext)
		}
	}
}

func TestPendingDropsDeadHooks(t *testing.T) {
	dir := t.TempDir()
	r := askAt(t, dir, "s1", t0)
	was := alive
	alive = func(int) bool { return false }
	defer func() { alive = was }()
	if got, _ := Pending(dir, t0); len(got) != 0 {
		t.Errorf("Pending kept a request whose hook exited: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "s1", r.ID+".json")); err == nil {
		t.Error("the request file was kept")
	}
	r = askAt(t, dir, "s1", t0)
	if err := Respond(dir, r, Answer{Behavior: "allow"}); !errors.Is(err, ErrAnswered) {
		t.Errorf("Respond to a dead hook = %v", err)
	}
}

func TestAskRejectsBadSession(t *testing.T) {
	dir := t.TempDir()
	for _, s := range []string{"", "..", "a/b"} {
		if _, err := Ask(dir, Request{Session: s, Asked: t0, Deadline: t0.Add(time.Minute)}); err == nil {
			t.Errorf("Ask(%q) succeeded", s)
		}
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("wrote %d entries", len(es))
	}
}

func TestRespondTwice(t *testing.T) {
	dir := t.TempDir()
	r := askAt(t, dir, "s1", t0)
	if err := Respond(dir, r, Answer{Behavior: "deny", Message: "no"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "s1", r.ID+".answer"))
	if string(b) != `{"behavior":"deny","message":"no"}`+"\n" {
		t.Errorf("answer = %q", b)
	}
	if err := Respond(dir, r, Answer{Behavior: "allow"}); !errors.Is(err, ErrAnswered) {
		t.Errorf("second Respond = %v", err)
	}
}

func TestListening(t *testing.T) {
	p := filepath.Join(t.TempDir(), "listening")
	if Listening(p, t0) {
		t.Error("listening with no file")
	}
	os.WriteFile(p, nil, 0o600)
	if Listening(p, t0) {
		t.Error("listening with an empty file")
	}
	os.WriteFile(p, []byte("garbage"), 0o600)
	if Listening(p, t0) {
		t.Error("listening with a garbage file")
	}
	if err := Listen(p, t0); err != nil {
		t.Fatal(err)
	}
	if !Listening(p, t0.Add(4*time.Second)) {
		t.Error("not listening 4s after Listen")
	}
	if Listening(p, t0.Add(6*time.Second)) {
		t.Error("still listening 6s after Listen")
	}
}
