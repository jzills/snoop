package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// The ? overlay lists every key for the current screen. Footers show only
// the most used keys plus "? help", so they always fit.

type helpSection struct {
	title string
	keys  [][2]string // key, description
}

var helpTabs = helpSection{"Tabs", [][2]string{
	{"1 2 3 4 5", "Timeline, Tools, Stats, Files, Diff"},
	{"[ ]", "previous / next tab"},
}}

var helpGeneral = helpSection{"General", [][2]string{
	{"!", "jump to the latest alert (error, waiting tool, end of turn, compaction, context 90%, permission request)"},
	{"A  D", "allow once / deny the session's permission request (needs snoop hooks install)"},
	{"y", "copy the selection: a command, diff, file content, message or result (Files: the file's changes)"},
	{"e", "export the timeline as shown to Markdown in the current directory"},
	{"esc", "clear search and filter, then go back"},
	{"?", "show or hide this help"},
	{"q  ctrl+c", "quit"},
}}

var helpPicker = []helpSection{
	{"Sessions", [][2]string{
		{"↑↓  j k", "move"},
		{"pgup pgdn", "page (also ctrl+u ctrl+d)"},
		{"g  G", "first / last"},
		{"enter  →", "open the session"},
		{"wheel", "move"},
		{"o", "sort by the next column: age, project, branch, session, size"},
		{"O", "reverse the order"},
		{"", "search and find results stay best match first until you choose a column"},
	}},
	{"Search", [][2]string{
		{"/", "fuzzy search titles, projects and branches"},
		{"enter", "keep the search and browse results"},
		{"esc", "clear the search (with none: quit)"},
		{"c", "only sessions for this directory"},
	}},
	{"Find", [][2]string{
		{"f", "find sessions by a file they read or changed, a command they ran or a prompt; every word must match one of them"},
		{"", "subagents count too; transcripts are indexed in the background and cached"},
		{"enter", "keep the results, then open a session with the timeline searched for the same words"},
		{"esc", "leave find"},
	}},
	{"Live", [][2]string{
		{"w", "the live board: every session written in the last 30 minutes, across projects (also snoop --live)"},
	}},
	{"Summary", [][2]string{
		{"s", "switch between the list and a summary of the sessions listed: cost, tokens, daily use, tools, skills, MCP, subagents, models"},
		{"", "follows c and the search; transcripts are scanned in the background and cached"},
		{"↑↓", "scroll the summary"},
	}},
	{"General", [][2]string{
		{"?", "show or hide this help"},
		{"q  ctrl+c", "quit"},
	}},
}

var helpBoard = []helpSection{
	{"Live board", [][2]string{
		{"↑↓  j k", "move"},
		{"g  G", "first / last"},
		{"enter  →", "open the session; esc comes back here"},
		{"esc  w", "back to the session picker"},
		{"A  D", "allow once / deny the selected session's permission request (needs snoop hooks install)"},
		{"o", "sort by the next column: state, project, session, context, cost, age"},
		{"O", "reverse the order"},
	}},
	{"States", [][2]string{
		{"? asking", "the session asks for permission to use a tool; A allows it once, D denies it"},
		{glyphPending + " waiting", "a tool call has had no result for --stall (30s): often a permission prompt"},
		{glyphOK + " your turn", "the turn ended, or was interrupted"},
		{"● working", "replying or running a tool or subagent"},
		{glyphSystem + " idle", "no prompts yet, or nothing written for 10 minutes"},
		{"", "sessions leave the board after 30 minutes without writes"},
	}},
	{"Alerts", [][2]string{
		{"--bell", "ring the terminal bell on the alerts chosen with --alerts"},
		{"--notify", "send a desktop notification for them, naming the session: a Windows toast on WSL, the terminal's own in iTerm2, WezTerm, Ghostty and kitty or over SSH, osascript or notify-send elsewhere"},
		{"", "none while snoop has focus; one per session and alert a minute; one between several snoops"},
	}},
	{"General", [][2]string{
		{"?", "show or hide this help"},
		{"q  ctrl+c", "quit"},
	}},
}

var helpTimeline = []helpSection{
	{"Move", [][2]string{
		{"↑↓  j k", "move"},
		{"pgup pgdn", "page (also ctrl+b ctrl+f, space)"},
		{"g", "first event"},
		{"G", "last event, and resume following"},
		{"n  N", "next / previous prompt"},
		{"wheel", "move (scrolls the detail pane under the mouse)"},
	}},
	{"Search and filter", [][2]string{
		{"/", "search text, commands, paths and results; every word must match"},
		{"enter  esc", "keep / clear the search"},
		{"f  F", "cycle filter: all, chat, tools, skills, mcp, agents, errors"},
		{"t", "show thinking blocks"},
	}},
	{"Detail pane", [][2]string{
		{"enter  tab  →", "focus it (fills the screen on narrow terminals)"},
		{"J K", "scroll it without leaving the list (also ctrl+d ctrl+u)"},
		{"↑↓ g G", "scroll, when focused"},
		{"tab  ←  esc", "back to the list"},
	}},
	{"Subagents", [][2]string{
		{"x", "expand or collapse an Agent call's subagent inline"},
		{"X", "expand every subagent (nested ones too); again to collapse all"},
		{"●", "a collapsed subagent has events matching the search or filter"},
		{"a  o", "open the subagent transcript of an Agent call"},
		{"esc", "return to the parent session"},
	}},
	{"Live", [][2]string{
		{"↓", "in the header: following, so new events are selected as they arrive"},
		{"", "moving up pauses following; G resumes it"},
	}},
	helpTabs,
	helpGeneral,
}

