package review

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// branchRepo makes a repository whose main has one commit, a develop with
// one more, and a feature branch off develop with two commits of its own,
// then leaves an uncommitted change and an untracked file on the feature
// branch.
func branchRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	commit := func(file, body, msg string) {
		write(t, filepath.Join(dir, file), body)
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-qm", msg)
	}
	git(t, dir, "init", "-q", "-b", "main")
	commit("a.go", "package a\n", "init")
	git(t, dir, "checkout", "-qb", "develop")
	commit("d.go", "package d\n", "develop work")
	git(t, dir, "checkout", "-qb", "feat/x")
	commit("b.go", "package b\n", "one")
	commit("c.go", "package c\n", "two")
	write(t, filepath.Join(dir, "b.go"), "package b\n\nvar y = 1\n")
	write(t, filepath.Join(dir, "new.txt"), "hi\n")
	return dir
}

func paths(d *Diff) string {
	var ps []string
	for _, f := range d.Files {
		ps = append(ps, f.Path)
	}
	return strings.Join(ps, " ")
}

func TestLoadBranch(t *testing.T) {
	dir := branchRepo(t)
	d, err := LoadBranch(dir, "develop")
	if err != nil {
		t.Fatal(err)
	}
	// develop's own d.go isn't the branch's change.
	if got := paths(d); got != "b.go c.go new.txt" {
		t.Fatalf("paths = %q", got)
	}
	if d.Base != "develop" || d.Commits != 2 {
		t.Errorf("base %q, commits %d", d.Base, d.Commits)
	}
	for _, f := range d.Files {
		if want := f.Path != "c.go"; f.Uncommitted != want {
			t.Errorf("%s uncommitted = %v", f.Path, f.Uncommitted)
		}
	}
	// New-side line numbers are the working tree's.
	b := d.Files[0]
	if last := b.Hunks[0].Lines[len(b.Hunks[0].Lines)-1]; last.New != 3 || last.Text != "var y = 1" {
		t.Errorf("b.go's last line = %+v", last)
	}
}

func TestGuessBasePrefersTheClosest(t *testing.T) {
	dir := branchRepo(t)
	if got := GuessBase(dir); got != "develop" {
		t.Errorf("GuessBase = %q, want develop", got)
	}
}

func TestGuessBaseTieGoesToDevelop(t *testing.T) {
	dir := branchRepo(t)
	git(t, dir, "branch", "-f", "main", "develop")
	if got := GuessBase(dir); got != "develop" {
		t.Errorf("GuessBase = %q, want develop", got)
	}
}

func TestGuessBasePrefersRemoteTracking(t *testing.T) {
	dir := branchRepo(t)
	// origin/develop is develop; the local develop has fallen behind.
	git(t, dir, "update-ref", "refs/remotes/origin/develop", "develop")
	git(t, dir, "branch", "-f", "develop", "main")
	if got := GuessBase(dir); got != "origin/develop" {
		t.Errorf("GuessBase = %q, want origin/develop", got)
	}
}

func TestGuessBaseFromConfig(t *testing.T) {
	dir := branchRepo(t)
	git(t, dir, "config", "snoop.base", "main")
	if got := GuessBase(dir); got != "main" {
		t.Errorf("snoop.base: GuessBase = %q", got)
	}
	if err := SaveBase(dir, "develop"); err != nil {
		t.Fatal(err)
	}
	if got := GuessBase(dir); got != "develop" {
		t.Errorf("after SaveBase: GuessBase = %q", got)
	}
	// A base that no longer resolves falls through.
	git(t, dir, "config", "branch.feat/x.snoopBase", "gone")
	git(t, dir, "config", "--unset", "snoop.base")
	if got := GuessBase(dir); got != "develop" {
		t.Errorf("with a gone base: GuessBase = %q", got)
	}
}

func TestGuessBaseNone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "work")
	write(t, filepath.Join(dir, "a.go"), "package a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "init")
	if got := GuessBase(dir); got != "" {
		t.Errorf("GuessBase = %q, want none", got)
	}
}

func TestBases(t *testing.T) {
	dir := branchRepo(t)
	git(t, dir, "branch", "spike", "main")
	bs := Bases(dir)
	var names []string
	for _, b := range bs {
		names = append(names, b.Name)
	}
	got := strings.Join(names, " ")
	if !strings.HasPrefix(got, "develop main") || !strings.Contains(got, "spike") || strings.Contains(got, "feat/x") {
		t.Errorf("Bases = %q", got)
	}
	if bs[0].Ahead != 2 || bs[1].Ahead != 3 {
		t.Errorf("ahead = %d, %d", bs[0].Ahead, bs[1].Ahead)
	}
}
