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
