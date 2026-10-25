// Package summary condenses whole sessions into small, cacheable records
// and totals them, for the picker's view across sessions.
package summary

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	json "github.com/goccy/go-json"

	"github.com/jzills/snoop/internal/parse"
	"github.com/jzills/snoop/internal/stats"
	"github.com/jzills/snoop/internal/store"
)

// Bump when Session changes shape, so stale cache entries are re-read.
const cacheVersion = 2

type Count struct {
	N      int `json:"n"`
	Errors int `json:"e,omitempty"`
}

type Day struct {
	Tokens int     `json:"t"`
	Cost   float64 `json:"c,omitempty"`
}

// Session is what the summary needs from one transcript.
type Session struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod"`

	Prompts   int     `json:"prompts"`
	ToolCalls int     `json:"tools"`
	Errors    int     `json:"errors"`
	Tokens    int     `json:"tokens"`
	Cost      float64 `json:"cost"`
	HasCost   bool    `json:"hasCost"`

	Tools  map[string]Count `json:"toolCounts,omitempty"`
	Skills map[string]Count `json:"skills,omitempty"`
	MCP    map[string]Count `json:"mcp,omitempty"`
	Agents map[string]Count `json:"agents,omitempty"`
	Models map[string]int   `json:"models,omitempty"` // API requests
	Days   map[string]Day   `json:"days,omitempty"`   // by local date, from each turn's start

	Find *Index `json:"find,omitempty"` // what the picker's find searches
}

// Of summarises a parsed session.
func Of(s *parse.Session) Session {
	st := stats.Compute(s)
	out := Session{
		Prompts: st.Prompts, ToolCalls: st.ToolCalls, Errors: st.Errors,
		Tokens: s.Usage.Total(), Cost: s.CostUSD, HasCost: s.CostUSD > 0 || len(s.ModelCost) > 0,
		Tools: counts(st.Tools), Skills: counts(st.Skills), MCP: counts(st.MCP), Agents: counts(st.Agents),
		Models: map[string]int{}, Days: map[string]Day{},
	}
	for m, n := range s.Models {
		out.Models[m] = n
	}
	for _, t := range s.Turns {
		if t.Start.IsZero() {
			continue
		}
		k := t.Start.Local().Format(time.DateOnly)
		d := out.Days[k]
		d.Tokens += t.Usage.Total()
		if usd, ok := s.TurnCost(t); ok {
			d.Cost += usd
		}
		out.Days[k] = d
	}
	return out
}

func counts(cs []stats.Count) map[string]Count {
	if len(cs) == 0 {
		return nil
	}
	m := make(map[string]Count, len(cs))
	for _, c := range cs {
		m[c.Name] = Count{c.N, c.Errors}
	}
	return m
}

// Cache keeps summaries on disk, keyed by transcript path and valid while
// the file's size and modification time are unchanged.
type Cache struct {
	path    string
	mu      sync.Mutex
	entries map[string]Session
	dirty   bool
}

type cacheFile struct {
	Version  int                `json:"version"`
	Sessions map[string]Session `json:"sessions"`
}

// OpenCache reads dir/summaries.json. An empty dir keeps the cache in
// memory only; a missing or outdated file starts empty.
func OpenCache(dir string) *Cache {
	c := &Cache{entries: map[string]Session{}}
	if dir == "" {
		return c
	}
	c.path = filepath.Join(dir, "summaries.json")
	var f cacheFile
	if b, err := os.ReadFile(c.path); err == nil && json.Unmarshal(b, &f) == nil && f.Version == cacheVersion {
		c.entries = f.Sessions
	}
	return c
}

// DefaultDir is where the cache lives: the user cache directory's snoop
// folder, or "" when there is none.
func DefaultDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "snoop")
}

func (c *Cache) get(path string, size int64, mod time.Time) (Session, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.entries[path]
	return s, ok && s.Size == size && s.ModTime.Equal(mod)
}

func (c *Cache) put(path string, s Session) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[path] = s
	c.dirty = true
}

