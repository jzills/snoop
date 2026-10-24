package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	json "github.com/goccy/go-json"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
)

// The Files tab lists every file the session (and its subagents) read,
// edited or wrote, with each file's changes in the detail pane.

type fileOp int

const (
	opNone fileOp = iota
	opRead
	opEdit
	opWrite
)

// fileTouch is one tool call on a file.
type fileTouch struct {
	ev             *parse.Event
	op             fileOp
	added, removed int
	agent          string       // subagent description; empty for the session itself
	via            *parse.Event // the Agent call in this session that ran the subagent
}

type fileEntry struct {
	path                       string
	reads, edits, writes, errs int
	added, removed             int
	last                       time.Time
	touches                    []fileTouch // in time order
}

func (f *fileEntry) readOnly() bool { return f.edits+f.writes == 0 }

// fileSource is one transcript's events, with the subagent they came from.
type fileSource struct {
	events []*parse.Event
	agent  string
	via    *parse.Event
}

// fileCall reports which file a tool call touches and how, with the lines
// it added and removed. Calls that touch no file report opNone.
func fileCall(t *parse.ToolCall) (path string, op fileOp, added, removed int) {
	var in map[string]any
	if json.Unmarshal(t.Input, &in) != nil {
		return "", opNone, 0, 0
	}
	get := func(k string) string { s, _ := in[k].(string); return s }
	switch t.Name {
	case "Read":
		return get("file_path"), opRead, 0, 0
	case "Write":
		return get("file_path"), opWrite, lineCount(get("content")), 0
	case "Edit":
		a, r := lineDelta(get("old_string"), get("new_string"))
		return get("file_path"), opEdit, a, r
	case "MultiEdit":
		es, _ := in["edits"].([]any)
		for _, x := range es {
			if m, ok := x.(map[string]any); ok {
				old, _ := m["old_string"].(string)
				nw, _ := m["new_string"].(string)
				a, r := lineDelta(old, nw)
				added, removed = added+a, removed+r
			}
		}
		return get("file_path"), opEdit, added, removed
	case "NotebookEdit":
		return get("notebook_path"), opEdit, lineCount(get("new_source")), 0
	}
	return "", opNone, 0, 0
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1
}

// lineDelta counts the lines an edit adds and removes, ignoring lines it
// leaves unchanged at the start and end.
func lineDelta(old, nw string) (added, removed int) {
	a, b := splitLines(old), splitLines(nw)
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	return len(b), len(a)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// buildFiles groups file tool calls by path (relative to cwd when inside
// it). Changed files come first, then files only read; each by most recent
// activity. keep filters by display path.
func buildFiles(sources []fileSource, cwd string, keep func(string) bool) []*fileEntry {
	byPath := map[string]*fileEntry{}
	var out []*fileEntry
	for _, src := range sources {
		for _, e := range src.events {
			if e.Tool == nil {
				continue
			}
			p, op, added, removed := fileCall(e.Tool)
			if op == opNone || p == "" {
				continue
			}
			if cwd != "" {
				p = strings.TrimPrefix(p, cwd+"/")
			}
			if !keep(p) {
				continue
			}
			f := byPath[p]
			if f == nil {
				f = &fileEntry{path: p}
				byPath[p] = f
				out = append(out, f)
			}
			f.touches = append(f.touches, fileTouch{ev: e, op: op, added: added, removed: removed, agent: src.agent, via: src.via})
			if e.Time.After(f.last) {
				f.last = e.Time
			}
			if e.Tool.IsError {
				f.errs++
				continue
			}
			switch op {
			case opRead:
				f.reads++
			case opEdit:
				f.edits++
			case opWrite:
				f.writes++
			}
			f.added += added
			f.removed += removed
		}
	}
	for _, f := range out {
		sort.SliceStable(f.touches, func(i, j int) bool { return f.touches[i].ev.Time.Before(f.touches[j].ev.Time) })
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].readOnly() != out[j].readOnly() {
			return !out[i].readOnly()
		}
		return out[i].last.After(out[j].last)
	})
	return out
}

// How often the Files tab re-reads subagent transcripts, which grow without
// the session's own transcript changing.
const filesTTL = 2 * time.Second

// fileSources is the session plus, for top-level sessions, every subagent
// transcript on disk.
func (v *view) fileSources() []fileSource {
	srcs := []fileSource{{events: v.sess.Events}}
	if v.label != "" {
		return srcs
	}
	calls := map[string]*parse.Event{}
	for _, e := range v.sess.Events {
		if e.Tool != nil && e.Tool.AgentID != "" {
			calls[e.Tool.AgentID] = e
		}
	}
	for _, id := range store.SubagentIDs(v.rootPath) {
		sub, _ := v.sub(id)
		if sub == nil {
			continue
		}
		meta, _ := v.subagent(id)
		label := meta.Description
		if label == "" {
			label = meta.AgentType
		}
		srcs = append(srcs, fileSource{events: sub.sess.Events, agent: label, via: calls[id]})
	}
	return srcs
}

