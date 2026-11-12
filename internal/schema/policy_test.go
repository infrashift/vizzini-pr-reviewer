package schema

import (
	"errors"
	"reflect"
	"testing"
)

func TestPolicy_DefaultComesFromSchema(t *testing.T) {
	p, err := newValidator(t).Policy(nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Severity{SeverityCritical, SeverityMajor}; !reflect.DeepEqual(p.FailOn, want) {
		t.Errorf("default fail_on %v, want %v", p.FailOn, want)
	}
}

func TestPolicy_Explicit(t *testing.T) {
	v := newValidator(t)
	p, err := v.Policy([]Severity{SeverityMinor})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.FailOn, []Severity{SeverityMinor}) {
		t.Errorf("fail_on %v", p.FailOn)
	}

	none, err := v.Policy([]Severity{})
	if err != nil {
		t.Fatal(err)
	}
	if none.FailOn == nil || len(none.FailOn) != 0 {
		t.Errorf("empty policy fail_on %#v, want empty non-nil", none.FailOn)
	}
}

func TestPolicy_RejectsUnknownSeverity(t *testing.T) {
	_, err := newValidator(t).Policy([]Severity{"High"})
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

// Every combination of finding severities against every gate built from a
// subset of severities: a review blocks exactly when some finding's severity
// is in fail_on.
func TestPolicy_AllSeverityPermutations(t *testing.T) {
	v := newValidator(t)
	n := len(Severities)
	for gateMask := range 1 << n {
		failOn := []Severity{}
		for i, s := range Severities {
			if gateMask&(1<<i) != 0 {
				failOn = append(failOn, s)
			}
		}
		gate, err := v.Policy(failOn)
		if err != nil {
			t.Fatalf("gate %v: %v", failOn, err)
		}
		for findMask := range 1 << n {
			var findings []Finding
			for i, s := range Severities {
				if findMask&(1<<i) != 0 {
					findings = append(findings, Finding{File: "a.go", StartLine: 1, EndLine: 1, Severity: s, Category: CategoryLogic, Title: "t", Body: "b"})
				}
			}
			blocking := gate.Blocking(findings)
			wantBlocking := gateMask & findMask
			gotMask := 0
			for _, f := range blocking {
				gotMask |= 1 << indexOf(f.Severity)
			}
			if gotMask != wantBlocking {
				t.Errorf("gate %v, findings mask %04b: blocking mask %04b, want %04b", failOn, findMask, gotMask, wantBlocking)
			}
		}
	}
}

// The default gate blocks Critical and Major only.
func TestPolicy_DefaultBlocksCriticalAndMajorOnly(t *testing.T) {
	gate, err := newValidator(t).Policy(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[Severity]bool{SeverityCritical: true, SeverityMajor: true, SeverityMinor: false, SeverityTrivial: false}
	for s, block := range want {
		if gate.Blocks(s) != block {
			t.Errorf("default gate Blocks(%s) = %v, want %v", s, !block, block)
		}
	}
}

func indexOf(s Severity) int {
	for i, x := range Severities {
		if x == s {
			return i
		}
	}
	return -1
}

func TestRiskPolicy_BandAndSensitivity(t *testing.T) {
	v, err := New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.RiskPolicy([]byte(`{"auto_approve_threshold": 10, "high_threshold": 50, "sensitive_weight": 2}`))
	if err != nil {
		t.Fatal(err)
	}
	for score, want := range map[int]RiskBand{0: RiskBandLow, 10: RiskBandLow, 11: RiskBandMedium, 49: RiskBandMedium, 50: RiskBandHigh, 100: RiskBandHigh} {
		if got := p.Band(score); got != want {
			t.Errorf("Band(%d) = %s, want %s", score, got, want)
		}
	}
	if !p.IsSensitive(2) || p.IsSensitive(1.99) {
		t.Error("IsSensitive must be weight >= sensitive_weight")
	}
	if _, err := v.RiskPolicy([]byte(`{"auto_approve_threshold": 50, "high_threshold": 40}`)); err == nil {
		t.Error("high_threshold below auto_approve_threshold accepted")
	}
}

func TestNoiseFloor_DefaultComesFromSchema(t *testing.T) {
	n, err := newValidator(t).NoiseFloor(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []NoiseRule{
		{Severity: SeverityTrivial},
		{Severity: SeverityMinor, Categories: []Category{CategoryMaintainability, CategoryPerformance}},
	}
	if !reflect.DeepEqual(n.Rules, want) {
		t.Errorf("default rules %+v, want %+v", n.Rules, want)
	}
}

func TestNoiseFloor_DefaultMatches(t *testing.T) {
	n, err := newValidator(t).NoiseFloor(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		s    Severity
		c    Category
		want bool
	}{
		{SeverityTrivial, CategoryLogic, true},
		{SeverityTrivial, CategorySecurity, true},
		{SeverityMinor, CategoryMaintainability, true},
		{SeverityMinor, CategoryPerformance, true},
		{SeverityMinor, CategoryLogic, false},
		{SeverityMinor, CategorySecurity, false},
		{SeverityMinor, CategoryArchitecture, false},
		{SeverityMajor, CategoryMaintainability, false},
		{SeverityCritical, CategoryPerformance, false},
	} {
		if got := n.Matches(tc.s, tc.c); got != tc.want {
			t.Errorf("Matches(%s, %s) = %v, want %v", tc.s, tc.c, got, tc.want)
		}
	}
}

func TestNoiseFloor_ExplicitAndEmpty(t *testing.T) {
	v := newValidator(t)
	n, err := v.NoiseFloor([]byte(`{"rules": [{"severity": "Minor"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !n.Matches(SeverityMinor, CategoryLogic) || n.Matches(SeverityTrivial, CategoryLogic) {
		t.Errorf("explicit rules %+v", n.Rules)
	}
	none, err := v.NoiseFloor([]byte(`{"rules": []}`))
	if err != nil {
		t.Fatal(err)
	}
	if none.Matches(SeverityTrivial, CategoryMaintainability) {
		t.Error("an empty floor must filter nothing")
	}
	var nilFloor *NoiseFloor
	if nilFloor.Matches(SeverityTrivial, CategoryLogic) {
		t.Error("a nil floor must filter nothing")
	}
}

func TestNoiseFloor_RejectsInvalid(t *testing.T) {
	v := newValidator(t)
	for _, in := range []string{
		`{"rules": [{"severity": "Low"}]}`,
		`{"rules": [{"severity": "Minor", "categories": ["Style"]}]}`,
		`{"rules": [{"severity": "Minor", "categories": []}]}`,
		`{"rules": [{"category": "Logic"}]}`,
		`{"rules": [], "extra": true}`,
	} {
		if _, err := v.NoiseFloor([]byte(in)); err == nil {
			t.Errorf("NoiseFloor(%s) accepted", in)
		}
	}
}
