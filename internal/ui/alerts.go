package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
)

// Alerts flag what happens while a session is watched live: tool errors,
// tools waiting a long time, the end of a turn, compactions, a nearly full
// context, and, when you set a limit, cost and long turns. Lines read when
// a session opens never raise alerts.

const (
	alertError   = "error"
	alertStall   = "stall"
	alertTurn    = "turn"
	alertCompact = "compact"
	alertContext = "ctx"
	alertAsk     = "ask"
	alertCost    = "cost"
	alertLong    = "long"
)

// AlertKinds lists the alert names --alerts accepts.
var AlertKinds = []string{alertError, alertStall, alertTurn, alertCompact, alertContext, alertAsk, alertCost, alertLong}

// Limits are the thresholds of the cost and long-turn alerts; zero turns
// one off.
type Limits struct {
	Cost     float64       // alert each time a session's cost passes another multiple of this, in dollars
	LongTurn time.Duration // alert when a turn has run this long
}

const ctxAlertPct = 90

type alert struct {
	kind string
	text string       // shown in the header
	note string       // a notification's text: what happened, without glyphs
	ev   *parse.Event // what ! jumps to
	id   string       // what the alert is about, when not an event: a request's ID

	// The session it came from, set by whoever raises it.
	path, title string
}

// key identifies the event an alert is about, the same in every snoop
// watching the session, so only one of them notifies.
func (a *alert) key() string {
	id := a.text
	switch {
	case a.id != "":
		id = a.id
	case a.ev != nil:
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
	case alertStall, alertCost, alertLong:
		return sAccent.Render(a.text)
	case alertAsk:
		return sAccent.Bold(true).Render(a.text)
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
	// A turn whose end was alerted while its Stop hook waited for a reply,
	// before Claude Code recorded it as done.
	endedEarly *parse.Turn

	// The limits: set by their first check, so what the session had
	// reached when it opened isn't alerted.
	limitsSeen bool
	costSteps  int         // multiples of the cost limit passed
	longTurn   *parse.Turn // the turn alerted as long
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

func (w *watch) snap(s *parse.Session) liveState {
	st := liveState{events: len(s.Events), errors: s.Errors, ctx: ctxPct(s)}
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
		if t.Done && (t != before.turn || !before.done) && t != w.endedEarly {
			note := "Your turn"
			if d := t.Duration(); d > 0 {
				note += " · finished after " + dur(d.Truncate(time.Second))
			}
			out = append(out, &alert{kind: alertTurn, text: glyphOK + " your turn", note: note, ev: t.Events[len(t.Events)-1]})
		}
	}
	if n := s.Errors - before.errors; n > 0 {
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

// limits raises an alert when the session's cost passes another multiple
// of the cost limit, and one when its current turn has run longer than the
// long-turn limit. The first check only notes where the session is.
func (w *watch) limits(s *parse.Session, now time.Time, l Limits) []*alert {
	var out []*alert
	first := !w.limitsSeen
	w.limitsSeen = true
	if l.Cost > 0 {
		if usd, estimated, ok := s.Cost(); ok {
			n := int(usd / l.Cost)
			if !first && n > w.costSteps {
				at := dollars(float64(n) * l.Cost)
				if estimated {
					at = "about " + at
				}
				a := &alert{kind: alertCost, text: "$ cost passed " + at, note: "Cost passed " + at, id: fmt.Sprintf("cost-%d", n)}
				if k := len(s.Events); k > 0 {
					a.ev = s.Events[k-1]
				}
				out = append(out, a)
			}
			w.costSteps = max(w.costSteps, n)
		}
	}
	if l.LongTurn > 0 && len(s.Turns) > 0 {
		t := s.Turns[len(s.Turns)-1]
		last := t.Events[len(t.Events)-1]
		running := !t.Done && t != w.endedEarly && !t.Start.IsZero() &&
			!(last.Kind == parse.KindSystem && strings.HasPrefix(last.Text, interrupted))
		if running && t != w.longTurn && now.Sub(t.Start) >= l.LongTurn {
			w.longTurn = t
			if first {
				return out
			}
			d := waited(now.Sub(t.Start))
			out = append(out, &alert{kind: alertLong, text: glyphPending + " turn running " + d,
				note: "Turn running for " + d, ev: t.Events[0], id: fmt.Sprintf("long-%d", t.Start.UnixNano())})
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
