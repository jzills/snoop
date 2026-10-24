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
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jzills/snoop/internal/store"
	"github.com/jzills/snoop/internal/ui"
)

func main() {
	follow := flag.Bool("follow", false, "follow the newest session for the project and switch to new ones")
	flag.BoolVar(follow, "f", false, "shorthand for --follow")
	project := flag.String("project", "", "project directory (default: current directory)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: snoop [--follow] [--project dir] [session-id|path]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	root := store.Root()
	cwd := *project
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cwd, _ = filepath.Abs(cwd)
	opts := ui.Options{Root: root, ProjectDir: store.ProjectDir(root, cwd), Follow: *follow}

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
