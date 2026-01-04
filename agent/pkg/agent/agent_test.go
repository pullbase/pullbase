package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/pullbase/pullbase/agent/pkg/config"
)

// waitForCondition polls a condition function until it returns true or timeout is reached
func waitForCondition(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for condition: %s (waited %v)", message, timeout)
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

// MockConfigManager implements config.ConfigManager for testing
type MockConfigManager struct {
	mock.Mock
}

func (m *MockConfigManager) Load(filePath string) (*config.ServerConfig, error) {
	args := m.Called(filePath)
	var cfg *config.ServerConfig
	if args.Get(0) != nil {
		cfg = args.Get(0).(*config.ServerConfig)
	}
	return cfg, args.Error(1)
}

// Update Apply signature to include PackageManager
func (m *MockConfigManager) Apply(cfg *config.ServerConfig, svcManager config.ServiceManager, pkgManager config.PackageManager) error {
	args := m.Called(cfg, svcManager, pkgManager)
	return args.Error(0)
}

// Update CheckDrift signature to include PackageManager
func (m *MockConfigManager) CheckDrift(cfg *config.ServerConfig, svcManager config.ServiceManager, pkgManager config.PackageManager) ([]string, error) {
	args := m.Called(cfg, svcManager, pkgManager)
	var drifts []string
	if args.Get(0) != nil {
		drifts = args.Get(0).([]string)
	}
	return drifts, args.Error(1)
}

// Mock for config.PackageManager
type MockPackageManager struct {
	mock.Mock
}

func (m *MockPackageManager) Install(name string, version string) error {
	args := m.Called(name, version)
	return args.Error(0)
}

func (m *MockPackageManager) Remove(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *MockPackageManager) IsInstalled(name string) (bool, error) {
	args := m.Called(name)
	return args.Bool(0), args.Error(1)
}

// Mock for config.ServiceManager
type MockServiceManager struct {
	mock.Mock
}

func (m *MockServiceManager) Start(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *MockServiceManager) Stop(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *MockServiceManager) Enable(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *MockServiceManager) Disable(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *MockServiceManager) IsActive(name string) (bool, error) {
	args := m.Called(name)
	return args.Bool(0), args.Error(1)
}

func (m *MockServiceManager) IsEnabled(name string) (bool, error) {
	args := m.Called(name)
	return args.Bool(0), args.Error(1)
}

func (m *MockServiceManager) ReloadOrRestart(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func TestLogin(t *testing.T) {

	correctUser := "testuser"
	correctPass := "testpass"
	expectedToken := "test-jwt-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check path and method
		assert.Equal(t, "/api/v1/auth/login", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Decode request
		var reqBody map[string]string
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		// Check credentials
		username, uOk := reqBody["username"]
		password, pOk := reqBody["password"]
		serverID, sOk := reqBody["server_id"]

		if !uOk || !pOk {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Verify server_id is present (as agent sends it)
		if !sOk {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// For testing, accept any server_id value
		_ = serverID

		if username == correctUser && password == correctPass {
			// Send success response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := LoginResponse{AccessToken: expectedToken}
			json.NewEncoder(w).Encode(resp)
		} else {
			// Send unauthorized response
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer server.Close()

	t.Run("Successful Login", func(t *testing.T) {

		a := &Agent{}

		a.CentralURL = server.URL
		a.agentUsername = correctUser
		a.agentPassword = correctPass
		a.httpClient = server.Client()

		err := a.login()
		require.NoError(t, err)
		assert.Equal(t, expectedToken, a.jwtToken)
	})

	t.Run("Failed Login - Incorrect Password", func(t *testing.T) {
		// Remove t.Parallel() since this shares the main server

		a := &Agent{}
		a.CentralURL = server.URL
		a.httpClient = server.Client()
		a.agentUsername = "wronguser"
		a.agentPassword = "wrongpass"

		err := a.login()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status code 401")
		assert.Equal(t, "", a.jwtToken)
	})

	t.Run("Failed Login - Server Error", func(t *testing.T) {

		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer errorServer.Close()

		a := &Agent{
			CentralURL:    errorServer.URL,
			httpClient:    errorServer.Client(),
			agentUsername: correctUser,
			agentPassword: correctPass,
		}

		err := a.login()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status code 500")
		assert.Equal(t, "", a.jwtToken)
	})

	t.Run("Agent Login with Server ID", func(t *testing.T) {
		// Remove t.Parallel() to maintain test ordering

		testServerID := "test-server-123"
		expectedToken := "env-scoped-token"

		serverIDServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check path and method
			assert.Equal(t, "/api/v1/auth/login", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Decode request
			var reqBody map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&reqBody)
			require.NoError(t, err)

			// Check credentials and server_id
			username, uOk := reqBody["username"]
			password, pOk := reqBody["password"]
			serverID, sOk := reqBody["server_id"]

			if !uOk || !pOk || !sOk {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Verify server_id is sent correctly
			assert.Equal(t, testServerID, serverID, "Agent should send server_id")

			if username == correctUser && password == correctPass {
				// Send success response with environment-scoped token
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := LoginResponse{AccessToken: expectedToken}
				json.NewEncoder(w).Encode(resp)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
		}))
		defer serverIDServer.Close()

		a := &Agent{
			ServerID:      testServerID,
			CentralURL:    serverIDServer.URL,
			agentUsername: correctUser,
			agentPassword: correctPass,
			httpClient:    serverIDServer.Client(),
		}

		err := a.login()
		require.NoError(t, err, "Login should succeed with server_id")
		assert.Equal(t, expectedToken, a.jwtToken, "Should receive environment-scoped token")
	})
}

func TestReportStatus(t *testing.T) {
	t.Parallel()

	testAgentID := "test-agent-123"
	testJwtToken := "test-jwt-token"
	newTestJwtToken := "new-test-jwt-token"
	testCommitHash := "abcdef123456"
	testStatus := "Applied"
	testIsDrifted := false
	testErrMsg := ""

	dummyRepoPath := t.TempDir()
	err := exec.Command("git", "init", dummyRepoPath).Run()
	require.NoError(t, err, "Failed to run 'git init' in dummy repo path. Is git installed?")

	// --- Test: Standard Report ---
	t.Run("Standard Report", func(t *testing.T) {
		t.Parallel()

		handlerCalled := false
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			require.Equal(t, http.MethodPut, r.Method, "Expected PUT request")
			expectedPath := "/api/v1/agent/status"
			require.Equal(t, expectedPath, r.URL.Path, "Unexpected request path")
			authHeader := r.Header.Get("Authorization")
			expectedAuthHeader := "Bearer " + testJwtToken
			require.Equal(t, expectedAuthHeader, authHeader, "Incorrect or missing Authorization header")

			w.WriteHeader(http.StatusOK)
		}))
		defer mockServer.Close()

		testAgent, err := New(testAgentID, mockServer.URL, dummyRepoPath, "", "", "", "user", "pass", "", 1*time.Second, "", false, "test")
		require.NoError(t, err, "Failed to create agent")
		testAgent.jwtToken = testJwtToken
		testAgent.ExportDoReportStatus(testCommitHash, testStatus, testIsDrifted, testErrMsg)
		assert.True(t, handlerCalled, "Mock server handler was not called")
	})

	t.Run("ReportWithError", func(t *testing.T) {
		t.Parallel()

		handlerCalled := false
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			require.Equal(t, http.MethodPut, r.Method)
			bodyBytes, _ := io.ReadAll(r.Body)
			var payload statusUpdatePayload
			_ = json.Unmarshal(bodyBytes, &payload)
			_ = r.Body.Close()
			assert.Equal(t, "Error", payload.Status)
			assert.True(t, payload.IsDrifted)
			assert.NotNil(t, payload.ErrorMessage)
			assert.Equal(t, "Something went wrong", *payload.ErrorMessage)
			w.WriteHeader(http.StatusOK)
		}))
		defer mockServer.Close()

		testAgent, err := New(testAgentID, mockServer.URL, dummyRepoPath, "", "", "", "user", "pass", "", 1*time.Second, "", false, "test")
		require.NoError(t, err, "Failed to create agent")
		testAgent.jwtToken = testJwtToken
		testAgent.ExportDoReportStatus(testCommitHash, "Error", true, "Something went wrong")
		assert.True(t, handlerCalled, "Mock server handler was not called in error case")
	})

	// --- Test: Token Renewal on 401 ---
	t.Run("Token Renewal", func(t *testing.T) {
		callCount := 0
		loginAttempted := false
		retrySuccessful := false
		loginUsername := "agent-user-for-renewal"
		loginPassword := "agent-pass-for-renewal"

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			fmt.Printf("Mock Server: Received call %d: %s %s\n", callCount, r.Method, r.URL.Path)

			switch r.URL.Path {
			case "/api/v1/auth/login":
				fmt.Println("Mock Server: Handling login request")
				// This should be call #2
				require.Equal(t, 2, callCount, "Login request was not the second call")
				loginAttempted = true
				var reqBody map[string]string
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.Equal(t, loginUsername, reqBody["username"])
				assert.Equal(t, loginPassword, reqBody["password"])

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := LoginResponse{AccessToken: newTestJwtToken}
				json.NewEncoder(w).Encode(resp)
				fmt.Println("Mock Server: Sent new token")

			case "/api/v1/agent/status":
				fmt.Printf("Mock Server: Handling status request #%d\n", callCount)
				authHeader := r.Header.Get("Authorization")
				switch callCount {
				case 1:
					fmt.Println("Mock Server: Expecting initial token, returning 401")
					expectedAuthHeader := "Bearer " + testJwtToken
					assert.Equal(t, expectedAuthHeader, authHeader)
					w.WriteHeader(http.StatusUnauthorized)
				case 3:
					fmt.Println("Mock Server: Expecting new token, returning 200")
					expectedAuthHeader := "Bearer " + newTestJwtToken
					assert.Equal(t, expectedAuthHeader, authHeader)
					retrySuccessful = true
					w.WriteHeader(http.StatusOK)
				default:
					t.Fatalf("Unexpected call count (%d) to status endpoint", callCount)
				}
			default:
				t.Fatalf("Unexpected request path: %s", r.URL.Path)
			}
		}))
		defer mockServer.Close()

		testAgent, err := New(testAgentID, mockServer.URL, dummyRepoPath, "", "", "", loginUsername, loginPassword, "", 1*time.Second, "", false, "test")
		require.NoError(t, err, "Failed to create agent for renewal test")

		testAgent.jwtToken = testJwtToken

		// Trigger the status report which should initiate the renewal flow
		testAgent.ExportDoReportStatus(testCommitHash, testStatus, testIsDrifted, testErrMsg)

		// Assert the flow occurred as expected
		assert.Equal(t, 3, callCount, "Expected exactly 3 calls to the mock server (status[401]->login->status[200])")
		assert.True(t, loginAttempted, "Expected agent to attempt re-login")
		assert.True(t, retrySuccessful, "Expected status report retry to be successful with new token")
		assert.Equal(t, newTestJwtToken, testAgent.ExportGetJwtToken(), "Agent should have stored the new token")
	})
}

