package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// newClockApp opens a session made of lines in an app whose clock reads
// bStart plus at.
func newClockApp(t *testing.T, at time.Duration, lines ...string) *App {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(Options{Root: t.TempDir(), Stall: 30 * time.Second})
	a.now = func() time.Time { return bStart.Add(at) }
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a.Update(sessionsMsg{})
	openNow(a, openMsg{path: p})
	t.Cleanup(func() {
		for a.top() != nil {
			a.pop()
		}
	})
	return a
}

func screenLines(a *App) []string { return strings.Split(ansi.Strip(a.View()), "\n") }

func TestHeaderSaysWorkingWhileAToolRuns(t *testing.T) {
	a := newClockApp(t, 20*time.Second, bPrompt, bBash)
	top := screenLines(a)[0]
	if !strings.Contains(top, "working") {
		t.Errorf("top bar %q, want working", top)
	}
	if strings.Contains(top, "live") || strings.Contains(top, "active") {
		t.Errorf("top bar %q still says live or active", top)
	}
}

func TestHeaderSaysWorkingRightAfterAWrite(t *testing.T) {
	a := newClockApp(t, 20*time.Second, bPrompt, bBash)
	a.top().onTail(lines(`{"type":"assistant","uuid":"a9","timestamp":"2026-10-25T10:00:19Z","message":{"id":"m9","content":[{"type":"text","text":"still going"}]}}`))
	if top := screenLines(a)[0]; !strings.Contains(top, "working") || strings.Contains(top, "live") {
		t.Errorf("top bar %q, want working and no live", top)
	}
}

func TestHeaderSaysYourTurnOnceTheTurnEnds(t *testing.T) {
	a := newClockApp(t, time.Minute, bPrompt, bBash, bBashDone, bTurnEnd)
	if top := screenLines(a)[0]; !strings.Contains(top, "your turn") || strings.Contains(top, "working") {
		t.Errorf("top bar %q, want your turn", top)
	}
}

func TestHeaderSaysWaitingPastTheStall(t *testing.T) {
	a := newClockApp(t, 2*time.Minute, bPrompt, bBash)
	if top := screenLines(a)[0]; !strings.Contains(top, "waiting") {
		t.Errorf("top bar %q, want waiting", top)
	}
}

// lastRow is the last non-blank row of the timeline.
func lastRow(a *App) string {
	ls := screenLines(a)
	ls = ls[:len(ls)-2] // the footer rule and footer
	for i := len(ls) - 1; i >= 0; i-- {
		if strings.TrimSpace(ls[i]) != "" {
			return ls[i]
		}
	}
	return ""
}

func TestTimelineEndsWithTheRunningTool(t *testing.T) {
	a := newClockApp(t, 20*time.Second, bPrompt, bBash)
	row := lastRow(a)
	if !strings.Contains(row, "Bash") || !strings.Contains(row, "go test") || !strings.Contains(row, "15s") {
		t.Errorf("last row %q, want the running Bash call and 15s", row)
	}
	if !strings.ContainsAny(row, spinFrames) {
		t.Errorf("last row %q has no spinner", row)
	}
}

func TestTimelineEndsWithReplying(t *testing.T) {
	a := newClockApp(t, 10*time.Second, bPrompt)
	if row := lastRow(a); !strings.Contains(row, "replying") || !strings.Contains(row, "10s") {
		t.Errorf("last row %q, want replying for 10s", row)
	}
}

func TestTimelineHasNoSpinnerOnYourTurn(t *testing.T) {
	a := newClockApp(t, time.Minute, bPrompt, bBash, bBashDone, bTurnEnd)
	for _, l := range screenLines(a) {
		if strings.ContainsAny(l, spinFrames) || strings.Contains(l, "replying") {
			t.Errorf("row %q shows work after the turn ended", l)
		}
	}
}

func TestRunningToolRowSpins(t *testing.T) {
	a := newClockApp(t, 20*time.Second, bPrompt, bBash)
	ls := screenLines(a)
	for _, l := range ls {
		if strings.Contains(l, "go test") && strings.Contains(l, glyphPending) {
			t.Errorf("running row %q still shows the static pending glyph", l)
		}
	}
}

func TestSpinnerTicksOnlyWhileWorking(t *testing.T) {
	a := newClockApp(t, 20*time.Second, bPrompt, bBash)
	a.View()
	if !a.animating() {
		t.Fatal("not animating while a tool runs")
	}
	_, cmd := a.Update(tickMsg{})
	if !a.spinning || cmd == nil {
		t.Fatal("the tick didn't start the spinner")
	}
	if _, cmd := a.Update(spinMsg{}); cmd == nil {
		t.Error("the spinner stopped while still working")
	}

	done := newClockApp(t, time.Minute, bPrompt, bBash, bBashDone, bTurnEnd)
	done.View()
	if done.animating() {
		t.Error("animating after the turn ended")
	}
	done.spinning = true
	if _, cmd := done.Update(spinMsg{}); cmd != nil || done.spinning {
		t.Error("the spinner kept ticking after the work ended")
	}
}

func TestBoardWorkingRowSpins(t *testing.T) {
	got := rowWorking.render(func(s lipgloss.Style) lipgloss.Style { return s }, "⠹")
	if !strings.Contains(got, "⠹ working") {
		t.Errorf("working = %q, want the spinner frame", got)
	}
}

func TestSpinFrameAdvances(t *testing.T) {
	if spinFrame(bStart) == spinFrame(bStart.Add(spinEvery)) {
		t.Error("the frame doesn't change after one step")
	}
}

func TestASubagentKeepsTheSessionWorkingWhileItsQuiet(t *testing.T) {
	a := newClockApp(t, 11*time.Minute, bPrompt, bAgent)
	if top := screenLines(a)[0]; !strings.Contains(top, "idle") {
		t.Fatalf("top bar %q, want idle with no subagent writes", top)
	}
	a.Update(subWriteMsg{a.top(), bStart.Add(10*time.Minute + 30*time.Second)})
	if top := screenLines(a)[0]; !strings.Contains(top, "working") {
		t.Errorf("top bar %q, want working while the subagent writes", top)
	}
}

func TestAFinishedSubagentShowsNoState(t *testing.T) {
	done := `{"type":"user","uuid":"r2","timestamp":"2026-10-25T10:00:30Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t2","content":"mapped"}]},"toolUseResult":{"agentId":"x1"}}`
	a := newClockApp(t, 40*time.Second, bPrompt, bAgent, done)
	parent := a.top().path
	writeSubagent(t, parent, "x1")
	sub := filepath.Join(strings.TrimSuffix(parent, ".jsonl"), "subagents", "agent-x1.jsonl")
	os.WriteFile(sub, []byte(bPrompt+"\n"), 0o644)
	v, err := openView(sub, parent, "map the parser", false)
	if err != nil {
		t.Fatal(err)
	}
	a.push(v)
	if top := screenLines(a)[0]; strings.Contains(top, "working") {
		t.Errorf("top bar %q, want no state for a finished subagent", top)
	}
}
