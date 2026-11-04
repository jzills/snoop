package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/review"
)

// A reply is a message to a session's agent, typed after R. It goes to the
// inbox, which the hooks read: when the session's turn has ended and its
// Stop hook waits, the agent gets it at once; otherwise after its current
// tool call, or with the next prompt typed in its terminal.

type repliedMsg struct {
	session   string
	text      string
	installed bool // the hooks that deliver it are installed
	err       error
}

// reply sends text to the session's inbox, or with no hooks to deliver it
// from there, leaves it for onReplied to copy.
func reply(inbox, session, text string, settings []string) tea.Cmd {
	return func() tea.Msg {
		m := repliedMsg{session: session, text: text, installed: review.Installed(settings...)}
		if m.installed {
			_, m.err = review.Reply(inbox, session, text, time.Now())
		}
		return m
	}
}

// replySettings are the settings files that may hold the session's hooks:
// the user's, and its project's.
func (a *App) replySettings(session string) []string {
	for _, v := range a.stack {
		if v.sessionID() == session {
			return v.settingsFiles()
		}
	}
	fs := []string{review.UserSettings(a.opts.Root)}
	if b := a.board; b != nil {
		for _, r := range b.rows {
			if r.info.ID == session && r.f.Sess.CWD != "" {
				fs = append(fs, filepath.Join(r.f.Sess.CWD, ".claude", "settings.json"), filepath.Join(r.f.Sess.CWD, ".claude", "settings.local.json"))
			}
		}
	}
	return fs
}

// onReplied says where the reply went.
func (a *App) onReplied(m repliedMsg) tea.Cmd {
	switch {
	case m.err != nil:
		a.err = "reply failed: " + m.err.Error()
		return nil
	case !m.installed:
		a.note = sKey.Render(" Copied the reply to the clipboard.") +
			sDim.Render(" To send replies straight to the agent, run snoop hooks install.")
		return copyCmd(m.text)
	}
	if _, ok := a.turns[m.session]; ok {
		a.note = sOK.Render(" "+glyphOK+" sent") + sDim.Render("; the agent has it now")
		return nil
	}
	a.note = sOK.Render(" "+glyphOK+" sent") +
		sDim.Render("; the agent gets it after its current tool call, or with your next prompt")
	return nil
}

// onTurns keeps the turns waiting for a reply, and returns an alert for
// each that just ended. Claude Code records a turn as done only once the
// Stop hook returns, so for a waiting turn this is when it ends. Every
// watch on a waiting session notes it, a view opened since included, so
// none alerts the end again when it's recorded.
func (a *App) onTurns(ts map[string]review.Turn) []*alert {
	a.turns = ts
	for id := range ts {
		for _, v := range a.stack {
			if v.label == "" && v.sessionID() == id {
				v.markWaited(v.sess)
			}
		}
		if b := a.board; b != nil {
			for _, r := range b.rows {
				if r.info.ID == id {
					r.watch.markWaited(r.f.Sess)
				}
			}
		}
	}
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

// maxQuote is how many lines of a result or message a reply quotes.
const maxQuote = 8

// replyAbout is the event a reply from this view is about: the selected
// call or message in the Timeline, or the selected call in Tools.
func (v *view) replyAbout() *parse.Event {
	switch v.tab {
	case tabTimeline:
		return v.selected()
	case tabTools:
		if g := v.selectedGroup(); g != nil {
			return g.ev
		}
	}
	return nil
}

// quoteEvent is what a reply about e says first, and a short label for it;
// both "" for an event a reply can't be about.
func quoteEvent(e *parse.Event) (label, about string) {
	if e == nil {
		return "", ""
	}
	at := ""
	if !e.Time.IsZero() {
		at = " at " + e.Time.Local().Format("15:04:05")
	}
	quote := func(s string) string {
		var b strings.Builder
		ls := parse.Lines(strings.TrimRight(s, "\n"))
		for i, l := range ls {
			if i == maxQuote {
				fmt.Fprintf(&b, "    … %d more %s\n", len(ls)-maxQuote, plural(len(ls)-maxQuote, "line"))
				break
			}
			b.WriteString("    " + l + "\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}
	switch e.Kind {
	case parse.KindTool:
		t := e.Tool
		label = t.Name
		if t.Summary != "" {
			label += ": " + t.Summary
		}
		label = ansi.Truncate(oneLine(label), 40, "…")
		about = "About the " + t.Name + " call" + at
		if t.Summary != "" {
			about += " (" + oneLine(t.Summary) + ")"
		}
		switch {
		case !t.HasResult:
			about += ", still running."
		case t.IsError:
			about += ", which failed:\n" + quote(t.Result)
		case strings.TrimSpace(t.Result) != "":
			about += ", which returned:\n" + quote(t.Result)
		default:
			about += "."
		}
	case parse.KindText:
		label = "your message" + at
		about = "About your message" + at + ":\n" + quote(e.Text)
	case parse.KindThinking:
		label = "your thinking" + at
		about = "About your thinking" + at + ":\n" + quote(e.Text)
	case parse.KindUser:
		label = "my prompt" + at
		about = "About my prompt" + at + ":\n" + quote(e.Text)
	}
	return label, about
}
