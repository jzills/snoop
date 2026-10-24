// Package store finds Claude Code session transcripts on disk.
package store

import (
	"bytes"
	json "github.com/goccy/go-json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jzills/snoop/internal/parse"
)

// Root returns the directory holding per-project transcript folders.
func Root() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "projects")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

var reUnsafe = regexp.MustCompile(`[^a-zA-Z0-9]`)

// ProjectDir maps a working directory to its transcript folder, the way
// Claude Code names them (every non-alphanumeric rune becomes '-').
func ProjectDir(root, cwd string) string {
	return filepath.Join(root, reUnsafe.ReplaceAllString(cwd, "-"))
}

type SessionInfo struct {
	ID          string
	Path        string
	ProjectDir  string
	CWD         string
	Branch      string
	Title       string
	FirstPrompt string
	ModTime     time.Time
	Size        int64
}

// Name is the best short label for the session.
func (s SessionInfo) Name() string {
	switch {
	case s.Title != "":
		return s.Title
	case s.FirstPrompt != "":
		return s.FirstPrompt
	}
	return s.ID
}

// Project is the working directory if known, else the folder name.
func (s SessionInfo) Project() string {
	if s.CWD != "" {
		return s.CWD
	}
	return filepath.Base(s.ProjectDir)
}

// List returns every top-level session under root, newest first. Only the
// head and tail of each file are read, so this stays fast on large logs.
func List(root string) ([]SessionInfo, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	out := make([]SessionInfo, len(paths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU()*2)
	for i, p := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p string) {
			defer func() { <-sem; wg.Done() }()
			out[i] = Info(p)
		}(i, p)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Newest returns the most recently modified session in a project folder.
func Newest(projectDir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err != nil {
		return "", err
	}
	var best string
	var bestT time.Time
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(bestT) {
			best, bestT = p, st.ModTime()
		}
	}
	if best == "" {
		return "", os.ErrNotExist
	}
	return best, nil
}

const peek = 128 << 10

type headRecord struct {
	Type      string `json:"type"`
	CWD       string `json:"cwd"`
	GitBranch string `json:"gitBranch"`
	IsMeta    bool   `json:"isMeta"`
	AITitle   string `json:"aiTitle"`
	Message   *struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// Info reads summary metadata for one transcript.
func Info(path string) SessionInfo {
	info := SessionInfo{
		ID:         strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		Path:       path,
		ProjectDir: filepath.Dir(path),
	}
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return info
	}
	info.ModTime, info.Size = st.ModTime(), st.Size()

	head := make([]byte, min(peek, st.Size()))
	n, _ := io.ReadFull(f, head)
	scan(head[:n], &info, false)
	if st.Size() > peek {
		tail := make([]byte, peek)
		n, _ := f.ReadAt(tail, st.Size()-peek)
		tail = tail[:n]
		// Drop the partial first line.
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:]
		}
		scan(tail, &info, true)
	}
	return info
}

func scan(buf []byte, info *SessionInfo, isTail bool) {
	for _, line := range bytes.Split(buf, []byte{'\n'}) {
		if len(line) == 0 || parse.Skippable(line) {
			continue
		}
		var r headRecord
		if json.Unmarshal(line, &r) != nil {
			continue
		}
		if r.CWD != "" && (info.CWD == "" || isTail) {
			info.CWD = r.CWD
		}
		if r.GitBranch != "" && (info.Branch == "" || isTail) {
			info.Branch = r.GitBranch
		}
		if r.Type == "ai-title" && r.AITitle != "" {
			info.Title = r.AITitle
		}
		// Prefer the first real prompt over slash commands like /clear.
		if r.Type == "user" && !r.IsMeta && !isTail && r.Message != nil &&
			(info.FirstPrompt == "" || strings.HasPrefix(info.FirstPrompt, "/")) {
			s := strings.Join(strings.Fields(parse.PromptText(r.Message.Content)), " ")
			if s != "" && (info.FirstPrompt == "" || !strings.HasPrefix(s, "/")) {
				info.FirstPrompt = s
			}
		}
	}
}

type SubagentMeta struct {
	ID          string
	Path        string
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	Name        string `json:"name"`
	Finished    bool   `json:"finished"`
}

// SubagentPath returns where a subagent's transcript lives for a session.
func SubagentPath(sessionPath, agentID string) string {
	return filepath.Join(strings.TrimSuffix(sessionPath, ".jsonl"), "subagents", "agent-"+agentID+".jsonl")
}

// SubagentIDs lists the ids of every subagent transcript of a session,
// nested subagents included.
func SubagentIDs(sessionPath string) []string {
	ps, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(sessionPath, ".jsonl"), "subagents", "agent-*.jsonl"))
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "agent-"), ".jsonl")
	}
	return ids
}

// Subagent reads a subagent's metadata, if present.
func Subagent(sessionPath, agentID string) (SubagentMeta, bool) {
	p := SubagentPath(sessionPath, agentID)
	if _, err := os.Stat(p); err != nil {
		return SubagentMeta{}, false
	}
	m := SubagentMeta{ID: agentID, Path: p}
	if b, err := os.ReadFile(strings.TrimSuffix(p, ".jsonl") + ".meta.json"); err == nil {
		json.Unmarshal(b, &m)
	}
	return m, true
}

// Load parses a whole transcript and returns the session plus the byte
// offset parsed up to, for tailing.
func Load(path string) (*parse.Session, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	s := parse.NewSession()
	n, err := s.Read(f)
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	return s, n, err
}
