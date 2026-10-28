package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)

func TestSendClaim(t *testing.T) {
	dir := t.TempDir()
	p1, err := Send(dir, "s1", "first\n", t0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Send(dir, "s1", "second", t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !Waiting(p1) {
		t.Error("not waiting after send")
	}
	text, done, err := Claim(dir, "s1", t0.Add(2*time.Second))
	if err != nil || text != "first\n\nsecond\n" {
		t.Fatalf("Claim = %q, %v", text, err)
	}
	if Waiting(p1) {
		t.Error("still waiting after claim")
	}
	done()
	if text, _, _ := Claim(dir, "s1", t0.Add(time.Hour)); text != "" {
		t.Errorf("second Claim = %q", text)
	}
	if left, _ := os.ReadDir(filepath.Join(dir, "s1")); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
	if text, _, err := Claim(dir, "other", t0); text != "" || err != nil {
		t.Errorf("empty inbox: %q %v", text, err)
	}
}

func TestClaimOrphan(t *testing.T) {
	dir := t.TempDir()
	Send(dir, "s", "lost", t0)
	// Claimed but never delivered: done isn't called.
	if text, _, _ := Claim(dir, "s", t0); text == "" {
		t.Fatal("nothing claimed")
	}
	if text, _, _ := Claim(dir, "s", t0.Add(10*time.Second)); text != "" {
		t.Errorf("reclaimed too soon: %q", text)
	}
	text, done, _ := Claim(dir, "s", t0.Add(2*time.Minute))
	if text != "lost\n" {
		t.Errorf("orphan = %q", text)
	}
	done()
	if text, _, _ := Claim(dir, "s", t0.Add(time.Hour)); text != "" {
		t.Errorf("delivered twice: %q", text)
	}
}

func TestClaimOnce(t *testing.T) {
	dir := t.TempDir()
	for i := range 20 {
		Send(dir, "s", fmt.Sprintf("m%02d", i), t0.Add(time.Duration(i)))
	}
	var mu sync.Mutex
	var got []string
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text, done, _ := Claim(dir, "s", t0)
			done()
			if text != "" {
				mu.Lock()
				got = append(got, strings.Split(strings.TrimSpace(text), "\n\n")...)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	seen := map[string]int{}
	for _, m := range got {
		seen[m]++
	}
	if len(seen) != 20 || len(got) != 20 {
		t.Errorf("got %d texts, %d distinct", len(got), len(seen))
	}
}

func TestBadSession(t *testing.T) {
	for _, s := range []string{"", "..", "a/b", `a\b`} {
		if _, err := Send(t.TempDir(), s, "x", t0); err == nil {
			t.Errorf("Send(%q) accepted", s)
		}
	}
}
