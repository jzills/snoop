package ui

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	json "github.com/goccy/go-json"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
	"github.com/jzills/snoop/internal/syntax"
)

// Long tool results are capped so a 1MB Read does not stall rendering.
const maxDetailLines = 600

func wrap(s string, w int) string {
	return ansi.Wrap(s, w, "")
}

func capLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + "\n" + sDim.Render(fmt.Sprintf("… %d more lines", len(lines)-n))
}

func section(title string) string {
	return sSection.Render(title)
}

// subagentLookup finds a subagent's metadata by id.
type subagentLookup func(id string) (store.SubagentMeta, bool)

func renderDetail(e *parse.Event, lookup subagentLookup, w int) string {
	if e == nil {
		return sDim.Render("nothing selected")
	}
	var b strings.Builder
	ts := ""
	if !e.Time.IsZero() {
		ts = e.Time.Local().Format("Jan 02 15:04:05")
	}
	head := func(glyph string, gs lipgloss.Style, label string) {
		b.WriteString(gs.Render(glyph) + " " + sPaneTitle.Render(label) + "  " + sDim.Render(ts) + "\n")
	}
	switch e.Kind {
	case parse.KindUser:
		head(glyphUser, sPrompt, "Prompt")
		b.WriteString("\n" + wrap(capLines(e.Text, maxDetailLines), w))
	case parse.KindText:
		head(glyphText, sAccent, "Assistant")
		if e.Model != "" {
			b.WriteString(sDim.Render(e.Model) + "\n")
		}
		b.WriteString("\n" + wrap(capLines(e.Text, maxDetailLines), w))
	case parse.KindThinking:
		head(glyphThinking, sThinking, "Thinking")
		b.WriteString("\n" + sThinking.Render(wrap(capLines(e.Text, maxDetailLines), w)))
	case parse.KindCompact:
		head(glyphCompact, sDim, "Compacted")
		b.WriteString("\n" + wrap(compactText(e.Compact), w))
		if c := e.Compact; c != nil && c.Post == 0 {
			b.WriteString("\n" + sDim.Render("context after: waiting for the next request"))
		}
	case parse.KindSystem:
		head(glyphSystem, sDim, "System")
		b.WriteString("\n" + wrap(capLines(e.Text, maxDetailLines), w))
	case parse.KindTool:
		renderTool(&b, e, lookup, w, ts)
	}
	return b.String()
}