func TestReconcileState(t *testing.T) {
	t.Parallel()

	dummyRepoPath := t.TempDir()
	cmd := exec.Command("git", "init", dummyRepoPath)
	err := cmd.Run()
	require.NoError(t, err, "Failed to run 'git init' in dummy repo path. Is git installed?")

	relativeConfigPath := "config.yaml"
	expectedConfigLoadPath := filepath.Join(dummyRepoPath, relativeConfigPath)

	// Test case: Successful Apply
	t.Run("Successful Apply", func(t *testing.T) {
		t.Parallel()

		testAgent, err := New(
			"test-reconcile", "", dummyRepoPath, "http://dummy.git", "main",
			relativeConfigPath, "", "", "", 1*time.Second, "", false, "test",
		)
		require.NoError(t, err, "Failed to create agent")

		mockMgr := new(MockConfigManager)
		testAgent.configManager = mockMgr

		dummyConfig := &config.ServerConfig{}
		mockMgr.On("Load", expectedConfigLoadPath).Return(dummyConfig, nil).Once()
		mockMgr.On("Apply", dummyConfig, mock.Anything, mock.Anything).Return(nil).Once()

		err = testAgent.reconcileState()
		require.NoError(t, err)
		mockMgr.AssertExpectations(t)
	})

	t.Run("Load Fails", func(t *testing.T) {
		t.Parallel()

		testAgent, err := New(
			"test-reconcile", "", dummyRepoPath, "http://dummy.git", "main",
			relativeConfigPath, "", "", "", 1*time.Second, "", false, "test",
		)
		require.NoError(t, err, "Failed to create agent")

		mockMgr := new(MockConfigManager)
		testAgent.configManager = mockMgr

		loadErr := errors.New("failed to load config")
		mockMgr.On("Load", expectedConfigLoadPath).Return(nil, loadErr).Once()

		err = testAgent.reconcileState()
		require.Error(t, err)
		assert.ErrorIs(t, err, loadErr)

		mockMgr.AssertNotCalled(t, "Apply", mock.Anything, mock.Anything, mock.Anything)
		mockMgr.AssertExpectations(t)
	})

	t.Run("Apply Fails", func(t *testing.T) {
		t.Parallel()

		testAgent, err := New(
			"test-reconcile", "", dummyRepoPath, "http://dummy.git", "main",
			relativeConfigPath, "", "", "", 1*time.Second, "", false, "test",
		)
		require.NoError(t, err, "Failed to create agent")

		mockMgr := new(MockConfigManager)
		testAgent.configManager = mockMgr

		dummyConfig := &config.ServerConfig{}
		applyErr := errors.New("failed to apply config")
		mockMgr.On("Load", expectedConfigLoadPath).Return(dummyConfig, nil).Once()
		mockMgr.On("Apply", dummyConfig, mock.Anything, mock.Anything).Return(applyErr).Once()

		err = testAgent.reconcileState()
		require.Error(t, err)
		assert.ErrorIs(t, err, applyErr)
		mockMgr.AssertExpectations(t)
	})
}