func (v *view) rebuildFiles() {
	var sel string
	if f := v.selectedFile(); f != nil {
		sel = f.path
	}
	terms := strings.Fields(strings.ToLower(v.search.Value()))
	v.files = buildFiles(v.fileSources(), v.sess.CWD, func(p string) bool {
		lp := strings.ToLower(p)
		for _, t := range terms {
			if !strings.Contains(lp, t) {
				return false
			}
		}
		return true
	})
	v.filesBuilt, v.filesDirty = time.Now(), false
	for i, f := range v.files {
		if f.path == sel {
			v.fCursor = i
			return
		}
	}
	v.fCursor = max(0, min(v.fCursor, len(v.files)-1))
}

func (v *view) selectedFile() *fileEntry {
	if v.fCursor < len(v.files) {
		return v.files[v.fCursor]
	}
	return nil
}

func (v *view) moveFile(d int) {
	v.fCursor = max(0, min(v.fCursor+d, len(v.files)-1))
}

func (v *view) updateFiles(k tea.KeyMsg, w, bodyH int) (tea.Cmd, *view) {
	switch k.String() {
	case "up", "k":
		v.moveFile(-1)
	case "down", "j":
		v.moveFile(1)
	case "pgup", "ctrl+b":
		v.moveFile(-bodyH)
	case "pgdown", "ctrl+f", " ":
		v.moveFile(bodyH)
	case "g", "home":
		v.moveFile(-len(v.files))
	case "G", "end":
		v.moveFile(len(v.files))
	case "ctrl+d", "J":
		v.detail.HalfViewDown()
	case "ctrl+u", "K":
		v.detail.HalfViewUp()
	case "n":
		v.jumpMark(1)
	case "N":
		v.jumpMark(-1)
	case "enter":
		if f := v.selectedFile(); f != nil {
			v.showTouch(lastChange(f))
		}
	case "tab", "right", "l":
		v.focusDetail(w)
	case "/":
		v.typing = true
		return v.search.Focus(), nil
	}
	return nil, nil
}

// lastChange is the file's latest edit or write, else its latest read.
func lastChange(f *fileEntry) fileTouch {
	for i := len(f.touches) - 1; i >= 0; i-- {
		if f.touches[i].op != opRead {
			return f.touches[i]
		}
	}
	return f.touches[len(f.touches)-1]
}

// showTouch shows a file touch in the timeline: the call itself, or for a
// subagent's call the Agent call that ran it.
func (v *view) showTouch(t fileTouch) {
	switch {
	case t.agent == "":
		v.jumpTo(t.ev)
	case t.via != nil:
		v.jumpTo(t.via)
	}
}

// jumpMark scrolls the detail pane to the next or previous change.
func (v *view) jumpMark(dir int) {
	y := v.detail.YOffset
	if dir > 0 {
		for _, m := range v.fMarks {
			if m > y {
				v.detail.SetYOffset(m)
				return
			}
		}
		return
	}
	for i := len(v.fMarks) - 1; i >= 0; i-- {
		if v.fMarks[i] < y {
			v.detail.SetYOffset(v.fMarks[i])
			return
		}
	}
}

const bashGap = " changes made through Bash aren't tracked"

func (v *view) filesView(w, h int) string {
	if v.zoom && w < wideLayout {
		return v.detailPane(w, h)
	}
	listW := w
	if w >= wideLayout {
		listW = w * 11 / 20
	}
	listH := max(h-1, 1)
	v.fOffset = scrollOffset(v.fOffset, v.fCursor, len(v.files), listH)
	lines := make([]string, 0, h)
	for i := v.fOffset; i < len(v.files) && len(lines) < listH; i++ {
		lines = append(lines, fileRow(v.files[i], i == v.fCursor, listW))
	}
	if len(v.files) == 0 {
		msg := " no files touched yet"
		if v.search.Value() != "" {
			msg = " no files match"
		}
		lines = append(lines, sDim.Render(msg))
	}
	for len(lines) < listH {
		lines = append(lines, strings.Repeat(" ", listW))
	}
	lines = append(lines, sFaint.Render(fit(bashGap, listW)))
	list := strings.Join(lines, "\n")
	if w < wideLayout {
		return list
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, list, v.detailPane(w-listW, h))
}

