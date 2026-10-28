package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/stats"
	"github.com/jzills/snoop/internal/store"
	"github.com/jzills/snoop/internal/tail"
)

const (
	tabTimeline = iota
	tabTools
	tabStats
	tabFiles
	tabDiff
)

var tabNames = []string{"Timeline", "Tools", "Stats", "Files", "Diff"}

// Kind filters, cycled with f.
var kindFilters = []struct {
	name  string
	match func(*parse.Event) bool
}{
	{"all", func(*parse.Event) bool { return true }},
	{"chat", func(e *parse.Event) bool { return e.Kind == parse.KindUser || e.Kind == parse.KindText }},
	{"tools", func(e *parse.Event) bool { return e.Tool != nil }},
	{"skills", func(e *parse.Event) bool { return e.Tool != nil && e.Tool.Kind == parse.ToolSkill }},
	{"mcp", func(e *parse.Event) bool { return e.Tool != nil && e.Tool.Kind == parse.ToolMCP }},
	{"agents", func(e *parse.Event) bool { return e.Tool != nil && e.Tool.Kind == parse.ToolAgent }},
	{"errors", func(e *parse.Event) bool { return e.Tool != nil && e.Tool.IsError }},
}

type tailMsg struct {
	v *view
	u tail.Update
}

type view struct {
	path     string
	rootPath string // top-level session transcript, for locating subagents
	label    string // subagent description, empty for top-level sessions
	sess     *parse.Session
	stats    stats.Stats

	shown  []*parse.Event
	hay    map[*parse.Event]hayEntry
	cursor int
	offset int
	follow bool // keep the cursor on the newest event
	tab    int
	kind   int
	think  bool
	search textinput.Model
	typing bool
	focusR bool // detail pane has focus
	zoom   bool // narrow terminals: detail fills the body
	detail viewport.Model
	page   viewport.Model // Tools and Stats tabs

	// What the detail pane currently shows, to avoid needless re-renders
	// and to keep the scroll position while the same event stays selected.
	detailEv    *parse.Event
	detailW     int
	detailDone  bool
	detailAgent bool   // subagent finished, for Agent rows
	detailGroup string // key of a group shown in the Tools tab
	detailGSig  string // its counts, which change as calls arrive

	// Tools tab: calls grouped by kind and name.
	groups  []*groupNode
	gRows   []*groupNode // visible rows
	gCursor int
	gOffset int
	gOpen   map[string]bool

	// Files tab: files touched, rebuilt when dirty or every filesTTL.
	files      []*fileEntry
	fCursor    int
	fOffset    int
	fMarks     []int // detail pane line of each change, for n/N
	filesBuilt time.Time
	filesDirty bool
	subs       map[string]*store.Follower

	// Subagents shown inline in the timeline.
	expanded map[string]bool       // by agent id
	depth    map[*parse.Event]int  // nesting of shown subagent events
	marked   map[*parse.Event]bool // collapsed calls whose subagent matches the search

	tl       *tail.Tailer
	lastLive time.Time

	// Alerts: the newest is shown in the header until a key is pressed.
	alert      *alert
	alertShown bool
	flash      string // replaces the footer until the next key
	watch

	// Subagent metadata is cached briefly so redraws don't read meta.json.
	agents map[string]agentEntry

	// Diff tab: the working tree's changes and review comments.
	rev reviewState
}

type agentEntry struct {
	meta store.SubagentMeta
	ok   bool
	at   time.Time
}

const agentMetaTTL = 2 * time.Second

// subagent looks up a subagent's metadata, re-reading it from disk at most
// every agentMetaTTL, or after new transcript lines arrive.
func (v *view) subagent(id string) (store.SubagentMeta, bool) {
	if c, hit := v.agents[id]; hit && time.Since(c.at) < agentMetaTTL {
		return c.meta, c.ok
	}
	meta, ok := store.Subagent(v.rootPath, id)
	v.agents[id] = agentEntry{meta, ok, time.Now()}
	return meta, ok
}

