// Package export writes a session, or the part of it shown, as Markdown.
package export

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"github.com/jzills/snoop/internal/parse"
)

// Tool results longer than this are cut, with a note.
const maxResultLines = 2000

// Item is one event to export; Depth > 0 for events of a subagent shown
// under its Agent call.
type Item struct {
	Ev    *parse.Event
	Depth int
}

// All is every event of a session except thinking, as the export command
// writes it.
func All(s *parse.Session) []Item {
	var out []Item
	for _, e := range s.Events {
		if e.Kind != parse.KindThinking {
			out = append(out, Item{Ev: e})
		}
	}
	return out
}

// Filename is the default export name: <id-8>-<YYYYMMDD-HHMM>.md, from a
// session's or subagent's id.
func Filename(id string, now time.Time) string {
	if len(id) > 8 {
		id = id[:8]
	}
	return id + "-" + now.Format("20060102-1504") + ".md"
}

// Markdown renders items: prompts as headings, replies as text, tool calls
// as collapsible <details> blocks, subagent events quoted one level per depth.
// The heading is title, or else the session's.
func Markdown(s *parse.Session, title string, items []Item) string {
	var b strings.Builder
	if title == "" {
		title = s.Title
	}
	if title == "" {
		title = s.ID
	}
	b.WriteString("# " + oneLine(title) + "\n\n")
	var meta []string
	if s.CWD != "" {
		meta = append(meta, "`"+filepath.Base(s.CWD)+"`")
	}
	if s.Branch != "" {
		meta = append(meta, "branch `"+s.Branch+"`")
	}
	if !s.Start.IsZero() {
		meta = append(meta, s.Start.Local().Format("2006-01-02 15:04"))
	}
	meta = append(meta, "session `"+s.ID+"`")
	b.WriteString(strings.Join(meta, " · ") + "\n")

	for _, it := range items {
		block := event(it.Ev)
		if block == "" {
			continue
		}
		b.WriteString("\n" + quote(block, it.Depth))
	}
	return b.String()
}

func event(e *parse.Event) string {
	switch e.Kind {
	case parse.KindUser:
		first, rest, _ := strings.Cut(strings.TrimSpace(e.Text), "\n")
		out := "## " + oneLine(first) + "\n"
		if rest = strings.TrimSpace(rest); rest != "" {
			out += "\n" + rest + "\n"
		}
		return out
	case parse.KindText:
		return e.Text + "\n"
	case parse.KindThinking:
		return "<details><summary>Thinking</summary>\n\n" + e.Text + "\n\n</details>\n"
	case parse.KindSystem:
		return "*" + oneLine(e.Text) + "*\n"
	case parse.KindCompact:
		return "*Context compacted*\n"
	case parse.KindTool:
		return tool(e)
	}
	return ""
}

func tool(e *parse.Event) string {
	t := e.Tool
	status := "running"
	switch {
	case t.IsError:
		status = "error"
	case t.HasResult:
		status = "done"
	}
	if d := e.Duration(); d > 0 {
		status += " in " + d.Round(time.Millisecond).String()
	}
	name := t.Name
	if t.Kind != parse.ToolBuiltin {
		name = t.Group()
	}
	summary := "<code>" + html(name) + "</code>"
	if t.Summary != "" {
		summary += " " + html(t.Summary)
	}
	var b strings.Builder
	b.WriteString("<details><summary>" + summary + " — " + status + "</summary>\n\n")
	lang, body := Input(t)
	b.WriteString(fence(lang, body))
	if t.HasResult && t.Result != "" {
		b.WriteString("\nResult:\n\n" + fence("", capLines(t.Result, maxResultLines)))
	}
	b.WriteString("\n</details>\n")
	return b.String()
}

// Input is a tool call's input as text, with a code fence language: the
// command for Bash, a diff for edits, the content for Write, else JSON.
func Input(t *parse.ToolCall) (lang, body string) {
	var in map[string]any
	json.Unmarshal(t.Input, &in)
	get := func(k string) string { s, _ := in[k].(string); return s }
	switch t.Name {
	case "Bash":
		return "sh", get("command")
	case "Edit", "MultiEdit":
		var parts []string
		for _, ed := range t.Edits() {
			parts = append(parts, Diff(ed.Old, ed.New))
		}
		return "diff", strings.Join(parts, "\n")
	case "Write":
		return "", get("content")
	}
	pretty, err := json.MarshalIndent(in, "", "  ")
	if err != nil || in == nil {
		return "json", string(t.Input)
	}
	return "json", string(pretty)
}

// Diff is an edit as "-"/"+" lines.
func Diff(old, nw string) string {
	var b strings.Builder
	for _, l := range parse.Lines(old) {
		b.WriteString("- " + l + "\n")
	}
	for _, l := range parse.Lines(nw) {
		b.WriteString("+ " + l + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// fence wraps body in a code fence longer than any backtick run inside it.
func fence(lang, body string) string {
	n, run := 3, 0
	for _, r := range body {
		if r == '`' {
			run++
			n = max(n, run+1)
		} else {
			run = 0
		}
	}
	f := strings.Repeat("`", n)
	return f + lang + "\n" + strings.TrimRight(body, "\n") + "\n" + f + "\n"
}

// quote prefixes every line with one "> " per depth.
func quote(s string, depth int) string {
	if depth == 0 {
		return s
	}
	p := strings.Repeat("> ", depth)
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(p+l, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}

func capLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… %d more lines", len(lines)-n)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func html(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
