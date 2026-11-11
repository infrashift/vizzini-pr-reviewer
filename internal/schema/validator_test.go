package schema

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"cuelang.org/go/cue"
)

func newValidator(t *testing.T) *Validator {
	t.Helper()
	v, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return v
}

const validFinding = `{"file":"a.go","start_line":3,"end_line":5,"severity":"Major","category":"Logic","title":"t","body":"b"}`

func TestValidateReview(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr string // substring; empty means valid
	}{
		{"valid with findings", `{"summary":"s","risk_score":40,"findings":[` + validFinding + `]}`, ""},
		{"valid without findings", `{"summary":"","risk_score":0,"findings":[]}`, ""},
		{"valid with suggestion", `{"summary":"s","risk_score":100,"findings":[{"file":"a.go","start_line":1,"end_line":1,"severity":"Trivial","category":"Maintainability","title":"t","body":"b","suggestion":"x := 1"}]}`, ""},

		{"malformed JSON", `{"summary":"s",`, "not valid JSON"},
		{"not an object", `["summary"]`, "findings"},
		{"empty input", ``, "not valid JSON"},
		{"invalid severity High", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"Major"`, `"High"`, 1) + `]}`, "severity"},
		{"lowercase severity", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"Major"`, `"major"`, 1) + `]}`, "severity"},
		{"invalid category", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"Logic"`, `"Style"`, 1) + `]}`, "category"},
		{"end before start", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"end_line":5`, `"end_line":2`, 1) + `]}`, "end_line"},
		{"start line zero", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"start_line":3`, `"start_line":0`, 1) + `]}`, "start_line"},
		{"fractional line", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"end_line":5`, `"end_line":5.5`, 1) + `]}`, "end_line"},
		{"empty file", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"a.go"`, `""`, 1) + `]}`, "file"},
		{"empty title", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"title":"t"`, `"title":""`, 1) + `]}`, "title"},
		{"missing body", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `,"body":"b"`, ``, 1) + `]}`, "body"},
		{"risk score above 100", `{"summary":"s","risk_score":101,"findings":[]}`, "risk_score"},
		{"negative risk score", `{"summary":"s","risk_score":-1,"findings":[]}`, "risk_score"},
		{"risk score as string", `{"summary":"s","risk_score":"40","findings":[]}`, "risk_score"},
		{"missing summary", `{"risk_score":1,"findings":[]}`, "summary"},
		{"unknown top-level field", `{"summary":"s","risk_score":1,"findings":[],"verdict":"ok"}`, "verdict"},
		{"unknown finding field", `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"body":"b"`, `"body":"b","confidence":0.9`, 1) + `]}`, "confidence"},
	}

	v := newValidator(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := v.ValidateReview([]byte(tt.json))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				if out == nil {
					t.Fatal("nil output for valid input")
				}
				return
			}
			if err == nil {
				t.Fatalf("want error mentioning %q, got valid output %+v", tt.wantErr, out)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("error %v does not wrap ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateReview_Decodes(t *testing.T) {
	data := `{"summary":"looks fine","risk_score":42,"findings":[{"file":"a.go","start_line":3,"end_line":5,"severity":"Critical","category":"Security","title":"SQLi","body":"user input in query","suggestion":"use args"}]}`
	got, err := newValidator(t).ValidateReview([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	want := &ReviewOutput{
		Summary:   "looks fine",
		RiskScore: 42,
		Findings: []Finding{{
			File: "a.go", StartLine: 3, EndLine: 5,
			Severity: SeverityCritical, Category: CategorySecurity,
			Title: "SQLi", Body: "user input in query", Suggestion: "use args",
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// The error text goes back to the model verbatim, so it must name the field
// and the allowed values without noise.
func TestValidateReview_ErrorIsRepairFriendly(t *testing.T) {
	bad := `{"summary":"s","risk_score":1,"findings":[` + strings.Replace(validFinding, `"Major"`, `"High"`, 1) + `]}`
	_, err := newValidator(t).ValidateReview([]byte(bad))
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	for _, want := range []string{`findings.0.severity: conflicting values "Critical" and "High"`, `conflicting values "Trivial" and "High"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
	if strings.Contains(msg, "empty disjunction") {
		t.Errorf("error %q keeps the disjunction summary line", msg)
	}
}

// CUE treats the open list [...#Finding] as concrete, defaulting to [], so an
// omitted "findings" means "no findings" rather than an error.
func TestValidateReview_MissingFindingsIsEmpty(t *testing.T) {
	got, err := newValidator(t).ValidateReview([]byte(`{"summary":"s","risk_score":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 0 {
		t.Errorf("findings = %v, want empty", got.Findings)
	}
}

func TestValidate_UnknownDefinition(t *testing.T) {
	var out map[string]any
	err := newValidator(t).Validate("#Nope", []byte(`{}`), &out)
	if err == nil || errors.Is(err, ErrInvalid) {
		t.Errorf("want a non-ErrInvalid error for an unknown definition, got %v", err)
	}
}

// The Go enums must list exactly the values of the CUE disjunctions, in order,
// so prompts and policy code never drift from the schema.
func TestEnumsMatchSchema(t *testing.T) {
	v := newValidator(t)
	check := func(def string, want []string) {
		t.Helper()
		op, args := v.schema.LookupPath(cue.ParsePath(def)).Expr()
		if op != cue.OrOp {
			t.Fatalf("%s is not a disjunction (op %v)", def, op)
		}
		got := make([]string, 0, len(args))
		for _, a := range args {
			s, err := a.String()
			if err != nil {
				t.Fatalf("%s: %v", def, err)
			}
			got = append(got, s)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %v in CUE, Go has %v", def, got, want)
		}
	}

	sev := make([]string, 0, len(Severities))
	for _, s := range Severities {
		sev = append(sev, string(s))
	}
	check("#Severity", sev)

	cat := make([]string, 0, len(Categories))
	for _, c := range Categories {
		cat = append(cat, string(c))
	}
	check("#Category", cat)

	types := make([]string, 0, len(PRTypes))
	for _, p := range PRTypes {
		types = append(types, string(p))
	}
	check("#PRType", types)
}

func TestValidateDescription(t *testing.T) {
	v := newValidator(t)
	valid := `{"title":"Add retries","type":"Feature","summary":"s","changes":[{"file":"a.go","description":"d"}]}`
	got, err := v.ValidateDescription([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	want := &PRDescription{Title: "Add retries", Type: PRTypeFeature, Summary: "s", Changes: []FileChange{{File: "a.go", Description: "d"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	for name, bad := range map[string]string{
		"unknown type":    strings.Replace(valid, `"Feature"`, `"Enhancement"`, 1),
		"empty title":     strings.Replace(valid, `"Add retries"`, `""`, 1),
		"empty change":    strings.Replace(valid, `"description":"d"`, `"description":""`, 1),
		"extra field":     strings.Replace(valid, `"summary":"s"`, `"summary":"s","breaking":true`, 1),
		"missing summary": strings.Replace(valid, `,"summary":"s"`, ``, 1),
	} {
		if _, err := v.ValidateDescription([]byte(bad)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestValidateImprove(t *testing.T) {
	v := newValidator(t)
	valid := `{"suggestions":[{"file":"a.go","start_line":2,"end_line":4,"category":"Performance","summary":"s","existing_code":"x","improved_code":"y"}]}`
	got, err := v.ValidateImprove([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Suggestions) != 1 || got.Suggestions[0].Category != CategoryPerformance || got.Suggestions[0].EndLine != 4 {
		t.Errorf("decoded %+v", got)
	}
	if _, err := v.ValidateImprove([]byte(`{"suggestions":[]}`)); err != nil {
		t.Errorf("empty suggestions rejected: %v", err)
	}

	for name, bad := range map[string]string{
		"end before start":  strings.Replace(valid, `"end_line":4`, `"end_line":1`, 1),
		"bad category":      strings.Replace(valid, `"Performance"`, `"Speed"`, 1),
		"empty improvement": strings.Replace(valid, `"improved_code":"y"`, `"improved_code":""`, 1),
		"missing existing":  strings.Replace(valid, `"existing_code":"x",`, ``, 1),
	} {
		if _, err := v.ValidateImprove([]byte(bad)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestValidator_Concurrent(t *testing.T) {
	v := newValidator(t)
	valid := []byte(`{"summary":"s","risk_score":40,"findings":[` + validFinding + `]}`)
	invalid := []byte(`{"summary":"s","risk_score":400,"findings":[]}`)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, wantErr := valid, false
			if i%2 == 1 {
				data, wantErr = invalid, true
			}
			for range 20 {
				_, err := v.ValidateReview(data)
				if (err != nil) != wantErr {
					t.Errorf("goroutine %d: err = %v, wantErr %v", i, err, wantErr)
					return
				}
			}
		}()
	}
	wg.Wait()
}
