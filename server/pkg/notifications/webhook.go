// Package notifications provides webhook notification services for PullBase events.
package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pullbase/pullbase/server/pkg/logging"
)

// EventType represents the type of notification event
type EventType string

const (
	EventDriftDetected     EventType = "drift_detected"
	EventApplyError        EventType = "apply_error"
	EventAgentDisconnected EventType = "agent_disconnected"
	EventApplySuccess      EventType = "apply_success"
	EventTest              EventType = "test"
)

// WebhookPayload represents the notification payload sent to webhook URLs
type WebhookPayload struct {
	Event           EventType `json:"event"`
	Timestamp       time.Time `json:"timestamp"`
	EnvironmentID   int64     `json:"environment_id"`
	EnvironmentName string    `json:"environment_name"`
	ServerID        string    `json:"server_id,omitempty"`
	ServerName      string    `json:"server_name,omitempty"`
	Details         string    `json:"details,omitempty"`
	CommitHash      string    `json:"commit_hash,omitempty"`
}

// Service handles sending webhook notifications
type Service struct {
	client     *http.Client
	maxRetries int
	baseDelay  time.Duration
	logger     *logging.Logger
}

// NewService creates a new notification service
func NewService() *Service {
	return &Service{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		maxRetries: 3,
		baseDelay:  1 * time.Second,
		logger:     logging.NewLogger(logging.Options{}),
	}
}

// NewServiceWithConfig creates a notification service with custom configuration
func NewServiceWithConfig(timeout time.Duration, maxRetries int, baseDelay time.Duration) *Service {
	return &Service{
		client: &http.Client{
			Timeout: timeout,
		},
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
		logger:     logging.NewLogger(logging.Options{}),
	}
}

// SendNotification sends a notification to the specified webhook URL with retry logic
func (s *Service) SendNotification(ctx context.Context, webhookURL string, payload WebhookPayload) error {
	if webhookURL == "" {
		return nil // No webhook configured, skip silently
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < s.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := s.baseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			s.logger.Info("webhook retry attempt",
				"attempt", attempt+1,
				"max_retries", s.maxRetries,
				"event", payload.Event,
				"url", webhookURL)
		}

		err = s.sendRequest(ctx, webhookURL, jsonPayload)
		if err == nil {
			s.logger.Info("webhook notification sent",
				"event", payload.Event,
				"url", webhookURL)
			return nil
		}
		lastErr = err
		s.logger.Warn("webhook attempt failed",
			"attempt", attempt+1,
			"event", payload.Event,
			"error", err)
	}

	return fmt.Errorf("webhook delivery failed after %d attempts: %w", s.maxRetries, lastErr)
}

// sendRequest sends a single HTTP request to the webhook URL
func (s *Service) sendRequest(ctx context.Context, webhookURL string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "PullBase-Webhook/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Accept 2xx status codes as success
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}

// SendAsync sends a notification asynchronously using the provided context for cancellation.
// A 30s timeout is applied on top of the caller context to bound retries.
func (s *Service) SendAsync(ctx context.Context, webhookURL string, payload WebhookPayload) {
	if webhookURL == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		if err := s.SendNotification(ctx, webhookURL, payload); err != nil {
			s.logger.Error("failed to send webhook notification",
				"event", payload.Event,
				"environment_id", payload.EnvironmentID,
				"error", err)
		}
	}()
}

// BuildDriftPayload creates a payload for drift detection events
func BuildDriftPayload(envID int64, envName, serverID, serverName, commitHash, details string) WebhookPayload {
	return WebhookPayload{
		Event:           EventDriftDetected,
		Timestamp:       time.Now().UTC(),
		EnvironmentID:   envID,
		EnvironmentName: envName,
		ServerID:        serverID,
		ServerName:      serverName,
		CommitHash:      commitHash,
		Details:         details,
	}
}

// BuildApplyErrorPayload creates a payload for apply error events
func BuildApplyErrorPayload(envID int64, envName, serverID, serverName, commitHash, errorMsg string) WebhookPayload {
	return WebhookPayload{
		Event:           EventApplyError,
		Timestamp:       time.Now().UTC(),
		EnvironmentID:   envID,
		EnvironmentName: envName,
		ServerID:        serverID,
		ServerName:      serverName,
		CommitHash:      commitHash,
		Details:         errorMsg,
	}
}

// BuildAgentDisconnectedPayload creates a payload for agent disconnect events
func BuildAgentDisconnectedPayload(envID int64, envName, serverID, serverName string, lastSeen time.Time) WebhookPayload {
	return WebhookPayload{
		Event:           EventAgentDisconnected,
		Timestamp:       time.Now().UTC(),
		EnvironmentID:   envID,
		EnvironmentName: envName,
		ServerID:        serverID,
		ServerName:      serverName,
		Details:         fmt.Sprintf("Agent last seen at %s", lastSeen.Format(time.RFC3339)),
	}
}

// BuildTestPayload creates a payload for testing webhook configuration
func BuildTestPayload(envID int64, envName string) WebhookPayload {
	return WebhookPayload{
		Event:           EventTest,
		Timestamp:       time.Now().UTC(),
		EnvironmentID:   envID,
		EnvironmentName: envName,
		Details:         "This is a test notification from PullBase",
	}
}