func TestCheckAndReportDrift(t *testing.T) {
	t.Parallel()

	dummyRepoPath := t.TempDir()
	cmd := exec.Command("git", "init", dummyRepoPath)
	err := cmd.Run()
	require.NoError(t, err, "Failed to run 'git init' in dummy repo path. Is git installed?")

	relativeConfigPath := "config.yaml"
	expectedConfigLoadPath := filepath.Join(dummyRepoPath, relativeConfigPath)

	var reportedPayload statusUpdatePayload
	handlerCalled := false
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &reportedPayload)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	testAgent, err := New(
		"test-drift", mockServer.URL, dummyRepoPath, "http://dummy.git", "main",
		relativeConfigPath, "", "", "", 1*time.Second, "", false, "test",
	)
	require.NoError(t, err)
	testAgent.jwtToken = "fake-token"

	t.Run("No Drift", func(t *testing.T) {
		mockMgr := new(MockConfigManager)
		testAgent.configManager = mockMgr
		testAgent.autoReconcile = false

		handlerCalled = false
		reportedPayload = statusUpdatePayload{}
		dummyConfig := &config.ServerConfig{ /* ... */ }
		mockMgr.On("Load", expectedConfigLoadPath).Return(dummyConfig, nil).Once()
		mockMgr.On("CheckDrift", dummyConfig, mock.Anything, mock.Anything).Return([]string{}, nil).Once()

		err := testAgent.checkAndReportDrift()
		require.NoError(t, err)
		assert.True(t, handlerCalled, "Status report handler not called")
		assert.False(t, reportedPayload.IsDrifted, "Reported payload should indicate no drift")
		assert.Equal(t, "Applied", reportedPayload.Status)
		assert.Empty(t, reportedPayload.ErrorMessage)
		mockMgr.AssertExpectations(t)
	})

	t.Run("Drift Detected (Auto-Reconcile Disabled)", func(t *testing.T) {
		mockMgr := new(MockConfigManager)
		testAgent.configManager = mockMgr
		testAgent.autoReconcile = false

		handlerCalled = false
		reportedPayload = statusUpdatePayload{}
		dummyConfig := &config.ServerConfig{ /* ... */ }
		driftMessages := []string{"Package drift", "File drift"}
		mockMgr.On("Load", expectedConfigLoadPath).Return(dummyConfig, nil).Once()
		mockMgr.On("CheckDrift", dummyConfig, mock.Anything, mock.Anything).Return(driftMessages, nil).Once()

		err := testAgent.checkAndReportDrift()
		require.NoError(t, err)
		assert.True(t, handlerCalled, "Status report handler not called")
		assert.True(t, reportedPayload.IsDrifted, "Reported payload should indicate drift")
		assert.Equal(t, "Drift Detected", reportedPayload.Status)
		assert.NotNil(t, reportedPayload.ErrorMessage, "Error message should not be nil")
		assert.Contains(t, *reportedPayload.ErrorMessage, "Package drift")
		assert.Contains(t, *reportedPayload.ErrorMessage, "File drift")
		mockMgr.AssertExpectations(t)
	})

	t.Run("Drift Detected with Auto-Reconcile Success", func(t *testing.T) {
		mockMgr := new(MockConfigManager)
		testAgent.configManager = mockMgr
		testAgent.autoReconcile = true

		var reportedPayloads []statusUpdatePayload
		handlerCallCount := 0
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCallCount++
			bodyBytes, _ := io.ReadAll(r.Body)
			var payload statusUpdatePayload
			_ = json.Unmarshal(bodyBytes, &payload)
			reportedPayloads = append(reportedPayloads, payload)
			_ = r.Body.Close()
			w.WriteHeader(http.StatusOK)
		}))
		defer mockServer.Close()
		testAgent.CentralURL = mockServer.URL

		dummyConfig := &config.ServerConfig{ /* ... */ }
		driftMessages := []string{"Package nginx should be installed but is absent", "Service httpd desired running state 'running', actual state is false"}
		mockMgr.On("Load", expectedConfigLoadPath).Return(dummyConfig, nil).Times(2) // Called twice: once for drift check, once for reconcile
		mockMgr.On("CheckDrift", dummyConfig, mock.Anything, mock.Anything).Return(driftMessages, nil).Once()
		mockMgr.On("Apply", dummyConfig, mock.Anything, mock.Anything).Return(nil).Once()

		err := testAgent.checkAndReportDrift()
		require.NoError(t, err)

		// Wait for both status updates to be received
		waitForCondition(t, func() bool {
			return handlerCallCount == 2 && len(reportedPayloads) == 2
		}, 2*time.Second, "receiving 2 status updates")

		assert.Equal(t, 2, handlerCallCount, "Should have received exactly 2 status updates")
		require.Len(t, reportedPayloads, 2, "Should have captured 2 status updates")

		firstUpdate := reportedPayloads[0]
		assert.True(t, firstUpdate.IsDrifted, "First update should indicate drift")
		assert.Equal(t, "Drift Detected", firstUpdate.Status, "First update should be 'Drift Detected'")
		assert.NotNil(t, firstUpdate.ErrorMessage, "First update should contain drift details")
		assert.Contains(t, *firstUpdate.ErrorMessage, "Package nginx should be installed but is absent")
		assert.Contains(t, *firstUpdate.ErrorMessage, "Service httpd")

		secondUpdate := reportedPayloads[1]
		assert.False(t, secondUpdate.IsDrifted, "Second update should not indicate drift after reconciliation")
		assert.Equal(t, "Applied", secondUpdate.Status, "Second update should be 'Applied'")
		assert.NotNil(t, secondUpdate.ErrorMessage, "Second update should contain success message")
		assert.Equal(t, "Drift auto-reconciled successfully", *secondUpdate.ErrorMessage)

		mockMgr.AssertExpectations(t)
	})

	t.Run("Drift Detected with Auto-Reconcile Failure", func(t *testing.T) {
		mockMgr := new(MockConfigManager)
		testAgent.configManager = mockMgr
		testAgent.autoReconcile = true

		var reportedPayloads []statusUpdatePayload
		handlerCallCount := 0
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCallCount++
			bodyBytes, _ := io.ReadAll(r.Body)
			var payload statusUpdatePayload
			_ = json.Unmarshal(bodyBytes, &payload)
			reportedPayloads = append(reportedPayloads, payload)
			_ = r.Body.Close()
			w.WriteHeader(http.StatusOK)
		}))
		defer mockServer.Close()
		testAgent.CentralURL = mockServer.URL

		dummyConfig := &config.ServerConfig{ /* ... */ }
		driftMessages := []string{"Package drift", "Service drift"}
		reconcileErr := errors.New("failed to apply configuration")
		mockMgr.On("Load", expectedConfigLoadPath).Return(dummyConfig, nil).Times(2)
		mockMgr.On("CheckDrift", dummyConfig, mock.Anything, mock.Anything).Return(driftMessages, nil).Once()
		mockMgr.On("Apply", dummyConfig, mock.Anything, mock.Anything).Return(reconcileErr).Once()

		err := testAgent.checkAndReportDrift()
		require.NoError(t, err, "checkAndReportDrift should not return error even if reconciliation fails")

		// Wait for both status updates to be received
		waitForCondition(t, func() bool {
			return handlerCallCount == 2 && len(reportedPayloads) == 2
		}, 2*time.Second, "receiving 2 status updates")

		assert.Equal(t, 2, handlerCallCount, "Should have received exactly 2 status updates")
		require.Len(t, reportedPayloads, 2, "Should have captured 2 status updates")

		firstUpdate := reportedPayloads[0]
		assert.True(t, firstUpdate.IsDrifted, "First update should indicate drift")
		assert.Equal(t, "Drift Detected", firstUpdate.Status, "First update should be 'Drift Detected'")

		secondUpdate := reportedPayloads[1]
		assert.False(t, secondUpdate.IsDrifted, "Second update should not indicate drift")
		assert.Equal(t, "Error", secondUpdate.Status, "Second update should be 'Error'")
		assert.NotNil(t, secondUpdate.ErrorMessage, "Second update should contain error message")
		assert.Contains(t, *secondUpdate.ErrorMessage, "Failed to auto-reconcile drift")

		mockMgr.AssertExpectations(t)

		testAgent.autoReconcile = false
	})

	t.Run("CheckDrift Error", func(t *testing.T) {
		mockMgr := new(MockConfigManager)
		testAgent.configManager = mockMgr
		testAgent.autoReconcile = false

		handlerCalled := false
		reportedPayload := statusUpdatePayload{}
		dummyConfig := &config.ServerConfig{ /* ... */ }
		checkErr := errors.New("failed to check drift")
		mockMgr.On("Load", expectedConfigLoadPath).Return(dummyConfig, nil).Once()
		mockMgr.On("CheckDrift", dummyConfig, mock.Anything, mock.Anything).Return(nil, checkErr).Once()

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			bodyBytes, _ := io.ReadAll(r.Body)
			json.Unmarshal(bodyBytes, &reportedPayload)
			defer r.Body.Close()
			w.WriteHeader(http.StatusOK)
		}))
		defer mockServer.Close()
		testAgent.CentralURL = mockServer.URL

		err := testAgent.checkAndReportDrift()
		require.Error(t, err)
		assert.ErrorIs(t, err, checkErr)

		// Wait for status update to be received
		waitForCondition(t, func() bool {
			return handlerCalled
		}, 2*time.Second, "receiving status update")

		assert.True(t, handlerCalled, "Status report handler not called when error occurred")
		assert.Equal(t, "Error", reportedPayload.Status)

		if reportedPayload.ErrorMessage == nil {
			t.Error("Error message pointer is nil but should contain error text")
		} else {
			assert.Contains(t, *reportedPayload.ErrorMessage, "failed to check drift")
		}
		mockMgr.AssertExpectations(t)
	})
}

