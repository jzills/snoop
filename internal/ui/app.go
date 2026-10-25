// Package ui is the Bubble Tea interface: a session picker and a stack of
// session views (top-level session, then any subagents opened from it).
package ui

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/notify"
	"github.com/jzills/snoop/internal/tail"
)

type Options struct {
	Root       string // transcript root, normally ~/.claude/projects
	ProjectDir string // transcript folder for the current directory
	Path       string // open this transcript directly
	Follow     bool   // attach to the newest session and switch when a new one starts

	CacheDir string // where session summaries are cached; "" keeps them in memory

	Live bool // start on the live board

	Stall    time.Duration   // alert when a live tool call has no result after this long
	Bell     bool            // ring the terminal bell on alerts
	Notifier notify.Sender   // send desktop notifications on alerts; nil for none
	BellOn   map[string]bool // alert kinds that ring the bell or notify
}

type App struct {
	opts   Options
	w, h   int
	picker picker
	board  *board // the live board, while it is showing or under the stack
	stack  []*view
	err    string

	showHelp bool
	helpVP   viewport.Model

	// Notifications.
	focused  bool                 // snoop's window has focus, as the terminal reports it
	noted    map[string]time.Time // last notification per session and alert kind
	claimDir string               // shared by every snoop, so one event notifies once
}

type tickMsg struct{}

func New(opts Options) *App {
	p := newPicker(opts.ProjectDir, opts.CacheDir)
	a := &App{opts: opts, picker: p, noted: map[string]time.Time{}}
	if opts.CacheDir != "" {
		a.claimDir = filepath.Join(opts.CacheDir, "notified")
	}
	if opts.Live {
		a.board = newBoard(opts.Root)
	}
	return a
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{loadSessions(a.opts.Root), tick()}
	if a.board != nil {
		cmds = append(cmds, a.board.scanCmd(time.Now()))
	}
	if a.opts.Path != "" {
		path := a.opts.Path
		cmds = append(cmds, func() tea.Msg { return openMsg{path: path} })
	}
	return tea.Batch(cmds...)
}

