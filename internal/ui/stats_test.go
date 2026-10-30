package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Every tab leaves one blank line between the tabs and its content.
func TestStatsTabSpacing(t *testing.T) {
	v := newTestView(t)
	v.update(key("3"), 120, 30)
	lines := strings.Split(ansi.Strip(v.render(120, 30)), "\n")
	for i, l := range lines {
		if strings.Contains(l, "Timeline") && strings.Contains(l, "Stats") {
			if strings.TrimSpace(lines[i+1]) != "" || strings.TrimSpace(lines[i+2]) != "Overview" {
				t.Errorf("after the tabs:\n%q\n%q", lines[i+1], lines[i+2])
			}
			return
		}
	}
	t.Fatal("no tab row")
}
