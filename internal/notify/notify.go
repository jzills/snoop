// Package notify shows desktop notifications, choosing one way to deliver
// them for where snoop runs: the terminal's own notifications (escape
// sequences, which also work over SSH), or the system's (a Windows toast
// from WSL, osascript on macOS, notify-send on a Linux desktop).
package notify

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

type Note struct {
	Title string
	Body  string
}

// Sender delivers notes one way.
type Sender interface {
	Send(Note) error
	Name() string // for --version-like reporting and tests
}

// Modes --notify accepts.
const (
	Auto     = "auto"
	Terminal = "terminal"
	System   = "system"
)

// Env is what Choose reads, so tests can describe any machine.
type Env struct {
	Getenv   func(string) string
	LookPath func(string) (string, error)
	GOOS     string
	Out      io.Writer // where terminal sequences go
}

// Choose picks how to deliver notes. Auto prefers the terminal where its
// notifications are native or the system's would land on another machine
// (SSH), then the system's, then the terminal's escape sequences.
func Choose(mode string, env Env) (Sender, error) {
	term := terminal{getenv: env.Getenv, out: env.Out}
	switch mode {
	case Terminal:
		return term, nil
	case System:
		if s := system(env); s != nil {
			return s, nil
		}
		return nil, errors.New("no system notifications here (looked for powershell.exe under WSL, osascript on macOS, notify-send with a display on Linux)")
	case Auto, "":
		if env.Getenv("SSH_CONNECTION") != "" || env.Getenv("SSH_TTY") != "" || nativeTerminal(env.Getenv) {
			return term, nil
		}
		if s := system(env); s != nil {
			return s, nil
		}
		return term, nil
	}
	return nil, fmt.Errorf("unknown notify mode %q (want %s, %s or %s)", mode, Auto, Terminal, System)
}

func system(env Env) Sender {
	has := func(name string) bool { _, err := env.LookPath(name); return err == nil }
	switch {
	case (env.Getenv("WSL_DISTRO_NAME") != "" || env.Getenv("WSL_INTEROP") != "") && has("powershell.exe"):
		return command{"windows toast", wslToast}
	case env.GOOS == "darwin" && has("osascript"):
		return command{"osascript", osascript}
	case env.GOOS == "linux" && (env.Getenv("DISPLAY") != "" || env.Getenv("WAYLAND_DISPLAY") != "") && has("notify-send"):
		return command{"notify-send", notifySend}
	}
	return nil
}

// nativeTerminal reports terminals whose escape-sequence notifications are
// native ones, and that focus the right tab when clicked.
func nativeTerminal(getenv func(string) string) bool {
	return getenv("KITTY_WINDOW_ID") != "" || osc9(getenv)
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

// terminal writes an escape sequence the terminal turns into a notification.
type terminal struct {
	getenv func(string) string
	out    io.Writer
}

func (t terminal) Name() string { return "terminal" }

func (t terminal) Send(n Note) error {
	_, err := io.WriteString(t.out, Sequence(t.getenv, n))
	return err
}

const maxText = 200 // runes of a title or body

// Sequence is the escape sequence that shows n in the terminal getenv
// describes: OSC 99 in kitty, OSC 9 where it is a notification, OSC 777
// elsewhere. Inside tmux it is wrapped for passthrough, which tmux only
// forwards with `set -g allow-passthrough on`.
func Sequence(getenv func(string) string, n Note) string {
	title, body := clean(n.Title), clean(n.Body)
	var seq string
	switch {
	case getenv("KITTY_WINDOW_ID") != "":
		seq = "\x1b]99;i=snoop:d=0;" + title + "\a\x1b]99;i=snoop:p=body;" + body + "\a"
	case osc9(getenv):
		seq = "\x1b]9;" + title + ": " + body + "\a"
	default:
		seq = "\x1b]777;notify;" + title + ";" + body + "\a"
	}
	switch {
	case getenv("TMUX") != "":
		seq = "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	case getenv("STY") != "":
		seq = "\x1bP" + seq + "\x1b\\"
	}
	return seq
}

// clean keeps text from ending or extending a sequence: control characters
// and the ";" OSC 777 separates fields with are replaced.
func clean(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == maxText {
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

// command runs a program that shows the note; the text travels as
// arguments, never through a shell.
type command struct {
	name string
	argv func(Note) []string
}

func (c command) Name() string { return c.name }

func (c command) Send(n Note) error {
	n = Note{clean(n.Title), clean(n.Body)}
	args := c.argv(n)
	cmd := exec.Command(args[0], args[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v: %s", c.name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func osascript(n Note) []string {
	return []string{"osascript",
		"-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)", "-e", "end run",
		n.Title, n.Body}
}

func notifySend(n Note) []string {
	return []string{"notify-send", "--app-name=snoop", n.Title, n.Body}
}

// wslToast shows a Windows toast from WSL through Windows PowerShell. The
// script is passed encoded, and the text as single-quoted literals, so
// nothing in it is ever interpreted.
func wslToast(n Note) []string {
	lit := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := `$ErrorActionPreference = 'Stop'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$x = $t.GetElementsByTagName('text')
$x.Item(0).AppendChild($t.CreateTextNode(` + lit(n.Title) + `)) > $null
$x.Item(1).AppendChild($t.CreateTextNode(` + lit(n.Body) + `)) > $null
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe').Show([Windows.UI.Notifications.ToastNotification]::new($t))
`
	return []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePS(script)}
}

// encodePS is a script as -EncodedCommand takes it: base64 of UTF-16LE.
func encodePS(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// claimAge is how long a claim is kept before it is cleaned up.
const claimAge = time.Hour

// Claim records that the notification identified by key was sent, and
// reports whether this caller is the first: several snoops watching the
// same session each raise the same alert, and only one should notify. An
// empty dir claims nothing and always succeeds.
func Claim(dir, key string, now time.Time) bool {
	if dir == "" {
		return true
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return true
	}
	sum := sha256.Sum256([]byte(key))
	f, err := os.OpenFile(filepath.Join(dir, hex.EncodeToString(sum[:12])), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return !errors.Is(err, os.ErrExist)
	}
	f.Close()
	sweep(dir, now)
	return true
}

// sweep removes claims older than claimAge.
func sweep(dir string, now time.Time) {
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > claimAge {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
