package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/jzills/snoop/internal/review"
)

// The Diff tab's file list is rows: one per file in the flat list, and
// after T, a tree of folders whose open folders show their contents, as
// in the Files tab.

type diffNode struct {
	key            string // a file's path, or a folder's path ending in /
	name           string
	depth          int
	file           *review.File // nil for a folder
	files          int          // a folder's files
	added, removed int
	ours           bool // a folder holds a file the session changed
	children       []*diffNode
}

// buildDiffTree arranges files into folders, merging folders that hold
// only one folder. Folders come first; within each, the session's own
// files first, then by name.
func buildDiffTree(files []*review.File, ours map[string]bool) []*diffNode {
	root := &diffNode{}
	for _, f := range files {
		parts := pathParts(f.Path)
		n, key := root, ""
		for _, dir := range parts[:len(parts)-1] {
			key += dir + "/"
			var next *diffNode
			for _, c := range n.children {
				if c.file == nil && c.key == key {
					next = c
					break
				}
			}
			if next == nil {
				next = &diffNode{key: key, name: dir + "/"}
				n.children = append(n.children, next)
			}
			n = next
		}
		n.children = append(n.children, &diffNode{key: f.Path, name: parts[len(parts)-1], file: f,
			files: 1, added: f.Added, removed: f.Removed, ours: ours[f.Path]})
	}
	finishDiffTree(root, -1)
	return root.children
}

func finishDiffTree(n *diffNode, depth int) {
	for n.file == nil && depth >= 0 && len(n.children) == 1 && n.children[0].file == nil {
		c := n.children[0]
		n.key, n.name, n.children = c.key, n.name+c.name, c.children
	}
	n.depth = depth
	for _, c := range n.children {
		finishDiffTree(c, depth+1)
		if n.file == nil {
			n.files += c.files
			n.added += c.added
			n.removed += c.removed
			n.ours = n.ours || c.ours
		}
	}
	sort.SliceStable(n.children, func(i, j int) bool {
		a, b := n.children[i], n.children[j]
		if (a.file == nil) != (b.file == nil) {
			return a.file == nil
		}
		if a.ours != b.ours {
			return a.ours
		}
		return a.name < b.name
	})
}

// diffOpen reports whether a folder shows its contents; every folder
// starts open.
func (v *view) diffOpen(n *diffNode) bool {
	open, ok := v.rev.open[n.key]
	return !ok || open
}

// layoutDiff lays out the rows, keeping the row with selKey selected when
// it is still shown.
func (v *view) layoutDiff(selKey string) {
	r := &v.rev
	r.rows = r.rows[:0]
	files := v.diffFiles()
	if v.prefs.diffTree {
		var walk func([]*diffNode)
		walk = func(ns []*diffNode) {
			for _, n := range ns {
				r.rows = append(r.rows, n)
				if n.file == nil && v.diffOpen(n) {
					walk(n.children)
				}
			}
		}
		walk(buildDiffTree(files, r.ours))
	} else {
		for _, f := range files {
			r.rows = append(r.rows, &diffNode{key: f.Path, name: f.Path, file: f, files: 1, added: f.Added, removed: f.Removed, ours: r.ours[f.Path]})
		}
	}
	for i, n := range r.rows {
		if n.key == selKey {
			r.cursor = i
			return
		}
	}
	r.cursor = max(0, min(r.cursor, len(r.rows)-1))
}

// selectedDiffRow is the selected row of the file list, or nil.
func (v *view) selectedDiffRow() *diffNode {
	if v.rev.cursor < len(v.rev.rows) {
		return v.rev.rows[v.rev.cursor]
	}
	return nil
}

// selectedDiffKey is the selected row's key, or "".
func (v *view) selectedDiffKey() string {
	if n := v.selectedDiffRow(); n != nil {
		return n.key
	}
	return ""
}

// toggleDiffTree switches between the flat list and the tree, keeping the
// same file selected.
func (v *view) toggleDiffTree() {
	sel := v.selectedDiffKey()
	v.prefs.diffTree = !v.prefs.diffTree
	v.layoutDiff(sel)
}

func (v *view) setDiffOpen(n *diffNode, open bool) {
	v.rev.open[n.key] = open
	v.layoutDiff(n.key)
}

// toDiffParent selects the folder holding the selected row.
func (v *view) toDiffParent() {
	n := v.selectedDiffRow()
	if n == nil {
		return
	}
	for i := v.rev.cursor - 1; i >= 0; i-- {
		if v.rev.rows[i].depth < n.depth {
			v.rev.cursor = i
			return
		}
	}
}

