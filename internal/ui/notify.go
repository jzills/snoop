package ui

import (
	"os"
	"strings"
)

// Desktop notifications (--notify) are terminal escape sequences, so they
// work over SSH and need no helper program. Terminals disagree on which
// sequence they understand; the one to send is picked from the
// environment. Inside tmux it is wrapped for passthrough, which tmux only
// forwards with `set -g allow-passthrough on`.

const notifyMax = 200 // runes of body text

// notifySeq is the escape sequence that shows body as a notification
// titled "snoop" in the terminal getenv describes.
func notifySeq(getenv func(string) string, body string) string {
	body = cleanNotify(body)
	var seq string
	switch {
	case getenv("KITTY_WINDOW_ID") != "":
		seq = "\x1b]99;i=snoop:d=0;snoop\a\x1b]99;i=snoop:p=body;" + body + "\a"
	case osc9(getenv):
		seq = "\x1b]9;snoop: " + body + "\a"
	default:
		seq = "\x1b]777;notify;snoop;" + body + "\a"
	}
	switch {
	case getenv("TMUX") != "":
		seq = "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	case getenv("STY") != "":
		seq = "\x1bP" + seq + "\x1b\\"
	}
	return seq
}

// osc9 reports terminals known to show OSC 9 as a notification.
func osc9(getenv func(string) string) bool {
	switch getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "ghostty":
		return true
	}
	// iTerm2 sets this, and it survives into tmux and SSH.
	return getenv("LC_TERMINAL") == "iTerm2"
}

// cleanNotify keeps body from ending or extending the sequence: control
// characters and the ";" OSC 777 separates fields with are replaced.
func cleanNotify(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == notifyMax {
			break
		}
		switch {
		case r == ';':
			r = ','
		case r == '\n' || r == '\t':
			r = ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			continue
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
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

func writeTerm(s string) { os.Stderr.WriteString(s) }
