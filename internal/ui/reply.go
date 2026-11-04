package ui

import (
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/review"
)

// A reply is a message to a session's agent, typed after R. It goes to the
// inbox, which the hooks read: when the session's turn has ended and its
// Stop hook waits, the agent gets it at once; otherwise after its current
// tool call, or with the next prompt typed in its terminal.

type repliedMsg struct {
	session string
	waiting bool // the session's turn had ended and its hook was waiting
	err     error
}

func reply(inbox, session, text string) tea.Cmd {
	return func() tea.Msg {
		_, err := review.Reply(inbox, session, text, time.Now())
		return repliedMsg{session: session, err: err}
	}
}

// onReplied says where the reply went.
func (a *App) onReplied(m repliedMsg) {
	if m.err != nil {
		a.err = "reply failed: " + m.err.Error()
		return
	}
	if _, ok := a.turns[m.session]; ok {
		a.note = sOK.Render(" "+glyphOK+" sent") + sDim.Render("; the agent has it now")
		return
	}
	a.note = sOK.Render(" "+glyphOK+" sent") +
		sDim.Render("; the agent gets it after its current tool call, or with your next prompt")
}

// onTurns keeps the turns waiting for a reply, and returns an alert for
// each that just ended. Claude Code records a turn as done only once the
// Stop hook returns, so for a waiting turn this is when it ends.
func (a *App) onTurns(ts map[string]review.Turn) []*alert {
	a.turns = ts
	seen := map[string]time.Time{}
	var out []*alert
	for id, t := range ts {
		seen[id] = t.Ended
		if ended, ok := a.seenTurns[id]; ok && ended.Equal(t.Ended) {
			continue
		}
		if al := a.turnAlert(t); al != nil {
			out = append(out, al)
		}
	}
	// A session keeps its last turn while the hook may still be ending, so
	// a turn missed by one poll isn't alerted again.
	for id, ended := range a.seenTurns {
		if _, ok := seen[id]; !ok && time.Since(ended) < turnMemory {
			seen[id] = ended
		}
	}
	a.seenTurns = seen
	return out
}

// turnMemory is how long snoop remembers an alerted turn.
const turnMemory = time.Hour

// turnAlert raises the end-of-turn alert for a waiting turn, in its view
// when it's open, else on the board, and has the watch skip the alert it
// would raise when the turn is finally recorded.
func (a *App) turnAlert(t review.Turn) *alert {
	note := "Your turn"
	if t.Last != "" {
		note += ": " + oneLine(t.Last)
	}
	for _, v := range a.stack {
		if v.label != "" || v.sessionID() != t.Session || len(v.sess.Turns) == 0 {
			continue
		}
		last := v.sess.Turns[len(v.sess.Turns)-1]
		if last.Done {
			return nil
		}
		v.endedEarly = last
		al := &alert{kind: alertTurn, text: glyphOK + " your turn", note: note, ev: last.Events[len(last.Events)-1],
			path: v.rootPath, title: noteTitle(filepath.Base(t.Cwd), "")}
		v.own([]*alert{al})
		v.raise([]*alert{al})
		return al
	}
	if b := a.board; b != nil {
		for _, row := range b.rows {
			if row.info.ID != t.Session || len(row.f.Sess.Turns) == 0 {
				continue
			}
			last := row.f.Sess.Turns[len(row.f.Sess.Turns)-1]
			if last.Done {
				return nil
			}
			row.watch.endedEarly = last
			al := &alert{kind: alertTurn, text: row.project() + ": " + glyphOK + " your turn", note: note,
				path: row.f.Path, title: noteTitle(row.project(), row.info.Name())}
			b.alert, b.alertShown = al, true
			return al
		}
	}
	return nil
}

// replyLine is the footer of a session whose turn waits for a reply.
func replyLine(t review.Turn) string {
	// The key first, so a long message is what gets cut off.
	return sOK.Render(" "+glyphOK+" your turn") + "   " + keyHelp("R", "reply") + sDim.Render("   "+oneLine(t.Last))
}