func openView(path, rootPath, label string, followNew bool) (*view, error) {
	s, off, err := store.Load(path)
	if err != nil {
		return nil, err
	}
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.PromptStyle = sKey
	ti.Placeholder = "search text, commands, paths, results"
	v := &view{
		path: path, rootPath: rootPath, label: label, sess: s,
		hay: map[*parse.Event]hayEntry{}, search: ti, follow: true, gOpen: map[string]bool{}, agents: map[string]agentEntry{}, subs: map[string]*store.Follower{}, expanded: map[string]bool{}, watch: newWatch(rootPath), rev: newReviewState(),
		detail: viewport.New(0, 0), page: viewport.New(0, 0),
	}
	v.tl, _ = tail.Open(path, off, followNew)
	v.stats = stats.Compute(s)
	v.refresh()
	v.cursor = max(len(v.shown)-1, 0)
	return v, nil
}

// own marks alerts as this session's, for notifications.
func (v *view) own(as []*alert) []*alert {
	name := v.label
	if name == "" {
		name = v.sess.Title
	}
	if name == "" && len(v.sess.Turns) > 0 {
		name = v.sess.Turns[0].Prompt.Text
	}
	if name == "" {
		name = v.sess.ID
	}
	for _, a := range as {
		a.path, a.title = v.path, noteTitle(v.project(), name)
	}
	return as
}

// project names the session's project in notifications.
func (v *view) project() string {
	if v.sess.CWD != "" {
		return filepath.Base(v.sess.CWD)
	}
	return ""
}

func (v *view) close() {
	if v.tl != nil {
		v.tl.Close()
	}
}

func (v *view) waitTail() tea.Cmd {
	if v.tl == nil {
		return nil
	}
	return func() tea.Msg { return tailMsg{v, v.tl.Wait()} }
}

// refresh recomputes the visible events after a filter or data change.
func (v *view) refresh() {
	var sel *parse.Event
	if v.cursor < len(v.shown) {
		sel = v.shown[v.cursor]
	}
	terms := strings.Fields(strings.ToLower(v.search.Value()))
	match := kindFilters[v.kind].match
	filtering := v.kind != 0 || len(terms) > 0
	keep := func(e *parse.Event) bool {
		if e.Kind == parse.KindThinking && !v.think && !filtering {
			return false
		}
		return match(e) && v.matches(e, terms)
	}
	v.shown = v.shown[:0]
	v.depth, v.marked = map[*parse.Event]int{}, map[*parse.Event]bool{}
	// Expanded subagents' events follow their call, indented. While
	// filtering, a call whose subagent has matches is kept (and marked when
	// collapsed) even if the call itself doesn't match.
	var walk func(events []*parse.Event, depth int)
	walk = func(events []*parse.Event, depth int) {
		for _, e := range events {
			id := agentID(e)
			open := id != "" && v.expanded[id] && depth < maxSubDepth
			var sub *store.Follower
			if id != "" && (open || filtering) {
				sub, _ = v.sub(id)
			}
			inner := false
			if sub != nil && filtering {
				for _, x := range sub.Sess.Events {
					if keep(x) {
						inner = true
						break
					}
				}
			}
			if !keep(e) && !inner {
				continue
			}
			v.shown = append(v.shown, e)
			if depth > 0 {
				v.depth[e] = depth
			}
			switch {
			case open && sub != nil:
				walk(sub.Sess.Events, depth+1)
			case inner:
				v.marked[e] = true
			}
		}
	}
	walk(v.sess.Events, 0)
	v.rebuildGroups()
	v.filesDirty = true
	if v.follow {
		v.cursor = max(len(v.shown)-1, 0)
		return
	}
	// Keep the same event selected when possible.
	v.cursor = min(v.cursor, max(len(v.shown)-1, 0))
	for i, e := range v.shown {
		if e == sel {
			v.cursor = i
			break
		}
	}
}

