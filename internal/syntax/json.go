package syntax

import (
	"bytes"
	"encoding/json"
	"strings"
)

// maxJSON is the largest text IndentJSON reformats, so a huge result
// doesn't slow the detail pane down.
const maxJSON = 256 << 10

// IndentJSON indents s when it is a JSON object or array, such as a tool
// result an MCP server returned on one line. It reports false for anything
// else, and for text over maxJSON bytes.
func IndentJSON(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) > maxJSON || s == "" || s[0] != '{' && s[0] != '[' {
		return "", false
	}
	var buf bytes.Buffer
	if json.Indent(&buf, []byte(s), "", "  ") != nil {
		return "", false
	}
	return buf.String(), true
}

// Segment is a run of a tool result: indented JSON, or other text as it
// was.
type Segment struct {
	Text string
	JSON bool
}

// JSONSegments splits a result into JSON and other text, so command
// output such as `{"state":"OPEN"}` followed by `exit 0` can show its JSON
// indented. The whole result is one JSON value, or each line that is a
// JSON object or array on its own is indented; JSON already spread over
// lines is left alone. It reports false when there's no JSON to indent,
// and for results over maxJSON bytes.
func JSONSegments(s string) ([]Segment, bool) {
	if len(s) > maxJSON {
		return nil, false
	}
	if js, ok := IndentJSON(s); ok {
		return []Segment{{js, true}}, true
	}
	var out []Segment
	found := false
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		seg := Segment{Text: line}
		if js, ok := IndentJSON(line); ok {
			seg, found = Segment{js, true}, true
		}
		if n := len(out); n > 0 && out[n-1].JSON == seg.JSON {
			out[n-1].Text += "\n" + seg.Text
			continue
		}
		out = append(out, seg)
	}
	if !found {
		return nil, false
	}
	return out, true
}
