package summary

import (
	"strings"

	json "github.com/goccy/go-json"

	"github.com/jzills/snoop/internal/parse"
)

// Index is what the picker's find (f) searches in a session: the first line
// of each prompt, the files it and its subagents read or changed, and the
// commands they ran.
type Index struct {
	Prompts  []string `json:"p,omitempty"`
	Files    []string `json:"f,omitempty"`
	Commands []string `json:"c,omitempty"`
}

const (
	indexLine     = 200 // runes kept of a prompt or command
	indexCommands = 300 // commands kept per session
)

// indexer builds an Index, keeping each entry once.
type indexer struct {
	idx  Index
	seen map[string]bool
}

func newIndexer() *indexer { return &indexer{seen: map[string]bool{}} }

func (x *indexer) add(list *[]string, kind, s string) {
	if s == "" || x.seen[kind+s] {
		return
	}
	x.seen[kind+s] = true
	*list = append(*list, s)
}

// session adds a transcript's prompts, when prompts is set, and its files
// and commands.
func (x *indexer) session(s *parse.Session, prompts bool) {
	if prompts {
		for _, t := range s.Turns {
			x.add(&x.idx.Prompts, "p", clip(t.Prompt.Text))
		}
	}
	for _, e := range s.Events {
		if e.Tool == nil {
			continue
		}
		x.add(&x.idx.Files, "f", e.Tool.FilePath())
		if e.Tool.Name == "Bash" && len(x.idx.Commands) < indexCommands {
			var in struct {
				Command string `json:"command"`
			}
			json.Unmarshal(e.Tool.Input, &in)
			x.add(&x.idx.Commands, "c", clip(in.Command))
		}
	}
}

// clip is the first line of s, at most indexLine runes.
func clip(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > indexLine {
		s = string(r[:indexLine])
	}
	return s
}

type HitKind int

// In the order a session's shown hit is chosen.
const (
	HitFile HitKind = iota
	HitCommand
	HitPrompt
)

// Hit is a session that matched: the first matching entry, by kind, and
// how many entries matched in all.
type Hit struct {
	Path string
	Kind HitKind
	Text string
	N    int
}

// Find returns the sessions among paths, in that order, with an index entry
// containing every word of query, ignoring case. Sessions not yet indexed
// never match.
func Find(sums map[string]Session, paths []string, query string) []Hit {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}
	var hits []Hit
	for _, p := range paths {
		idx := sums[p].Find
		if idx == nil {
			continue
		}
		h := Hit{Path: p}
		for kind, list := range [][]string{idx.Files, idx.Commands, idx.Prompts} {
			for _, s := range list {
				if !matchAll(strings.ToLower(s), terms) {
					continue
				}
				if h.N == 0 {
					h.Kind, h.Text = HitKind(kind), s
				}
				h.N++
			}
		}
		if h.N > 0 {
			hits = append(hits, h)
		}
	}
	return hits
}

func matchAll(s string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(s, t) {
			return false
		}
	}
	return true
}
