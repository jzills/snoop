package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/review"
	"github.com/jzills/snoop/internal/store"
)

// The Diff tab shows the git diff of the session's working tree, takes
// comments on lines, ranges and files, and sends them to the agent through
// the inbox the hooks deliver from.

// reviewState is the Diff tab's state.
type reviewState struct {
	diff    *review.Diff
	err     error
	loading bool
	polling bool
	seq     int       // only the newest load is applied
	at      time.Time // when the last load started
	sig     int       // changeCount when it started
	ours    map[string]bool
	subs    map[string]subChanges // subagent transcripts, by path

	cursor, offset int // file list
	line, lOffset  int // row in the selected file's diff
	anchor         int // first row of a range selection, or -1

	comments []pending
	nextID   int
	input    textinput.Model
	typing   bool
	editing  int // id of the comment being edited, or 0 for a new one
	target   review.Comment

	sending   bool
	sentPath  string // the last send, until it is delivered
	sentN     int
	delivered bool
	leaveAsk  bool // leaving with unsent comments asks first
	leaveQuit bool // and the leaving is quitting

	// The branch diff's base, once guessed or chosen, and the B list.
	base    string
	picking bool
	bases   []review.Base
	bCursor int
}

// pending is a comment not sent yet. Its id survives edits and deletes,
// so a send removes exactly the comments it sent.
type pending struct {
	review.Comment
	id int
}

// subChanges is the files a subagent changed, as of a transcript size.
type subChanges struct {
	size  int64
	paths []string
}

func newReviewState() reviewState {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = sKey
	ti.CharLimit = 4000
	return reviewState{anchor: -1, input: ti}
}

type diffMsg struct {
	v    *view
	seq  int
	base string // the branch diff's base, when there is one
	d    *review.Diff
	err  error
	ours map[string]bool
	subs map[string]subChanges
}

type sentMsg struct {
	v         *view
	ids       []int
	text      string
	path      string
	installed bool
	err       error
}

// reviewPollMsg reports what changed on disk since the last look: a
// subagent transcript, or the last send being delivered.
type reviewPollMsg struct {
	v          *view
	subsMoved  bool
	delivered  bool
	sentPath   string
	pollForTab bool
}

// diffRefresh is the least time between automatic reloads.
const diffRefresh = 2 * time.Second

// changeCount counts calls that can change files, so the tab reloads only
// when one lands. Agent calls count when they start, so a new subagent's
// transcript is picked up.
func (v *view) changeCount() int {
	n := 0
	for _, e := range v.sess.Events {
		t := e.Tool
		switch {
		case t == nil:
		case t.Kind == parse.ToolAgent || t.AgentID != "":
			n++
		case t.HasResult:
			switch t.Name {
			case "Edit", "MultiEdit", "Write", "NotebookEdit", "Bash":
				n++
			}
		}
	}
	return n
}

// changedPaths are the absolute paths a list of events edited or wrote.
func changedPaths(events []*parse.Event) []string {
	var out []string
	for _, e := range events {
		if e.Tool == nil {
			continue
		}
		if p, op, _, _ := fileCall(e.Tool); p != "" && (op == opEdit || op == opWrite) {
			out = append(out, p)
		}
	}
	return out
}

// loadDiff runs git, and reads any subagent transcripts that changed, in
// the background.
func (v *view) loadDiff() tea.Cmd {
	r := &v.rev
	if v.sess.CWD == "" {
		r.err = fmt.Errorf("the session has no working directory")
		return nil
	}
	r.seq++
	r.loading, r.at, r.sig = true, time.Now(), v.changeCount()
	seq, cwd, root, base, scope := r.seq, v.sess.CWD, v.rootPath, r.base, v.prefs.diffScope
	own := changedPaths(v.sess.Events)
	cache := r.subs
	return func() tea.Msg {
		d, err := loadScoped(cwd, base, scope)
		subs := readSubs(root, cache)
		if err != nil {
			return diffMsg{v: v, seq: seq, err: err, subs: subs}
		}
		for _, c := range subs {
			own = append(own, c.paths...)
		}
		return diffMsg{v: v, seq: seq, base: d.Base, d: d, ours: ownPaths(d.Root, cwd, own), subs: subs}
	}
}

