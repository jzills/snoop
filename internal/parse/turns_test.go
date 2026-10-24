package parse

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Two prompts; m1 is written as two records (one per block); the first turn
// ends with turn_duration, the second holds a compaction.
const turnsFixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-24T10:00:00Z","message":{"content":"first"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"hm"}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:02Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:05Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
{"type":"assistant","uuid":"a3","timestamp":"2026-10-24T10:00:06Z","message":{"id":"m2","model":"claude-opus-5-5","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":200}}}
{"type":"system","subtype":"turn_duration","durationMs":21391,"timestamp":"2026-10-24T10:00:07Z"}
{"type":"user","uuid":"u2","timestamp":"2026-10-24T10:01:00Z","message":{"content":[{"type":"text","text":"second"},{"type":"text","text":"more"}]}}
{"type":"system","subtype":"compact_boundary","content":"Conversation compacted","timestamp":"2026-10-24T10:01:01Z","compactMetadata":{"trigger":"auto","preTokens":468293}}
{"type":"user","uuid":"cs","isCompactSummary":true,"timestamp":"2026-10-24T10:01:02Z","message":{"content":"This session is being continued from a previous conversation."}}
{"type":"assistant","uuid":"a4","timestamp":"2026-10-24T10:01:10Z","message":{"id":"m3","model":"claude-opus-5-5","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":4,"output_tokens":4,"cache_read_input_tokens":31000}}}
`

func parseString(t *testing.T, in string) *Session {
	t.Helper()
	s, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTurnsSplitOnPrompts(t *testing.T) {
	s := parseString(t, turnsFixture)
	if len(s.Turns) != 2 {
		t.Fatalf("turns = %d, want 2 (two text blocks and the compact summary start none)", len(s.Turns))
	}
	if s.Turns[0].Prompt.Text != "first" || s.TurnOf(s.Turns[1].Prompt) != s.Turns[1] {
		t.Errorf("prompts = %q, TurnOf mismatch", s.Turns[0].Prompt.Text)
	}
	if s.Turns[0].Tools != 1 {
		t.Errorf("tools = %d", s.Turns[0].Tools)
	}
	for _, e := range s.Events {
		if strings.HasPrefix(e.Text, "This session is being continued") && e.Kind != KindSystem {
			t.Errorf("compact summary kind = %v, want system", e.Kind)
		}
	}
}

func TestTurnUsageCountsMessageOnce(t *testing.T) {
	s := parseString(t, turnsFixture)
	t0 := s.Turns[0]
	if want := (Usage{Input: 11, Output: 7, CacheRead: 300, CacheCreate: 20}); t0.Usage != want {
		t.Errorf("usage = %+v, want %+v", t0.Usage, want)
	}
	if t0.Requests != 2 || t0.ContextEnd != 201 {
		t.Errorf("requests = %d, context end = %d", t0.Requests, t0.ContextEnd)
	}
	for _, e := range s.Events[1:3] {
		if e.Ctx != 130 {
			t.Errorf("%s ctx = %d, want 130", e.UUID, e.Ctx)
		}
	}
}

func TestTurnDurationRecorded(t *testing.T) {
	s := parseString(t, turnsFixture)
	if t0 := s.Turns[0]; !t0.Done || t0.Duration() != 21391*time.Millisecond {
		t.Errorf("turn 0 done=%v duration=%v", t0.Done, t0.Duration())
	}
	if t1 := s.Turns[1]; t1.Done || t1.Duration() != 10*time.Second {
		t.Errorf("turn 1 done=%v duration=%v, want running 10s", t1.Done, t1.Duration())
	}
}

func TestCompaction(t *testing.T) {
	s := parseString(t, turnsFixture)
	var c *Compaction
	for _, e := range s.Events {
		if e.Kind == KindCompact {
			c = e.Compact
		}
	}
	if c == nil || c.Trigger != "auto" || c.Pre != 468293 || c.Post != 31004 {
		t.Fatalf("compaction = %+v", c)
	}
	if s.Compactions != 1 || !s.Turns[1].Compacted || s.Turns[0].Compacted {
		t.Errorf("compactions = %d, compacted = %v %v", s.Compactions, s.Turns[0].Compacted, s.Turns[1].Compacted)
	}
	if s.Context != 31004 || s.PeakContext != 31004 {
		t.Errorf("context = %d, peak = %d", s.Context, s.PeakContext)
	}
}

func TestContextWindow(t *testing.T) {
	req := func(ctx int) string {
		return `{"type":"assistant","message":{"id":"x","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":` + strconv.Itoa(ctx) + `}}}` + "\n"
	}
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"default", req(1000), 200_000},
		{"reported", `{"type":"system","subtype":"local_command","content":"x","contextUsage":{"raw_max_tokens":1000000}}` + "\n" + req(1000), 1_000_000},
		{"usage report", `{"type":"system","subtype":"local_command","content":"x","usageReport":{"session":{"model_usage":{"claude-opus-5-5":{"contextWindow":500000}}}}}` + "\n" + req(1000), 500_000},
		{"1m model", `{"type":"user","requestedModel":"claude-opus-5-5[1m]","message":{"content":"hi"}}` + "\n" + req(1000), 1_000_000},
		{"inferred", req(250_000), 1_000_000},
	}
	for _, c := range cases {
		if got := parseString(t, c.in).ContextWindow(); got != c.want {
			t.Errorf("%s: window = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestRequestsBeforeFirstPrompt(t *testing.T) {
	s := parseString(t, `{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"resumed"}],"usage":{"input_tokens":5,"cache_read_input_tokens":900}}}
`)
	if len(s.Turns) != 0 || s.Context != 905 {
		t.Errorf("turns = %d, context = %d", len(s.Turns), s.Context)
	}
}

func TestTurnCostSumsToTotal(t *testing.T) {
	in := `{"type":"user","uuid":"u1","message":{"content":"a"}}
{"type":"assistant","message":{"id":"m1","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":100}}}
{"type":"user","uuid":"u2","message":{"content":"b"}}
{"type":"assistant","message":{"id":"m2","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":400}}}
{"type":"cost-state","totalCostUSD":3.5,"modelUsage":{"claude-opus-5-5":{"costUSD":3.0},"claude-haiku-5-5":{"costUSD":0.5}}}
`
	s := parseString(t, in)
	c0, ok0 := s.TurnCost(s.Turns[0])
	c1, ok1 := s.TurnCost(s.Turns[1])
	if !ok0 || !ok1 {
		t.Fatal("cost not available")
	}
	if math.Abs(c0+c1-3.5) > 1e-9 {
		t.Errorf("turn costs %v + %v != 3.5", c0, c1)
	}
	if math.Abs(c1-4*c0) > 1e-9 {
		t.Errorf("turn 1 (4x weight) = %v, turn 0 = %v", c1, c0)
	}
}

func TestTurnCostWithoutCostState(t *testing.T) {
	s := parseString(t, turnsFixture)
	if _, ok := s.TurnCost(s.Turns[0]); ok {
		t.Error("cost reported without a cost-state record")
	}
}
