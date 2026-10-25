package ui

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	json "github.com/goccy/go-json"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
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
			b.WriteString(diff(ed.Old, ed.New, w) + "\n")
		}
	case "Write":
		b.WriteString(sBold.Render(get("file_path")) + "\n")
		b.WriteString(sAdd.Render(wrap(capLines(get("content"), maxDetailLines), w)) + "\n")
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
		} else {
			b.WriteString(wrap(res, w))
		}
	}
}

var sAgentRunning = lipgloss.NewStyle().Foreground(cAgent)

// diff shows an edit as "-"/"+" lines; an empty side shows no lines.
func diff(old, nw string, w int) string {
	var b strings.Builder
	for _, l := range parse.Lines(capLines(old, maxDetailLines/2)) {
		b.WriteString(sDel.Render(wrap("- "+l, w)) + "\n")
	}
	for _, l := range parse.Lines(capLines(nw, maxDetailLines/2)) {
		b.WriteString(sAdd.Render(wrap("+ "+l, w)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
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