// readSubs finds the files each subagent changed, reading only the
// transcripts that grew since cache was made.
func readSubs(sessionPath string, cache map[string]subChanges) map[string]subChanges {
	out := map[string]subChanges{}
	for _, id := range store.SubagentIDs(sessionPath) {
		p := store.SubagentPath(sessionPath, id)
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if c, ok := cache[p]; ok && c.size == info.Size() {
			out[p] = c
			continue
		}
		s, _, err := store.Load(p)
		if err != nil {
			continue
		}
		out[p] = subChanges{size: info.Size(), paths: changedPaths(s.Events)}
	}
	return out
}

// subsMoved reports whether any subagent transcript appeared or changed
// size since cache was made.
func subsMoved(sessionPath string, cache map[string]subChanges) bool {
	ids := store.SubagentIDs(sessionPath)
	if len(ids) != len(cache) {
		return true
	}
	for _, id := range ids {
		p := store.SubagentPath(sessionPath, id)
		info, err := os.Stat(p)
		if c, ok := cache[p]; err != nil || !ok || c.size != info.Size() {
			return true
		}
	}
	return false
}

// ownPaths maps the absolute paths the session changed to paths relative
// to the repository root. The transcript records paths under the session's
// directory as Claude Code saw it, which may go through a symlink that git
// has resolved, so that form of the root is tried too.
func ownPaths(root, cwd string, paths []string) map[string]bool {
	roots := []string{root}
	if real, err := filepath.EvalSymlinks(cwd); err == nil {
		if sub, err := filepath.Rel(root, real); err == nil && !strings.HasPrefix(sub, "..") {
			alias := cwd
			if sub != "." {
				alias = strings.TrimSuffix(cwd, string(filepath.Separator)+sub)
			}
			if alias != cwd || sub == "." {
				roots = append(roots, alias)
			}
		}
	}
	ours := map[string]bool{}
	for _, p := range paths {
		for _, r := range roots {
			if rel, err := filepath.Rel(r, p); err == nil && !strings.HasPrefix(rel, "..") {
				ours[filepath.ToSlash(rel)] = true
				break
			}
		}
	}
	return ours
}

// pollReview looks at the disk in the background: on the Diff tab, for
// subagent transcripts that changed; after a send, for its delivery.
func (v *view) pollReview(now time.Time) tea.Cmd {
	r := &v.rev
	forTab := v.tab == tabDiff && !r.loading && now.Sub(r.at) >= diffRefresh
	waiting := r.sentPath != "" && !r.delivered
	if r.polling || (!forTab && !waiting) {
		return nil
	}
	if forTab && v.changeCount() != r.sig {
		return v.loadDiff()
	}
	r.polling = true
	root, cache, sent := v.rootPath, r.subs, r.sentPath
	return func() tea.Msg {
		m := reviewPollMsg{v: v, sentPath: sent, pollForTab: forTab}
		if forTab {
			m.subsMoved = subsMoved(root, cache)
		}
		if sent != "" {
			m.delivered = !review.Waiting(sent)
		}
		return m
	}
}

func (v *view) onPoll(m reviewPollMsg) tea.Cmd {
	r := &v.rev
	r.polling = false
	if m.sentPath != "" && m.sentPath == r.sentPath && m.delivered {
		r.delivered = true
	}
	if m.pollForTab && m.subsMoved && v.tab == tabDiff && !r.loading {
		return v.loadDiff()
	}
	return nil
}

