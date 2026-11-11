// Tests for vizzini/review-gate. Run with:
//
//	garmr test policies/garmr/vizzini/review.cue policies/garmr/vizzini/review-gate_test.cue
//
// No package clause: test files are not part of the policy package.

_finding: {
	file:       "internal/store/users.go"
	start_line: 44
	end_line:   45
	severity:   string | *"Minor"
	category:   string | *"Logic"
	title:      "t"
	body:       "b"
}

_report: {
	apiVersion:       "vizzini.infrashift.io/v1"
	kind:             "Review"
	summary:          "s"
	risk_score:       int | *10
	base_risk_score:  int | *10
	model_risk_score: int | *5
	auto_approve:     false
	findings: [...] | *[]
	chunks: int | *1
}

policy: "vizzini/review-gate"

tests: [
	{
		name:  "clean review is allowed"
		input: _report
		expect: {decision: "allow", noViolations: true}
	},
	{
		name: "Minor and Trivial findings are allowed"
		input: _report & {findings: [_finding, _finding & {severity: "Trivial"}]}
		expect: {decision: "allow", noViolations: true}
	},
	{
		name: "a Critical finding is denied"
		input: _report & {findings: [_finding & {severity: "Critical"}]}
		expect: {decision: "deny", violations: [{id: "VZ-001"}]}
	},
	{
		name: "a Major finding is denied"
		input: _report & {findings: [_finding, _finding & {severity: "Major"}]}
		expect: {decision: "deny", violations: [{id: "VZ-001"}]}
	},
	{
		name: "a violated design decision is denied even at Minor"
		input: _report & {findings: [_finding & {category: "Architecture"}]}
		expect: {decision: "deny", violations: [{id: "VZ-002"}]}
	},
	{
		name: "skipped hunks are denied"
		input: _report & {skipped: ["big.sql"]}
		expect: {decision: "deny", violations: [{id: "VZ-003"}]}
	},
]
