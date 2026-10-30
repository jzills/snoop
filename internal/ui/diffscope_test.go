package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// branchApp opens a session in a repository on feat/x, which left develop
// two commits ago and has an uncommitted change on top.
func branchApp(t *testing.T) (*App, *view, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	os.MkdirAll(repo, 0o755)
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	commit := func(file, body string) {
		os.WriteFile(filepath.Join(repo, file), []byte(body), 0o644)
		git("add", ".")
		git("commit", "-qm", file)
	}
	git("init", "-q", "-b", "main")
	commit("a.go", "package a\n")
	git("checkout", "-qb", "develop")
	commit("d.go", "package d\n")
	git("checkout", "-qb", "feat/x")
	commit("b.go", "package b\n")
	commit("c.go", "package c\n")
	os.WriteFile(filepath.Join(repo, "b.go"), []byte("package b\n\nvar y = 1\n"), 0o644)

	root := filepath.Join(tmp, "projects")
	os.MkdirAll(filepath.Join(root, "p"), 0o755)
	sess := filepath.Join(root, "p", "s.jsonl")
	os.WriteFile(sess, []byte(`{"type":"user","uuid":"u1","cwd":"`+filepath.ToSlash(repo)+`","timestamp":"2026-10-24T10:00:00Z","message":{"content":"go"}}`+"\n"), 0o644)
	a := New(Options{Root: root})
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	v, err := openView(sess, sess, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.close)
	a.push(v)
	return a, v, repo
}

func diffPaths(v *view) string {
	var ps []string
	for _, f := range v.diffFiles() {
		ps = append(ps, f.Path)
	}
	return strings.Join(ps, " ")
}

func TestDiffOpensInBranchScope(t *testing.T) {
	a, v, _ := branchApp(t)
	press(a, "5")
	if got := diffPaths(v); got != "b.go c.go" {
		t.Fatalf("branch files = %q", got)
	}
	out := ansi.Strip(a.View())
	for _, want := range []string{"Uncommitted   Branch · vs develop · 2 commits", "b switch"} {
		if !strings.Contains(out, want) {
			t.Errorf("Diff tab lacks %q:\n%s", want, out)
		}
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 || (f[0] != "b.go" && f[0] != "▌b.go" && f[0] != "c.go" && f[0] != "▌c.go") {
			continue
		}
		if want := strings.HasSuffix(f[0], "b.go"); strings.Contains(l, "uncommitted") != want {
			t.Errorf("row %q: uncommitted should be %v", l, want)
		}
	}
	// b switches to the uncommitted changes, and back.
	press(a, "b")
	if got := diffPaths(v); got != "b.go" || v.branchShown() {
		t.Errorf("uncommitted files = %q", got)
	}
	press(a, "b")
	if got := diffPaths(v); got != "b.go c.go" {
		t.Errorf("back to branch = %q", got)
	}
}

func TestDiffPickBase(t *testing.T) {
	a, v, repo := branchApp(t)
	press(a, "5", "B")
	if !v.rev.picking || len(v.rev.bases) < 2 || v.rev.bases[v.rev.bCursor].Name != "develop" {
		t.Fatalf("picking %v, bases %+v, cursor %d", v.rev.picking, v.rev.bases, v.rev.bCursor)
	}
	out := ansi.Strip(a.View())
	if !strings.Contains(out, "Compare the branch with") || !strings.Contains(out, "main") {
		t.Errorf("base list:\n%s", out)
	}
	press(a, "down", "enter")
	if v.rev.picking || v.rev.diff.Base != "main" || diffPaths(v) != "b.go c.go d.go" {
		t.Errorf("after picking main: base %q, files %q", v.rev.diff.Base, diffPaths(v))
	}
	cmd := exec.Command("git", "-C", repo, "config", "--get", "branch.feat/x.snoopBase")
	if out, _ := cmd.Output(); strings.TrimSpace(string(out)) != "main" {
		t.Errorf("saved base = %q", out)
	}
	press(a, "B", "esc")
	if v.rev.picking || a.top() != v {
		t.Error("esc should close the list and stay in the session")
	}
}

func TestDiffAutoScopeWithoutCommits(t *testing.T) {
	a, v, repo := branchApp(t)
	if out, err := exec.Command("git", "-C", repo, "checkout", "-qf", "main").CombinedOutput(); err != nil {
		t.Fatalf("checkout: %v %s", err, out)
	}
	press(a, "5")
	if v.branchShown() {
		t.Errorf("on main, with nothing ahead, the tab should open uncommitted: %+v", v.rev.diff)
	}
}