func TestFetchTargetState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		serverResponse     any
		statusCode         int
		initialTargetHash  string
		expectedTargetHash string
		expectError        bool
	}{
		{
			name: "Success with new target hash",
			serverResponse: map[string]string{
				"repo_url":           "https://github.com/test/repo.git",
				"branch":             "main",
				"deploy_path":        "/config",
				"target_commit_hash": "abcdef1234567890",
			},
			statusCode:         http.StatusOK,
			initialTargetHash:  "",
			expectedTargetHash: "abcdef1234567890",
			expectError:        false,
		},
		{
			name: "No change in target hash",
			serverResponse: map[string]string{
				"repo_url":           "https://github.com/test/repo.git",
				"branch":             "main",
				"deploy_path":        "/config",
				"target_commit_hash": "abcdef1234567890",
			},
			statusCode:         http.StatusOK,
			initialTargetHash:  "abcdef1234567890",
			expectedTargetHash: "abcdef1234567890",
			expectError:        false,
		},
		{
			name: "Empty target hash",
			serverResponse: map[string]string{
				"repo_url":           "https://github.com/test/repo.git",
				"branch":             "main",
				"deploy_path":        "/config",
				"target_commit_hash": "",
			},
			statusCode:         http.StatusOK,
			initialTargetHash:  "abcdef1234567890",
			expectedTargetHash: "abcdef1234567890",
			expectError:        false,
		},
		{
			name:               "Server error",
			serverResponse:     nil,
			statusCode:         http.StatusInternalServerError,
			initialTargetHash:  "abcdef1234567890",
			expectedTargetHash: "abcdef1234567890",
			expectError:        true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

				assert.Equal(t, "/api/v1/agent/serverinfo", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)

				w.WriteHeader(tc.statusCode)
				if tc.serverResponse != nil {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(tc.serverResponse)
				}
			}))
			defer server.Close()

			a := &Agent{
				ServerID:         "test-server",
				CentralURL:       server.URL,
				targetCommitHash: tc.initialTargetHash,
				httpClient:       server.Client(),
			}

			err := a.fetchTargetState()

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedTargetHash, a.targetCommitHash)
		})
	}
}

