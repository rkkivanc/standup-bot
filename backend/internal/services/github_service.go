package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const githubAPIBase = "https://api.github.com"

// Commit represents a single GitHub commit with the fields used by the standup bot.
type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

type githubCommitResponse struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

// GitHubService fetches commits from the GitHub REST API.
type GitHubService struct {
	httpClient *http.Client
}

// NewGitHubService creates a GitHubService with a sensible timeout.
func NewGitHubService() *GitHubService {
	return &GitHubService{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// FetchCommitsSince returns commits for the given repo made after the `since`
// timestamp (RFC3339 / ISO 8601). The token is used only in-memory and is
// never logged or persisted.
func (s *GitHubService) FetchCommitsSince(ctx context.Context, token, owner, repo, since string) ([]Commit, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/commits?since=%s", githubAPIBase, owner, repo, since)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build github request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	// Allow override for testing or CI environments.
	if base := os.Getenv("GITHUB_API_BASE"); base != "" {
		req.URL.Host = base
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// handled below
	case http.StatusUnauthorized:
		return nil, &GitHubError{Code: 401, Message: "invalid or expired GitHub token"}
	case http.StatusNotFound:
		return nil, &GitHubError{Code: 404, Message: fmt.Sprintf("repository %s/%s not found", owner, repo)}
	default:
		return nil, fmt.Errorf("github returned unexpected status %d", resp.StatusCode)
	}

	var raw []githubCommitResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode github response: %w", err)
	}

	commits := make([]Commit, 0, len(raw))
	for _, c := range raw {
		commits = append(commits, Commit{
			SHA:     c.SHA,
			Message: c.Commit.Message,
			Author:  c.Commit.Author.Name,
			Date:    c.Commit.Author.Date,
		})
	}
	return commits, nil
}

// GitHubError is a structured error returned for known GitHub API failures.
type GitHubError struct {
	Code    int
	Message string
}

func (e *GitHubError) Error() string {
	return fmt.Sprintf("github error %d: %s", e.Code, e.Message)
}
