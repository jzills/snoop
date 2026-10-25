package ui

import (
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/notify"
)

// Desktop notifications (--notify) go out for the alert kinds chosen with
// --alerts, at most one per session and kind a minute, and none while
// snoop's own window has focus. Several snoops watching one session send
// one notification between them.

const noteEvery = time.Minute

// noteTitle names a session in a notification: "api · Fix the flaky test".
func noteTitle(project, name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if r := []rune(name); len(r) > 60 {
		name = string(r[:59]) + "…"
	}
	if project == "" {
		return name
	}
	return project + " · " + name
}

// chosen is the newest alert of a kind the user picked, or nil.
func chosen(as []*alert, on map[string]bool) *alert {
	for i := len(as) - 1; i >= 0; i-- {
		if on[as[i].kind] {
			return as[i]
		}
	}
	return nil
}

// chosenPerSession is the newest chosen alert of each session, in the
// order they were raised: the live board raises alerts for many sessions
// at once, and each gets its own notification.
func chosenPerSession(as []*alert, on map[string]bool) []*alert {
	newest := map[string]int{}
	for i, a := range as {
		if on[a.kind] {
			newest[a.path] = i
		}
	}
	var out []*alert
	for i, a := range as {
		if j, ok := newest[a.path]; ok && j == i {
			out = append(out, a)
		}
	}
	return out
}

type notifyErrMsg struct{ err error }

// notifyCmd sends a notification for the newest chosen alert of each
// session, unless it shouldn't go out.
func (a *App) notifyCmd(as []*alert, now time.Time) tea.Cmd {
	if a.opts.Notifier == nil || a.focused {
		return nil
	}
	var cmds []tea.Cmd
	for _, c := range chosenPerSession(as, a.opts.BellOn) {
		cmds = append(cmds, a.notifyOne(c, now))
	}
	return tea.Batch(cmds...)
}

func (a *App) notifyOne(c *alert, now time.Time) tea.Cmd {
	every := c.path + "|" + c.kind
	if last, ok := a.noted[every]; ok && now.Sub(last) < noteEvery {
		return nil
	}
	if !notify.Claim(a.claimDir, c.key(), now) {
		return nil
	}
	a.noted[every] = now
	n, sender := notify.Note{Title: c.title, Body: c.note}, a.opts.Notifier
	if n.Title == "" {
		n.Title = "snoop"
	}
	return func() tea.Msg {
		if err := sender.Send(n); err != nil {
			return notifyErrMsg{err}
		}
		return nil
	}
}

func writeTerm(s string) { os.Stderr.WriteString(s) }
