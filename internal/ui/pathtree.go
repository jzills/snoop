package ui

import "strings"

// The Files and Diff tabs both show their files as a tree of folders. What
// they share is here: grouping paths into folders, and the moves on the
// tree that don't depend on what a row holds. Each tab keeps its own
// totals, order and rows.

// pathGroup is a folder of a path tree, or one of its items.
type pathGroup struct {
	key, name string // a folder's key ends in /
	depth     int
	item      int // the item's index, or -1 for a folder
	children  []*pathGroup
}

// groupPaths arranges paths into folders. A folder holding only one folder
// is merged with it, so deep paths take one row.
func groupPaths(paths []string) []*pathGroup {
	root := &pathGroup{item: -1}
	for i, p := range paths {
		parts := pathParts(p)
		n, key := root, ""
		for _, dir := range parts[:len(parts)-1] {
			key += dir + "/"
			var next *pathGroup
			for _, c := range n.children {
				if c.item < 0 && c.key == key {
					next = c
					break
				}
			}
			if next == nil {
				next = &pathGroup{key: key, name: dir + "/", item: -1}
				n.children = append(n.children, next)
			}
			n = next
		}
		n.children = append(n.children, &pathGroup{key: p, name: parts[len(parts)-1], item: i})
	}
	mergeChains(root, -1)
	return root.children
}

func mergeChains(n *pathGroup, depth int) {
	for n.item < 0 && depth >= 0 && len(n.children) == 1 && n.children[0].item < 0 {
		c := n.children[0]
		n.key, n.name, n.children = c.key, n.name+c.name, c.children
	}
	n.depth = depth
	for _, c := range n.children {
		mergeChains(c, depth+1)
	}
}

// treeNode is a row of a tab's tree.
type treeNode[N any] interface {
	comparable
	nodeKey() string
	nodeDepth() int
	isFolder() bool
	kids() []N
}

// folderNodes is every folder in ns, at any depth.
func folderNodes[N treeNode[N]](ns []N) []N {
	var out []N
	for _, n := range ns {
		if n.isFolder() {
			out = append(out, n)
			out = append(out, folderNodes(n.kids())...)
		}
	}
	return out
}

// findNode is the node with key in ns, at any depth.
func findNode[N treeNode[N]](ns []N, key string) (N, bool) {
	for _, n := range ns {
		if n.nodeKey() == key {
			return n, true
		}
		if m, ok := findNode(n.kids(), key); ok {
			return m, true
		}
	}
	var zero N
	return zero, false
}

// expandFrom is E on a tree: on a folder, it opens the folder and every
// folder in it, or once all are open, closes them; on a file, the same for
// every folder. It returns the key to select after: the selected row's, or
// for a file the closing hid, its top folder's.
func expandFrom[N treeNode[N]](tree []N, sel N, hasSel bool, open func(N) bool, set func(key string, open bool)) string {
	scope := tree
	if hasSel && sel.isFolder() {
		if n, ok := findNode(tree, sel.nodeKey()); ok {
			scope = []N{n}
		}
	}
	all := folderNodes(scope)
	expand := false
	for _, n := range all {
		if !open(n) {
			expand = true
		}
	}
	for _, n := range all {
		set(n.nodeKey(), expand)
	}
	if !hasSel {
		return ""
	}
	key := sel.nodeKey()
	if !expand && !sel.isFolder() {
		for _, n := range tree {
			if n.isFolder() && strings.HasPrefix(key, n.nodeKey()) {
				key = n.nodeKey()
			}
		}
	}
	return key
}

// parentRow is the index of the folder holding rows[i], or i at the top.
func parentRow[N treeNode[N]](rows []N, i int) int {
	if i >= len(rows) {
		return i
	}
	for j := i - 1; j >= 0; j-- {
		if rows[j].nodeDepth() < rows[i].nodeDepth() {
			return j
		}
	}
	return i
}
