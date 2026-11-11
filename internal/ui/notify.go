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

// badge says which signals are on, beside snoop in the top bar, so you
// know alerts will reach you: notifications are held while snoop has
// focus, and marked once one fails.
func (a *App) badge() string {
	var parts []string
	if a.opts.Notifier != nil {
		switch {
		case a.noteFailed:
			parts = append(parts, sErr.Render(glyphErr+" notify"))
		case a.focused:
			parts = append(parts, sDim.Render("○ notify"))
		default:
			parts = append(parts, sAccent.Render("● notify"))
		}
	}
	if a.opts.Bell {
		parts = append(parts, sAccent.Render("● bell"))
	}
	return strings.Join(parts, "  ")
}

// signalsHelp explains the badge: how notifications go out and for which
// alerts.
func (a *App) signalsHelp() (helpSection, bool) {
	if a.opts.Notifier == nil && !a.opts.Bell {
		return helpSection{}, false
	}
	var kinds []string
	for _, k := range AlertKinds {
		if a.opts.BellOn[k] {
			kinds = append(kinds, k)
		}
	}
	on := "for " + strings.Join(kinds, ", ")
	if len(kinds) == 0 {
		on = "for no alerts (see --alerts)"
	}
	s := helpSection{title: "Signals"}
	if n := a.opts.Notifier; n != nil {
		s.keys = append(s.keys,
			[2]string{"● notify", "desktop notifications by " + n.Name() + ", " + on},
			[2]string{"○ notify", "held while snoop has focus"},
			[2]string{glyphErr + " notify", "the last notification failed to send"})
	}
	if a.opts.Bell {
		s.keys = append(s.keys, [2]string{"● bell", "the terminal bell rings " + on})
	}
	return s, true
}
