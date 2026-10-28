package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/review"
	"github.com/jzills/snoop/internal/tail"
)

// diffApp opens a session whose working directory is a git repository with
// a.go changed by the session and b.go changed by someone else. hooks
// installs snoop's hooks in the user settings beside the transcripts.
func diffApp(t *testing.T, hooks bool) (*App, *view, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	repo := filepath.Join(tmp, "repo")
	os.MkdirAll(repo, 0o755)
	gitRun := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitRun("init", "-q")
	os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n\nvar x = 1\nvar y = 1\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "b.go"), []byte("package b\n"), 0o644)
	gitRun("add", ".")
	gitRun("commit", "-qm", "init")
	os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n\nvar x = 2\nvar y = 2\nvar z = 3\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "b.go"), []byte("package b // changed\n"), 0o644)

	root := filepath.Join(tmp, "projects")
	os.MkdirAll(filepath.Join(root, "p"), 0o755)
	sess := filepath.Join(root, "p", "sess-1.jsonl")
	fx := `{"type":"user","uuid":"u1","cwd":"` + filepath.ToSlash(repo) + `","sessionId":"sess-1","timestamp":"2026-10-24T10:00:00Z","message":{"content":"edit"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:01Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"` + filepath.ToSlash(filepath.Join(repo, "a.go")) + `","old_string":"1","new_string":"2"}}]}}
{"type":"user","uuid":"r1","timestamp":"2026-10-24T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
`
	os.WriteFile(sess, []byte(fx), 0o644)
	if hooks {
		bin := filepath.Join(tmp, "snoop")
		os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
		if _, err := review.Install(review.UserSettings(root), `"`+bin+`" hook`); err != nil {
			t.Fatal(err)
		}
	}
	a := New(Options{Root: root})
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	v, err := openView(sess, sess, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if a.isOpen(sess) {
			v.close()
		}
	})
	a.push(v)
	return a, v, filepath.Join(tmp, "state", "snoop", "inbox", "sess-1")
}

// press sends keys and runs what they return, as Bubble Tea would, except
// for the tail watcher and clipboard writes.
func press(a *App, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		default:
			msg = key(k)
		}
		_, cmd := a.Update(msg)
		runCmd(a, cmd)
	}
}

func runCmd(a *App, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch m := cmd().(type) {
	case diffMsg:
		_, next := a.Update(m)
		runCmd(a, next)
	case sentMsg:
		a.Update(m) // its cmd writes to the clipboard
	case reviewPollMsg:
		_, next := a.Update(m)
		runCmd(a, next)
	case tea.BatchMsg:
		for _, c := range m {
			runCmd(a, c)
		}
	}
}

func typeComment(a *App, s string) {
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	press(a, "enter")
}

func inbox(t *testing.T, dir string) []string {
	t.Helper()
	es, _ := os.ReadDir(dir)
	var out []string
	for _, e := range es {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		out = append(out, string(b))
	}
	return out
}

func TestDiffTabLoads(t *testing.T) {
	a, v, _ := diffApp(t, true)
	press(a, "5")
	out := ansi.Strip(a.View())
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "var x = 2") {
		t.Fatalf("view:\n%s", out)
	}
	fs := v.diffFiles()
	if len(fs) != 2 || fs[0].Path != "a.go" || !v.rev.ours["a.go"] || v.rev.ours["b.go"] {
		t.Errorf("files = %v, ours = %v", fs, v.rev.ours)
	}
}

func TestDiffNotRepo(t *testing.T) {
	a, v, _ := diffApp(t, true)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	v.sess.CWD = t.TempDir()
	press(a, "5")
	if out := ansi.Strip(a.View()); !strings.Contains(out, "not a git repository") {
		t.Errorf("view:\n%s", out)
	}
}

func TestCommentLineAndSend(t *testing.T) {
	a, v, dir := diffApp(t, true)
	press(a, "5", "tab", "j", "c")
	if !v.rev.typing {
		t.Fatal("c didn't open the comment input")
	}
	typeComment(a, "fix")
	if len(v.rev.comments) != 1 || len(v.rev.comments[0].Lines) != 1 {
		t.Fatalf("comments = %+v", v.rev.comments)
	}
	press(a, "S")
	got := inbox(t, dir)
	if len(got) != 1 || !strings.Contains(got[0], "a.go:") || !strings.Contains(got[0], "fix") {
		t.Fatalf("inbox = %q", got)
	}
	if len(v.rev.comments) != 0 {
		t.Error("comments kept after sending")
	}
	if out := ansi.Strip(a.View()); !strings.Contains(out, "sent 1 comment") {
		t.Errorf("footer:\n%s", out)
	}
	// Once the hook takes it, the footer says so.
	text, done, _ := review.Claim(filepath.Dir(dir), "sess-1", v.sess.End)
	done()
	if !strings.Contains(text, "fix") {
		t.Errorf("claimed %q", text)
	}
	runCmd(a, v.pollReview(time.Now()))
	press(a, "j")
	if out := ansi.Strip(a.View()); !strings.Contains(out, "delivered 1 comment") {
		t.Errorf("footer after delivery:\n%s", out)
	}
}