// diffTreeRow draws a row of the list: a file, or in the tree a folder
// with its chevron and totals.
func (v *view) diffTreeRow(n *diffNode, sel bool, w int) string {
	if n.file != nil && !v.prefs.diffTree {
		return v.diffFileRow(n.file, "", n.file.Path, sel, w)
	}
	lead := strings.Repeat("  ", max(n.depth, 0))
	if n.file != nil {
		return v.diffFileRow(n.file, lead+"  ", n.name, sel, w)
	}
	chevron := "▹ "
	if v.diffOpen(n) {
		chevron = "▿ "
	}
	var bg lipgloss.TerminalColor = lipgloss.NoColor{}
	if sel {
		bg = cSelBg
	}
	st := func(s lipgloss.Style) lipgloss.Style { return s.Background(bg) }
	mark := st(sText).Render(" ")
	if sel {
		mark = st(sAccent).Render("▌")
	}
	nComments := 0
	for _, c := range v.rev.comments {
		if strings.HasPrefix(c.Path, n.key) {
			nComments++
		}
	}
	note := st(sText).Render("   ")
	if nComments > 0 {
		note = st(sKey).Render(fmt.Sprintf(" ●%d", min(nComments, 9)))
	}
	right := st(sAdd).Render(fmt.Sprintf(" %6s", plusMinus("+", n.added))) +
		st(sDel).Render(fmt.Sprintf(" %6s", plusMinus("−", n.removed))) + note + st(sText).Render(" ")
	nameStyle := sText
	if !n.ours {
		nameStyle = sDim
	}
	name := n.name
	if n.depth == 0 {
		name = tildePath(name)
	}
	nameW := max(w-1-lipgloss.Width(right)-lipgloss.Width(lead+chevron), 1)
	line := mark + st(sFaint).Render(lead+chevron) + st(nameStyle).Render(fit(fitLeft(name, nameW), nameW)) + right
	if pad := w - lipgloss.Width(line); pad > 0 {
		line += st(sText).Render(strings.Repeat(" ", pad))
	}
	return fit(line, w)
}

// renderDiffFolder summarises a folder: its totals, then its files.
func (v *view) renderDiffFolder(n *diffNode, w int) string {
	var b strings.Builder
	b.WriteString(sBold.Render(wrap(tildePath(n.key), w)) + "\n")
	b.WriteString(sDim.Render(fmt.Sprintf("%d %s · ", n.files, plural(n.files, "file"))) + sAdd.Render(plusMinus("+", n.added)) + " " + sDel.Render(plusMinus("−", n.removed)) + "\n\n")
	var walk func([]*diffNode)
	walk = func(ns []*diffNode) {
		for _, c := range ns {
			if c.file != nil {
				st := sText
				if !c.ours {
					st = sDim
				}
				rel := strings.TrimPrefix(c.key, n.key)
				b.WriteString(st.Render(fit(rel, max(w-16, 8))) + sAdd.Render(fmt.Sprintf(" %6s", plusMinus("+", c.added))) +
					sDel.Render(fmt.Sprintf(" %6s", plusMinus("−", c.removed))) + "\n")
			}
			walk(c.children)
		}
	}
	walk(n.children)
	b.WriteString("\n" + sKey.Render("space") + sDim.Render(" open or close · ") + sKey.Render("T") + sDim.Render(" flat list"))
	return b.String()
}

// expandAllDiff opens the selected folder and every folder in it, or on a
// file, every folder; once they are all open, it closes them.
func (v *view) expandAllDiff() {
	r := &v.rev
	tree := buildDiffTree(v.diffFiles(), r.ours)
	scope := tree
	sel := v.selectedDiffRow()
	if sel != nil && sel.file == nil {
		scope = nil
		var find func([]*diffNode)
		find = func(ns []*diffNode) {
			for _, n := range ns {
				if n.key == sel.key {
					scope = []*diffNode{n}
				}
				find(n.children)
			}
		}
		find(tree)
	}
	var all []*diffNode
	var walk func([]*diffNode)
	walk = func(ns []*diffNode) {
		for _, n := range ns {
			if n.file == nil {
				all = append(all, n)
				walk(n.children)
			}
		}
	}
	walk(scope)
	expand := false
	for _, n := range all {
		if !v.diffOpen(n) {
			expand = true
		}
	}
	for _, n := range all {
		r.open[n.key] = expand
	}
	key := v.selectedDiffKey()
	// A file hidden by closing them moves the cursor to its folder.
	if !expand && sel != nil && sel.file != nil {
		for _, n := range tree {
			if n.file == nil && strings.HasPrefix(key, n.key) {
				key = n.key
			}
		}
	}
	v.layoutDiff(key)
}
