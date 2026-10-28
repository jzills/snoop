package review

import (
	"fmt"
	"strings"
)

// Comment is a review comment on some lines of a file, or on the whole
// file when Lines is empty. It quotes the lines, so it still makes sense if
// the file changes before the agent reads it.
type Comment struct {
	Path  string
	Lines []Line
	Text  string
}

// Covers reports whether the comment is on line l of path.
func (c Comment) Covers(path string, l Line) bool {
	if c.Path != path {
		return false
	}
	for _, x := range c.Lines {
		if x == l {
			return true
		}
	}
	return false
}

// num is the line's number in the file as it is now, or in the old file for
// a removed line.
func num(l Line) int {
	if l.Kind == '-' {
		return l.Old
	}
	return l.New
}

// where is "path:40-42", "path:7 (removed)" or "path (whole file)".
func (c Comment) where() string {
	if len(c.Lines) == 0 {
		return c.Path + " (whole file)"
	}
	removed := true
	lo, hi := 0, 0
	for _, l := range c.Lines {
		if l.Kind != '-' {
			removed = false
		}
	}
	for _, l := range c.Lines {
		if l.Kind == '-' && !removed {
			continue
		}
		n := num(l)
		if lo == 0 || n < lo {
			lo = n
		}
		hi = max(hi, n)
	}
	s := fmt.Sprintf("%s:%d", c.Path, lo)
	if hi != lo {
		s += fmt.Sprintf("-%d", hi)
	}
	if removed {
		s += " (removed)"
	}
	return s
}

// Format is the message the agent receives.
func Format(cs []Comment) string {
	var b strings.Builder
	b.WriteString("Review comments from snoop:\n")
	for _, c := range cs {
		b.WriteString("\n" + c.where() + "\n")
		w := 0
		for _, l := range c.Lines {
			w = max(w, len(fmt.Sprint(num(l))))
		}
		for _, l := range c.Lines {
			fmt.Fprintf(&b, "    %*d %c%s\n", w, num(l), l.Kind, l.Text)
		}
		b.WriteString(c.Text + "\n")
	}
	return b.String()
}
