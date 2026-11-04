package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// spend is an Opus 5.5 reply with n million input tokens: $4 each.
func spend(id string, n int) []byte {
	return []byte(fmt.Sprintf(`{"type":"assistant","uuid":"%s","timestamp":"2026-10-25T10:00:06Z","message":{"id":"%s","model":"claude-opus-5-5","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":%d}}}`, id, id, n*1_000_000))
}

func TestCostAlert(t *testing.T) {
	s := sessOf(bPrompt)
	s.Feed(spend("c1", 2)) // $8, as the session opened
	w := newWatch("")
	lim := Limits{Cost: 5}
	if as := w.limits(s, bStart, lim, false); len(as) != 0 {
		t.Errorf("alerted for the cost the session opened with: %+v", as)
	}
	s.Feed(spend("c2", 1)) // $12
	as := w.limits(s, bStart, lim, false)
	if len(as) != 1 || as[0].kind != alertCost || as[0].text != "$ cost passed about $10.00" || as[0].note != "Cost passed about $10.00" {
		t.Fatalf("alerts = %+v", as)
	}
	if as := w.limits(s, bStart, lim, false); len(as) != 0 {
		t.Errorf("alerted twice: %+v", as)
	}
	s.Feed(spend("c3", 3)) // $24: two multiples at once, one alert
	if as := w.limits(s, bStart, lim, false); len(as) != 1 || !strings.HasSuffix(as[0].text, "$20.00") {
		t.Errorf("alerts = %+v", as)
	}
	if as := w.limits(s, bStart, Limits{}, false); len(as) != 0 {
		t.Errorf("alerted with no limit: %+v", as)
	}
}

func TestLongTurnAlert(t *testing.T) {
	lim := Limits{LongTurn: 15 * time.Minute}
	s := sessOf(bPrompt, bBash)
	w := newWatch("")
	if as := w.limits(s, bStart.Add(time.Minute), lim, false); len(as) != 0 {
		t.Errorf("alerted early: %+v", as)
	}
	// A tool can run for minutes without writing anything.
	as := w.limits(s, bStart.Add(16*time.Minute), lim, false)
	if len(as) != 1 || as[0].kind != alertLong || as[0].text != "◌ turn running 16m00s" || as[0].ev == nil {
		t.Fatalf("alerts = %+v", as)
	}
	if as := w.limits(s, bStart.Add(20*time.Minute), lim, false); len(as) != 0 {
		t.Errorf("alerted twice for one turn: %+v", as)
	}
}

func TestLongTurnNotAlerted(t *testing.T) {
	lim := Limits{LongTurn: 15 * time.Minute}
	cases := map[string]func(w *watch) []*alert{
		// Opened past the limit: a session left running, or one that died.
		"already long when opened": func(w *watch) []*alert {
			return w.limits(sessOf(bPrompt, bBash), bStart.Add(time.Hour), lim, false)
		},
		"ended": func(w *watch) []*alert {
			s := sessOf(bPrompt, bBash, bBashDone, bTurnEnd)
			w.limits(s, bStart, lim, false)
			return w.limits(s, bStart.Add(time.Hour), lim, false)
		},
		"interrupted": func(w *watch) []*alert {
			s := sessOf(bPrompt, bBash, bInterrupt)
			w.limits(s, bStart, lim, false)
			return w.limits(s, bStart.Add(time.Hour), lim, false)
		},
		"waiting for a reply": func(w *watch) []*alert {
			s := sessOf(bPrompt, bBash, bBashDone)
			w.limits(s, bStart, lim, true)
			return w.limits(s, bStart.Add(time.Hour), lim, true)
		},
		// Closed mid-turn: nothing written for staleAfter and no call
		// running.
		"closed mid-turn": func(w *watch) []*alert {
			s := sessOf(bPrompt, bBash, bBashDone)
			w.limits(s, bStart, lim, false)
			return w.limits(s, bStart.Add(time.Hour), lim, false)
		},
	}
	for name, run := range cases {
		w := newWatch("")
		if as := run(&w); len(as) != 0 {
			t.Errorf("%s: alerts = %+v", name, as)
		}
	}
}

// A reply sets a waiting turn going again, and the long-turn limit counts
// from then.
func TestLongTurnCountsFromAReply(t *testing.T) {
	lim := Limits{LongTurn: 15 * time.Minute}
	s := sessOf(bPrompt, bBash)
	w := newWatch("")
	w.limits(s, bStart, lim, false)
	w.limits(s, bStart.Add(10*time.Minute), lim, true) // waiting at 10m
	if as := w.limits(s, bStart.Add(20*time.Minute), lim, false); len(as) != 0 {
		t.Errorf("counted from the turn's start: %+v", as)
	}
	if as := w.limits(s, bStart.Add(26*time.Minute), lim, false); len(as) != 1 || as[0].text != "◌ turn running 16m00s" {
		t.Errorf("alerts = %+v", as)
	}
}

// The board checks the limits on every poll, but not against what a
// session had reached when the board opened.
func TestBoardSkipsLimitsReachedBeforeItOpened(t *testing.T) {
	b, _ := newTestBoard(t)
	never := func(string) bool { return false }
	lim := Limits{LongTurn: time.Minute}
	b.poll(time.Now(), 0, lim, never)
	// busy.jsonl's turn started in 2026 and is long past a minute, but it
	// was that long already when the board opened.
	if as := b.poll(time.Now(), 0, lim, never); len(as) != 0 {
		t.Errorf("alerts = %+v", as)
	}
}
