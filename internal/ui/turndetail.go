package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
)

// Waterfall rows beyond this are summarised as "+N more".
const maxWaterfall = 60

// renderTurnDetail is the "where the time went" summary shown under a
// prompt: totals, the slowest tool calls, and a waterfall of every call.
func renderTurnDetail(s *parse.Session, t *parse.Turn, w int, now time.Time) string {
	var tools []*parse.Event
	var toolTime time.Duration
	for _, e := range t.Events {
		if e.Tool != nil {
			tools = append(tools, e)
			toolTime += e.Duration()
		}
	}
	var b strings.Builder
	kv := func(k, v string) { b.WriteString(sDim.Render(fmt.Sprintf(" %-12s", k)) + sText.Render(v) + "\n") }
	b.WriteString(section("Turn") + "\n")
	d := dur(t.Duration())
	if !t.Done {
		d += sDim.Render(" (so far)")
	}
	kv("Duration", d)
	kv("Requests", fmt.Sprint(t.Requests))
	kv("Tool calls", fmt.Sprintf("%d · %s", len(tools), dur(toolTime)))
	toks := count(t.Usage.Total())
	if usd, ok := s.TurnCost(t); ok {
		toks += " · " + costText(usd)
	}
	kv("Tokens", toks)
	if t.ContextEnd > 0 {
		kv("Context", tokens(t.ContextEnd)+" at end")
	}
	if len(tools) == 0 {
		return b.String()
	}

	slow := append([]*parse.Event(nil), tools...)
	sort.SliceStable(slow, func(i, j int) bool { return slow[i].Duration() > slow[j].Duration() })
	b.WriteString(section("Slowest") + "\n")
	for _, e := range slow[:min(5, len(slow))] {
		if e.Duration() == 0 {
			break
		}
		b.WriteString(sText.Render(fmt.Sprintf(" %7s ", dur(e.Duration()))) + toolLabel(e, w-10) + "\n")
	}

	b.WriteString(section("Waterfall") + "\n")
	b.WriteString(waterfall(t, tools, w, now))
	return b.String()
}

func toolLabel(e *parse.Event, w int) string {
	t := e.Tool
	ks := lipgloss.NewStyle().Foreground(kindColor(t.Kind))
	name := t.Name
	if t.Kind != parse.ToolBuiltin {
		name = t.Group()
	}
	return ks.Render(kindGlyph(t.Kind)+" ") + fit(ks.Render(name)+" "+sDim.Render(t.Summary), max(w-2, 1))
}

// waterfall places each call's bar by its start and end within the turn,
// so parallel calls overlap. Calls still running reach the turn's end, or
// now while the session is active.
func waterfall(t *parse.Turn, tools []*parse.Event, w int, now time.Time) string {
	end := t.End
	if !t.Done && now.Sub(t.End) < 2*time.Minute && now.After(end) {
		end = now
	}
	span := end.Sub(t.Start)
	labelW := min(max(w/3, 10), 24)
	trackW := max(w-labelW-9, 8)
	var b strings.Builder
	for i, e := range tools {
		if i == maxWaterfall {
			b.WriteString(sDim.Render(fmt.Sprintf(" +%d more", len(tools)-i)) + "\n")
			break
		}
		stop := end
		if e.Tool.HasResult && !e.Tool.ResultTime.IsZero() {
			stop = e.Tool.ResultTime
		}
		from := min(cell(e.Time.Sub(t.Start), span, trackW), trackW-1)
		to := max(cell(stop.Sub(t.Start), span, trackW), from+1)
		track := strings.Repeat(" ", from) + strings.Repeat("━", to-from) + strings.Repeat(" ", max(trackW-to, 0))
		ks := lipgloss.NewStyle().Foreground(kindColor(e.Tool.Kind))
		if e.Tool.IsError {
			ks = sErr
		}
		name := e.Tool.Name
		if e.Tool.Kind != parse.ToolBuiltin {
			name = e.Tool.Group()
		}
		d := dur(e.Duration())
		if !e.Tool.HasResult {
			d = glyphPending
		}
		b.WriteString(" " + ks.Render(fit(name, labelW)) + " " + ks.Render(fit(track, trackW)) + sDim.Render(fmt.Sprintf(" %6s", d)) + "\n")
	}
	return b.String()
}

// cell maps an offset within span onto a boundary 0..w of a w-cell track.
func cell(off, span time.Duration, w int) int {
	if span <= 0 || off <= 0 {
		return 0
	}
	return min(int(int64(off)*int64(w)/int64(span)), w)
}
