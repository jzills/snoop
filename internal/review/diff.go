// Package review is everything behind the Diff tab that isn't UI: the git
// diff of a session's working tree, review comments and the text sent to
// the agent, the inbox the comments wait in, and the Claude Code hooks
// that deliver them.
package review

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Line is one line of a diff. Old and New are its line numbers on each
// side, 0 where it doesn't exist.
type Line struct {
	Kind     byte // ' ', '+' or '-'
	Old, New int
	Text     string
}

type Hunk struct {
	Header string
	Lines  []Line
}

// File is one changed file. Added and Removed count every line, even past
// the lines kept when a diff is truncated.
type File struct {
	Path           string // relative to the repository root, slash-separated
	Added, Removed int
	Hunks          []Hunk
	Binary         bool
	Untracked      bool
	Deleted        bool
	Truncated      bool
}

type Diff struct {
	Root  string // the repository's top-level directory
	Files []*File
}

var (
	ErrNotRepo = errors.New("not a git repository")
	ErrNoGit   = errors.New("git isn't installed")
)

// Limits that keep a generated file or a stray directory from swamping the
// tab.
const (
	maxUntracked = 200  // untracked files shown
	sniffBytes   = 8000 // read to tell binary files from text
)

// MaxLines is how many lines of a file's diff are kept.
const MaxLines = 3000

// Load runs git in dir and returns the working tree's changes against
// HEAD, with untracked files shown as added.
func Load(dir string) (*Diff, error) {
	top, err := run(dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, ErrNoGit) {
			return nil, err
		}
		return nil, ErrNotRepo
	}
	root := strings.TrimSpace(top)
	base := "HEAD"
	if _, err := run(root, nil, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		// No commits yet: compare with the empty tree.
		t, err := run(root, strings.NewReader(""), "hash-object", "-t", "tree", "--stdin")
		if err != nil {
			return nil, err
		}
		base = strings.TrimSpace(t)
	}
	out, err := run(root, nil, "diff", base, "--no-color", "--no-ext-diff", "-U3")
	if err != nil {
		return nil, err
	}
	d := &Diff{Root: root, Files: Parse(out)}
	others, err := run(root, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, p := range strings.Split(others, "\x00") {
		if p != "" {
			names = append(names, p)
		}
	}
	sort.Strings(names)
	for i, p := range names {
		if i == maxUntracked {
			break
		}
		d.Files = append(d.Files, untracked(root, p))
	}
	return d, nil
}

// run runs git in dir. GIT_OPTIONAL_LOCKS=0 keeps it from taking the index
// lock, which would get in the way of the agent's own git commands.
func run(dir string, stdin *strings.Reader, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "core.quotePath=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return "", ErrNoGit
	}
	if err != nil {
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// untracked shows a file git doesn't track as entirely added. It reads at
// most MaxLines lines, so a huge log or dataset costs no more than a small
// file; past that, Added counts only the lines read.
func untracked(root, path string) *File {
	f := &File{Path: path, Untracked: true}
	fh, err := os.Open(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return f
	}
	defer fh.Close()
	br := bufio.NewReaderSize(fh, sniffBytes)
	if head, _ := br.Peek(sniffBytes); bytes.IndexByte(head, 0) >= 0 {
		f.Binary = true
		return f
	}
	sc := bufio.NewScanner(br)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	var h Hunk
	for sc.Scan() {
		if len(h.Lines) == MaxLines {
			f.Truncated = true
			break
		}
		h.Lines = append(h.Lines, Line{Kind: '+', New: len(h.Lines) + 1, Text: strings.TrimSuffix(sc.Text(), "\r")})
	}
	if sc.Err() != nil {
		f.Truncated = true // a line longer than the buffer
	}
	if len(h.Lines) == 0 {
		return f
	}
	f.Added = len(h.Lines)
	h.Header = fmt.Sprintf("@@ -0,0 +1,%d @@", len(h.Lines))
	f.Hunks = []Hunk{h}
	return f
}

// Parse reads a unified diff as git prints it.
func Parse(out string) []*File {
	var files []*File
	var f *File
	var h *Hunk
	kept, oldN, newN := 0, 0, 0
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			f = &File{Path: gitPath(l)}
			files = append(files, f)
			h, kept = nil, 0
		case f == nil:
		case h == nil && strings.HasPrefix(l, "deleted file mode"):
			f.Deleted = true
		case h == nil && strings.HasPrefix(l, "Binary files "):
			f.Binary = true
		case h == nil && strings.HasPrefix(l, "rename to "):
			f.Path = unquote(strings.TrimPrefix(l, "rename to "))
		case h == nil && strings.HasPrefix(l, "+++ "):
			if p := strings.TrimPrefix(l, "+++ "); p != "/dev/null" {
				f.Path = strings.TrimPrefix(unquote(p), "b/")
			}
		case strings.HasPrefix(l, "@@ "):
			f.Hunks = append(f.Hunks, Hunk{Header: l})
			h = &f.Hunks[len(f.Hunks)-1]
			oldN, newN = hunkStart(l)
		case h == nil || l == "":
		case l[0] == '+' || l[0] == '-' || l[0] == ' ':
			ln := Line{Kind: l[0], Text: l[1:]}
			switch l[0] {
			case '+':
				f.Added++
				ln.New = newN
				newN++
			case '-':
				f.Removed++
				ln.Old = oldN
				oldN++
			default:
				ln.Old, ln.New = oldN, newN
				oldN++
				newN++
			}
			if kept < MaxLines {
				h.Lines = append(h.Lines, ln)
				kept++
			} else {
				f.Truncated = true
			}
		}
	}
	// Hunks past the limit have no lines left; drop them.
	for _, f := range files {
		n := 0
		for _, h := range f.Hunks {
			if len(h.Lines) > 0 {
				f.Hunks[n] = h
				n++
			}
		}
		f.Hunks = f.Hunks[:n]
	}
	return files
}

// gitPath takes the path from a "diff --git a/x b/x" line; the ---/+++
// lines that follow replace it when there are any.
func gitPath(l string) string {
	rest := strings.TrimPrefix(l, "diff --git ")
	if strings.HasPrefix(rest, "\"") {
		// Quoted: the second quoted string is b/….
		if i := strings.Index(rest, "\" \""); i >= 0 {
			return strings.TrimPrefix(unquote(rest[i+2:]), "b/")
		}
	}
	if i := strings.Index(rest, " b/"); i >= 0 {
		return rest[i+3:]
	}
	return rest
}

// unquote undoes git's C-style quoting of unusual paths.
func unquote(p string) string {
	if !strings.HasPrefix(p, "\"") {
		return p
	}
	if s, err := strconv.Unquote(p); err == nil {
		return s
	}
	return strings.Trim(p, "\"")
}

// hunkStart reads the first line numbers from "@@ -a,b +c,d @@".
func hunkStart(h string) (old, nw int) {
	f := strings.Fields(h)
	if len(f) < 3 {
		return 1, 1
	}
	num := func(s string) int {
		s, _, _ = strings.Cut(s[1:], ",")
		n, _ := strconv.Atoi(s)
		return n
	}
	return num(f[1]), num(f[2])
}