func renderTool(b *strings.Builder, e *parse.Event, lookup subagentLookup, w int, ts string) {
	t := e.Tool
	ks := lipgloss.NewStyle().Foreground(kindColor(t.Kind)).Bold(true)
	b.WriteString(ks.Render(kindGlyph(t.Kind)+" "+t.Name) + "  " + sDim.Render(t.Kind.String()+" · "+ts) + "\n")
	switch {
	case t.IsError:
		b.WriteString(sErr.Render(glyphErr + " error"))
	case t.HasResult:
		b.WriteString(sOK.Render(glyphOK + " done"))
		if d := e.Duration(); d > 0 {
			b.WriteString(sDim.Render(" in " + dur(d)))
		}
	default:
		b.WriteString(sDim.Render(glyphPending + " running…"))
	}
	b.WriteString("\n")

	var in map[string]any
	json.Unmarshal(t.Input, &in)
	get := func(k string) string { s, _ := in[k].(string); return s }

	b.WriteString(section("Input") + "\n")
	switch t.Name {
	case "Bash":
		if d := get("description"); d != "" {
			b.WriteString(sDim.Render(wrap(d, w)) + "\n")
		}
		b.WriteString(sAccent.Render("$ ") + wrap(capLines(get("command"), maxDetailLines), w-2) + "\n")
	case "Edit", "MultiEdit":
		b.WriteString(sBold.Render(get("file_path")) + "\n")
		for _, ed := range t.Edits() {
			b.WriteString(diff(get("file_path"), ed.Old, ed.New, w) + "\n")
		}
	case "Write":
		b.WriteString(sBold.Render(get("file_path")) + "\n")
		b.WriteString(codeBlock(get("file_path"), get("content"), w) + "\n")
	case "Agent", "Task":
		if typ := get("subagent_type"); typ != "" {
			b.WriteString(sDim.Render("type ") + lipgloss.NewStyle().Foreground(cAgent).Render(typ) + "\n")
		}
		b.WriteString(sBold.Render(wrap(get("description"), w)) + "\n\n")
		b.WriteString(wrap(capLines(get("prompt"), maxDetailLines), w) + "\n")
	default:
		b.WriteString(prettyInput(t.Input, w) + "\n")
	}

	if t.AgentID != "" {
		b.WriteString(section("Subagent") + "\n")
		if meta, ok := lookup(t.AgentID); ok {
			state := sOK.Render("finished")
			if !meta.Finished {
				state = sAgentRunning.Render("running")
			}
			b.WriteString(sDim.Render(meta.AgentType+" · ") + state + "  " + sKey.Render("a") + sDim.Render(" open transcript") + "\n")
		} else {
			b.WriteString(sDim.Render("transcript not found for "+t.AgentID) + "\n")
		}
	}

	if t.HasResult {
		b.WriteString(section("Result") + "\n")
		res := capLines(strings.TrimRight(t.Result, "\n"), maxDetailLines)
		if t.IsError {
			b.WriteString(sErr.Render(wrap(res, w)))
		} else if code, ok := readBlock(t, w); ok {
			b.WriteString(code)
		} else if segs, ok := resultJSON(t); ok {
			b.WriteString(resultSegments(segs, w))
		} else {
			b.WriteString(wrap(res, w))
		}
	}
}

// readBlock shows a Read result's code colored by the file's language,
// with its line numbers dimmed, and any text after the numbered lines as
// it was. It reports false for other calls, for a file in no known
// language, and for a result that doesn't start with numbered lines, such
// as an error.
func readBlock(t *parse.ToolCall, w int) (string, bool) {
	path := t.InputString("file_path")
	if t.Name != "Read" || syntax.Lang(path) == "" {
		return "", false
	}
	nums, code, tail := readLines(strings.TrimRight(t.Result, "\n"))
	if len(code) == 0 {
		return "", false
	}
	more := len(code) - maxDetailLines
	if more > 0 {
		nums, code, tail = nums[:maxDetailLines], code[:maxDetailLines], ""
	}
	numW := 0
	for _, n := range nums {
		numW = max(numW, len(n))
	}
	lines, st := renderCode(path, strings.Join(code, "\n"), sText)
	var b strings.Builder
	for i, l := range lines {
		if st != hlWaiting {
			l = sDim.Render(fmt.Sprintf("%*s  ", numW, nums[i])) + l
		}
		b.WriteString(wrap(l, w) + "\n")
	}
	if more > 0 {
		b.WriteString(sDim.Render(fmt.Sprintf("… %d more lines", more)) + "\n")
	}
	if tail != "" {
		b.WriteString(wrap(tail, w) + "\n")
	}
	return strings.TrimRight(b.String(), "\n"), true
}

// readLines splits the numbered lines Claude Code returns for a Read, such
// as "12\tcode" or "    12→code", into line numbers and code, up to the
// first line that isn't numbered. The rest, such as a note Claude Code
// added, is tail.
func readLines(res string) (nums, code []string, tail string) {
	lines := strings.Split(res, "\n")
	for i, l := range lines {
		n, c, ok := numbered(l)
		if !ok {
			return nums, code, strings.Join(lines[i:], "\n")
		}
		nums, code = append(nums, n), append(code, c)
	}
	return nums, code, ""
}

func numbered(l string) (num, code string, ok bool) {
	l = strings.TrimLeft(l, " ")
	end := strings.IndexFunc(l, func(r rune) bool { return r < '0' || r > '9' })
	if end <= 0 {
		return "", "", false
	}
	switch rest := l[end:]; {
	case strings.HasPrefix(rest, "\t"):
		return l[:end], rest[1:], true
	case strings.HasPrefix(rest, "→"):
		return l[:end], rest[len("→"):], true
	}
	return "", "", false
}

