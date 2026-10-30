package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/review"
)

// While the live board or a session is open, snoop marks itself as
// listening, so the PermissionRequest hook waits for an answer from it,
// and reads the requests waiting. Both touch the disk, so they run as
// commands off the UI.

const listenEvery = 2 * time.Second

type asksMsg struct {
	reqs []review.Request
	err  error
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
		dir := a.opts.AsksDir
		cmds = append(cmds, func() tea.Msg {
			rs, err := review.Pending(dir, now)
			return asksMsg{rs, err}
		})
	}
	return cmds
}

// onAsks keeps the waiting requests by session, oldest first.
func (a *App) onAsks(m asksMsg) {
	a.asksPolling = false
	if m.err != nil {
		return
	}
	a.asks = map[string][]review.Request{}
	for _, r := range m.reqs {
		a.asks[r.Session] = append(a.asks[r.Session], r)
	}
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

// answerer sends answers, and holds the deny reason while it is typed.
type answerer struct {
	input  textinput.Model
	typing bool
	target review.Request
}

type answeredMsg struct {
	allow bool
	r     review.Request
	err   error
}

func respond(dir string, r review.Request, a review.Answer) tea.Cmd {
	return func() tea.Msg {
		return answeredMsg{allow: a.Behavior == "allow", r: r, err: review.Respond(dir, r, a)}
	}
}

// startDeny asks for the reason, which Claude sees; it may be empty.
func (an *answerer) startDeny(r review.Request) tea.Cmd {
	if an.input.Prompt == "" {
		an.input = textinput.New()
		an.input.Prompt = "deny reason (optional) "
		an.input.PromptStyle = sKey
		an.input.Placeholder = "what Claude should do instead"
	}
	an.input.SetValue("")
	an.typing, an.target = true, r
	return an.input.Focus()
}

// key takes every key while the reason is typed: enter denies, esc
// cancels.
func (an *answerer) key(k tea.KeyMsg, dir string) tea.Cmd {
	switch k.String() {
	case "enter":
		an.typing = false
		an.input.Blur()
		return respond(dir, an.target, review.Answer{Behavior: "deny", Message: strings.TrimSpace(an.input.Value())})
	case "esc":
		an.typing = false
		an.input.Blur()
		return nil
	}
	var cmd tea.Cmd
	an.input, cmd = an.input.Update(k)
	return cmd
}

// askTarget is the oldest request of the session on screen: the open
// session, or the board's selected row.
func (a *App) askTarget() (review.Request, bool) {
	id := ""
	switch v := a.top(); {
	case v != nil:
		id = v.sessionID()
	case a.board != nil:
		a.board.asks = a.asks
		a.board.arrange(time.Now(), a.stallFor())
		if a.board.cursor < len(a.board.order) {
			id = a.board.order[a.board.cursor].info.ID
		}
	}
	if rs := a.asks[id]; len(rs) > 0 {
		return rs[0], true
	}
	return review.Request{}, false
}

// onAnswered drops the answered request and says how it went.
func (a *App) onAnswered(m answeredMsg) {
	switch {
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
	return sAccent.Bold(true).Render(" ? "+askSummary(rs[0])) + sDim.Render(more) + "   " +
		keyHelp("A", "allow once", "D", "deny")
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
