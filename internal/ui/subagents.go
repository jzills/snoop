package ui

import (
	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/store"
)

// Subagent transcripts are loaded on demand: when expanded inline in the
// timeline, searched, or listed in the Files tab. Each is read once, then
// only the lines added since.

// Deepest subagent nesting shown inline.
const maxSubDepth = 4

// sub returns subagent id's transcript, reading any lines added since the
// last call; changed reports whether there were any.
func (v *view) sub(id string) (f *store.Follower, changed bool) {
	if f = v.subs[id]; f != nil {
		changed, _ = f.Poll()
		return f, changed
	}
	meta, ok := v.subagent(id)
	if !ok {
		return nil, false
	}
	f, err := store.Track(meta.Path)
	if err != nil {
		return nil, false
	}
	v.subs[id] = f
	return f, true
}

func agentID(e *parse.Event) string {
	if e.Tool == nil {
		return ""
	}
	return e.Tool.AgentID
}

// pollSubs reads new lines of expanded subagents, which grow while the
// session's own transcript is quiet, and reports whether any arrived.
func (v *view) pollSubs() bool {
	changed := false
	for id, open := range v.expanded {
		if open {
			if _, c := v.sub(id); c {
				changed = true
			}
		}
	}
	return changed
}

// toggleAll expands every subagent call in the timeline, nested ones
// included, or collapses them all when none is collapsed.
func (v *view) toggleAll() {
	collapsed := false
	for _, e := range v.shown {
		if id := agentID(e); id != "" && !v.expanded[id] {
			collapsed = true
		}
	}
	if !collapsed {
		v.expanded = map[string]bool{}
		v.refresh()
		return
	}
	// Expanding reveals nested calls; repeat until nothing new appears.
	for range maxSubDepth {
		grew := false
		for _, e := range v.shown {
			if id := agentID(e); id != "" && !v.expanded[id] {
				v.expanded[id] = true
				grew = true
			}
		}
		v.refresh()
		if !grew {
			return
		}
	}
}
