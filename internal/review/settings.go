package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// HookEvents are the Claude Code hook events that deliver comments: after
// each tool call, when the agent tries to finish, and with the next prompt.
var HookEvents = []string{"PostToolUse", "Stop", "UserPromptSubmit"}

const hookTimeout = 10 // seconds

// UserSettings is Claude Code's user settings file, next to the projects
// folder that holds the transcripts.
func UserSettings(projectsRoot string) string {
	return filepath.Join(filepath.Dir(projectsRoot), "settings.json")
}

var reHookCmd = regexp.MustCompile(`(^|[/\\"' ])snoop(\.exe)?["']? hook$`)

// IsHookCommand reports whether a hook command runs snoop hook.
func IsHookCommand(cmd string) bool { return reHookCmd.MatchString(cmd) }

// Settings files are edited as ordered objects, so keys snoop doesn't touch
// keep their order and content.
type kv struct {
	k string
	v json.RawMessage
}

type object []kv

func (o *object) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return errors.New("not a JSON object")
	}
	*o = nil
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return err
		}
		*o = append(*o, kv{t.(string), v})
	}
	_, err := dec.Token()
	return err
}

func (o object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, e := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(e.k)
		b.Write(k)
		b.WriteByte(':')
		b.Write(e.v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (o object) get(k string) (json.RawMessage, bool) {
	for _, e := range o {
		if e.k == k {
			return e.v, true
		}
	}
	return nil, false
}

func (o *object) set(k string, v json.RawMessage) {
	for i, e := range *o {
		if e.k == k {
			(*o)[i].v = v
			return
		}
	}
	*o = append(*o, kv{k, v})
}

func (o *object) del(k string) {
	for i, e := range *o {
		if e.k == k {
			*o = append((*o)[:i], (*o)[i+1:]...)
			return
		}
	}
}

// group is one entry in a hook event's list: a matcher and its commands.
type group struct {
	Hooks []struct {
		Command string `json:"command"`
	} `json:"hooks"`
}

// snoopCommand is the command of a group's snoop hook, or "".
func snoopCommand(raw json.RawMessage) string {
	var g group
	if json.Unmarshal(raw, &g) != nil {
		return ""
	}
	for _, h := range g.Hooks {
		if IsHookCommand(h.Command) {
			return h.Command
		}
	}
	return ""
}

func isSnoopGroup(raw json.RawMessage) bool { return snoopCommand(raw) != "" }

// runnable reports whether a hook command's program exists, so hooks left
// pointing at a removed or upgraded-away binary don't count as installed.
func runnable(cmd string) bool {
	prog := strings.TrimSuffix(cmd, " hook")
	prog = strings.Trim(prog, `"'`)
	if strings.HasPrefix(cmd, `"`) {
		if s, err := strconv.Unquote(strings.TrimSuffix(cmd, " hook")); err == nil {
			prog = s
		}
	}
	if filepath.IsAbs(prog) {
		info, err := os.Stat(prog)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(prog)
	return err == nil
}

func readSettings(path string) (object, []byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return object{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return object{}, b, nil
	}
	var o object
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return o, b, nil
}

// events reads the "hooks" object and each event's list of groups.
func events(o object) (object, map[string][]json.RawMessage, error) {
	hooks := object{}
	if raw, ok := o.get("hooks"); ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, nil, fmt.Errorf("hooks: %w", err)
		}
	}
	lists := map[string][]json.RawMessage{}
	for _, e := range hooks {
		var l []json.RawMessage
		if err := json.Unmarshal(e.v, &l); err != nil {
			return nil, nil, fmt.Errorf("hooks.%s: %w", e.k, err)
		}
		lists[e.k] = l
	}
	return hooks, lists, nil
}

// Installed reports whether any of the settings files runs snoop hook on
// every event that delivers comments.
func Installed(settings ...string) bool {
	for _, p := range settings {
		o, _, err := readSettings(p)
		if err != nil {
			continue
		}
		_, lists, err := events(o)
		if err != nil {
			continue
		}
		all := true
		for _, ev := range HookEvents {
			found := false
			for _, g := range lists[ev] {
				if c := snoopCommand(g); c != "" && runnable(c) {
					found = true
				}
			}
			all = all && found
		}
		if all {
			return true
		}
	}
	return false
}

// Install makes each event run command, replacing snoop hooks that run
// another one (an older install's path) and keeping everything else in the
// file.
func Install(settings, command string) (changed bool, err error) {
	return edit(settings, func(ev string, l []json.RawMessage) []json.RawMessage {
		var kept []json.RawMessage
		for _, g := range l {
			switch snoopCommand(g) {
			case command:
				return l
			case "":
				kept = append(kept, g)
			}
		}
		l = kept
		type hook struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		}
		g := struct {
			Matcher string `json:"matcher,omitempty"`
			Hooks   []hook `json:"hooks"`
		}{Hooks: []hook{{"command", command, hookTimeout}}}
		if ev == "PostToolUse" {
			g.Matcher = "*"
		}
		b, _ := json.Marshal(g)
		return append(l, b)
	})
}

// Uninstall removes snoop's groups, and any event or hooks object they
// leave empty.
func Uninstall(settings string) (changed bool, err error) {
	return edit(settings, func(_ string, l []json.RawMessage) []json.RawMessage {
		var out []json.RawMessage
		for _, g := range l {
			if !isSnoopGroup(g) {
				out = append(out, g)
			}
		}
		return out
	})
}

func edit(settings string, change func(ev string, l []json.RawMessage) []json.RawMessage) (bool, error) {
	o, old, err := readSettings(settings)
	if err != nil {
		return false, err
	}
	hooks, lists, err := events(o)
	if err != nil {
		return false, fmt.Errorf("%s: %w", settings, err)
	}
	dirty := false
	for _, ev := range HookEvents {
		l := change(ev, lists[ev])
		if rawEqual(l, lists[ev]) {
			continue
		}
		dirty = true
		if len(l) == 0 {
			hooks.del(ev)
			continue
		}
		b, _ := json.Marshal(l)
		hooks.set(ev, b)
	}
	if !dirty {
		return false, nil
	}
	if len(hooks) == 0 {
		o.del("hooks")
	} else {
		b, _ := json.Marshal(hooks)
		o.set("hooks", b)
	}
	out, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	if bytes.Equal(out, old) {
		return false, nil
	}
	return true, writeAtomic(settings, out)
}

func rawEqual(a, b []json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// writeAtomic replaces a file through a temporary file in the same
// directory, keeping its permissions.
func writeAtomic(path string, b []byte) error {
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(f.Name(), mode); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
