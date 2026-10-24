package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
)

// Two calls in parallel (t1 0-10s, t2 0-5s), then t3 10-20s.
const waterfallFixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-24T10:00:00Z","message":{"content":"go"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:00Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"slow"}},{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"x"}}]}}
{"type":"user","uuid":"r2","timestamp":"2026-10-24T10:00:05Z","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":"ok"}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:10Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:10Z","message":{"id":"m2","content":[{"type":"tool_use","id":"t3","name":"Grep","input":{"pattern":"p"}}]}}
{"type":"user","uuid":"r3","timestamp":"2026-10-24T10:00:20Z","message":{"content":[{"type":"tool_result","tool_use_id":"t3","content":"ok"}]}}
{"type":"system","subtype":"turn_duration","durationMs":20000,"timestamp":"2026-10-24T10:00:20Z"}
`

func TestWaterfallPlacesCalls(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(waterfallFixture))
	tr := s.Turns[0]
	var tools []*parse.Event
	for _, e := range tr.Events {
		if e.Tool != nil {
			tools = append(tools, e)
		}
	}
	// w=43: label 14 cells, track 20 cells, so 1s per cell.
	lines := strings.Split(strings.TrimRight(ansi.Strip(waterfall(tr, tools, 43, time.Now())), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	bars := []struct{ from, to int }{{0, 10}, {0, 5}, {10, 20}}
	for i, l := range lines {
		track := []rune(l)[1+14+1:][:20]
		got := strings.TrimRight(string(track), " ")
		from := len([]rune(got)) - len([]rune(strings.TrimLeft(got, " ")))
		to := len([]rune(got))
		if from != bars[i].from || to != bars[i].to {
			t.Errorf("call %d bar = [%d,%d), want [%d,%d): %q", i, from, to, bars[i].from, bars[i].to, l)
		}
	}
}

func TestTurnDetailSummary(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(waterfallFixture))
	out := ansi.Strip(renderTurnDetail(s, s.Turns[0], 60, time.Now()))
	for _, want := range []string{"20.0s", "3 · 25.0s", "Slowest", "10.0s ▸ Bash slow", "Waterfall"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "so far") {
		t.Error("finished turn marked as in progress")
	}
}
