// Package parse turns Claude Code session JSONL into a list of display events.
//
// The transcript format is undocumented and changes between Claude Code
// versions, so the parser is deliberately lenient: unknown record types and
// fields are ignored and malformed lines are skipped.
package parse

import (
	"bufio"
	"bytes"
	json "github.com/goccy/go-json"
	"io"
	"strings"
	"time"
)

type Kind int

const (
	KindUser Kind = iota
	KindText
	KindThinking
	KindTool
	KindSystem
	KindCompact
)

type ToolKind int

const (
	ToolBuiltin ToolKind = iota
	ToolSkill
	ToolMCP
	ToolAgent
)

func (k ToolKind) String() string {
	return [...]string{"builtin", "skill", "mcp", "agent"}[k]
}

type ToolCall struct {
	ID      string
	Name    string
	Kind    ToolKind
	Input   json.RawMessage
	Summary string // one-line description of the input

	HasResult  bool
	Result     string
	IsError    bool
	ResultTime time.Time

	// AgentID links an Agent (or forked Skill) call to subagents/agent-<id>.jsonl.
	AgentID string

	group string // see Group; computed once when the call is parsed
}

// Group names what a call is grouped under within its kind: the skill name,
// the MCP server·tool, the subagent type, or the tool name.
func (t *ToolCall) Group() string {
	if t.group == "" {
		t.group = t.groupName()
	}
	return t.group
}

func (t *ToolCall) groupName() string {
	switch t.Kind {
	case ToolSkill:
		return t.inputString("skill", "?")
	case ToolMCP:
		server, tool := t.MCPParts()
		return server + "·" + tool
	case ToolAgent:
		return t.inputString("subagent_type", "general-purpose")
	}
	return t.Name
}

// InputString is a string field of the call's input, or "".
func (t *ToolCall) InputString(key string) string { return t.inputString(key, "") }

func (t *ToolCall) inputString(key, def string) string {
	var in map[string]any
	json.Unmarshal(t.Input, &in)
	if s, ok := in[key].(string); ok && s != "" {
		return s
	}
	return def
}

// Edit is one replacement an Edit or MultiEdit call makes.
type Edit struct {
	Old string `json:"old_string"`
	New string `json:"new_string"`
}

// Edits is the replacements an Edit or MultiEdit call makes, or nil for
// any other call.
func (t *ToolCall) Edits() []Edit {
	switch t.Name {
	case "Edit":
		var e Edit
		json.Unmarshal(t.Input, &e)
		return []Edit{e}
	case "MultiEdit":
		var in struct {
			Edits []Edit `json:"edits"`
		}
		json.Unmarshal(t.Input, &in)
		return in.Edits
	}
	return nil
}

// Lines splits text into lines, ignoring a final newline: "" has none.
func Lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// FilePath is the file a built-in file tool read, edited or wrote, or "".
func (t *ToolCall) FilePath() string {
	switch t.Name {
	case "Read", "Write", "Edit", "MultiEdit":
		return t.inputString("file_path", "")
	case "NotebookEdit":
		return t.inputString("notebook_path", "")
	}
	return ""
}

