package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
)

// groupNode is a row in the Tools tab tree: a tool kind (depth 0), a name
// within it such as a skill or subagent type (depth 1), or one call (depth 2).
type groupNode struct {
	key      string // stable across rebuilds, for expansion and selection
	depth    int
	label    string
	kind     parse.ToolKind
	children []*groupNode
	ev       *parse.Event // call rows only

	n, errs int
	time    time.Duration // summed over calls with a known duration
	timed   int
}

func (g *groupNode) add(e *parse.Event) {
	g.n++
	if e.Tool.IsError {
		g.errs++
	}
	if d := e.Duration(); d > 0 {
		g.time += d
		g.timed++
	}
}

func (g *groupNode) avg() time.Duration {
	if g.timed == 0 {
		return 0
	}
	return g.time / time.Duration(g.timed)
}

var kindTitles = [...]string{"Built-in tools", "Skills", "MCP", "Subagents"}

// buildGroups groups the tool calls that pass keep by kind, then name.
// Kinds keep their fixed order, names are sorted by call count, and calls
// stay in time order.
func buildGroups(events []*parse.Event, keep func(*parse.Event) bool) []*groupNode {
	var roots [len(kindTitles)]*groupNode
	names := map[string]*groupNode{}
	for _, e := range events {
		t := e.Tool
		if t == nil || !keep(e) {
			continue
		}
		r := roots[t.Kind]
		if r == nil {
			r = &groupNode{key: fmt.Sprintf("k%d", t.Kind), label: kindTitles[t.Kind], kind: t.Kind}
			roots[t.Kind] = r
		}
		name := t.Group()
		g := names[r.key+"/"+name]
		if g == nil {
			g = &groupNode{key: r.key + "/" + name, depth: 1, label: name, kind: t.Kind}
			names[g.key] = g
			r.children = append(r.children, g)
		}
		c := &groupNode{key: "c/" + t.ID, depth: 2, label: t.Summary, kind: t.Kind, ev: e}
		g.children = append(g.children, c)
		for _, n := range []*groupNode{r, g, c} {
			n.add(e)
		}
	}
	var out []*groupNode
	for _, r := range roots {
		if r == nil {
			continue
		}
		sort.SliceStable(r.children, func(i, j int) bool {
			a, b := r.children[i], r.children[j]
			if a.n != b.n {
				return a.n > b.n
			}
			return a.label < b.label
		})
		out = append(out, r)
	}
	return out
}

// isOpen reports whether a group shows its children. Kinds start open and
// names closed; while searching everything is open so matches are visible.
func (v *view) isOpen(g *groupNode) bool {
	return v.search.Value() != "" || v.savedOpen(g)
}

// savedOpen is the expansion the user chose, ignoring any search.
func (v *view) savedOpen(g *groupNode) bool {
	if open, ok := v.gOpen[g.key]; ok {
		return open
	}
	return g.depth == 0
}

// rebuildGroups regroups the session's calls (honouring the search) and
// keeps the same row selected when it still exists.
func (v *view) rebuildGroups() {
	var selKey string
	if g := v.selectedGroup(); g != nil {
		selKey = g.key
	}
	terms := strings.Fields(strings.ToLower(v.search.Value()))
	v.groups = buildGroups(v.sess.Events, func(e *parse.Event) bool { return v.matches(e, terms) })
	v.flattenGroups(selKey)
}

func (v *view) flattenGroups(selKey string) {
	v.gRows = v.gRows[:0]
	var walk func([]*groupNode)
	walk = func(ns []*groupNode) {
		for _, n := range ns {
			v.gRows = append(v.gRows, n)
			if len(n.children) > 0 && v.isOpen(n) {
				walk(n.children)
			}
		}
	}
	walk(v.groups)
	for i, n := range v.gRows {
		if n.key == selKey {
			v.gCursor = i
			return
		}
	}
	v.gCursor = max(0, min(v.gCursor, len(v.gRows)-1))
}

func (v *view) selectedGroup() *groupNode {
	if v.gCursor < len(v.gRows) {
		return v.gRows[v.gCursor]
	}
	return nil
}

func (v *view) setOpen(g *groupNode, open bool) {
	if len(g.children) == 0 {
		return
	}
	v.gOpen[g.key] = open
	v.flattenGroups(g.key)
}

