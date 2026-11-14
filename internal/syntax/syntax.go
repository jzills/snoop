// Package syntax splits source code into classed spans, line by line, so
// the UI can color code in its own palette.
package syntax

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Class is what a span of code is, coarse enough for a terminal palette.
type Class uint8

const (
	Plain Class = iota
	Keyword
	Type
	Constant
	Number
	String
	Comment
	Function
	Tag
	Attr
)

// Span is a run of text, never holding a newline, and its class.
type Span struct {
	Text  string
	Class Class
}

// byName caches lexers by file name, since matching tries every lexer's
// patterns; a nil entry means none matched.
var byName sync.Map // string -> chroma.Lexer

// lexer finds a lexer by the file's name, or nil when none claims it. It
// never guesses from content: a wrong guess is worse than no color.
func lexer(path string) chroma.Lexer {
	if path == "" {
		return nil
	}
	name := filepath.Base(path)
	if l, ok := byName.Load(name); ok {
		l, _ := l.(chroma.Lexer)
		return l
	}
	var l chroma.Lexer
	// Plain text has a lexer, but nothing to color.
	if m := lexers.Match(name); m != nil && m != lexers.Fallback && m.Config().Name != "plaintext" {
		l = chroma.Coalesce(m)
	}
	byName.Store(name, l)
	return l
}

// Lang is the name of the language matching the file's name, or "".
func Lang(path string) string {
	if l := lexer(path); l != nil {
		return l.Config().Name
	}
	return ""
}

// Lines highlights src as the language the file's name suggests. It
// returns one slice of spans per line of src, as strings.Split(src, "\n")
// counts them, or nil when no language matches or the lexer fails. Each
// line's spans hold exactly that line's text.
func Lines(path, src string) (out [][]Span) {
	l := lexer(path)
	if l == nil {
		return nil
	}
	// Chroma panics on some lexer errors; a lexer that fails leaves the
	// code plain.
	defer func() {
		if recover() != nil {
			out = nil
		}
	}()
	n := strings.Count(src, "\n") + 1
	// No EnsureLF: it turns a lone \r into a newline, which would shift
	// every later line.
	it, err := l.Tokenise(&chroma.TokeniseOptions{State: "root"}, src)
	if err != nil {
		return nil
	}
	out = make([][]Span, 1, n)
	for t := it(); t != chroma.EOF; t = it() {
		c := classOf(t.Type)
		for i, part := range strings.Split(t.Value, "\n") {
			if i > 0 {
				out = append(out, nil)
			}
			if part == "" {
				continue
			}
			last := &out[len(out)-1]
			if k := len(*last); k > 0 && (*last)[k-1].Class == c {
				(*last)[k-1].Text += part
			} else {
				*last = append(*last, Span{Text: part, Class: c})
			}
		}
	}
	// Lexers may add a final newline; keep exactly src's lines.
	for len(out) < n {
		out = append(out, nil)
	}
	out = out[:n]
	// The lexer may still change text, such as invalid UTF-8; such a line
	// is drawn as it is, uncolored, so no wrong code is shown.
	for i, line := range strings.Split(src, "\n") {
		if text(out[i]) != line {
			out[i] = []Span{{Text: line}}
			if line == "" {
				out[i] = nil
			}
		}
	}
	return out
}

func text(spans []Span) string {
	if len(spans) == 1 {
		return spans[0].Text
	}
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func classOf(t chroma.TokenType) Class {
	switch {
	case t == chroma.CommentPreproc || t == chroma.CommentPreprocFile:
		return Keyword
	case t.InCategory(chroma.Comment):
		return Comment
	case t == chroma.KeywordConstant:
		return Constant
	case t == chroma.KeywordType:
		return Type
	case t.InCategory(chroma.Keyword):
		return Keyword
	case t.InSubCategory(chroma.LiteralString):
		return String
	case t.InSubCategory(chroma.LiteralNumber):
		return Number
	case t.InCategory(chroma.Literal):
		return Constant
	case t == chroma.NameFunction || t == chroma.NameFunctionMagic || t == chroma.NameClass:
		return Function
	case t == chroma.NameBuiltin || t == chroma.NameBuiltinPseudo:
		return Type
	case t == chroma.NameConstant:
		return Constant
	case t == chroma.NameTag:
		return Tag
	case t == chroma.NameAttribute || t == chroma.NameDecorator:
		return Attr
	}
	return Plain
}
