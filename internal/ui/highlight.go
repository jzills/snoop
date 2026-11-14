package ui

import (
	"hash/maphash"
	"slices"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/syntax"
)

// Code is colored by language in the background: lexing takes tens of
// milliseconds for a few hundred lines, too slow for Update or View. Code
// is never drawn plain while its colors are on the way. A pane leaves the
// code's lines blank for hlGrace, which covers most waits unnoticed, then
// shows a spinner until the colors land.
//
// So that panes rarely wait, the Diff tab colors its files in the job that
// runs git, and a session's edits and writes are colored ahead, as it
// opens and as calls arrive.

// hlMsg says the highlighter started or finished some code.
type hlMsg struct{}

// hlKey is a piece of code by its language and content, so the same code
// under two spellings of its path is colored once.
type hlKey struct {
	lang string
	sum  uint64
	n    int
}

type hlJob struct {
	key  hlKey
	path string // for its language
	src  string
}

// hlEntry is colored code and when it was last used, so a full cache
// drops what's gone longest unseen.
type hlEntry struct {
	spans [][]syntax.Span // nil when the lexer failed: drawn plain
	used  uint64
}

type highlighter struct {
	mu    sync.Mutex
	done  map[hlKey]*hlEntry
	clock uint64 // counts uses, for hlEntry.used
	// queued holds when a pane first asked for the code, or the zero time
	// for code only colored ahead.
	queued  map[hlKey]time.Time
	waiting int     // keys in queued that a pane asked for
	jobs    []hlJob // a stack: what a pane is waiting for goes on top
	waits   int     // lookups that found the code still on the way
	version int     // bumped as code a pane waited for lands, so cached renders redraw
	wake    chan struct{}
	ready   chan struct{}
	start   sync.Once
}

const (
	hlMaxEntries = 4096
	hlMaxBytes   = 512 << 10 // past this, code stays plain
	hlGrace      = 150 * time.Millisecond
	hlAhead      = 300  // calls colored ahead, newest first
	hlDiffLines  = 1500 // lexed with the diff, about 100 ms; the rest goes ahead
)

var (
	hl     = newHighlighter()
	hlSeed = maphash.MakeSeed()
	hlNow  = time.Now
)

func newHighlighter() *highlighter {
	return &highlighter{
		done:   map[hlKey]*hlEntry{},
		queued: map[hlKey]time.Time{},
		wake:   make(chan struct{}, 1),
		ready:  make(chan struct{}, 1),
	}
}

// hlState is how far along a piece of code is.
type hlState uint8

const (
	hlPlain   hlState = iota // no language matches, it's too big, or the lexer failed
	hlReady                  // the spans are here
	hlWaiting                // on the way
)

func keyFor(path, src string) (hlKey, bool) {
	if len(src) > hlMaxBytes {
		return hlKey{}, false
	}
	lang := syntax.Lang(path)
	if lang == "" {
		return hlKey{}, false
	}
	return hlKey{lang: lang, sum: maphash.String(hlSeed, src), n: len(src)}, true
}