func (v *view) moveGroup(d int) {
	v.gCursor = max(0, min(v.gCursor+d, len(v.gRows)-1))
}

// jumpTo shows an event in the timeline, clearing the kind filter and the
// search if they would hide it. An event still hidden (thinking, while
// thinking is off) selects the last shown event before it.
func (v *view) jumpTo(e *parse.Event) {
	v.tab = tabTimeline
	v.follow = false
	if v.selectShown(e) {
		return
	}
	v.kind = 0
	v.typing = false
	v.search.Blur()
	v.search.SetValue("")
	v.refresh()
	if v.selectShown(e) {
		return
	}
	for i, x := range v.shown {
		if v.depth[x] == 0 && !x.Time.After(e.Time) {
			v.cursor = i
		}
	}
}

func (v *view) selectShown(e *parse.Event) bool {
	for i, x := range v.shown {
		if x == e {
			v.cursor = i
			return true
		}
	}
	return false
}

func (v *view) updateTools(k tea.KeyMsg, w, bodyH int) tea.Cmd {
	g := v.selectedGroup()
	switch k.String() {
	case "up", "k":
		v.moveGroup(-1)
	case "down", "j":
		v.moveGroup(1)
	case "pgup", "ctrl+b":
		v.moveGroup(-bodyH)
	case "pgdown", "ctrl+f":
		v.moveGroup(bodyH)
	case "g", "home":
		v.moveGroup(-len(v.gRows))
	case "G", "end":
		v.moveGroup(len(v.gRows))
	case "ctrl+d", "J":
		v.detail.HalfViewDown()
	case "ctrl+u", "K":
		v.detail.HalfViewUp()
	case "enter", " ":
		switch {
		case g == nil:
		case g.ev != nil && k.String() == "enter":
			v.jumpTo(g.ev)
		case g.ev == nil:
			v.setOpen(g, !v.isOpen(g))
		}
	case "right", "l":
		if g != nil && g.ev == nil && !v.isOpen(g) {
			v.setOpen(g, true)
		} else {
			v.focusDetail(w)
		}
	case "left", "h":
		switch {
		case g == nil:
		case g.ev == nil && v.isOpen(g):
			v.setOpen(g, false)
		default:
			v.toParent(g)
		}
	case "tab":
		v.focusDetail(w)
	case "E":
		// Expand every group, or once all are open, reset to the default
		// view. Decided from the saved state, since a search shows every
		// group open; expanding during a search keeps other groups' state.
		var all []*groupNode
		var walk func([]*groupNode)
		walk = func(ns []*groupNode) {
			for _, n := range ns {
				if n.ev == nil && len(n.children) > 0 {
					all = append(all, n)
					walk(n.children)
				}
			}
		}
		walk(v.groups)
		expand := false
		for _, n := range all {
			if !v.savedOpen(n) {
				expand = true
			}
		}
		if expand {
			for _, n := range all {
				v.gOpen[n.key] = true
			}
		} else {
			v.gOpen = map[string]bool{}
		}
		var sel string
		if g != nil {
			sel = g.key
		}
		v.flattenGroups(sel)
	case "/":
		v.typing = true
		return v.search.Focus()
	case "a", "o":
		if g != nil && g.ev != nil {
			return v.openSubagent(g.ev)
		}
	}
	return nil
}

// toParent moves the cursor to the group containing g.
func (v *view) toParent(g *groupNode) {
	for i := v.gCursor - 1; i >= 0; i-- {
		if v.gRows[i].depth < g.depth {
			v.gCursor = i
			return
		}
	}
}

func (v *view) focusDetail(w int) {
	if w >= wideLayout {
		v.focusR = true
	} else {
		v.zoom = true
	}
}

func (v *view) toolsView(w, h int) string {
	if v.zoom && w < wideLayout {
		return v.detailPane(w, h)
	}
	listW := w
	if w >= wideLayout {
		listW = w * 11 / 20
	}
	v.gOffset = scrollOffset(v.gOffset, v.gCursor, len(v.gRows), h)
	lines := make([]string, 0, h)
	for i := v.gOffset; i < len(v.gRows) && len(lines) < h; i++ {
		lines = append(lines, v.groupRow(v.gRows[i], i == v.gCursor, listW))
	}
	if len(v.gRows) == 0 {
		msg := " no tool calls yet"
		if v.search.Value() != "" {
			msg = " no tool calls match"
		}
		lines = append(lines, sDim.Render(msg))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", listW))
	}
	list := strings.Join(lines, "\n")
	if w < wideLayout {
		return list
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, list, v.detailPane(w-listW, h))
}

