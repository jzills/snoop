package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/jzills/snoop/internal/notify"
	"github.com/jzills/snoop/internal/ui"
)

// Long flags take two dashes and short ones one, so -follow is an error
// rather than another spelling of --follow.

const docsURL = "https://jzills.github.io/snoop/docs/reference/cli/"

// cliFlags is snoop's own flags, before any command.
type cliFlags struct {
	fs                                *pflag.FlagSet
	help, version, follow, live, bell bool
	project, alerts                   string
	stall, longTurn                   time.Duration
	costAlert                         float64
	notify                            notifyFlag
}

// flagGroups orders the help: each group's flags, in this order.
var flagGroups = []struct {
	title string
	names []string
}{
	{"Viewing", []string{"follow", "live", "project"}},
	{"Alerts", []string{"alerts", "bell", "notify", "stall", "long-turn", "cost-alert"}},
	{"Other", []string{"help", "version"}},
}

func newCLIFlags() *cliFlags {
	f := &cliFlags{fs: pflag.NewFlagSet("snoop", pflag.ContinueOnError)}
	fs := f.fs
	fs.SortFlags = false
	fs.BoolVarP(&f.follow, "follow", "f", false, "follow the newest session for the project and switch to new ones")
	fs.BoolVar(&f.live, "live", false, "show every session written in the last 30 minutes, across projects")
	fs.StringVar(&f.project, "project", "", "the project `dir` that --follow uses (default: the current directory)")
	fs.StringVar(&f.alerts, "alerts", "error,turn,ask,cost,long", "the comma-separated `list` of alerts that ring the bell or notify, from "+strings.Join(ui.AlertKinds, ","))
	fs.BoolVar(&f.bell, "bell", false, "ring the terminal bell on alerts")
	fs.Var(&f.notify, "notify", "send desktop notifications on alerts: auto, terminal or system")
	fs.Lookup("notify").NoOptDefVal = notify.Auto
	fs.DurationVar(&f.stall, "stall", 30*time.Second, "alert when a tool call has no result after this long (0 disables)")
	fs.DurationVar(&f.longTurn, "long-turn", 0, "alert when a turn has run this long, such as 15m (0 disables)")
	fs.Float64Var(&f.costAlert, "cost-alert", 0, "alert each time a session's cost passes another multiple of this many `dollars`, such as 5 (0 disables)")
	fs.BoolVarP(&f.help, "help", "h", false, "show this help")
	fs.BoolVar(&f.version, "version", false, "print the version and exit")
	fs.Usage = func() { f.usage(fs.Output()) }
	return f
}

func (f *cliFlags) usage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  snoop [flags] [session-id|path]
  snoop export <session-id|path> [-o file]
  snoop hooks install|uninstall|status [--project] [--ask-wait duration]

With no session, snoop opens the session picker.

Commands:
  export   write a session as Markdown, without thinking blocks
  hooks    manage the hooks that deliver review comments and replies
`)
	col := 0
	for _, fl := range allFlags(f.fs) {
		col = max(col, len(flagName(fl)))
	}
	for _, g := range flagGroups {
		var flags []*pflag.Flag
		for _, n := range g.names {
			flags = append(flags, f.fs.Lookup(n))
		}
		fmt.Fprintf(w, "\n%s:\n%s", g.title, flagTable(flags, col))
	}
	fmt.Fprintf(w, "\nSee %s for more.\n", docsURL)
}

// flagName is how the help names a flag, such as "-f, --follow" or
// "    --stall duration".
func flagName(fl *pflag.Flag) string {
	n := "    "
	if fl.Shorthand != "" {
		n = "-" + fl.Shorthand + ", "
	}
	n += "--" + fl.Name
	varname, _ := pflag.UnquoteUsage(fl)
	switch {
	case fl.Value.Type() == "bool":
	case fl.NoOptDefVal != "":
		n += "[=" + fl.NoOptDefVal + "]"
	case varname != "":
		n += " " + varname
	}
	return n
}

// flagTable lists flags as "  -f, --follow   what it does", the
// descriptions starting at least col columns in and wrapped to 80 columns.
func flagTable(flags []*pflag.Flag, col int) string {
	names := make([]string, len(flags))
	for i, fl := range flags {
		names[i] = flagName(fl)
		col = max(col, len(names[i]))
	}
	var b strings.Builder
	for i, fl := range flags {
		_, desc := pflag.UnquoteUsage(fl)
		if d := fl.DefValue; d != "" && d != "0" && d != "0s" && d != "false" && d != "[]" {
			if fl.Value.Type() == "duration" {
				d = shortDuration(d)
			}
			desc += " (default " + d + ")"
		}
		lead := fmt.Sprintf("  %-*s   ", col, names[i])
		b.WriteString(lead + wrapWords(desc, 80-len(lead), strings.Repeat(" ", len(lead))) + "\n")
	}
	return b.String()
}

// shortDuration drops the zero units Go prints, so 5m0s reads 5m.
func shortDuration(d string) string {
	if strings.HasSuffix(d, "m0s") {
		d = strings.TrimSuffix(d, "0s")
	}
	if strings.HasSuffix(d, "h0m") {
		d = strings.TrimSuffix(d, "0m")
	}
	return d
}

// wrapWords wraps s at width, starting each line after the first with pad.
func wrapWords(s string, width int, pad string) string {
	var b strings.Builder
	n := 0
	for i, word := range strings.Fields(s) {
		switch {
		case i == 0:
		case n+1+len(word) > width:
			b.WriteString("\n" + pad)
			n = 0
		default:
			b.WriteByte(' ')
			n++
		}
		b.WriteString(word)
		n += len(word)
	}
	return b.String()
}

// singleDashLong finds an argument such as -follow, a long flag given one
// dash, so the error can name the right spelling.
func singleDashLong(fs *pflag.FlagSet, args []string) (string, bool) {
	for _, a := range args {
		if a == "--" {
			break
		}
		if len(a) < 3 || a[0] != '-' || a[1] == '-' {
			continue
		}
		name, _, _ := strings.Cut(a[1:], "=")
		if fs.Lookup(name) != nil {
			return name, true
		}
	}
	return "", false
}

// parseFlags parses args, explaining a long flag given one dash.
func parseFlags(fs *pflag.FlagSet, args []string) error {
	if name, ok := singleDashLong(fs, args); ok {
		return fmt.Errorf("use --%s, with two dashes, not -%s", name, name)
	}
	return fs.Parse(args)
}

// parseCommand parses a command's flags, exiting after its help or on a
// mistake.
func parseCommand(fs *pflag.FlagSet, args []string) {
	err := parseFlags(fs, args)
	switch {
	case errors.Is(err, pflag.ErrHelp):
		os.Exit(0)
	case err != nil:
		fmt.Fprintf(os.Stderr, "snoop %s: %v\n\n", fs.Name(), err)
		fs.Usage()
		os.Exit(2)
	}
}

// allFlags is a command's flags in the order they were added.
func allFlags(fs *pflag.FlagSet) []*pflag.Flag {
	var out []*pflag.Flag
	fs.VisitAll(func(f *pflag.Flag) { out = append(out, f) })
	return out
}

// notifyFlag is --notify: bare it means auto, or it names a way to notify.
type notifyFlag string

func (f *notifyFlag) String() string { return string(*f) }

func (f *notifyFlag) Type() string { return "mode" }

func (f *notifyFlag) Set(v string) error {
	switch v {
	case notify.Auto, notify.Terminal, notify.System:
	default:
		return fmt.Errorf("want auto, terminal or system")
	}
	*f = notifyFlag(v)
	return nil
}
