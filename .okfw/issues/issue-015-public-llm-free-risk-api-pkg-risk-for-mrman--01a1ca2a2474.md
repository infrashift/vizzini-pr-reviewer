---
id: "01a1ca2a-2474-707d-9729-d7f488f9e241"
title: "ISSUE-015: Public LLM-free risk API (pkg/risk) for mrman"
type: "Issue"
kind: "issue"
tags:
  - "issue"
  - "open"
  - "proposed"
  - "risk"
  - "mrman"
  - "api"
created: "2026-11-11T04:32:01-06:00"
timestamp: "2026-11-11T04:32:01-06:00"
description: "Expose an LLM-free pkg/risk (AssessRisk, Policy, Band, ParseReport) that mrman can embed without linking the LLM client; the prerequisite for ISSUE-013."
---

# ISSUE-015: Public LLM-free risk API (pkg/risk) for mrman

**Status:** open (proposed) · **Raised:** 2026-11-11 by the mrman side · **Resolves:** the prerequisite in [ISSUE-013](/issues/issue-013-explore-what-mrman-gains-from-consuming-vizzinis-risk-score--01a1ca04b280.md) · **Respects:** [ADR-0008](/decisions/adr-0008-deterministic-risk-score-model-can-only-raise-it-sensitive-paths-never-auto-approve--01a1c9e969a2.md)

## Decision from the mrman side (closes ISSUE-013's discovery question)
mrman will build all five candidate uses, opt-in behind `[risk] enabled` (off by default):
1. File tree marks sensitive files (weight ≥ `sensitive_weight`) and optionally shows per-file points.
2. MR list risk chip plus sort by `base_risk_score`.
3. Review-session header: `risk_score` with the diff/model split, and `auto_approve` as a suggestion only.
4. A divergence warning when `model_risk_score` is far above `base_risk_score`.
5. `risk_files` exposed to agents over mrman's JSON CLI.

mrman will **embed** a Vizzini package for uses 1, 2 and 5. It will **never call a model**: uses 3 and 4 read a `pr-review --format json` report (or a garmr envelope) that the user or CI produced. mrman displays bands with the number (`● med 42`).

## What mrman needs
A new public package `pkg/risk` that mrman can import **without linking any LLM, tokenizer or network code**.

### Hard constraint: a leaf dependency set
`go list -deps ./pkg/risk` must not contain `github.com/sashabaranov/go-openai`, `github.com/pkoukk/tiktoken-go` or `net/http`. CUE (`cuelang.org/go`, which mrman already depends on at v0.17.1) and `go-gitdiff` are fine. Add a test that enforces this, so a later refactor can't silently drag the LLM client into mrman's binary.

### API (names negotiable, semantics not)
```go
package risk

// Policy is the validated #RiskPolicy (weights, default_weight, scale,
// auto_approve_threshold, sensitive_weight, high_threshold).
type Policy struct { /* exported fields mirroring schema.RiskPolicy */ }

func DefaultPolicy() *Policy                       // schema defaults
func LoadPolicy(data []byte) (*Policy, error)      // CUE-validated + ValidateRiskPatterns; nil/empty → defaults
func (p *Policy) IsSensitive(weight float64) bool  // weight >= SensitiveWeight

// FileChange is a parsed file, so callers with their own diff model need not
// re-serialize to a patch. Added/Deleted are line texts without the +/- prefix.
type FileChange struct {
    Path    string
    Added   []string
    Deleted []string
    Binary  bool
}

type FileRisk struct { // same JSON as today's analysis.FileRisk
    Path string; Weight float64; Changed, ComplexityDelta int; Points float64
}
type Assessment struct {
    Score int        `json:"score"`  // == base_risk_score
    Files []FileRisk `json:"files"`  // == risk_files, highest first
}

func ScoreFile(f FileChange, p *Policy) FileRisk           // one file, no sorting
func Combine(files []FileRisk, p *Policy) int              // total points → 0..100
func Score(files []FileChange, p *Policy) Assessment       // ScoreFile + Combine + sort
func AssessPatch(r io.Reader, p *Policy) (Assessment, error) // unified diff in, same result

type Band string // "low" | "medium" | "high"
func (p *Policy) Band(score int) Band // low ≤ auto_approve_threshold < medium < high_threshold ≤ high

// Report is the risk subset of a ReviewOutput, decoded without importing pkg/reviewer.
type Report struct {
    RiskScore      int        `json:"risk_score"`
    BaseRiskScore  int        `json:"base_risk_score"`
    ModelRiskScore int        `json:"model_risk_score"`
    RiskFiles      []FileRisk `json:"risk_files"`
    AutoApprove    bool       `json:"auto_approve"`
}
// ParseReport accepts either a bare `pr-review --format json` review output or
// the garmr envelope produced by reviewer.MarshalReport, and rejects anything else.
func ParseReport(data []byte) (*Report, error)
```

Why `ScoreFile`/`Combine` and not only `Score`: mrman can list one path twice (a patch series touches it once per patch), so it needs per-entry results aligned to its own file indices, not path-keyed ones.

### Single implementation
`internal/analysis.ScoreRisk` must delegate to `pkg/risk` (or the reverse), and `pkg/reviewer.FileRisk` becomes an alias of `risk.FileRisk`. The score mrman shows must be byte-identical to the `base_risk_score` that `Review` emits for the same diff and policy. Add a test that runs both on the existing testdata.

### Schema addition
Add `high_threshold: int & >=0 & <=100 | *60` to `#RiskPolicy`, with a constraint that it's greater than `auto_approve_threshold`. This lets bands be policy-tuned (ISSUE-013's open question about bands), and every consumer shares one definition.

### Policy location convention
Document `.vizzini/risk.json` at the repository root as the conventional per-repo policy. mrman will look for it there. `pr-review --risk-policy` keeps working as before.

### CLI (small, optional but useful)
`pr-review --mode risk [--risk-policy f] [--format json|markdown]`: LLM-free, needs no proxy or env, and prints the `Assessment`. This is useful for CI and for checking mrman's numbers by hand.

### Release
mrman will pin a pseudo-version of the commit that lands this. Tagging isn't required, but the `pkg/risk` API should be treated as stable from then on.

## Out of scope here
- `OrderFilesByDependency` as a leaf package (ISSUE-006). mrman may want it later, in a separate issue.
- garmr policy evaluation inside mrman.

## Acceptance Criteria
- [ ] `pkg/risk` with the API above, docs, and ≥ the repo's coverage bar
- [ ] Leaf-dependency test (no go-openai, tiktoken-go or net/http in `go list -deps ./pkg/risk`)
- [ ] `analysis.ScoreRisk`, `Review` and `pkg/risk` share one implementation; an equivalence test proves `Assessment.Score == ReviewOutput.BaseRiskScore`
- [ ] `ParseReport` handles both the bare review JSON and the `MarshalReport` envelope (fixtures for each, plus a rejection case)
- [ ] `high_threshold` in `#RiskPolicy` with a default and validation; `Policy.Band` tested at the boundaries
- [ ] `.vizzini/risk.json` convention documented
- [ ] (optional) `pr-review --mode risk`
- [ ] ISSUE-013 updated with the decision and closed

## Related
- [ISSUE-007: Automated risk scoring and approvals](/issues/issue-007-automated-risk-scoring-and-approvals--01a1c9b61578.md)
- [ISSUE-013](/issues/issue-013-explore-what-mrman-gains-from-consuming-vizzinis-risk-score--01a1ca04b280.md)
