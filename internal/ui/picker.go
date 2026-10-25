package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/jzills/snoop/internal/store"
	"github.com/jzills/snoop/internal/summary"
)

type sessionsMsg struct {
	infos []store.SessionInfo
	err   error
}

type openMsg struct{ path string }

// showBoardMsg switches from the picker to the live board.
type showBoardMsg struct{}

type picker struct {
	all         []store.SessionInfo
	shown       []int
	cursor      int
	offset      int
	input       textinput.Model
	filtering   bool
	projectOnly bool
	projectDir  string // transcript folder for the current working directory
	loaded      bool
	err         error

	// Summary across the listed sessions (s).
	summary  bool
	page     viewport.Model
	cache    *summary.Cache
	scanning bool
	scanned  int
	sums     map[string]summary.Session // by transcript path
	bad      map[string]bool            // unreadable transcripts
}

func newPicker(projectDir, cacheDir string) picker {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.PromptStyle = sKey
	ti.Placeholder = "fuzzy search titles, projects, branches"
	return picker{input: ti, projectDir: projectDir, page: viewport.New(0, 0),
		cache: summary.OpenCache(cacheDir), sums: map[string]summary.Session{}, bad: map[string]bool{}}
}

func loadSessions(root string) tea.Cmd {
	return func() tea.Msg {
		infos, err := store.List(root)
		return sessionsMsg{infos, err}
	}
}

type pickerSource []store.SessionInfo

func (p pickerSource) String(i int) string {
	s := p[i]
	return s.Name() + " " + filepath.Base(s.Project()) + " " + s.Branch
}
func (p pickerSource) Len() int { return len(p) }

func (p *picker) refilter() {
	base := make([]int, 0, len(p.all))
	for i, s := range p.all {
		if !p.projectOnly || s.ProjectDir == p.projectDir {
			base = append(base, i)
		}
	}
	q := strings.TrimSpace(p.input.Value())
	if q == "" {
		p.shown = base
	} else {
		src := make(pickerSource, len(base))
		for j, i := range base {
			src[j] = p.all[i]
		}
		p.shown = p.shown[:0]
		for _, m := range fuzzy.FindFrom(q, src) {
			p.shown = append(p.shown, base[m.Index])
		}
	}
	p.cursor = min(p.cursor, max(len(p.shown)-1, 0))
}

func (p *picker) update(msg tea.Msg, height int) tea.Cmd {
	switch msg := msg.(type) {
	case sessionsMsg:
		p.all, p.err, p.loaded = msg.infos, msg.err, true
		p.refilter()
	case tea.KeyMsg:
		if p.filtering {
			switch msg.String() {
			case "esc":
				p.filtering = false
				p.input.Blur()
				p.input.SetValue("")
				p.refilter()
				return nil
			case "enter":
				p.filtering = false
				p.input.Blur()
				return nil
			case "up", "ctrl+p", "down", "ctrl+n":
				// fall through to navigation
			default:
				var cmd tea.Cmd
				p.input, cmd = p.input.Update(msg)
				p.cursor = 0
				p.refilter()
				return cmd
			}
		}
		if p.summary {
			switch msg.String() {
			case "s", "c", "/", "esc":
			default:
				var cmd tea.Cmd
				p.page, cmd = p.page.Update(msg)
				return cmd
			}
		}
		switch msg.String() {
		case "s":
			return p.toggleSummary()
		case "w":
			return func() tea.Msg { return showBoardMsg{} }
		case "esc":
			p.input.SetValue("")
			p.cursor = 0
			p.refilter()
		case "/":
			p.filtering = true
			return p.input.Focus()
		case "up", "k", "ctrl+p":
			p.cursor = max(p.cursor-1, 0)
		case "down", "j", "ctrl+n":
			p.cursor = min(p.cursor+1, max(len(p.shown)-1, 0))
		case "pgup", "ctrl+u":
			p.cursor = max(p.cursor-height/2, 0)
		case "pgdown", "ctrl+d":
			p.cursor = min(p.cursor+height/2, max(len(p.shown)-1, 0))
		case "g", "home":
			p.cursor = 0
		case "G", "end":
			p.cursor = max(len(p.shown)-1, 0)
		case "c":
			p.projectOnly = !p.projectOnly
			p.cursor = 0
			p.refilter()
		case "enter", "l", "right":
			if len(p.shown) > 0 {
				path := p.all[p.shown[p.cursor]].Path
				return func() tea.Msg { return openMsg{path} }
			}
		}
	}
	return nil
}