func TestCommentRange(t *testing.T) {
	a, v, _ := diffApp(t, true)
	press(a, "5", "tab", "v", "j", "j", "c")
	typeComment(a, "these")
	if len(v.rev.comments) != 1 || len(v.rev.comments[0].Lines) != 3 {
		t.Fatalf("comments = %+v", v.rev.comments)
	}
	if v.rev.anchor != -1 {
		t.Error("range kept after commenting")
	}
}

func TestFileComment(t *testing.T) {
	a, v, _ := diffApp(t, true)
	press(a, "5", "C")
	typeComment(a, "split this")
	if len(v.rev.comments) != 1 || !strings.Contains(review.Format([]review.Comment{v.rev.comments[0].Comment}), "a.go (whole file)") {
		t.Fatalf("comments = %+v", v.rev.comments)
	}
	// C again edits it rather than adding another.
	press(a, "C")
	if v.rev.input.Value() != "split this" {
		t.Errorf("input = %q", v.rev.input.Value())
	}
}

func TestSendWithoutHooks(t *testing.T) {
	a, v, dir := diffApp(t, false)
	press(a, "5", "C")
	typeComment(a, "x")
	press(a, "S")
	if got := inbox(t, dir); len(got) != 0 {
		t.Errorf("inbox = %q", got)
	}
	if len(v.rev.comments) != 1 {
		t.Error("comment dropped")
	}
	if !strings.Contains(ansi.Strip(v.flash), "snoop hooks install") {
		t.Errorf("flash = %q", ansi.Strip(v.flash))
	}
}

func TestDeleteEditComment(t *testing.T) {
	a, v, _ := diffApp(t, true)
	press(a, "5", "tab", "j", "c")
	typeComment(a, "first")
	press(a, "e")
	if !v.rev.typing || v.rev.input.Value() != "first" {
		t.Fatalf("e: typing %v, %q", v.rev.typing, v.rev.input.Value())
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	press(a, "enter")
	if len(v.rev.comments) != 1 || v.rev.comments[0].Text != "first!" {
		t.Fatalf("comments = %+v", v.rev.comments)
	}
	press(a, "x")
	if len(v.rev.comments) != 0 {
		t.Error("x didn't delete")
	}
}

func TestLeaveWithUnsent(t *testing.T) {
	a, v, _ := diffApp(t, true)
	press(a, "5", "C")
	typeComment(a, "x")
	press(a, "esc")
	if a.top() != v || !strings.Contains(ansi.Strip(v.flash), "1 unsent comment") {
		t.Fatalf("esc left, or didn't ask: %q", ansi.Strip(v.flash))
	}
	press(a, "esc")
	if a.top() != v || len(v.rev.comments) != 1 {
		t.Fatal("esc at the prompt didn't stay")
	}
	press(a, "esc", "d")
	if a.top() != nil {
		t.Error("d didn't leave")
	}
}

func TestTypingCommentKeepsKeys(t *testing.T) {
	a, v, _ := diffApp(t, true)
	press(a, "5", "tab", "c")
	// q and esc-free keys go into the comment, not to the app.
	a.Update(key("q"))
	a.Update(key("1"))
	if v.rev.input.Value() != "q1" || v.tab != tabDiff {
		t.Errorf("input %q, tab %d", v.rev.input.Value(), v.tab)
	}
	press(a, "esc")
	if v.rev.typing || len(v.rev.comments) != 0 || a.top() != v {
		t.Error("esc didn't just cancel the comment")
	}
}

// A refresh can leave the selected file with no lines while they have
// focus; G there used to index an empty list.
func TestGOnFileWithoutLines(t *testing.T) {
	a, v, _ := diffApp(t, true)
	press(a, "5", "tab")
	v.rev.diff = &review.Diff{Root: "/r", Files: []*review.File{{Path: "img.png", Binary: true}}}
	v.rev.cursor = 0
	press(a, "G", "end", "c", "e", "x", "K")
	if v.focusR {
		t.Error("focus stayed on a file without lines")
	}
	if ms, _ := filepath.Glob("sess-1-*.md"); len(ms) > 0 {
		t.Errorf("e on the file list exported the session: %v", ms)
	}
	a.View()
}

// Comments added, edited or deleted while a send is under way are kept or
// dropped exactly as the user left them, and a second S waits.
func TestSendRemovesOnlyWhatItSent(t *testing.T) {
	a, v, dir := diffApp(t, true)
	press(a, "5", "tab", "c")
	typeComment(a, "first")
	_, send := a.Update(key("S"))
	if _, again := a.Update(key("S")); again != nil {
		t.Error("a second S sent again")
	}
	press(a, "j", "c")
	typeComment(a, "second")
	press(a, "k", "e") // edit "first" while it is being sent
	runCmd(a, send)
	if len(v.rev.comments) != 1 || v.rev.comments[0].Text != "second" {
		t.Fatalf("comments = %+v", v.rev.comments)
	}
	typeComment(a, " more")
	if len(v.rev.comments) != 2 {
		t.Errorf("an edit of a sent comment was lost: %+v", v.rev.comments)
	}
	if got := inbox(t, dir); len(got) != 1 || strings.Contains(got[0], "second") {
		t.Errorf("inbox = %q", got)
	}
}

func TestOwnPathsThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real", "repo")
	os.MkdirAll(filepath.Join(real, "sub"), 0o755)
	link := filepath.Join(tmp, "link")
	os.Symlink(filepath.Join(tmp, "real"), link)
	cwd := filepath.Join(link, "repo", "sub")
	ours := ownPaths(real, cwd, []string{filepath.Join(cwd, "a.go"), filepath.Join(link, "repo", "b.go"), "/elsewhere/c.go"})
	if !ours["sub/a.go"] || !ours["b.go"] || len(ours) != 2 {
		t.Errorf("ours = %v", ours)
	}
}