// resultSegments shows a result's JSON indented and colored and the rest
// as it was, maxDetailLines in all.
func resultSegments(segs []syntax.Segment, w int) string {
	segs, more := syntax.Fit(segs, maxDetailLines)
	var parts []string
	for _, sg := range segs {
		parts = append(parts, segment(sg, w))
	}
	if more > 0 {
		parts = append(parts, sDim.Render(fmt.Sprintf("… %d more lines", more)))
	}
	return strings.Join(parts, "\n")
}

// segment draws one run of a result, its JSON colored and anything else,
// or JSON still plain, in the text color.
func segment(sg syntax.Segment, w int) string {
	if !sg.JSON {
		return wrap(sg.Text, w)
	}
	lines, _ := renderCode("result.json", sg.Text, sText)
	return wrap(strings.Join(lines, "\n"), w)
}

// jsonCache keeps the last result's segments, since the detail pane
// redraws the same event while its code is colored, and indenting a big
// result each time would slow Update down.
var jsonCache struct {
	sync.Mutex
	tool *parse.ToolCall
	res  string
	segs []syntax.Segment
	ok   bool
}

func resultJSON(t *parse.ToolCall) ([]syntax.Segment, bool) {
	c := &jsonCache
	c.Lock()
	defer c.Unlock()
	if c.tool != t || c.res != t.Result {
		c.tool, c.res = t, t.Result
		c.segs, c.ok = syntax.JSONSegments(t.Result)
	}
	return c.segs, c.ok
}

var sAgentRunning = lipgloss.NewStyle().Foreground(cAgent)

// diff shows an edit to the file at path as "-"/"+" lines, the code
// colored by its language; an empty side shows no lines.
func diff(path, old, nw string, w int) string {
	var b strings.Builder
	side := func(lines []string, more int, mark string, st lipgloss.Style) {
		if len(lines) == 0 {
			return
		}
		code, state := renderCode(path, strings.Join(lines, "\n"), st)
		for _, l := range code {
			if state != hlWaiting {
				l = st.Render(mark) + l
			}
			b.WriteString(wrap(l, w) + "\n")
		}
		if more > 0 {
			b.WriteString(sDim.Render(fmt.Sprintf("… %d more lines", more)) + "\n")
		}
	}
	o, n, oMore, nMore := editSides(old, nw)
	side(o, oMore, "- ", sDel)
	side(n, nMore, "+ ", sAdd)
	return strings.TrimRight(b.String(), "\n")
}

// codeBlock shows a written file's content, colored by its language, or
// in the added-line color when none matches.
func codeBlock(path, src string, w int) string {
	lines, more := writeLines(src)
	code, _ := renderCode(path, strings.Join(lines, "\n"), sAdd)
	out := wrap(strings.Join(code, "\n"), w)
	if more > 0 {
		out += "\n" + sDim.Render(fmt.Sprintf("… %d more lines", more))
	}
	return out
}

// prettyInput shows top-level fields as labelled blocks so multi-line
// strings keep their real newlines; anything else falls back to JSON.
func prettyInput(raw []byte, w int) string {
	var in map[string]stdjson.RawMessage
	if json.Unmarshal(raw, &in) != nil || len(in) == 0 {
		return prettyJSON(raw, w)
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		var s string
		if json.Unmarshal(in[k], &s) != nil {
			s = prettyJSON(in[k], w-2)
		} else {
			s = sText.Render(wrap(capLines(s, maxDetailLines), w-2))
		}
		b.WriteString(sDim.Render(k) + "\n" + indent(s, "  ") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func indent(s, pad string) string {
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func prettyJSON(raw []byte, w int) string {
	var buf bytes.Buffer
	if stdjson.Indent(&buf, raw, "", "  ") != nil {
		return wrap(string(raw), w)
	}
	return sText.Render(wrap(capLines(buf.String(), maxDetailLines), w))
}
