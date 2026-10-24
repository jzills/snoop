// Package stats aggregates a parsed session into counts for the stats view.
package stats

import (
	"sort"
	"time"

	"github.com/jzills/snoop/internal/parse"
)

type Count struct {
	Name   string
	N      int
	Errors int
}

type Stats struct {
	Prompts   int
	Replies   int
	ToolCalls int
	Errors    int
	ByKind    [4]int // indexed by parse.ToolKind
	Duration  time.Duration
	ToolTime  time.Duration

	Tools  []Count
	Skills []Count
	MCP    []Count // by server
	Agents []Count // by subagent type
}

func Compute(s *parse.Session) Stats {
	var st Stats
	tools, skills, mcp, agents := counter{}, counter{}, counter{}, counter{}
	prompted := map[string]bool{}
	for _, e := range s.Events {
		switch e.Kind {
		case parse.KindUser:
			// One message can hold several text blocks; count it once.
			if e.UUID == "" || !prompted[e.UUID] {
				prompted[e.UUID] = true
				st.Prompts++
			}
		case parse.KindText:
			st.Replies++
		case parse.KindTool:
			t := e.Tool
			st.ToolCalls++
			st.ByKind[t.Kind]++
			st.ToolTime += e.Duration()
			if t.IsError {
				st.Errors++
			}
			tools.add(t.Name, t.IsError)
			switch t.Kind {
			case parse.ToolSkill:
				skills.add(t.Group(), t.IsError)
			case parse.ToolMCP:
				srv, _ := t.MCPParts()
				mcp.add(srv, t.IsError)
			case parse.ToolAgent:
				agents.add(t.Group(), t.IsError)
			}
		}
	}
	if !s.Start.IsZero() {
		st.Duration = s.End.Sub(s.Start)
	}
	st.Tools, st.Skills, st.MCP, st.Agents = tools.sorted(), skills.sorted(), mcp.sorted(), agents.sorted()
	return st
}

type counter map[string]*Count

func (c counter) add(name string, isErr bool) {
	n := c[name]
	if n == nil {
		n = &Count{Name: name}
		c[name] = n
	}
	n.N++
	if isErr {
		n.Errors++
	}
}

func (c counter) sorted() []Count {
	out := make([]Count, 0, len(c))
	for _, v := range c {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Name < out[j].Name
	})
	return out
}
