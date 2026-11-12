// Canonical contract for every LLM review response (ADR-0003). Definitions are
// closed: unknown fields are rejected, not ignored.
package review

#Severity: "Critical" | "Major" | "Minor" | "Trivial"
#Category: "Security" | "Performance" | "Logic" | "Maintainability" | "Architecture"

#Finding: {
	file:        string & !=""
	start_line:  int & >=1
	end_line:    int & >=start_line
	severity:    #Severity
	category:    #Category
	title:       string & !=""
	body:        string & !=""
	suggestion?: string
	// citations holds IDs of OKF knowledge-base documents the finding relies
	// on (Milestone 4.5). An Architecture finding must cite at least one: a
	// violated design decision has to be named, not asserted.
	citations?: [...string & !=""]
	if category == "Architecture" {
		citations: [_, ...]
	}
}

#ReviewOutput: {
	summary:    string
	risk_score: int & >=0 & <=100
	findings: [...#Finding]
}

// Describe mode: a pull request description.
#PRType: "Feature" | "Bugfix" | "Refactor" | "Docs" | "Tests" | "Chore"

#FileChange: {
	file:        string & !=""
	description: string & !=""
}

#PRDescription: {
	title:   string & !=""
	type:    #PRType
	summary: string & !=""
	changes: [...#FileChange]
}

// Improve mode: concrete code suggestions.
#CodeSuggestion: {
	file:          string & !=""
	start_line:    int & >=1
	end_line:      int & >=start_line
	category:      #Category
	summary:       string & !=""
	existing_code: string
	improved_code: string & !=""
}

#ImproveOutput: {
	suggestions: [...#CodeSuggestion]
}

// Policy gate (Milestone 4.1): which severities block a change. The CLI exits
// with code 2 when any finding's severity is listed in fail_on.
#PolicyGate: {
	fail_on: [...#Severity] | *["Critical", "Major"]
}

// Noise floor (ISSUE-032, ADR-0024): kinds of finding that the review
// benchmark found are rarely worth a reviewer's time. On 26 real pull
// requests, judged blind, 6% of Trivial findings and about a fifth of Minor
// Performance findings were real problems, against 81% of Major ones. Minor
// Maintainability findings were as noisy, but models sometimes file a real
// bug there, so the default keeps them. A finding a rule matches leaves
// findings for filtered: reported, never posted, never gated on.
#NoiseRule: {
	severity: #Severity
	// Categories; any may match. Omitted: every category.
	categories?: [#Category, ...#Category]
}

#NoiseFloor: {
	rules: [...#NoiseRule] | *[
		{severity: "Trivial"},
		{severity: "Minor", categories: ["Performance"]},
	]
}

// Risk scoring (Milestone 4.4). Each changed file's points are its weight
// times (changed lines + 5 x added cyclomatic complexity). The score is
// 100 x (1 - e^(-total/scale)), so it rises quickly at first and never
// exceeds 100. Weights are matched in order and the first match wins, so list
// specific patterns before general ones. A pattern without "/" matches the
// base name at any depth; "**" matches any number of directories.
#RiskWeight: {
	pattern: string & !=""
	weight:  number & >=0 & <=10
}

#RiskPolicy: {
	weights: [...#RiskWeight] | *[
		{pattern: "*_test.go", weight: 0.3},
		{pattern: "**/testdata/**", weight: 0.2},
		{pattern: "**/docs/**", weight: 0.1},
		{pattern: "*.md", weight: 0.1},
		{pattern: "**/auth/**", weight: 3},
		{pattern: "**/security/**", weight: 3},
		{pattern: "**/crypto/**", weight: 3},
		{pattern: "**/schema/**", weight: 2.5},
		{pattern: "**/migrations/**", weight: 2.5},
	]
	default_weight:         number & >=0 & <=10 | *1
	scale:                  number & >0 | *150
	auto_approve_threshold: int & >=0 & <=100 | *20
	// Touching any file whose weight is at least sensitive_weight rules out
	// auto-approval however small the change: a one-line auth fix still
	// needs a human.
	sensitive_weight: number & >0 | *2.5
	// Bands for display (ISSUE-015): a score at or below
	// auto_approve_threshold is low, one at or above high_threshold is high,
	// anything between is medium. It must stay above auto_approve_threshold,
	// so raising that threshold past 59 means setting this one too.
	high_threshold: (int | *60) & >=0 & <=100 & >auto_approve_threshold
}

// The band a risk score falls in under a #RiskPolicy.
#RiskBand: "low" | "medium" | "high"

// Slash commands (Milestone 4.6). /explain answers with #Explanation;
// /refactor and /docstrings answer with #ImproveOutput; /reconsider with
// #Reconsideration.
#Explanation: {
	answer: string & !=""
}

// /reconsider (ISSUE-029): the bot re-evaluates its own finding after a
// developer disputes it. "withdrawn" resolves the bot's thread; "stands"
// answers and leaves the thread open for a person to decide.
#Reconsideration: {
	verdict:     "withdrawn" | "stands"
	explanation: string & !=""
}

// Pre-code planning (Milestone 4.7): a plan a coding agent can execute.
#FileChangeKind: "create" | "modify" | "delete"

#AffectedFile: {
	path:   string & !=""
	change: #FileChangeKind
	reason: string & !=""
}

