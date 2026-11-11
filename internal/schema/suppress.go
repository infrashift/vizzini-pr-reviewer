package schema

import "fmt"

// Suppression mirrors #Suppression: a repository's decision not to be told
// about a kind of finding again (ISSUE-024 slice 5, ADR-0020).
type Suppression struct {
	ID          string     `json:"id"`
	Reason      string     `json:"reason"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	Paths       []string   `json:"paths,omitempty"`
	Categories  []Category `json:"categories,omitempty"`
	Title       string     `json:"title,omitempty"`
	Expires     string     `json:"expires,omitempty"`
}

// Suppressions mirrors #Suppressions.
type Suppressions struct {
	Suppressions []Suppression `json:"suppressions"`
}

// SuppressedFinding mirrors #SuppressedFinding.
type SuppressedFinding struct {
	Rule    string  `json:"rule"`
	Reason  string  `json:"reason"`
	Finding Finding `json:"finding"`
}

// Suppressions validates JSON suppression rules against #Suppressions
// (including its at-least-one check) and that rule ids are unique.
func (v *Validator) Suppressions(data []byte) (*Suppressions, error) {
	var s Suppressions
	if err := v.Validate("#Suppressions", data, &s); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i := range s.Suppressions {
		id := s.Suppressions[i].ID
		if seen[id] {
			return nil, fmt.Errorf("%w: suppression id %q is used twice", ErrInvalid, id)
		}
		seen[id] = true
	}
	return &s, nil
}
