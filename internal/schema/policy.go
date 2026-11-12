package schema

import (
	"encoding/json"
	"fmt"
	"slices"
)

// PolicyGate mirrors #PolicyGate: the severities that block a change.
type PolicyGate struct {
	FailOn []Severity `json:"fail_on"`
}

// Policy builds a gate through the CUE schema, so the default and the allowed
// severities come from #PolicyGate. A nil failOn selects the schema default
// (Critical and Major); an empty, non-nil slice blocks nothing.
func (v *Validator) Policy(failOn []Severity) (*PolicyGate, error) {
	input := map[string]any{}
	if failOn != nil {
		input["fail_on"] = failOn
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode policy: %w", err)
	}
	var p PolicyGate
	if err := v.Validate("#PolicyGate", data, &p); err != nil {
		return nil, err
	}
	if p.FailOn == nil {
		p.FailOn = []Severity{}
	}
	return &p, nil
}

// RiskWeight mirrors #RiskWeight.
type RiskWeight struct {
	Pattern string  `json:"pattern"`
	Weight  float64 `json:"weight"`
}

// RiskPolicy mirrors #RiskPolicy: path weights and scoring parameters.
type RiskPolicy struct {
	Weights              []RiskWeight `json:"weights"`
	DefaultWeight        float64      `json:"default_weight"`
	Scale                float64      `json:"scale"`
	AutoApproveThreshold int          `json:"auto_approve_threshold"`
	SensitiveWeight      float64      `json:"sensitive_weight"`
	// HighThreshold is the lowest score in the high band; the schema keeps it
	// above AutoApproveThreshold.
	HighThreshold int `json:"high_threshold"`
}

// RiskBand mirrors #RiskBand: a coarse reading of a risk score.
type RiskBand string

// Risk bands, lowest first.
const (
	RiskBandLow    RiskBand = "low"
	RiskBandMedium RiskBand = "medium"
	RiskBandHigh   RiskBand = "high"
)

// IsSensitive reports whether a file of this path weight always needs a
// human: touching one rules out auto-approval however small the change.
func (p *RiskPolicy) IsSensitive(weight float64) bool {
	return weight >= p.SensitiveWeight
}

// Band places a score in a band: low up to AutoApproveThreshold, high from
// HighThreshold, medium between.
func (p *RiskPolicy) Band(score int) RiskBand {
	switch {
	case score <= p.AutoApproveThreshold:
		return RiskBandLow
	case score >= p.HighThreshold:
		return RiskBandHigh
	default:
		return RiskBandMedium
	}
}

// RiskPolicy validates a JSON risk policy against #RiskPolicy. Fields left
// out take their schema defaults; nil or empty data selects every default.
func (v *Validator) RiskPolicy(data []byte) (*RiskPolicy, error) {
	if len(data) == 0 {
		data = []byte("{}")
	}
	var p RiskPolicy
	if err := v.Validate("#RiskPolicy", data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// NoiseRule mirrors #NoiseRule.
type NoiseRule struct {
	Severity   Severity   `json:"severity"`
	Categories []Category `json:"categories,omitempty"`
}

// NoiseFloor mirrors #NoiseFloor: the kinds of finding that are filtered
// out of a review's findings (ISSUE-032, ADR-0024).
type NoiseFloor struct {
	Rules []NoiseRule `json:"rules"`
}

// NoiseFloor validates a JSON noise floor against #NoiseFloor. Nil or empty
// data selects the schema's default rules; {"rules": []} filters nothing.
func (v *Validator) NoiseFloor(data []byte) (*NoiseFloor, error) {
	if len(data) == 0 {
		data = []byte("{}")
	}
	var n NoiseFloor
	if err := v.Validate("#NoiseFloor", data, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

// Matches reports whether a finding of this severity and category is below
// the floor: some rule names its severity and, if the rule lists
// categories, its category. A nil floor matches nothing.
func (n *NoiseFloor) Matches(s Severity, c Category) bool {
	if n == nil {
		return false
	}
	return slices.ContainsFunc(n.Rules, func(r NoiseRule) bool {
		return r.Severity == s && (len(r.Categories) == 0 || slices.Contains(r.Categories, c))
	})
}

// Blocks reports whether findings of severity s fail the gate.
func (p *PolicyGate) Blocks(s Severity) bool {
	return slices.Contains(p.FailOn, s)
}

// Blocking returns the findings whose severity is in FailOn, in order.
func (p *PolicyGate) Blocking(findings []Finding) []Finding {
	var out []Finding
	for i := range findings {
		if p.Blocks(findings[i].Severity) {
			out = append(out, findings[i])
		}
	}
	return out
}