// Save writes the cache if anything changed, dropping entries for
// transcripts not in keep (deleted sessions).
func (c *Cache) Save(keep []store.SessionInfo) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	live := map[string]bool{}
	for _, i := range keep {
		live[i.Path] = true
	}
	for p := range c.entries {
		if !live[p] {
			delete(c.entries, p)
			c.dirty = true
		}
	}
	if !c.dirty || c.path == "" {
		return nil
	}
	b, err := json.Marshal(cacheFile{cacheVersion, c.entries})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	c.dirty = false
	return os.Rename(tmp, c.path)
}

// Result is one scanned session; Err is set for unreadable transcripts.
type Result struct {
	Path string
	Sum  Session
	Err  error
}

// Scan summarises sessions in the background, from the cache when it is
// current, and closes the channel when done.
func Scan(infos []store.SessionInfo, c *Cache) <-chan Result {
	out := make(chan Result)
	jobs := make(chan store.SessionInfo)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out <- scanOne(i, c)
			}
		}()
	}
	go func() {
		for _, i := range infos {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()
	return out
}

func scanOne(i store.SessionInfo, c *Cache) Result {
	if s, ok := c.get(i.Path, i.Size, i.ModTime); ok {
		return Result{Path: i.Path, Sum: s}
	}
	sess, _, err := store.Load(i.Path)
	if err != nil {
		return Result{Path: i.Path, Err: err}
	}
	s := Of(sess)
	s.Size, s.ModTime = i.Size, i.ModTime
	x := newIndexer()
	x.session(sess, true)
	for _, id := range store.SubagentIDs(i.Path) {
		if sub, _, err := store.Load(store.SubagentPath(i.Path, id)); err == nil {
			x.session(sub, false)
		}
	}
	s.Find = &x.idx
	c.put(i.Path, s)
	return Result{Path: i.Path, Sum: s}
}

// Named is a count with its name, for ranked lists.
type Named struct {
	Name string
	Count
}

// DayTotal is one calendar day of the daily chart.
type DayTotal struct {
	Date time.Time
	Day
}

type Totals struct {
	Sessions  int
	Prompts   int
	ToolCalls int
	Errors    int
	Tokens    int
	Cost      float64
	CostKnown int // sessions with a recorded cost

	Tools, Skills, MCP, Agents, Models []Named
	Days                               []DayTotal // the last `days` days, oldest first
}

// Total adds up summaries, with daily totals for the days up to today.
func Total(sums []Session, now time.Time, days int) Totals {
	var t Totals
	tools, skills, mcp, agents, models := map[string]Count{}, map[string]Count{}, map[string]Count{}, map[string]Count{}, map[string]Count{}
	daily := map[string]Day{}
	add := func(dst, src map[string]Count) {
		for k, c := range src {
			d := dst[k]
			d.N += c.N
			d.Errors += c.Errors
			dst[k] = d
		}
	}
	for _, s := range sums {
		t.Sessions++
		t.Prompts += s.Prompts
		t.ToolCalls += s.ToolCalls
		t.Errors += s.Errors
		t.Tokens += s.Tokens
		if s.HasCost {
			t.Cost += s.Cost
			t.CostKnown++
		}
		add(tools, s.Tools)
		add(skills, s.Skills)
		add(mcp, s.MCP)
		add(agents, s.Agents)
		for m, n := range s.Models {
			c := models[m]
			c.N += n
			models[m] = c
		}
		for k, d := range s.Days {
			x := daily[k]
			x.Tokens += d.Tokens
			x.Cost += d.Cost
			daily[k] = x
		}
	}
	t.Tools, t.Skills, t.MCP, t.Agents, t.Models = ranked(tools), ranked(skills), ranked(mcp), ranked(agents), ranked(models)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for i := days - 1; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		t.Days = append(t.Days, DayTotal{d, daily[d.Format(time.DateOnly)]})
	}
	return t
}

func ranked(m map[string]Count) []Named {
	out := make([]Named, 0, len(m))
	for k, c := range m {
		out = append(out, Named{k, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Name < out[j].Name
	})
	return out
}
