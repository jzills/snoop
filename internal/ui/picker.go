package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/jzills/snoop/internal/store"
	"github.com/jzills/snoop/internal/summary"
)

type sessionsMsg struct {
	infos []store.SessionInfo
	err   error
}

// openMsg opens a session, with search set in its timeline when given, or
// a subagent's transcript of the session root, labelled label.
type openMsg struct {
	path   string
	search string
	root   string // the session a subagent belongs to; "" for a session
	label  string
}

// openedMsg is a view parsed in the background for the open numbered seq.
type openedMsg struct {
	v   *view
	err error
	seq int
	openMsg
}

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
	sort        listSort // by pickerCols
	projectDir  string   // transcript folder for the current working directory
	loaded      bool
	err         error

	// Find in every session's prompts, files and commands (f); the input
	// is shared with the fuzzy search.
	find bool
	hits map[string]summary.Hit // by transcript path

	// Summary across the listed sessions (s).
	summary  bool
	page     viewport.Model
	cache    *summary.Cache
	scanning bool
	saved    time.Time // when a running scan last saved the cache
	scanned  int
	sums     map[string]summary.Session // by transcript path
	bad      map[string]bool            // unreadable transcripts
}

func newPicker(projectDir, cacheDir string) picker {
	ti := textinput.New()
	ti.PromptStyle = sKey
	p := picker{input: ti, projectDir: projectDir, page: viewport.New(0, 0),
		cache: summary.OpenCache(cacheDir), sums: map[string]summary.Session{}, bad: map[string]bool{}}
	p.setFind(false)
	return p
}

