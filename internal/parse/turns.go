package parse

import (
	"strings"
	"time"
)

// Turn is one prompt the person typed and everything up to the next one.
type Turn struct {
	Prompt     *Event
	Events     []*Event // the prompt and everything after it, in order
	Start, End time.Time
	Recorded   time.Duration // from turn_duration, once the turn is Done
	Done       bool          // Claude Code recorded the end of the turn
	Usage      Usage
	Requests   int
	Tools      int
	ContextEnd int // context size of the turn's last request
	Compacted  bool

	weight map[string]float64 // weighted tokens per model, for TurnCost
	est    float64            // estimated cost, for requests to models with prices
}

// Duration is the recorded turn time when known, else first to last event.
func (t *Turn) Duration() time.Duration {
	if t.Done && t.Recorded > 0 {
		return t.Recorded
	}
	if t.Start.IsZero() || t.End.Before(t.Start) {
		return 0
	}
	return t.End.Sub(t.Start)
}

// Compaction is a compact_boundary: the context was summarised.
type Compaction struct {
	Trigger string // "manual" or "auto"
	Pre     int    // context tokens before
	Post    int    // context of the next request, 0 until it arrives
}

// TurnOf returns the turn started by prompt event e, or nil.
func (s *Session) TurnOf(e *Event) *Turn { return s.turnByPrompt[e] }

func (s *Session) cur() *Turn {
	if len(s.Turns) == 0 {
		return nil
	}
	return s.Turns[len(s.Turns)-1]
}

// track files a new event into the current turn, starting a turn on a
// prompt. Several text blocks of one message start only one turn.
func (s *Session) track(e *Event) {
	t := s.cur()
	if e.Kind == KindUser && (t == nil || e.UUID == "" || t.Prompt.UUID != e.UUID) {
		t = &Turn{Prompt: e, Events: []*Event{e}, Start: e.Time, End: e.Time, weight: map[string]float64{}}
		s.Turns = append(s.Turns, t)
		s.turnByPrompt[e] = t
		return
	}
	if t == nil {
		return
	}
	t.Events = append(t.Events, e)
	if e.Tool != nil {
		t.Tools++
	}
	if e.Kind == KindCompact {
		t.Compacted = true
	}
	t.touch(e.Time)
}

func (t *Turn) touch(ts time.Time) {
	if !ts.IsZero() && ts.After(t.End) {
		t.End = ts
	}
}

// Relative token prices, stable across Claude models: input 1, cache write
// 1.25, cache read 0.1, output 5.
func (u Usage) weight() float64 {
	return float64(u.Input) + 1.25*float64(u.CacheCreate) + 0.1*float64(u.CacheRead) + 5*float64(u.Output)
}

// Context is the prompt size of a request: everything sent as input.
func (u Usage) Context() int { return u.Input + u.CacheRead + u.CacheCreate }

func (u *Usage) add(o Usage) {
	u.Input += o.Input
	u.Output += o.Output
	u.CacheRead += o.CacheRead
	u.CacheCreate += o.CacheCreate
	u.CacheCreate1h += o.CacheCreate1h
	u.Thinking += o.Thinking
}

// request records one API request's usage, counted once per message id.
func (s *Session) request(model string, u Usage) {
	s.Usage.add(u)
	w := u.weight()
	s.weight[model] += w
	p, priced := priceOf(model)
	if priced {
		s.est[model] += u.usd(p)
	}
	if model != "" && model != "<synthetic>" {
		s.lastModel = model
	}
	ctx := u.Context()
	if ctx > 0 {
		s.Context = ctx
		s.PeakContext = max(s.PeakContext, ctx)
		if s.pending != nil {
			s.pending.Post = ctx
			s.pending = nil
		}
	}
	t := s.cur()
	if t == nil {
		return
	}
	t.Requests++
	t.Usage.add(u)
	t.weight[model] += w
	if priced {
		t.est += u.usd(p)
	}
	if ctx > 0 {
		t.ContextEnd = ctx
	}
}

// ContextWindow is the context limit of the session's model: as Claude Code
// reported it, else inferred from the model id or the context actually used.
func (s *Session) ContextWindow() int {
	switch {
	case s.ctxMax > 0:
		return s.ctxMax
	case s.windows[s.lastModel] > 0:
		return s.windows[s.lastModel]
	case s.oneM, strings.Contains(s.lastModel, "[1m]"), s.PeakContext > 200_000, s.prePeak > 200_000:
		return 1_000_000
	}
	return 200_000
}

// recorded reports whether Claude Code recorded the session's cost, which
// it does when the session ends.
func (s *Session) recorded() bool { return s.CostUSD > 0 || len(s.ModelCost) > 0 }

// Cost is the session's cost: as recorded, or while the session runs,
// estimated from its tokens. ok is false when neither is possible.
func (s *Session) Cost() (usd float64, estimated, ok bool) {
	if s.recorded() {
		if s.CostUSD > 0 {
			return s.CostUSD, false, true
		}
		for _, c := range s.ModelCost {
			usd += c
		}
		return usd, false, true
	}
	for _, c := range s.est {
		usd += c
	}
	return usd, true, len(s.est) > 0
}

// ModelCostUSD is one model's cost, recorded or estimated.
func (s *Session) ModelCostUSD(model string) float64 {
	if s.recorded() {
		return s.ModelCost[model]
	}
	return s.est[model]
}

// TurnCost estimates a turn's cost. With a recorded cost, it shares each
// model's cost across turns by weighted tokens; models seen only in
// cost-state (used by subagents) are shared by total weight. Without one,
// it prices the turn's requests. ok is false when neither is possible.
func (s *Session) TurnCost(t *Turn) (usd float64, ok bool) {
	if !s.recorded() {
		return t.est, len(s.est) > 0
	}
	var tw, sw float64
	for _, w := range t.weight {
		tw += w
	}
	for _, w := range s.weight {
		sw += w
	}
	if sw == 0 {
		return 0, true
	}
	if len(s.ModelCost) == 0 {
		return s.CostUSD * tw / sw, true
	}
	for m, cost := range s.ModelCost {
		if s.weight[m] > 0 {
			usd += cost * t.weight[m] / s.weight[m]
		} else {
			usd += cost * tw / sw
		}
	}
	return usd, true
}
