package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/tail"
)

func lines(s ...string) tail.Update {
	var u tail.Update
	for _, l := range s {
		u.Lines = append(u.Lines, []byte(l))
	}
	return u
}

func TestOpeningRaisesNoAlerts(t *testing.T) {
	v := newTestView(t)
	if v.alert != nil || v.unseenErrs != 0 || len(v.pending) != 0 {
		t.Errorf("alert on open: %+v, errs %d, pending %d", v.alert, v.unseenErrs, len(v.pending))
	}
}

func TestErrorAlert(t *testing.T) {
	v := newTestView(t)
	v.onTail(lines(
		`{"type":"assistant","uuid":"a9","timestamp":"2026-10-24T10:01:00Z","message":{"id":"m9","content":[{"type":"tool_use","id":"t9","name":"Bash","input":{"command":"false"}}]}}`,
		`{"type":"user","uuid":"r9","timestamp":"2026-10-24T10:01:01Z","message":{"content":[{"type":"tool_result","tool_use_id":"t9","content":"exit 1","is_error":true}]}}`,
	))
	if v.alert == nil || v.alert.kind != alertError || v.alert.ev.Tool.ID != "t9" || v.unseenErrs != 1 {
		t.Fatalf("alert = %+v, unseen = %d", v.alert, v.unseenErrs)
	}
	if h := ansi.Strip(v.header(140)); !strings.Contains(h, "1✗") || !strings.Contains(h, "error: Bash") {
		t.Errorf("header = %q", h)
	}
	// ! jumps to the failed call and marks errors seen.
	v.update(key("!"), 140, 30)
	if sel := v.selected(); sel == nil || sel.Tool == nil || sel.Tool.ID != "t9" || v.unseenErrs != 0 {
		t.Errorf("! selected %+v, unseen %d", sel, v.unseenErrs)
	}
}

func TestTurnAndCompactAlerts(t *testing.T) {
	v := newTestView(t)
	as := v.onTail(lines(`{"type":"system","subtype":"compact_boundary","content":"x","timestamp":"2026-10-24T10:01:00Z","compactMetadata":{"trigger":"auto","preTokens":1000}}`))
	if len(as) != 1 || as[0].kind != alertCompact {
		t.Fatalf("compaction alerts = %+v", as)
	}
	as = v.onTail(lines(`{"type":"system","subtype":"turn_duration","durationMs":5000,"timestamp":"2026-10-24T10:01:01Z"}`))
	if len(as) != 1 || as[0].kind != alertTurn || v.alert.kind != alertTurn {
		t.Fatalf("turn alerts = %+v", as)
	}
	// A second turn_duration for the same turn does not repeat it.
	if as := v.onTail(lines(`{"type":"system","subtype":"turn_duration","durationMs":6000,"timestamp":"2026-10-24T10:01:02Z"}`)); len(as) != 0 {
		t.Errorf("repeated turn alert: %+v", as)
	}
}

func TestContextAlertOnCrossing(t *testing.T) {
	v := newTestView(t)
	req := func(id string, n int) string {
		return `{"type":"assistant","uuid":"` + id + `","message":{"id":"` + id + `","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":` + strconv.Itoa(n) + `}}}`
	}
	if as := v.onTail(lines(req("c1", 170_000))); len(as) != 0 {
		t.Errorf("85%% raised %+v", as)
	}
	if as := v.onTail(lines(req("c2", 185_000))); len(as) != 1 || as[0].kind != alertContext || as[0].text != "ctx 92%" {
		t.Errorf("crossing 90%% raised %+v", as)
	}
	if as := v.onTail(lines(req("c3", 190_000))); len(as) != 0 {
		t.Errorf("staying above 90%% raised %+v", as)
	}
}

func TestStallAlertOncePerCall(t *testing.T) {
	v := newTestView(t)
	v.onTail(lines(
		`{"type":"assistant","uuid":"a9","timestamp":"2026-10-24T10:01:00Z","message":{"id":"m9","content":[{"type":"tool_use","id":"t9","name":"Bash","input":{"command":"sleep"}},{"type":"tool_use","id":"t10","name":"Agent","input":{"description":"long"}}]}}`,
	))
	start := time.Date(2026, 10, 24, 10, 1, 0, 0, time.UTC)
	if as := v.stalls(start.Add(29*time.Second), 30*time.Second); len(as) != 0 {
		t.Errorf("stalled early: %+v", as)
	}
	as := v.stalls(start.Add(31*time.Second), 30*time.Second)
	if len(as) != 1 || as[0].ev.Tool.ID != "t9" || !strings.Contains(as[0].text, "may be a permission prompt") {
		t.Fatalf("stall alerts = %+v (Agent calls are excluded)", as)
	}
	if as := v.stalls(start.Add(60*time.Second), 30*time.Second); len(as) != 0 {
		t.Errorf("repeated stall alert: %+v", as)
	}
}

func TestBellOnlyForChosenKinds(t *testing.T) {
	a := New(Options{Bell: true, BellOn: map[string]bool{alertError: true}})
	if got := a.signalText([]*alert{{kind: alertCompact}}); got != "" {
		t.Errorf("bell rang for a compaction: %q", got)
	}
	if got := a.signalText([]*alert{{kind: alertCompact}, {kind: alertError}}); got != "\a" {
		t.Errorf("bell for an error = %q", got)
	}
}

func TestKeyHidesAlert(t *testing.T) {
	a := newTestApp(t, 120, 40)
	v := a.top()
	v.raise([]*alert{{kind: alertTurn, text: "✓ your turn", ev: v.sess.Events[0]}})
	a.Update(key("j"))
	if v.alertShown {
		t.Error("alert still shown after a key")
	}
}

func TestForkedSkillIsNotAStall(t *testing.T) {
	v := newTestView(t)
	v.onTail(lines(
		`{"type":"assistant","uuid":"a9","timestamp":"2026-10-24T10:01:00Z","message":{"id":"m9","content":[{"type":"tool_use","id":"t9","name":"Skill","input":{"skill":"review"}}]}}`,
	))
	start := time.Date(2026, 10, 24, 10, 1, 0, 0, time.UTC)
	// A plain Skill call with nothing running under it can wait on you.
	if as := v.stalls(start.Add(31*time.Second), 30*time.Second); len(as) != 1 {
		t.Fatalf("plain Skill stall alerts = %+v", as)
	}
	v.pending[v.sess.Events[len(v.sess.Events)-1]] = false
	// Once its forked subagent writes, it is running, not waiting.
	writeSubagent(t, v.rootPath, "f1")
	if as := v.stalls(start.Add(2*time.Minute), 30*time.Second); len(as) != 0 {
		t.Errorf("forked Skill raised a stall: %+v", as)
	}
}

func writeSubagent(t *testing.T, sessionPath, id string) {
	t.Helper()
	dir := filepath.Join(strings.TrimSuffix(sessionPath, ".jsonl"), "subagents")
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, "agent-"+id+".jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestContextAlertWithoutEvents(t *testing.T) {
	s := sessOf()
	w := newWatch("")
	before := w.snap(s)
	s.Feed([]byte(`{"type":"assistant","uuid":"c1","message":{"id":"c1","content":[{"type":"text","text":" "}],"usage":{"input_tokens":190000}}}`))
	if len(s.Events) != 0 {
		t.Fatalf("fixture has %d events, want none", len(s.Events))
	}
	as := w.alerts(s, before)
	if len(as) != 1 || as[0].kind != alertContext || as[0].ev != nil {
		t.Errorf("alerts = %+v", as)
	}
}
