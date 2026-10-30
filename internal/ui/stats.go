package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/stats"
)

func bar(n, maxN, w int, c lipgloss.TerminalColor) string {
	if maxN == 0 || w <= 0 {
		return strings.Repeat(" ", max(w, 0))
	}
	// Eighth-block resolution for smooth bars.
	eighths := n * w * 8 / maxN
	if n > 0 && eighths == 0 {
		eighths = 1
	}
	full, part := eighths/8, eighths%8
	s := strings.Repeat("█", full)
	if part > 0 {
		s += string([]rune("▏▎▍▌▋▊▉")[part-1])
	}
	return lipgloss.NewStyle().Foreground(c).Render(s) + strings.Repeat(" ", max(w-full-boolInt(part > 0), 0))
}

// thinBar is a bar drawn as a line, lighter than bar's blocks, with half
// cells for precision.
func thinBar(n, maxN, w int, c lipgloss.TerminalColor) string {
	if maxN == 0 || w <= 0 {
		return strings.Repeat(" ", max(w, 0))
	}
	halves := n * w * 2 / maxN
	if n > 0 && halves == 0 {
		halves = 1
	}
	s := strings.Repeat("━", halves/2)
	if halves%2 == 1 {
		s += "╸"
	}
	return lipgloss.NewStyle().Foreground(c).Render(s) + sFaint.Render(strings.Repeat("─", max(w-lipgloss.Width(s), 0)))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func renderStats(s *parse.Session, st stats.Stats, w int) string {
	var b strings.Builder
	b.WriteString(statsHeadline(s, st, w))
	b.WriteString(statsTurns(s, w))
	tools, toks := statsTools(s, st), statsTokens(s)
	if w >= 100 {
		leftW := w * 11 / 20
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(leftW).Render(tools(leftW-4)), toks(w-leftW-2)))
	} else {
		b.WriteString(tools(w-2) + toks(w-2))
	}
	b.WriteString(statsModels(s))
	return b.String()
}

// costLabel is the session's cost, marked ≈ while it's an estimate from
// tokens; "" when there is neither.
func costLabel(s *parse.Session) string {
	usd, estimated, ok := s.Cost()
	switch {
	case !ok:
		return ""
	case estimated:
		return approx(usd)
	}
	return dollars(usd)
}

// approx is an estimated amount, marked ≈ unless it's under a cent.
func approx(usd float64) string {
	if d := dollars(usd); d[0] == '$' {
		return "≈" + d
	}
	return dollars(usd)
}

// dollars shows an amount in cents, and an amount too small for a cent as
// under one.
func dollars(usd float64) string {
	if usd > 0 && usd < 0.005 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", usd)
}

// statsHeadline is four figures, bright, each with a dim caption.
func statsHeadline(s *parse.Session, st stats.Stats, w int) string {
	cost, costNote := "—", "cost"
	if _, estimated, ok := s.Cost(); ok {
		cost = costLabel(s)
		if estimated {
			costNote = "cost · estimated"
		}
	}
	u := s.Usage
	cached := ""
	if t := u.Total(); t > 0 {
		cached = fmt.Sprintf("%d%% from cache", (u.CacheRead*100+t/2)/t)
	}
	ctx, ctxNote := "—", "context"
	if s.PeakContext > 0 {
		ctx = tokens(s.Context) + " of " + tokens(s.ContextWindow())
		ctxNote = "context · peak " + tokens(s.PeakContext)
		if n := s.Compactions; n > 0 {
			ctxNote += fmt.Sprintf(" · %d %s", n, plural(n, "compaction"))
		}
	}
	tiles := [][2]string{
		{cost, costNote},
		{dur(st.Duration), fmt.Sprintf("%d %s", st.Prompts, plural(st.Prompts, "prompt"))},
		{count(u.Total()) + " tokens", cached},
		{ctx, ctxNote},
	}
	// Each tile is as wide as its text, with the room left shared out
	// between them.
	widths, used := make([]int, len(tiles)), 0
	for i, t := range tiles {
		widths[i] = max(lipgloss.Width(t[0]), lipgloss.Width(t[1]))
		used += widths[i]
	}
	gap := max((w-1-used)/len(tiles), 3)
	var top, bottom strings.Builder
	for i, t := range tiles {
		top.WriteString(sBold.Render(fit(t[0], widths[i]+gap)))
		bottom.WriteString(sDim.Render(fit(t[1], widths[i]+gap)))
	}
	return " " + top.String() + "\n " + bottom.String() + "\n"
}

