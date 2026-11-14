package syntax

import (
	"strings"
	"testing"
)

func join(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func classAt(spans []Span, text string) (Class, bool) {
	for _, s := range spans {
		if strings.Contains(s.Text, text) {
			return s.Class, true
		}
	}
	return 0, false
}

func TestLinesKeepsTextAndLineCount(t *testing.T) {
	src := "package main\n\n// Hello says hi.\nfunc Hello() string {\n\treturn \"hi\" // done\n}"
	got := Lines("cmd/hello.go", src)
	want := strings.Split(src, "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if join(got[i]) != want[i] {
			t.Errorf("line %d = %q, want %q", i, join(got[i]), want[i])
		}
	}
	checks := []struct {
		line int
		text string
		want Class
	}{
		{0, "package", Keyword},
		{2, "Hello says", Comment},
		{3, "func", Keyword},
		{3, "Hello", Function},
		{3, "string", Type},
		{4, `"hi"`, String},
		{4, "// done", Comment},
	}
	for _, c := range checks {
		cl, ok := classAt(got[c.line], c.text)
		if !ok || cl != c.want {
			t.Errorf("line %d %q: class %d (found %v), want %d", c.line, c.text, cl, ok, c.want)
		}
	}
}

func TestLinesTrailingNewline(t *testing.T) {
	src := "x = 1\n"
	got := Lines("a.py", src)
	if len(got) != 2 || join(got[0]) != "x = 1" || len(got[1]) != 0 {
		t.Fatalf("got %#v", got)
	}
	if cl, _ := classAt(got[0], "1"); cl != Number {
		t.Errorf("1 is class %d, want Number", cl)
	}
}

func TestMultiLineTokensSplit(t *testing.T) {
	src := "/* one\n   two */\nint x;"
	got := Lines("a.c", src)
	if len(got) != 3 {
		t.Fatalf("got %d lines", len(got))
	}
	if cl, _ := classAt(got[1], "two"); cl != Comment {
		t.Errorf("second comment line is class %d", cl)
	}
}

func TestUnknownFile(t *testing.T) {
	if Lines("notes.unknownext", "hello") != nil || Lines("", "x") != nil {
		t.Error("an unknown file should get no spans")
	}
	if Lang("README") != "" || Lang("notes.txt") != "" {
		t.Error("README and plain text should not match a language")
	}
	if Lines("notes.txt", "hello") != nil {
		t.Error("plain text should get no spans")
	}
	if Lang("internal/ui/app.go") == "" || Lang("Makefile") == "" || Lang("site/app.tsx") == "" {
		t.Error("common files should match")
	}
}

func TestLinesKeepSourceText(t *testing.T) {
	for _, src := range []string{
		"a := \"x\ry\"\nb := 2\nc := 3",
		"s := \"\xff\xfe\"\nd := 4",
		"x := 1\r\ny := 2\r\n",
	} {
		got := Lines("a.go", src)
		want := strings.Split(src, "\n")
		if len(got) != len(want) {
			t.Fatalf("%q: got %d lines, want %d", src, len(got), len(want))
		}
		for i := range want {
			if join(got[i]) != want[i] {
				t.Errorf("%q line %d = %q, want %q", src, i, join(got[i]), want[i])
			}
		}
	}
}

func BenchmarkLinesGo(b *testing.B) {
	line := "func add(a, b int) int { return a + b } // sum\n"
	src := strings.Repeat(line, 600)
	b.ResetTimer()
	for b.Loop() {
		Lines("x.go", src)
	}
}
