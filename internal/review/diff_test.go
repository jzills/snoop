package review

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureDiff = `diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -1,3 +1,3 @@ package a
 package a
-var x = 1
+var x = 2

@@ -10,3 +10,4 @@ func f() {
 	a()
+	b()
 	c()
 }
\ No newline at end of file
diff --git a/new.go b/new.go
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/new.go
@@ -0,0 +1,2 @@
+package n
+var y = 1
diff --git a/old.go b/old.go
deleted file mode 100644
index 4444444..0000000
--- a/old.go
+++ /dev/null
@@ -1 +0,0 @@
-package old
diff --git a/img.png b/img.png
index 5555555..6666666 100644
Binary files a/img.png and b/img.png differ
`

func TestParse(t *testing.T) {
	fs := Parse(fixtureDiff)
	if len(fs) != 4 {
		t.Fatalf("got %d files", len(fs))
	}
	a, n, o, img := fs[0], fs[1], fs[2], fs[3]
	if a.Path != "a.go" || len(a.Hunks) != 2 || a.Added != 2 || a.Removed != 1 {
		t.Errorf("a.go = %+v", a)
	}
	h := a.Hunks[1]
	if h.Header != "@@ -10,3 +10,4 @@ func f() {" || len(h.Lines) != 4 {
		t.Fatalf("hunk = %+v", h)
	}
	if l := h.Lines[1]; l.Kind != '+' || l.New != 11 || l.Old != 0 || l.Text != "\tb()" {
		t.Errorf("added line = %+v", l)
	}
	if l := h.Lines[2]; l.Kind != ' ' || l.Old != 11 || l.New != 12 {
		t.Errorf("context line = %+v", l)
	}
	if l := a.Hunks[0].Lines[1]; l.Kind != '-' || l.Old != 2 || l.New != 0 {
		t.Errorf("removed line = %+v", l)
	}
	if n.Path != "new.go" || n.Added != 2 || n.Deleted {
		t.Errorf("new.go = %+v", n)
	}
	if o.Path != "old.go" || !o.Deleted || o.Removed != 1 {
		t.Errorf("old.go = %+v", o)
	}
	if img.Path != "img.png" || !img.Binary {
		t.Errorf("img.png = %+v", img)
	}
}

func TestParseQuotedPath(t *testing.T) {
	d := "diff --git \"a/dir/na\\303\\257ve file.go\" \"b/dir/na\\303\\257ve file.go\"\n" +
		"--- \"a/dir/na\\303\\257ve file.go\"\n+++ \"b/dir/na\\303\\257ve file.go\"\n@@ -1 +1 @@\n-a\n+b\n"
	fs := Parse(d)
	if len(fs) != 1 || fs[0].Path != "dir/naïve file.go" {
		t.Fatalf("got %+v", fs)
	}
}

func TestParseTruncates(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/big b/big\n--- a/big\n+++ b/big\n@@ -0,0 +1,3500 @@\n")
	for range 3500 {
		b.WriteString("+x\n")
	}
	f := Parse(b.String())[0]
	n := 0
	for _, h := range f.Hunks {
		n += len(h.Lines)
	}
	if n != MaxLines || !f.Truncated || f.Added != 3500 {
		t.Errorf("lines %d, truncated %v, added %d", n, f.Truncated, f.Added)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRepo makes a repository with a committed a.go, then changes it and
// adds an untracked and an ignored file.
func testRepo(t *testing.T) string {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "a.go"), "package a\n\nvar x = 1\n")
	write(t, filepath.Join(dir, ".gitignore"), "ignored.txt\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "init")
	write(t, filepath.Join(dir, "a.go"), "package a\n\nvar x = 2\n")
	write(t, filepath.Join(dir, "new.txt"), "one\ntwo\n")
	write(t, filepath.Join(dir, "ignored.txt"), "no\n")
	return dir
}

func TestLoad(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := testRepo(t)
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	if got, _ := filepath.EvalSymlinks(d.Root); got != want {
		t.Errorf("root = %q, want %q", d.Root, want)
	}
	var paths []string
	for _, f := range d.Files {
		paths = append(paths, f.Path)
	}
	if strings.Join(paths, " ") != "a.go new.txt" {
		t.Fatalf("paths = %v", paths)
	}
	n := d.Files[1]
	if !n.Untracked || n.Added != 2 || n.Hunks[0].Lines[1] != (Line{Kind: '+', New: 2, Text: "two"}) {
		t.Errorf("new.txt = %+v", n)
	}
}

func TestLoadNoCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "staged.go"), "package s\n")
	write(t, filepath.Join(dir, "loose.go"), "package l\n")
	git(t, dir, "add", "staged.go")
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range d.Files {
		paths = append(paths, f.Path)
	}
	if strings.Join(paths, " ") != "staged.go loose.go" {
		t.Errorf("paths = %v", paths)
	}
}

func TestLoadNotRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	if _, err := Load(t.TempDir()); !errors.Is(err, ErrNotRepo) {
		t.Errorf("err = %v", err)
	}
}

func TestLoadBinaryUntracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := testRepo(t)
	write(t, filepath.Join(dir, "blob.bin"), "a\x00b")
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range d.Files {
		if f.Path == "blob.bin" && (!f.Binary || len(f.Hunks) != 0) {
			t.Errorf("blob.bin = %+v", f)
		}
	}
}

func TestUntrackedTruncates(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "big.log"), strings.Repeat("line\n", 3500))
	f := untracked(dir, "big.log")
	if len(f.Hunks) != 1 || len(f.Hunks[0].Lines) != MaxLines || !f.Truncated || f.Added != MaxLines {
		t.Errorf("hunks %d, truncated %v, added %d", len(f.Hunks), f.Truncated, f.Added)
	}
	write(t, filepath.Join(dir, "long.txt"), strings.Repeat("x", 2<<20))
	if f := untracked(dir, "long.txt"); !f.Truncated {
		t.Error("a line past the buffer isn't marked truncated")
	}
}