// statsTurns is a table of the turns, with a bar for each one's share of
// the cost (or of the time, when cost isn't known). The costliest is
// bright.
func statsTurns(s *parse.Session, w int) string {
	if len(s.Turns) == 0 {
		return ""
	}
	weight := func(t *parse.Turn) float64 {
		if usd, ok := s.TurnCost(t); ok {
			return usd
		}
		return float64(t.Duration())
	}
	top := 0.0
	for _, t := range s.Turns {
		top = max(top, weight(t))
	}
	numW := len(fmt.Sprint(len(s.Turns)))
	barW := min(max(w/8, 6), 16)
	cols := fmt.Sprintf("%7s %8s %8s %8s", "time", "tokens", "cost", "context")
	nameW := max(w-numW-lipgloss.Width(cols)-barW-6, 12)
	var b strings.Builder
	b.WriteString(sSection.Render(" Turns") + "\n")
	b.WriteString(sDim.Render(" "+strings.Repeat(" ", numW+2+nameW)+cols) + "\n")
	for i, t := range s.Turns {
		toks, cost, ctx := "", "", ""
		if n := t.Usage.Total(); n > 0 {
			toks, ctx = count(n), tokens(t.ContextEnd)
			if usd, ok := s.TurnCost(t); ok {
				cost = costText(usd)
			}
		}
		if t.Compacted {
			ctx = glyphCompact + " " + ctx
		}
		name := sText
		if weight(t) == top && len(s.Turns) > 1 {
			name = sPrompt
		}
		b.WriteString(" " + sDim.Render(fmt.Sprintf("%*d  ", numW, i+1)) + name.Render(fit(firstLine(t.Prompt.Text), nameW)) +
			sText.Render(fmt.Sprintf("%7s %8s %8s", dur(t.Duration()), toks, cost)) + sDim.Render(fmt.Sprintf(" %8s", ctx)) +
			"  " + thinBar(int(weight(t)*1000), int(top*1000), barW, cAccent) + "\n")
	}
	return b.String()
}

type toolRow struct {
	name        string
	kind        parse.ToolKind
	n, errs     int
	took, timed time.Duration
}

// statsTools lists the tools used most, by name, with their kind's glyph
// and color, errors and average time.
func statsTools(s *parse.Session, st stats.Stats) func(w int) string {
	return func(w int) string {
		rows := map[string]*toolRow{}
		var order []*toolRow
		for _, e := range s.Events {
			t := e.Tool
			if t == nil {
				continue
			}
			name := t.Name
			if t.Kind != parse.ToolBuiltin {
				name = t.Group()
			}
			r := rows[name]
			if r == nil {
				r = &toolRow{name: name, kind: t.Kind}
				rows[name] = r
				order = append(order, r)
			}
			r.n++
			if t.IsError {
				r.errs++
			}
			if d := e.Duration(); d > 0 {
				r.took += d
				r.timed++
			}
		}
		sort.SliceStable(order, func(i, j int) bool { return order[i].n > order[j].n })
		var b strings.Builder
		if len(order) == 0 {
			return ""
		}
		head := fmt.Sprintf("%d %s", st.ToolCalls, plural(st.ToolCalls, "call"))
		if st.Errors > 0 {
			head += fmt.Sprintf(" · %d %s", st.Errors, plural(st.Errors, "error"))
		}
		b.WriteString(sSection.Render(" Tools") + sDim.Render("  "+head+" · "+dur(st.ToolTime)) + "\n")
		nameW := 8
		for _, r := range order {
			nameW = max(nameW, lipgloss.Width(r.name)+2)
		}
		nameW = min(nameW, max(w/3, 12))
		barW := max(w-nameW-26, 4)
		for i, r := range order {
			if i == 7 {
				b.WriteString(sDim.Render(fmt.Sprintf("   and %d more in the Tools tab", len(order)-7)) + "\n")
				break
			}
			ks := lipgloss.NewStyle().Foreground(kindColor(r.kind))
			errs := "    "
			if r.errs > 0 {
				errs = sErr.Render(fmt.Sprintf(" %2d%s", r.errs, glyphErr))
			}
			avg := ""
			if r.timed > 0 {
				avg = dur(r.took / time.Duration(r.timed))
			}
			b.WriteString(" " + ks.Render(kindGlyph(r.kind)+" ") + sText.Render(fit(r.name, nameW)) +
				sText.Render(fmt.Sprintf("%4d ", r.n)) + thinBar(r.n, order[0].n, barW, kindColor(r.kind)) + errs +
				sDim.Render(fmt.Sprintf(" %6s", avg)) + "\n")
		}
		return b.String()
	}
}

