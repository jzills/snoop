package ui

import (
	"fmt"
	"sort"
	"strings"
)

// The Files tab shows its files as a flat list or, after T, as a tree of
// folders. Either way the list is rows of fileNodes: in the flat list one
// per file, in the tree the open folders' contents too.

type fileNode struct {
	key      string // a file's path, or a folder's path ending in /
	name     string // what the row shows
	depth    int
	file     *fileEntry // nil for a folder
	sum      fileEntry  // a folder's totals
	children []*fileNode
}

func (n *fileNode) entry() *fileEntry {
	if n.file != nil {
		return n.file
	}
	return &n.sum
}

// prefs are choices that carry from one session view to the next.
type prefs struct {
	filesTree bool
}

// pathParts splits a path into folders and the file name. An absolute
// path's first folder keeps its slash, so files outside the project sit
// under it.
func pathParts(p string) []string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if strings.HasPrefix(p, "/") {
		parts[0] = "/" + parts[0]
	}
	return parts
}

// buildFileTree arranges files into folders. A folder holding only one
// folder is merged with it, so deep paths take one row. Folders come
// before files; within each, changed ones come first, then by name.
func buildFileTree(files []*fileEntry) []*fileNode {
	root := &fileNode{}
	for _, f := range files {
		parts := pathParts(f.path)
		n, key := root, ""
		for _, dir := range parts[:len(parts)-1] {
			key += dir + "/"
			var next *fileNode
			for _, c := range n.children {
				if c.file == nil && c.key == key {
					next = c
					break
				}
			}
			if next == nil {
				next = &fileNode{key: key, name: dir + "/"}
				n.children = append(n.children, next)
			}
			n = next
		}
		n.children = append(n.children, &fileNode{key: f.path, name: parts[len(parts)-1], file: f})
	}
	finishTree(root, -1)
	return root.children
}

// finishTree merges single-folder chains, adds up each folder's totals,
// sets depths and sorts.
func finishTree(n *fileNode, depth int) {
	for n.file == nil && depth >= 0 && len(n.children) == 1 && n.children[0].file == nil {
		c := n.children[0]
		n.key, n.name, n.children = c.key, n.name+c.name, c.children
	}
	n.depth = depth
	for _, c := range n.children {
		finishTree(c, depth+1)
		e := c.entry()
		s := &n.sum
		s.reads, s.edits, s.writes, s.errs = s.reads+e.reads, s.edits+e.edits, s.writes+e.writes, s.errs+e.errs
		s.added, s.removed = s.added+e.added, s.removed+e.removed
		if e.last.After(s.last) {
			s.last = e.last
		}
	}
	sort.SliceStable(n.children, func(i, j int) bool {
		a, b := n.children[i], n.children[j]
		if (a.file == nil) != (b.file == nil) {
			return a.file == nil
		}
		if a.entry().readOnly() != b.entry().readOnly() {
			return !a.entry().readOnly()
		}
		return a.name < b.name
	})
}

// fileOpen reports whether a folder shows its contents. Folders with
// changes start open and folders only read start closed; while searching
// everything is open, so matches are visible.
func (v *view) fileOpen(n *fileNode) bool {
	return v.search.Value() != "" || v.savedFileOpen(n)
}

func (v *view) savedFileOpen(n *fileNode) bool {
	if open, ok := v.fOpen[n.key]; ok {
		return open
	}
	return !n.sum.readOnly()
}

// flattenFiles lays out the rows, keeping the row with selKey selected
// when it is still shown.
func (v *view) flattenFiles(selKey string) {
	v.fRows, v.fRowsTree = v.fRows[:0], v.prefs.filesTree
	if v.prefs.filesTree {
		var walk func([]*fileNode)
		walk = func(ns []*fileNode) {
			for _, n := range ns {
				v.fRows = append(v.fRows, n)
				if n.file == nil && v.fileOpen(n) {
					walk(n.children)
				}
			}
		}
		walk(v.fTree)
	} else {
		for _, f := range v.files {
			v.fRows = append(v.fRows, &fileNode{key: f.path, name: f.path, file: f})
		}
	}
	for i, n := range v.fRows {
		if n.key == selKey {
			v.fCursor = i
			return
		}
	}
	v.fCursor = max(0, min(v.fCursor, len(v.fRows)-1))
}

