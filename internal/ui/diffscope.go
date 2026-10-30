package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/review"
)

// The Diff tab shows one of two scopes: the changes since the last commit,
// or everything the branch changed since it left its base. b switches, B
// picks the base.

const (
	scopeAuto        = iota // branch when it has commits, else uncommitted
	scopeUncommitted        // git diff HEAD
	scopeBranch             // the working tree against the merge base
)

type basesMsg struct {
	v     *view
	bases []review.Base
}

type baseSavedMsg struct {
	v    *view
	base string
	err  error
}

// branchShown reports whether the diff on screen is a branch diff.
func (v *view) branchShown() bool { return v.rev.diff != nil && v.rev.diff.Base != "" }

// loadScoped loads the diff for the scope chosen: a branch diff against
// base, guessed when empty, falling back to the uncommitted diff when
// there is no base, or, until a scope is chosen, no commits on the branch.
func loadScoped(cwd, base string, scope int) (*review.Diff, error) {
	if scope != scopeUncommitted {
		if base == "" {
			base = review.GuessBase(cwd)
		}
		if base != "" {
			d, err := review.LoadBranch(cwd, base)
			if err != nil || scope == scopeBranch || d.Commits > 0 {
				return d, err
			}
		}
	}
	return review.Load(cwd)
}

// switchScope flips between the two scopes, for every session view.
func (v *view) switchScope() tea.Cmd {
	if v.branchShown() {
		v.prefs.diffScope = scopeUncommitted
	} else {
		v.prefs.diffScope = scopeBranch
	}
	return v.loadDiff()
}

// startPick lists the bases to choose from, off the UI.
func (v *view) startPick() tea.Cmd {
	r := &v.rev
	r.picking, r.bases, r.bCursor = true, nil, 0
	cwd := v.sess.CWD
	return func() tea.Msg { return basesMsg{v: v, bases: review.Bases(cwd)} }
}

func (v *view) onBases(m basesMsg) {
	r := &v.rev
	r.bases = m.bases
	for i, b := range r.bases {
		if b.Name == r.base {
			r.bCursor = i
		}
	}
}

// pickKey takes every key while the base list is open.
func (v *view) pickKey(k tea.KeyMsg) tea.Cmd {
	r := &v.rev
	switch k.String() {
	case "up", "k":
		r.bCursor = max(r.bCursor-1, 0)
	case "down", "j":
		r.bCursor = min(r.bCursor+1, max(len(r.bases)-1, 0))
	case "esc", "B", "q":
		r.picking = false
	case "enter":
		if r.bCursor >= len(r.bases) {
			return nil
		}
		r.picking = false
		base, cwd := r.bases[r.bCursor].Name, v.sess.CWD
		r.base, v.prefs.diffScope = base, scopeBranch
		return tea.Batch(v.loadDiff(), func() tea.Msg {
			return baseSavedMsg{v: v, base: base, err: review.SaveBase(cwd, base)}
		})
	}
	return nil
}

// scopeLine shows both scopes, the active one bright, with what the
// branch diff compares.
func (v *view) scopeLine(w int) string {
	d := v.rev.diff
	seg := func(name string, on bool) string {
		if on {
			return sAccent.Bold(true).Render(name)
		}
		return sDim.Render(name)
	}
	branch := v.branchShown()
	left := " " + seg("Uncommitted", !branch) + "   " + seg("Branch", branch)
	switch {
	case branch:
		left += sDim.Render(fmt.Sprintf(" · vs %s · %d %s", d.Base, d.Commits, plural(d.Commits, "commit")))
	case v.prefs.diffScope == scopeBranch:
		left += sDim.Render(" · no base found; press B to choose one")
	}
	right := keyHelp("b", "switch", "B", "base") + " "
	gap := max(w-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return fit(left+strings.Repeat(" ", gap)+right, w)
}

// pickView lists the bases, with how many commits the branch has since
// each.
func (v *view) pickView(w, h int) string {
	r := &v.rev
	lines := []string{" " + sSection.MarginTop(0).Render("Compare the branch with"), ""}
	if r.bases == nil {
		lines = append(lines, sDim.Render(" listing branches …"))
	} else if len(r.bases) == 0 {
		lines = append(lines, sDim.Render(" no other branches"))
	}
	nameW := 10
	for _, b := range r.bases {
		nameW = max(nameW, lipgloss.Width(b.Name))
	}
	nameW = min(nameW, max(w-24, 10))
	off := scrollOffset(0, r.bCursor, len(r.bases), max(h-2, 1))
	for i := off; i < len(r.bases) && len(lines) < h; i++ {
		b := r.bases[i]
		sel := i == r.bCursor
		mark, name := "  ", sText.Render(fit(b.Name, nameW))
		if sel {
			mark, name = sAccent.Render("▌ "), sBold.Render(fit(b.Name, nameW))
		}
		note := fmt.Sprintf("%d %s since", b.Ahead, plural(b.Ahead, "commit"))
		if b.Name == r.base {
			note += " · current"
		}
		lines = append(lines, mark+name+"  "+sDim.Render(note))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