func (v *view) onDiff(m diffMsg) {
	r := &v.rev
	if m.seq != r.seq {
		return
	}
	r.loading = false
	r.subs = m.subs
	oldFile := v.diffFile()
	var sel string
	if oldFile != nil {
		sel = oldFile.Path
	}
	oldRows := diffRows(oldFile)
	r.err = m.err
	if m.err != nil {
		r.diff, v.focusR = nil, false
		r.cursor, r.line, r.lOffset, r.anchor = 0, 0, 0, -1
		return
	}
	r.diff, r.ours = m.d, m.ours
	if m.base != "" {
		r.base = m.base
	}
	// The session's own files first, each group in git's order.
	sort.SliceStable(m.d.Files, func(i, j int) bool {
		return r.ours[m.d.Files[i].Path] && !r.ours[m.d.Files[j].Path]
	})
	r.cursor = 0
	same := false
	for i, f := range m.d.Files {
		if f.Path == sel {
			r.cursor, same = i, true
		}
	}
	rows := diffRows(v.diffFile())
	switch {
	case !same:
		// A different file: start at its top.
		r.line, r.lOffset, r.anchor = v.clampLine(0), 0, -1
	case !sameRows(oldRows, rows):
		// The file changed under the cursor; a range would now cover other
		// lines.
		r.line, r.anchor = v.clampLine(r.line), -1
	}
	if len(rows) == 0 {
		v.focusR = false
	}
}

func sameRows(a, b []dRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].header != b[i].header || (a[i].line == nil) != (b[i].line == nil) || (a[i].line != nil && *a[i].line != *b[i].line) {
			return false
		}
	}
	return true
}

func (v *view) diffFiles() []*review.File {
	if v.rev.diff == nil {
		return nil
	}
	return v.rev.diff.Files
}

func (v *view) diffFile() *review.File {
	fs := v.diffFiles()
	if v.rev.cursor < len(fs) {
		return fs[v.rev.cursor]
	}
	return nil
}

// dRow is a row of a file's diff: a hunk header or a line.
type dRow struct {
	header string
	line   *review.Line
}

func diffRows(f *review.File) []dRow {
	if f == nil {
		return nil
	}
	var rows []dRow
	for i := range f.Hunks {
		h := &f.Hunks[i]
		rows = append(rows, dRow{header: h.Header})
		for j := range h.Lines {
			rows = append(rows, dRow{line: &h.Lines[j]})
		}
	}
	return rows
}

// clampLine keeps a row index on a line of the selected file, never a
// header.
func (v *view) clampLine(i int) int {
	rows := diffRows(v.diffFile())
	if len(rows) == 0 {
		return 0
	}
	i = max(0, min(i, len(rows)-1))
	for j := i; j < len(rows); j++ {
		if rows[j].line != nil {
			return j
		}
	}
	for j := i; j >= 0; j-- {
		if rows[j].line != nil {
			return j
		}
	}
	return 0
}

func (v *view) moveDiffFile(d int) {
	r := &v.rev
	n := len(v.diffFiles())
	if n == 0 {
		return
	}
	c := max(0, min(r.cursor+d, n-1))
	if c != r.cursor {
		r.cursor, r.line, r.lOffset, r.anchor = c, v.clampLineFor(c), 0, -1
	}
}

func (v *view) clampLineFor(file int) int {
	old := v.rev.cursor
	v.rev.cursor = file
	l := v.clampLine(0)
	v.rev.cursor = old
	return l
}

// moveLine moves the cursor by d lines, skipping hunk headers.
func (v *view) moveLine(d int) {
	rows := diffRows(v.diffFile())
	r := &v.rev
	i := r.line
	step := 1
	if d < 0 {
		step, d = -1, -d
	}
	for d > 0 {
		j := i + step
		for j >= 0 && j < len(rows) && rows[j].line == nil {
			j += step
		}
		if j < 0 || j >= len(rows) {
			break
		}
		i = j
		d--
	}
	r.line = i
}

