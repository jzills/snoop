package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	osc52 "github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/export"
	"github.com/jzills/snoop/internal/parse"
)

// copyText is what y copies for an event: the command for Bash, the diff
// for edits, the content for Write, the full text of messages, and the
// result (else the input) for other tools.
func copyText(e *parse.Event) string {
	t := e.Tool
	if t == nil {
		if e.Kind == parse.KindCompact {
			return compactText(e.Compact)
		}
		return e.Text
	}
	switch t.Name {
	case "Bash", "Edit", "MultiEdit", "Write":
		_, body := export.Input(t)
		return body
	}
	if t.HasResult && t.Result != "" {
		return t.Result
	}
	_, body := export.Input(t)
	return body
}

// fileDiff is every change to a file, each headed by its time and tool.
func fileDiff(f *fileEntry) string {
	var parts []string
	for _, t := range f.touches {
		if t.op == opRead {
			continue
		}
		_, body := export.Input(t.ev.Tool)
		if t.ev.Tool.Name == "NotebookEdit" {
			body = copyText(t.ev)
		}
		head := fmt.Sprintf("@@ %s %s %s", t.ev.Time.Local().Format("15:04:05"), t.ev.Tool.Name, f.path)
		parts = append(parts, head+"\n"+body)
	}
	return strings.Join(parts, "\n")
}

// selectionText is what y copies on the current tab.
func (v *view) selectionText() string {
	switch v.tab {
	case tabTools:
		if g := v.selectedGroup(); g != nil && g.ev != nil {
			return copyText(g.ev)
		}
	case tabFiles:
		if f := v.selectedFile(); f != nil {
			return fileDiff(f)
		}
	case tabTimeline:
		if e := v.selected(); e != nil {
			return copyText(e)
		}
	}
	return ""
}

// copyCmd puts text on the clipboard with OSC 52, which works over SSH and
// in tmux (with set-clipboard on).
func copyCmd(text string) tea.Cmd {
	return func() tea.Msg {
		seq := osc52.New(text)
		switch {
		case os.Getenv("TMUX") != "":
			seq = seq.Tmux()
		case os.Getenv("STY") != "":
			seq = seq.Screen()
		}
		seq.WriteTo(os.Stderr)
		return nil
	}
}

func (v *view) copySelection() tea.Cmd {
	text := v.selectionText()
	if text == "" {
		v.flash = sDim.Render(" nothing to copy here")
		return nil
	}
	v.flash = sOK.Render(fmt.Sprintf(" %s copied %s chars", glyphOK, count(len([]rune(text)))))
	return copyCmd(text)
}

// exportView writes the timeline as shown (search, filter, thinking and
// expanded subagents included) to Markdown in the current directory.
func (v *view) exportView(now time.Time) {
	items := make([]export.Item, len(v.shown))
	for i, e := range v.shown {
		items[i] = export.Item{Ev: e, Depth: v.depth[e]}
	}
	// A subagent's transcript carries its parent's session id, so it is
	// named and titled after itself.
	name, title := export.Filename(v.sess.ID, now), ""
	if v.label != "" {
		id := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(v.path), ".jsonl"), "agent-")
		name, title = "agent-"+export.Filename(id, now), v.label
	}
	if err := os.WriteFile(name, []byte(export.Markdown(v.sess, title, items)), 0o644); err != nil {
		v.flash = sErr.Render(" " + glyphErr + " " + err.Error())
		return
	}
	abs, _ := filepath.Abs(name)
	v.flash = sOK.Render(fmt.Sprintf(" %s exported %d events to %s", glyphOK, len(items), abs))
}
