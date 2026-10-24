package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInfoFirstPromptFromBlocks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	lines := []string{
		`{"type":"attachment","attachment":{"huge":"` + strings.Repeat("x", 1000) + `"}}`,
		`{"type":"user","cwd":"/repo","gitBranch":"main","message":{"content":"<command-name>/clear</command-name>"}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"describe this screenshot"},{"type":"image","source":{}}]}}`,
	}
	os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	info := Info(p)
	if info.FirstPrompt != "describe this screenshot" {
		t.Errorf("first prompt = %q", info.FirstPrompt)
	}
	if info.CWD != "/repo" || info.Branch != "main" {
		t.Errorf("cwd/branch = %q %q", info.CWD, info.Branch)
	}
}
