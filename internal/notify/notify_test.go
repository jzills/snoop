package notify

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func envOf(kv map[string]string, goos string, have ...string) Env {
	return Env{
		Getenv: func(k string) string { return kv[k] },
		LookPath: func(name string) (string, error) {
			for _, h := range have {
				if h == name {
					return "/bin/" + name, nil
				}
			}
			return "", errors.New("not found")
		},
		GOOS: goos,
		Out:  &bytes.Buffer{},
	}
}

func TestChoose(t *testing.T) {
	wsl := map[string]string{"WSL_DISTRO_NAME": "Ubuntu", "WT_SESSION": "x"}
	for _, tc := range []struct {
		name, mode string
		env        Env
		want       string
	}{
		{"WSL in Windows Terminal", Auto, envOf(wsl, "linux", "powershell.exe"), "windows toast"},
		{"WSL without interop", Auto, envOf(wsl, "linux"), "terminal"},
		{"WSL over SSH", Auto, envOf(map[string]string{"WSL_DISTRO_NAME": "U", "SSH_CONNECTION": "1 2 3 4"}, "linux", "powershell.exe"), "terminal"},
		{"WezTerm on WSL", Auto, envOf(map[string]string{"WSL_DISTRO_NAME": "U", "TERM_PROGRAM": "WezTerm"}, "linux", "powershell.exe"), "terminal"},
		{"macOS Terminal.app", Auto, envOf(map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, "darwin", "osascript"), "osascript"},
		{"iTerm2", Auto, envOf(map[string]string{"TERM_PROGRAM": "iTerm.app"}, "darwin", "osascript"), "terminal"},
		{"Linux desktop", Auto, envOf(map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, "linux", "notify-send"), "notify-send"},
		{"Linux console", Auto, envOf(nil, "linux", "notify-send"), "terminal"},
		{"forced terminal", Terminal, envOf(wsl, "linux", "powershell.exe"), "terminal"},
		{"forced system", System, envOf(map[string]string{"TERM_PROGRAM": "iTerm.app"}, "darwin", "osascript"), "osascript"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Choose(tc.mode, tc.env)
			if err != nil || s.Name() != tc.want {
				t.Errorf("Choose = %v, %v; want %s", s, err, tc.want)
			}
		})
	}
	if _, err := Choose(System, envOf(nil, "linux")); err == nil {
		t.Error("forced system without any system notifier did not fail")
	}
	if _, err := Choose("loud", envOf(nil, "linux")); err == nil {
		t.Error("unknown mode did not fail")
	}
}

func TestSequence(t *testing.T) {
	n := Note{"api · Fix tests", "Your turn"}
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"default", nil, "\x1b]777;notify;api · Fix tests;Your turn\a"},
		{"iTerm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, "\x1b]9;api · Fix tests: Your turn\a"},
		{"iTerm2 inside tmux", map[string]string{"LC_TERMINAL": "iTerm2", "TMUX": "/tmp/t"},
			"\x1bPtmux;\x1b\x1b]9;api · Fix tests: Your turn\a\x1b\\"},
		{"kitty", map[string]string{"KITTY_WINDOW_ID": "1"},
			"\x1b]99;i=snoop:d=0;api · Fix tests\a\x1b]99;i=snoop:p=body;Your turn\a"},
		{"screen", map[string]string{"STY": "1.pts"}, "\x1bP\x1b]777;notify;api · Fix tests;Your turn\a\x1b\\"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sequence(func(k string) string { return tc.env[k] }, n); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
	got := Sequence(func(string) string { return "" }, Note{"t", "a;b\x1b]0;evil\a\nc"})
	if want := "\x1b]777;notify;t;a,b]0,evil c\a"; got != want {
		t.Errorf("unsanitised: got %q want %q", got, want)
	}
}

func TestWSLToastQuotesItsText(t *testing.T) {
	argv := wslToast(Note{"it's api", "say 'hi' $(rm -rf /)"})
	raw, err := base64.StdEncoding.DecodeString(argv[len(argv)-1])
	if err != nil {
		t.Fatal(err)
	}
	u := make([]uint16, len(raw)/2)
	for i := range u {
		u[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	script := string(utf16.Decode(u))
	for _, want := range []string{"CreateTextNode('it''s api')", "CreateTextNode('say ''hi'' $(rm -rf /)')"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %s:\n%s", want, script)
		}
	}
}

func TestClaimOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notified")
	now := time.Now()
	if !Claim(dir, "a", now) {
		t.Fatal("first claim failed")
	}
	if Claim(dir, "a", now) {
		t.Error("second claim of the same key succeeded")
	}
	if !Claim(dir, "b", now) {
		t.Error("a different key was refused")
	}
	// Old claims are swept, so the key can notify again later.
	es, _ := os.ReadDir(dir)
	old := now.Add(-2 * time.Hour)
	for _, e := range es {
		os.Chtimes(filepath.Join(dir, e.Name()), old, old)
	}
	Claim(dir, "c", now)
	if !Claim(dir, "a", now) {
		t.Error("a claim older than an hour was not swept")
	}
	if !Claim("", "a", now) || !Claim("", "a", now) {
		t.Error("claims without a directory should always succeed")
	}
}
