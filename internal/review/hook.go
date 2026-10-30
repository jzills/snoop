package review

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// hookInput is the part of Claude Code's hook input snoop needs.
type hookInput struct {
	SessionID string `json:"session_id"`
	Event     string `json:"hook_event_name"`

	// PermissionRequest only.
	Cwd       string          `json:"cwd"`
	AgentID   string          `json:"agent_id"`
	AgentType string          `json:"agent_type"`
	Tool      string          `json:"tool_name"`
	Input     json.RawMessage `json:"tool_input"`
}

// DefaultAskWait is how long the hook waits for snoop to answer a
// permission request. The terminal's own prompt shows meanwhile, and
// answering there ends the hook, so the wait delays nothing.
const DefaultAskWait = 5 * time.Minute

const (
	askPoll = 100 * time.Millisecond
	// After the deadline, how long the hook still waits for a request
	// snoop claimed just before it.
	askGrace = 2 * time.Second
)

// HookOptions are where the hook finds comments, requests and the
// listening file, and its clock.
type HookOptions struct {
	Inbox, Asks, Listening string
	AskWait                time.Duration
	Now                    func() time.Time
	Sleep                  func(time.Duration)
}

// Hook delivers a session's waiting comments in the shape the hook event
// expects, and on a permission request waits for snoop's answer while a
// snoop is listening. It prints nothing when there is nothing to say, and
// runs after every tool call, so it reads only its input and the inbox.
func Hook(in io.Reader, out io.Writer, o HookOptions) error {
	var h hookInput
	if err := json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&h); err != nil {
		return err
	}
	if h.SessionID == "" {
		return errors.New("hook input has no session_id")
	}
	var shape func(string) any
	switch h.Event {
	case "Stop":
		// Blocking the stop hands the comments to the agent as the reason
		// to keep going.
		shape = func(t string) any { return map[string]string{"decision": "block", "reason": t} }
	case "PostToolUse", "UserPromptSubmit":
		shape = func(t string) any {
			return map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": h.Event, "additionalContext": t}}
		}
	case "PermissionRequest":
		return ask(h, out, o)
	default:
		return nil
	}
	text, done, err := Claim(o.Inbox, h.SessionID, o.Now())
	if err != nil || text == "" {
		return err
	}
	if err := json.NewEncoder(out).Encode(shape(text)); err != nil {
		return err
	}
	done()
	return nil
}

// ask writes the request, waits for an answer and prints it. Without an
// answer by the deadline it prints nothing, so the prompt decides.
func ask(h hookInput, out io.Writer, o HookOptions) error {
	now := o.Now()
	if !Listening(o.Listening, now) {
		return nil
	}
	wait := o.AskWait
	if wait <= 0 {
		wait = DefaultAskWait
	}
	r, err := Ask(o.Asks, Request{Session: h.SessionID, Cwd: h.Cwd, AgentID: h.AgentID, AgentType: h.AgentType,
		Tool: h.Tool, Input: h.Input, Asked: now, Deadline: now.Add(wait)})
	if err != nil {
		return err
	}
	base := filepath.Join(o.Asks, h.SessionID, r.ID)
	deadline := r.Deadline
	for {
		if b, err := os.ReadFile(base + answerExt); err == nil {
			var a Answer
			if err := json.Unmarshal(b, &a); err != nil {
				return err
			}
			os.Remove(base + answerExt)
			os.Remove(base + claimedExt)
			return json.NewEncoder(out).Encode(map[string]any{"hookSpecificOutput": map[string]any{
				"hookEventName": "PermissionRequest", "decision": a}})
		}
		if !o.Now().Before(deadline) {
			// Give up, unless snoop claimed the request in time and is
			// writing its answer.
			if err := os.Remove(base + askExt); err == nil || !errors.Is(err, os.ErrNotExist) || deadline.After(r.Deadline) {
				os.Remove(base + claimedExt)
				return nil
			}
			deadline = deadline.Add(askGrace)
		}
		o.Sleep(askPoll)
	}
}
