package parse

import (
	"fmt"
	json "github.com/goccy/go-json"
	"regexp"
	"sort"
	"strings"
)

// Input fields that best describe each built-in tool call, in priority order.
var summaryFields = map[string][]string{
	"Bash":            {"command"},
	"Read":            {"file_path"},
	"Write":           {"file_path"},
	"Edit":            {"file_path"},
	"MultiEdit":       {"file_path"},
	"NotebookEdit":    {"notebook_path"},
	"Grep":            {"pattern"},
	"Glob":            {"pattern"},
	"WebSearch":       {"query"},
	"WebFetch":        {"url"},
	"ToolSearch":      {"query"},
	"Agent":           {"description", "prompt"},
	"Task":            {"description", "prompt"},
	"SendMessage":     {"to", "message"},
	"Monitor":         {"description", "command"},
	"Artifact":        {"action", "file_path", "url"},
	"ScheduleWakeup":  {"reason"},
	"TodoWrite":       {},
	"AskUserQuestion": {},
}

func summarize(t *ToolCall) string {
	var in map[string]any
	if json.Unmarshal(t.Input, &in) != nil {
		return ""
	}
	switch t.Name {
	case "Skill":
		s := str(in["skill"])
		if a := str(in["args"]); a != "" {
			s += " " + a
		}
		return oneLine(s)
	case "Agent", "Task":
		s := str(in["description"])
		if typ := str(in["subagent_type"]); typ != "" {
			s = "[" + typ + "] " + s
		}
		return oneLine(s)
	case "AskUserQuestion":
		if qs, ok := in["questions"].([]any); ok && len(qs) > 0 {
			if q, ok := qs[0].(map[string]any); ok {
				return oneLine(str(q["question"]))
			}
		}
		return ""
	case "TodoWrite":
		if todos, ok := in["todos"].([]any); ok {
			return fmt.Sprintf("%d todos", len(todos))
		}
		return ""
	}
	if fields, ok := summaryFields[t.Name]; ok {
		for _, f := range fields {
			if v := str(in[f]); v != "" {
				return oneLine(v)
			}
		}
	}
	// Unknown and MCP tools: first string field, in stable key order.
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := str(in[k]); v != "" {
			return oneLine(k + "=" + v)
		}
	}
	return ""
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "…"
	}
	return s
}

var (
	reReminder = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
	reCmdName  = regexp.MustCompile(`(?s)^<command-name>(.*?)</command-name>`)
	reCmdArgs  = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
	reStdout   = regexp.MustCompile(`(?s)^<local-command-(?:stdout|stderr)>(.*?)</local-command-(?:stdout|stderr)>`)
	reTask     = regexp.MustCompile(`(?s)^\s*<task-notification>.*</task-notification>`)
	reTaskSum  = regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)
	reTaskStat = regexp.MustCompile(`(?s)<status>(.*?)</status>`)
)

// UserText classifies the text of a user record. Claude Code stores more
// than typed prompts as user messages: slash-command output, background
// task notifications and interruption notices come back as KindSystem.
// Slash commands the person typed stay KindUser, shown as "/cmd args".
// Only Claude Code's own wrapper tags are interpreted, and only when they
// open the message (as they always do in real transcripts), so a prompt that
// merely quotes them is left exactly as written.
func UserText(s string) (string, Kind) {
	s = strings.TrimSpace(reReminder.ReplaceAllString(s, ""))
	if m := reCmdName.FindStringSubmatch(s); m != nil {
		cmd := strings.TrimSpace(m[1])
		if a := reCmdArgs.FindStringSubmatch(s); a != nil && strings.TrimSpace(a[1]) != "" {
			cmd += " " + strings.TrimSpace(a[1])
		}
		return cmd, KindUser
	}
	if m := reStdout.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1]), KindSystem
	}
	if reTask.MatchString(s) {
		text := "Background task"
		if m := reTaskStat.FindStringSubmatch(s); m != nil {
			text += " " + strings.TrimSpace(m[1])
		}
		if m := reTaskSum.FindStringSubmatch(s); m != nil {
			text += ": " + strings.TrimSpace(m[1])
		}
		return text, KindSystem
	}
	if strings.HasPrefix(s, "[Request interrupted by user") {
		return s, KindSystem
	}
	return s, KindUser
}

// CleanMarkup returns the readable text of a user or system message.
func CleanMarkup(s string) string {
	text, _ := UserText(s)
	return text
}