func fileRow(f *fileEntry, sel bool, w int) string {
	var bg lipgloss.TerminalColor = lipgloss.NoColor{}
	if sel {
		bg = cSelBg
	}
	st := func(s lipgloss.Style) lipgloss.Style { return s.Background(bg) }
	mark := st(sText).Render(" ")
	if sel {
		mark = st(sAccent).Render("▌")
	}
	cnt := func(n int, glyph string) string {
		if n == 0 {
			return st(sFaint).Render(fmt.Sprintf(" %4s", "·"))
		}
		return st(sText).Render(fmt.Sprintf(" %3d%s", n, glyph))
	}
	right := cnt(f.reads, "r") + cnt(f.edits, "e") + cnt(f.writes, "w") +
		st(sAdd).Render(fmt.Sprintf(" %6s", plusMinus("+", f.added))) +
		st(sDel).Render(fmt.Sprintf(" %6s", plusMinus("−", f.removed)))
	if f.errs > 0 {
		right += st(sErr).Render(fmt.Sprintf(" %2d%s ", f.errs, glyphErr))
	} else {
		right += st(sText).Render("     ")
	}
	nameStyle := sText
	if f.readOnly() {
		nameStyle = sDim
	}
	nameW := w - 1 - lipgloss.Width(right)
	line := mark + st(nameStyle).Render(fit(fitLeft(f.path, nameW), nameW)) + right
	if pad := w - lipgloss.Width(line); pad > 0 {
		line += st(sText).Render(strings.Repeat(" ", pad))
	}
	return fit(line, w)
}

// fitLeft shortens s to w cells by dropping its start, so a path keeps its
// file name.
func fitLeft(s string, w int) string {
	r := []rune(s)
	if len(r) <= w || w < 2 {
		return s
	}
	return "…" + string(r[len(r)-w+1:])
}

func plusMinus(sign string, n int) string {
	if n == 0 {
		return ""
	}
	return sign + fmt.Sprint(n)
}

// renderFileDetail lists every touch of a file, with diffs for changes. It
// also returns the line each change starts on, for n/N.
func renderFileDetail(f *fileEntry, w int) (string, []int) {
	var b strings.Builder
	var marks []int
	b.WriteString(sBold.Render(wrap(f.path, w)) + "\n")
	var parts []string
	for _, p := range []struct {
		n    int
		word string
	}{{f.edits, "edit"}, {f.writes, "write"}, {f.reads, "read"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, plural(p.n, p.word)))
		}
	}
	summary := sDim.Render(strings.Join(parts, " · "))
	if f.added+f.removed > 0 {
		summary += sDim.Render(" · ") + sAdd.Render(plusMinus("+", f.added)) + " " + sDel.Render(plusMinus("−", f.removed))
	}
	if f.errs > 0 {
		summary += sDim.Render(" · ") + sErr.Render(fmt.Sprintf("%d %s", f.errs, plural(f.errs, "error")))
	}
	b.WriteString(summary + "\n")
	b.WriteString(section("History") + "\n")
	for _, t := range f.touches {
		e := t.ev
		status := sOK.Render(glyphOK)
		switch {
		case e.Tool.IsError:
			status = sErr.Render(glyphErr)
		case !e.Tool.HasResult:
			status = sDim.Render(glyphPending)
		}
		ts := ""
		if !e.Time.IsZero() {
			ts = e.Time.Local().Format("15:04:05")
		}
		who := ""
		if t.agent != "" {
			who = lipgloss.NewStyle().Foreground(cAgent).Render("  ⤷ " + t.agent)
		}
		if t.op != opRead {
			marks = append(marks, strings.Count(b.String(), "\n"))
			b.WriteString("\n")
		}
		b.WriteString(status + " " + sFaint.Render(ts) + " " + sText.Render(e.Tool.Name) + fit(who, max(w-20, 0)) + "\n")
		if t.op == opRead {
			continue
		}
		var in map[string]any
		json.Unmarshal(e.Tool.Input, &in)
		get := func(k string) string { s, _ := in[k].(string); return s }
		switch e.Tool.Name {
		case "Edit":
			b.WriteString(diff(get("old_string"), get("new_string"), w) + "\n")
		case "MultiEdit":
			es, _ := in["edits"].([]any)
			for _, x := range es {
				if m, ok := x.(map[string]any); ok {
					old, _ := m["old_string"].(string)
					nw, _ := m["new_string"].(string)
					b.WriteString(diff(old, nw, w) + "\n")
				}
			}
		case "Write":
			b.WriteString(sAdd.Render(wrap(capLines(get("content"), maxDetailLines), w)) + "\n")
		case "NotebookEdit":
			b.WriteString(sAdd.Render(wrap(capLines(get("new_source"), maxDetailLines), w)) + "\n")
		}
		if e.Tool.IsError {
			b.WriteString(sErr.Render(wrap(capLines(e.Tool.Result, 20), w)) + "\n")
		}
	}
	b.WriteString("\n" + sKey.Render("n N") + sDim.Render(" next / previous change · ") + sKey.Render("enter") + sDim.Render(" show in timeline"))
	return b.String(), marks
}
