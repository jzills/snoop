package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
)

// tokens formats a token count compactly for the meter: 950, 142k, 1M.
func tokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 1_000_000:
		return fmt.Sprintf("%dk", (n+500)/1000)
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e6), ".0") + "M"
}

// contextMeter renders "ctx 142k/200k ▰▰▰▰▰▰▰▱▱▱ 71%", or "ctx 71%" when
// narrower than 28 cells; empty before the first request.
func contextMeter(used, window, w int) string {
	if used <= 0 || window <= 0 {
		return ""
	}
	pct := used * 100 / window
	st := sDim
	switch {
	case pct >= 90:
		st = sErr
	case pct >= 70:
		st = sAccent
	}
	if w < 28 {
		return st.Render(fmt.Sprintf("ctx %d%%", pct))
	}
	filled := min((used*10+window/2)/window, 10)
	bar := strings.Repeat("▰", filled) + strings.Repeat("▱", 10-filled)
	return st.Render(fmt.Sprintf("ctx %s/%s %s %d%%", tokens(used), tokens(window), bar, pct))
}

func costText(usd float64) string { return fmt.Sprintf("≈$%.2f", usd) }

// turnSummary is the plain "48.2k · ≈$0.31 · 2m10s" shown on a prompt row,
// dropping cost below 22 cells and tokens below 12.
func turnSummary(s *parse.Session, t *parse.Turn, w int) string {
	var parts []string
	if n := t.Usage.Total(); n > 0 && w >= 12 {
		parts = append(parts, count(n))
		if usd, ok := s.TurnCost(t); ok && w >= 22 {
			parts = append(parts, costText(usd))
		}
	}
	if d := dur(t.Duration()); d != "" {
		parts = append(parts, d)
	}
	text := strings.Join(parts, " · ")
	if lipgloss.Width(text) > w {
		return fit(text, w)
	}
	return text
}

// compactText describes a compaction: "compacted (auto) 468k → 31k".
func compactText(c *parse.Compaction) string {
	if c == nil {
		return "compacted"
	}
	s := "compacted"
	if c.Trigger != "" {
		s += " (" + c.Trigger + ")"
	}
	if c.Pre > 0 {
		s += " " + tokens(c.Pre)
		if c.Post > 0 {
			s += " → " + tokens(c.Post)
		}
	}
	return s
}

// renderTurns is the Stats tab's per-prompt breakdown.
func renderTurns(s *parse.Session, w int) string {
	if len(s.Turns) == 0 {
		return ""
	}
	var longest int64
	for _, t := range s.Turns {
		longest = max(longest, int64(t.Duration()))
	}
	nameW := min(max(w/3, 16), 40)
	barW := max(w-nameW-38, 6)
	var b strings.Builder
	b.WriteString(section(" Turns") + "\n")
	for _, t := range s.Turns {
		// Turns without API requests (slash commands) leave the cells blank.
		toks, cost, ctx := "", "", ""
		if n := t.Usage.Total(); n > 0 {
			toks, ctx = count(n), tokens(t.ContextEnd)
			if usd, ok := s.TurnCost(t); ok {
				cost = costText(usd)
			}
		}
		mark := ""
		if t.Compacted {
			mark = sDim.Render(" " + glyphCompact)
		}
		b.WriteString(" " + sPrompt.Render(fit(firstLine(t.Prompt.Text), nameW)) + " " +
			bar(int(t.Duration()/1e6), int(longest/1e6), barW, cAccent) +
			sText.Render(fmt.Sprintf(" %7s %7s %7s", dur(t.Duration()), toks, cost)) +
			sDim.Render(fmt.Sprintf(" %5s", ctx)) + mark + "\n")
	}
	return b.String()
}

// contextLine summarises context use for the Stats overview.
func contextLine(s *parse.Session) string {
	if s.PeakContext == 0 {
		return "—"
	}
	out := fmt.Sprintf("%s / %s · peak %s", tokens(s.Context), tokens(s.ContextWindow()), tokens(s.PeakContext))
	if n := s.Compactions; n > 0 {
		out += fmt.Sprintf(" · %d %s", n, plural(n, "compaction"))
	}
	return out
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
