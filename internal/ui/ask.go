package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/review"
)

// While the live board or a session is open, snoop marks itself as
// listening, so the PermissionRequest hook waits for an answer from it,
// and reads the requests waiting. Both touch the disk, so they run as
// commands off the UI.

const listenEvery = 2 * time.Second

type asksMsg struct {
	reqs  []review.Request
	turns map[string]review.Turn
	err   error
}

// listening reports whether snoop can show requests: on the board or in a
// session, not in the picker.
func (a *App) listening() bool { return a.board != nil || len(a.stack) > 0 }

// askCmds refreshes the listening file every listenEvery and reads the
// waiting requests, one read at a time.
func (a *App) askCmds(now time.Time) []tea.Cmd {
	if a.opts.AsksDir == "" || a.opts.ListeningFile == "" || !a.listening() {
		return nil
	}
	var cmds []tea.Cmd
	if now.Sub(a.lastListen) >= listenEvery {
		a.lastListen = now
		path := a.opts.ListeningFile
		cmds = append(cmds, func() tea.Msg {
			review.Listen(path, now)
			return nil
		})
	}
	if !a.asksPolling {
		a.asksPolling = true
		dir, turnsDir := a.opts.AsksDir, a.opts.TurnsDir
		cmds = append(cmds, func() tea.Msg {
			rs, err := review.Pending(dir, now)
			var ts map[string]review.Turn
			if turnsDir != "" {
				ts, _ = review.WaitingTurns(turnsDir, now)
			}
			return asksMsg{rs, ts, err}
		})
	}
	return cmds
}

// onAsks keeps the waiting requests by session, oldest first, and the
// turns waiting for a reply, and raises an alert for each new one: in its
// session's view when it's open, else on the board.
func (a *App) onAsks(m asksMsg) tea.Cmd {
	a.asksPolling = false
	if m.err != nil {
		return nil
	}
	turnAlerts := a.onTurns(m.turns)
	a.asks = map[string][]review.Request{}
	seen := map[string]bool{}
	var out []*alert
	for _, r := range m.reqs {
		a.asks[r.Session] = append(a.asks[r.Session], r)
		seen[r.ID] = true
		if !a.seenAsks[r.ID] {
			out = append(out, a.askAlert(r))
		}
	}
	a.seenAsks = seen
	return a.signal(append(turnAlerts, out...))
}

// askAlert raises the alert for a new request and returns it.
func (a *App) askAlert(r review.Request) *alert {
	note := r.Tool + " wants permission"
	if r.Tool == "Bash" {
		var in struct{ Command string }
		json.Unmarshal(r.Input, &in)
		note = "Bash wants to run " + oneLine(in.Command)
	}
	al := &alert{kind: alertAsk, text: "? " + askSummary(r), note: note, id: r.ID, path: r.Session,
		title: noteTitle(filepath.Base(r.Cwd), "")}
	for _, v := range a.stack {
		if v.label != "" || v.sessionID() != r.Session {
			continue
		}
		for i := len(v.sess.Events) - 1; i >= 0; i-- {
			if e := v.sess.Events[i]; e.Tool != nil && e.Tool.Name == r.Tool && !e.Tool.HasResult {
				al.ev = e
				break
			}
		}
		v.own([]*alert{al})
		v.raise([]*alert{al})
		return al
	}
	if b := a.board; b != nil {
		for _, row := range b.rows {
			if row.info.ID == r.Session {
				al.text = row.project() + ": " + al.text
				al.path, al.title = row.f.Path, noteTitle(row.project(), row.info.Name())
				b.alert, b.alertShown = al, true
			}
		}
	}
	return al
}

// askSummary is a request on one line: the tool and what it wants.
func askSummary(r review.Request) string {
	var in map[string]any
	json.Unmarshal(r.Input, &in)
	str := func(k string) string { s, _ := in[k].(string); return s }
	s := r.Tool
	switch {
	case r.Tool == "Bash":
		s = "Bash: " + str("command")
	case r.Tool == "Edit" || r.Tool == "MultiEdit" || r.Tool == "Write" || r.Tool == "NotebookEdit":
		s = r.Tool + ": " + str("file_path")
	case r.Tool == "WebFetch":
		s = "WebFetch: " + str("url")
	case strings.HasPrefix(r.Tool, "mcp__"):
		if server, tool, ok := strings.Cut(strings.TrimPrefix(r.Tool, "mcp__"), "__"); ok {
			s = server + "·" + tool
		}
	}
	if r.AgentID != "" {
		via := r.AgentType
		if via == "" {
			via = "subagent"
		}
		s += " via " + via
	}
	return oneLine(s)
}

