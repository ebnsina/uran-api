// Package git handles GitHub integration: webhook verification and parsing.
package git

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// PushEvent is the subset of a GitHub push payload we care about.
type PushEvent struct {
	Ref        string `json:"ref"` // e.g. "refs/heads/main"
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
}

// Branch returns the branch name from the push ref, or "" if it is not a
// branch ref (e.g. a tag push).
func (p PushEvent) Branch() string {
	if after, ok := strings.CutPrefix(p.Ref, "refs/heads/"); ok {
		return after
	}
	return ""
}

// VerifySignature reports whether the X-Hub-Signature-256 header matches an
// HMAC-SHA256 of body keyed by secret. The secret is required by config, so an
// empty or malformed header simply fails verification.
func VerifySignature(secret string, body []byte, sigHeader string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(sigHeader, prefix) {
		return false
	}
	want := strings.TrimPrefix(sigHeader, prefix)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	got := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(got), []byte(want))
}

// ParsePush decodes a push event body.
func ParsePush(body []byte) (PushEvent, error) {
	var p PushEvent
	if err := json.Unmarshal(body, &p); err != nil {
		return p, fmt.Errorf("parse push event: %w", err)
	}
	return p, nil
}

// PullRequestEvent is the subset of a GitHub pull_request payload we use.
type PullRequestEvent struct {
	Action      string `json:"action"` // opened | synchronize | reopened | closed | ...
	Number      int    `json:"number"`
	PullRequest struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
}

// ShouldDeploy reports whether the PR action warrants a (re)build of the
// preview environment.
func (p PullRequestEvent) ShouldDeploy() bool {
	switch p.Action {
	case "opened", "synchronize", "reopened":
		return true
	default:
		return false
	}
}

// ShouldTeardown reports whether the PR action should remove the preview.
func (p PullRequestEvent) ShouldTeardown() bool {
	return p.Action == "closed"
}

// ParsePullRequest decodes a pull_request event body.
func ParsePullRequest(body []byte) (PullRequestEvent, error) {
	var p PullRequestEvent
	if err := json.Unmarshal(body, &p); err != nil {
		return p, fmt.Errorf("parse pull_request event: %w", err)
	}
	return p, nil
}
