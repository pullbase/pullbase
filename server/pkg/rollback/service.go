package rollback

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pullbase/pullbase/server/pkg/models"
)

// Repository defines the interface for environment and rollback DB operations
type Repository interface {
	GetEnvironment(ctx context.Context, id int64) (*models.Environment, error)
	CreateRollbackEvent(ctx context.Context, event *models.RollbackEvent) error
	UpdateRollbackEventStatus(ctx context.Context, id int64, status string, errorMsg *string) error
	GetRollbackEvent(ctx context.Context, id int64) (*models.RollbackEvent, error)
	ListRollbackEvents(ctx context.Context, environmentID int64, limit, offset int) ([]*models.RollbackEvent, error)
	GetEnvironmentCommitHistory(ctx context.Context, environmentID int64, limit int) ([]*models.CommitInfo, error)
	UpdateEnvironmentCommit(ctx context.Context, environmentID int64, commit string) error
	CreateEvent(ctx context.Context, event *models.Event) error
}

// GitMonitor defines the interface for git operations needed for rollback
type GitMonitor interface {
	CommitExists(ctx context.Context, repoURL, commit string) (bool, error)
	CheckoutCommit(ctx context.Context, repoURL, commit string) error
}

// RollbackService defines the interface for rollback operations used by other packages
type RollbackService interface {
	InitiateRollback(ctx context.Context, req *RollbackRequest) (*RollbackResponse, error)
}

type Service struct {
	repo       Repository
	gitMonitor GitMonitor
	logger     *slog.Logger
}

func NewService(repo Repository, gitMonitor GitMonitor, logger *slog.Logger) *Service {
	return &Service{
		repo:       repo,
		gitMonitor: gitMonitor,
		logger:     logger,
	}
}

type RollbackRequest struct {
	EnvironmentID int64  `json:"environment_id"`
	ToCommit      string `json:"to_commit"`
	Reason        string `json:"reason"`
	InitiatedBy   string `json:"initiated_by"`
}

type RollbackResponse struct {
	RollbackID int64  `json:"rollback_id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

func (s *Service) InitiateRollback(ctx context.Context, req *RollbackRequest) (*RollbackResponse, error) {
	s.logger.Info("Initiating rollback",
		"environment_id", req.EnvironmentID,
		"to_commit", req.ToCommit,
		"initiated_by", req.InitiatedBy)

	env, err := s.repo.GetEnvironment(ctx, req.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	if env == nil {
		return nil, fmt.Errorf("environment not found")
	}

	fromCommit := ""
	if env.DeployedCommit != nil {
		fromCommit = *env.DeployedCommit
	}
	if req.ToCommit == fromCommit {
		return nil, fmt.Errorf("environment is already at commit %s", req.ToCommit)
	}

	exists, err := s.gitMonitor.CommitExists(ctx, env.RepoURL, req.ToCommit)
	if err != nil {
		return nil, fmt.Errorf("failed to validate commit: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("commit %s does not exist in repository", req.ToCommit)
	}

	rollbackEvent := &models.RollbackEvent{
		EnvironmentID: req.EnvironmentID,
		FromCommit:    fromCommit,
		ToCommit:      req.ToCommit,
		InitiatedBy:   req.InitiatedBy,
		Status:        "pending",
		Reason:        req.Reason,
		CreatedAt:     time.Now(),
	}

	err = s.repo.CreateRollbackEvent(ctx, rollbackEvent)
	if err != nil {
		return nil, fmt.Errorf("failed to create rollback event: %w", err)
	}

	go func(parent context.Context, event *models.RollbackEvent, environment *models.Environment) {
		timeout := 2 * time.Minute
		if deadline, ok := parent.Deadline(); ok {
			if remaining := time.Until(deadline); remaining < timeout {
				timeout = remaining
			}
		}

		execCtx, cancel := backgroundWithTimeout(timeout)
		defer cancel()

		s.executeRollback(execCtx, event, environment)
	}(ctx, rollbackEvent, env)

	return &RollbackResponse{
		RollbackID: rollbackEvent.ID,
		Status:     "pending",
		Message:    "Rollback initiated successfully",
	}, nil
}

func (s *Service) executeRollback(ctx context.Context, event *models.RollbackEvent, env *models.Environment) {
	err := s.repo.UpdateRollbackEventStatus(ctx, event.ID, "in_progress", nil)
	if err != nil {
		s.logger.Error("Failed to update rollback status to in_progress", "error", err)
		return
	}

	s.logger.Info("Executing rollback",
		"rollback_id", event.ID,
		"from_commit", event.FromCommit,
		"to_commit", event.ToCommit)

	// Checkout the target commit
	err = s.gitMonitor.CheckoutCommit(ctx, env.RepoURL, event.ToCommit)
	if err != nil {
		s.finalizeRollback(ctx, event.ID, "failed", fmt.Sprintf("Failed to checkout commit: %v", err))
		return
	}

	// Update the environment's deployed commit
	err = s.repo.UpdateEnvironmentCommit(ctx, event.EnvironmentID, event.ToCommit)
	if err != nil {
		s.finalizeRollback(ctx, event.ID, "failed", fmt.Sprintf("Failed to update environment commit: %v", err))
		return
	}

	logEvent := &models.Event{
		EnvironmentID: &event.EnvironmentID,
		EventType:     "rollback_completed",
		Message:       fmt.Sprintf("Rollback completed from %s to %s. Reason: %s", event.FromCommit, event.ToCommit, event.Reason),
		Timestamp:     time.Now(),
	}

	err = s.repo.CreateEvent(ctx, logEvent)
	if err != nil {
		s.logger.Error("Failed to create rollback event log", "error", err)
	}

	s.finalizeRollback(ctx, event.ID, "completed", "")
}

func (s *Service) finalizeRollback(ctx context.Context, rollbackID int64, status string, errorMsg string) {
	var errorPtr *string
	if errorMsg != "" {
		errorPtr = &errorMsg
	}

	err := s.repo.UpdateRollbackEventStatus(ctx, rollbackID, status, errorPtr)
	if err != nil {
		s.logger.Error("Failed to finalize rollback status", "error", err)
	}

	if status == "completed" {
		s.logger.Info("Rollback completed successfully", "rollback_id", rollbackID)
	} else {
		s.logger.Error("Rollback failed", "rollback_id", rollbackID, "error", errorMsg)
	}
}

func (s *Service) GetRollbackStatus(ctx context.Context, rollbackID int64) (*models.RollbackEvent, error) {
	return s.repo.GetRollbackEvent(ctx, rollbackID)
}

func (s *Service) ListRollbacks(ctx context.Context, environmentID int64, limit, offset int) ([]*models.RollbackEvent, error) {
	return s.repo.ListRollbackEvents(ctx, environmentID, limit, offset)
}

func (s *Service) GetAvailableCommits(ctx context.Context, environmentID int64, limit int) ([]*models.CommitInfo, error) {
	return s.repo.GetEnvironmentCommitHistory(ctx, environmentID, limit)
}

func (s *Service) ValidateRollbackRequest(ctx context.Context, req *RollbackRequest) error {
	if req.EnvironmentID <= 0 {
		return fmt.Errorf("invalid environment ID")
	}

	if req.ToCommit == "" {
		return fmt.Errorf("target commit cannot be empty")
	}

	if req.InitiatedBy == "" {
		return fmt.Errorf("initiated_by cannot be empty")
	}

	if len(req.ToCommit) < 7 || len(req.ToCommit) > 40 {
		return fmt.Errorf("invalid commit hash format")
	}

	return nil
}
