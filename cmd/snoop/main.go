// Command snoop is a terminal viewer for Claude Code session transcripts.
//
//	snoop                 pick a session
//	snoop --follow        follow the newest session for this directory, live
//	snoop <id|path>       open a session by id (prefix) or file path
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/store"
	"github.com/jzills/snoop/internal/ui"
)

func main() {
	follow := flag.Bool("follow", false, "follow the newest session for the project and switch to new ones")
	flag.BoolVar(follow, "f", false, "shorthand for --follow")
	project := flag.String("project", "", "project directory (default: current directory)")
	stall := flag.Duration("stall", 30*time.Second, "alert when a tool call has no result after this long (0 disables)")
	ring := flag.Bool("bell", false, "ring the terminal bell on alerts")
	alerts := flag.String("alerts", "error,turn", "alerts that ring the bell: "+strings.Join(ui.AlertKinds, ","))
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: snoop [--follow] [--project dir] [--bell] [session-id|path]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	root := store.Root()
	cwd := *project
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cwd, _ = filepath.Abs(cwd)
	opts := ui.Options{Root: root, ProjectDir: store.ProjectDir(root, cwd), Follow: *follow,
		Stall: *stall, Bell: *ring, BellOn: map[string]bool{}}
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

	if _, err := tea.NewProgram(ui.New(opts), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
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
