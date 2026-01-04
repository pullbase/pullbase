package git

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// AuthProvider returns Git transport credentials. Callers may return nil auth when not required.
type AuthProvider func() (transport.AuthMethod, error)

type Watcher struct {
	repo         *git.Repository
	path         string
	remoteURL    string
	branchName   string
	interval     time.Duration
	lastHash     string
	authProvider AuthProvider
}

func New(repoPath, remoteURL, branchName string, pollInterval time.Duration, authProvider AuthProvider) (*Watcher, error) {
	log.Printf("Initializing Git watcher for repo at %s (remote: %s, branch: %s)", repoPath, remoteURL, branchName)

	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		log.Printf("Error creating directory %s: %v", repoPath, err)
		return nil, err
	}

	watcher := &Watcher{
		path:         repoPath,
		remoteURL:    remoteURL,
		branchName:   branchName,
		interval:     pollInterval,
		authProvider: authProvider,
	}

	repo, err := git.PlainOpen(repoPath)
	if err == git.ErrRepositoryNotExists {
		log.Printf("Repository not found at %s, cloning from %s (branch: %s)", repoPath, remoteURL, branchName)

		cloneOpts := &git.CloneOptions{
			URL:           remoteURL,
			ReferenceName: plumbing.NewBranchReferenceName(branchName),
			SingleBranch:  false,
		}

		if auth, authErr := watcher.getAuth(); authErr != nil {
			return nil, fmt.Errorf("failed to obtain git credentials: %w", authErr)
		} else if auth != nil {
			cloneOpts.Auth = auth
		}

		repo, err = git.PlainClone(repoPath, false, cloneOpts)
		if err != nil {
			log.Printf("Error cloning repository: %v", err)
			return nil, fmt.Errorf("failed to clone repository %s: %w", remoteURL, err)
		}
	} else if err != nil {
		log.Printf("Error opening repository: %v", err)
		return nil, err
	}

	log.Printf("Successfully initialized Git watcher")
	watcher.repo = repo
	return watcher, nil
}

// CheckoutCommit checks out a specific commit hash in the repository.
func (w *Watcher) CheckoutCommit(commitHash string) error {
	log.Printf("Checking out specific commit: %s", commitHash)

	log.Printf("Fetching all objects to ensure commit %s is available...", commitHash)
	fetchOpts := &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []gitconfig.RefSpec{gitconfig.RefSpec("+refs/heads/*:refs/remotes/origin/*")},
		Force:      true,
	}

	if err := w.fetch(fetchOpts); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		log.Printf("Warning: fetch operation before checkout returned: %v", err)
	}

	worktree, err := w.repo.Worktree()
	if err != nil {
		log.Printf("Error getting worktree: %v", err)
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	hash := plumbing.NewHash(commitHash)

	_, err = w.repo.CommitObject(hash)
	if err != nil {
		log.Printf("Commit %s not found locally, attempting targeted fetch", commitHash)
		fetchSpecific := &git.FetchOptions{
			RemoteName: "origin",
			RefSpecs:   []gitconfig.RefSpec{gitconfig.RefSpec(fmt.Sprintf("+%s:%s", commitHash, commitHash))},
		}
		if fetchErr := w.fetch(fetchSpecific); fetchErr != nil && !errors.Is(fetchErr, git.NoErrAlreadyUpToDate) {
			log.Printf("Error fetching specific commit %s: %v", commitHash, fetchErr)
		}

		if _, verifyErr := w.repo.CommitObject(hash); verifyErr != nil {
			return fmt.Errorf("commit %s not found in repository even after fetch attempts: %w", commitHash, verifyErr)
		}
	}

	log.Printf("Resetting to commit %s...", commitHash)
	if err := worktree.Reset(&git.ResetOptions{
		Commit: hash,
		Mode:   git.HardReset,
	}); err != nil {
		log.Printf("Error resetting worktree to commit %s: %v", commitHash, err)
		return fmt.Errorf("failed to reset to commit %s: %w", commitHash, err)
	}

	w.lastHash = commitHash
	log.Printf("Successfully checked out commit: %s", commitHash)
	return nil
}

// GetCurrentCommit returns the current commit hash of the HEAD
func (w *Watcher) GetCurrentCommit() (string, error) {
	ref, err := w.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD reference: %w", err)
	}

	return ref.Hash().String(), nil
}

func (w *Watcher) fetch(opts *git.FetchOptions) error {
	if auth, err := w.getAuth(); err != nil {
		return err
	} else if auth != nil {
		opts.Auth = auth
	}
	return w.repo.Fetch(opts)
}

func (w *Watcher) getAuth() (transport.AuthMethod, error) {
	if w.authProvider == nil {
		return nil, nil
	}
	return w.authProvider()
}