func TestRefreshClearsRange(t *testing.T) {
	a, v, _ := diffApp(t, true)
	press(a, "5", "tab", "v", "j")
	repo := v.sess.CWD
	os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n\nvar w = 0\nvar x = 2\nvar y = 2\nvar z = 3\n"), 0o644)
	press(a, "r")
	if v.rev.anchor != -1 {
		t.Error("range kept across a refresh that changed the file")
	}
}

func TestQuitWithUnsentAsks(t *testing.T) {
	a, v, _ := diffApp(t, true)
	press(a, "5", "C")
	typeComment(a, "x")
	if _, cmd := a.Update(key("q")); cmd != nil {
		t.Fatal("q quit with an unsent comment")
	}
	if !strings.Contains(ansi.Strip(v.flash), "discard and quit") {
		t.Errorf("flash = %q", ansi.Strip(v.flash))
	}
	_, cmd := a.Update(key("d"))
	if cmd == nil {
		t.Fatal("d didn't quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("d didn't quit")
	}
}

func TestFollowWaitsForUnsentComments(t *testing.T) {
	a, v, _ := diffApp(t, true)
	a.opts.Follow = true
	press(a, "5", "C")
	typeComment(a, "x")
	a.Update(tailMsg{v: v, u: tail.Update{NewSession: "/next.jsonl"}})
	if a.top() != v || a.followNext != "/next.jsonl" {
		t.Fatalf("followed away from unsent comments (next %q)", a.followNext)
	}
	v.rev.comments = nil
	_, cmd := a.Update(tickMsg{})
	found := false
	for _, c := range cmd().(tea.BatchMsg) {
		if c == nil {
			continue
		}
		if m, ok := c().(openMsg); ok && m.path == "/next.jsonl" {
			found = true
		}
	}
	if !found || a.followNext != "" {
		t.Error("the held-back session didn't open once the comments were gone")
	}
}

// Subagents' edits count as the session's, and a subagent writing more
// triggers a refresh.
func TestSubagentEdits(t *testing.T) {
	a, v, _ := diffApp(t, true)
	sub := filepath.Join(strings.TrimSuffix(v.rootPath, ".jsonl"), "subagents")
	os.MkdirAll(sub, 0o755)
	agent := filepath.Join(sub, "agent-x1.jsonl")
	line := `{"type":"assistant","uuid":"s1","timestamp":"2026-10-24T10:00:05Z","message":{"id":"x1","content":[{"type":"tool_use","id":"st1","name":"Edit","input":{"file_path":"` + filepath.ToSlash(filepath.Join(v.sess.CWD, "b.go")) + `","old_string":"a","new_string":"b"}}]}}` + "\n"
	os.WriteFile(agent, []byte(line), 0o644)
	press(a, "5")
	if !v.rev.ours["b.go"] {
		t.Fatalf("a subagent's edit isn't the session's: %v", v.rev.ours)
	}
	f, _ := os.OpenFile(agent, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(line)
	f.Close()
	v.rev.at = time.Time{}
	seq := v.rev.seq
	runCmd(a, v.pollReview(time.Now()))
	if v.rev.seq == seq {
		t.Error("a growing subagent transcript didn't refresh the diff")
	}
}
