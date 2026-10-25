// Command snoop is a terminal viewer for Claude Code session transcripts.
//
//	snoop                 pick a session
//	snoop --follow        follow the newest session for this directory, live
//	snoop <id|path>       open a session by id (prefix) or file path
//	snoop export <id|path> [-o file]
//	                        write a session as Markdown (stdout by default)
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/export"
	"github.com/jzills/snoop/internal/notify"
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
	showVersion := flag.Bool("version", false, "print the version and exit")
	follow := flag.Bool("follow", false, "follow the newest session for the project and switch to new ones")
	flag.BoolVar(follow, "f", false, "shorthand for --follow")
	live := flag.Bool("live", false, "show every session written in the last 30 minutes, across projects")
	project := flag.String("project", "", "project directory (default: current directory)")
	stall := flag.Duration("stall", 30*time.Second, "alert when a tool call has no result after this long (0 disables)")
	ring := flag.Bool("bell", false, "ring the terminal bell on alerts")
	var notifyMode notifyFlag
	flag.Var(&notifyMode, "notify", "send desktop notifications on alerts: auto (bare --notify), terminal or system")
	alerts := flag.String("alerts", "error,turn", "alerts that ring the bell or notify: "+strings.Join(ui.AlertKinds, ","))
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: snoop [--follow | --live] [--project dir] [--bell] [--notify] [--version] [session-id|path]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(versionText())
		return
	}

	root := store.Root()
	if flag.Arg(0) == "export" {
		exportCmd(root, flag.Args()[1:])
		return
	}
	cwd := *project
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cwd, _ = filepath.Abs(cwd)
	if *live && (*follow || flag.NArg() > 0) {
		fail(fmt.Errorf("--live shows every session; it can't be combined with --follow or a session"))
	}
	opts := ui.Options{Root: root, ProjectDir: store.ProjectDir(root, cwd), Follow: *follow, Live: *live,
		Stall: *stall, Bell: *ring, BellOn: map[string]bool{}, CacheDir: summary.DefaultDir()}
	if notifyMode != "" {
		s, err := notify.Choose(string(notifyMode), notify.Env{Getenv: os.Getenv, LookPath: exec.LookPath, GOOS: runtime.GOOS, Out: os.Stderr})
		if err != nil {
			fail(err)
		}
		opts.Notifier = s
	}
	for _, k := range strings.Split(*alerts, ",") {
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
	case flag.NArg() > 0:
		p, err := resolve(root, flag.Arg(0))
		if err != nil {
			fail(err)
		}
		opts.Path = p
	case *follow:
		p, err := store.Newest(opts.ProjectDir)
		if err != nil {
			fail(fmt.Errorf("no sessions for %s (looked in %s)", cwd, opts.ProjectDir))
		}
		opts.Path = p
	}

	app := ui.New(opts)
	_, err := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus()).Run()
	app.Close()
	if err != nil {
		fail(err)
	}
}

// exportCmd writes a whole session as Markdown, without thinking blocks.
func exportCmd(root string, args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	out := fs.String("o", "", "write to this file instead of stdout")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: snoop export <session-id|path> [-o file]\n\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	// Allow the flag after the session too.
	rest := fs.Args()
	if len(rest) > 1 {
		fs.Parse(rest[1:])
		rest = append(rest[:1], fs.Args()...)
	}
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

// notifyFlag is --notify: bare it means auto, or it names a way to notify.
type notifyFlag string

func (f *notifyFlag) String() string   { return string(*f) }
func (f *notifyFlag) IsBoolFlag() bool { return true }

func (f *notifyFlag) Set(v string) error {
	switch v {
	case "true":
		v = notify.Auto
	case "false":
		v = ""
	case notify.Auto, notify.Terminal, notify.System:
	default:
		return fmt.Errorf("want auto, terminal or system")
	}
	*f = notifyFlag(v)
	return nil
}
