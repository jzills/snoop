// Package ui is the Bubble Tea interface: a session picker and a stack of
// session views (top-level session, then any subagents opened from it).
package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/tail"
)

type Options struct {
	Root       string // transcript root, normally ~/.claude/projects
	ProjectDir string // transcript folder for the current directory
	Path       string // open this transcript directly
	Follow     bool   // attach to the newest session and switch when a new one starts
}

type App struct {
	opts   Options
	w, h   int
	picker picker
	stack  []*view
	err    string

	showHelp bool
	helpVP   viewport.Model
}

type tickMsg struct{}

func New(opts Options) *App {
	p := newPicker(opts.ProjectDir)
	return &App{opts: opts, picker: p}
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{loadSessions(a.opts.Root), tick()}
	if a.opts.Path != "" {
		path := a.opts.Path
		cmds = append(cmds, func() tea.Msg { return openMsg{path} })
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
	case tickMsg:
		return a, tick()
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
		} else {
			a.picker.cursor = max(0, min(a.picker.cursor+d, len(a.picker.shown)-1))
		}
		return a, nil
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
		return a, a.push(v)
	case tailMsg:
		if msg.u.Err == tail.ErrClosed {
			return a, nil
		}
		if msg.u.NewSession != "" && a.opts.Follow && len(a.stack) > 0 && msg.v == a.stack[0] {
			path := msg.u.NewSession
			return a, tea.Batch(msg.v.waitTail(), func() tea.Msg { return openMsg{path} })
		}
		msg.v.onTail(msg.u)
		return a, msg.v.waitTail()
	case tea.KeyMsg:
		a.err = ""
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		}
		v := a.top()
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
		if v == nil {
			if msg.String() == "q" && !a.picker.filtering {
				return a, tea.Quit
			}
			if msg.String() == "esc" && !a.picker.filtering && a.picker.input.Value() == "" {
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
				if a.top() == nil {
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

func (a *App) View() string {
	if a.w == 0 {
		return ""
	}
	if a.showHelp {
		return a.helpView()
	}
	var out string
	if v := a.top(); v != nil {
		out = v.render(a.w, a.h)
	} else {
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
