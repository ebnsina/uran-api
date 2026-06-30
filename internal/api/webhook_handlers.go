package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/ebnsina/uran-api/internal/git"
	"github.com/ebnsina/uran-api/internal/store"
)

// maxWebhookBody caps the webhook payload size we will read.
const maxWebhookBody = 5 << 20 // 5 MiB

type webhookResp struct {
	Matched int           `json:"matched"`
	Deploys []deployBrief `json:"deploys"`
}

type deployBrief struct {
	DeployID  int64  `json:"deploy_id"`
	ServiceID int64  `json:"service_id"`
	CommitSHA string `json:"commit_sha"`
}

// handleGitHubWebhook verifies the HMAC signature and dispatches by event type:
// "push" triggers production deploys; "pull_request" manages preview envs.
func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	if !git.VerifySignature(s.webhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	switch event := r.Header.Get("X-GitHub-Event"); event {
	case "push", "":
		s.handlePush(w, r, body)
	case "pull_request":
		s.handlePullRequest(w, r, body)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"ignored": event})
	}
}

// handlePush enqueues a deploy for every service whose repo + branch match.
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request, body []byte) {
	push, err := git.ParsePush(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid push payload")
		return
	}
	branch := push.Branch()
	if branch == "" {
		writeJSON(w, http.StatusOK, map[string]string{"ignored": "non-branch ref"})
		return
	}

	services, err := s.matchServices(r, push.Repository.CloneURL, push.Repository.HTMLURL, branch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not match services")
		return
	}

	resp := webhookResp{}
	for _, svc := range services {
		d, err := s.enqueueDeploy(r.Context(), svc.ID, push.After)
		if err != nil {
			s.log.Error("enqueue deploy from webhook failed", "service_id", svc.ID, "err", err)
			continue
		}
		resp.Deploys = append(resp.Deploys, deployBrief{DeployID: d.ID, ServiceID: svc.ID, CommitSHA: d.CommitSHA})
	}
	resp.Matched = len(resp.Deploys)
	s.log.Info("github push handled", "repo", push.Repository.FullName, "branch", branch, "matched", resp.Matched)
	writeJSON(w, http.StatusOK, resp)
}

// handlePullRequest builds (or tears down) a preview environment per service of
// the PR's repo, keyed by PR number.
func (s *Server) handlePullRequest(w http.ResponseWriter, r *http.Request, body []byte) {
	pr, err := git.ParsePullRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid pull_request payload")
		return
	}
	if !pr.ShouldDeploy() && !pr.ShouldTeardown() {
		writeJSON(w, http.StatusOK, map[string]string{"ignored": "pr action " + pr.Action})
		return
	}

	services, err := s.matchServicesAnyBranch(r, pr.Repository.CloneURL, pr.Repository.HTMLURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not match services")
		return
	}

	resp := webhookResp{}
	for _, svc := range services {
		if pr.ShouldTeardown() {
			if err := s.store.Notify(r.Context(), store.TeardownChannel, fmt.Sprintf("%d:%d", svc.ID, pr.Number)); err != nil {
				s.log.Error("notify teardown failed", "service_id", svc.ID, "err", err)
				continue
			}
			resp.Deploys = append(resp.Deploys, deployBrief{ServiceID: svc.ID})
			continue
		}
		d, err := s.enqueuePreview(r.Context(), svc.ID, pr.PullRequest.Head.SHA, pr.Number)
		if err != nil {
			s.log.Error("enqueue preview failed", "service_id", svc.ID, "err", err)
			continue
		}
		resp.Deploys = append(resp.Deploys, deployBrief{DeployID: d.ID, ServiceID: svc.ID, CommitSHA: d.CommitSHA})
	}
	resp.Matched = len(resp.Deploys)
	s.log.Info("github pull_request handled", "repo", pr.Repository.FullName, "action", pr.Action, "pr", pr.Number, "matched", resp.Matched)
	writeJSON(w, http.StatusOK, resp)
}

// matchServices looks up services matching either candidate URL for the branch,
// de-duplicating by service ID.
func (s *Server) matchServices(r *http.Request, cloneURL, htmlURL, branch string) ([]serviceRef, error) {
	return s.dedupeServices(func(url string) ([]store.Service, error) {
		return s.store.ServicesByRepo(r.Context(), url, branch)
	}, cloneURL, htmlURL)
}

// matchServicesAnyBranch looks up services for a repo regardless of branch (for
// pull requests), de-duplicating by service ID.
func (s *Server) matchServicesAnyBranch(r *http.Request, cloneURL, htmlURL string) ([]serviceRef, error) {
	return s.dedupeServices(func(url string) ([]store.Service, error) {
		return s.store.ServicesByRepoURL(r.Context(), url)
	}, cloneURL, htmlURL)
}

// dedupeServices runs lookup against each non-empty URL and de-duplicates the
// results by service ID.
func (s *Server) dedupeServices(lookup func(url string) ([]store.Service, error), urls ...string) ([]serviceRef, error) {
	seen := map[int64]bool{}
	var out []serviceRef
	for _, url := range urls {
		if url == "" {
			continue
		}
		matches, err := lookup(url)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			if !seen[m.ID] {
				seen[m.ID] = true
				out = append(out, serviceRef{ID: m.ID})
			}
		}
	}
	return out, nil
}

type serviceRef struct{ ID int64 }