// jumpHunk moves to the first line of the next or previous hunk.
func (v *view) jumpHunk(dir int) {
	rows := diffRows(v.diffFile())
	r := &v.rev
	start := r.line
	// Back up to this hunk's header first when going backwards.
	if dir < 0 {
		for start > 0 && rows[start].line != nil {
			start--
		}
	}
	for j := start + dir; j >= 0 && j < len(rows); j += dir {
		if rows[j].line == nil && j+1 < len(rows) {
			r.line = j + 1
			return
		}
	}
}

// selectedLines is the range selection, or the line under the cursor.
func (v *view) selectedLines() []review.Line {
	rows := diffRows(v.diffFile())
	r := &v.rev
	if len(rows) == 0 {
		return nil
	}
	lo, hi := r.line, r.line
	if r.anchor >= 0 {
		lo, hi = min(r.anchor, r.line), max(r.anchor, r.line)
	}
	var out []review.Line
	for i := lo; i <= hi && i < len(rows); i++ {
		if rows[i].line != nil {
			out = append(out, *rows[i].line)
		}
	}
	return out
}

// commentAt is the comment on line l of the selected file, or nil.
func (v *view) commentAt(l review.Line) *pending {
	f := v.diffFile()
	if f == nil {
		return nil
	}
	for i := range v.rev.comments {
		if v.rev.comments[i].Covers(f.Path, l) {
			return &v.rev.comments[i]
		}
	}
	return nil
}

func (v *view) fileComment(path string) *pending {
	for i, c := range v.rev.comments {
		if c.Path == path && len(c.Lines) == 0 {
			return &v.rev.comments[i]
		}
	}
	return nil
}

func (v *view) removeComments(ids ...int) {
	r := &v.rev
	out := r.comments[:0]
	for _, c := range r.comments {
		if !containsInt(ids, c.id) {
			out = append(out, c)
		}
	}
	r.comments = out
}

func containsInt(xs []int, x int) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// startComment opens the input for a new comment, or for editing c.
func (v *view) startComment(target review.Comment, c *pending) tea.Cmd {
	r := &v.rev
	r.target, r.editing, r.typing = target, 0, true
	r.input.SetValue("")
	r.input.Placeholder = "comment on " + commentWhere(target)
	if c != nil {
		r.editing = c.id
		r.input.SetValue(c.Text)
		r.input.CursorEnd()
	}
	return r.input.Focus()
}

func commentWhere(c review.Comment) string {
	if len(c.Lines) == 0 {
		return "the whole file"
	}
	if len(c.Lines) == 1 {
		return "this line"
	}
	return fmt.Sprintf("these %d lines", len(c.Lines))
}

// typeComment handles keys while a comment is being written.
func (v *view) typeComment(k tea.KeyMsg) tea.Cmd {
	r := &v.rev
	switch k.String() {
	case "esc":
		r.typing = false
		r.input.Blur()
		return nil
	case "enter":
		r.typing = false
		r.input.Blur()
		text := strings.TrimSpace(r.input.Value())
		switch {
		case r.editing != 0 && text == "":
			v.removeComments(r.editing)
		case r.editing != 0:
			// The comment may have been sent meanwhile; then this is new.
			found := false
			for i := range r.comments {
				if r.comments[i].id == r.editing {
					r.comments[i].Text, found = text, true
				}
			}
			if !found {
				v.addComment(r.target, text)
			}
		case text != "":
			v.addComment(r.target, text)
		}
		r.anchor = -1
		return nil
	}
	var cmd tea.Cmd
	r.input, cmd = r.input.Update(k)
	return cmd
}

func (v *view) addComment(c review.Comment, text string) {
	r := &v.rev
	r.nextID++
	c.Text = text
	r.comments = append(r.comments, pending{Comment: c, id: r.nextID})
}

