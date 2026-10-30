package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
)

// The live board (--live, or w in the picker) lists every session written
// in the last liveWindow, across projects, with what each is doing and
// whether it is waiting on you. Sessions are found by stat alone; only
// live ones are parsed, then followed by reading what was added.

const (
	liveWindow = 30 * time.Minute // a session drops off after this long without writes
	boardScan  = 5 * time.Second  // how often to look for sessions starting or stopping
	staleAfter = 10 * time.Minute // a turn with no writes for this long is no longer working
)

type rowStatus int

// In display order.
const (
	rowWaiting rowStatus = iota
	rowYourTurn
	rowWorking
	rowIdle
)

func (s rowStatus) String() string {
	return [...]string{"waiting", "your turn", "working", "idle"}[s]
}

func (s rowStatus) render(sel func(lipgloss.Style) lipgloss.Style) string {
	switch s {
	case rowWaiting:
		return sel(sAccent.Bold(true)).Render(glyphPending + " waiting")
	case rowYourTurn:
		return sel(sOK).Render(glyphOK + " your turn")
	case rowWorking:
		return sel(sText).Render("● working")
	}
	return sel(sDim).Render(glyphSystem + " idle")
}

const interrupted = "[Request interrupted by user"

// rowState says what a session is doing at now, given when it, or one of
// its subagents, last wrote. A tool call with no result after stall is
// waiting on you however long ago it started: a permission prompt waits
// forever. Calls running a subagent of root take minutes and never count
// as waiting.
func rowState(s *parse.Session, root string, last, now time.Time, stall time.Duration) (rowStatus, string) {
	if len(s.Turns) == 0 {
		return rowIdle, "no prompts yet"
	}
	t := s.Turns[len(s.Turns)-1]
	if e := t.Events[len(t.Events)-1]; e.Kind == parse.KindSystem && strings.HasPrefix(e.Text, interrupted) {
		return rowYourTurn, "interrupted " + agoAt(e.Time, now)
	}
	if t.Done {
		return rowYourTurn, "turn ended " + agoAt(t.End, now)
	}
	var running *parse.Event
	for _, e := range t.Events {
		if e.Tool == nil || e.Tool.HasResult {
			continue
		}
		if now.Sub(e.Time) >= stall && !runsSubagent(e, root) {
			return rowWaiting, fmt.Sprintf("waiting %s on %s · may be a permission prompt",
				waited(now.Sub(e.Time)), e.Tool.Name)
		}
		running = e
	}
	if now.Sub(last) >= staleAfter {
		return rowIdle, "no activity for " + dur(now.Sub(last).Truncate(time.Minute))
	}
	if running != nil {
		if running.Tool.Kind == parse.ToolAgent {
			return rowWorking, "⤷ " + running.Tool.Group() + ": " + running.Tool.Summary
		}
		return rowWorking, glyphPending + " " + running.Tool.Name + "  " + running.Tool.Summary
	}
	return rowWorking, "replying"
}

// agoAt is ago measured from now rather than the clock.
func agoAt(t, now time.Time) string {
	d := now.Sub(t)
	if d < time.Minute {
		return "just now"
	}
	return dur(d.Truncate(time.Minute)) + " ago"
}

type boardRow struct {
	info  store.SessionInfo
	f     *store.Follower
	watch watch
	last  time.Time // newest write to the transcript or its subagents

	status rowStatus // as of the last view
	detail string
}

func (r *boardRow) project() string { return filepath.Base(r.info.Project()) }

type board struct {
	root     string
	rows     map[string]*boardRow
	loading  map[string]bool
	order    []*boardRow // as last shown
	cursor   int
	offset   int
	sort     listSort // by boardCols
	scanned  bool     // the first discovery has finished
	lastScan time.Time

	alert      *alert
	alertShown bool
}

func newBoard(root string) *board {
	return &board{root: root, rows: map[string]*boardRow{}, loading: map[string]bool{}}
}

type boardScanMsg struct {
	acts []store.Activity
	now  time.Time
}

type boardLoadedMsg struct {
	act  store.Activity
	info store.SessionInfo
	f    *store.Follower
	err  error
}

// scanCmd looks for live sessions off the UI goroutine.
func (b *board) scanCmd(now time.Time) tea.Cmd {
	b.lastScan = now
	root := b.root
	return func() tea.Msg {
		return boardScanMsg{store.Recent(root, now.Add(-liveWindow)), now}
	}
}

