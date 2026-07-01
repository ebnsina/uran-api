// Package git also holds the GitHub OAuth client used to connect an org's
// account and list its repositories for the "pick a repo" service flow.
package git

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	githubOAuthTokenURL = "https://github.com/login/oauth/access_token"
	githubAPIBase       = "https://api.github.com"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Repo is a repository the connected account can deploy from.
type Repo struct {
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// ExchangeCode swaps an OAuth authorization code for a user access token.
func ExchangeCode(ctx context.Context, clientID, clientSecret, code string) (string, error) {
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, githubOAuthTokenURL,
		strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github token exchange: %w", err)
	}
	defer resp.Body.Close()

	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if out.AccessToken == "" {
		if out.Error != "" {
			return "", fmt.Errorf("github: %s", out.Error)
		}
		return "", fmt.Errorf("github returned no access token")
	}
	return out.AccessToken, nil
}

// AccountLogin returns the login of the user the token belongs to.
func AccountLogin(ctx context.Context, token string) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if err := ghGet(ctx, token, "/user", &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

// ListRepos returns repositories the connected account can access, most
// recently pushed first (a few pages — enough for a picker).
func ListRepos(ctx context.Context, token string) ([]Repo, error) {
	out := []Repo{}
	for page := 1; page <= 3; page++ {
		var batch []Repo
		path := fmt.Sprintf(
			"/user/repos?per_page=100&sort=pushed&affiliation=owner,collaborator,organization_member&page=%d",
			page)
		if err := ghGet(ctx, token, path, &batch); err != nil {
			return nil, err
		}
		out = append(out, batch...)
		if len(batch) < 100 {
			break
		}
	}
	return out, nil
}

func ghGet(ctx context.Context, token, path string, dst any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("github GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