// updateDiff handles a key on the Diff tab; ok is false for keys it leaves
// to the session view.
func (v *view) updateDiff(k tea.KeyMsg, w, bodyH int) (cmd tea.Cmd, ok bool) {
	r := &v.rev
	if r.picking {
		return v.pickKey(k), true
	}
	f := v.diffFile()
	switch k.String() {
	case "r":
		return v.loadDiff(), true
	case "b":
		return v.switchScope(), true
	case "B":
		return v.startPick(), true
	case "S":
		return v.sendComments(), true
	case "C":
		if f != nil {
			c := v.fileComment(f.Path)
			return v.startComment(review.Comment{Path: f.Path}, c), true
		}
		return nil, true
	}
	if v.focusR && len(diffRows(f)) == 0 {
		// The file lost its lines in a refresh.
		v.focusR = false
	}
	if !v.focusR {
		r.anchor = -1
		switch k.String() {
		case "up", "k":
			v.moveDiffFile(-1)
		case "down", "j":
			v.moveDiffFile(1)
		case "pgup", "ctrl+b":
			v.moveDiffFile(-bodyH)
		case "pgdown", "ctrl+f", " ":
			v.moveDiffFile(bodyH)
		case "g", "home":
			v.moveDiffFile(-len(v.diffFiles()))
		case "G", "end":
			v.moveDiffFile(len(v.diffFiles()))
		case "enter", "tab", "l", "right":
			if len(diffRows(f)) > 0 {
				v.focusR = true
				r.line = v.clampLine(r.line)
			}
		case "e", "x":
			// They edit and delete comments here, so they mustn't export
			// the session or toggle a timeline row from the file list.
		default:
			return nil, false
		}
		return nil, true
	}
	switch k.String() {
	case "up", "k":
		v.moveLine(-1)
	case "down", "j":
		v.moveLine(1)
	case "pgup", "ctrl+b", "ctrl+u":
		v.moveLine(-bodyH / 2)
	case "pgdown", "ctrl+f", "ctrl+d", " ":
		v.moveLine(bodyH / 2)
	case "g", "home":
		r.line = v.clampLine(0)
	case "G", "end":
		r.line = v.clampLine(len(diffRows(f)) - 1)
	case "J", "n":
		v.jumpHunk(1)
	case "K", "N":
		v.jumpHunk(-1)
	case "v":
		if r.anchor >= 0 {
			r.anchor = -1
		} else {
			r.anchor = r.line
		}
	case "c":
		if f == nil {
			return nil, true
		}
		lines := v.selectedLines()
		if len(lines) == 0 {
			return nil, true
		}
		return v.startComment(review.Comment{Path: f.Path, Lines: lines}, nil), true
	case "e":
		if l := v.cursorLine(); l != nil {
			if c := v.commentAt(*l); c != nil {
				return v.startComment(c.Comment, c), true
			}
		}
	case "x":
		if l := v.cursorLine(); l != nil {
			if c := v.commentAt(*l); c != nil {
				v.removeComments(c.id)
			}
		}
	case "tab", "h", "left":
		v.focusR = false
		r.anchor = -1
	default:
		return nil, false
	}
	return nil, true
}

func (v *view) cursorLine() *review.Line {
	rows := diffRows(v.diffFile())
	if v.rev.line < len(rows) {
		return rows[v.rev.line].line
	}
	return nil
}

// sessionID names the session in the inbox: its transcript's file name,
// which subagents' views share through rootPath.
func (v *view) sessionID() string {
	return strings.TrimSuffix(filepath.Base(v.rootPath), ".jsonl")
}

// settingsFiles are the settings that may hold the hooks: the user's,
// next to the transcripts, and the session's project's.
func (v *view) settingsFiles() []string {
	fs := []string{review.UserSettings(filepath.Dir(filepath.Dir(v.rootPath)))}
	if v.sess.CWD != "" {
		fs = append(fs, filepath.Join(v.sess.CWD, ".claude", "settings.json"), filepath.Join(v.sess.CWD, ".claude", "settings.local.json"))
	}
	return fs
}