func (v *view) matches(e *parse.Event, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	// A tool's searchable text grows when its result arrives.
	done := e.Tool != nil && e.Tool.HasResult
	h, ok := v.hay[e]
	if !ok || h.done != done {
		h = hayEntry{haystack(e), done}
		v.hay[e] = h
	}
	for _, t := range terms {
		if !strings.Contains(h.text, t) {
			return false
		}
	}
	return true
}

type hayEntry struct {
	text string
	done bool
}

func haystack(e *parse.Event) string {
	if t := e.Tool; t != nil {
		return strings.ToLower(t.Name + " " + t.Summary + " " + string(t.Input) + " " + t.Result)
	}
	return strings.ToLower(e.Text)
}

func (v *view) selected() *parse.Event {
	if v.cursor < len(v.shown) {
		return v.shown[v.cursor]
	}
	return nil
}

func (v *view) move(d int) {
	if len(v.shown) == 0 {
		return
	}
	v.cursor = max(0, min(v.cursor+d, len(v.shown)-1))
	v.follow = v.cursor == len(v.shown)-1
}

// inputActive reports whether keys are going to a text input.
func (v *view) inputActive() bool { return v.typing || v.rev.typing }

func (v *view) update(msg tea.Msg, w, h int) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		if v.rev.typing {
			return v.typeComment(k)
		}
		if v.typing {
			switch k.String() {
			case "esc":
				v.typing = false
				v.search.Blur()
				v.search.SetValue("")
				v.refresh()
			case "enter":
				v.typing = false
				v.search.Blur()
			default:
				var cmd tea.Cmd
				v.search, cmd = v.search.Update(msg)
				v.follow = false
				v.refresh()
				return cmd
			}
			return nil
		}
		bodyH := h - chromeH
		if v.tab == tabDiff {
			if cmd, ok := v.updateDiff(k, w, bodyH); ok {
				return cmd
			}
		}
		switch k.String() {
		case "y":
			return v.copySelection()
		case "e":
			v.exportView(time.Now())
			return nil
		case "!":
			if a := v.alert; a != nil && a.ev != nil {
				v.unseenErrs = 0
				v.zoom, v.focusR = false, false
				v.jumpTo(a.ev)
			}
			return nil
		case "1", "2", "3", "4", "5":
			return v.setTab(int(k.String()[0] - '1'))
		case "]":
			return v.setTab((v.tab + 1) % len(tabNames))
		case "[":
			return v.setTab((v.tab + len(tabNames) - 1) % len(tabNames))
		}
		if v.tab == tabStats {
			var cmd tea.Cmd
			v.page, cmd = v.page.Update(msg)
			return cmd
		}
		if v.tab == tabDiff {
			return nil
		}
		if v.focusR || v.zoom {
			switch k.String() {
			case "tab", "h", "left":
				v.focusR = false
				v.zoom = false
				return nil
			case "enter":
				if v.zoom {
					v.zoom = false
					return nil
				}
			case "g":
				v.detail.GotoTop()
				return nil
			case "G":
				v.detail.GotoBottom()
				return nil
			case "a", "o", "/", "f", "t", "n", "N", "E", "x", "X":
				// handled below
			default:
				var cmd tea.Cmd
				v.detail, cmd = v.detail.Update(msg)
				return cmd
			}
		}
		switch v.tab {
		case tabTools:
			return v.updateTools(k, w, bodyH)
		case tabFiles:
			return v.updateFiles(k, w, bodyH)
		}
		switch k.String() {
		case "up", "k":
			v.move(-1)
		case "down", "j":
			v.move(1)
		case "pgup", "ctrl+b":
			v.move(-bodyH)
		case "pgdown", "ctrl+f", " ":
			v.move(bodyH)
		case "g", "home":
			v.move(-len(v.shown))
		case "G", "end":
			v.move(len(v.shown))
		case "ctrl+d", "J":
			v.detail.HalfViewDown()
		case "ctrl+u", "K":
			v.detail.HalfViewUp()
		case "n":
			v.jumpPrompt(1)
		case "N":
			v.jumpPrompt(-1)
		case "tab", "l", "right":
			v.focusDetail(w)
		case "enter":
			if w >= wideLayout {
				v.focusR = !v.focusR
			} else {
				v.zoom = true
			}
		case "/":
			v.typing = true
			return v.search.Focus()
		case "f":
			v.kind = (v.kind + 1) % len(kindFilters)
			v.follow = false
			v.refresh()
		case "F":
			v.kind = (v.kind + len(kindFilters) - 1) % len(kindFilters)
			v.follow = false
			v.refresh()
		case "t":
			v.think = !v.think
			v.refresh()
		case "x":
			if e := v.selected(); e != nil && agentID(e) != "" {
				v.follow = false
				v.expanded[agentID(e)] = !v.expanded[agentID(e)]
				v.refresh()
			}
		case "X":
			v.follow = false
			v.toggleAll()
		case "a", "o":
			if e := v.selected(); e != nil {
				return v.openSubagent(e)
			}
		}
	}
	return nil
}

