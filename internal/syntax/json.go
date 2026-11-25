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
