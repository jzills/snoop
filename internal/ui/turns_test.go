package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
)

func TestContextMeter(t *testing.T) {
	if m := contextMeter(142_000, 200_000, 40); !strings.Contains(m, "142k/200k") || !strings.Contains(m, "71%") {
		t.Errorf("wide meter = %q", m)
	}
	if m := contextMeter(142_000, 200_000, 20); !strings.Contains(m, "ctx 71%") || strings.Contains(m, "/") {
		t.Errorf("narrow meter = %q", m)
	}
	if m := contextMeter(0, 200_000, 40); m != "" {
		t.Errorf("empty meter = %q", m)
	}
}

const costFixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-24T10:00:00Z","message":{"content":"build the thing"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:02:10Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":200,"cache_read_input_tokens":48000}}}
{"type":"cost-state","totalCostUSD":0.31}
`

func TestTurnSummaryNarrow(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(costFixture))
	tr := s.Turns[0]
	for _, w := range []int{40, 20, 10} {
		got := turnSummary(s, tr, w)
		if lipgloss.Width(got) > w {
			t.Errorf("w=%d: %q is %d wide", w, got, lipgloss.Width(got))
		}
		if hasCost := strings.Contains(got, "≈$"); hasCost != (w >= 22) {
			t.Errorf("w=%d: %q cost shown = %v", w, got, hasCost)
		}
	}
	if got := turnSummary(s, tr, 40); got != "48.2k · ≈$0.31 · 2m10s" {
		t.Errorf("summary = %q", got)
	}
}

func TestPromptRowFitsWidth(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(costFixture))
	for _, w := range []int{50, 80, 140} {
		if row := renderTimelineRow(s, s.Events[0], false, w); lipgloss.Width(row) != w {
			t.Errorf("w=%d: row is %d wide: %q", w, lipgloss.Width(row), row)
		}
	}
}

func TestCompactText(t *testing.T) {
	c := &parse.Compaction{Trigger: "auto", Pre: 468_293}
	if got := compactText(c); got != "compacted (auto) 468k" {
		t.Errorf("before = %q", got)
	}
	c.Post = 31_004
	if got := compactText(c); got != "compacted (auto) 468k → 31k" {
		t.Errorf("after = %q", got)
	}
}