// setTab switches tabs; the Diff tab reads the diff each time it opens.
func (v *view) setTab(t int) tea.Cmd {
	prev := v.tab
	v.tab = t
	v.page.GotoTop()
	if t != prev && (v.focusR || v.zoom) {
		v.focusR, v.zoom = false, false
	}
	if t == tabDiff && t != prev {
		return v.loadDiff()
	}
	return nil
}

// openSubagent opens the transcript of an Agent (or forked Skill) call.
func (v *view) openSubagent(e *parse.Event) tea.Cmd {
	if e.Tool == nil || e.Tool.AgentID == "" {
		return nil
	}
	meta, ok := v.subagent(e.Tool.AgentID)
	if !ok {
		return nil
	}
	label := meta.Description
	if label == "" {
		label = e.Tool.Summary
	}
	msg := openMsg{path: meta.Path, root: v.rootPath, label: label}
	return func() tea.Msg { return msg }
}

// wheel scrolls whatever is under the mouse.
func (v *view) wheel(d int, overDetail bool) {
	scroll := func(vp *viewport.Model) {
		if d < 0 {
			vp.ScrollUp(-d)
		} else {
			vp.ScrollDown(d)
		}
	}
	switch {
	case v.tab == tabStats:
		scroll(&v.page)
	case overDetail || v.zoom:
		scroll(&v.detail)
	case v.tab == tabTools:
		v.moveGroup(d)
	case v.tab == tabDiff && overDetail:
		v.moveLine(d)
	case v.tab == tabDiff:
		v.moveDiffFile(d)
	case v.tab == tabFiles:
		v.moveFile(d)
	default:
		v.move(d)
	}
}

func (v *view) jumpPrompt(dir int) {
	for i := v.cursor + dir; i >= 0 && i < len(v.shown); i += dir {
		if v.shown[i].Kind == parse.KindUser && v.depth[v.shown[i]] == 0 {
			v.cursor = i
			v.follow = i == len(v.shown)-1
			return
		}
	}
}

// onTail applies new transcript lines and returns the alerts they raise.
// A reset (the file was rewritten) re-reads everything without alerts.
func (v *view) onTail(u tail.Update) []*alert {
	if u.Reset {
		v.sess = parse.NewSession()
		v.hay = map[*parse.Event]hayEntry{}
		v.detailEv = nil
		v.watch = newWatch(v.rootPath)
	}
	if len(u.Lines) == 0 && !u.Reset {
		return nil
	}
	before := v.snap(v.sess)
	for _, l := range u.Lines {
		v.sess.Feed(l)
	}
	v.lastLive = time.Now()
	v.agents = map[string]agentEntry{}
	v.stats = stats.Compute(v.sess)
	v.refresh()
	if u.Reset {
		return nil
	}
	as := v.alerts(v.sess, before)
	v.raise(as)
	return as
}

