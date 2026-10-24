package export

import (
	"strings"
	"testing"
	"time"

	"github.com/jzills/snoop/internal/parse"
)

const fixture = `{"type":"ai-title","sessionId":"0123456789ab","aiTitle":"Fix it"}
{"type":"user","uuid":"u1","sessionId":"0123456789ab","cwd":"/src/repo","gitBranch":"main","timestamp":"2026-10-24T10:00:00Z","message":{"content":"please fix\nthe build"}}
{"type":"assistant","uuid":"a0","timestamp":"2026-10-24T10:00:00.5Z","message":{"id":"m0","content":[{"type":"thinking","thinking":"secret plan"}]}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"echo '` + "```" + `'"}},{"type":"tool_use","id":"t2","name":"Edit","input":{"file_path":"a.go","old_string":"x","new_string":"y"}}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"<b>ok</b>"},{"type":"tool_result","tool_use_id":"t2","content":"no match","is_error":true}]}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:03Z","message":{"id":"m2","content":[{"type":"text","text":"Fixed."}]}}
`

func TestMarkdown(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(fixture))
	md := Markdown(s, All(s))
	for _, want := range []string{
		"# Fix it\n",
		"`repo` · branch `main`",
		"## please fix\n\nthe build\n",
		"<details><summary><code>Bash</code> echo '```' — done in 1s</summary>",
		"````sh\necho '```'\n````",
		"```diff\n- x\n+ y\n```",
		"<code>Edit</code> a.go — error",
		"Result:\n\n```\n<b>ok</b>\n```",
		"\nFixed.\n",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "secret plan") {
		t.Error("thinking exported by default")
	}
}

func TestSubagentEventsQuoted(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(fixture))
	md := Markdown(s, []Item{{Ev: s.Events[len(s.Events)-1], Depth: 2}})
	if !strings.Contains(md, "\n> > Fixed.\n") {
		t.Errorf("depth 2 not quoted:\n%s", md)
	}
}

func TestFilename(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(fixture))
	if got := Filename(s, time.Date(2026, 10, 24, 9, 5, 0, 0, time.Local)); got != "01234567-20261024-0905.md" {
		t.Errorf("filename = %q", got)
	}
}

func TestResultCapped(t *testing.T) {
	long := strings.Repeat("line\n", maxResultLines+5)
	if got := capLines(long, maxResultLines); !strings.HasSuffix(got, "… 6 more lines") {
		t.Errorf("cap tail = %q", got[len(got)-30:])
	}
}
