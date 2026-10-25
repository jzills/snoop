package summary

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
)

const fixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-23T10:00:00Z","message":{"content":"one"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-23T10:00:01Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"x"}},{"type":"tool_use","id":"t2","name":"Skill","input":{"skill":"review"}}],"usage":{"input_tokens":100}}}
{"type":"user","uuid":"r1","timestamp":"2026-10-23T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"no","is_error":true}]}}
{"type":"user","uuid":"u2","timestamp":"2026-10-24T10:00:00Z","message":{"content":"two"}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m2","model":"claude-opus-5-5","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":300}}}
{"type":"cost-state","totalCostUSD":2.0}
`

func TestOf(t *testing.T) {
	s, _ := parse.Parse(strings.NewReader(fixture))
	sum := Of(s)
	if sum.Prompts != 2 || sum.ToolCalls != 2 || sum.Errors != 1 || sum.Tokens != 400 || !sum.HasCost || sum.Cost != 2 {
		t.Errorf("summary = %+v", sum)
	}
	if sum.Tools["Bash"] != (Count{1, 1}) || sum.Skills["review"].N != 1 || sum.Models["claude-opus-5-5"] != 2 {
		t.Errorf("counts = %v %v %v", sum.Tools, sum.Skills, sum.Models)
	}
	d1 := sum.Days[time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC).Local().Format(time.DateOnly)]
	d2 := sum.Days[time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC).Local().Format(time.DateOnly)]
	if d1.Tokens != 100 || d2.Tokens != 300 || math.Abs(d1.Cost-0.5) > 1e-9 || math.Abs(d2.Cost-1.5) > 1e-9 {
		t.Errorf("days = %+v %+v", d1, d2)
	}
}

func TestTotal(t *testing.T) {
	now := time.Date(2026, 10, 24, 15, 0, 0, 0, time.Local)
	a := Session{Prompts: 2, ToolCalls: 4, Errors: 1, Tokens: 10, Cost: 1, HasCost: true,
		Tools: map[string]Count{"Bash": {3, 1}, "Read": {1, 0}}, Days: map[string]Day{"2026-10-24": {10, 1}}}
	b := Session{Prompts: 1, ToolCalls: 2, Tokens: 5, Tools: map[string]Count{"Read": {2, 0}},
		Days: map[string]Day{"2026-10-24": {5, 0}, "2026-01-01": {99, 9}}}
	tot := Total([]Session{a, b}, now, 30)
	if tot.Sessions != 2 || tot.Prompts != 3 || tot.ToolCalls != 6 || tot.Cost != 1 || tot.CostKnown != 1 {
		t.Errorf("totals = %+v", tot)
	}
	if tot.Tools[0] != (Named{"Bash", Count{3, 1}}) || tot.Tools[1] != (Named{"Read", Count{3, 0}}) {
		t.Errorf("tools = %v (ties break by name)", tot.Tools)
	}
	last := tot.Days[len(tot.Days)-1]
	if len(tot.Days) != 30 || last.Date.Format(time.DateOnly) != "2026-10-24" || last.Tokens != 15 {
		t.Errorf("days = %d, last = %+v (days outside the window are left out)", len(tot.Days), last)
	}
}

func TestScanUsesCache(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	os.WriteFile(p, []byte(fixture), 0o644)
	bad := filepath.Join(dir, "gone.jsonl")
	infos := []store.SessionInfo{store.Info(p), {Path: bad, Size: 1}}

	c := OpenCache(filepath.Join(dir, "cache"))
	var got []Result
	for r := range Scan(infos, c) {
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("results = %d", len(got))
	}
	for _, r := range got {
		if (r.Path == bad) != (r.Err != nil) {
			t.Errorf("%s err = %v", r.Path, r.Err)
		}
	}
	if err := c.Save(infos[:1]); err != nil {
		t.Fatal(err)
	}

	// A new cache reads the saved entry instead of the transcript.
	os.WriteFile(p, []byte("garbage that would parse to nothing\n"), 0o644)
	os.Chtimes(p, infos[0].ModTime, infos[0].ModTime)
	infos[0].Size = int64(len(fixture)) // as listed before the rewrite
	c2 := OpenCache(filepath.Join(dir, "cache"))
	for r := range Scan(infos[:1], c2) {
		if r.Sum.Prompts != 2 {
			t.Errorf("cached prompts = %d, want 2 (read from cache)", r.Sum.Prompts)
		}
	}
	if _, ok := c2.entries[bad]; ok {
		t.Error("deleted session kept in cache")
	}
}

const indexFixture = `{"type":"user","uuid":"u1","timestamp":"2026-10-23T10:00:00Z","message":{"content":"Fix the flaky parser test\nit fails on CI"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-23T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/src/api/internal/parse/parse.go"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"go test ./internal/parse/\necho done"}},{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"go test ./internal/parse/"}},{"type":"tool_use","id":"t4","name":"Agent","input":{"subagent_type":"Explore"}}]}}
{"type":"user","uuid":"r4","timestamp":"2026-10-23T10:00:09Z","toolUseResult":{"agentId":"a1b2"},"message":{"content":[{"type":"tool_result","tool_use_id":"t4","content":"done"}]}}
`

const subFixture = `{"type":"assistant","uuid":"s1","timestamp":"2026-10-23T10:00:03Z","message":{"id":"sm1","content":[{"type":"tool_use","id":"st1","name":"Edit","input":{"file_path":"/src/api/internal/parse/turns.go","old_string":"a","new_string":"b"}},{"type":"tool_use","id":"st2","name":"Bash","input":{"command":"gofmt -l ."}}]}}
`

func TestScanIndexesPromptsFilesAndCommands(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	os.WriteFile(p, []byte(indexFixture), 0o644)
	sub := store.SubagentPath(p, "a1b2")
	os.MkdirAll(filepath.Dir(sub), 0o755)
	os.WriteFile(sub, []byte(subFixture), 0o644)

	var r Result
	for r = range Scan([]store.SessionInfo{store.Info(p)}, OpenCache("")) {
	}
	idx := r.Sum.Find
	if idx == nil {
		t.Fatalf("no index: %+v", r)
	}
	if len(idx.Prompts) != 1 || idx.Prompts[0] != "Fix the flaky parser test" {
		t.Errorf("prompts = %q, want the first line of each", idx.Prompts)
	}
	if want := []string{"/src/api/internal/parse/parse.go", "/src/api/internal/parse/turns.go"}; strings.Join(idx.Files, ",") != strings.Join(want, ",") {
		t.Errorf("files = %q, want %q (a subagent's edit included)", idx.Files, want)
	}
	if want := []string{"go test ./internal/parse/", "gofmt -l ."}; strings.Join(idx.Commands, ",") != strings.Join(want, ",") {
		t.Errorf("commands = %q, want %q (first lines, once each)", idx.Commands, want)
	}
}

func TestOldCacheVersionIsDiscarded(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "summaries.json"), []byte(`{"version":1,"sessions":{"/x.jsonl":{"size":1,"prompts":3}}}`), 0o644)
	if c := OpenCache(dir); len(c.entries) != 0 {
		t.Errorf("entries from a version 1 cache: %v", c.entries)
	}
}

func TestFind(t *testing.T) {
	sums := map[string]Session{
		"/a.jsonl": {Find: &Index{
			Prompts:  []string{"rename the parser package"},
			Files:    []string{"/src/api/internal/parse/parse.go", "/src/api/internal/parse/turns.go"},
			Commands: []string{"go test ./internal/parse/"},
		}},
		"/b.jsonl": {Find: &Index{Prompts: []string{"write the changelog"}, Commands: []string{"git log"}}},
		"/c.jsonl": {}, // not indexed yet
	}
	paths := []string{"/c.jsonl", "/b.jsonl", "/a.jsonl"}

	hits := Find(sums, paths, "PARSE go")
	if len(hits) != 1 || hits[0].Path != "/a.jsonl" {
		t.Fatalf("hits = %+v, want /a.jsonl", hits)
	}
	// Files come first; every matching entry counts.
	if h := hits[0]; h.Kind != HitFile || h.Text != "/src/api/internal/parse/parse.go" || h.N != 3 {
		t.Errorf("hit = %+v, want parse.go of 3 matches", h)
	}
	// Words match within one entry, not across a session.
	if hits := Find(sums, paths, "changelog git"); len(hits) != 0 {
		t.Errorf("words split across entries matched: %+v", hits)
	}
	if hits := Find(sums, paths, "log"); len(hits) != 1 || hits[0].Kind != HitCommand || hits[0].Text != "git log" {
		t.Errorf("hits = %+v, want b's command before its prompt", hits)
	}
	// Order and scope are the caller's.
	if hits := Find(sums, []string{"/b.jsonl"}, "parse"); len(hits) != 0 {
		t.Errorf("a path not passed matched: %+v", hits)
	}
	if hits := Find(sums, paths, "  "); len(hits) != 0 {
		t.Errorf("a blank query matched: %+v", hits)
	}
}
