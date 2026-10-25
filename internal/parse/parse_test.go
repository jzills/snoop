package parse

import (
	"strings"
	"testing"
)

const fixture = `{"type":"ai-title","sessionId":"s1","aiTitle":"Fix the build"}
{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-10-24T10:00:00.000Z","cwd":"/repo","gitBranch":"main","version":"2.5.0","message":{"role":"user","content":"please fix the build"}}
{"type":"user","uuid":"u2","isMeta":true,"timestamp":"2026-10-24T10:00:00.100Z","message":{"role":"user","content":"meta caveat"}}
this is not json
{"type":"api-request-blob","sessionId":"s1","hash":"abc"}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01.000Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"Let me look."}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20,"output_tokens_details":{"thinking_tokens":3}}}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:02.000Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go build ./...","description":"Build"}}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:03.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"exit status 1","is_error":true}]},"toolUseResult":"Error: exit status 1"}
{"type":"assistant","uuid":"a3","timestamp":"2026-10-24T10:00:04.000Z","message":{"id":"m2","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"t2","name":"Skill","input":{"skill":"superpowers:brainstorming","args":"x"}},{"type":"tool_use","id":"t3","name":"mcp__github__get_issue","input":{"repo":"a/b","number":1}},{"type":"tool_use","id":"t4","name":"Agent","input":{"description":"Explore code","subagent_type":"Explore","prompt":"look"}}],"usage":{"input_tokens":1,"output_tokens":2}}}
{"type":"user","uuid":"r2","timestamp":"2026-10-24T10:00:05.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t4","content":[{"type":"text","text":"launched"}]}]},"toolUseResult":{"status":"async_launched","agentId":"abc123"}}
{"type":"system","subtype":"local_command","timestamp":"2026-10-24T10:00:06.000Z","content":"<command-name>/mcp</command-name>\n<command-message>mcp</command-message>\n<command-args></command-args>"}
{"type":"user","uuid":"u3","timestamp":"2026-10-24T10:00:07.000Z","message":{"role":"user","content":[{"type":"text","text":"thanks<system-reminder>ignore me</system-reminder>"}]}}
{"type":"cost-state","sessionId":"s1","totalCostUSD":1.25}
{"type":"assistant","uuid":"a4","timestamp":"2026-10-24T10:00:08.000Z","message":{"id":"m3","model":"claude-opus-5-5","content":[{"type":"text","text":"Done."}]}}
{"type":"user","uuid":"partial","message":{"role":"user","content":"half writ`

func TestParse(t *testing.T) {
	s := NewSession()
	n, err := s.Read(strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(strings.LastIndex(fixture, "\n") + 1); n != want {
		t.Errorf("consumed %d bytes, want %d (partial line must be left)", n, want)
	}

	if s.ID != "s1" || s.Title != "Fix the build" || s.CWD != "/repo" || s.Branch != "main" || s.Version != "2.5.0" {
		t.Errorf("metadata = %q %q %q %q %q", s.ID, s.Title, s.CWD, s.Branch, s.Version)
	}
	if s.CostUSD != 1.25 {
		t.Errorf("cost = %v", s.CostUSD)
	}
	if s.Requests != 3 {
		t.Errorf("requests = %d, want 3 (m1 counted once)", s.Requests)
	}
	if want := (Usage{Input: 11, Output: 7, CacheRead: 100, CacheCreate: 20, Thinking: 3}); s.Usage != want {
		t.Errorf("usage = %+v, want %+v", s.Usage, want)
	}

	var kinds []Kind
	for _, e := range s.Events {
		kinds = append(kinds, e.Kind)
	}
	want := []Kind{KindUser, KindThinking, KindTool, KindTool, KindTool, KindTool, KindSystem, KindUser, KindText}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}

	bash := s.Tool("t1")
	if bash.Kind != ToolBuiltin || bash.Summary != "go build ./..." || !bash.HasResult || !bash.IsError || bash.Result != "exit status 1" {
		t.Errorf("bash = %+v", bash)
	}
	if sk := s.Tool("t2"); sk.Kind != ToolSkill || sk.Summary != "superpowers:brainstorming x" || sk.HasResult {
		t.Errorf("skill = %+v", sk)
	}
	mcp := s.Tool("t3")
	if srv, tool := mcp.MCPParts(); mcp.Kind != ToolMCP || srv != "github" || tool != "get_issue" || mcp.Summary != "repo=a/b" {
		t.Errorf("mcp = %+v (%s/%s)", mcp, srv, tool)
	}
	if ag := s.Tool("t4"); ag.Kind != ToolAgent || ag.AgentID != "abc123" || ag.Summary != "[Explore] Explore code" || ag.Result != "launched" {
		t.Errorf("agent = %+v", ag)
	}
	if got := s.Events[6].Text; got != "/mcp" {
		t.Errorf("system text = %q", got)
	}
	if got := s.Events[7].Text; got != "thanks" {
		t.Errorf("reminder not stripped: %q", got)
	}
}

func TestFeedIncremental(t *testing.T) {
	lines := strings.Split(fixture, "\n")
	s := NewSession()
	total := 0
	for _, l := range lines[:len(lines)-1] {
		total += len(s.Feed([]byte(l)))
	}
	if total != len(s.Events) || total != 9 {
		t.Errorf("incremental events = %d, session has %d", total, len(s.Events))
	}
}

