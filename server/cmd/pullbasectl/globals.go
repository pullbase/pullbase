package main

import (
	"context"
	"regexp"
	"time"

	"github.com/pullbase/pullbase/server/pkg/githubapp"
)

const bootstrapPasswordMinLength = 10

var bootstrapUsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,64}$`)

type installationTokenFetcher interface {
	GetInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error)
}

var (
	newHTTPClient = buildHTTPClient

	newGitHubAppClient = func(cfg githubapp.Config) (installationTokenFetcher, error) {
		return githubapp.NewClient(cfg)
	}
)
