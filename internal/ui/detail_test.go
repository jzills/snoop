package ui

import (
	"encoding/json"
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
