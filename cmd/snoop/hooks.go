package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jzills/snoop/internal/review"
)

// hookCmd is what Claude Code runs on each hook event: it hands the
// session any review comments waiting for it, and lets snoop answer
// permission requests. It never fails the hook, so a problem with snoop
// can't stop an agent.
func hookCmd(args []string) {
	wait := review.DefaultAskWait
	if len(args) == 2 && args[0] == "--ask-wait" {
		// A bad value keeps the default rather than failing the hook.
		if d, err := time.ParseDuration(args[1]); err == nil && d > 0 {
			wait = d
		}
	}
	o := review.HookOptions{Inbox: review.InboxDir(), Asks: review.AsksDir(), Listening: review.ListeningFile(),
		AskWait: wait, Now: time.Now, Sleep: time.Sleep}
	if err := review.Hook(os.Stdin, os.Stdout, o); err != nil {
		fmt.Fprintln(os.Stderr, "snoop hook:", err)
	}
}

// hooksCmd installs, removes or reports the hooks that deliver review
// comments.
func hooksCmd(root string, args []string) {
	fs := flag.NewFlagSet("hooks", flag.ExitOnError)
	project := fs.Bool("project", false, "use .claude/settings.json in the current directory instead of your user settings")
	askWait := fs.Duration("ask-wait", review.DefaultAskWait, "with install: how long a permission request waits for an answer from snoop (1s to 30m)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: snoop hooks install|uninstall|status [--project]\n\n")
		fmt.Fprintf(os.Stderr, "The hooks deliver comments sent from the Diff tab to the agent, and let\n")
		fmt.Fprintf(os.Stderr, "the live board and session views answer permission requests.\n\n")
		fs.PrintDefaults()
	}
	if len(args) == 0 {
		fs.Usage()
		os.Exit(2)
	}
	action := args[0]
	fs.Parse(args[1:])
	settings := review.UserSettings(root)
	if *project {
		settings = filepath.Join(".claude", "settings.json")
	}
	switch action {
	case "install":
		if *askWait < time.Second || *askWait > 30*time.Minute {
			fmt.Fprintf(os.Stderr, "--ask-wait must be between 1s and 30m, not %v\n", *askWait)
			os.Exit(2)
		}
		changed, err := review.Install(settings, fmt.Sprintf("%q hook", hookPath()), *askWait)
		if err != nil {
			fail(err)
		}
		if changed {
			fmt.Printf("Added snoop's hooks (%v) to %s.\n", review.HookEvents, settings)
		} else {
			fmt.Printf("The hooks are already in %s.\n", settings)
		}
	case "uninstall":
		changed, err := review.Uninstall(settings)
		if err != nil {
			fail(err)
		}
		if changed {
			fmt.Printf("Removed snoop's hooks from %s.\n", settings)
		} else {
			fmt.Printf("There were no snoop hooks in %s.\n", settings)
		}
	case "status":
		missing := review.Missing(settings)
		switch {
		case len(missing) == 0:
			fmt.Printf("Installed in %s.\n", settings)
		case review.Installed(settings):
			fmt.Printf("Installed in %s, without %s. Run snoop hooks install to add it.\n", settings, strings.Join(missing, ", "))
		default:
			fmt.Printf("Not installed in %s.\n", settings)
			os.Exit(1)
		}
	default:
		fs.Usage()
		os.Exit(2)
	}
}

// hookPath is the snoop the hooks run: the one on PATH when it is this
// binary, since package managers keep that path across upgrades while
// the binary it links to moves (a Homebrew Cellar path, say).
func hookPath() string {
	exe, err := os.Executable()
	if err != nil {
		fail(err)
	}
	if p, err := exec.LookPath("snoop"); err == nil {
		if abs, err := filepath.Abs(p); err == nil && sameFile(abs, exe) {
			return abs
		}
	}
	return exe
}

func sameFile(a, b string) bool {
	x, err1 := os.Stat(a)
	y, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(x, y)
}
