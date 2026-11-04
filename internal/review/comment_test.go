package review

import "testing"

func TestFormat(t *testing.T) {
	cs := []Comment{
		{Path: "internal/store/store.go", Text: "Don't swallow the error; return it.", Lines: []Line{
			{Kind: '+', New: 40, Text: "\tif err != nil {"},
			{Kind: '+', New: 41, Text: "\t\treturn nil"},
			{Kind: '+', New: 42, Text: "\t}"},
		}},
		{Path: "internal/ui/board.go", Text: "This should live in its own package."},
		{Path: "x.go", Text: "Keep this.", Lines: []Line{{Kind: '-', Old: 7, Text: "keep()"}}},
		{Path: "y.go", Text: "Why?", Lines: []Line{{Kind: '-', Old: 3, Text: "a"}, {Kind: '+', New: 9, Text: "b"}, {Kind: ' ', Old: 4, New: 10, Text: "c"}}},
	}
	want := "Review comments from snoop:\n" +
		"\ninternal/store/store.go:40-42\n" +
		"    40 +\tif err != nil {\n" +
		"    41 +\t\treturn nil\n" +
		"    42 +\t}\n" +
		"Don't swallow the error; return it.\n" +
		"\ninternal/ui/board.go (whole file)\n" +
		"This should live in its own package.\n" +
		"\nx.go:7 (removed)\n" +
		"    7 -keep()\n" +
		"Keep this.\n" +
		"\ny.go:9-10\n" +
		"     3 -a\n" +
		"     9 +b\n" +
		"    10  c\n" +
		"Why?\n"
	if got := Format(cs); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCovers(t *testing.T) {
	l := Line{Kind: '+', New: 3, Text: "x"}
	c := Comment{Path: "a.go", Lines: []Line{l}}
	if !c.Covers("a.go", l) || c.Covers("b.go", l) || c.Covers("a.go", Line{Kind: '+', New: 4, Text: "x"}) {
		t.Error("Covers")
	}
}

func TestFormatChangeComment(t *testing.T) {
	got := Format([]Comment{{Path: "a.go", At: "the Edit at 14:03:12", Text: "keep the old name",
		Lines: []Line{{Kind: '-', Text: "func old() {"}, {Kind: '+', Text: "func renamed() {"}}}})
	want := "Review comments from snoop:\n\na.go (the Edit at 14:03:12)\n    -func old() {\n    +func renamed() {\nkeep the old name\n"
	if got != want {
		t.Errorf("Format = %q", got)
	}
}
