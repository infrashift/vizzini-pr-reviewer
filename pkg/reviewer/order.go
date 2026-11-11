package reviewer

import (
	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/infrashift/vizzini-pr-reviewer/internal/analysis"
)

// OrderFilesByDependency orders a diff's files for a dependency walkthrough:
// each Go file comes after the changed files it depends on (imports, or type
// references within a package), so data models come first, then the logic
// that uses them, then the UI. Ties and cycles are broken by path depth, then
// path; non-Go files follow in diff order. Only the diff is read, so it works
// on any patch, with no checkout. The input slice is not modified.
func OrderFilesByDependency(files []*gitdiff.File) []*gitdiff.File {
	return analysis.OrderFilesByDependency(files)
}