// setFind switches the input between fuzzy search and find.
func (p *picker) setFind(on bool) {
	p.find = on
	if on {
		p.input.Prompt = "find "
		p.input.Placeholder = "a file, command or prompt in any session"
	} else {
		p.input.Prompt = "/ "
		p.input.Placeholder = "fuzzy search titles, projects, branches"
	}
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

// The picker's columns, in order; each sorts in its natural order.
var pickerCols = []struct {
	title string
	cmp   func(a, b store.SessionInfo) int
}{
	{"AGE", func(a, b store.SessionInfo) int { return cmpDesc(a.ModTime.UnixNano(), b.ModTime.UnixNano()) }},
	{"PROJECT", func(a, b store.SessionInfo) int {
		return cmpAsc(strings.ToLower(filepath.Base(a.Project())), strings.ToLower(filepath.Base(b.Project())))
	}},
	{"BRANCH", func(a, b store.SessionInfo) int { return cmpAsc(a.Branch, b.Branch) }},
	{"SESSION", func(a, b store.SessionInfo) int { return cmpAsc(strings.ToLower(a.Name()), strings.ToLower(b.Name())) }},
	{"SIZE", func(a, b store.SessionInfo) int { return cmpDesc(a.Size, b.Size) }},
}

// sortShown orders the listed sessions by the chosen column. Search and
// find results keep their own order (best match first) until a column is
// chosen.
func (p *picker) sortShown(query bool) {
	if query && p.sort.isDefault() {
		return
	}
	cmp := pickerCols[p.sort.col].cmp
	sort.SliceStable(p.shown, func(i, j int) bool {
		a, b := p.all[p.shown[i]], p.all[p.shown[j]]
		if c := cmp(a, b); c != 0 {
			return p.sort.less(c)
		}
		return a.ModTime.After(b.ModTime)
	})
}

// resort applies a new order, keeping the same session selected.
func (p *picker) resort() {
	sel := ""
	if p.cursor < len(p.shown) {
		sel = p.all[p.shown[p.cursor]].Path
	}
	p.refilter()
	for i, j := range p.shown {
		if p.all[j].Path == sel {
			p.cursor = i
		}
	}
}

func (p *picker) refilter() {
	base := make([]int, 0, len(p.all))
	for i, s := range p.all {
		if !p.projectOnly || s.ProjectDir == p.projectDir {
			base = append(base, i)
		}
	}
	q := strings.TrimSpace(p.input.Value())
	defer p.sortShown(q != "" || p.find)
	switch {
	case p.find:
		paths := make([]string, len(base))
		at := make(map[string]int, len(base))
		for j, i := range base {
			paths[j], at[p.all[i].Path] = p.all[i].Path, i
		}
		p.hits = map[string]summary.Hit{}
		p.shown = p.shown[:0]
		for _, h := range summary.Find(p.sums, paths, q) {
			p.hits[h.Path] = h
			p.shown = append(p.shown, at[h.Path])
		}
	case q == "":
		p.shown = append(p.shown[:0], base...)
	default:
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
				p.setFind(false)
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
			p.setFind(false)
			p.cursor = 0
			p.refilter()
		case "/", "f":
			if find := msg.String() == "f"; find != p.find {
				p.input.SetValue("")
				p.setFind(find)
				p.cursor = 0
				p.refilter()
			}
			p.filtering = true
			if p.find {
				return tea.Batch(p.input.Focus(), p.startScan())
			}
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
		case "o":
			p.sort.next(len(pickerCols))
			p.resort()
		case "O":
			p.sort.rev = !p.sort.rev
			p.resort()
		case "c":
			p.projectOnly = !p.projectOnly
			p.cursor = 0
			p.refilter()
		case "enter", "l", "right":
			if len(p.shown) > 0 {
				msg := openMsg{path: p.all[p.shown[p.cursor]].Path}
				if p.find {
					msg.search = strings.TrimSpace(p.input.Value())
				}
				return func() tea.Msg { return msg }
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
	// snoop on the left, the count on the right.
	n := sText.Render(fmt.Sprintf("%d", len(p.shown)))
	right := n + sDim.Render(" sessions in "+scope)
	switch {
	case p.summary:
		right = sDim.Render("Summary of ") + right
	case p.find:
		right = sDim.Render("Find · ") + n + sDim.Render(" matching in "+scope)
	}
	if prog := p.scanProgress(); prog != "" {
		right = prog + "  " + right
	}
	b.WriteString(topBar(right, w) + "\n")

	bodyH := h - 4
	if p.summary {
		b.WriteString(p.summaryView(w, bodyH) + "\n")
		b.WriteString(sFaint.Render(strings.Repeat("─", w)) + "\n")
		if p.filtering || p.input.Value() != "" {
			b.WriteString(fit(p.input.View(), w))
		} else {
			b.WriteString(fit(" "+keyHelp("s", "sessions", "c", "this project", "?", "help"), w))
		}
		return b.String()
	}
	switch {
	case p.err != nil:
		b.WriteString(sErr.Render(" " + p.err.Error()))
	case !p.loaded:
		b.WriteString(sDim.Render(" loading…"))
	case p.find && strings.TrimSpace(p.input.Value()) == "":
		b.WriteString(sDim.Render(" type to find a file, command or prompt in any session; every word must match"))
	case p.find && len(p.shown) == 0 && p.scanning:
		b.WriteString(sDim.Render(" no matches yet"))
	case len(p.shown) == 0 && p.find:
		b.WriteString(sDim.Render(" no matches"))
	case len(p.shown) == 0:
		b.WriteString(sDim.Render(" no sessions"))
	}
	projW := min(22, w/5)
	branchW := min(20, w/6)
	fixed := 1 + 7 + projW + branchW + 7 + 7
	if p.loaded && len(p.shown) > 0 {
		h := p.sort.head
		b.WriteString(fit(sColHead.Render(" "+fit(" "+h("AGE", 0), 7)+"  "+fit(h("PROJECT", 1), projW)+"  "+fit(h("BRANCH", 2), branchW)+"  "+
			fit(h("SESSION", 3), max(w-fixed, 10))+fmt.Sprintf("%7s", h("SIZE", 4))), w) + "\n")
		bodyH--
	}
	p.offset = scrollOffset(p.offset, p.cursor, len(p.shown), bodyH)
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
		line := mark +
			st(sDim).Render(fit(" "+ago(s.ModTime), 7)) + st(sDim).Render("  ") +
			st(sAccent).Render(fit(filepath.Base(s.Project()), projW)) + st(sDim).Render("  ") +
			st(sDim).Render(fit(s.Branch, branchW)) + st(sDim).Render("  ") +
			p.nameCell(s, st, nameStyle, max(w-fixed, 10)) +
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
		b.WriteString(fit(" "+keyHelp("enter", "open", "/", "search", "f", "find", "o", "sort", "w", "live", "?", "help"), w))
	}
	return b.String()
}

// nameCell is the session's name, or in find its matching entry.
func (p *picker) nameCell(s store.SessionInfo, st func(lipgloss.Style) lipgloss.Style, nameStyle lipgloss.Style, w int) string {
	h, ok := p.hits[s.Path]
	if !p.find || !ok {
		return st(nameStyle).Render(fit(s.Name(), w))
	}
	glyph := [...]string{"▸ ", "$ ", glyphUser + " "}[h.Kind]
	more := ""
	if h.N > 1 {
		more = fmt.Sprintf("  +%d more", h.N-1)
	}
	avail := max(w-lipgloss.Width(more)-lipgloss.Width(glyph), 1)
	text := ansi.Truncate(glyph+excerpt(oneLine(h.Text), p.input.Value(), avail), max(w-lipgloss.Width(more), 1), "…")
	return fit(st(nameStyle).Render(text)+st(sDim).Render(more), w)
}

// excerpt drops the start of a long s, so the first word of query it
// contains shows within w cells.
func excerpt(s, query string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	lower := []rune(strings.ToLower(s))
	if len(lower) != len(r) {
		return s
	}
	for _, t := range strings.Fields(strings.ToLower(query)) {
		i := strings.Index(string(lower), t)
		if i < 0 {
			continue
		}
		at := len([]rune(string(lower)[:i]))
		if end := at + len([]rune(t)); end > w-1 {
			start := min(max(at-w/3, 1), len(r)-1)
			return "…" + string(r[start:])
		}
		return s
	}
	return s
}