// onScan updates when each row last wrote, drops rows gone quiet, and
// returns a load for each session not yet followed.
func (b *board) onScan(msg boardScanMsg) []tea.Cmd {
	b.scanned = true
	seen := map[string]bool{}
	var loads []tea.Cmd
	for _, a := range msg.acts {
		seen[a.Path] = true
		if r := b.rows[a.Path]; r != nil {
			if a.Last.After(r.last) {
				r.last = a.Last
			}
			continue
		}
		if b.loading[a.Path] {
			continue
		}
		b.loading[a.Path] = true
		act := a
		loads = append(loads, func() tea.Msg {
			f, err := store.Track(act.Path)
			return boardLoadedMsg{act: act, info: store.Info(act.Path), f: f, err: err}
		})
	}
	for p := range b.rows {
		if !seen[p] {
			delete(b.rows, p)
		}
	}
	return loads
}

// onLoaded adds a followed session. What it already holds never alerts.
func (b *board) onLoaded(msg boardLoadedMsg) {
	delete(b.loading, msg.act.Path)
	if msg.err != nil {
		return
	}
	b.rows[msg.act.Path] = &boardRow{info: msg.info, f: msg.f, watch: newWatch(msg.act.Path), last: msg.act.Last}
}

// poll reads what each session added since the last poll and returns the
// alerts it raises, prefixed with the project. Sessions for which open is
// true are shown in a view, which raises their alerts itself.
func (b *board) poll(now time.Time, stall time.Duration, open func(path string) bool) []*alert {
	var out []*alert
	for _, p := range b.sorted() {
		r := b.rows[p]
		before := r.watch.snap(r.f.Sess)
		changed, reset := r.f.Poll()
		if reset {
			r.watch = newWatch(p)
			continue
		}
		var as []*alert
		if changed {
			r.last = now
			as = r.watch.alerts(r.f.Sess, before)
		}
		if stall > 0 {
			as = append(as, r.watch.stalls(now, stall)...)
		}
		if open(p) {
			continue
		}
		for _, a := range as {
			a.text = r.project() + ": " + a.text
			a.path, a.title = p, noteTitle(r.project(), r.info.Name())
			out = append(out, a)
		}
	}
	if len(out) > 0 {
		b.alert, b.alertShown = out[len(out)-1], true
	}
	return out
}

