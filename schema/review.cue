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
}

// Slash commands (Milestone 4.6). /explain answers with #Explanation;
// /refactor and /docstrings answer with #ImproveOutput.
#Explanation: {
	answer: string & !=""
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
