package review

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

// A branch diff shows everything the current branch changed since it left
// its base, committed or not: the working tree against the merge base.
// Comparing the working tree keeps new-side line numbers those of the
// files on disk, so comments read the same in either diff.

// Base is a branch to compare with, and how many commits the current
// branch has that it doesn't.
type Base struct {
	Name  string
	Ahead int
}

// usualBases are the branches tried when guessing, in the order that
// breaks ties: a repository with develop merges into it.
var usualBases = []string{"develop", "main", "master", "trunk"}

// moreBases is how many other branches Bases lists.
const moreBases = 10

var errDetached = errors.New("HEAD isn't on a branch")

// LoadBranch diffs the working tree in dir against where its branch left
// base, with untracked files, and marks the files also changed since the
// last commit.
func LoadBranch(dir, base string) (*Diff, error) {
	root, err := repoRoot(dir)
	if err != nil {
		return nil, err
	}
	mb, err := run(root, nil, "merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}
	mb = strings.TrimSpace(mb)
	d, err := load(root, mb)
	if err != nil {
		return nil, err
	}
	d.Base, d.Commits = base, ahead(root, mb)
	changed := map[string]bool{}
	if out, err := run(root, nil, "diff", "HEAD", "--name-only", "-z", "--no-ext-diff"); err == nil {
		for _, p := range strings.Split(out, "\x00") {
			changed[p] = true
		}
	}
	for _, f := range d.Files {
		f.Uncommitted = f.Untracked || changed[f.Path]
	}
	return d, nil
}

// GuessBase picks the base for dir's branch: the one chosen for it with
// SaveBase, else snoop.base in git config, else the closest of the usual
// bases. It returns "" when there is none.
func GuessBase(dir string) string {
	root, err := repoRoot(dir)
	if err != nil {
		return ""
	}
	if cur := currentBranch(root); cur != "" {
		if b := config(root, "branch."+cur+".snoopBase"); b != "" && resolves(root, b) {
			return b
		}
	}
	if b := config(root, "snoop.base"); b != "" && resolves(root, b) {
		return b
	}
	if cs := candidates(root); len(cs) > 0 {
		return cs[0].Name
	}
	return ""
}

// Bases lists bases to choose from: the usual ones, closest first, then
// the branches with the newest commits.
func Bases(dir string) []Base {
	root, err := repoRoot(dir)
	if err != nil {
		return nil
	}
	out := candidates(root)
	seen := map[string]bool{}
	for _, b := range out {
		seen[b.Name] = true
	}
	cur := currentBranch(root)
	refs, _ := run(root, nil, "for-each-ref", "--sort=-committerdate", "--format=%(refname:short)", "refs/heads", "refs/remotes/origin")
	n := 0
	for _, r := range strings.Split(refs, "\n") {
		if r == "" || r == cur || r == "origin" || r == "origin/HEAD" || seen[r] || n == moreBases {
			continue
		}
		seen[r] = true
		out = append(out, Base{r, ahead(root, r)})
		n++
	}
	return out
}

// SaveBase remembers base for dir's current branch, in git config.
func SaveBase(dir, base string) error {
	root, err := repoRoot(dir)
	if err != nil {
		return err
	}
	cur := currentBranch(root)
	if cur == "" {
		return errDetached
	}
	_, err = run(root, nil, "config", "branch."+cur+".snoopBase", base)
	return err
}

// candidates are the usual bases that exist, closest first: for each, the
// remote-tracking branch if there is one, since a local copy may be stale.
func candidates(root string) []Base {
	cur := currentBranch(root)
	var out []Base
	for _, name := range usualBases {
		ref := "origin/" + name
		if !resolves(root, ref) {
			ref = name
			if name == cur || !resolves(root, ref) {
				continue
			}
		}
		out = append(out, Base{ref, ahead(root, ref)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ahead < out[j].Ahead })
	return out
}

// ahead counts the commits on HEAD that ref doesn't have.
func ahead(root, ref string) int {
	out, err := run(root, nil, "rev-list", "--count", ref+"..HEAD")
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

func currentBranch(root string) string {
	out, err := run(root, nil, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func config(root, key string) string {
	out, err := run(root, nil, "config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func resolves(root, ref string) bool {
	_, err := run(root, nil, "rev-parse", "--verify", "-q", ref+"^{commit}")
	return err == nil
}
