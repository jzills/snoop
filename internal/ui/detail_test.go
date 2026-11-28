package ui

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
)

func resultEvent(t *testing.T, result string, isErr bool) *parse.Event {
	t.Helper()
	res, _ := json.Marshal(result)
	in := `{"type":"assistant","uuid":"a1","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"mcp__x__get","input":{}}]}}
{"type":"user","uuid":"r1","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":` + map[bool]string{true: "true", false: "false"}[isErr] + `,"content":` + string(res) + `}]}}
`
	s, _ := parse.Parse(strings.NewReader(in))
	for _, e := range s.Events {
		if e.Tool != nil {
			return e
		}
	}
	t.Fatal("no tool call")
	return nil
}

func noLookup(string) (store.SubagentMeta, bool) { return store.SubagentMeta{}, false }

func TestJSONResultIndented(t *testing.T) {
	e := resultEvent(t, `{"id":7,"tags":["a"]}`, false)
	want := "{\n  \"id\": 7,\n  \"tags\": [\n    \"a\"\n  ]\n}"
	waitSpans(t, func() bool {
		return strings.HasSuffix(ansi.Strip(renderDetail(e, noLookup, 80)), want)
	})
}

func TestNonJSONResultUnchanged(t *testing.T) {
	for _, c := range []struct {
		res   string
		isErr bool
	}{
		{`{"id":7} and more`, false},
		{`{"error":"nope"}`, true},
	} {
		e := resultEvent(t, c.res, c.isErr)
		if got := ansi.Strip(renderDetail(e, noLookup, 80)); !strings.HasSuffix(got, c.res) {
			t.Errorf("result %q rendered as:\n%s", c.res, got)
		}
	}
}

func TestMixedJSONResultIndented(t *testing.T) {
	e := resultEvent(t, "tagged\n{\"id\":7}\nexit 0", false)
	want := "Result\ntagged\n{\n  \"id\": 7\n}\nexit 0"
	waitSpans(t, func() bool {
		return strings.HasSuffix(ansi.Strip(renderDetail(e, noLookup, 80)), want)
	})
}

func readEvent(t *testing.T, path, result string) *parse.Event {
	t.Helper()
	p, _ := json.Marshal(path)
	res, _ := json.Marshal(result)
	in := `{"type":"assistant","uuid":"a1","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":` + string(p) + `}}]}}
{"type":"user","uuid":"r1","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":` + string(res) + `}]}}
`
	s, _ := parse.Parse(strings.NewReader(in))
	return s.Events[len(s.Events)-1]
}

func TestReadResultColored(t *testing.T) {
	var src []string
	for i := 1; i <= 10; i++ {
		src = append(src, strconv.Itoa(i)+"\tkey"+strconv.Itoa(i)+": value")
	}
	e := readEvent(t, "/repo/chart/values.yaml", strings.Join(src, "\n"))
	want := "Result\n 1  key1: value\n 2  key2: value"
	var got string
	waitSpans(t, func() bool {
		got = renderDetail(e, noLookup, 80)
		return strings.Contains(ansi.Strip(got), want)
	})
	if !strings.HasSuffix(ansi.Strip(got), "10  key10: value") {
		t.Errorf("last line of:\n%s", ansi.Strip(got))
	}
}

func TestReadResultPlainWhenUnknown(t *testing.T) {
	for _, c := range []struct{ path, res string }{
		{"/repo/notes.unknownext", "1\tone\n2\ttwo"},
		{"/repo/a.yaml", "File does not exist."},
	} {
		got := ansi.Strip(renderDetail(readEvent(t, c.path, c.res), noLookup, 80))
		if !strings.HasSuffix(got, "Result\n"+c.res) {
			t.Errorf("%s rendered as:\n%s", c.path, got)
		}
	}
}

func TestReadResultKeepsTrailingText(t *testing.T) {
	res := "1\tkey: value\n2\tother: 2\n\n<system-reminder>\nnote\n</system-reminder>"
	e := readEvent(t, "/repo/values.yaml", res)
	want := "Result\n1  key: value\n2  other: 2\n\n<system-reminder>\nnote\n</system-reminder>"
	waitSpans(t, func() bool { return strings.HasSuffix(ansi.Strip(renderDetail(e, noLookup, 80)), want) })
}

func TestReadResultArrowAndPadding(t *testing.T) {
	e := readEvent(t, "/repo/values.yaml", "     9→key: value\n    10→other: 2")
	want := "Result\n 9  key: value\n10  other: 2"
	waitSpans(t, func() bool { return strings.HasSuffix(ansi.Strip(renderDetail(e, noLookup, 80)), want) })
}
