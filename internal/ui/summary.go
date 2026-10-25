package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/store"
	"github.com/jzills/snoop/internal/summary"
)

// The picker's summary (s) totals the sessions it currently lists. Every
// transcript is summarised in the background, from the cache when current.

const summaryDays = 30

type scanStartedMsg struct{ ch <-chan summary.Result }
type scanResultMsg struct {
	r  summary.Result
	ch <-chan summary.Result
}
type scanDoneMsg struct{}

func startScan(infos []store.SessionInfo, c *summary.Cache) tea.Cmd {
	return func() tea.Msg { return scanStartedMsg{summary.Scan(infos, c)} }
}

func waitScan(ch <-chan summary.Result) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return scanDoneMsg{}
		}
		return scanResultMsg{r, ch}
	}
}

// onScan applies scan progress; it runs whatever screen is showing.
func (p *picker) onScan(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case scanStartedMsg:
		return waitScan(msg.ch)
	case scanResultMsg:
		p.scanned++
		if msg.r.Err != nil {
			p.bad[msg.r.Path] = true
		} else {
			p.sums[msg.r.Path] = msg.r.Sum
			delete(p.bad, msg.r.Path)
		}
		// Find results fill in as sessions are indexed, a batch at a time.
		if p.find && p.scanned%findBatch == 0 {
			p.refilter()
		}
		// The cache is saved as the scan goes, so quitting part way through
		// keeps what was parsed.
		if now := time.Now(); now.Sub(p.saved) >= cacheSaveEvery {
			p.saved = now
			c, all := p.cache, p.all
			return tea.Batch(waitScan(msg.ch), func() tea.Msg { c.Save(all); return nil })
		}
		return waitScan(msg.ch)
	case scanDoneMsg:
		p.scanning = false
		p.cache.Save(p.all)
		if p.find {
			p.refilter()
		}
	}
	return nil
}

// toggleSummary switches between the list and the summary, starting a
// scan (cheap once cached) each time the summary opens.
func (p *picker) toggleSummary() tea.Cmd {
	p.summary = !p.summary
	p.page.GotoTop()
	if !p.summary {
		return nil
	}
	return p.startScan()
}

// cacheSaveEvery is how often a running scan saves the cache.
const cacheSaveEvery = 5 * time.Second

// save writes the summaries scanned so far.
func (p *picker) save() {
	if p.cache != nil && p.loaded {
		p.cache.Save(p.all)
	}
}

// findBatch is how many scanned sessions update find's results at once.
const findBatch = 25

// startScan summarises every session in the background, unless a scan is
// running; once cached, a scan only stats the files.
func (p *picker) startScan() tea.Cmd {
	if p.scanning || !p.loaded {
		return nil
	}
	p.scanning, p.scanned, p.saved = true, 0, time.Now()
	return startScan(p.all, p.cache)
}

func (p *picker) summaryView(w, h int) string {
	var sums []summary.Session
	bad := 0
	for _, i := range p.shown {
		path := p.all[i].Path
		if s, ok := p.sums[path]; ok {
			sums = append(sums, s)
		} else if p.bad[path] {
			bad++
		}
	}
	p.page.Width, p.page.Height = w, h
	p.page.SetContent(renderSummary(summary.Total(sums, time.Now(), summaryDays), bad, w-2))
	return p.page.View()
}

func (p *picker) scanProgress() string {
	if !p.scanning {
		return ""
	}
	return sAccent.Render(fmt.Sprintf("scanning %d/%d", p.scanned, len(p.all)))
}

func renderSummary(t summary.Totals, bad, w int) string {
	var b strings.Builder
	kv := func(k, v string) string { return sDim.Render(fmt.Sprintf(" %-12s", k)) + sText.Render(v) }
	sessions := fmt.Sprint(t.Sessions)
	if bad > 0 {
		sessions += sErr.Render(fmt.Sprintf(" (%d unreadable)", bad))
	}
	cost := "—"
	if t.CostKnown > 0 {
		cost = fmt.Sprintf("$%.2f", t.Cost)
		if t.CostKnown < t.Sessions {
			cost += sDim.Render(fmt.Sprintf(" (%d of %d sessions record cost)", t.CostKnown, t.Sessions))
		}
	}
	errRate := ""
	if t.ToolCalls > 0 {
		errRate = fmt.Sprintf(" (%.1f%%)", 100*float64(t.Errors)/float64(t.ToolCalls))
	}
	left := strings.Join([]string{kv("Sessions", sessions), kv("Prompts", fmt.Sprint(t.Prompts)),
		kv("Tool calls", fmt.Sprint(t.ToolCalls)), kv("Errors", fmt.Sprint(t.Errors)+errRate)}, "\n")
	right := strings.Join([]string{kv("Cost", cost), kv("Tokens", count(t.Tokens))}, "\n")
	b.WriteString(section(" Overview") + "\n")
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(max(w/2, 36)).Render(left), right) + "\n")

	// Daily cost, or tokens when no session in range recorded cost.
	maxCost, maxTok := 0.0, 0
	for _, d := range t.Days {
		maxCost, maxTok = max(maxCost, d.Cost), max(maxTok, d.Tokens)
	}
	if maxTok > 0 {
		title := fmt.Sprintf(" Daily cost (last %d days, ≈ split by tokens)", len(t.Days))
		if maxCost == 0 {
			title = fmt.Sprintf(" Daily tokens (last %d days)", len(t.Days))
		}
		b.WriteString(section(title) + "\n")
		barW := max(w-30, 6)
		for _, d := range t.Days {
			n, top := d.Tokens, maxTok
			if maxCost > 0 {
				n, top = int(d.Cost*100), int(maxCost*100)
			}
			cost := ""
			if d.Cost > 0 {
				cost = costText(d.Cost)
			}
			tok := ""
			if d.Tokens > 0 {
				tok = count(d.Tokens)
			}
			b.WriteString(sDim.Render(" "+d.Date.Format("Mon Jan 02")) + " " + bar(n, top, barW, cAccent) +
				sText.Render(fmt.Sprintf(" %8s", cost)) + sDim.Render(fmt.Sprintf(" %7s", tok)) + "\n")
		}
	}

	list := func(title string, ns []summary.Named, c lipgloss.TerminalColor, errs bool) {
		if len(ns) == 0 {
			return
		}
		b.WriteString(section(" "+title) + "\n")
		nameW := min(max(w/3, 20), 40)
		barW := max(w-nameW-22, 6)
		for _, x := range ns[:min(len(ns), 12)] {
			e := ""
			if errs && x.Errors > 0 {
				rate := fmt.Sprintf("%.0f%%", 100*float64(x.Errors)/float64(x.N))
				if 100*x.Errors < x.N {
					rate = "<1%"
				}
				e = sErr.Render(fmt.Sprintf(" %d✗ %s", x.Errors, rate))
			}
			b.WriteString(" " + lipgloss.NewStyle().Foreground(c).Render(fit(x.Name, nameW)) + " " +
				bar(x.N, ns[0].N, barW, c) + sText.Render(fmt.Sprintf(" %6d", x.N)) + e + "\n")
		}
	}
	list("Tools", t.Tools, cBuiltin, true)
	list("Skills", t.Skills, cSkill, true)
	list("MCP servers", t.MCP, cMCP, true)
	list("Subagents", t.Agents, cAgent, true)
	list("Models (API requests)", t.Models, cAccent, false)
	return b.String()
}