// sorted is the row paths in a stable order, so alerts come out the same
// way each poll.
func (b *board) sorted() []string {
	ps := make([]string, 0, len(b.rows))
	for p := range b.rows {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

// The board's columns, in order; each sorts in its natural order. Rows
// that tie are newest first.
var boardCols = []struct {
	title string
	cmp   func(a, b *boardRow) int
}{
	{"STATE", func(a, b *boardRow) int { return cmpAsc(int(a.status), int(b.status)) }},
	{"PROJECT", func(a, b *boardRow) int { return cmpAsc(strings.ToLower(a.project()), strings.ToLower(b.project())) }},
	{"SESSION", func(a, b *boardRow) int {
		return cmpAsc(strings.ToLower(a.info.Name()), strings.ToLower(b.info.Name()))
	}},
	{"CONTEXT", func(a, b *boardRow) int { return cmpDesc(ctxPct(a.f.Sess), ctxPct(b.f.Sess)) }},
	{"COST", func(a, b *boardRow) int { return cmpDesc(a.f.Sess.CostUSD, b.f.Sess.CostUSD) }},
	{"AGE", func(a, b *boardRow) int { return cmpDesc(a.last.UnixNano(), b.last.UnixNano()) }},
}

// arrange works out each row's state and orders them by the chosen
// column: by default waiting first, then your turn, working and idle,
// newest first within each.
func (b *board) arrange(now time.Time, stall time.Duration) {
	var selected *boardRow
	if b.cursor < len(b.order) {
		selected = b.order[b.cursor]
	}
	b.order = b.order[:0]
	for _, r := range b.rows {
		r.status, r.detail = rowState(r.f.Sess, r.info.Path, r.last, now, stall)
		b.order = append(b.order, r)
	}
	cmp := boardCols[b.sort.col].cmp
	sort.Slice(b.order, func(i, j int) bool {
		x, y := b.order[i], b.order[j]
		if c := cmp(x, y); c != 0 {
			return b.sort.less(c)
		}
		if !x.last.Equal(y.last) {
			return x.last.After(y.last)
		}
		return x.f.Path < y.f.Path
	})
	// The cursor stays on the same session as rows move around it.
	b.cursor = min(b.cursor, max(len(b.order)-1, 0))
	for i, r := range b.order {
		if r == selected {
			b.cursor = i
		}
	}
}

func (b *board) update(msg tea.KeyMsg, height int) tea.Cmd {
	b.alertShown = false
	n := len(b.order)
	switch msg.String() {
	case "up", "k", "ctrl+p":
		b.cursor = max(b.cursor-1, 0)
	case "down", "j", "ctrl+n":
		b.cursor = min(b.cursor+1, max(n-1, 0))
	case "g", "home":
		b.cursor = 0
	case "G", "end":
		b.cursor = max(n-1, 0)
	case "o":
		b.sort.next(len(boardCols))
	case "O":
		b.sort.rev = !b.sort.rev
	case "enter", "l", "right":
		if b.cursor < n {
			path := b.order[b.cursor].f.Path
			return func() tea.Msg { return openMsg{path: path} }
		}
	}
	return nil
}

func (b *board) view(w, h int, now time.Time, stall time.Duration) string {
	b.arrange(now, stall)
	var sb strings.Builder
	waiting := 0
	for _, r := range b.order {
		if r.status == rowWaiting {
			waiting++
		}
	}
	// snoop on the left, the count on the right.
	right := sDim.Render("Live · ") +
		sText.Render(fmt.Sprint(len(b.order))) + sDim.Render(" "+plural(len(b.order), "session"))
	if waiting > 0 {
		right += sDim.Render(" · ") + sAccent.Bold(true).Render(fmt.Sprintf("%d waiting on you", waiting))
	}
	if b.alert != nil && b.alertShown {
		right = b.alert.render() + "   " + right
	}
	sb.WriteString(topBar(right, w) + "\n")

	bodyH := h - 5
	stateW, projW, ctxW, costW, ageW := 12, min(20, w/6), 9, 8, 6
	nameW := max(w-(1+stateW+2+projW+2+ctxW+2+costW+2+ageW+2), 10)
	hd := b.sort.head
	sb.WriteString(fit(sColHead.Render(" "+fit(hd("STATE", 0), stateW)+"  "+fit(hd("PROJECT", 1), projW)+"  "+fit(hd("SESSION", 2), nameW)+"  "+
		fit(hd("CONTEXT", 3), ctxW)+"  "+fmt.Sprintf("%*s", costW, hd("COST", 4))+"  "+fmt.Sprintf("%*s", ageW, hd("AGE", 5))), w) + "\n")
	switch {
	case !b.scanned:
		sb.WriteString(sDim.Render(" looking for live sessions…") + "\n")
	case len(b.order) == 0:
		sb.WriteString(sDim.Render(fmt.Sprintf(" no session written in the last %s", dur(liveWindow))) + "\n")
	}
	per := 2 // lines per row
	b.offset = scrollOffset(b.offset, b.cursor, len(b.order), max(bodyH/per, 1))
	for i := b.offset; i < len(b.order) && (i-b.offset+1)*per <= bodyH; i++ {
		r := b.order[i]
		sel := i == b.cursor
		st := func(base lipgloss.Style) lipgloss.Style {
			if sel {
				return base.Background(cSelBg)
			}
			return base
		}
		mark := st(sText).Render(" ")
		if sel {
			mark = st(sAccent).Render("▌")
		}
		s := r.f.Sess
		ctx, cost := "", ""
		if pct := ctxPct(s); s.Context > 0 {
			// contextMeter's colors, drawn here so the selection shows through.
			cs := sDim
			switch {
			case pct >= 90:
				cs = sErr
			case pct >= 70:
				cs = sAccent
			}
			ctx = st(cs).Render(fit(fmt.Sprintf("%d%%", pct), ctxW))
		} else {
			ctx = st(sDim).Render(fit("", ctxW))
		}
		if s.CostUSD > 0 {
			cost = fmt.Sprintf("$%.2f", s.CostUSD)
		}
		nameStyle := sText
		if sel {
			nameStyle = sBold
		}
		line := mark + fit(r.status.render(st), stateW) + st(sDim).Render("  ") +
			st(sAccent).Render(fit(r.project(), projW)) + st(sDim).Render("  ") +
			st(nameStyle).Render(fit(r.info.Name(), nameW)) + st(sDim).Render("  ") +
			ctx + st(sDim).Render("  ") +
			st(sDim).Render(fit(fmt.Sprintf("%*s", costW, cost), costW)) + st(sDim).Render("  ") +
			st(sDim).Render(fit(fmt.Sprintf("%*s", ageW, ago(r.last)), ageW))
		sb.WriteString(fit(line, w) + "\n")
		detailStyle := sDim
		if r.status == rowWaiting {
			detailStyle = sAccent
		}
		sb.WriteString(fit(st(sText).Render(" ")+strings.Repeat(" ", stateW+2)+detailStyle.Render(oneLine(r.detail)), w) + "\n")
	}
	if pad := h - 2 - strings.Count(sb.String(), "\n"); pad > 0 {
		sb.WriteString(strings.Repeat("\n", pad))
	}
	sb.WriteString(sFaint.Render(strings.Repeat("─", w)) + "\n")
	sb.WriteString(fit(" "+keyHelp("↑↓", "move", "enter", "open", "o", "sort", "esc", "sessions", "?", "help"), w))
	return sb.String()
}

// oneLine collapses whitespace, so a multi-line command stays on its row.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