func TestAgentVersionInitialization(t *testing.T) {
	t.Parallel()

	dummyRepoPath := t.TempDir()
	err := exec.Command("git", "init", dummyRepoPath).Run()
	require.NoError(t, err, "Failed to run 'git init' in dummy repo path")

	t.Run("New initializes version correctly", func(t *testing.T) {
		testVersion := "v1.2.3-test"

		agent, err := New(
			"test-server", "http://localhost:8080", dummyRepoPath,
			"http://example.git", "main", "config.yaml",
			"", "", "", 1*time.Second, "", false, testVersion,
		)
		require.NoError(t, err)

		assert.Equal(t, testVersion, agent.version, "Agent version should be initialized correctly")
	})

	t.Run("New handles empty version", func(t *testing.T) {
		agent, err := New(
			"test-server", "http://localhost:8080", dummyRepoPath,
			"http://example.git", "main", "config.yaml",
			"", "", "", 1*time.Second, "", false, "",
		)
		require.NoError(t, err)

		assert.Equal(t, "", agent.version, "Agent should accept empty version")
	})

	t.Run("New handles dev version", func(t *testing.T) {
		agent, err := New(
			"test-server", "http://localhost:8080", dummyRepoPath,
			"http://example.git", "main", "config.yaml",
			"", "", "", 1*time.Second, "", false, "dev",
		)
		require.NoError(t, err)

		assert.Equal(t, "dev", agent.version, "Agent should accept dev version")
	})
}

