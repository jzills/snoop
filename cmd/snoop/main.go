// Command snoop is a terminal viewer for Claude Code session transcripts.
//
//	snoop                 pick a session
//	snoop --follow        follow the newest session for this directory, live
//	snoop <id|path>       open a session by id (prefix) or file path
//	snoop export <id|path> [-o file]
//	                        write a session as Markdown (stdout by default)
//	snoop hooks install|uninstall|status [--project]
//	                        manage the Claude Code hooks that deliver review comments
//	snoop hook            run by those hooks: deliver waiting comments
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/pflag"

	"github.com/jzills/snoop/internal/export"
	"github.com/jzills/snoop/internal/notify"
	"github.com/jzills/snoop/internal/review"
	"github.com/jzills/snoop/internal/store"
	"github.com/jzills/snoop/internal/summary"
	"github.com/jzills/snoop/internal/ui"
)

// Stamped by scripts/build_binaries.sh; a plain go build or go install
// reports "dev", with the commit and date Go recorded when it has them.
var (
	version = "dev"
	commit  string
	date    string
)

// versionText is what --version prints: the version, then how it was built.
func versionText() string {
	c, d := commit, date
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; version == "dev" && v != "" && v != "(devel)" {
			version = strings.TrimPrefix(v, "v")
		}
		for _, s := range info.Settings {
			switch {
			case s.Key == "vcs.revision" && c == "":
				c = s.Value[:min(len(s.Value), 7)]
			case s.Key == "vcs.time" && d == "":
				d = s.Value[:min(len(s.Value), 10)]
			}
		}
	}
	v := "snoop " + version
	if version != "dev" {
		v = "snoop v" + version
	}
	var parts []string
	for _, p := range []string{c, d, runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return v + "\n" + strings.Join(parts, " · ")
}

func main() {
	f, cmd, cmdArgs, err := parseMain(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "snoop: %v\nRun snoop --help for usage.\n", err)
		os.Exit(2)
	}
	switch cmd {
	case "export":
		exportCmd(store.Root(), cmdArgs)
		return
	case "hook":
		hookCmd(cmdArgs)
		return
	case "hooks":
		hooksCmd(store.Root(), cmdArgs)
		return
	}
	if f.help {
		f.usage(os.Stdout)
		return
	}
	if f.version {
		fmt.Println(versionText())
		return
	}
	if len(f.args) > 1 {
		fail(fmt.Errorf("open one session at a time, not %d", len(f.args)))
	}
	root := store.Root()
	cwd := f.project
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cwd, _ = filepath.Abs(cwd)
	if f.costAlert < 0 || f.longTurn < 0 {
		fail(fmt.Errorf("--cost-alert and --long-turn can't be negative"))
	}
	if f.live && (f.follow || len(f.args) > 0) {
		fail(fmt.Errorf("--live shows every session; it can't be combined with --follow or a session"))
	}
	opts := ui.Options{Root: root, ProjectDir: store.ProjectDir(root, cwd), Follow: f.follow, Live: f.live,
		Stall: f.stall, Limits: ui.Limits{Cost: f.costAlert, LongTurn: f.longTurn}, Bell: f.bell, BellOn: map[string]bool{}, CacheDir: summary.DefaultDir(),
		AsksDir: review.AsksDir(), ListeningFile: review.ListeningFile(), InboxDir: review.InboxDir(), TurnsDir: review.TurnsDir()}
	if f.notify != "" {
		s, err := notify.Choose(string(f.notify), notify.Env{Getenv: os.Getenv, LookPath: exec.LookPath, GOOS: runtime.GOOS, Out: os.Stderr})
		if err != nil {
			fail(err)
		}
		opts.Notifier = s
	}
	for _, k := range strings.Split(f.alerts, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if !slices.Contains(ui.AlertKinds, k) {
			fail(fmt.Errorf("unknown alert %q (want %s)", k, strings.Join(ui.AlertKinds, ",")))
		}
		opts.BellOn[k] = true
	}

	switch {
	case len(f.args) > 0:
		p, err := resolve(root, f.args[0])
		if err != nil {
			fail(err)
		}
		opts.Path = p
	case f.follow:
		p, err := store.Newest(opts.ProjectDir)
		if err != nil {
			fail(fmt.Errorf("no sessions for %s (looked in %s)", cwd, opts.ProjectDir))
		}
		opts.Path = p
	}

	app := ui.New(opts)
	_, err = tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus()).Run()
	app.Close()
	if err != nil {
		fail(err)
	}
}

// exportCmd writes a whole session as Markdown, without thinking blocks.
func exportCmd(root string, args []string) {
	fs := pflag.NewFlagSet("export", pflag.ContinueOnError)
	out := fs.StringP("output", "o", "", "write to this `file` instead of stdout")
	parseCommand(fs, args, func(w io.Writer) {
		fmt.Fprintf(w, "Usage:\n  snoop export <session-id|path> [-o file]\n\nWrite a session as Markdown, without thinking blocks.\n\nFlags:\n%s", flagTable(allFlags(fs), 0))
	})
	rest := fs.Args()
	if len(rest) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	p, err := resolve(root, rest[0])
	if err != nil {
		fail(err)
	}
	s, _, err := store.Load(p)
	if err != nil {
		fail(err)
	}
	md := export.Markdown(s, "", export.All(s))
	if *out == "" {
		fmt.Print(md)
		return
	}
	if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
		fail(err)
	}
}

// resolve accepts a transcript path or a (prefix of a) session id.
func resolve(root, arg string) (string, error) {
	if strings.HasSuffix(arg, ".jsonl") {
		if _, err := os.Stat(arg); err == nil {
			return filepath.Abs(arg)
		}
	}
	matches, _ := filepath.Glob(filepath.Join(root, "*", arg+"*.jsonl"))
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session matching %q", arg)
	case 1:
		return matches[0], nil
	}
	return "", fmt.Errorf("%q matches %d sessions; use a longer id", arg, len(matches))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "snoop:", err)
	os.Exit(1)
}
