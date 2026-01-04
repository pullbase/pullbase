package gitmonitor

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// GitHubProvider implements GitProvider for GitHub
type GitHubProvider struct {
	client *http.Client
}

// NewGitHubProvider creates a new GitHub provider
func NewGitHubProvider() *GitHubProvider {
	return &GitHubProvider{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// GitHubWebhookPayload represents GitHub webhook payload
type GitHubWebhookPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	HeadCommit struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"head_commit"`
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"commits"`
}

// GitHubAPIError represents GitHub API error response
type GitHubAPIError struct {
	Message string `json:"message"`
}

func (g *GitHubProvider) ValidateSignature(payload []byte, signature string, secret string) error {
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format")
	}

	expectedSignature := signature[7:] // Remove "sha256=" prefix

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	actualSignature := hex.EncodeToString(h.Sum(nil))

	if expectedSignature != actualSignature {
		return fmt.Errorf("signature validation failed")
	}

	return nil
}

func (g *GitHubProvider) ParseWebhook(payload []byte) (*WebhookEvent, error) {
	var ghPayload GitHubWebhookPayload
	if err := json.Unmarshal(payload, &ghPayload); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub webhook payload: %w", err)
	}

	// Extract branch name from ref (e.g., "refs/heads/main" -> "main")
	branch := strings.TrimPrefix(ghPayload.Ref, "refs/heads/")

	// Use CloneURL to match the database repo_url format
	repoURL := ghPayload.Repository.CloneURL
	if repoURL == "" {
		// Fallback to constructing from FullName if CloneURL is not available
		repoURL = fmt.Sprintf("https://github.com/%s.git", ghPayload.Repository.FullName)
	}

	event := &WebhookEvent{
		Provider:   ProviderGitHub,
		EventType:  "push",
		Repository: repoURL,
		Branch:     branch,
		CommitHash: ghPayload.HeadCommit.ID,
		CommitMsg:  ghPayload.HeadCommit.Message,
		Author:     ghPayload.HeadCommit.Author.Name,
		Timestamp:  ghPayload.HeadCommit.Timestamp,
		RawPayload: payload,
	}

	return event, nil
}

func (g *GitHubProvider) RegisterWebhook(ctx context.Context, repoURL, webhookURL, token string) (string, error) {
	// Parse repo URL to get owner/repo
	owner, repo, err := g.parseRepoURL(repoURL)
	if err != nil {
		return "", err
	}

	// Create GitHub API client with token
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := &http.Client{Transport: tc.Transport}

	// Prepare webhook payload
	webhookData := map[string]interface{}{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": map[string]string{
			"url":          webhookURL,
			"content_type": "json",
		},
	}

	payload, err := json.Marshal(webhookData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal webhook data: %w", err)
	}

	// Create webhook via GitHub API
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/hooks", owner, repo)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(payload)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to register webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var apiError GitHubAPIError
		if err := json.NewDecoder(resp.Body).Decode(&apiError); err != nil {
			return "", fmt.Errorf("webhook registration failed with status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("webhook registration failed: %s", apiError.Message)
	}

	// Parse response to get webhook ID
	var webhookResponse struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&webhookResponse); err != nil {
		return "", fmt.Errorf("failed to parse webhook response: %w", err)
	}

	return fmt.Sprintf("%d", webhookResponse.ID), nil
}

func (g *GitHubProvider) UnregisterWebhook(ctx context.Context, repoURL, webhookID, token string) error {
	owner, repo, err := g.parseRepoURL(repoURL)
	if err != nil {
		return err
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := &http.Client{Transport: tc.Transport}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/hooks/%s", owner, repo, webhookID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to unregister webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("webhook unregistration failed with status %d", resp.StatusCode)
	}

	return nil
}

func (g *GitHubProvider) GetCommitInfo(ctx context.Context, repoURL, commitHash, token string) (*CommitInfo, error) {
	owner, repo, err := g.parseRepoURL(repoURL)
	if err != nil {
		return nil, err
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := &http.Client{Transport: tc.Transport}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", owner, repo, commitHash)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiError GitHubAPIError
		if err := json.NewDecoder(resp.Body).Decode(&apiError); err != nil {
			return nil, fmt.Errorf("failed to get commit info with status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("failed to get commit info: %s", apiError.Message)
	}

	var commitResponse struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&commitResponse); err != nil {
		return nil, fmt.Errorf("failed to parse commit response: %w", err)
	}

	return &CommitInfo{
		Hash:      commitResponse.SHA,
		Message:   commitResponse.Commit.Message,
		Author:    commitResponse.Commit.Author.Name,
		Timestamp: commitResponse.Commit.Author.Date,
	}, nil
}

func (g *GitHubProvider) CheckoutCommit(ctx context.Context, repoURL, commitHash, token string) error {
	// For webhook-based monitoring, we don't actually checkout commits
	// This is handled by the rollback service when needed
	// We just verify the commit exists
	exists, err := g.CommitExists(ctx, repoURL, commitHash, token)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("commit %s does not exist", commitHash)
	}
	return nil
}

func (g *GitHubProvider) CommitExists(ctx context.Context, repoURL, commitHash, token string) (bool, error) {
	owner, repo, err := g.parseRepoURL(repoURL)
	if err != nil {
		return false, err
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := &http.Client{Transport: tc.Transport}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", owner, repo, commitHash)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check commit existence: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func (g *GitHubProvider) GetProvider() Provider {
	return ProviderGitHub
}

// parseRepoURL extracts owner and repo from GitHub URL
func (g *GitHubProvider) parseRepoURL(repoURL string) (owner, repo string, err error) {
	// Handle different GitHub URL formats
	// https://github.com/owner/repo.git
	// https://github.com/owner/repo
	// git@github.com:owner/repo.git

	originalURL := repoURL
	repoURL = strings.TrimSuffix(repoURL, ".git")

	if strings.Contains(repoURL, "git@github.com:") {
		parts := strings.Split(repoURL, "git@github.com:")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid GitHub SSH URL format")
		}
		ownerRepoParts := strings.Split(parts[1], "/")
		if len(ownerRepoParts) != 2 {
			return "", "", fmt.Errorf("invalid GitHub SSH URL format")
		}
		return ownerRepoParts[0], ownerRepoParts[1], nil
	}

	if strings.Contains(repoURL, "github.com") {
		parts := strings.Split(repoURL, "github.com/")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid GitHub URL format")
		}
		ownerRepo := strings.TrimPrefix(parts[1], "/")
		ownerRepoParts := strings.Split(ownerRepo, "/")
		if len(ownerRepoParts) != 2 {
			return "", "", fmt.Errorf("invalid GitHub URL format")
		}
		return ownerRepoParts[0], ownerRepoParts[1], nil
	}

	return "", "", fmt.Errorf("unsupported GitHub URL format: %s", originalURL)
}
