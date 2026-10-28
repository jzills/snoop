package review

import (
	"encoding/json"
	"errors"
	"io"
	"time"
)

// hookInput is the part of Claude Code's hook input snoop needs.
type hookInput struct {
	SessionID string `json:"session_id"`
	Event     string `json:"hook_event_name"`
}

// Hook delivers a session's waiting comments in the shape the hook event
// expects. It prints nothing when nothing waits, and runs after every tool
// call, so it reads only its input and the inbox.
func Hook(in io.Reader, out io.Writer, dir string, now time.Time) error {
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
	default:
		return nil
	}
	text, done, err := Claim(dir, h.SessionID, now)
	if err != nil || text == "" {
		return err
	}
	if err := json.NewEncoder(out).Encode(shape(text)); err != nil {
		return err
	}
	done()
	return nil
}
