// Command stubllm is a stand-in for a LiteLLM proxy used by
// scripts/e2e-golint.sh when no real model is configured. It speaks the
// OpenAI chat-completions API and answers a review request with one finding
// per line of the prompt's <lint> block, and nothing else. Its answers come
// only from the diagnostics it was sent, so a finding in the output proves
// that diagnostic travelled lint JSON -> changed-line filter -> prompt ->
// schema validation -> anchoring. Every user prompt is appended to -log.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type finding struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Severity  string `json:"severity"`
	Category  string `json:"category"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

// Security rule ids from SARIF tools: ruff's flake8-bandit (S608), Roslyn's
// security analyzers (CA2100, CA3xxx, CA5xxx), gosec's own codes (G202), and
// ESLint's code-execution rules plus eslint-plugin-security (security/...).
var securityRule = regexp.MustCompile(`^(S\d{3}|CA2100|CA[35]\d{3}|G\d{3}|no-eval|no-implied-eval|no-new-func|@typescript-eslint/no-implied-eval|security/\S+)$`)

// "path:line[:col] linter: text", as rendered by analysis.Diagnostic.String.
// The linter name may contain spaces ("Microsoft (R) Visual C# Compiler"),
// so it runs to the first ": ".
var lintLine = regexp.MustCompile(`^(\S+?):(\d+)(?::\d+)? (.+?): (.+)$`)

func main() {
	addr := flag.String("addr", "127.0.0.1:18765", "listen address")
	logPath := flag.String("log", "", "file to append every user prompt to")
	flag.Parse()

	var mu sync.Mutex
	http.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string    `json:"model"`
			Messages []message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user := ""
		for _, m := range req.Messages {
			if m.Role == "user" {
				user = m.Content
				break
			}
		}
		if *logPath != "" {
			mu.Lock()
			if f, err := os.OpenFile(*logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				_, _ = fmt.Fprintf(f, "=== prompt ===\n%s\n", user)
				_ = f.Close()
			}
			mu.Unlock()
		}

		findings := []finding{}
		if _, block, ok := strings.Cut(user, "<lint>\n"); ok {
			block, _, _ = strings.Cut(block, "</lint>")
			for _, l := range strings.Split(strings.TrimSpace(block), "\n") {
				m := lintLine.FindStringSubmatch(l)
				if m == nil {
					continue
				}
				line, _ := strconv.Atoi(m[2])
				sev, cat := "Minor", "Maintainability"
				rule, _, _ := strings.Cut(m[4], ":")
				switch {
				case m[3] == "gosec" || securityRule.MatchString(rule):
					sev, cat = "Major", "Security"
				case m[3] == "errcheck":
					cat = "Logic"
				}
				findings = append(findings, finding{
					File: m[1], StartLine: line, EndLine: line, Severity: sev, Category: cat,
					Title: m[3] + ": " + truncate(m[4], 60),
					Body:  "Reported by " + m[3] + ": " + m[4] + ". (stubllm echoes linter output; a real model explains and proposes a fix.)",
				})
			}
		}
		answer, _ := json.Marshal(map[string]any{
			"summary":    fmt.Sprintf("stubllm: %d linter diagnostic(s) on changed lines.", len(findings)),
			"risk_score": 10,
			"findings":   findings,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "stub", "object": "chat.completion", "model": req.Model,
			"choices": []map[string]any{{"index": 0, "finish_reason": "stop",
				"message": map[string]string{"role": "assistant", "content": string(answer)}}},
		})
	})

	srv := &http.Server{Addr: *addr, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("stubllm listening on http://%s/v1", *addr)
	log.Fatal(srv.ListenAndServe())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
