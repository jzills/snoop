package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func prompt(uuid, text string) string {
	return `{"type":"user","uuid":"` + uuid + `","timestamp":"2026-10-25T10:00:00Z","message":{"role":"user","content":"` + text + `"}}` + "\n"
}

func appendTo(t *testing.T, p, s string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func TestFollowerReadsWhatWasAdded(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	appendTo(t, p, prompt("u1", "one"))
	f, err := Track(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Sess.Events) != 1 {
		t.Fatalf("events on load = %d", len(f.Sess.Events))
	}
	if changed, reset := f.Poll(); changed || reset {
		t.Errorf("poll of an unchanged file = %v %v", changed, reset)
	}

	// A line still being written is left for the next poll.
	full := prompt("u2", "two")
	appendTo(t, p, full[:20])
	if changed, _ := f.Poll(); changed || len(f.Sess.Events) != 1 {
		t.Errorf("partial line: changed %v, events %d", changed, len(f.Sess.Events))
	}
	appendTo(t, p, full[20:])
	if changed, reset := f.Poll(); !changed || reset || len(f.Sess.Events) != 2 {
		t.Errorf("completed line: changed %v reset %v, events %d", changed, reset, len(f.Sess.Events))
	}
}

func TestFollowerReloadsAShrunkFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	appendTo(t, p, prompt("u1", "one")+prompt("u2", "two"))
	f, err := Track(p)
	if err != nil {
		t.Fatal(err)
	}
	old := f.Sess
	os.WriteFile(p, []byte(prompt("u3", "three")), 0o644)
	changed, reset := f.Poll()
	if !changed || !reset || f.Sess == old || len(f.Sess.Events) != 1 {
		t.Errorf("after truncation: changed %v reset %v, new session %v, events %d",
			changed, reset, f.Sess != old, len(f.Sess.Events))
	}
}

func TestRecentIncludesBusySubagents(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-repo")
	os.MkdirAll(filepath.Join(proj, "quiet", "subagents"), 0o755)
	now := time.Now()
	write := func(p string, at time.Time) {
		os.WriteFile(p, []byte(prompt("u", "x")), 0o644)
		os.Chtimes(p, at, at)
	}
	write(filepath.Join(proj, "fresh.jsonl"), now.Add(-time.Minute))
	write(filepath.Join(proj, "old.jsonl"), now.Add(-2*time.Hour))
	// The main transcript is quiet while its subagent works.
	write(filepath.Join(proj, "quiet.jsonl"), now.Add(-50*time.Minute))
	write(filepath.Join(proj, "quiet", "subagents", "agent-a1.jsonl"), now.Add(-10*time.Second))

	got := map[string]time.Time{}
	for _, a := range Recent(root, now.Add(-30*time.Minute)) {
		got[filepath.Base(a.Path)] = a.Last
	}
	if len(got) != 2 {
		t.Fatalf("recent = %v, want fresh and quiet", got)
	}
	if _, ok := got["old.jsonl"]; ok {
		t.Error("old.jsonl is listed")
	}
	if last := got["quiet.jsonl"]; now.Sub(last) > time.Minute {
		t.Errorf("quiet.jsonl last write = %v ago, want its subagent's", now.Sub(last))
	}
}