const wideLayout = 110

// chromeH is the rows around a session's body: the top bar and its rule,
// the session's details, a blank line, the tabs, a blank line under them,
// the footer rule and the footer.
const chromeH = 8

func (v *view) render(w, h int) string {
	// Before the header, which shows the file count.
	if v.tab == tabFiles && (v.filesDirty || time.Since(v.filesBuilt) > filesTTL) {
		v.rebuildFiles()
	}
	var b strings.Builder
	b.WriteString(v.header(w) + "\n\n")
	b.WriteString(v.tabs(w) + "\n\n")
	bodyH := max(h-chromeH, 1)

	var body string
	switch v.tab {
	case tabTimeline:
		body = v.timeline(w, bodyH)
	case tabTools:
		body = v.toolsView(w, bodyH)
	case tabStats:
		body = v.pageView(renderStats(v.sess, v.stats, w-2), w, bodyH)
	case tabFiles:
		body = v.filesView(w, bodyH)
	case tabDiff:
		body = v.diffView(w, bodyH)
	}
	b.WriteString(body + "\n")
	b.WriteString(sFaint.Render(strings.Repeat("─", w)) + "\n")
	b.WriteString(v.footer(w))
	return b.String()
}

func (v *view) header(w int) string {
	s := v.sess
	title := s.Title
	if v.label != "" {
		title = "⤷ " + v.label
	}
	if title == "" {
		title = s.ID
	}
	left := sBold.Render(" " + title)
	var meta []string
	if s.CWD != "" {
		meta = append(meta, filepath.Base(s.CWD))
	}
	if s.Branch != "" {
		meta = append(meta, s.Branch)
	}
	var alerts, live, status []string
	if v.unseenErrs > 0 {
		alerts = append(alerts, sErr.Render(fmt.Sprintf("%d%s", v.unseenErrs, glyphErr)))
	}
	if v.alertShown && v.alert != nil {
		alerts = append(alerts, v.alert.render())
	}
	if m := contextMeter(s.Context, s.ContextWindow(), w/3); m != "" {
		status = append(status, m)
	}
	if time.Since(v.lastLive) < 5*time.Second {
		live = append(live, sLive.Render("● live"))
	} else if time.Since(s.End) < 2*time.Minute {
		live = append(live, sOK.Render("○ active"))
	}
	// The top bar has the alerts and whether the session is live; the row
	// under it has the session, with its directory and branch giving way
	// to the context meter when space is short.
	details := strings.Join(append([]string{sDim.Render(strings.Join(meta, " · "))}, status...), "  ")
	if w-lipgloss.Width(left)-lipgloss.Width(details)-1 < 1 {
		details = strings.Join(status, "  ")
	}
	return topBar(strings.Join(append(alerts, live...), "  "), w) + "\n" + spread(left, details, w)
}

