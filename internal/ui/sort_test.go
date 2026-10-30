package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/jzills/snoop/internal/store"
)

func sortPicker() picker {
	now := time.Now()
	p := newPicker("", "")
	p.all = []store.SessionInfo{
		{Path: "/p/-src-web/b.jsonl", Title: "bravo", Branch: "main", ModTime: now.Add(-time.Hour), Size: 300},
		{Path: "/p/-src-api/a.jsonl", Title: "alpha", Branch: "dev", ModTime: now, Size: 100},
		{Path: "/p/-src-cli/c.jsonl", Title: "charlie", Branch: "fix", ModTime: now.Add(-2 * time.Hour), Size: 200},
	}
	p.loaded = true
	p.refilter()
	return p
}

func pickerTitles(p *picker) string {
	var ts []string
	for _, i := range p.shown {
		ts = append(ts, p.all[i].Title)
	}
	return strings.Join(ts, " ")
}

func TestPickerSortsByColumn(t *testing.T) {
	p := sortPicker()
	if got := pickerTitles(&p); got != "alpha bravo charlie" {
		t.Fatalf("by age = %q", got)
	}
	if out := ansi.Strip(p.view(120, 10)); !strings.Contains(out, "AGE ▾") {
		t.Errorf("header doesn't mark the sorted column:\n%s", out)
	}
	p.cursor = 1 // bravo
	for range 4 {
		p.update(key("o"), 10)
	}
	// Size, largest first; the selection stays on bravo.
	if got := pickerTitles(&p); got != "bravo charlie alpha" || p.all[p.shown[p.cursor]].Title != "bravo" {
		t.Errorf("by size = %q, cursor on %q", got, p.all[p.shown[p.cursor]].Title)
	}
	if out := ansi.Strip(p.view(120, 10)); !strings.Contains(out, "SIZE ▾") || strings.Contains(out, "AGE ▾") {
		t.Errorf("header after o:\n%s", out)
	}
	p.update(key("O"), 10)
	if got := pickerTitles(&p); got != "alpha charlie bravo" {
		t.Errorf("by size reversed = %q", got)
	}
	if out := ansi.Strip(p.view(120, 10)); !strings.Contains(out, "SIZE ▴") {
		t.Errorf("header after O:\n%s", out)
	}
	p.update(key("o"), 10) // back to age, in its natural order
	if got := pickerTitles(&p); got != "alpha bravo charlie" || p.sort.rev {
		t.Errorf("o should wrap to age, newest first: %q", got)
	}
}

func TestPickerSearchKeepsBestMatchFirst(t *testing.T) {
	p := sortPicker()
	p.input.SetValue("charlie")
	p.refilter()
	if got := pickerTitles(&p); !strings.HasPrefix(got, "charlie") {
		t.Errorf("search results = %q", got)
	}
}

func TestBoardSortsByColumn(t *testing.T) {
	b, _ := newTestBoard(t)
	now := time.Now()
	b.arrange(now, 30*time.Second)
	if b.order[0].status != rowWaiting {
		t.Fatalf("default order should put waiting first")
	}
	// Age: done.jsonl is a minute old, busy.jsonl seconds old.
	for range 5 {
		b.update(key("o"), 20)
	}
	b.arrange(now, 30*time.Second)
	if !strings.HasSuffix(b.order[0].f.Path, "busy.jsonl") {
		t.Errorf("by age, newest first: %s", b.order[0].f.Path)
	}
	b.update(key("O"), 20)
	b.arrange(now, 30*time.Second)
	if !strings.HasSuffix(b.order[0].f.Path, "done.jsonl") {
		t.Errorf("by age reversed: %s", b.order[0].f.Path)
	}
	if out := ansi.Strip(b.view(120, 20, now, 30*time.Second)); !strings.Contains(out, "AGE ▴") {
		t.Errorf("header doesn't mark the sorted column:\n%s", out)
	}
}
