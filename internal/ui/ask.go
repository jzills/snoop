package ui

import (
	"encoding/json"
	"strings"
	"time"

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
