package reviewer

import (
	"strings"
	"testing"
)

func TestFormatReview(t *testing.T) {
	r := &ReviewOutput{
		Summary:        "Query is built by concatenation.",
		RiskScore:      92,
		BaseRiskScore:  40,
		ModelRiskScore: 92,
		Findings: []Finding{
			{File: "internal/store/users.go", StartLine: 44, EndLine: 45, Severity: SeverityCritical, Category: CategorySecurity,
				Title: "SQL injection", Body: "name is concatenated.", Suggestion: "row := q(ctx, \"... $1\", name)"},
			{File: "README.md", StartLine: 3, EndLine: 3, Severity: SeverityTrivial, Category: CategoryMaintainability,
				Title: "Typo", Body: "Spelling."},
		},
		Excluded: []string{"go.sum"},
		Skipped:  []string{"big.sql"},
	}
	want := "## Review\n\n" +
		"Query is built by concatenation.\n\n" +
		"**Risk:** 92/100 (diff 40, model 92) · **Findings:** 2 (1 Critical, 1 Trivial)\n" +
		"\n### [CRITICAL] SQL injection\n\n" +
		"`internal/store/users.go:44-45` · Security\n\n" +
		"name is concatenated.\n" +
		"\n**Suggestion**\n\n" +
		"```go\nrow := q(ctx, \"... $1\", name)\n```\n" +
		"\n### [TRIVIAL] Typo\n\n" +
		"`README.md:3` · Maintainability\n\n" +
		"Spelling.\n" +
		"\n_Not reviewed (generated or vendored): `go.sum`_\n" +
		"\n_Skipped (hunk larger than the token budget): `big.sql`_\n"
	if got := FormatReview(r); got != want {
		t.Errorf("FormatReview:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatReview_NoFindings(t *testing.T) {
	got := FormatReview(&ReviewOutput{Summary: "Fine.", Findings: []Finding{}})
	if !strings.Contains(got, "**Findings:** none") || strings.Contains(got, "###") {
		t.Errorf("got:\n%s", got)
	}
}

func TestFormatReview_Citations(t *testing.T) {
	got := FormatReview(&ReviewOutput{
		Findings: []Finding{{File: "a.go", StartLine: 1, EndLine: 1, Severity: SeverityMajor, Category: CategoryArchitecture,
			Title: "Builds SQL by hand", Body: "Violates the store ADR.", Citations: []string{"adr-0001-store"}}},
		KnowledgeDocs: []string{"adr-0001-store", "note-store-perf"},
	})
	for _, want := range []string{"Violates the store ADR.\n\nCites: `adr-0001-store`\n", "_Knowledge base consulted: `adr-0001-store`, `note-store-perf`_"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatReview_AutoApprove(t *testing.T) {
	got := FormatReview(&ReviewOutput{Summary: "Docs only.", RiskScore: 4, BaseRiskScore: 4, ModelRiskScore: 2, Findings: []Finding{}, AutoApprove: true})
	if !strings.Contains(got, "**Risk:** 4/100 (diff 4, model 2)") || !strings.Contains(got, "**Auto-approve:** yes") {
		t.Errorf("got:\n%s", got)
	}
	if strings.Contains(FormatReview(&ReviewOutput{Findings: []Finding{}}), "Auto-approve") {
		t.Error("auto-approve line shown when not approved")
	}
}

func TestFormatDescription(t *testing.T) {
	got := FormatDescription(&PRDescription{
		Title: "Add retries", Type: PRTypeFeature, Summary: "Retries transient errors.",
		Changes: []FileChange{{File: "internal/llm/client.go", Description: "Adds backoff."}},
	})
	want := "# Add retries\n\n**Type:** Feature\n\nRetries transient errors.\n\n## Changes\n\n- `internal/llm/client.go`: Adds backoff.\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatSuggestions(t *testing.T) {
	got := FormatSuggestions([]CodeSuggestion{{
		File: "scripts/run.sh", StartLine: 2, EndLine: 4, Category: CategoryLogic,
		Summary: "Quote the variable", ExistingCode: "rm -rf $DIR/", ImprovedCode: "rm -rf \"${DIR:?}/\"",
	}})
	for _, want := range []string{
		"### Quote the variable", "`scripts/run.sh:2-4` · Logic",
		"**Before**\n\n```bash\nrm -rf $DIR/\n```", "**After**\n\n```bash\nrm -rf \"${DIR:?}/\"\n```",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if empty := FormatSuggestions(nil); !strings.Contains(empty, "No improvements suggested.") {
		t.Errorf("empty: %q", empty)
	}
}

func TestCodeBlock_FenceCannotBeEscaped(t *testing.T) {
	code := "doc := `x`\n```\nnot the end\n```"
	got := codeBlock("a.go", code)
	if !strings.HasPrefix(got, "````go\n") || !strings.HasSuffix(got, "\n````\n") {
		t.Errorf("fence not lengthened past the inner ```:\n%s", got)
	}
}

func TestFenceLang(t *testing.T) {
	for file, want := range map[string]string{"a.go": "go", "x/Makefile": "makefile", "B.PY": "python", "noext": "", "c.unknown": ""} {
		if got := fenceLang(file); got != want {
			t.Errorf("fenceLang(%q) = %q, want %q", file, got, want)
		}
	}
}

// The helpers must stay viewport-safe: no tables, no HTML.
func TestMarkdown_ViewportSafe(t *testing.T) {
	out := FormatReview(&ReviewOutput{Summary: "s", Findings: []Finding{{File: "a.go", StartLine: 1, EndLine: 1, Severity: SeverityMajor, Category: CategoryLogic, Title: "t", Body: "b"}}}) +
		FormatDescription(&PRDescription{Title: "t", Type: PRTypeDocs, Summary: "s"}) +
		FormatSuggestions([]CodeSuggestion{{File: "a.go", StartLine: 1, EndLine: 1, Category: CategoryLogic, Summary: "s", ImprovedCode: "x"}})
	for _, bad := range []string{"|---", "<br", "<div", "<table"} {
		if strings.Contains(out, bad) {
			t.Errorf("output contains %q", bad)
		}
	}
}