// sendComments queues the pending comments for the agent, or copies them
// when no hooks are installed to deliver them.
func (v *view) sendComments() tea.Cmd {
	r := &v.rev
	if r.sending {
		return nil
	}
	if len(r.comments) == 0 {
		v.flash = sDim.Render(" no comments to send; c comments on a line, C on the file")
		return nil
	}
	cs := make([]review.Comment, len(r.comments))
	ids := make([]int, len(r.comments))
	for i, c := range r.comments {
		cs[i], ids[i] = c.Comment, c.id
	}
	r.sending = true
	text := review.Format(cs)
	settings, id, dir := v.settingsFiles(), v.sessionID(), review.InboxDir()
	return func() tea.Msg {
		m := sentMsg{v: v, ids: ids, text: text, installed: review.Installed(settings...)}
		if m.installed {
			m.path, m.err = review.Send(dir, id, text, time.Now())
		}
		return m
	}
}

func (v *view) onSent(m sentMsg) tea.Cmd {
	r := &v.rev
	r.sending = false
	n := len(m.ids)
	switch {
	case m.err != nil:
		v.flash = sErr.Render(" " + glyphErr + " couldn't send: " + m.err.Error())
		return nil
	case !m.installed:
		v.flash = sKey.Render(fmt.Sprintf(" Copied %d %s to the clipboard.", n, plural(n, "comment"))) +
			sDim.Render(" To send them straight to the agent, run snoop hooks install, then press S again.")
		return copyCmd(m.text)
	}
	v.removeComments(m.ids...)
	r.sentPath, r.sentN, r.delivered = m.path, n, false
	v.flash = sOK.Render(fmt.Sprintf(" %s sent %d %s", glyphOK, n, plural(n, "comment")))
	return nil
}

// sendStatus describes the last send in the footer.
func (v *view) sendStatus() string {
	r := &v.rev
	if r.sentPath == "" {
		return ""
	}
	n := fmt.Sprintf("%d %s", r.sentN, plural(r.sentN, "comment"))
	switch {
	case r.delivered:
		return sOK.Render(glyphOK+" delivered "+n) + "  "
	case time.Since(v.sess.End) > 2*time.Minute:
		return sDim.Render("sent "+n+", waiting for the session to resume") + "  "
	}
	return sDim.Render("sent "+n+", waiting for the agent") + "  "
}