func (v *view) selectedFileRow() *fileNode {
	if v.fCursor < len(v.fRows) {
		return v.fRows[v.fCursor]
	}
	return nil
}

func (v *view) setFileOpen(n *fileNode, open bool) {
	v.fOpen[n.key] = open
	v.flattenFiles(n.key)
}

// toggleFilesTree switches between the flat list and the tree, keeping the
// same file selected.
func (v *view) toggleFilesTree() {
	sel := ""
	if n := v.selectedFileRow(); n != nil {
		sel = n.key
	}
	v.prefs.filesTree = !v.prefs.filesTree
	v.flattenFiles(sel)
}

// expandAllFiles opens every folder, or once all are open, goes back to
// the default.
func (v *view) expandAllFiles() {
	var all []*fileNode
	var walk func([]*fileNode)
	walk = func(ns []*fileNode) {
		for _, n := range ns {
			if n.file == nil {
				all = append(all, n)
				walk(n.children)
			}
		}
	}
	walk(v.fTree)
	expand := false
	for _, n := range all {
		if !v.savedFileOpen(n) {
			expand = true
		}
	}
	sel := ""
	if n := v.selectedFileRow(); n != nil {
		sel = n.key
	}
	if expand {
		for _, n := range all {
			v.fOpen[n.key] = true
		}
	} else {
		clear(v.fOpen)
	}
	v.flattenFiles(sel)
}

// toFileParent selects the folder holding the selected row.
func (v *view) toFileParent() {
	n := v.selectedFileRow()
	if n == nil {
		return
	}
	for i := v.fCursor - 1; i >= 0; i-- {
		if v.fRows[i].depth < n.depth {
			v.fCursor = i
			return
		}
	}
}

// fileTreeRow draws a row of the tree: a folder with its chevron and
// totals, or a file under it.
func (v *view) fileTreeRow(n *fileNode, sel bool, w int) string {
	if !v.prefs.filesTree {
		return fileRow(n.file, n.name, sel, w)
	}
	lead := strings.Repeat("  ", n.depth)
	if n.file == nil {
		chevron := "▹ "
		if v.fileOpen(n) {
			chevron = "▿ "
		}
		lead += chevron
	} else {
		lead += "  "
	}
	return fileRow(n.entry(), lead+n.name, sel, w)
}

// renderFolderDetail summarises a folder: its totals, then its files.
func renderFolderDetail(n *fileNode, w int) string {
	var b strings.Builder
	b.WriteString(sBold.Render(wrap(n.key, w)) + "\n")
	b.WriteString(fileSummary(&n.sum) + "\n")
	b.WriteString(section("Files") + "\n")
	var walk func([]*fileNode)
	walk = func(ns []*fileNode) {
		for _, c := range ns {
			if c.file == nil {
				walk(c.children)
				continue
			}
			f := c.file
			style := sText
			if f.readOnly() {
				style = sDim
			}
			line := style.Render(strings.TrimPrefix(f.path, n.key))
			if f.added+f.removed > 0 {
				line += "  " + sAdd.Render(plusMinus("+", f.added)) + " " + sDel.Render(plusMinus("−", f.removed))
			}
			if f.errs > 0 {
				line += "  " + sErr.Render(fmt.Sprintf("%d%s", f.errs, glyphErr))
			}
			b.WriteString(fit(line, w) + "\n")
		}
	}
	walk(n.children)
	return b.String()
}

// folderSig changes when anything shown in a folder's detail does.
func folderSig(n *fileNode) string {
	s := n.sum
	return fmt.Sprintf("%d/%d/%d/%d/%d/%d", s.reads, s.edits, s.writes, s.errs, s.added, s.removed)
}
