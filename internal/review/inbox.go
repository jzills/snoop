package review

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The inbox holds sent comments until the hook delivers them: one
// directory per session, one file per send. Sends are written under a
// temporary name and renamed into place, and the hook claims each by
// renaming it, so a send is never read half-written or delivered twice.

const (
	sentExt    = ".txt"
	claimedExt = ".claimed"
	// A claimed file still there after this was orphaned by a hook that
	// died before delivering it, so it is delivered again.
	orphanAge = time.Minute
)

// StateDir is where snoop keeps what it hands to its hooks: the inbox,
// permission requests and the listening file.
func StateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "snoop")
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "snoop")
	case "windows":
		if d, err := os.UserConfigDir(); err == nil {
			return filepath.Join(d, "snoop")
		}
	}
	return filepath.Join(home, ".local", "state", "snoop")
}

// InboxDir is where snoop keeps comments waiting to be delivered.
func InboxDir() string { return filepath.Join(StateDir(), "inbox") }

// sessionDir keeps a session id from naming anything outside the inbox.
func sessionDir(dir, session string) (string, error) {
	if session == "" || strings.ContainsAny(session, `/\`) || session == "." || session == ".." {
		return "", fmt.Errorf("bad session id %q", session)
	}
	return filepath.Join(dir, session), nil
}

// Send queues text for a session and returns the file it was written to,
// which disappears once the text is delivered.
func Send(dir, session, text string, now time.Time) (string, error) {
	d, err := sessionDir(dir, session)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%020d-%d", now.UnixNano(), os.Getpid())
	tmp := filepath.Join(d, name+".tmp")
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		return "", err
	}
	path := filepath.Join(d, name+sentExt)
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// Waiting reports whether a sent file is still waiting to be delivered.
func Waiting(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Claim takes every text waiting for a session, oldest first, joined by a
// blank line; "" when nothing waits. Each text is returned by one Claim
// only, however many run at once. Call done once the text has been passed
// on: until then the claimed files stay, and are delivered again if done is
// never called.
func Claim(dir, session string, now time.Time) (text string, done func(), err error) {
	done = func() {}
	d, err := sessionDir(dir, session)
	if err != nil {
		return "", done, err
	}
	entries, err := os.ReadDir(d)
	if errors.Is(err, fs.ErrNotExist) {
		return "", done, nil
	}
	if err != nil {
		return "", done, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, sentExt) || orphaned(n, now) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var texts, claimed []string
	for _, n := range names {
		base, _, _ := strings.Cut(n, sentExt)
		to := filepath.Join(d, fmt.Sprintf("%s%s.%d-%d%s", base, sentExt, now.UnixNano(), os.Getpid(), claimedExt))
		if os.Rename(filepath.Join(d, n), to) != nil {
			continue // another hook got it first
		}
		b, err := os.ReadFile(to)
		if err != nil {
			continue
		}
		texts = append(texts, strings.TrimRight(string(b), "\n"))
		claimed = append(claimed, to)
	}
	if len(texts) == 0 {
		return "", done, nil
	}
	done = func() {
		for _, p := range claimed {
			os.Remove(p)
		}
	}
	return strings.Join(texts, "\n\n") + "\n", done, nil
}

// orphaned reports whether a claimed file was claimed more than orphanAge
// ago. The claim time is in the name, as a rename keeps the old mtime.
func orphaned(name string, now time.Time) bool {
	if !strings.HasSuffix(name, claimedExt) {
		return false
	}
	_, rest, ok := strings.Cut(name, sentExt+".")
	if !ok {
		return false
	}
	ns, _, _ := strings.Cut(rest, "-")
	n, err := strconv.ParseInt(ns, 10, 64)
	return err == nil && now.Sub(time.Unix(0, n)) > orphanAge
}