#PlanStep: {
	title:       string & !=""
	description: string & !=""
	files: [...string & !=""]
}

#PlanPhase: {
	name: string & !=""
	steps: [#PlanStep, ...#PlanStep]
}

#ImplementationPlan: {
	summary: string & !=""
	affected_files: [#AffectedFile, ...#AffectedFile]
	phases: [#PlanPhase, ...#PlanPhase]
	// Shell commands that prove the work is done (tests, linters, builds).
	verification: [string & !="", ...string & !=""]
	risks?: [...string & !=""]
	// Knowledge-base concepts (paths listed in the OKF index) the plan follows.
	references?: [...string & !=""]
}

// Report envelope (ADR-0010): what pr-review --format json prints, and what
// policy engines such as garmr consume. apiVersion and kind let them target
// Vizzini reports precisely. The review report is defined field by field and
// closed, so policies can rely on its shape; the other kinds carry the
// envelope over their own definitions above.
#APIVersion: "vizzini.infrashift.io/v1"
#ReportKind: "Review" | "PRDescription" | "Suggestions" | "CommandResult" | "ImplementationPlan" | "RiskAssessment"

#Report: {
	apiVersion: #APIVersion
	kind:       #ReportKind
	// The run that produced the report (ISSUE-025, ADR-0015).
	run_id?:    string & !=""
	pr_key?:    string & !=""
	iteration?: string & !=""
	requester?: string & !=""
	agent?:     string & !=""
	usage?:     #Usage
	...
}

#FileRisk: {
	path:             string & !=""
	weight:           number & >=0
	changed:          int & >=0
	complexity_delta: int
	points:           number & >=0
}

// What pr-review --mode risk prints: the deterministic score alone, computed
// without a model (ISSUE-015). Closed like #ReviewReport.
#RiskAssessmentReport: {
	apiVersion: #APIVersion
	kind:       "RiskAssessment"
	score:      int & >=0 & <=100
	band:       #RiskBand
	files: [...#FileRisk]
}

#ReviewReport: {
	apiVersion:       #APIVersion
	kind:             "Review"
	summary:          string
	risk_score:       int & >=0 & <=100
	base_risk_score:  int & >=0 & <=100
	model_risk_score: int & >=0 & <=100
	risk_files?: [...#FileRisk]
	auto_approve: bool
	findings: [...#Finding]
	knowledge_docs?: [...string & !=""]
	excluded?: [...string & !=""]
	skipped?: [...string & !=""]
	chunks:       int & >=0
	diagnostics?: int & >=0
	// Same-span duplicates folded into one finding (ISSUE-019, ADR-0012).
	merged_findings?: int & >=1
	// Repository-context snippets given to the model (ISSUE-023, ADR-0014).
	context_snippets?: int & >=1
	// Findings a repository suppression rule matched: reported here, never
	// posted (ISSUE-024 slice 5). Added by Vizzini after the model answered;
	// the model's #ReviewOutput cannot carry them.
	suppressed?: [...#SuppressedFinding]
	// Findings below the noise floor (ISSUE-032, ADR-0024): reported here,
	// never posted or gated on. Added by Vizzini after the model answered.
	filtered?: [...#Finding]
	// Attribution and usage (ISSUE-025, ADR-0015): run_id joins the
	// proxy's spend logs, pr_key and iteration place the run in its pull
	// request's lifecycle.
	run_id?:    string & !=""
	pr_key?:    string & !=""
	iteration?: string & !=""
	requester?: string & !=""
	agent?:     string & !=""
	usage?:     #Usage
}

// What a run's model requests consumed (ISSUE-025).
#Usage: {
	requests:            int & >=0
	retries:             int & >=0
	repairs:             int & >=0
	input_tokens:        int & >=0
	output_tokens:       int & >=0
	cached_input_tokens: int & >=0
	cost_usd?:           number & >=0
}

// Repository suppression rules (ISSUE-024 slice 5, ADR-0020): findings a
// team decided, in a reviewed change, not to be told about again. A rule
// matches a finding when every condition it names holds; it must name at
// least one of fingerprint, paths or title, so no rule silences everything.
#Suppression: {
	id:     string & =~"^[A-Za-z0-9][A-Za-z0-9._-]*$"
	reason: string & !=""
	// The finding's fingerprint (vizzini-bot's v1 recipe: file, category,
	// normalized title). Exact, but a model rewording the title escapes it.
	fingerprint?: string & =~"^v1:[0-9a-f]{16}$"
	// Path globs with the exclude-glob semantics; any may match.
	paths?: [string & !="", ...string & !=""]
	// Categories; any may match.
	categories?: [#Category, ...#Category]
	// A case-insensitive substring of the finding's title.
	title?: string & !=""
	// The rule lapses after this day (YYYY-MM-DD, UTC).
	expires?: string & =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"
}

#Suppressions: {
	suppressions: [...#Suppression]
	// Every rule names at least one of fingerprint, paths or title.
	_specific: [for r in suppressions {
		[if r.fingerprint != _|_ {1}, if r.paths != _|_ {1}, if r.title != _|_ {1}] & [_, ...]
	}]
}

#SuppressedFinding: {
	rule:    string & !=""
	reason:  string & !=""
	finding: #Finding
}
