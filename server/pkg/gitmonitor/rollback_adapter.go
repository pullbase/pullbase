package gitmonitor

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// RollbackGitMonitorAdapter implements the rollback.GitMonitor interface for EnvironmentMonitor
type RollbackGitMonitorAdapter struct {
	monitor *EnvironmentMonitor
}

// NewRollbackGitMonitorAdapter creates a new rollback git monitor adapter for EnvironmentMonitor
func NewRollbackGitMonitorAdapter(monitor *EnvironmentMonitor) *RollbackGitMonitorAdapter {
	return &RollbackGitMonitorAdapter{
		monitor: monitor,
	}
}

// CommitExists checks if a commit exists in the repository
func (rgma *RollbackGitMonitorAdapter) CommitExists(ctx context.Context, repoURL, commit string) (bool, error) {
	normalizedRepoURL := normalizeRepoURL(repoURL)

	environments := rgma.monitor.GetAllEnvironments()
	for _, env := range environments {
		if normalizeRepoURL(env.RepoURL) == normalizedRepoURL {
			return rgma.monitor.CommitExists(ctx, env.ID, commit)
		}
	}
	return false, fmt.Errorf("environment not found for repository: %s", repoURL)
}

// CheckoutCommit checks out a specific commit in the repository
func (rgma *RollbackGitMonitorAdapter) CheckoutCommit(ctx context.Context, repoURL, commit string) error {
	normalizedRepoURL := normalizeRepoURL(repoURL)

	environments := rgma.monitor.GetAllEnvironments()
	for _, env := range environments {
		if normalizeRepoURL(env.RepoURL) == normalizedRepoURL {
			return rgma.monitor.CheckoutCommit(ctx, env.ID, commit)
		}
	}
	return fmt.Errorf("environment not found for repository: %s", repoURL)
}

// normalizeRepoURL normalizes repository URLs for consistent comparison
func normalizeRepoURL(repoURL string) string {
	// Remove .git suffix if present
	repoURL = strings.TrimSuffix(repoURL, ".git")

	if parsedURL, err := url.Parse(repoURL); err == nil && parsedURL.Host != "" {
		parsedURL.Path = strings.Trim(parsedURL.Path, "/")
		return parsedURL.String()
	}

	return repoURL
}
