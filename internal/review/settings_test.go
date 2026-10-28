package review

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSnoop makes an executable to stand in for snoop, so the hooks count
// as installed, and returns the hook command that runs it.
func fakeSnoop(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "snoop")
	os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755)
	return fmt.Sprintf("%q hook", p)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInstallCreatesFile(t *testing.T) {
	cmd := fakeSnoop(t)
	p := filepath.Join(t.TempDir(), "sub", "settings.json")
	if changed, err := Install(p, cmd); !changed || err != nil {
		t.Fatalf("Install = %v, %v", changed, err)
	}
	hooks := readJSON(t, p)["hooks"].(map[string]any)
	if len(hooks) != 3 {
		t.Fatalf("hooks = %v", hooks)
	}
	post := hooks["PostToolUse"].([]any)[0].(map[string]any)
	h := post["hooks"].([]any)[0].(map[string]any)
	if post["matcher"] != "*" || h["type"] != "command" || h["command"] != cmd || h["timeout"] != 10.0 {
		t.Errorf("PostToolUse = %v", post)
	}
	if _, ok := hooks["Stop"].([]any)[0].(map[string]any)["matcher"]; ok {
		t.Error("Stop has a matcher")
	}
	if !Installed(p) {
		t.Error("not Installed")
	}
}

const existing = `{
  "model": "x",
  "hooks": {
    "PostToolUse": [
      {"matcher": "Write|Edit", "hooks": [{"type": "command", "command": "gofmt"}]}
    ]
  },
  "env": {"A": "1"}
}
`

func TestInstallKeepsOtherSettings(t *testing.T) {
	cmd := fakeSnoop(t)
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(existing), 0o600)
	if _, err := Install(p, cmd); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	if i, j, k := strings.Index(s, `"model"`), strings.Index(s, `"hooks"`), strings.Index(s, `"env"`); !(i < j && j < k) {
		t.Errorf("key order changed:\n%s", s)
	}
	m := readJSON(t, p)
	if m["model"] != "x" || m["env"].(map[string]any)["A"] != "1" {
		t.Errorf("settings lost:\n%s", s)
	}
	post := m["hooks"].(map[string]any)["PostToolUse"].([]any)
	if len(post) != 2 || post[0].(map[string]any)["matcher"] != "Write|Edit" {
		t.Errorf("PostToolUse = %v", post)
	}
	if info, _ := os.Stat(p); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode())
	}
}

func TestInstallIdempotent(t *testing.T) {
	cmd := fakeSnoop(t)
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(existing), 0o644)
	Install(p, cmd)
	before, _ := os.ReadFile(p)
	if changed, err := Install(p, cmd); changed || err != nil {
		t.Errorf("second Install = %v, %v", changed, err)
	}
	if after, _ := os.ReadFile(p); !bytes.Equal(before, after) {
		t.Error("file changed")
	}
}

// After an upgrade moves the binary, the old hooks run nothing: they don't
// count as installed, and installing again replaces them.
func TestInstallReplacesStalePath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(existing), 0o644)
	Install(p, `"/gone/Cellar/snoop/1.0/bin/snoop" hook`)
	if Installed(p) {
		t.Error("hooks for a missing binary count as installed")
	}
	cmd := fakeSnoop(t)
	if changed, err := Install(p, cmd); !changed || err != nil {
		t.Fatalf("Install = %v, %v", changed, err)
	}
	if !Installed(p) {
		t.Error("not Installed after reinstalling")
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "/gone/") {
		t.Errorf("stale hook kept:\n%s", b)
	}
	post := readJSON(t, p)["hooks"].(map[string]any)["PostToolUse"].([]any)
	if len(post) != 2 {
		t.Errorf("PostToolUse = %v", post)
	}
}

func TestUninstall(t *testing.T) {
	cmd := fakeSnoop(t)
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(existing), 0o644)
	Install(p, cmd)
	if changed, err := Uninstall(p); !changed || err != nil {
		t.Fatalf("Uninstall = %v, %v", changed, err)
	}
	m := readJSON(t, p)
	hooks := m["hooks"].(map[string]any)
	if len(hooks) != 1 || len(hooks["PostToolUse"].([]any)) != 1 {
		t.Errorf("hooks = %v", hooks)
	}
	if Installed(p) {
		t.Error("still Installed")
	}

	// Only snoop's hooks: the hooks object goes too.
	q := filepath.Join(t.TempDir(), "settings.json")
	Install(q, cmd)
	Uninstall(q)
	if _, ok := readJSON(t, q)["hooks"]; ok {
		t.Error("empty hooks object left")
	}
}

func TestInstalledInvalidJSON(t *testing.T) {
	cmd := fakeSnoop(t)
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(`{"hooks": `), 0o644)
	if Installed(p) {
		t.Error("Installed on bad JSON")
	}
	if _, err := Install(p, cmd); err == nil {
		t.Error("Install accepted bad JSON")
	}
	if b, _ := os.ReadFile(p); string(b) != `{"hooks": ` {
		t.Error("bad file overwritten")
	}
	if Installed(filepath.Join(t.TempDir(), "missing.json")) {
		t.Error("Installed with no file")
	}
}

func TestIsHookCommand(t *testing.T) {
	for _, c := range []string{`snoop hook`, `"/a b/snoop" hook`, `/usr/bin/snoop hook`, `C:\x\snoop.exe hook`, `"C:\x\snoop.exe" hook`} {
		if !IsHookCommand(c) {
			t.Errorf("%q not recognised", c)
		}
	}
	for _, c := range []string{`snooper hook`, `snoop hooks install`, `gofmt`, `notsnoop hook`} {
		if IsHookCommand(c) {
			t.Errorf("%q recognised", c)
		}
	}
}
