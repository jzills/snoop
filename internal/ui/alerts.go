package ui

import (
	"fmt"
	"time"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
)

// Alerts flag what happens while a session is watched live: tool errors,
// tools waiting a long time, the end of a turn, compactions and a nearly
// full context. Lines read when a session opens never raise alerts.

const (
	alertError   = "error"
	alertStall   = "stall"
	alertTurn    = "turn"
	alertCompact = "compact"
	alertContext = "ctx"
)

// AlertKinds lists the alert names --alerts accepts.
var AlertKinds = []string{alertError, alertStall, alertTurn, alertCompact, alertContext}

const ctxAlertPct = 90

type alert struct {
	kind string
	text string       // shown in the header
	note string       // a notification's text: what happened, without glyphs
	ev   *parse.Event // what ! jumps to

	// The session it came from, set by whoever raises it.
	path, title string
}

// key identifies the event an alert is about, the same in every snoop
// watching the session, so only one of them notifies.
func (a *alert) key() string {
	id := a.text
	if a.ev != nil {
		id = a.ev.UUID
		if a.ev.Tool != nil {
			id = a.ev.Tool.ID
		}
	}
	return a.path + "|" + a.kind + "|" + id
}

func (a *alert) render() string {
	switch a.kind {
	case alertError, alertContext:
		return sErr.Render(a.text)
	case alertTurn:
		return sOK.Render(a.text)
	case alertStall:
		return sAccent.Render(a.text)
	}
	return sDim.Render(a.text)
}

func ctxPct(s *parse.Session) int {
	if w := s.ContextWindow(); w > 0 {
		return s.Context * 100 / w
	}
	return 0
}

// watch is the alert state of one live session: the session view and the
// live board each keep one per session they follow.
type watch struct {
	root       string // the session's transcript, whose subagents it checks
	unseenErrs int
	pending    map[*parse.Event]bool // live tool calls without a result; true once alerted
}

func newWatch(root string) watch { return watch{root: root, pending: map[*parse.Event]bool{}} }

// runsSubagent reports calls that run a subagent, which take minutes and
// never wait on you: Agent calls, and forked Skill calls, known by a
// subagent of the session writing since the call began.
func runsSubagent(e *parse.Event, root string) bool {
	switch e.Tool.Kind {
	case parse.ToolAgent:
		return true
	case parse.ToolSkill:
		return root != "" && store.SubagentWrittenSince(root, e.Time)
	}
	return false
}

// liveState is what a watch compares before and after new lines.
type liveState struct {
	events, errors, ctx int
	turn                *parse.Turn
	done                bool
}

// errorCount counts tool calls that failed.
func errorCount(s *parse.Session) int {
	n := 0
	for _, e := range s.Events {
		if e.Tool != nil && e.Tool.IsError {
			n++
		}
	}
	return n
}

func (w *watch) snap(s *parse.Session) liveState {
	st := liveState{events: len(s.Events), errors: errorCount(s), ctx: ctxPct(s)}
	if n := len(s.Turns); n > 0 {
		st.turn = s.Turns[n-1]
		st.done = st.turn.Done
	}
	return st
}

// alerts compares the session against its state before new lines arrived
// and raises alerts, the most important last.
func (w *watch) alerts(s *parse.Session, before liveState) []*alert {
	var out []*alert
	for _, e := range s.Events[before.events:] {
		switch {
		case e.Kind == parse.KindCompact:
			note := "Context compacted"
			if e.Compact != nil && e.Compact.Trigger != "" {
				note += " (" + e.Compact.Trigger + ")"
			}
			out = append(out, &alert{kind: alertCompact, text: glyphCompact + " compacted", note: note, ev: e})
		case e.Tool != nil && e.Tool.Kind != parse.ToolAgent && !e.Tool.HasResult:
			// Subagents legitimately run for minutes; other tools are
			// watched for stalls.
			w.pending[e] = false
		}
	}
	if pct := ctxPct(s); before.ctx < ctxAlertPct && pct >= ctxAlertPct {
		a := &alert{kind: alertContext, text: fmt.Sprintf("ctx %d%%", pct), note: fmt.Sprintf("Context %d%% full", pct)}
		if n := len(s.Events); n > 0 {
			a.ev = s.Events[n-1]
		}
		out = append(out, a)
	}
	if n := len(s.Turns); n > 0 {
		t := s.Turns[n-1]
		if t.Done && (t != before.turn || !before.done) {
			note := "Your turn"
			if d := t.Duration(); d > 0 {
				note += " · finished after " + dur(d.Truncate(time.Second))
			}
			out = append(out, &alert{kind: alertTurn, text: glyphOK + " your turn", note: note, ev: t.Events[len(t.Events)-1]})
		}
	}
	if n := errorCount(s) - before.errors; n > 0 {
		w.unseenErrs += n
		var last *parse.Event
		for _, e := range s.Events {
			if e.Tool != nil && e.Tool.IsError && (last == nil || e.Tool.ResultTime.After(last.Tool.ResultTime)) {
				last = e
			}
		}
		out = append(out, &alert{kind: alertError, text: glyphErr + " error: " + last.Tool.Name,
			note: "Error in " + last.Tool.Name, ev: last})
	}
	return out
}

// stalls raises one alert per live tool call still without a result after
// stall, other than those running a subagent.
func (w *watch) stalls(now time.Time, stall time.Duration) []*alert {
	var out []*alert
	for e, alerted := range w.pending {
		switch {
		case e.Tool.HasResult:
			delete(w.pending, e)
		case !alerted && now.Sub(e.Time) >= stall && !runsSubagent(e, w.root):
			w.pending[e] = true
			what := fmt.Sprintf("waiting %s on %s · may be a permission prompt", waited(now.Sub(e.Time)), e.Tool.Name)
			out = append(out, &alert{kind: alertStall, text: glyphPending + " " + what, note: "W" + what[1:], ev: e})
		}
	}
	return out
}

// raise shows the newest alert in the header.
func (v *view) raise(as []*alert) {
	if len(as) > 0 {
		v.alert, v.alertShown = as[len(as)-1], true
	}
}
