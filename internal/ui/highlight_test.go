package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/review"
	"github.com/jzills/snoop/internal/syntax"
)

// waitSpans asks for spans until the background highlighter has them.
func waitSpans(t *testing.T, get func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !get() {
		if time.Now().After(deadline) {
			t.Fatal("highlighting never landed")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func spanText(spans []syntax.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestDiffSpansFollowEachSide(t *testing.T) {
	f := &review.File{Path: "pkg/calc.go", Hunks: []review.Hunk{{
		Header: "@@ -1,4 +1,4 @@",
		Lines: []review.Line{
			{Kind: ' ', Old: 1, New: 1, Text: "/* totals"},
			{Kind: '-', Old: 2, Text: "   old */"},
			{Kind: '+', New: 2, Text: "   new */"},
			{Kind: ' ', Old: 3, New: 3, Text: "func Sum(a, b int) int { return a + b }"},
		},
	}}}
	var rows [][]syntax.Span
	waitSpans(t, func() bool {
		rows, _, _ = diffSpans(f)
		return rows[1] != nil && rows[2] != nil && rows[3] != nil
	})
	if len(rows) != 5 || rows[0] != nil {
		t.Fatalf("got %d rows, want a header and 4 lines", len(rows))
	}
	for i, l := range f.Hunks[0].Lines {
		if got := spanText(rows[i+1]); got != l.Text {
			t.Errorf("row %d = %q, want %q", i+1, got, l.Text)
		}
	}
	// Both sides continue the comment the context line opened.
	for _, i := range []int{2, 3} {
		if rows[i][0].Class != syntax.Comment {
			t.Errorf("row %d starts as class %d, want a comment", i, rows[i][0].Class)
		}
	}
}

func TestDiffRowWithSpansFillsWidth(t *testing.T) {
	v := &view{}
	l := review.Line{Kind: '+', New: 7, Text: "\treturn \"a very long string that will not fit in the pane\""}
	spans := []syntax.Span{{Text: "\t"}, {Text: "return", Class: syntax.Keyword}, {Text: " "}, {Text: l.Text[8:], Class: syntax.String}}
	got := v.diffRow(dRow{line: &l}, spans, 40, 1, false, false)
	if w := lipgloss.Width(got); w != 40 {
		t.Errorf("row is %d wide, want 40", w)
	}
	plain := ansi.Strip(got)
	if !strings.Contains(plain, "+     return \"a very") || !strings.HasSuffix(strings.TrimRight(plain, " "), "…") {
		t.Errorf("row = %q", plain)
	}
}

func TestCodeBlockFallsBackForUnknownFiles(t *testing.T) {
	got := ansi.Strip(codeBlock("notes.unknownext", "one\ntwo", 80))
	if got != "one\ntwo" {
		t.Errorf("codeBlock = %q", got)
	}
}

// paused swaps in a highlighter whose worker hasn't started, so code stays
// on the way until the test lets it run.
func paused(t *testing.T) (run func()) {
	t.Helper()
	old := hl
	h := newHighlighter()
	h.start.Do(func() {})
	hl = h
	t.Cleanup(func() { hl = old })
	return func() {
		go h.work()
		h.wake <- struct{}{}
	}
}

func atTime(t *testing.T, now time.Time) {
	t.Helper()
	old := hlNow
	hlNow = func() time.Time { return now }
	t.Cleanup(func() { hlNow = old })
}

func TestCodeWaitsBlankThenSpins(t *testing.T) {
	run := paused(t)
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	atTime(t, start)
	src := "package x\n\nfunc A() {}"
	got, st := renderCode("a.go", src, sAdd)
	if st != hlWaiting || len(got) != 3 || strings.Join(got, "") != "" {
		t.Fatalf("before the grace, renderCode = %q (state %d), want 3 blank lines", got, st)
	}
	if !hl.busy() {
		t.Error("a pane waiting should keep the spinner turning")
	}
	atTime(t, start.Add(hlGrace))
	got, _ = renderCode("a.go", src, sAdd)
	if first := ansi.Strip(got[0]); first != spinFrame(start.Add(hlGrace))+" highlighting · 0s" || strings.Contains(strings.Join(got, ""), "package") {
		t.Fatalf("after the grace, renderCode = %q, want the spinner and no plain code", got)
	}
	run()
	waitSpans(t, func() bool { _, st := renderCode("a.go", src, sAdd); return st == hlReady })
	got, _ = renderCode("a.go", src, sAdd)
	if ansi.Strip(strings.Join(got, "\n")) != src {
		t.Errorf("colored code = %q", got)
	}
	if hl.busy() {
		t.Error("nothing is waiting any more")
	}
}

func TestColoredAheadIsNotAWait(t *testing.T) {
	paused(t)
	k, _ := keyFor("a.go", "var x = 1")
	hl.ahead([]hlJob{{key: k, path: "a.go", src: "var x = 1"}})
	if hl.busy() {
		t.Error("code colored ahead shouldn't turn the spinner")
	}
	// A pane asking moves it to the top.
	hl.ahead([]hlJob{{key: hlKey{lang: "Go", sum: 1}, path: "b.go", src: "y"}})
	hl.get("a.go", "var x = 1")
	if top := hl.jobs[len(hl.jobs)-1]; top.key != k || len(hl.jobs) != 2 {
		t.Errorf("jobs = %+v, want the asked-for code on top", hl.jobs)
	}
}

func TestSameCodeUnderTwoPathsIsOneEntry(t *testing.T) {
	a, _ := keyFor("/home/u/src/x/main.go", "package main")
	b, _ := keyFor("main.go", "package main")
	if a != b {
		t.Error("the same Go code under two paths should share its colors")
	}
}

func TestDiffPaneNeverDrawsPlainCode(t *testing.T) {
	paused(t)
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	atTime(t, start)
	f := &review.File{Path: "a.go", Added: 1, Hunks: []review.Hunk{{
		Header: "@@ -1 +1,2 @@",
		Lines:  []review.Line{{Kind: ' ', Old: 1, New: 1, Text: "package secretword"}, {Kind: '+', New: 2, Text: "var y = 2"}},
	}}}
	if rows, waiting, _ := diffSpans(f); !waiting || len(rows) != 3 {
		t.Fatalf("diffSpans: %d rows, waiting %v", len(rows), waiting)
	}
	colorDiff(&review.Diff{Files: []*review.File{f}}, nil)
	if rows, waiting, _ := diffSpans(f); waiting || spanText(rows[1]) != "package secretword" {
		t.Errorf("after colorDiff the file should be ready, waiting %v", waiting)
	}
}

func TestColorAheadQueuesEditsAndWrites(t *testing.T) {
	paused(t)
	events := []*parse.Event{
		{Tool: &parse.ToolCall{Name: "Write", Input: []byte(`{"file_path":"/r/a.go","content":"package a\n"}`)}},
		{Tool: &parse.ToolCall{Name: "Read", Input: []byte(`{"file_path":"/r/b.go"}`)}},
		{Tool: &parse.ToolCall{Name: "Edit", Input: []byte(`{"file_path":"/r/c.py","old_string":"x = 1","new_string":"x = 2"}`)}},
	}
	colorAhead(events)()
	if len(hl.jobs) != 3 {
		t.Fatalf("queued %d jobs, want the write and both sides of the edit", len(hl.jobs))
	}
	// The newest call is colored first, and the stack pops from the end.
	if last := hl.jobs[len(hl.jobs)-1]; last.path != "/r/c.py" {
		t.Errorf("first to color is %q, want the newest call's code", last.path)
	}
	lines, _ := writeLines("package a\n")
	if k, _ := keyFor("a.go", strings.Join(lines, "\n")); hl.queued[k] != (time.Time{}) {
		t.Error("the write's content should be queued under the key the pane asks for")
	} else if _, ok := hl.queued[k]; !ok {
		t.Error("the write's content isn't queued under the key the pane asks for")
	}
}

func TestFailedLexIsPlain(t *testing.T) {
	paused(t)
	src := "package x\nfunc A() {}"
	k, _ := keyFor("a.go", src)
	hl.store(k, nil) // what syntax.Lines returns when the lexer fails
	got, st := renderCode("a.go", src, sAdd)
	if st != hlPlain || ansi.Strip(strings.Join(got, "\n")) != src {
		t.Errorf("renderCode = %q (state %d), want the plain code", got, st)
	}
}

func TestFullCacheKeepsRecentCode(t *testing.T) {
	paused(t)
	shown, _ := keyFor("a.go", "var shown = 1")
	hl.store(shown, [][]syntax.Span{{{Text: "var shown = 1"}}})
	for i := 0; len(hl.done) < hlMaxEntries; i++ {
		hl.store(hlKey{lang: "Go", sum: uint64(i + 1)}, nil)
		if i%100 == 0 {
			hl.get("a.go", "var shown = 1") // drawn again and again
		}
	}
	hl.store(hlKey{lang: "Go", sum: 1 << 40}, nil)
	if len(hl.done) > hlMaxEntries/2+2 {
		t.Errorf("cache holds %d after evicting, want about half", len(hl.done))
	}
	if _, ok := hl.done[shown]; !ok {
		t.Error("code on screen lost its colors")
	}
}

func TestColoredAheadDoesNotRedraw(t *testing.T) {
	paused(t)
	ahead, _ := keyFor("a.go", "var a = 1")
	hl.ahead([]hlJob{{key: ahead, path: "a.go", src: "var a = 1"}})
	v := hl.ver()
	hl.store(ahead, [][]syntax.Span{{{Text: "var a = 1"}}})
	if hl.ver() != v {
		t.Error("code no pane waited for shouldn't redraw")
	}
	hl.get("b.go", "var b = 2")
	wanted, _ := keyFor("b.go", "var b = 2")
	hl.store(wanted, [][]syntax.Span{{{Text: "var b = 2"}}})
	if hl.ver() == v || hl.busy() {
		t.Error("code a pane waited for should redraw, and nothing waits after")
	}
}