func (p *picker) view(w, h int) string {
	var b strings.Builder
	scope := "all projects"
	if p.projectOnly {
		scope = "this project"
	}
	what := "  Claude Code sessions · "
	if p.summary {
		what = "  Summary of "
	}
	head := sTitle.Render(" snoop") + sDim.Render(what) +
		sText.Render(fmt.Sprintf("%d", len(p.shown))) + sDim.Render(" sessions in "+scope)
	if prog := p.scanProgress(); prog != "" {
		head += "  " + prog
	}
	b.WriteString(fit(head, w) + "\n")
	b.WriteString(sFaint.Render(strings.Repeat("─", w)) + "\n")

	bodyH := h - 4
	if p.summary {
		b.WriteString(p.summaryView(w, bodyH) + "\n")
		b.WriteString(sFaint.Render(strings.Repeat("─", w)) + "\n")
		if p.filtering || p.input.Value() != "" {
			b.WriteString(fit(p.input.View(), w))
		} else {
			b.WriteString(fit(" "+keyHelp("↑↓", "scroll", "s", "sessions", "c", "this project", "/", "search", "?", "help"), w))
		}
		return b.String()
	}
	switch {
	case p.err != nil:
		b.WriteString(sErr.Render(" " + p.err.Error()))
	case !p.loaded:
		b.WriteString(sDim.Render(" loading…"))
	case len(p.shown) == 0:
		b.WriteString(sDim.Render(" no sessions"))
	}
	p.offset = scrollOffset(p.offset, p.cursor, len(p.shown), bodyH)
	projW := min(22, w/5)
	branchW := min(20, w/6)
	for i := p.offset; i < len(p.shown) && i < p.offset+bodyH; i++ {
		s := p.all[p.shown[i]]
		sel := i == p.cursor
		st := func(base lipgloss.Style) lipgloss.Style {
			if sel {
				return base.Background(cSelBg)
			}
			return base
		}
		mark := st(sText).Render(" ")
		if sel {
			mark = st(sAccent).Render("▌")
		}
		nameStyle := sText
		if sel {
			nameStyle = sBold
		}
		fixed := 1 + 7 + projW + branchW + 7 + 4
		line := mark +
			st(sDim).Render(fit(" "+ago(s.ModTime), 7)) + st(sDim).Render(" ") +
			st(sAccent).Render(fit(filepath.Base(s.Project()), projW)) + st(sDim).Render(" ") +
			st(sDim).Render(fit(s.Branch, branchW)) + st(sDim).Render(" ") +
			st(nameStyle).Render(fit(s.Name(), max(w-fixed, 10))) +
			st(sDim).Render(fit(fmt.Sprintf("%7s", bytesize(s.Size)), 7))
		b.WriteString(fit(line, w) + "\n")
	}
	// Pad to the footer.
	lines := strings.Count(b.String(), "\n")
	if pad := h - 2 - lines; pad > 0 {
		b.WriteString(strings.Repeat("\n", pad))
	}
	b.WriteString(sFaint.Render(strings.Repeat("─", w)) + "\n")
	if p.filtering || p.input.Value() != "" {
		b.WriteString(fit(p.input.View(), w))
	} else {
		b.WriteString(fit(" "+keyHelp("↑↓", "move", "enter", "open", "/", "search", "c", "this project", "s", "summary", "?", "help"), w))
	}
	return b.String()
}
