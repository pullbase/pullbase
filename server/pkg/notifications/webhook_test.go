package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewService(t *testing.T) {
	svc := NewService()
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.maxRetries != 3 {
		t.Errorf("expected maxRetries=3, got %d", svc.maxRetries)
	}
	if svc.baseDelay != time.Second {
		t.Errorf("expected baseDelay=1s, got %v", svc.baseDelay)
	}
}

func TestNewServiceWithConfig(t *testing.T) {
	svc := NewServiceWithConfig(5*time.Second, 5, 500*time.Millisecond)
	if svc.client.Timeout != 5*time.Second {
		t.Errorf("expected timeout=5s, got %v", svc.client.Timeout)
	}
	if svc.maxRetries != 5 {
		t.Errorf("expected maxRetries=5, got %d", svc.maxRetries)
	}
	if svc.baseDelay != 500*time.Millisecond {
		t.Errorf("expected baseDelay=500ms, got %v", svc.baseDelay)
	}
}

func TestSendNotification_Success(t *testing.T) {
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("User-Agent") != "PullBase-Webhook/1.0" {
			t.Errorf("expected User-Agent PullBase-Webhook/1.0, got %s", r.Header.Get("User-Agent"))
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := NewServiceWithConfig(5*time.Second, 1, 10*time.Millisecond)
	payload := BuildTestPayload(1, "test-env")

	err := svc.SendNotification(context.Background(), server.URL, payload)
	if err != nil {
		t.Fatalf("SendNotification failed: %v", err)
	}

	if receivedPayload.Event != EventTest {
		t.Errorf("expected event=%s, got %s", EventTest, receivedPayload.Event)
	}
	if receivedPayload.EnvironmentID != 1 {
		t.Errorf("expected environmentID=1, got %d", receivedPayload.EnvironmentID)
	}
	if receivedPayload.EnvironmentName != "test-env" {
		t.Errorf("expected environmentName=test-env, got %s", receivedPayload.EnvironmentName)
	}
}

func TestSendNotification_EmptyURL(t *testing.T) {
	svc := NewService()
	payload := BuildTestPayload(1, "test-env")

	err := svc.SendNotification(context.Background(), "", payload)
	if err != nil {
		t.Errorf("expected no error for empty URL, got %v", err)
	}
}

func TestSendNotification_Retry(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := NewServiceWithConfig(5*time.Second, 3, 10*time.Millisecond)
	payload := BuildTestPayload(1, "test-env")

	err := svc.SendNotification(context.Background(), server.URL, payload)
	if err != nil {
		t.Fatalf("SendNotification failed after retries: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestSendNotification_AllRetriesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	svc := NewServiceWithConfig(5*time.Second, 2, 10*time.Millisecond)
	payload := BuildTestPayload(1, "test-env")

	err := svc.SendNotification(context.Background(), server.URL, payload)
	if err == nil {
		t.Fatal("expected error after all retries failed")
	}
}

func TestSendNotification_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := NewServiceWithConfig(5*time.Second, 3, 50*time.Millisecond)
	payload := BuildTestPayload(1, "test-env")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.SendNotification(ctx, server.URL, payload)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSendNotification_Accept2xxCodes(t *testing.T) {
	codes := []int{200, 201, 202, 204}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			svc := NewServiceWithConfig(5*time.Second, 1, 10*time.Millisecond)
			payload := BuildTestPayload(1, "test-env")

			err := svc.SendNotification(context.Background(), server.URL, payload)
			if err != nil {
				t.Fatalf("expected success for status %d, got error: %v", code, err)
			}
		})
	}
}

func TestBuildDriftPayload(t *testing.T) {
	payload := BuildDriftPayload(5, "prod", "server-01", "Web Server", "abc123", "drift details")

	if payload.Event != EventDriftDetected {
		t.Errorf("expected event=%s, got %s", EventDriftDetected, payload.Event)
	}
	if payload.EnvironmentID != 5 {
		t.Errorf("expected environmentID=5, got %d", payload.EnvironmentID)
	}
	if payload.EnvironmentName != "prod" {
		t.Errorf("expected environmentName=prod, got %s", payload.EnvironmentName)
	}
	if payload.ServerID != "server-01" {
		t.Errorf("expected serverID=server-01, got %s", payload.ServerID)
	}
	if payload.ServerName != "Web Server" {
		t.Errorf("expected serverName=Web Server, got %s", payload.ServerName)
	}
	if payload.CommitHash != "abc123" {
		t.Errorf("expected commitHash=abc123, got %s", payload.CommitHash)
	}
	if payload.Details != "drift details" {
		t.Errorf("expected details=drift details, got %s", payload.Details)
	}
}

func TestBuildApplyErrorPayload(t *testing.T) {
	payload := BuildApplyErrorPayload(5, "prod", "server-01", "Web Server", "abc123", "apply failed")

	if payload.Event != EventApplyError {
		t.Errorf("expected event=%s, got %s", EventApplyError, payload.Event)
	}
	if payload.Details != "apply failed" {
		t.Errorf("expected details=apply failed, got %s", payload.Details)
	}
}

func TestBuildAgentDisconnectedPayload(t *testing.T) {
	lastSeen := time.Now().Add(-10 * time.Minute)
	payload := BuildAgentDisconnectedPayload(5, "prod", "server-01", "Web Server", lastSeen)

	if payload.Event != EventAgentDisconnected {
		t.Errorf("expected event=%s, got %s", EventAgentDisconnected, payload.Event)
	}
	if payload.Details == "" {
		t.Error("expected details to contain last seen time")
	}
}

func TestBuildTestPayload(t *testing.T) {
	payload := BuildTestPayload(5, "prod")

	if payload.Event != EventTest {
		t.Errorf("expected event=%s, got %s", EventTest, payload.Event)
	}
	if payload.EnvironmentID != 5 {
		t.Errorf("expected environmentID=5, got %d", payload.EnvironmentID)
	}
	if payload.EnvironmentName != "prod" {
		t.Errorf("expected environmentName=prod, got %s", payload.EnvironmentName)
	}
}