// get is src's spans for a pane drawing them now. While they're on the
// way it returns hlWaiting and since, when a pane first asked.
func (h *highlighter) get(path, src string) (spans [][]syntax.Span, st hlState, since time.Time) {
	k, ok := keyFor(path, src)
	if !ok {
		return nil, hlPlain, time.Time{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.done[k]; ok {
		h.touch(e)
		if e.spans == nil {
			return nil, hlPlain, time.Time{}
		}
		return e.spans, hlReady, time.Time{}
	}
	h.waits++
	since = h.queued[k]
	if !since.IsZero() {
		return nil, hlWaiting, since // asked for already, and on its way
	}
	// New, or only colored ahead: put it on top, and tell the app a pane
	// waits, so the spinner turns.
	since = hlNow()
	h.queued[k] = since
	h.waiting++
	for i, j := range h.jobs {
		if j.key == k {
			h.jobs = append(h.jobs[:i], h.jobs[i+1:]...)
			break
		}
	}
	h.jobs = append(h.jobs, hlJob{key: k, path: path, src: src})
	h.kick()
	h.signal()
	return nil, hlWaiting, since
}

// ahead queues code to color before any pane asks, under what panes are
// waiting for. The first of jobs is colored first.
func (h *highlighter) ahead(jobs []hlJob) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var add []hlJob
	for i := len(jobs) - 1; i >= 0; i-- {
		j := jobs[i]
		if e, ok := h.done[j.key]; ok {
			h.touch(e)
			continue
		}
		if _, ok := h.queued[j.key]; ok {
			continue
		}
		h.queued[j.key] = time.Time{}
		add = append(add, j)
	}
	if len(add) == 0 {
		return
	}
	h.jobs = append(add, h.jobs...)
	h.kick()
}

// now colors src on the calling goroutine, for work already off the UI,
// and keeps the result. It reports whether it had to lex.
func (h *highlighter) now(path, src string) bool {
	k, ok := keyFor(path, src)
	if !ok {
		return false
	}
	h.mu.Lock()
	e, done := h.done[k]
	if done {
		h.touch(e) // a reloaded diff keeps its colors
	}
	h.mu.Unlock()
	if done {
		return false
	}
	h.store(k, syntax.Lines(path, src))
	return true
}

// store keeps the spans. Only code a pane waited for redraws: code colored
// ahead was never drawn waiting.
func (h *highlighter) store(k hlKey, spans [][]syntax.Span) {
	h.mu.Lock()
	if len(h.done) >= hlMaxEntries {
		h.evict()
	}
	h.clock++
	h.done[k] = &hlEntry{spans: spans, used: h.clock}
	waited := !h.queued[k].IsZero()
	delete(h.queued, k)
	if waited {
		h.waiting--
		h.version++
	}
	h.mu.Unlock()
	if waited {
		h.signal()
	}
}

// touch marks an entry used; h.mu is held.
func (h *highlighter) touch(e *hlEntry) {
	h.clock++
	e.used = h.clock
}

// evict drops the half of the cache used longest ago, so code on screen,
// looked up on every draw, keeps its colors; h.mu is held.
func (h *highlighter) evict() {
	used := make([]uint64, 0, len(h.done))
	for _, e := range h.done {
		used = append(used, e.used)
	}
	slices.Sort(used)
	cut := used[len(used)/2]
	for k, e := range h.done {
		if e.used < cut {
			delete(h.done, k)
		}
	}
}

// kick starts the worker on the jobs; h.mu is held.
func (h *highlighter) kick() {
	h.start.Do(func() { go h.work() })
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *highlighter) signal() {
	select {
	case h.ready <- struct{}{}:
	default:
	}
}

func (h *highlighter) work() {
	for range h.wake {
		for {
			h.mu.Lock()
			if len(h.jobs) == 0 {
				h.mu.Unlock()
				break
			}
			j := h.jobs[len(h.jobs)-1]
			h.jobs = h.jobs[:len(h.jobs)-1]
			_, done := h.done[j.key]
			h.mu.Unlock()
			if !done {
				h.store(j.key, syntax.Lines(j.path, j.src))
			}
		}
	}
}

// ver counts the results panes waited for.
func (h *highlighter) ver() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.version
}

// waitCount is how many lookups have found code on the way, so a render
// can tell it drew a wait.
func (h *highlighter) waitCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.waits
}

// busy reports whether a pane is waiting for code, so the spinner turns.
func (h *highlighter) busy() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.waiting > 0
}

// waitHighlight waits for the highlighter; the app re-arms it on each
// hlMsg.
func waitHighlight() tea.Msg {
	<-hl.ready
	return hlMsg{}
}

// waitLine is what stands for code on the way: blank at first, then a
// spinner.
func waitLine(since time.Time) string {
	now := hlNow()
	if now.Sub(since) < hlGrace {
		return ""
	}
	return spinLine(now, "highlighting", since)
}

// The palette leaves green and red to added and removed lines.
var (
	sCodeKeyword  = lipgloss.NewStyle().Foreground(cSkill)
	sCodeType     = lipgloss.NewStyle().Foreground(cBuiltin)
	sCodeString   = lipgloss.NewStyle().Foreground(cAgent)
	sCodeComment  = lipgloss.NewStyle().Foreground(cDim).Italic(true)
	sCodeFunction = lipgloss.NewStyle().Foreground(cAccent)
	sCodeAttr     = lipgloss.NewStyle().Foreground(cMCP)
)

