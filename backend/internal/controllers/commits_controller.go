package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"workshop-backend/internal/services"
)
// CommitsController handles POST /api/commits requests.
type CommitsController struct {
	github *services.GitHubService
}

// NewCommitsController creates a CommitsController with the given GitHubService.
func NewCommitsController(github *services.GitHubService) *CommitsController {
	return &CommitsController{github: github}
}

// commitsRequest is the JSON body expected from the frontend.
type commitsRequest struct {
	GithubToken string `json:"github_token"`
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
}

// HandleCommits is the HTTP handler for POST /api/commits.
// It fetches the last 24 hours of commits for the specified repo.
// The GitHub token is used only in-memory and is never logged or persisted.
func (c *CommitsController) HandleCommits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req commitsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.GithubToken == "" || req.Owner == "" || req.Repo == "" {
		writeError(w, http.StatusBadRequest, "github_token, owner, and repo are required")
		return
	}

	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)

	commits, err := c.github.FetchCommitsSince(r.Context(), req.GithubToken, req.Owner, req.Repo, since)
	if err != nil {
		var ghErr *services.GitHubError
		if errors.As(err, &ghErr) {
			writeError(w, ghErr.Code, ghErr.Message)
			return
		}
		writeError(w, http.StatusBadGateway, "failed to fetch commits: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"commits": commits})
}
