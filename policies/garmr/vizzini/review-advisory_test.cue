// Tests for vizzini/review-advisory. Run with:
//
//	garmr test policies/garmr/vizzini/review.cue policies/garmr/vizzini/review-advisory_test.cue

_report: {
	apiVersion:       "vizzini.infrashift.io/v1"
	kind:             "Review"
	summary:          "s"
	risk_score:       int | *10
	base_risk_score:  int | *10
	model_risk_score: int | *5
	auto_approve:     false
	findings: []
	risk_files: [...] | *[{path: "internal/api/list.go", weight: 1, changed: 12, complexity_delta: 1, points: 17}]
	chunks: int | *1
}

policy: "vizzini/review-advisory"

tests: [
	{
		name:  "low-risk review is allowed"
		input: _report
		expect: {decision: "allow", noViolations: true}
	},
	{
		name: "high risk warns"
		input: _report & {risk_score: 85, base_risk_score: 85}
		expect: {decision: "warn", violations: [{id: "VZA-001"}]}
	},
	{
		name: "a sensitive path warns"
		input: _report & {risk_files: [{path: "internal/auth/login.go", weight: 3, changed: 3, complexity_delta: 0, points: 9}]}
		expect: {decision: "warn", violations: [{id: "VZA-002"}]}
	},
	{
		name: "a report without risk_files passes the sensitive-path rule"
		input: {
			apiVersion: "vizzini.infrashift.io/v1", kind: "Review", summary:    "s"
			risk_score: 10, base_risk_score:              10, model_risk_score: 5, auto_approve: false
			findings: [], chunks: 1
		}
		expect: {decision: "allow", noViolations: true}
	},
	{
		name: "nothing reviewable warns"
		input: _report & {chunks: 0, risk_files: []}
		expect: {decision: "warn", violations: [{id: "VZA-003"}]}
	},
]
