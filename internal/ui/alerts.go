package ui

import (
	"fmt"
	"time"

	"github.com/jzills/snoop/internal/parse"
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
	text string
	ev   *parse.Event // what ! jumps to
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
	unseenErrs int
	pending    map[*parse.Event]bool // live tool calls without a result; true once alerted
}

func newWatch() watch { return watch{pending: map[*parse.Event]bool{}} }

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
			out = append(out, &alert{alertCompact, glyphCompact + " compacted", e})
		case e.Tool != nil && e.Tool.Kind != parse.ToolAgent && !e.Tool.HasResult:
			// Subagents legitimately run for minutes; other tools are
			// watched for stalls.
			w.pending[e] = false
		}
	}
	if pct := ctxPct(s); before.ctx < ctxAlertPct && pct >= ctxAlertPct {
		out = append(out, &alert{alertContext, fmt.Sprintf("ctx %d%%", pct), s.Events[len(s.Events)-1]})
	}
	if n := len(s.Turns); n > 0 {
		t := s.Turns[n-1]
		if t.Done && (t != before.turn || !before.done) {
			out = append(out, &alert{alertTurn, glyphOK + " your turn", t.Events[len(t.Events)-1]})
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
		out = append(out, &alert{alertError, glyphErr + " error: " + last.Tool.Name, last})
	}
	return out
}

// stalls raises one alert per live tool call still without a result after
// stall.
func (w *watch) stalls(now time.Time, stall time.Duration) []*alert {
	var out []*alert
	for e, alerted := range w.pending {
		switch {
		case e.Tool.HasResult:
			delete(w.pending, e)
		case !alerted && now.Sub(e.Time) >= stall:
			w.pending[e] = true
			out = append(out, &alert{alertStall, fmt.Sprintf("%s waiting %s on %s · may be a permission prompt",
				glyphPending, waited(now.Sub(e.Time)), e.Tool.Name), e})
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
