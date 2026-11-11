package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// badgeApp is an app on the picker with the given options and a session
// it can open.
func badgeApp(t *testing.T, opts Options) (*App, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	os.WriteFile(p, []byte(fixture), 0o644)
	opts.Root = t.TempDir()
	a := New(opts)
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a.Update(sessionsMsg{})
	t.Cleanup(func() {
		for a.top() != nil {
			a.pop()
		}
	})
	return a, p
}

func topLine(a *App) string { return strings.Split(ansi.Strip(a.View()), "\n")[0] }

func TestNoBadgeWithoutSignals(t *testing.T) {
	a, _ := badgeApp(t, Options{})
	if top := topLine(a); strings.Contains(top, "notify") || strings.Contains(top, "bell") {
		t.Errorf("top bar %q has a badge with nothing turned on", top)
	}
}

func TestNotifyBadgeOnEveryScreen(t *testing.T) {
	a, p := badgeApp(t, Options{Notifier: &sent{}, BellOn: map[string]bool{alertTurn: true}})
	if top := topLine(a); !strings.Contains(top, "snoop  ● notify") {
		t.Errorf("picker top bar %q, want the notify badge beside snoop", top)
	}
	openNow(a, openMsg{path: p})
	if top := topLine(a); !strings.Contains(top, "● notify") {
		t.Errorf("session top bar %q, want the notify badge", top)
	}
	a.pop()
	a.board = newBoard(a.opts.Root)
	if top := topLine(a); !strings.Contains(top, "● notify") {
		t.Errorf("board top bar %q, want the notify badge", top)
	}
}

func TestNotifyBadgeIsHeldWhileFocused(t *testing.T) {
	a, _ := badgeApp(t, Options{Notifier: &sent{}})
	a.Update(tea.FocusMsg{})
	if top := topLine(a); !strings.Contains(top, "○ notify") {
		t.Errorf("top bar %q, want the held badge while snoop has focus", top)
	}
	a.Update(tea.BlurMsg{})
	if top := topLine(a); !strings.Contains(top, "● notify") {
		t.Errorf("top bar %q, want the armed badge once focus leaves", top)
	}
}

func TestBellBadge(t *testing.T) {
	a, _ := badgeApp(t, Options{Bell: true})
	if top := topLine(a); !strings.Contains(top, "bell") || strings.Contains(top, "notify") {
		t.Errorf("top bar %q, want only the bell badge", top)
	}
}

func TestNotifyBadgeShowsAFailure(t *testing.T) {
	a, _ := badgeApp(t, Options{Notifier: &sent{}})
	a.Update(notifyErrMsg{errors.New("no dbus")})
	if top := topLine(a); !strings.Contains(top, glyphErr+" notify") {
		t.Errorf("top bar %q, want the failed badge", top)
	}
}

func TestHelpSaysHowNotificationsGoOut(t *testing.T) {
	a, _ := badgeApp(t, Options{Notifier: &sent{}, Bell: true, BellOn: map[string]bool{alertTurn: true, alertError: true}})
	a.Update(key("?"))
	help := ansi.Strip(a.View())
	for _, want := range []string{"test", "error, turn", "held while snoop has focus"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q:\n%s", want, help)
		}
	}
}