func TestReportStatusIncludesVersion(t *testing.T) {
	t.Parallel()

	dummyRepoPath := t.TempDir()
	err := exec.Command("git", "init", dummyRepoPath).Run()
	require.NoError(t, err, "Failed to run 'git init' in dummy repo path")

	t.Run("status payload includes agent version", func(t *testing.T) {
		testVersion := "v1.0.0-test"
		var receivedPayload statusUpdatePayload
		payloadReceived := false

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/agent/status" {
				bodyBytes, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(bodyBytes, &receivedPayload)
				_ = r.Body.Close()
				payloadReceived = true
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer mockServer.Close()

		agent, err := New(
			"test-server", mockServer.URL, dummyRepoPath,
			"http://example.git", "main", "config.yaml",
			"user", "pass", "", 1*time.Second, "", false, testVersion,
		)
		require.NoError(t, err)
		agent.jwtToken = "fake-token"

		agent.ExportDoReportStatus("abc123", "Applied", false, "")

		require.True(t, payloadReceived, "Status payload should be sent")
		assert.Equal(t, testVersion, receivedPayload.AgentVersion, "Payload should include correct agent version")
	})

	t.Run("status payload with empty version", func(t *testing.T) {
		var receivedPayload statusUpdatePayload
		payloadReceived := false

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/agent/status" {
				bodyBytes, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(bodyBytes, &receivedPayload)
				_ = r.Body.Close()
				payloadReceived = true
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer mockServer.Close()

		agent, err := New(
			"test-server", mockServer.URL, dummyRepoPath,
			"http://example.git", "main", "config.yaml",
			"user", "pass", "", 1*time.Second, "", false, "",
		)
		require.NoError(t, err)
		agent.jwtToken = "fake-token"

		agent.ExportDoReportStatus("abc123", "Applied", false, "")

		require.True(t, payloadReceived, "Status payload should be sent")
		assert.Equal(t, "", receivedPayload.AgentVersion, "Payload should handle empty version")
	})
}
