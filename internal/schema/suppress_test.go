package schema

import (
	"errors"
	"testing"
)

func TestSuppressions(t *testing.T) {
	v, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, ok := range []string{
		`{"suppressions":[]}`,
		`{"suppressions":[{"id":"S1","reason":"frozen module","paths":["src/Legacy/**"],"categories":["Maintainability"]}]}`,
		`{"suppressions":[{"id":"S2","reason":"r","title":"unused variable","expires":"2027-01-31"}]}`,
		`{"suppressions":[{"id":"S3","reason":"r","fingerprint":"v1:0123456789abcdef"}]}`,
	} {
		if _, err := v.Suppressions([]byte(ok)); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for name, bad := range map[string]string{
		"names nothing specific": `{"suppressions":[{"id":"S","reason":"r","categories":["Logic"]}]}`,
		"no reason":              `{"suppressions":[{"id":"S","reason":"","title":"x"}]}`,
		"unknown field":          `{"suppressions":[{"id":"S","reason":"r","title":"x","severity":"Minor"}]}`,
		"bad fingerprint":        `{"suppressions":[{"id":"S","reason":"r","fingerprint":"abc"}]}`,
		"bad date":               `{"suppressions":[{"id":"S","reason":"r","title":"x","expires":"next year"}]}`,
		"unknown category":       `{"suppressions":[{"id":"S","reason":"r","title":"x","categories":["Style"]}]}`,
		"empty paths":            `{"suppressions":[{"id":"S","reason":"r","paths":[]}]}`,
		"duplicate id":           `{"suppressions":[{"id":"S","reason":"r","title":"x"},{"id":"S","reason":"r","title":"y"}]}`,
		"bad id":                 `{"suppressions":[{"id":"has space","reason":"r","title":"x"}]}`,
	} {
		if _, err := v.Suppressions([]byte(bad)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want invalid", name, err)
		}
	}
}