func (v *view) diffView(w, h int) (out string) {
	r := &v.rev
	files := v.diffFiles()
	var msg string
	switch {
	case r.err != nil:
		msg = " " + r.err.Error()
	case r.diff == nil:
		msg = " reading the diff …"
	case len(files) == 0 && v.branchShown():
		msg = " no changes against " + r.diff.Base + " in " + r.diff.Root
	case len(files) == 0:
		msg = " no changes against HEAD in " + r.diff.Root
	}
	if r.picking {
		return v.pickView(w, h)
	}
	if r.diff != nil {
		// The scopes, then a blank line, above the files.
		top := v.scopeLine(w) + "\n" + strings.Repeat(" ", w) + "\n"
		h = max(h-2, 1)
		defer func() { out = top + out }()
	}
	if msg != "" {
		lines := []string{sDim.Render(fit(msg, w))}
		for len(lines) < h {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n")
	}
	if w < wideLayout && v.focusR {
		return v.diffPane(w, h, true)
	}
	listW := w
	if w >= wideLayout {
		listW = w * 2 / 5
	}
	r.offset = scrollOffset(r.offset, r.cursor, len(files), h)
	lines := make([]string, 0, h)
	for i := r.offset; i < len(files) && len(lines) < h; i++ {
		lines = append(lines, v.diffFileRow(files[i], i == r.cursor, listW))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", listW))
	}
	list := strings.Join(lines, "\n")
	if w < wideLayout {
		return list
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, list, v.diffPane(w-listW, h, false))
}

func (v *view) diffFileRow(f *review.File, sel bool, w int) string {
	var bg lipgloss.TerminalColor = lipgloss.NoColor{}
	if sel {
		bg = cSelBg
	}
	st := func(s lipgloss.Style) lipgloss.Style { return s.Background(bg) }
	mark := st(sText).Render(" ")
	if sel {
		mark = st(sAccent).Render("▌")
	}
	n := 0
	for _, c := range v.rev.comments {
		if c.Path == f.Path {
			n++
		}
	}
	note := st(sText).Render("   ")
	if n > 0 {
		note = st(sKey).Render(fmt.Sprintf(" ●%d", min(n, 9)))
	}
	right := st(sAdd).Render(fmt.Sprintf(" %6s", plusMinus("+", f.Added))) +
		st(sDel).Render(fmt.Sprintf(" %6s", plusMinus("−", f.Removed))) + note + st(sText).Render(" ")
	nameStyle := sText
	if !v.rev.ours[f.Path] {
		nameStyle = sDim
	}
	tag := ""
	switch {
	case f.Untracked:
		tag = " new"
	case f.Deleted:
		tag = " deleted"
	}
	if f.Uncommitted && !f.Untracked {
		tag += " uncommitted"
	}
	nameW := w - 1 - lipgloss.Width(right) - len(tag)
	line := mark + st(nameStyle).Render(fit(fitLeft(f.Path, nameW), nameW)) + st(sFaint).Render(tag) + right
	if pad := w - lipgloss.Width(line); pad > 0 {
		line += st(sText).Render(strings.Repeat(" ", pad))
	}
	return fit(line, w)
}

// diffPane draws the selected file's diff; full means it fills a narrow
// terminal, without the border.
func (v *view) diffPane(w, h int, full bool) string {
	r := &v.rev
	border := lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).
		BorderForeground(cFaint).PaddingLeft(1)
	innerW := max(w-2, 10)
	if v.focusR {
		border = border.BorderForeground(cAccent)
	}
	if full {
		border, innerW = lipgloss.NewStyle().PaddingLeft(1), max(w-1, 10)
	}
	f := v.diffFile()
	var out []string
	head := sBold.Render(fitLeft(f.Path, innerW))
	var flags []string
	if !r.ours[f.Path] {
		flags = append(flags, "not changed by this session")
	}
	if f.Untracked {
		flags = append(flags, "untracked")
	}
	if f.Deleted {
		flags = append(flags, "deleted")
	}
	if f.Truncated {
		flags = append(flags, fmt.Sprintf("showing the first %d lines", review.MaxLines))
	}
	if c := v.fileComment(f.Path); c != nil {
		flags = append(flags, "● "+firstLine(c.Text))
	}
	out = append(out, head)
	out = append(out, sDim.Render(fit(strings.Join(flags, " · "), innerW)))
	bodyH := max(h-2, 1)
	rows := diffRows(f)
	switch {
	case f.Binary:
		out = append(out, sDim.Render("binary file"))
	case len(rows) == 0:
		out = append(out, sDim.Render("no lines to show"))
	}
	numW := 1
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			numW = max(numW, len(fmt.Sprint(max(l.Old, l.New))))
		}
	}
	// Each line comment's text shows under the last line it covers,
	// wrapped and indented to the code.
	indent := 2*numW + 4
	notes := make([][]string, len(rows))
	for i, row := range rows {
		if row.line != nil {
			if c := v.commentAt(*row.line); c != nil && c.Lines[len(c.Lines)-1] == *row.line {
				notes[i] = strings.Split(wrap("● "+c.Text, max(innerW-indent, 10)), "\n")
			}
		}
	}
	r.lOffset = scrollOffset(r.lOffset, r.line, len(rows), bodyH)
	// Notes take lines too: scroll on until the cursor's row and its note
	// fit.
	for r.lOffset < r.line && r.line < len(rows) {
		n := 0
		for i := r.lOffset; i <= r.line; i++ {
			n += 1 + len(notes[i])
		}
		if n <= bodyH {
			break
		}
		r.lOffset++
	}
	lo, hi := -1, -1
	if r.anchor >= 0 && v.focusR {
		lo, hi = min(r.anchor, r.line), max(r.anchor, r.line)
	}
	for i := r.lOffset; i < len(rows) && len(out) < h; i++ {
		out = append(out, v.diffRow(rows[i], innerW, numW, v.focusR && i == r.line, i >= lo && i <= hi))
		for _, n := range notes[i] {
			if len(out) < h {
				out = append(out, sKey.Render(fit(strings.Repeat(" ", indent)+n, innerW)))
			}
		}
	}
	for len(out) < h {
		out = append(out, "")
	}
	return border.Height(h).Render(strings.Join(out, "\n"))
}

