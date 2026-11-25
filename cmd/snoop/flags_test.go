package main

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestEveryFlagInOneGroup(t *testing.T) {
	f := newCLIFlags()
	seen := map[string]int{}
	for _, g := range flagGroups {
		for _, n := range g.names {
			if f.fs.Lookup(n) == nil {
				t.Errorf("group %s names unknown flag %s", g.title, n)
			}
			seen[n]++
		}
	}
	f.fs.VisitAll(func(fl *pflag.Flag) {
		if seen[fl.Name] != 1 {
			t.Errorf("--%s is in %d help groups, want 1", fl.Name, seen[fl.Name])
		}
	})
}

func TestParseFlags(t *testing.T) {
	f := newCLIFlags()
	if err := parseFlags(f.fs, []string{"64f3", "-f", "--notify", "--bell"}); err != nil {
		t.Fatal(err)
	}
	if !f.follow || !f.bell || f.notify != "auto" || f.fs.Arg(0) != "64f3" {
		t.Errorf("follow %v, bell %v, notify %q, args %q", f.follow, f.bell, f.notify, f.fs.Args())
	}

	f = newCLIFlags()
	if err := parseFlags(f.fs, []string{"--notify=system", "--stall", "1m"}); err != nil {
		t.Fatal(err)
	}
	if f.notify != "system" || f.stall.String() != "1m0s" {
		t.Errorf("notify %q, stall %v", f.notify, f.stall)
	}
}

func TestParseFlagsRejects(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-follow"}, "use --follow, with two dashes, not -follow"},
		{[]string{"-stall=1m"}, "use --stall"},
		{[]string{"--f"}, "unknown flag: --f"},
		{[]string{"--notify=loud"}, "want auto, terminal or system"},
	} {
		err := parseFlags(newCLIFlags().fs, c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: err = %v, want %q", c.args, err, c.want)
		}
	}
	// A short flag, or a session id after --, isn't mistaken for a long flag.
	if err := parseFlags(newCLIFlags().fs, []string{"--", "-live"}); err != nil {
		t.Errorf("after --: %v", err)
	}
}

func TestUsage(t *testing.T) {
	var b strings.Builder
	newCLIFlags().usage(&b)
	out := b.String()
	for _, want := range []string{
		"  -f, --follow ",
		"      --notify[=auto] ",
		"      --stall duration ",
		"(default 30s)",
		"  -h, --help ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help is missing %q:\n%s", want, out)
		}
	}
	for _, l := range strings.Split(out, "\n") {
		if len(l) > 80 {
			t.Errorf("line wider than 80 columns: %q", l)
		}
	}
	if strings.Contains(out, "true|false") {
		t.Errorf("help shows bool values:\n%s", out)
	}
}

func TestShortDuration(t *testing.T) {
	for in, want := range map[string]string{"5m0s": "5m", "30s": "30s", "1h0m0s": "1h", "1m30s": "1m30s"} {
		if got := shortDuration(in); got != want {
			t.Errorf("shortDuration(%q) = %q, want %q", in, got, want)
		}
	}
}