func codeStyle(c syntax.Class) lipgloss.Style {
	switch c {
	case syntax.Keyword, syntax.Tag:
		return sCodeKeyword
	case syntax.Type, syntax.Constant, syntax.Number:
		return sCodeType
	case syntax.String:
		return sCodeString
	case syntax.Comment:
		return sCodeComment
	case syntax.Function:
		return sCodeFunction
	case syntax.Attr:
		return sCodeAttr
	}
	return sText
}

// renderSpans draws a line of code on bg, with tabs as four spaces.
func renderSpans(spans []syntax.Span, bg lipgloss.TerminalColor) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(codeStyle(s.Class).Background(bg).Render(strings.ReplaceAll(s.Text, "\t", "    ")))
	}
	return b.String()
}

// renderCode draws src line by line, colored by its language, or in
// plain's color when none matches. While the colors are on the way the
// lines stay blank, the first holding the spinner after hlGrace, so the
// pane keeps its shape.
func renderCode(path, src string, plain lipgloss.Style) ([]string, hlState) {
	lines := strings.Split(src, "\n")
	spans, st, since := hl.get(path, src)
	out := make([]string, len(lines))
	for i, l := range lines {
		switch st {
		case hlReady:
			if i < len(spans) {
				out[i] = renderSpans(spans[i], lipgloss.NoColor{})
			}
		case hlWaiting:
			if i == 0 {
				out[i] = waitLine(since)
			}
		default:
			out[i] = plain.Render(l)
		}
	}
	return out, st
}

// editSides is the old and new code of an edit as the detail panes show
// it, at most half of maxDetailLines each, and how many lines each leaves
// out.
func editSides(old, nw string) (o, n []string, oMore, nMore int) {
	cut := func(s string) ([]string, int) {
		lines := parse.Lines(s)
		if more := len(lines) - maxDetailLines/2; more > 0 {
			return lines[:maxDetailLines/2], more
		}
		return lines, 0
	}
	o, oMore = cut(old)
	n, nMore = cut(nw)
	return
}

// writeLines is a written file's content as the detail panes show it, and
// how many lines it leaves out.
func writeLines(src string) ([]string, int) {
	lines := strings.Split(src, "\n")
	if n := len(lines); n > maxDetailLines {
		return lines[:maxDetailLines], n - maxDetailLines
	}
	return lines, 0
}

// callCode is a tool call that shows code, for coloring ahead.
type callCode struct {
	name  string
	input []byte
}

// callsAhead takes the newest edits and writes in events. It only reads
// fields set when a call is parsed, so it can run off the UI.
func callsAhead(events []*parse.Event) []callCode {
	var out []callCode
	for i := len(events) - 1; i >= 0 && len(out) < hlAhead; i-- {
		t := events[i].Tool
		if t == nil {
			continue
		}
		switch t.Name {
		case "Edit", "MultiEdit", "Write":
			out = append(out, callCode{name: t.Name, input: t.Input})
		}
	}
	return out
}

// colorAhead queues the code of the newest edits and writes in events for
// coloring, in the background.
func colorAhead(events []*parse.Event) tea.Cmd {
	if len(events) == 0 {
		return nil
	}
	return func() tea.Msg {
		calls := callsAhead(events)
		var jobs []hlJob
		add := func(path string, lines []string) {
			if len(lines) == 0 {
				return
			}
			src := strings.Join(lines, "\n")
			if k, ok := keyFor(path, src); ok {
				jobs = append(jobs, hlJob{key: k, path: path, src: src})
			}
		}
		for _, c := range calls {
			t := &parse.ToolCall{Name: c.name, Input: c.input}
			path := t.InputString("file_path")
			if c.name == "Write" {
				lines, _ := writeLines(t.InputString("content"))
				add(path, lines)
				continue
			}
			for _, ed := range t.Edits() {
				o, n, _, _ := editSides(ed.Old, ed.New)
				add(path, n)
				add(path, o)
			}
		}
		hl.ahead(jobs)
		return nil
	}
}
