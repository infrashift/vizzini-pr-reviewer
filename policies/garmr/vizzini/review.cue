// Vizzini review policies for garmr (ADR-0010).
//
// garmr evaluates the JSON report `pr-review --format json` prints. Reports
// carry apiVersion "vizzini.infrashift.io/v1" and kind "Review", which is
// what these policies target, so they never fire on other inputs.
//
//   GARMR_HTTP_ADDR=:8080 garmr-server --policy-dir policies/garmr
//   pr-review --mode review --fail-on none --format json > review.json
//   garmr eval --input review.json -n vizzini
//
// The namespace yields DENY when a gate rule fails, WARN when only advisory
// rules fail, and ALLOW otherwise. Copy the pack and tune it: severities,
// the risk threshold and the sensitive-path weight are plain values here.
package vizzini

_reviewTarget: resources: [{apiGroup: "vizzini.infrashift.io", kind: "Review"}]

// Gate: merge blockers.
reviewGate: {
	apiVersion: "policy.garmr.io/v1"
	kind:       "Policy"
	metadata: {
		name:      "review-gate"
		namespace: "vizzini"
	}
	spec: {
		description: "Blocks a change on Critical or Major findings, on violated design decisions, and on incomplete reviews."
		target:      _reviewTarget
		rules: [
			{
				id:          "VZ-001"
				description: "No Critical or Major findings"
				severity:    "critical"
				priority:    10
				expr: forEach: {
					path: "findings"
					as:   "f"
					condition: match: {path: "f.severity", notIn: ["Critical", "Major"]}
				}
				message:     "{{f.severity}} finding at {{f.file}}:{{f.start_line}}: {{f.title}}"
				remediation: "Fix the finding, or have a human reviewer dismiss it explicitly."
				category:    "review"
			},
			{
				id:          "VZ-002"
				description: "No violated design decisions"
				severity:    "high"
				priority:    20
				expr: forEach: {
					path: "findings"
					as:   "f"
					condition: match: {path: "f.category", notIn: ["Architecture"]}
				}
				message:     "change violates a documented design decision at {{f.file}}:{{f.start_line}}: {{f.title}}"
				remediation: "Follow the cited knowledge-base document, or supersede it with a new ADR first."
				category:    "architecture"
			},
			{
				id:          "VZ-003"
				description: "Every hunk was reviewed"
				severity:    "high"
				priority:    30
				expr: match: {path: "skipped", exists: false}
				message:     "some hunks exceeded the token budget and were not reviewed"
				remediation: "Re-run without --skip-oversized, or split the change."
				category:    "review"
			},
		]
		enforcement: action: "deny"
	}
}

// Advisory: worth a human's attention, but not blocking.
reviewAdvisory: {
	apiVersion: "policy.garmr.io/v1"
	kind:       "Policy"
	metadata: {
		name:      "review-advisory"
		namespace: "vizzini"
	}
	spec: {
		description: "Warns on high-risk changes, sensitive paths and changes with nothing reviewable."
		target:      _reviewTarget
		rules: [
			{
				id:          "VZA-001"
				description: "Risk score at most 70"
				severity:    "medium"
				priority:    10
				expr: match: {path: "risk_score", lessThanOrEqual: 70}
				message:     "risk score {{risk_score}}/100 (diff {{base_risk_score}}, model {{model_risk_score}})"
				remediation: "Ask for a second reviewer on high-risk changes."
				category:    "risk"
			},
			{
				id:          "VZA-002"
				description: "No sensitive paths touched"
				severity:    "medium"
				priority:    20
				// risk_files is omitted when nothing was reviewable; absent passes.
				expr: any: [
					{match: {path: "risk_files", exists: false}},
					{forEach: {
						path: "risk_files"
						as:   "r"
						condition: match: {path: "r.weight", lessThan: 2.5}
					}},
				]
				message:     "{{r.path}} is a sensitive path (risk weight {{r.weight}})"
				remediation: "Route the change to the owners of this area."
				category:    "risk"
			},
			{
				id:          "VZA-003"
				description: "Something was reviewed"
				severity:    "low"
				priority:    30
				expr: match: {path: "chunks", greaterThanOrEqual: 1}
				message:     "nothing reviewable (only generated, vendored or lock files changed)"
				remediation: "Check dependency bumps by hand; they are supply-chain decisions."
				category:    "review"
			},
		]
		enforcement: action: "warn"
	}
}
