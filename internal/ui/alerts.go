package ui

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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

// liveState is what onTail compares before and after new lines.
type liveState struct {
	events, errors, ctx int
	turn                *parse.Turn
	done                bool
}

func (v *view) liveState() liveState {
	st := liveState{events: len(v.sess.Events), errors: v.stats.Errors, ctx: ctxPct(v.sess)}
	if n := len(v.sess.Turns); n > 0 {
		st.turn = v.sess.Turns[n-1]
		st.done = st.turn.Done
	}
	return st
}

// liveAlerts compares the session against its state before new lines
// arrived and raises alerts, the most important last.
func (v *view) liveAlerts(before liveState) []*alert {
	var out []*alert
	for _, e := range v.sess.Events[before.events:] {
		switch {
		case e.Kind == parse.KindCompact:
			out = append(out, &alert{alertCompact, glyphCompact + " compacted", e})
		case e.Tool != nil && e.Tool.Kind != parse.ToolAgent && !e.Tool.HasResult:
			// Subagents legitimately run for minutes; other tools are
			// watched for stalls.
			v.pending[e] = false
		}
	}
	if pct := ctxPct(v.sess); before.ctx < ctxAlertPct && pct >= ctxAlertPct {
		out = append(out, &alert{alertContext, fmt.Sprintf("ctx %d%%", pct), v.sess.Events[len(v.sess.Events)-1]})
	}
	if n := len(v.sess.Turns); n > 0 {
		t := v.sess.Turns[n-1]
		if t.Done && (t != before.turn || !before.done) {
			out = append(out, &alert{alertTurn, glyphOK + " your turn", t.Events[len(t.Events)-1]})
		}
	}
	if n := v.stats.Errors - before.errors; n > 0 {
		v.unseenErrs += n
		var last *parse.Event
		for _, e := range v.sess.Events {
			if e.Tool != nil && e.Tool.IsError && (last == nil || e.Tool.ResultTime.After(last.Tool.ResultTime)) {
				last = e
			}
		}
		out = append(out, &alert{alertError, glyphErr + " error: " + last.Tool.Name, last})
	}
	return out
}

// checkStalls raises one alert per live tool call still without a result
// after stall.
func (v *view) checkStalls(now time.Time, stall time.Duration) []*alert {
	var out []*alert
	for e, alerted := range v.pending {
		switch {
		case e.Tool.HasResult:
			delete(v.pending, e)
		case !alerted && now.Sub(e.Time) >= stall:
			v.pending[e] = true
			out = append(out, &alert{alertStall, fmt.Sprintf("%s waiting %s on %s · may be a permission prompt",
				glyphPending, dur(now.Sub(e.Time).Truncate(time.Second)), e.Tool.Name), e})
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

// bell rings the terminal bell when any alert is one the user chose.
func bell(as []*alert, on map[string]bool) tea.Cmd {
	for _, a := range as {
		if on[a.kind] {
			return func() tea.Msg {
				os.Stderr.WriteString("\a")
				return nil
			}
		}
	}
	return nil
}