func (v *view) groupRow(g *groupNode, sel bool, w int) string {
	if g.ev != nil {
		// Calls render like timeline rows, indented under their group.
		indent := strings.Repeat(" ", 4)
		if sel {
			indent = lipgloss.NewStyle().Background(cSelBg).Render(indent)
		}
		return indent + renderRowName(g.ev, sel, w-4, rowOpts{hideName: true})
	}
	var bg lipgloss.TerminalColor = lipgloss.NoColor{}
	if sel {
		bg = cSelBg
	}
	st := func(s lipgloss.Style) lipgloss.Style { return s.Background(bg) }
	ks := lipgloss.NewStyle().Foreground(kindColor(g.kind))
	mark := st(sText).Render(" ")
	if sel {
		mark = st(sAccent).Render("▌")
	}
	chevron := "▹"
	if v.isOpen(g) {
		chevron = "▿"
	}
	indent := strings.Repeat("  ", g.depth)
	left := mark + st(sText).Render(indent) + st(sDim).Render(chevron+" ")
	label := g.label
	labelStyle := ks
	if g.depth == 0 {
		left += st(ks).Render(kindGlyph(g.kind) + " ")
		labelStyle = ks.Bold(true)
	}
	errs := st(sFaint).Render(fmt.Sprintf(" %4s", "·"))
	if g.errs > 0 {
		errs = st(sErr).Render(fmt.Sprintf(" %3d%s", g.errs, glyphErr))
	}
	right := st(sText).Render(fmt.Sprintf(" %5d", g.n)) + errs +
		st(sDim).Render(fmt.Sprintf(" %7s ", dur(g.avg())))
	labelW := w - lipgloss.Width(left) - lipgloss.Width(right)
	line := left + st(labelStyle).Render(fit(label, labelW)) + right
	if pad := w - lipgloss.Width(line); pad > 0 {
		line += st(sText).Render(strings.Repeat(" ", pad))
	}
	return fit(line, w)
}

// renderGroupDetail summarises a kind or name group for the detail pane.
func renderGroupDetail(g *groupNode, w int) string {
	var b strings.Builder
	ks := lipgloss.NewStyle().Foreground(kindColor(g.kind)).Bold(true)
	b.WriteString(ks.Render(kindGlyph(g.kind)+" "+g.label) + "\n")
	stats := fmt.Sprintf("%d calls", g.n)
	if g.errs > 0 {
		stats += " · " + sErr.Render(fmt.Sprintf("%d errors", g.errs))
	}
	if g.timed > 0 {
		stats += sDim.Render(" · avg " + dur(g.avg()) + " · total " + dur(g.time))
	}
	b.WriteString(sDim.Render(stats) + "\n")

	if g.depth == 0 {
		b.WriteString(section("By name") + "\n")
		nameW := min(max(w/2, 12), 40)
		barW := max(w-nameW-8, 4)
		for _, c := range g.children {
			b.WriteString(lipgloss.NewStyle().Foreground(kindColor(g.kind)).Render(fit(c.label, nameW)) + " " +
				bar(c.n, g.children[0].n, barW, kindColor(g.kind)) + sText.Render(fmt.Sprintf(" %5d", c.n)) + "\n")
		}
		return b.String()
	}

	b.WriteString(section("Recent calls") + "\n")
	calls := g.children
	if len(calls) > 15 {
		calls = calls[len(calls)-15:]
	}
	for i := len(calls) - 1; i >= 0; i-- {
		e := calls[i].ev
		status := sOK.Render(glyphOK)
		switch {
		case e.Tool.IsError:
			status = sErr.Render(glyphErr)
		case !e.Tool.HasResult:
			status = sDim.Render(glyphPending)
		}
		ts := "        "
		if !e.Time.IsZero() {
			ts = e.Time.Local().Format("15:04:05")
		}
		b.WriteString(status + " " + sFaint.Render(ts) + " " + sText.Render(fit(e.Tool.Summary, w-12)) + "\n")
	}
	b.WriteString("\n" + sKey.Render("enter") + sDim.Render(" expand · ") + sKey.Render("E") + sDim.Render(" expand all"))
	return b.String()
}
