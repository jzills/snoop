package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/review"
	"github.com/jzills/snoop/internal/store"
)

// A session's header says what it is doing, worked out the way the live
// board does, so the two always agree. While it works, a spinner ends the
// timeline and turns on in-flight calls. The spinner has its own tick,
// which runs only while something on screen is working.

const (
	spinFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	spinEvery  = 150 * time.Millisecond
)

type spinMsg struct{}

func spinTick() tea.Cmd {
	return tea.Tick(spinEvery, func(time.Time) tea.Msg { return spinMsg{} })
}

// spinFrame is the spinner as of now, so every spinner on screen agrees.
func spinFrame(now time.Time) string {
	fs := []rune(spinFrames)
	return string(fs[int(now.UnixNano()/int64(spinEvery))%len(fs)])
}

func (a *App) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// withHooks overrides a transcript's state with what the hooks report:
// a request waiting for an answer, or a turn the Stop hook holds open.
// Claude Code records the turn as done only once the Stop hook stops
// waiting for a reply.
func withHooks(st rowStatus, detail string, now time.Time, turn review.Turn, ended bool, asks []review.Request) (rowStatus, string) {
	if ended && st != rowYourTurn {
		st, detail = rowYourTurn, "turn ended "+agoAt(turn.Ended, now)+" · R replies"
	}
	if len(asks) > 0 {
		st, detail = rowAsking, askSummary(asks[0])
	}
	return st, detail
}

// viewStatus is what the session in v is doing at now. A subagent whose
// call has its result is finished, and a session no one has written to
// for liveWindow has no state to show.
func (a *App) viewStatus(v *view, now time.Time) (rowStatus, bool) {
	last := v.lastWrite()
	if now.Sub(last) >= liveWindow {
		return rowIdle, false
	}
	if v.label != "" {
		if call := a.parentCall(v); call != nil && !call.Running() {
			return rowIdle, false
		}
		st, _ := rowState(v.sess, v.rootPath, last, now, a.stallFor())
		return st, true
	}
	st, detail := rowState(v.sess, v.rootPath, last, now, a.stallFor())
	turn, ended := a.turns[v.sessionID()]
	st, _ = withHooks(st, detail, now, turn, ended, a.asks[v.sessionID()])
	return st, true
}

// parentCall is the Agent or Skill call that ran the subagent in v, from
// the view under it.
func (a *App) parentCall(v *view) *parse.ToolCall {
	id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(v.path), "agent-"), ".jsonl")
	for i := 1; i < len(a.stack); i++ {
		if a.stack[i] != v {
			continue
		}
		for _, e := range a.stack[i-1].sess.Events {
			if e.Tool != nil && e.Tool.AgentID == id {
				return e.Tool
			}
		}
	}
	return nil
}

// lastWrite is the newest write to the session or its subagents that the
// view knows of.
func (v *view) lastWrite() time.Time {
	last := v.sess.End
	for _, t := range []time.Time{v.lastLive, v.subLast} {
		if t.After(last) {
			last = t
		}
	}
	return last
}

type subWriteMsg struct {
	v    *view
	last time.Time
}

// subWriteCmd looks for the newest subagent write off the UI goroutine:
// a subagent can work for many minutes while the session is quiet.
func (v *view) subWriteCmd() tea.Cmd {
	path := v.rootPath
	return func() tea.Msg { return subWriteMsg{v, store.SubagentWrite(path)} }
}

// animating reports whether anything on screen is working, as of the
// last render.
func (a *App) animating() bool {
	if v := a.top(); v != nil {
		return v.known && v.status == rowWorking
	}
	if a.board != nil {
		for _, r := range a.board.order {
			if r.status == rowWorking {
				return true
			}
		}
	}
	return false
}

// stateLabel is the header's word for a session's state.
func stateLabel(st rowStatus, frame string) string {
	return st.render(func(s lipgloss.Style) lipgloss.Style { return s }, frame)
}

// workLine is the timeline's last row while the session works: what it
// is doing and for how long, then what a background subagent is doing.
func (v *view) workLine(now time.Time, w int) string {
	what, doing, since := "replying", "", time.Time{}
	if n := len(v.sess.Turns); n > 0 {
		t := v.sess.Turns[n-1]
		for _, e := range t.Events {
			if !e.Time.IsZero() {
				since = e.Time
			}
		}
		if bg := backgroundAgents(v.sess); t.Done && len(bg) > 0 {
			e := bg[len(bg)-1]
			what, since = inBackground(bg), e.Time
			if f := v.subs[e.Tool.AgentID]; f != nil && len(f.Sess.Events) > 0 {
				doing = describe(f.Sess.Events[len(f.Sess.Events)-1])
			}
		} else if e := runningCall(t); e != nil {
			what = e.Tool.Name + "  " + oneLine(e.Tool.Summary)
			if e.Tool.Kind == parse.ToolAgent {
				what = "⤷ " + e.Tool.Group() + "  " + oneLine(e.Tool.Summary)
			}
			since = e.Time
		}
	}
	line := " " + strings.Repeat(" ", 10) + sAccent.Render(spinFrame(now)) + " " + sText.Render(what)
	if !since.IsZero() {
		line += sDim.Render(" · " + waited(now.Sub(since)))
	}
	// What a background subagent is doing comes last, as the most likely
	// to be cut.
	if doing != "" {
		line += sText.Render("  " + doing)
	}
	return fit(line, w)
}

// runningCall is the newest call of t still without a result.
func runningCall(t *parse.Turn) *parse.Event {
	var running *parse.Event
	for _, e := range t.Events {
		if e.Tool != nil && !e.Tool.HasResult {
			running = e
		}
	}
	return running
}

// backgroundAgents is the subagents and forked skills still running in
// the background. Background shells are left out: a dev server never
// reports back.
func backgroundAgents(s *parse.Session) []*parse.Event {
	var out []*parse.Event
	for _, e := range s.RunningBackground() {
		if k := e.Tool.Kind; k == parse.ToolAgent || k == parse.ToolSkill {
			out = append(out, e)
		}
	}
	return out
}

// inBackground says how many run in the background, naming the newest.
func inBackground(bg []*parse.Event) string {
	t := bg[len(bg)-1].Tool
	name := t.Group()
	if t.Kind == parse.ToolAgent {
		name = "⤷ " + name
	}
	return fmt.Sprintf("%d in background · %s", len(bg), name)
}

// describe is what a subagent's newest event shows it doing.
func describe(e *parse.Event) string {
	switch {
	case e.Tool != nil:
		return e.Tool.Name + "  " + oneLine(e.Tool.Summary)
	case e.Kind == parse.KindThinking:
		return "thinking"
	case e.Kind == parse.KindText:
		return "replying"
	}
	return ""
}
