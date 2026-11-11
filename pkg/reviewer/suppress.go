package reviewer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"cuelang.org/go/cue/cuecontext"
	cueyaml "cuelang.org/go/encoding/yaml"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

// Suppression rules (ISSUE-024 slice 5, ADR-0020): a repository's reviewed
// decisions not to be told about a kind of finding again. A finding a rule
// matches is moved from ReviewOutput.Findings to ReviewOutput.Suppressed,
// with the rule's id and reason: it is not posted and does not fail the
// policy gate, but it still counts against auto-approval, so a rule can
// never unlock merging without a human.

// Suppression is one rule, validated against #Suppression.
type Suppression = schema.Suppression

// SuppressedFinding is a finding a rule matched.
type SuppressedFinding = schema.SuppressedFinding

// Suppressions is a validated rule set. The zero value and nil match
// nothing.
type Suppressions struct {
	rules []Suppression
}

// ParseSuppressions validates suppression rules, JSON or YAML, against
// #Suppressions:
//
//	suppressions:
//	  - id: SUP-1
//	    reason: The legacy formatter is frozen until its rewrite.
//	    paths: ["src/Legacy/**"]
//	    categories: [Maintainability]
//
// A rule names at least one of fingerprint, paths or title; categories and
// expires narrow it further.
func ParseSuppressions(data []byte) (*Suppressions, error) {
	if !json.Valid(data) {
		f, err := cueyaml.Extract("suppressions.yaml", data)
		if err != nil {
			return nil, fmt.Errorf("suppressions: %w", err)
		}
		v := cuecontext.New().BuildFile(f)
		if data, err = v.MarshalJSON(); err != nil {
			return nil, fmt.Errorf("suppressions: %w", err)
		}
	}
	val, err := reportValidator()
	if err != nil {
		return nil, err
	}
	s, err := val.Suppressions(data)
	if err != nil {
		return nil, fmt.Errorf("suppressions: %w", err)
	}
	return &Suppressions{rules: s.Suppressions}, nil
}

// Len is the number of rules.
func (s *Suppressions) Len() int {
	if s == nil {
		return 0
	}
	return len(s.rules)
}

// Match returns the first rule, in file order, that matches f and has not
// expired at now; nil when none does.
func (s *Suppressions) Match(f *Finding, now time.Time) *Suppression {
	if s == nil {
		return nil
	}
	today := now.UTC().Format(time.DateOnly)
	for i := range s.rules {
		r := &s.rules[i]
		if r.Expires != "" && r.Expires < today {
			continue // valid through its expiry day
		}
		if matchesRule(r, f) {
			return r
		}
	}
	return nil
}

func matchesRule(r *Suppression, f *Finding) bool {
	if r.Fingerprint != "" && r.Fingerprint != Fingerprint(f) {
		return false
	}
	if len(r.Categories) > 0 && !slices.Contains(r.Categories, f.Category) {
		return false
	}
	if r.Title != "" && !strings.Contains(strings.ToLower(f.Title), strings.ToLower(r.Title)) {
		return false
	}
	if len(r.Paths) > 0 && !slices.ContainsFunc(r.Paths, func(p string) bool {
		ok, err := diff.MatchGlob(p, strings.TrimPrefix(f.File, "/"))
		return err == nil && ok
	}) {
		return false
	}
	return true
}

// Apply splits findings into those no rule matches and those one does.
func (s *Suppressions) Apply(findings []Finding, now time.Time) (kept []Finding, suppressed []SuppressedFinding) {
	kept = make([]Finding, 0, len(findings))
	for i := range findings {
		if r := s.Match(&findings[i], now); r != nil {
			suppressed = append(suppressed, SuppressedFinding{Rule: r.ID, Reason: r.Reason, Finding: findings[i]})
			continue
		}
		kept = append(kept, findings[i])
	}
	return kept, suppressed
}

// Fingerprint identifies a finding independently of its line numbers, so a
// finding that moves when code above it changes keeps its identity: the
// file, the category and the normalized title. "v1" versions the recipe;
// vizzini-bot's thread markers and suppression rules use it.
func Fingerprint(f *Finding) string {
	title := strings.Join(strings.Fields(strings.ToLower(f.Title)), " ")
	sum := sha256.Sum256([]byte(f.File + "\x00" + string(f.Category) + "\x00" + title))
	return "v1:" + hex.EncodeToString(sum[:8])
}
