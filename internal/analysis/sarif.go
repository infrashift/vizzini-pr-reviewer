package analysis

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
)

// SARIF 2.1.0, only the parts lint-aware review needs: where a result is,
// which tool and rule produced it, and what it says.
type sarifLog struct {
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool struct {
		Driver struct {
			Name  string      `json:"name"`
			Rules []sarifRule `json:"rules"`
		} `json:"driver"`
	} `json:"tool"`
	OriginalURIBaseIDs map[string]sarifArtifactLocation `json:"originalUriBaseIds"`
	Results            []sarifResult                    `json:"results"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	ShortDescription sarifMessage `json:"shortDescription"`
	FullDescription  sarifMessage `json:"fullDescription"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifResult struct {
	RuleID    string       `json:"ruleId"`
	RuleIndex *int         `json:"ruleIndex"`
	Kind      string       `json:"kind"`
	Message   sarifMessage `json:"message"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
			Region           struct {
				StartLine   int `json:"startLine"`
				StartColumn int `json:"startColumn"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
	Suppressions []struct {
		Status string `json:"status"`
	} `json:"suppressions"`
}

// ParseSARIF reads a SARIF 2.1.0 log (ruff, Roslyn analyzers, Semgrep,
// CodeQL, …) into diagnostics. Each result's first physical location is used:
// its artifact URI is resolved through the run's originalUriBaseIds, file://
// URIs are made relative to root (the repository root, absolute), and
// relative paths are cleaned. Results with no line, results of kind "pass"
// or "notApplicable", and suppressed results are skipped. Linter is the
// tool's driver name; Text is "ruleId: message".
func ParseSARIF(r io.Reader, root string) ([]Diagnostic, error) {
	var log sarifLog
	if err := json.NewDecoder(r).Decode(&log); err != nil {
		return nil, fmt.Errorf("parse SARIF: %w", err)
	}
	if log.Runs == nil {
		return nil, errors.New("parse SARIF: no runs")
	}
	var out []Diagnostic
	for i := range log.Runs {
		run := &log.Runs[i]
		for j := range run.Results {
			res := &run.Results[j]
			if res.Kind == "pass" || res.Kind == "notApplicable" || suppressed(res) || len(res.Locations) == 0 {
				continue
			}
			loc := res.Locations[0].PhysicalLocation
			if loc.Region.StartLine < 1 {
				continue
			}
			file, ok := resolveURI(loc.ArtifactLocation, run.OriginalURIBaseIDs, root)
			if !ok {
				continue
			}
			text := strings.TrimSpace(res.Message.Text)
			if text == "" {
				text = ruleDescription(run, res)
			}
			if res.RuleID != "" {
				text = res.RuleID + ": " + text
			}
			out = append(out, Diagnostic{
				Linter: run.Tool.Driver.Name,
				Text:   text,
				File:   file,
				Line:   loc.Region.StartLine,
				Column: loc.Region.StartColumn,
			})
		}
	}
	return out, nil
}

func suppressed(r *sarifResult) bool {
	for _, s := range r.Suppressions {
		// SARIF: a suppression without a status, or with status "accepted",
		// is in effect; "underReview" and "rejected" are not.
		if s.Status == "" || s.Status == "accepted" {
			return true
		}
	}
	return false
}

// ruleDescription falls back to the rule's description when a result
// carries no message text (e.g. it uses a message id).
func ruleDescription(run *sarifRun, res *sarifResult) string {
	rules := run.Tool.Driver.Rules
	if res.RuleIndex != nil && *res.RuleIndex >= 0 && *res.RuleIndex < len(rules) {
		return describe(&rules[*res.RuleIndex])
	}
	for i := range rules {
		if rules[i].ID == res.RuleID {
			return describe(&rules[i])
		}
	}
	return ""
}

func describe(r *sarifRule) string {
	if s := strings.TrimSpace(r.ShortDescription.Text); s != "" {
		return s
	}
	return strings.TrimSpace(r.FullDescription.Text)
}

// resolveURI turns an artifact location into a repository-relative path.
// It reports false for locations outside root or with unusable URIs.
func resolveURI(loc sarifArtifactLocation, bases map[string]sarifArtifactLocation, root string) (string, bool) {
	uri := loc.URI
	// Resolve the uriBaseId chain. An unknown base is taken to be the
	// repository root; a chain longer than any sane log (a cycle) is rejected.
	const maxHops = 8
	base := loc.URIBaseID
	for hops := 0; base != ""; hops++ {
		if hops == maxHops {
			return "", false
		}
		b, ok := bases[base]
		if !ok {
			break
		}
		if !strings.Contains(uri, "://") && !strings.HasPrefix(uri, "/") {
			uri = strings.TrimSuffix(b.URI, "/") + "/" + uri
		}
		base = b.URIBaseID
	}

	if rest, ok := strings.CutPrefix(uri, "file://"); ok {
		// A file URI may name a host before the absolute path; only the
		// path matters, since reports are produced locally.
		if i := strings.IndexByte(rest, '/'); i > 0 {
			rest = rest[i:]
		}
		uri = rest
	} else if strings.Contains(uri, "://") {
		return "", false
	}
	p, err := url.PathUnescape(uri)
	if err != nil {
		return "", false
	}
	p = strings.ReplaceAll(p, `\`, "/")
	// A Windows drive path keeps its drive letter: a leading slash before the
	// drive (as file URIs write it) is dropped, and the path stays absolute.
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	if isAbs(p) {
		r := strings.TrimSuffix(strings.ReplaceAll(root, `\`, "/"), "/")
		if r == "" {
			return "", false
		}
		rel, ok := strings.CutPrefix(p, r+"/")
		if !ok {
			return "", false
		}
		p = rel
	}
	return normalize(p), true
}

func isAbs(p string) bool {
	return strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':')
}

// ParseLintReport reads a linter report in either supported format, telling
// them apart by their top-level keys: golangci-lint JSON has "Issues", SARIF
// has "runs". root is used to relativize absolute paths (SARIF file URIs,
// or absolute golangci-lint paths).
func ParseLintReport(r io.Reader, root string) ([]Diagnostic, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var probe map[string]json.RawMessage
	// Decode only the first JSON value: golangci-lint v2 may append its
	// stats text after the document.
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&probe); err != nil {
		return nil, fmt.Errorf("parse lint report: %w", err)
	}
	switch {
	case probe["runs"] != nil:
		return ParseSARIF(bytes.NewReader(data), root)
	case probe["Issues"] != nil:
		diags, err := ParseGolangciJSON(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return relativize(diags, root), nil
	default:
		return nil, errors.New("parse lint report: neither golangci-lint JSON (\"Issues\") nor SARIF (\"runs\")")
	}
}

func relativize(diags []Diagnostic, root string) []Diagnostic {
	r := strings.TrimSuffix(strings.ReplaceAll(root, `\`, "/"), "/")
	if r == "" {
		return diags
	}
	for i := range diags {
		if rel, ok := strings.CutPrefix(diags[i].File, r+"/"); ok {
			diags[i].File = path.Clean(rel)
		}
	}
	return diags
}
