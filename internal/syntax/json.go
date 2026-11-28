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
// was. Raw is the run as the result had it.
type Segment struct {
	Text, Raw string
	JSON      bool
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
		return []Segment{{js, strings.TrimSpace(s), true}}, true
	}
	var out []Segment
	found := false
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		seg := Segment{Text: line, Raw: line}
		if js, ok := IndentJSON(line); ok {
			seg, found = Segment{js, line, true}, true
		}
		if n := len(out); n > 0 && out[n-1].JSON == seg.JSON {
			out[n-1].Text += "\n" + seg.Text
			out[n-1].Raw += "\n" + seg.Raw
			continue
		}
		out = append(out, seg)
	}
	if !found {
		return nil, false
	}
	return out, true
}

// Fit keeps segments to n lines in all. A JSON run whose indented form
// would go past the limit keeps its raw lines instead, so a long one-line
// result isn't cut short by its own indenting. When the raw lines don't
// fit either, the run is cut, and more is how many lines of the result
// are left out.
func Fit(segs []Segment, n int) (out []Segment, more int) {
	lines := func(s string) int { return strings.Count(s, "\n") + 1 }
	for i, sg := range segs {
		if lines(sg.Text) > n {
			sg.Text = sg.Raw
		}
		if k := lines(sg.Text); k <= n {
			out, n = append(out, sg), n-k
			continue
		}
		raw := strings.Split(sg.Raw, "\n")
		if n > 0 {
			sg.Text = strings.Join(raw[:n], "\n")
			out = append(out, sg)
		}
		more = len(raw) - n
		for _, rest := range segs[i+1:] {
			more += lines(rest.Raw)
		}
		break
	}
	return out, more
}
