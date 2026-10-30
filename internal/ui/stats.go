package ui

import (
	"fmt"
	"sort"
	"strings"

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

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func renderStats(s *parse.Session, st stats.Stats, w int) string {
	var b strings.Builder
	kv := func(k, v string) string { return sDim.Render(fmt.Sprintf(" %-14s", k)) + sText.Render(v) }
	col := func(lines ...string) string { return strings.Join(lines, "\n") }

	cost := "—"
	if s.CostUSD > 0 {
		cost = fmt.Sprintf("$%.2f", s.CostUSD)
	}
	errRate := ""
	if st.ToolCalls > 0 {
		errRate = fmt.Sprintf(" (%.1f%%)", 100*float64(st.Errors)/float64(st.ToolCalls))
	}
	left := col(
		kv("Duration", dur(st.Duration)),
		kv("Prompts", fmt.Sprint(st.Prompts)),
		kv("Replies", fmt.Sprint(st.Replies)),
		kv("API requests", fmt.Sprint(s.Requests)),
		kv("Tool calls", fmt.Sprint(st.ToolCalls)),
		kv("Errors", fmt.Sprint(st.Errors)+errRate),
		kv("Tool time", dur(st.ToolTime)),
	)
	u := s.Usage
	right := col(
		kv("Cost", cost),
		kv("Input", count(u.Input)),
		kv("Output", count(u.Output)+sDim.Render(" ("+count(u.Thinking)+" thinking)")),
		kv("Cache read", count(u.CacheRead)),
		kv("Cache write", count(u.CacheCreate)),
		kv("Total tokens", count(u.Total())),
		kv("Context", contextLine(s)),
		kv("Version", s.Version),
	)
	// The first heading needs no margin: the tabs already end in a blank
	// line.
	b.WriteString(sSection.MarginTop(0).Render(" Overview") + "\n")
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(max(w/2, 36)).Render(left), right) + "\n")

	if st.ToolCalls > 0 {
		b.WriteString(section(" Tool mix") + "\n")
		barW := max(w-4, 10)
		var stack strings.Builder
		for k, seg := range mixSegments(st.ByKind, barW) {
			stack.WriteString(lipgloss.NewStyle().Foreground(kindColor(parse.ToolKind(k))).Render(strings.Repeat("█", seg)))
		}
		b.WriteString(" " + stack.String() + "\n ")
		for k := parse.ToolBuiltin; k <= parse.ToolAgent; k++ {
			ks := lipgloss.NewStyle().Foreground(kindColor(k))
			b.WriteString(ks.Render(kindGlyph(k)+" "+k.String()) + sDim.Render(fmt.Sprintf(" %d   ", st.ByKind[k])))
		}
		b.WriteString("\n")
	}

	list := func(title string, cs []stats.Count, c lipgloss.TerminalColor) {
		if len(cs) == 0 {
			return
		}
		b.WriteString(section(" "+title) + "\n")
		nameW := min(max(w/3, 20), 40)
		barW := max(w-nameW-12, 6)
		for _, x := range cs {
			errs := ""
			if x.Errors > 0 {
				errs = sErr.Render(fmt.Sprintf(" %d✗", x.Errors))
			}
			b.WriteString(" " + lipgloss.NewStyle().Foreground(c).Render(fit(x.Name, nameW)) + " " +
				bar(x.N, cs[0].N, barW, c) + sText.Render(fmt.Sprintf(" %4d", x.N)) + errs + "\n")
		}
	}
	list("Skills", st.Skills, cSkill)
	list("MCP servers", st.MCP, cMCP)
	list("Subagents", st.Agents, cAgent)

	if len(s.Models) > 0 {
		var models []stats.Count
		for m, n := range s.Models {
			models = append(models, stats.Count{Name: m, N: n})
		}
		sort.Slice(models, func(i, j int) bool {
			if models[i].N != models[j].N {
				return models[i].N > models[j].N
			}
			return models[i].Name < models[j].Name
		})
		list("Models (API requests)", models, cAccent)
	}
	b.WriteString(renderTurns(s, w))
	if s.PRURL != "" {
		b.WriteString(section(" Pull request") + "\n " + sText.Render(s.PRURL) + "\n")
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
