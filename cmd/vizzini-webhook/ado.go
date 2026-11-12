package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// adoCommentEvent is the service hook event "Pull request commented on".
const adoCommentEvent = "ms.vss-code.git-pullrequest-comment-event"

// adoEvent is what the receiver needs from an ADO service hook delivery
// (resource version 2.0, "Resource details to send: All").
type adoEvent struct {
	EventType   string
	CommentID   int
	CommentType string
	Content     string
	Author      string
	PRID        int
	Status      string
	Repo        string // "dev.azure.com/ORG/PROJECT/REPO"
	CloneURL    string // "https://dev.azure.com/ORG/PROJECT/_git/REPO"
	Target      string // "main" (from refs/heads/main)
}

type adoPayload struct {
	EventType string `json:"eventType"`
	Resource  struct {
		Comment struct {
			ID          int    `json:"id"`
			Content     string `json:"content"`
			CommentType string `json:"commentType"`
			Author      struct {
				UniqueName  string `json:"uniqueName"`
				DisplayName string `json:"displayName"`
			} `json:"author"`
		} `json:"comment"`
		PullRequest struct {
			PullRequestID int    `json:"pullRequestId"`
			Status        string `json:"status"`
			TargetRefName string `json:"targetRefName"`
			Repository    struct {
				RemoteURL string `json:"remoteUrl"`
			} `json:"repository"`
		} `json:"pullRequest"`
	} `json:"resource"`
}

func parseADOEvent(body []byte) (*adoEvent, error) {
	var p adoPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("not JSON: %w", err)
	}
	if p.EventType == "" {
		return nil, errors.New("no eventType")
	}
	ev := &adoEvent{EventType: p.EventType}
	if p.EventType != adoCommentEvent {
		return ev, nil
	}
	c, pr := p.Resource.Comment, p.Resource.PullRequest
	if pr.PullRequestID <= 0 || pr.Repository.RemoteURL == "" || !strings.HasPrefix(pr.TargetRefName, "refs/heads/") {
		return nil, errors.New("pull request, repository or target branch missing: set the hook's resource details to send to All")
	}
	repo, err := normalizeRepo(pr.Repository.RemoteURL)
	if err != nil {
		return nil, err
	}
	ev.CommentID, ev.CommentType, ev.Content = c.ID, c.CommentType, c.Content
	ev.Author = c.Author.UniqueName
	if ev.Author == "" {
		ev.Author = c.Author.DisplayName
	}
	ev.PRID, ev.Status = pr.PullRequestID, pr.Status
	ev.Repo = repo
	parts := strings.Split(repo, "/")
	ev.CloneURL = fmt.Sprintf("https://%s/%s/%s/_git/%s", parts[0], parts[1], parts[2], parts[3])
	ev.Target = strings.TrimPrefix(pr.TargetRefName, "refs/heads/")
	if !safeBranch(ev.Target) {
		return nil, fmt.Errorf("target branch %q: not a plain branch name", ev.Target)
	}
	return ev, nil
}

// key is the pull request's attribution key, as the CI template forms it.
func (ev *adoEvent) key() string {
	if ev.Repo == "" {
		return ""
	}
	return fmt.Sprintf("ado:%s#%d", ev.Repo, ev.PRID)
}

func (ev *adoEvent) job() *job {
	return &job{Key: ev.key(), Repo: ev.Repo, CloneURL: ev.CloneURL, PRID: ev.PRID, Target: ev.Target,
		WebURL: fmt.Sprintf("%s/pullrequest/%d", ev.CloneURL, ev.PRID), Requester: ev.Author}
}

// normalizeRepo turns a repository given as a remote URL
// (https://ORG@dev.azure.com/ORG/PROJECT/_git/REPO) or a key
// (dev.azure.com/ORG/PROJECT/REPO) into the key. Only dev.azure.com is
// accepted: the receiver builds clone URLs from it.
func normalizeRepo(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("repository %q: %w", s, err)
	}
	if !strings.EqualFold(u.Host, "dev.azure.com") {
		return "", fmt.Errorf("repository host %q: only dev.azure.com is supported", u.Host)
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) == 4 && parts[2] == "_git" {
		parts = []string{parts[0], parts[1], parts[3]}
	}
	if len(parts) != 3 {
		return "", fmt.Errorf("repository %q: want dev.azure.com/ORG/PROJECT/REPO", s)
	}
	for _, p := range parts {
		if p == "" || !safeSegment.MatchString(p) {
			return "", fmt.Errorf("repository %q: unexpected path segment %q", s, p)
		}
	}
	return "dev.azure.com/" + strings.Join(parts, "/"), nil
}

// safeSegment admits ADO organization, project and repository names
// (letters, digits, '.', '_', '-', and %-escaped spaces) and nothing a
// shell or git could read as an option or a path.
var safeSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._%-]*$`)

// commandRE matches a /vizzini command on a line of its own: the same
// pattern as vizzini-bot's commandLineRE.
var commandRE = regexp.MustCompile(`(?im)^[ \t]*/vizzini[ \t]+(\S[^\r\n]*)$`)

func hasCommand(content string) bool { return commandRE.MatchString(content) }

// safeBranch admits a branch name that is safe inside a refspec.
func safeBranch(b string) bool {
	return branchRE.MatchString(b) && !strings.Contains(b, "..") && !strings.Contains(b, "//") &&
		!strings.HasSuffix(b, "/") && !strings.HasSuffix(b, ".lock")
}

var branchRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/-]*$`)