var helpTools = []helpSection{
	{"Move", [][2]string{
		{"↑↓  j k", "move"},
		{"pgup pgdn", "page"},
		{"g  G", "first / last row"},
	}},
	{"Groups", [][2]string{
		{"space", "expand or collapse a group"},
		{"enter", "expand a group; on a call, show it in the timeline"},
		{"→  l", "expand a group, or focus the detail pane"},
		{"←  h", "collapse a group, or move to the group of a call"},
		{"E", "expand everything; again to reset"},
		{"/", "keep only matching calls, with their groups open"},
		{"a  o", "open the subagent transcript of an Agent call"},
	}},
	{"Detail pane", [][2]string{
		{"tab", "focus it"},
		{"J K", "scroll it (also ctrl+d ctrl+u)"},
	}},
	helpTabs,
	helpGeneral,
}

var helpStats = []helpSection{
	{"Scroll", [][2]string{
		{"↑↓  j k", "scroll"},
		{"pgup pgdn", "page"},
		{"wheel", "scroll"},
	}},
	helpTabs,
	helpGeneral,
}

var helpFiles = []helpSection{
	{"Move", [][2]string{
		{"↑↓  j k", "move"},
		{"pgup pgdn", "page (also space)"},
		{"g  G", "first / last file"},
	}},
	{"Files", [][2]string{
		{"enter", "show the file's latest change in the timeline (a subagent's change shows its Agent call)"},
		{"n  N", "next / previous change in the detail pane"},
		{"/", "keep only files whose path matches"},
		{"T", "switch between the list and a tree of folders"},
		{"", "changed files first, then files only read; most recent first"},
	}},
	{"Tree", [][2]string{
		{"space  enter", "open or close a folder"},
		{"←  →", "close / open a folder (← on a file moves to its folder)"},
		{"E", "open every folder, or go back"},
		{"", "changes made through Bash aren't tracked"},
	}},
	{"Detail pane", [][2]string{
		{"tab  →", "focus it"},
		{"J K", "scroll it (also ctrl+d ctrl+u)"},
	}},
	helpTabs,
	helpGeneral,
}

var helpDiff = []helpSection{
	{"Files", [][2]string{
		{"↑↓  j k", "move"},
		{"g  G", "first / last file"},
		{"enter  tab  →", "move into the file's lines"},
		{"", "files this session changed come first; the others are dimmed"},
		{"b", "switch between the uncommitted changes and the whole branch since it left its base"},
		{"B", "choose the base (saved for the branch in git config)"},
		{"r", "read the diff again (it also refreshes as the session edits files)"},
	}},
	{"Lines", [][2]string{
		{"↑↓  j k", "move"},
		{"J K  n N", "next / previous hunk"},
		{"pgup pgdn", "half a page"},
		{"tab  ←  esc", "back to the files"},
	}},
	{"Comments", [][2]string{
		{"c", "comment on the line, or on the range"},
		{"v", "start or clear a range; move to extend it"},
		{"C", "comment on the whole file"},
		{"e  x", "edit / delete the comment on the line"},
		{"S", "send every comment to the agent, which sees them after its current tool call, when it finishes, or with your next prompt"},
		{"", "sending needs the hooks from snoop hooks install; without them, S copies the comments instead"},
		{"y", "copy the file's diff"},
	}},
	helpTabs,
	helpGeneral,
}

// helpContent picks the sections and title for the current screen.
func (a *App) helpContent() (string, []helpSection) {
	v := a.top()
	switch {
	case v == nil && a.board != nil:
		return "Live board", helpBoard
	case v == nil:
		return "Session picker", helpPicker
	case v.tab == tabTools:
		return "Tools", helpTools
	case v.tab == tabStats:
		return "Stats", helpStats
	case v.tab == tabFiles:
		return "Files", helpFiles
	case v.tab == tabDiff:
		return "Diff", helpDiff
	}
	return "Timeline", helpTimeline
}

func renderHelp(title string, sections []helpSection, w int) string {
	keyW := 0
	for _, s := range sections {
		for _, k := range s.keys {
			keyW = max(keyW, lipgloss.Width(k[0]))
		}
	}
	descW := max(w-keyW-3, 10)
	var b strings.Builder
	b.WriteString(sTitle.Render("Keys") + sDim.Render(" · "+title))
	for _, s := range sections {
		b.WriteString("\n" + sSection.Render(s.title))
		for _, k := range s.keys {
			desc := strings.Split(wrap(k[1], descW), "\n")
			b.WriteString("\n " + sKey.Render(fit(k[0], keyW)) + "  " + sText.Render(desc[0]))
			for _, more := range desc[1:] {
				b.WriteString("\n " + strings.Repeat(" ", keyW) + "  " + sText.Render(more))
			}
		}
	}
	return b.String()
}

// helpView draws the overlay as a centred, scrollable box.
func (a *App) helpView() string {
	title, sections := a.helpContent()
	boxW := min(a.w-4, 84)
	innerW := boxW - 4
	content := renderHelp(title, sections, innerW)
	footer := sDim.Render("? or esc to close")
	innerH := min(lipgloss.Height(content), max(a.h-6, 3))
	if a.helpVP.Width != innerW || a.helpVP.Height != innerH {
		a.helpVP = viewport.New(innerW, innerH)
	}
	a.helpVP.SetContent(content)
	if !a.helpVP.AtBottom() || a.helpVP.YOffset > 0 {
		footer = sDim.Render("↑↓ scroll · ? or esc to close")
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cAccent).
		Padding(0, 1).Width(boxW).
		Render(a.helpVP.View() + "\n\n" + footer)
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, box)
}
