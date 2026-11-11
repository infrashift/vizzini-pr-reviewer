package reviewer

import (
	"fmt"
	"path"
	"strings"
)

// The Markdown helpers render results for a bubbletea viewport (typically
// through glamour) or a plain terminal. They use headings, lists and fenced
// code only: no tables, which overflow narrow panes, no HTML, and no emoji,
// whose cell width varies between terminals and breaks lipgloss layouts.

// FormatReview renders a review as Markdown.
func FormatReview(r *ReviewOutput) string {
	var b strings.Builder
	b.WriteString("## Review\n\n")
	if r.Summary != "" {
		b.WriteString(r.Summary)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "**Risk:** %d/100 · **Findings:** %s\n", r.RiskScore, countBySeverity(r.Findings))

	for i := range r.Findings {
		f := &r.Findings[i]
		fmt.Fprintf(&b, "\n### [%s] %s\n\n", strings.ToUpper(string(f.Severity)), f.Title)
		fmt.Fprintf(&b, "`%s` · %s\n\n", location(f.File, f.StartLine, f.EndLine), f.Category)
		b.WriteString(f.Body)
		b.WriteString("\n")
		if f.Suggestion != "" {
			b.WriteString("\n**Suggestion**\n\n")
			b.WriteString(codeBlock(f.File, f.Suggestion))
		}
	}

	if len(r.Excluded) > 0 {
		fmt.Fprintf(&b, "\n_Not reviewed (generated or vendored): %s_\n", codeList(r.Excluded))
	}
	if len(r.Skipped) > 0 {
		fmt.Fprintf(&b, "\n_Skipped (hunk larger than the token budget): %s_\n", codeList(r.Skipped))
	}
	return b.String()
}

// FormatDescription renders a PR description as Markdown.
func FormatDescription(d *PRDescription) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n**Type:** %s\n\n%s\n", d.Title, d.Type, d.Summary)
	if len(d.Changes) > 0 {
		b.WriteString("\n## Changes\n\n")
		for _, c := range d.Changes {
			fmt.Fprintf(&b, "- `%s`: %s\n", c.File, c.Description)
		}
	}
	return b.String()
}

// FormatSuggestions renders improvement suggestions as Markdown.
func FormatSuggestions(s []CodeSuggestion) string {
	var b strings.Builder
	b.WriteString("## Suggested Improvements\n")
	if len(s) == 0 {
		b.WriteString("\nNo improvements suggested.\n")
		return b.String()
	}
	for i := range s {
		c := &s[i]
		fmt.Fprintf(&b, "\n### %s\n\n`%s` · %s\n", c.Summary, location(c.File, c.StartLine, c.EndLine), c.Category)
		if c.ExistingCode != "" {
			b.WriteString("\n**Before**\n\n")
			b.WriteString(codeBlock(c.File, c.ExistingCode))
		}
		b.WriteString("\n**After**\n\n")
		b.WriteString(codeBlock(c.File, c.ImprovedCode))
	}
	return b.String()
}

func countBySeverity(findings []Finding) string {
	if len(findings) == 0 {
		return "none"
	}
	counts := map[Severity]int{}
	for i := range findings {
		counts[findings[i].Severity]++
	}
	parts := make([]string, 0, 4)
	for _, s := range []Severity{SeverityCritical, SeverityMajor, SeverityMinor, SeverityTrivial} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	return fmt.Sprintf("%d (%s)", len(findings), strings.Join(parts, ", "))
}

func location(file string, start, end int) string {
	if start == end {
		return fmt.Sprintf("%s:%d", file, start)
	}
	return fmt.Sprintf("%s:%d-%d", file, start, end)
}

func codeList(paths []string) string {
	q := make([]string, 0, len(paths))
	for _, p := range paths {
		q = append(q, "`"+p+"`")
	}
	return strings.Join(q, ", ")
}

// codeBlock fences code with a language hint from the file extension. The
// fence is longer than any backtick run inside the code, so model-written
// code containing ``` cannot break out of the block.
func codeBlock(file, code string) string {
	longest, run := 0, 0
	for _, r := range code {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + fenceLang(file) + "\n" + strings.TrimRight(code, "\n") + "\n" + fence + "\n"
}

var extLang = map[string]string{
	".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript", ".tsx": "tsx",
	".rs": "rust", ".java": "java", ".rb": "ruby", ".sh": "bash", ".yaml": "yaml", ".yml": "yaml",
	".json": "json", ".cue": "cue", ".md": "markdown", ".sql": "sql", ".c": "c", ".h": "c",
	".cpp": "cpp", ".cs": "csharp", ".tf": "hcl", ".hcl": "hcl", ".toml": "toml",
}

func fenceLang(file string) string {
	if path.Base(file) == "Makefile" {
		return "makefile"
	}
	return extLang[strings.ToLower(path.Ext(file))]
}