// Server and tool name for an mcp__<server>__<tool> call.
func (t *ToolCall) MCPParts() (server, tool string) {
	parts := strings.SplitN(strings.TrimPrefix(t.Name, "mcp__"), "__", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

type Event struct {
	Kind  Kind
	UUID  string
	Time  time.Time
	Text  string
	Model string
	Tool  *ToolCall

	Ctx     int         // context size of the request (assistant events)
	Compact *Compaction // KindCompact events
}

// Duration is how long a tool call took from call to result, or 0 when it
// has no result yet or either timestamp is missing.
func (e *Event) Duration() time.Duration {
	t := e.Tool
	if t == nil || !t.HasResult || t.ResultTime.IsZero() || e.Time.IsZero() {
		return 0
	}
	return t.ResultTime.Sub(e.Time)
}

type Usage struct {
	Input, Output, CacheRead, CacheCreate, Thinking int
}

func (u Usage) Total() int { return u.Input + u.Output + u.CacheRead + u.CacheCreate }

type Session struct {
	ID      string
	Title   string
	CWD     string
	Branch  string
	Version string
	Start   time.Time
	End     time.Time

	Events []*Event
	Usage  Usage
	Models map[string]int // assistant messages per model

	CostUSD   float64            // from cost-state, when present
	ModelCost map[string]float64 // cost per model, from cost-state
	PRURL     string
	Requests  int // distinct assistant API messages

	Turns       []*Turn
	Context     int // context size of the latest request
	PeakContext int
	Compactions int

	tools        map[string]*ToolCall
	messages     map[string]int // message id -> context size of the request
	turnByPrompt map[*Event]*Turn
	weight       map[string]float64 // weighted tokens per model
	pending      *Compaction        // waiting for the next request's context
	lastModel    string
	ctxMax       int            // context window Claude Code reported
	windows      map[string]int // context window per model, from usage reports
	oneM         bool           // a 1M-context model was requested
	prePeak      int            // largest context a compaction reported
}

func NewSession() *Session {
	return &Session{
		Models:       map[string]int{},
		tools:        map[string]*ToolCall{},
		messages:     map[string]int{},
		turnByPrompt: map[*Event]*Turn{},
		weight:       map[string]float64{},
		windows:      map[string]int{},
	}
}

// Tool returns the call with the given tool_use id, if seen.
func (s *Session) Tool(id string) *ToolCall { return s.tools[id] }

// Parse reads a whole transcript.
func Parse(r io.Reader) (*Session, error) {
	s := NewSession()
	_, err := s.Read(r)
	return s, err
}

// Read feeds every complete line from r and returns the number of bytes
// consumed. A trailing partial line (a file still being written) is left
// unconsumed so a tailer can resume from the returned offset.
func (s *Session) Read(r io.Reader) (int64, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	var n int64
	for {
		line, err := br.ReadBytes('\n')
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		n += int64(len(line))
		s.Feed(line)
	}
}

type rawRecord struct {
	Type          string          `json:"type"`
	UUID          string          `json:"uuid"`
	Timestamp     string          `json:"timestamp"`
	SessionID     string          `json:"sessionId"`
	CWD           string          `json:"cwd"`
	GitBranch     string          `json:"gitBranch"`
	Version       string          `json:"version"`
	IsMeta        bool            `json:"isMeta"`
	Subtype       string          `json:"subtype"`
	Content       json.RawMessage `json:"content"`
	Message       *rawMessage     `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
	AITitle       string          `json:"aiTitle"`
	TotalCostUSD  *float64        `json:"totalCostUSD"`
	PRURL         string          `json:"prUrl"`

	IsCompactSummary bool `json:"isCompactSummary"`
	DurationMs       int  `json:"durationMs"`
	CompactMetadata  *struct {
		Trigger   string `json:"trigger"`
		PreTokens int    `json:"preTokens"`
	} `json:"compactMetadata"`
	ModelUsage map[string]struct {
		CostUSD float64 `json:"costUSD"`
	} `json:"modelUsage"`
	ContextUsage *struct {
		RawMax int `json:"raw_max_tokens"`
	} `json:"contextUsage"`
	UsageReport *struct {
		Session struct {
			ModelUsage map[string]struct {
				ContextWindow int `json:"contextWindow"`
			} `json:"model_usage"`
		} `json:"session"`
	} `json:"usageReport"`
	RequestedModel string `json:"requestedModel"`
}

type rawMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *rawUsage       `json:"usage"`
}

type rawUsage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
	OutputDetails       struct {
		ThinkingTokens int `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
}

type rawBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// Feed parses one JSONL line and returns the events it added.
func (s *Session) Feed(line []byte) []*Event {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil
	}
	if Skippable(line) {
		return nil
	}
	var rec rawRecord
	if json.Unmarshal(line, &rec) != nil {
		return nil
	}
	ts, _ := time.Parse(time.RFC3339Nano, rec.Timestamp)
	if !ts.IsZero() {
		if s.Start.IsZero() || ts.Before(s.Start) {
			s.Start = ts
		}
		if ts.After(s.End) {
			s.End = ts
		}
	}
	if s.ID == "" && rec.SessionID != "" {
		s.ID = rec.SessionID
	}
	if rec.CWD != "" {
		s.CWD = rec.CWD
	}
	if rec.GitBranch != "" {
		s.Branch = rec.GitBranch
	}
	if rec.Version != "" {
		s.Version = rec.Version
	}
	s.noteWindow(&rec)

	switch rec.Type {
	case "ai-title":
		if rec.AITitle != "" {
			s.Title = rec.AITitle
		}
	case "cost-state":
		if rec.TotalCostUSD != nil {
			s.CostUSD = *rec.TotalCostUSD
		}
		if len(rec.ModelUsage) > 0 {
			s.ModelCost = map[string]float64{}
			for m, u := range rec.ModelUsage {
				s.ModelCost[m] = u.CostUSD
			}
		}
	case "pr-link":
		s.PRURL = rec.PRURL
	case "system":
		switch rec.Subtype {
		case "compact_boundary":
			return s.feedCompact(&rec, ts)
		case "turn_duration":
			if t := s.cur(); t != nil {
				t.Done = true
				t.Recorded = time.Duration(rec.DurationMs) * time.Millisecond
				t.touch(ts)
			}
			return nil
		}
		return s.feedSystem(&rec, ts)
	case "user":
		if rec.Message != nil && !rec.IsMeta {
			out := s.feedUser(&rec, ts)
			if t := s.cur(); t != nil {
				t.touch(ts) // tool results
			}
			return out
		}
	case "assistant":
		if rec.Message != nil {
			return s.feedAssistant(&rec, ts)
		}
	}
	return nil
}

// noteWindow records what a record says about the context window size.
func (s *Session) noteWindow(rec *rawRecord) {
	if c := rec.ContextUsage; c != nil && c.RawMax > 0 {
		s.ctxMax = c.RawMax
	}
	if r := rec.UsageReport; r != nil {
		for m, u := range r.Session.ModelUsage {
			if u.ContextWindow > 0 {
				s.windows[m] = u.ContextWindow
			}
		}
	}
	if strings.Contains(rec.RequestedModel, "[1m]") {
		s.oneM = true
	}
}

func (s *Session) feedCompact(rec *rawRecord, ts time.Time) []*Event {
	c := &Compaction{}
	if m := rec.CompactMetadata; m != nil {
		c.Trigger, c.Pre = m.Trigger, m.PreTokens
		s.prePeak = max(s.prePeak, m.PreTokens)
	}
	s.Compactions++
	s.pending = c
	return []*Event{s.add(&Event{Kind: KindCompact, UUID: rec.UUID, Time: ts, Text: "compacted", Compact: c})}
}

// Record types that carry nothing we display, often large (full API request
// payloads). They are recognised from the line prefix and never decoded.
var skipTypes = [][]byte{
	[]byte(`"type":"api-request`),
	[]byte(`"type":"attachment"`),
	[]byte(`"type":"file-history-`),
}

// Skippable reports whether a line is a record type with nothing to display.
func Skippable(line []byte) bool {
	head := line[:min(len(line), 256)]
	for _, t := range skipTypes {
		if bytes.Contains(head, t) {
			return true
		}
	}
	return false
}

func (s *Session) add(e *Event) *Event {
	s.Events = append(s.Events, e)
	s.track(e)
	return e
}

func (s *Session) feedSystem(rec *rawRecord, ts time.Time) []*Event {
	var text string
	if json.Unmarshal(rec.Content, &text) != nil {
		return nil
	}
	text = CleanMarkup(text)
	if text == "" {
		return nil
	}
	return []*Event{s.add(&Event{Kind: KindSystem, UUID: rec.UUID, Time: ts, Text: text})}
}

// PromptText returns what the person typed in a user message, whose content
// is either a string or a list of blocks. Tool results and text Claude Code
// injected (command output, notifications) are ignored.
func PromptText(raw json.RawMessage) string {
	var str string
	if json.Unmarshal(raw, &str) == nil {
		if t, k := UserText(str); k == KindUser {
			return t
		}
		return ""
	}
	var blocks []rawBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			if t, k := UserText(b.Text); k == KindUser && t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func (s *Session) feedUser(rec *rawRecord, ts time.Time) []*Event {
	var str string
	if json.Unmarshal(rec.Message.Content, &str) == nil {
		t, k := UserText(str)
		if rec.IsCompactSummary {
			k = KindSystem // the summary Claude Code writes after compacting
		}
		if t == "" {
			return nil
		}
		return []*Event{s.add(&Event{Kind: k, UUID: rec.UUID, Time: ts, Text: t})}
	}
	var blocks []rawBlock
	if json.Unmarshal(rec.Message.Content, &blocks) != nil {
		return nil
	}
	var out []*Event
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if t, k := UserText(b.Text); t != "" {
				out = append(out, s.add(&Event{Kind: k, UUID: rec.UUID, Time: ts, Text: t}))
			}
		case "tool_result":
			t := s.tools[b.ToolUseID]
			if t == nil {
				continue
			}
			t.HasResult = true
			t.IsError = b.IsError
			t.Result = resultText(b.Content)
			t.ResultTime = ts
			if id := agentID(rec.ToolUseResult); id != "" {
				t.AgentID = id
			}
		}
	}
	return out
}

