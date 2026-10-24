package tail

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitFor(t *testing.T, tl *Tailer) Update {
	t.Helper()
	ch := make(chan Update, 1)
	go func() { ch <- tl.Wait() }()
	select {
	case u := <-ch:
		return u
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for update")
	}
	return Update{}
}

func TestTail(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.jsonl")
	os.WriteFile(p, []byte("{\"x\":1}\n"), 0o644)

	tl, _ := Open(p, 8, true)
	defer tl.Close()

	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("{\"x\":2}\n{\"x\":")
	u := waitFor(t, tl)
	if len(u.Lines) != 1 || string(u.Lines[0]) != `{"x":2}` {
		t.Fatalf("lines = %q", u.Lines)
	}
	f.WriteString("3}\n")
	f.Close()
	u = waitFor(t, tl)
	if len(u.Lines) != 1 || string(u.Lines[0]) != `{"x":3}` {
		t.Fatalf("partial line not completed: %q", u.Lines)
	}

	np := filepath.Join(dir, "b.jsonl")
	os.WriteFile(np, nil, 0o644)
	if u := waitFor(t, tl); u.NewSession != np {
		t.Fatalf("new session = %q", u.NewSession)
	}
}

func TestCloseUnblocksWait(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	os.WriteFile(p, nil, 0o644)
	tl, _ := Open(p, 0, false)
	go func() { time.Sleep(50 * time.Millisecond); tl.Close() }()
	if u := waitFor(t, tl); u.Err != ErrClosed {
		t.Fatalf("err = %v", u.Err)
	}
}

func TestTruncatedFileIsReread(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	os.WriteFile(p, []byte("{\"x\":1}\n{\"x\":2}\n"), 0o644)
	tl, _ := Open(p, 16, false)
	defer tl.Close()

	os.WriteFile(p, []byte("{\"y\":1}\n"), 0o644)
	u := waitFor(t, tl)
	if !u.Reset || len(u.Lines) != 1 || string(u.Lines[0]) != `{"y":1}` {
		t.Fatalf("update = reset %v, lines %q", u.Reset, u.Lines)
	}
}