func (v *view) tabs(w int) string {
	var parts []string
	for i, n := range tabNames {
		label := n
		if i == v.tab {
			parts = append(parts, sTabOn.Render(label))
		} else {
			parts = append(parts, sTabOff.Render(label))
		}
	}
	left := " " + strings.Join(parts, "   ")
	var right string
	if v.tab == tabTimeline {
		var flags []string
		if v.kind != 0 {
			flags = append(flags, sKey.Render("filter:"+kindFilters[v.kind].name))
		}
		if q := v.search.Value(); q != "" {
			flags = append(flags, sKey.Render("“"+q+"”"))
		}
		if v.think {
			flags = append(flags, sThinking.Render("thinking"))
		}
		pos := fmt.Sprintf("%d/%d", min(v.cursor+1, len(v.shown)), len(v.shown))
		if v.follow {
			pos += " ↓"
		}
		flags = append(flags, sDim.Render(pos))
		right = strings.Join(flags, "  ")
	}
	if v.tab == tabTools {
		var flags []string
		if q := v.search.Value(); q != "" {
			flags = append(flags, sKey.Render("“"+q+"”"))
		}
		calls := 0
		for _, g := range v.groups {
			calls += g.n
		}
		flags = append(flags, sDim.Render(fmt.Sprintf("%d calls", calls)))
		right = strings.Join(flags, "  ")
	}
	if v.tab == tabFiles {
		var flags []string
		if q := v.search.Value(); q != "" {
			flags = append(flags, sKey.Render("“"+q+"”"))
		}
		flags = append(flags, sDim.Render(fmt.Sprintf("%d files", len(v.files))))
		right = strings.Join(flags, "  ")
	}
	if v.tab == tabDiff {
		right = v.diffTabsRight()
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		return fit(left, w)
	}
	return left + strings.Repeat(" ", gap) + right + " "
}

func (v *view) footer(w int) string {
	if v.flash != "" {
		return fit(v.flash, w)
	}
	if v.rev.typing {
		return fit(v.rev.input.View(), w)
	}
	if v.tab == tabDiff {
		help := keyHelp("tab", "lines", "c", "comment", "S", "send", "?", "help")
		if !v.focusR {
			help = keyHelp("enter", "lines", "C", "comment on file", "S", "send", "?", "help")
		}
		return fit(" "+v.sendStatus()+help, w)
	}
	if v.typing || (v.search.Value() != "" && v.tab != tabStats) {
		return fit(v.search.View(), w)
	}
	var help string
	switch {
	case v.tab == tabStats:
		help = keyHelp("1-5", "tabs", "esc", "back", "?", "help")
	case v.focusR || v.zoom:
		help = keyHelp("tab", "back to list", "a", "subagent", "?", "help")
	case v.tab == tabTools:
		help = keyHelp("space", "expand", "enter", "show in timeline", "?", "help")
	case v.tab == tabFiles:
		help = keyHelp("n N", "next change", "enter", "show in timeline", "?", "help")
	default:
		help = keyHelp("enter", "detail", "/", "search", "f", "filter", "?", "help")
	}
	return fit(" "+help, w)
}

func (v *view) pageView(content string, w, h int) string {
	v.page.Width, v.page.Height = w, h
	v.page.SetContent(content)
	return v.page.View()
}