func (v *view) diffRow(row dRow, w, numW int, cur, inRange bool) string {
	if row.line == nil {
		return sFaint.Render(fit(row.header, w))
	}
	l := row.line
	var bg lipgloss.TerminalColor = lipgloss.NoColor{}
	switch {
	case cur:
		bg = cSelBg
	case inRange:
		bg = cPromptBg
	}
	st := func(s lipgloss.Style) lipgloss.Style { return s.Background(bg) }
	gutter := st(sText).Render(" ")
	if v.commentAt(*l) != nil {
		gutter = st(sKey).Render("●")
	}
	num := func(n int) string {
		if n == 0 {
			return strings.Repeat(" ", numW)
		}
		return fmt.Sprintf("%*d", numW, n)
	}
	textStyle := sText
	switch l.Kind {
	case '+':
		textStyle = sAdd
	case '-':
		textStyle = sDel
	}
	prefix := gutter + st(sFaint).Render(num(l.Old)+" "+num(l.New)+" ") + st(textStyle).Render(string(l.Kind)+" ")
	body := strings.ReplaceAll(l.Text, "\t", "    ")
	line := prefix + st(textStyle).Render(fit(body, max(w-lipgloss.Width(prefix), 1)))
	if pad := w - lipgloss.Width(line); pad > 0 {
		line += st(sText).Render(strings.Repeat(" ", pad))
	}
	return fit(line, w)
}

// diffText is what y copies on the Diff tab: the selected file's diff.
func (v *view) diffText() string {
	f := v.diffFile()
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- " + f.Path + "\n")
	for _, h := range f.Hunks {
		b.WriteString(h.Header + "\n")
		for _, l := range h.Lines {
			b.WriteString(string(l.Kind) + l.Text + "\n")
		}
	}
	return b.String()
}

// diffTabsRight is the Diff tab's part of the tab bar.
func (v *view) diffTabsRight() string {
	var flags []string
	if n := len(v.rev.comments); n > 0 {
		flags = append(flags, sKey.Render(fmt.Sprintf("%d %s", n, plural(n, "comment"))))
	}
	if v.rev.loading {
		flags = append(flags, sDim.Render("refreshing"))
	}
	if fs := v.diffFiles(); fs != nil {
		flags = append(flags, sDim.Render(fmt.Sprintf("%d files", len(fs))))
	}
	return strings.Join(flags, "  ")
}

// askUnsent asks before leaving a session, or quitting, with n unsent
// comments; it reports whether it asked.
func (v *view) askUnsent(n int, quit bool) bool {
	if n == 0 {
		return false
	}
	v.rev.leaveAsk, v.rev.leaveQuit = true, quit
	act := "discard and leave"
	if quit {
		act = "discard and quit"
	}
	v.flash = sKey.Render(fmt.Sprintf(" %d unsent %s", n, plural(n, "comment"))) +
		sDim.Render("  ") + keyHelp("S", "send", "d", act, "esc", "stay")
	return true
}