func TestCleanMarkup(t *testing.T) {
	cases := map[string]string{
		"<command-name>/review</command-name><command-args>42</command-args>": "/review 42",
		"<local-command-stdout>hello</local-command-stdout>":                  "hello",
		"<local-command-stdout></local-command-stdout>":                       "",
		"plain <b>text</b>": "plain <b>text</b>",
	}
	for in, want := range cases {
		if got := CleanMarkup(in); got != want {
			t.Errorf("CleanMarkup(%q) = %q, want %q", in, got, want)
		}
	}
}

// Claude Code writes one record per content block and repeats the final
// usage on each (verified across 5,147 multi-record messages in real
// transcripts), so usage must be counted once per message id.
func TestUsageCountedOncePerMessage(t *testing.T) {
	rec := `{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":1,"output_tokens":950}}}` + "\n"
	s, _ := Parse(strings.NewReader(rec + rec + rec))
	if s.Usage.Output != 950 || s.Requests != 1 {
		t.Errorf("output = %d, requests = %d; want 950, 1", s.Usage.Output, s.Requests)
	}
}

func TestSkippable(t *testing.T) {
	for line, want := range map[string]bool{
		`{"type":"api-request-blob","hash":"x"}`:   true,
		`{"type":"attachment","attachment":{}}`:    true,
		`{"type":"file-history-snapshot"}`:         true,
		`{"type":"assistant","message":{}}`:        false,
		`{"type":"user","message":{"content":""}}`: false,
	} {
		if got := Skippable([]byte(line)); got != want {
			t.Errorf("Skippable(%s) = %v", line, got)
		}
	}
}

func TestUserText(t *testing.T) {
	cases := []struct {
		in, text string
		kind     Kind
	}{
		{"<command-name>/login</command-name><command-message>login</command-message><command-args></command-args>", "/login", KindUser},
		{"<local-command-stdout>Login successful</local-command-stdout>", "Login successful", KindSystem},
		{"<task-notification>\n<task-id>a1</task-id>\n<status>completed</status>\n<summary>Agent \"review\" finished</summary>\n<result>lots of text</result>\n</task-notification>", `Background task completed: Agent "review" finished`, KindSystem},
		{"[Request interrupted by user for tool use]", "[Request interrupted by user for tool use]", KindSystem},
		{`<div class="x">why is this red?</div>`, `<div class="x">why is this red?</div>`, KindUser},
		{"hi<system-reminder>injected</system-reminder>", "hi", KindUser},
	}
	for _, c := range cases {
		if text, kind := UserText(c.in); text != c.text || kind != c.kind {
			t.Errorf("UserText(%q) = %q, %v; want %q, %v", c.in, text, kind, c.text, c.kind)
		}
	}
}

func TestInjectedUserTextIsSystem(t *testing.T) {
	in := `{"type":"user","uuid":"u1","message":{"content":"<local-command-stdout>Login interrupted</local-command-stdout>"}}
{"type":"user","uuid":"u2","message":{"content":[{"type":"text","text":"<task-notification><status>completed</status><summary>done</summary></task-notification>"}]}}
`
	s, _ := Parse(strings.NewReader(in))
	for _, e := range s.Events {
		if e.Kind != KindSystem {
			t.Errorf("%q is %v, want KindSystem", e.Text, e.Kind)
		}
	}
	if got := PromptText([]byte(`"<task-notification><summary>x</summary></task-notification>"`)); got != "" {
		t.Errorf("PromptText of a notification = %q, want empty", got)
	}
}

func TestUserTextIgnoresQuotedTags(t *testing.T) {
	for _, in := range []string{
		"why does the log show <command-name>/clear</command-name> here?",
		"what is <local-command-stdout>ok</local-command-stdout> in my transcript?",
	} {
		if text, kind := UserText(in); text != in || kind != KindUser {
			t.Errorf("UserText(%q) = %q, %v; want the prompt unchanged", in, text, kind)
		}
	}
}

func TestFilePath(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"Read", `{"file_path":"/r/a.go"}`, "/r/a.go"},
		{"Edit", `{"file_path":"/r/b.go","old_string":"x","new_string":"y"}`, "/r/b.go"},
		{"MultiEdit", `{"file_path":"/r/c.go","edits":[]}`, "/r/c.go"},
		{"Write", `{"file_path":"/r/d.go","content":""}`, "/r/d.go"},
		{"NotebookEdit", `{"notebook_path":"/r/e.ipynb"}`, "/r/e.ipynb"},
		{"Bash", `{"command":"cat /r/f.go","file_path":"/r/f.go"}`, ""},
	} {
		c := ToolCall{Name: tc.name, Input: []byte(tc.input)}
		if got := c.FilePath(); got != tc.want {
			t.Errorf("%s FilePath() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEdits(t *testing.T) {
	edit := &ToolCall{Name: "Edit", Input: []byte(`{"file_path":"a.go","old_string":"","new_string":"x\n"}`)}
	if got := edit.Edits(); len(got) != 1 || got[0] != (Edit{"", "x\n"}) {
		t.Errorf("Edit = %+v", got)
	}
	multi := &ToolCall{Name: "MultiEdit", Input: []byte(`{"edits":[{"old_string":"a","new_string":"b"},{"old_string":"c","new_string":"d"}]}`)}
	if got := multi.Edits(); len(got) != 2 || got[1] != (Edit{"c", "d"}) {
		t.Errorf("MultiEdit = %+v", got)
	}
	if got := (&ToolCall{Name: "Bash", Input: []byte(`{"command":"ls"}`)}).Edits(); got != nil {
		t.Errorf("Bash = %+v", got)
	}
	if got := Lines(""); got != nil {
		t.Errorf(`Lines("") = %q`, got)
	}
	if got := Lines("a\nb\n"); len(got) != 2 {
		t.Errorf(`Lines("a\nb\n") = %q`, got)
	}
}