func (s *Session) feedAssistant(rec *rawRecord, ts time.Time) []*Event {
	m := rec.Message
	// Claude Code writes one record per content block, repeating the message
	// id and usage, so count usage once per message id.
	ctx, seen := s.messages[m.ID]
	if m.ID == "" || !seen {
		s.Requests++
		if m.Model != "" && m.Model != "<synthetic>" {
			s.Models[m.Model]++
		}
		var u Usage
		if r := m.Usage; r != nil {
			u = Usage{Input: r.InputTokens, Output: r.OutputTokens, CacheRead: r.CacheReadTokens,
				CacheCreate: r.CacheCreationTokens, Thinking: r.OutputDetails.ThinkingTokens}
		}
		s.request(m.Model, u)
		ctx = u.Context()
		if m.ID != "" {
			s.messages[m.ID] = ctx
		}
	}
	var blocks []rawBlock
	if json.Unmarshal(m.Content, &blocks) != nil {
		return nil
	}
	var out []*Event
	for _, b := range blocks {
		e := &Event{UUID: rec.UUID, Time: ts, Model: m.Model, Ctx: ctx}
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) == "" {
				continue
			}
			e.Kind, e.Text = KindText, strings.TrimSpace(b.Text)
		case "thinking":
			if strings.TrimSpace(b.Thinking) == "" {
				continue
			}
			e.Kind, e.Text = KindThinking, strings.TrimSpace(b.Thinking)
		case "tool_use":
			t := &ToolCall{ID: b.ID, Name: b.Name, Kind: Classify(b.Name), Input: b.Input}
			t.Summary = summarize(t)
			t.group = t.groupName()
			if s.CWD != "" {
				t.Summary = strings.ReplaceAll(t.Summary, s.CWD+"/", "")
			}
			s.tools[b.ID] = t
			e.Kind, e.Tool = KindTool, t
		default:
			continue
		}
		out = append(out, s.add(e))
	}
	return out
}

// Classify reports what kind of tool a tool name refers to.
func Classify(name string) ToolKind {
	switch {
	case name == "Skill":
		return ToolSkill
	case name == "Agent" || name == "Task":
		return ToolAgent
	case strings.HasPrefix(name, "mcp__"):
		return ToolMCP
	}
	return ToolBuiltin
}

func agentID(raw json.RawMessage) string {
	if len(raw) == 0 || raw[0] != '{' {
		return ""
	}
	var r struct {
		AgentID string `json:"agentId"`
	}
	json.Unmarshal(raw, &r)
	return r.AgentID
}

// resultText flattens tool_result content, which is either a string or a
// list of blocks.
func resultText(raw json.RawMessage) string {
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	var blocks []rawBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "image":
			parts = append(parts, "[image]")
		default:
			parts = append(parts, "["+b.Type+"]")
		}
	}
	return strings.Join(parts, "\n")
}