// answerer sends answers and replies, and holds the deny reason or the
// reply while it is typed.
type answerer struct {
	input   textinput.Model
	typing  bool
	target  review.Request
	replyTo string // the session a reply goes to; "" while a deny reason is typed
	about   string // what the reply quotes first, or ""
}

type answeredMsg struct {
	allow, always bool
	r             review.Request
	err           error
}

func respond(dir string, r review.Request, a review.Answer) tea.Cmd {
	return func() tea.Msg {
		return answeredMsg{allow: a.Behavior == "allow", always: len(a.Permissions) > 0, r: r, err: review.Respond(dir, r, a)}
	}
}

// alwaysSummary says what always allowing a request changes, as Claude
// Code's "don't ask again" would; "" when it offers nothing.
func alwaysSummary(r review.Request) string {
	where := map[string]string{
		"localSettings":   " in this project's local settings",
		"projectSettings": " in this project's settings",
		"userSettings":    " in your user settings",
		"session":         " for this session",
	}
	modes := map[string]string{"acceptEdits": "accept all edits", "plan": "plan mode", "bypassPermissions": "bypass permissions"}
	var parts []string
	for _, s := range r.SuggestionList() {
		switch s.Type {
		case "addRules":
			var rules []string
			for _, rl := range s.Rules {
				if rl.RuleContent == "" {
					rules = append(rules, rl.ToolName)
				} else {
					rules = append(rules, rl.ToolName+"("+rl.RuleContent+")")
				}
			}
			if len(rules) > 0 {
				verb := s.Behavior
				if verb == "" {
					verb = "allow"
				}
				parts = append(parts, verb+" "+strings.Join(rules, ", ")+where[s.Destination])
			}
		case "setMode":
			m := modes[s.Mode]
			if m == "" {
				m = s.Mode + " mode"
			}
			parts = append(parts, "switch to "+m+where[s.Destination])
		case "addDirectories":
			var ds []string
			for _, d := range s.Directories {
				ds = append(ds, tildePath(d))
			}
			parts = append(parts, "allow access to "+strings.Join(ds, ", ")+where[s.Destination])
		}
	}
	return strings.Join(parts, "; ")
}

// startDeny asks for the reason, which Claude sees; it may be empty.
func (an *answerer) startDeny(r review.Request) tea.Cmd {
	an.prompt("deny reason (optional) ", "what Claude should do instead")
	an.typing, an.target, an.replyTo = true, r, ""
	return an.input.Focus()
}

// startReply asks for a message to the session's agent, about the event e
// when it isn't nil: the reply quotes it first.
func (an *answerer) startReply(session string, e *parse.Event) tea.Cmd {
	label, about := quoteEvent(e)
	an.prompt("reply ", "a message for the agent; enter sends it")
	if label != "" {
		an.prompt("reply about "+label+" ", "a message for the agent; enter sends it")
	}
	an.typing, an.replyTo, an.about = true, session, about
	return an.input.Focus()
}

func (an *answerer) prompt(prompt, placeholder string) {
	if an.input.Prompt == "" {
		an.input = textinput.New()
		an.input.PromptStyle = sKey
	}
	an.input.Prompt, an.input.Placeholder = prompt, placeholder
	an.input.SetValue("")
}

// key takes every key while the reason or reply is typed: enter sends it
// (an empty reply sends nothing), esc cancels.
func (an *answerer) key(k tea.KeyMsg, asksDir, inboxDir string) tea.Cmd {
	switch k.String() {
	case "enter":
		an.typing = false
		an.input.Blur()
		text := strings.TrimSpace(an.input.Value())
		if an.replyTo != "" {
			if text == "" {
				return nil
			}
			if an.about != "" {
				text = an.about + "\n\n" + text
			}
			return reply(inboxDir, an.replyTo, text)
		}
		return respond(asksDir, an.target, review.Answer{Behavior: "deny", Message: text})
	case "esc":
		an.typing = false
		an.input.Blur()
		return nil
	}
	var cmd tea.Cmd
	an.input, cmd = an.input.Update(k)
	return cmd
}