func (v *view) timeline(w, h int) string {
	// Selecting an error counts as having seen the new ones.
	if e := v.selected(); e != nil && e.Tool != nil && e.Tool.IsError {
		v.unseenErrs = 0
	}
	if v.zoom && w < wideLayout {
		return v.detailPane(w, h)
	}
	listW := w
	if w >= wideLayout {
		listW = w * 11 / 20
	}
	v.offset = scrollOffset(v.offset, v.cursor, len(v.shown), h)
	// Each turn starts after a blank line, so the rows from the offset to
	// the cursor can take more than h lines; move down until they fit.
	gap := func(i int) bool {
		return i > v.offset && v.shown[i].Kind == parse.KindUser && v.depth[v.shown[i]] == 0
	}
	for v.offset < v.cursor {
		n := 0
		for i := v.offset; i <= v.cursor; i++ {
			n += 1 + boolInt(gap(i))
		}
		if n <= h {
			break
		}
		v.offset++
	}
	lines := make([]string, 0, h)
	var prev time.Time
	for i := v.offset; i < len(v.shown) && len(lines) < h; i++ {
		e := v.shown[i]
		if gap(i) {
			lines = append(lines, strings.Repeat(" ", listW))
			if len(lines) == h {
				break
			}
		}
		lines = append(lines, renderTimelineRow(v.sess, e, i == v.cursor, listW,
			rowOpts{depth: v.depth[e], open: v.expanded[agentID(e)], marked: v.marked[e], prev: prev}))
		if !e.Time.IsZero() {
			prev = e.Time
		}
	}
	if len(v.shown) == 0 {
		lines = append(lines, sDim.Render(" nothing matches"))
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

func (v *view) detailPane(w, h int) string {
	border := lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).
		BorderForeground(cFaint).PaddingLeft(1)
	if v.focusR || v.zoom {
		border = border.BorderForeground(cAccent)
	}
	innerW := max(w-2, 10)
	if v.zoom {
		border = lipgloss.NewStyle().PaddingLeft(1)
		innerW = max(w-1, 10)
	}
	e := v.selected()
	var group *groupNode
	var file *fileEntry
	if v.tab == tabFiles {
		e, file = nil, v.selectedFile()
	}
	if v.tab == tabTools {
		e = nil
		if g := v.selectedGroup(); g != nil {
			if g.ev != nil {
				e = g.ev
			} else {
				group = g
			}
		}
	}
	groupKey, groupSig := "", ""
	if group != nil {
		groupKey, groupSig = group.key, fmt.Sprintf("%d/%d/%d", group.n, group.errs, group.timed)
	}
	// A prompt shows its turn, which grows while the turn runs.
	var turn *parse.Turn
	if e != nil && group == nil {
		turn = v.sess.TurnOf(e)
	}
	if turn != nil {
		groupSig = fmt.Sprintf("t%d/%d/%v", len(turn.Events), turn.End.UnixNano(), turn.Done)
	}
	if file != nil {
		done := 0
		for _, t := range file.touches {
			if t.ev.Tool.HasResult {
				done++
			}
		}
		groupKey, groupSig = "f:"+file.path, fmt.Sprintf("%d/%d", len(file.touches), done)
	}
	done := e != nil && e.Tool != nil && e.Tool.HasResult
	agent := false
	if e != nil && e.Tool != nil && e.Tool.AgentID != "" {
		meta, _ := v.subagent(e.Tool.AgentID)
		agent = meta.Finished
	}
	if e != v.detailEv || innerW != v.detailW || done != v.detailDone || agent != v.detailAgent || groupKey != v.detailGroup || groupSig != v.detailGSig {
		switch {
		case group != nil:
			v.detail.SetContent(renderGroupDetail(group, innerW))
		case file != nil:
			content, marks := renderFileDetail(file, innerW)
			v.detail.SetContent(content)
			v.fMarks = marks
		case turn != nil:
			v.detail.SetContent(renderDetail(e, v.subagent, innerW) + "\n" + renderTurnDetail(v.sess, turn, innerW, time.Now()))
		default:
			v.detail.SetContent(renderDetail(e, v.subagent, innerW))
		}
		// Keep the scroll position while the same thing stays selected.
		if e != v.detailEv || groupKey != v.detailGroup {
			v.detail.GotoTop()
		}
		v.detailEv, v.detailW, v.detailDone, v.detailAgent = e, innerW, done, agent
		v.detailGroup, v.detailGSig = groupKey, groupSig
	}
	v.detail.Width, v.detail.Height = innerW, h
	return border.Height(h).Render(v.detail.View())
}

func renderRow(e *parse.Event, sel bool, w int) string {
	return renderRowName(e, sel, w, rowOpts{})
}

// renderTimelineRow adds the turn's totals to prompt rows.
func renderTimelineRow(s *parse.Session, e *parse.Event, sel bool, w int, o rowOpts) string {
	if t := s.TurnOf(e); t != nil {
		o.note = turnSummary(s, t, min(26, (w-10)/3))
	}
	return renderRowName(e, sel, w, o)
}

type rowOpts struct {
	hideName bool      // drop the tool name column (rows under their tool in the Tools tab)
	note     string    // shown on the right of non-tool rows
	depth    int       // subagent nesting, drawn as │ rules
	open     bool      // an Agent call expanded inline
	marked   bool      // a collapsed Agent call whose subagent matches the search
	prev     time.Time // the row above's time; a time in the same minute shows only its seconds
}

// renderRowName renders a timeline row.
func renderRowName(e *parse.Event, sel bool, w int, o rowOpts) string {
	showName, note := !o.hideName, o.note
	var bg lipgloss.TerminalColor = lipgloss.NoColor{}
	switch {
	case sel:
		bg = cSelBg
	case e.Kind == parse.KindUser:
		bg = cPromptBg
	}
	st := func(s lipgloss.Style) lipgloss.Style { return s.Background(bg) }
	mark := st(sText).Render(" ")
	if sel {
		mark = st(sAccent).Render("▌")
	}
	ts := ""
	if !e.Time.IsZero() {
		t := e.Time.Local()
		ts = t.Format("15:04:05")
		if p := o.prev.Local(); !o.prev.IsZero() && t.Truncate(time.Minute).Equal(p.Truncate(time.Minute)) {
			ts = "     " + t.Format(":05")
		}
	}
	prefix := mark + st(sFaint).Render(fit(ts, 8)+"  ")
	rest := w - 11
	if o.depth > 0 {
		prefix += st(lipgloss.NewStyle().Foreground(cAgent)).Render(strings.Repeat("│ ", o.depth))
		rest -= 2 * o.depth
	}

	var glyph, body, right string
	if note != "" {
		right = st(sDim).Render(" " + note + " ")
	}
	switch e.Kind {
	case parse.KindUser:
		glyph = st(sPrompt).Render(glyphUser)
		body = st(sPrompt).Render(fit(firstLine(e.Text), rest-2-lipgloss.Width(right)))
	case parse.KindText:
		glyph = st(sAccent).Render(glyphText)
		body = st(sText).Render(fit(firstLine(e.Text), rest-2))
	case parse.KindThinking:
		glyph = st(sThinking).Render(glyphThinking)
		body = st(sThinking).Render(fit(firstLine(e.Text), rest-2))
	case parse.KindSystem:
		glyph = st(sDim).Render(glyphSystem)
		body = st(sDim).Render(fit(firstLine(e.Text), rest-2))
	case parse.KindCompact:
		glyph = st(sDim).Render(glyphCompact)
		body = st(sDim).Render(fit(compactText(e.Compact), rest-2))
	case parse.KindTool:
		t := e.Tool
		ks := lipgloss.NewStyle().Foreground(kindColor(t.Kind))
		glyph = st(ks).Render(kindGlyph(t.Kind))
		if o.open {
			glyph = st(ks).Render("▾")
		}
		name := t.Name
		if t.Kind == parse.ToolMCP {
			name = t.Group()
		}
		status := st(sDim).Render(glyphPending)
		switch {
		case t.IsError:
			status = st(sErr).Render(glyphErr)
		case t.HasResult:
			status = st(sOK).Render(glyphOK)
		}
		d := dur(e.Duration())
		right = st(sFaint).Render(fmt.Sprintf(" %6s ", d)) + status + st(sText).Render(" ")
		if o.marked {
			right = st(sAccent).Render(" ●") + right
		}
		sumStyle := sDim
		if t.Kind == parse.ToolSkill {
			sumStyle = ks
		}
		if showName {
			nameW := min(16, max(rest/4, 8))
			sumW := rest - 2 - nameW - 1 - lipgloss.Width(right)
			body = st(ks).Render(fit(name, nameW)) + st(sText).Render("  ") + st(sumStyle).Render(fit(t.Summary, sumW-1))
		} else {
			body = st(sText).Render(fit(t.Summary, rest-2-lipgloss.Width(right)))
		}
	}
	line := prefix + glyph + st(sText).Render(" ") + body + right
	if pad := w - lipgloss.Width(line); pad > 0 {
		line += st(sText).Render(strings.Repeat(" ", pad))
	}
	return fit(line, w)
}
