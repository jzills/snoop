package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/stats"
)

// Every tab leaves one blank line between the tabs and its content.
func TestStatsTabSpacing(t *testing.T) {
	v := newTestView(t)
	v.update(key("3"), 120, 30)
	lines := strings.Split(ansi.Strip(v.render(120, 30)), "\n")
	for i, l := range lines {
		if strings.Contains(l, "Timeline") && strings.Contains(l, "Stats") {
			if strings.TrimSpace(lines[i+1]) != "" || strings.TrimSpace(lines[i+2]) == "" {
				t.Errorf("after the tabs:\n%q\n%q", lines[i+1], lines[i+2])
			}
			return
		}
	}
	t.Fatal("no tab row")
}

// Two turns on Opus 5.5 and one request on Haiku, still running: no
// cost-state yet.
const statsFixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-24T10:00:00Z","message":{"content":"Add rate limiting"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1000,"output_tokens":2000,"cache_read_input_tokens":50000}}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:02Z","message":{"id":"m2","model":"claude-haiku-5-5","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":500,"output_tokens":100}}}
{"type":"user","uuid":"u2","timestamp":"2026-10-24T10:01:00Z","message":{"content":"Make it configurable"}}
{"type":"assistant","uuid":"a3","timestamp":"2026-10-24T10:01:01Z","message":{"id":"m3","model":"claude-opus-5-5","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":200,"output_tokens":800,"cache_read_input_tokens":60000}}}
{"type":"system","subtype":"turn_duration","durationMs":5000,"timestamp":"2026-10-24T10:01:05Z"}
`

func statsOut(t *testing.T, in string) string {
	t.Helper()
	s, err := parse.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	s.Version = "2.1.310"
	return ansi.Strip(renderStats(s, stats.Compute(s), 120))
}

func TestStatsEstimatesARunningSessionsCost(t *testing.T) {
	out := statsOut(t, statsFixture)
	for _, want := range []string{"cost · estimated", "≈$"} {
		if !strings.Contains(out, want) {
			t.Errorf("Stats lacks %q:\n%s", want, out)
		}
	}
	// Every turn row has a cost.
	for _, prompt := range []string{"Add rate limiting", "Make it configurable"} {
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, prompt) && !strings.Contains(l, "≈$") {
				t.Errorf("turn row without a cost: %q", l)
			}
		}
	}
}

func TestStatsModelsAreStackedAtTheBottom(t *testing.T) {
	out := statsOut(t, statsFixture)
	i := strings.Index(out, "Models")
	if i < 0 || i < strings.Index(out, "Tokens") {
		t.Fatalf("no Models section below Tokens:\n%s", out)
	}
	rest := strings.Split(out[i:], "\n")
	if len(rest) < 3 || !strings.Contains(rest[1], "claude-opus-5-5") || !strings.Contains(rest[1], "2 requests") ||
		!strings.Contains(rest[2], "claude-haiku-5-5") || !strings.Contains(rest[2], "1 request") {
		t.Errorf("models aren't one per line, most used first:\n%s", out[i:])
	}
	if strings.Contains(out, "2.1.310") {
		t.Errorf("the Claude Code version is shown:\n%s", out)
	}
}

func TestStatsRecordedCostIsNotEstimated(t *testing.T) {
	out := statsOut(t, statsFixture+`{"type":"cost-state","totalCostUSD":1.5,"modelUsage":{"claude-opus-5-5":{"costUSD":1.4},"claude-haiku-5-5":{"costUSD":0.1}}}`+"\n")
	if strings.Contains(out, "estimated") || !strings.Contains(out, "$1.50") || !strings.Contains(out, "$1.40") {
		t.Errorf("recorded cost:\n%s", out)
	}
}