// tick redraws once a second so "● live" and relative ages decay.
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (a *App) top() *view {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

func (a *App) push(v *view) tea.Cmd {
	a.stack = append(a.stack, v)
	return v.waitTail()
}

func (a *App) pop() {
	if v := a.top(); v != nil {
		v.close()
		a.stack = a.stack[:len(a.stack)-1]
	}
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		return a, nil
	case tea.FocusMsg:
		a.focused = true
		return a, nil
	case tea.BlurMsg:
		a.focused = false
		return a, nil
	case notifyErrMsg:
		a.err = "notification failed: " + msg.err.Error()
		return a, nil
	case tickMsg:
		cmds := []tea.Cmd{tick()}
		for _, v := range a.stack {
			if v.pollSubs() {
				v.refresh()
			}
		}
		if a.opts.Stall > 0 {
			for _, v := range a.stack {
				as := v.stalls(time.Now(), a.opts.Stall)
				v.raise(as)
				cmds = append(cmds, a.signal(v.own(as)))
			}
		}
		if b := a.board; b != nil {
			now := time.Now()
			if now.Sub(b.lastScan) >= boardScan {
				cmds = append(cmds, b.scanCmd(now))
			}
			cmds = append(cmds, a.signal(b.poll(now, a.opts.Stall, a.isOpen)))
		}
		return a, tea.Batch(cmds...)
	case showBoardMsg:
		a.board = newBoard(a.opts.Root)
		return a, a.board.scanCmd(time.Now())
	case boardScanMsg:
		if a.board == nil {
			return a, nil
		}
		return a, tea.Batch(a.board.onScan(msg)...)
	case boardLoadedMsg:
		if a.board != nil {
			a.board.onLoaded(msg)
		}
		return a, nil
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return a, nil
		}
		d := 0
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			d = -3
		case tea.MouseButtonWheelDown:
			d = 3
		default:
			return a, nil
		}
		if a.showHelp {
			if d < 0 {
				a.helpVP.ScrollUp(-d)
			} else {
				a.helpVP.ScrollDown(d)
			}
		} else if v := a.top(); v != nil {
			v.wheel(d, msg.X >= a.w*11/20 && a.w >= wideLayout)
		} else if b := a.board; b != nil {
			b.cursor = max(0, min(b.cursor+d, len(b.order)-1))
		} else {
			a.picker.cursor = max(0, min(a.picker.cursor+d, len(a.picker.shown)-1))
		}
		return a, nil
	case scanStartedMsg, scanResultMsg, scanDoneMsg:
		return a, a.picker.onScan(msg)
	case openMsg:
		a.err = ""
		// A followed session that is replaced closes everything above it.
		v, err := openView(msg.path, msg.path, "", a.opts.Follow)
		if err != nil {
			a.err = err.Error()
			return a, nil
		}
		if a.opts.Follow {
			for len(a.stack) > 0 {
				a.pop()
			}
		}
		if msg.search != "" {
			// From find: show what matched, from the first match.
			v.search.SetValue(msg.search)
			v.refresh()
			v.cursor, v.follow = 0, false
		}
		return a, a.push(v)
	case tailMsg:
		if msg.u.Err == tail.ErrClosed {
			return a, nil
		}
		if msg.u.NewSession != "" && a.opts.Follow && len(a.stack) > 0 && msg.v == a.stack[0] {
			path := msg.u.NewSession
			return a, tea.Batch(msg.v.waitTail(), func() tea.Msg { return openMsg{path: path} })
		}
		as := msg.v.onTail(msg.u)
		return a, tea.Batch(msg.v.waitTail(), a.signal(msg.v.own(as)))
	case tea.KeyMsg:
		a.err = ""
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		}
		v := a.top()
		if v != nil && msg.String() != "!" {
			v.alertShown = false
		}
		if v != nil {
			v.flash = ""
		}
		// Keys typed faster than a read arrive as one multi-rune message;
		// outside text inputs, treat them as separate presses.
		typing := (v == nil && a.picker.filtering) || (v != nil && v.typing)
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !typing && !msg.Paste {
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				cmds = append(cmds, cmd)
			}
			return a, tea.Batch(cmds...)
		}
		if a.showHelp {
			switch msg.String() {
			case "?", "esc", "q":
				a.showHelp = false
			default:
				a.helpVP, _ = a.helpVP.Update(msg)
			}
			return a, nil
		}
		if msg.String() == "?" && !typing {
			a.showHelp = true
			a.helpVP.GotoTop()
			return a, nil
		}
		if v == nil && a.board != nil {
			switch msg.String() {
			case "q":
				return a, tea.Quit
			case "esc", "w":
				a.board = nil
				return a, loadSessions(a.opts.Root)
			}
			return a, a.board.update(msg, a.h)
		}
		if v == nil {
			if msg.String() == "q" && !a.picker.filtering {
				return a, tea.Quit
			}
			if msg.String() == "esc" && !a.picker.filtering && a.picker.input.Value() == "" && !a.picker.find {
				if a.picker.summary {
					return a, a.picker.toggleSummary()
				}
				return a, tea.Quit
			}
			return a, a.picker.update(msg, a.h)
		}
		if !v.typing {
			switch msg.String() {
			case "q":
				return a, tea.Quit
			case "esc", "backspace":
				if v.zoom || v.focusR {
					v.zoom, v.focusR = false, false
					return a, nil
				}
				if v.search.Value() != "" || v.kind != 0 {
					v.search.SetValue("")
					v.kind = 0
					v.refresh()
					return a, nil
				}
				a.pop()
				if a.top() == nil && a.board == nil {
					return a, loadSessions(a.opts.Root)
				}
				return a, nil
			}
		}
		cmd, sub := v.update(msg, a.w, a.h)
		if sub != nil {
			return a, a.push(sub)
		}
		return a, cmd
	}
	if v := a.top(); v == nil {
		return a, a.picker.update(msg, a.h)
	}
	return a, nil
}

// signal rings the bell and sends a notification for the newest alert of
// a kind the user chose.
func (a *App) signal(as []*alert) tea.Cmd {
	var bell tea.Cmd
	if out := a.signalText(as); out != "" {
		bell = func() tea.Msg {
			writeTerm(out)
			return nil
		}
	}
	return tea.Batch(bell, a.notifyCmd(as, time.Now()))
}

// signalText is what to write to the terminal for alerts: the bell.
func (a *App) signalText(as []*alert) string {
	if a.opts.Bell && chosen(as, a.opts.BellOn) != nil {
		return "\a"
	}
	return ""
}

// isOpen reports whether a session is shown in a view on the stack.
func (a *App) isOpen(path string) bool {
	for _, v := range a.stack {
		if v.path == path || v.rootPath == path {
			return true
		}
	}
	return false
}

// stallFor is the stall used to show a session as waiting on you; with the
// alert disabled (--stall 0) the default still applies to the display.
func (a *App) stallFor() time.Duration {
	if a.opts.Stall > 0 {
		return a.opts.Stall
	}
	return 30 * time.Second
}

func (a *App) View() string {
	if a.w == 0 {
		return ""
	}
	if a.showHelp {
		return a.helpView()
	}
	var out string
	switch v := a.top(); {
	case v != nil:
		out = v.render(a.w, a.h)
	case a.board != nil:
		out = a.board.view(a.w, a.h, time.Now(), a.stallFor())
	default:
		out = a.picker.view(a.w, a.h)
	}
	// Errors replace the footer until the next key press.
	if a.err != "" {
		if i := strings.LastIndexByte(out, '\n'); i >= 0 {
			out = out[:i+1] + fit(sErr.Render(" "+glyphErr+" "+a.err), a.w)
		}
	}
	return out
}
