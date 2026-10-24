package stats

import (
	"strings"
	"testing"

	"github.com/jzills/snoop/internal/parse"
)

const fixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-24T10:00:00Z","message":{"content":"go"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"false"}},{"type":"tool_use","id":"t3","name":"Skill","input":{"skill":"review"}},{"type":"tool_use","id":"t4","name":"mcp__gh__pr","input":{}},{"type":"tool_use","id":"t5","name":"Agent","input":{"subagent_type":"Explore"}}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:03Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"},{"type":"tool_result","tool_use_id":"t2","content":"no","is_error":true}]}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:10Z","message":{"id":"m2","content":[{"type":"text","text":"done"}]}}
`

func TestCompute(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(fixture))
	st := Compute(s)
	if st.Prompts != 1 || st.Replies != 1 || st.ToolCalls != 5 || st.Errors != 1 {
		t.Errorf("counts = %+v", st)
	}
	if st.ByKind != [4]int{2, 1, 1, 1} {
		t.Errorf("by kind = %v", st.ByKind)
	}
	if st.Tools[0].Name != "Bash" || st.Tools[0].N != 2 || st.Tools[0].Errors != 1 || st.ToolTime.Seconds() != 4 {
		t.Errorf("tools[0] = %+v", st.Tools[0])
	}
	if st.Skills[0].Name != "review" || st.MCP[0].Name != "gh" || st.Agents[0].Name != "Explore" {
		t.Errorf("skills/mcp/agents = %v %v %v", st.Skills, st.MCP, st.Agents)
	}
	if st.Duration.Seconds() != 10 {
		t.Errorf("duration = %v", st.Duration)
	}
}

func TestPromptsCountMessagesNotBlocks(t *testing.T) {
	in := `{"type":"user","uuid":"u1","message":{"content":[{"type":"text","text":"part one"},{"type":"text","text":"part two"}]}}
{"type":"user","uuid":"u2","message":{"content":"<local-command-stdout>cost: $1</local-command-stdout>"}}
{"type":"user","uuid":"u3","message":{"content":"second prompt"}}
`
	s, _ := parse.Parse(strings.NewReader(in))
	if p := Compute(s).Prompts; p != 2 {
		t.Errorf("prompts = %d, want 2", p)
	}
}