// statsTokens shows where the tokens went.
func statsTokens(s *parse.Session) func(w int) string {
	return func(w int) string {
		u := s.Usage
		rows := []struct {
			name string
			n    int
			note string
		}{
			{"input", u.Input, ""},
			{"output", u.Output, ""},
			{"cache read", u.CacheRead, ""},
			{"cache write", u.CacheCreate, ""},
		}
		if u.Thinking > 0 {
			rows[1].note = count(u.Thinking) + " thinking"
		}
		top := 0
		for _, r := range rows {
			top = max(top, r.n)
		}
		var b strings.Builder
		b.WriteString(sSection.Render(" Tokens") + sDim.Render(fmt.Sprintf("  %d API %s", s.Requests, plural(s.Requests, "request"))) + "\n")
		barW := min(max(w-24, 4), 40)
		for _, r := range rows {
			b.WriteString(" " + sDim.Render(fit(r.name, 12)) + sText.Render(fmt.Sprintf("%7s ", count(r.n))) +
				thinBar(r.n, top, barW, cAccent) + sDim.Render(" "+r.note) + "\n")
		}
		return b.String()
	}
}

// statsModels lists each model with its requests and cost, most used
// first, then the session's pull request.
func statsModels(s *parse.Session) string {
	var b strings.Builder
	if len(s.Models) > 0 {
		var models []string
		nameW := 0
		for m := range s.Models {
			models = append(models, m)
			nameW = max(nameW, lipgloss.Width(m))
		}
		sort.Slice(models, func(i, j int) bool {
			if s.Models[models[i]] != s.Models[models[j]] {
				return s.Models[models[i]] > s.Models[models[j]]
			}
			return models[i] < models[j]
		})
		_, estimated, priced := s.Cost()
		b.WriteString(sSection.Render(" Models") + "\n")
		for _, m := range models {
			n := s.Models[m]
			line := " " + sText.Render(fit(m, nameW+3)) + sDim.Render(fmt.Sprintf("%5d %s", n, plural(n, "request")))
			if usd := s.ModelCostUSD(m); priced && usd > 0 {
				c := dollars(usd)
				if estimated {
					c = approx(usd)
				}
				line += sText.Render(fmt.Sprintf("   %9s", c))
			}
			b.WriteString(line + "\n")
		}
	}
	if s.PRURL != "" {
		b.WriteString(sSection.Render(" Pull request") + "\n " + sText.Render(s.PRURL) + "\n")
	}
	return b.String()
}

// mixSegments splits a bar of width w between tool kinds in proportion to
// their counts. Kinds with calls get at least one cell, kinds without get
// none, and rounding leftovers go to the largest kind.
func mixSegments(byKind [4]int, w int) [4]int {
	var segs [4]int
	total, largest := 0, 0
	for k, n := range byKind {
		total += n
		if n > byKind[largest] {
			largest = k
		}
	}
	if total == 0 {
		return segs
	}
	used := 0
	for k, n := range byKind {
		if n > 0 {
			segs[k] = max(n*w/total, 1)
			used += segs[k]
		}
	}
	segs[largest] = max(segs[largest]+w-used, 1)
	return segs
}
