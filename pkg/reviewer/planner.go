package reviewer

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/infrashift/vizzini-pr-reviewer/internal/llm"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// IssuePayload is the issue a plan is made for (Milestone 4.7).
type IssuePayload struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
}

// PlanRequest is everything Plan reads.
type PlanRequest struct {
	Issue IssuePayload
	// FileTree lists repository-relative paths, e.g. from `git ls-files`.
	// When set, the plan's file references are checked against it.
	FileTree []string
	// OKFIndex is the knowledge base's root index.md. When set, the plan may
	// reference the concepts it lists, and only those.
	OKFIndex string
}

// Plan types, aliased from the schema so they always match the CUE shape.
type (
	ImplementationPlan = schema.ImplementationPlan
	PlanPhase          = schema.PlanPhase
	PlanStep           = schema.PlanStep
	AffectedFile       = schema.AffectedFile
)

// Budgets for the plan prompt's context.
const (
	maxTreeTokens  = 4000
	maxIndexTokens = 1500
)

// ErrEmptyIssue is returned when the issue has no title and no body.
var ErrEmptyIssue = errors.New("reviewer: the issue has no title or body")

var mdLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)

// Plan turns an issue into a schema-validated implementation plan. File
// paths are checked against FileTree (modify/delete must exist, create must
// not, step files must be affected files) and references against the OKF
// index, with violations sent back to the model for repair.
func (e *Engine) Plan(ctx context.Context, req *PlanRequest) (*ImplementationPlan, error) {
	ctx, _, _, finish := begin(ctx, "plan")
	defer finish()
	if strings.TrimSpace(req.Issue.Title) == "" && strings.TrimSpace(req.Issue.Body) == "" {
		return nil, ErrEmptyIssue
	}
	tree, omitted := e.fitLines(req.FileTree, maxTreeTokens)
	index := req.OKFIndex
	if e.chunker.CountTokens(index) > maxIndexTokens {
		index = e.truncateTokens(index, maxIndexTokens) + "\n[…truncated]"
	}
	p, err := llm.Render("plan", llm.PlanData{
		Title: req.Issue.Title, Body: req.Issue.Body, Labels: req.Issue.Labels,
		FileTree: strings.Join(tree, "\n"), TreeOmitted: omitted, OKFIndex: index,
	})
	if err != nil {
		return nil, err
	}

	exists := make(map[string]bool, len(req.FileTree))
	for _, f := range req.FileTree {
		exists[cleanPath(f)] = true
	}
	refs := map[string]bool{}
	for _, m := range mdLink.FindAllStringSubmatch(req.OKFIndex, -1) {
		refs[cleanPath(m[1])] = true
	}

	var out *ImplementationPlan
	_, err = e.llm.CompleteJSON(ctx, p, func(b []byte) error {
		plan, verr := e.validator.ValidatePlan(b)
		if verr != nil {
			return verr
		}
		if aerr := anchorPlan(plan, exists, refs); aerr != nil {
			return aerr
		}
		out = plan
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func anchorPlan(plan *ImplementationPlan, exists, refs map[string]bool) error {
	affected := map[string]bool{}
	for i, f := range plan.AffectedFiles {
		p := cleanPath(f.Path)
		affected[p] = true
		if len(exists) == 0 {
			continue
		}
		switch f.Change {
		case "modify", "delete":
			if !exists[p] {
				done := map[string]string{"modify": "modified", "delete": "deleted"}[f.Change]
				return fmt.Errorf("%w: affected_files.%d: %q does not exist in the repository tree, so it cannot be %s; use create or a path from the tree", schema.ErrInvalid, i, f.Path, done)
			}
		case "create":
			if exists[p] {
				return fmt.Errorf("%w: affected_files.%d: %q already exists; use modify", schema.ErrInvalid, i, f.Path)
			}
		}
	}
	for pi, ph := range plan.Phases {
		for si, st := range ph.Steps {
			for fi, f := range st.Files {
				if !affected[cleanPath(f)] {
					return fmt.Errorf("%w: phases.%d.steps.%d.files.%d: %q is not listed in affected_files", schema.ErrInvalid, pi, si, fi, f)
				}
			}
		}
	}
	for i, r := range plan.References {
		if refs[cleanPath(r)] {
			continue
		}
		if len(refs) == 0 {
			return fmt.Errorf("%w: references.%d: no knowledge-base index was provided; omit references", schema.ErrInvalid, i)
		}
		listed := make([]string, 0, len(refs))
		for k := range refs {
			listed = append(listed, k)
		}
		slices.Sort(listed)
		return fmt.Errorf("%w: references.%d: %q is not listed in the knowledge-base index; use one of: %s", schema.ErrInvalid, i, r, strings.Join(listed, ", "))
	}
	return nil
}

func cleanPath(p string) string {
	return strings.TrimPrefix(path.Clean(strings.TrimPrefix(strings.TrimSpace(p), "/")), "./")
}

// fitLines keeps whole lines, in order, while they fit n tokens, and reports
// how many were left out.
func (e *Engine) fitLines(lines []string, n int) (kept []string, omitted int) {
	used := 0
	for i, l := range lines {
		t := e.chunker.CountTokens(l + "\n")
		if used+t > n {
			return lines[:i], len(lines) - i
		}
		used += t
	}
	return lines, 0
}