// focusedSession is the session on screen: the open session, or the
// board's selected row.
func (a *App) focusedSession() string {
	switch v := a.top(); {
	case v != nil:
		return v.sessionID()
	case a.board != nil:
		a.board.asks, a.board.turns = a.asks, a.turns
		a.board.arrange(time.Now(), a.stallFor())
		if a.board.cursor < len(a.board.order) {
			return a.board.order[a.board.cursor].info.ID
		}
	}
	return ""
}

// askTarget is the oldest request of the session on screen.
func (a *App) askTarget() (review.Request, bool) {
	if rs := a.asks[a.focusedSession()]; len(rs) > 0 {
		return rs[0], true
	}
	return review.Request{}, false
}

// onAnswered drops the answered request and says how it went.
func (a *App) onAnswered(m answeredMsg) {
	switch {
	case m.err == nil && m.always:
		a.note = sOK.Render(" "+glyphOK+" always allowed "+askSummary(m.r)) + sDim.Render(": "+alwaysSummary(m.r))
	case m.err == nil && m.allow:
		a.note = sOK.Render(" " + glyphOK + " allowed " + askSummary(m.r))
	case m.err == nil:
		a.note = sDim.Render(" denied " + askSummary(m.r))
	case errors.Is(m.err, review.ErrAnswered):
		a.note = sDim.Render(" already answered")
	default:
		a.err = "answer failed: " + m.err.Error()
		return
	}
	rs := a.asks[m.r.Session]
	for i, r := range rs {
		if r.ID == m.r.ID {
			a.asks[m.r.Session] = append(rs[:i:i], rs[i+1:]...)
			break
		}
	}
}

// askLine is the footer of a session that is asking.
func askLine(rs []review.Request) string {
	more := ""
	if len(rs) > 1 {
		more = fmt.Sprintf("  (1 of %d)", len(rs))
	}
	return sAccent.Bold(true).Render(" ? "+askSummary(rs[0])) + sDim.Render(more) + "   " + askKeys(rs[0])
}

// askKeys are the keys that answer a request; always allowing only when
// Claude Code offers a way to stop asking.
func askKeys(r review.Request) string {
	if alwaysSummary(r) != "" {
		return keyHelp("A", "allow once", "Y", "always", "D", "deny")
	}
	return keyHelp("A", "allow once", "D", "deny")
}

// renderRequest shows what a request would do in full, so it can be
// judged before answering.
func renderRequest(r review.Request, w int) string {
	var in map[string]any
	json.Unmarshal(r.Input, &in)
	str := func(k string) string { s, _ := in[k].(string); return s }
	var b strings.Builder
	switch r.Tool {
	case "Bash":
		b.WriteString(sText.Render(wrap("$ "+str("command"), w)))
		if d := str("description"); d != "" {
			b.WriteString("\n" + sDim.Render(wrap(d, w)))
		}
	case "Edit":
		b.WriteString(sBold.Render(wrap(str("file_path"), w)) + "\n" + diff(str("old_string"), str("new_string"), w))
	case "Write":
		b.WriteString(sBold.Render(wrap(str("file_path"), w)) + "\n" + sAdd.Render(wrap(capLines(str("content"), maxDetailLines), w)))
	default:
		pretty, _ := json.MarshalIndent(in, "", "  ")
		b.WriteString(sText.Render(r.Tool) + "\n" + sDim.Render(wrap(string(pretty), w)))
	}
	if r.AgentID != "" || r.Cwd != "" {
		var where []string
		if r.AgentID != "" {
			via := r.AgentType
			if via == "" {
				via = "subagent"
			}
			where = append(where, "from "+via)
		}
		if r.Cwd != "" {
			where = append(where, "in "+tildePath(r.Cwd))
		}
		b.WriteString("\n" + sDim.Render(strings.Join(where, " · ")))
	}
	return b.String()
}
