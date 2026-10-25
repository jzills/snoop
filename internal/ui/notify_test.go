package ui

import (
	"testing"
)

func envOf(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestNotifySequence(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"default", nil, "\x1b]777;notify;snoop;api: your turn\a"},
		{"iTerm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, "\x1b]9;snoop: api: your turn\a"},
		{"iTerm2 inside tmux", map[string]string{"LC_TERMINAL": "iTerm2", "TMUX": "/tmp/t"},
			"\x1bPtmux;\x1b\x1b]9;snoop: api: your turn\a\x1b\\"},
		{"WezTerm", map[string]string{"TERM_PROGRAM": "WezTerm"}, "\x1b]9;snoop: api: your turn\a"},
		{"Ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, "\x1b]9;snoop: api: your turn\a"},
		{"kitty", map[string]string{"KITTY_WINDOW_ID": "1"},
			"\x1b]99;i=snoop:d=0;snoop\a\x1b]99;i=snoop:p=body;api: your turn\a"},
		{"screen", map[string]string{"STY": "1.pts"}, "\x1bP\x1b]777;notify;snoop;api: your turn\a\x1b\\"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := notifySeq(envOf(tc.env), "api: your turn"); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestNotifyTextIsSanitised(t *testing.T) {
	got := notifySeq(envOf(nil), "a;b\x1b]0;evil\a\nc")
	if want := "\x1b]777;notify;snoop;a,b]0,evil c\a"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestSignalNotifiesTheLastChosenAlert(t *testing.T) {
	on := map[string]bool{alertTurn: true}
	as := []*alert{{kind: alertTurn, text: "api: ✓ your turn"}, {kind: alertCompact, text: "api: compacted"}}
	if got := chosen(as, on); got == nil || got.kind != alertTurn {
		t.Errorf("chosen = %+v, want the turn alert", got)
	}
	if chosen([]*alert{{kind: alertCompact}}, on) != nil {
		t.Error("an alert kind not chosen was chosen")
	}
}
