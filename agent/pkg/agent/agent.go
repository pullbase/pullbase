package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/pullbase/pullbase/agent/pkg/config"
	"github.com/pullbase/pullbase/agent/pkg/git"
)

// StatusUpdate defines the structure for status updates passed internally.
type StatusUpdate struct {
	CommitHash   string
	Status       string
	IsDrifted    bool
	ErrorMessage string
}

const (
	gitTokenRefreshLeeway  = time.Minute
	defaultGitTokenBackoff = 10 * time.Second
)

type agentGitTokenResponse struct {
	Token          string    `json:"token"`
	ExpiresAt      time.Time `json:"expires_at"`
	RepoURL        string    `json:"repo_url"`
	Provider       string    `json:"provider"`
	InstallationID int64     `json:"installation_id"`
	Authentication string    `json:"authentication"`
}

type tokenRequestError struct {
	status     int
	retryAfter time.Duration
	message    string
}

func (e *tokenRequestError) Error() string {
	return e.message
}

type Agent struct {
	ServerID          string
	CentralURL        string
	PollInterval      time.Duration
	LocalRepoPath     string
	GitRepoURL        string
	GitBranch         string
	ConfigFilePath    string
	GitWatcher        *git.Watcher
	lastAppliedCommit string
	targetCommitHash  string
	httpClient        *http.Client
	agentUsername     string
	agentPassword     string
	agentToken        string
	jwtToken          string
	svcManager        config.ServiceManager
	pkgManager        config.PackageManager
	configManager     config.ConfigManager
	autoReconcile     bool
	dryRun            bool
	version           string
	Log               *log.Logger
	StatusChan        chan StatusUpdate
	StopChan          chan struct{}
	gitToken          string
	gitTokenExpiry    time.Time
	gitAuthDisabled   bool
	gitTokenBackoff   time.Time
}

// LoginResponse defines the structure for login responses from the server
type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

// statusUpdatePayload defines the structure for sending status updates.
type statusUpdatePayload struct {
	CommitHash   string  `json:"commit_hash"`
	Status       string  `json:"status"`
	IsDrifted    bool    `json:"is_drifted"`
	ErrorMessage *string `json:"error_message,omitempty"`
	AgentVersion string  `json:"agent_version,omitempty"`
}

// New creates and initializes a new Agent instance.
func New(
	serverID,
	centralURL,
	localRepoPath,
	gitRepoURL,
	gitBranch,
	configFilePath,
	agentUsername,
	agentPassword,
	agentToken string,
	pollInterval time.Duration,
	caCertPath string,
	dryRun bool,
	version string,
) (*Agent, error) {
	// Create custom HTTP client with TLS config
	httpClient, err := createHTTPClient(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	svcManager := config.DetectServiceManager()
	pkgManager := config.DetectPackageManager()

	a := &Agent{
		ServerID:       serverID,
		CentralURL:     strings.TrimSuffix(centralURL, "/"),
		PollInterval:   pollInterval,
		LocalRepoPath:  localRepoPath,
		GitRepoURL:     gitRepoURL,
		GitBranch:      gitBranch,
		ConfigFilePath: configFilePath,
		httpClient:     httpClient,
		agentUsername:  agentUsername,
		agentPassword:  agentPassword,
		agentToken:     agentToken,
		StatusChan:     make(chan StatusUpdate, 10),
		Log:            log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds),
		StopChan:       make(chan struct{}),
		svcManager:     svcManager,
		pkgManager:     pkgManager,
		configManager:  config.NewDefaultConfigManager(),
		dryRun:         dryRun,
		version:        version,
	}

	watcher, err := git.New(localRepoPath, gitRepoURL, gitBranch, pollInterval, a.gitAuth)
	if err != nil {
		return nil, err
	}
	a.GitWatcher = watcher

	return a, nil
}

// createHTTPClient initializes an HTTP client, potentially with a custom CA.
func createHTTPClient(caCertPath string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	// Check if TLS verification should be skipped (for development environments)
	skipTLSVerify := os.Getenv("SKIP_TLS_VERIFY") == "true"
	if skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
		log.Println("WARNING: HTTP client configured to skip TLS certificate verification (development mode)")
	} else if caCertPath != "" {
		// Load custom CA certificate
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate file %s: %w", caCertPath, err)
		}

		// Get system certificate pool
		rootCAs, _ := x509.SystemCertPool()
		if rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}

		// Append custom CA to the pool
		if ok := rootCAs.AppendCertsFromPEM(caCert); !ok {
			return nil, fmt.Errorf("failed to append CA certificate from %s", caCertPath)
		}

		// Configure TLS with the combined CA pool
		tlsConfig := &tls.Config{
			RootCAs: rootCAs,
		}
		transport.TLSClientConfig = tlsConfig
		log.Printf("INFO: HTTP client configured with custom CA certificate from %s", caCertPath)
	} else {
		log.Println("INFO: HTTP client configured with system default CA certificates")
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	return client, nil
}

// login attempts to authenticate with the central server and stores the JWT.
func (a *Agent) login() error {
	log.Printf("Attempting login to %s as user '%s' for server '%s'...", a.CentralURL, a.agentUsername, a.ServerID)

	loginURL := fmt.Sprintf("%s/api/v1/auth/login", a.CentralURL)

	payload := map[string]interface{}{
		"username":  a.agentUsername,
		"password":  a.agentPassword,
		"server_id": a.ServerID, // Keep for backward compatibility during transition
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal login payload: %w", err)
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		log.Printf("WARN: Login request failed: %v", err)
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: Login failed with status code %d", resp.StatusCode)
		return fmt.Errorf("login failed with status code %d", resp.StatusCode)
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		log.Printf("ERROR: Failed to decode login response: %v", err)
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	if loginResp.AccessToken == "" {
		log.Printf("ERROR: Login response did not contain access token")
		return fmt.Errorf("login response missing access token")
	}

	a.jwtToken = loginResp.AccessToken
	log.Printf("Successfully logged in and obtained JWT token for server %s.", a.ServerID)
	return nil
}

// doAuthenticatedRequest performs an HTTP request, handling potential 401 errors by attempting re-login.
func (a *Agent) doAuthenticatedRequest(req *http.Request) (*http.Response, error) {
	// Use agent token if available, otherwise use JWT token
	if a.agentToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.agentToken)
	} else if a.jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.jwtToken)
	} else {
		log.Printf("WARN: Making authenticated request without any authentication token.")
	}

	// Make the initial request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return resp, err // Return original error if request itself failed
	}

	// Check for Unauthorized status
	if resp.StatusCode == http.StatusUnauthorized {
		if a.agentToken != "" {
			log.Printf("ERROR: Agent token authentication failed. Token may be invalid or expired.")
			return resp, fmt.Errorf("agent token authentication failed")
		}

		log.Printf("INFO: Received 401 Unauthorized. Attempting re-login...")
		resp.Body.Close()

		if loginErr := a.login(); loginErr != nil {
			log.Printf("ERROR: Re-login failed: %v", loginErr)
			return nil, fmt.Errorf("re-login failed after 401: %w", loginErr)
		}

		log.Printf("INFO: Re-login successful. Retrying original request to %s...", req.URL)

		clonedReq := req.Clone(req.Context())
		if req.GetBody != nil {
			bodyCopy, getBodyErr := req.GetBody()
			if getBodyErr != nil {
				return nil, fmt.Errorf("failed to reset request body: %w", getBodyErr)
			}
			clonedReq.Body = bodyCopy
		}
		clonedReq.ContentLength = req.ContentLength
		clonedReq.Header.Set("Authorization", "Bearer "+a.jwtToken)

		retryResp, retryErr := a.httpClient.Do(clonedReq)

		return retryResp, retryErr
	}

	return resp, err
}

func (a *Agent) gitAuth() (transport.AuthMethod, error) {
	if a.gitAuthDisabled {
		return nil, nil
	}

	token, err := a.ensureGitToken()
	if err != nil {
		return nil, err
	}

	if token == "" {
		return nil, nil
	}

	return &githttp.BasicAuth{Username: "x-access-token", Password: token}, nil
}

func (a *Agent) ensureGitToken() (string, error) {
	if a.gitAuthDisabled {
		return "", nil
	}

	now := time.Now()
	if !a.gitTokenBackoff.IsZero() && now.Before(a.gitTokenBackoff) {
		a.Log.Printf("Git token requests for server %s are throttled until %s", a.ServerID, a.gitTokenBackoff.Format(time.RFC3339))
		return "", nil
	}

	if a.gitToken != "" && !a.gitTokenExpiry.IsZero() && time.Until(a.gitTokenExpiry) > gitTokenRefreshLeeway {
		return a.gitToken, nil
	}

	return a.refreshGitToken()
}

func (a *Agent) refreshGitToken() (string, error) {
	resp, err := a.requestGitToken()
	if err != nil {
		var tre *tokenRequestError
		if errors.As(err, &tre) {
			backoff := defaultGitTokenBackoff
			if tre.retryAfter > 0 {
				backoff = tre.retryAfter
			}
			a.gitTokenBackoff = time.Now().Add(backoff)
			a.Log.Printf("Git token request throttled (status %d); backing off for %s", tre.status, backoff)
			return "", nil
		}
		return "", err
	}

	if resp == nil || resp.Token == "" {
		a.gitToken = ""
		a.gitTokenExpiry = time.Time{}
		return "", nil
	}

	a.gitAuthDisabled = false
	a.gitTokenBackoff = time.Time{}
	a.gitToken = resp.Token
	a.gitTokenExpiry = resp.ExpiresAt

	if resp.RepoURL != "" {
		a.GitRepoURL = resp.RepoURL
	}

	return a.gitToken, nil
}

func (a *Agent) requestGitToken() (*agentGitTokenResponse, error) {
	url := fmt.Sprintf("%s/api/v1/agent/git-token", a.CentralURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create git token request: %w", err)
	}

	resp, err := a.doAuthenticatedRequest(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("git token response was nil")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var tokenResp agentGitTokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			return nil, fmt.Errorf("failed to decode git token response: %w", err)
		}
		log.Printf("INFO: Received GitHub installation token expiring at %s", tokenResp.ExpiresAt.Format(time.RFC3339))
		return &tokenResp, nil
	case http.StatusBadRequest, http.StatusNotFound:
		log.Printf("INFO: Git credential endpoint returned %d – assuming no authentication required", resp.StatusCode)
		a.gitAuthDisabled = true
		return nil, nil
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		retry := parseRetryAfter(resp.Header.Get("Retry-After"))
		body, _ := io.ReadAll(resp.Body)
		message := fmt.Sprintf("git token request returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return nil, &tokenRequestError{status: resp.StatusCode, retryAfter: retry, message: message}
	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git token request returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}

	if secs, err := strconv.Atoi(header); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}

	if when, err := http.ParseTime(header); err == nil {
		delta := time.Until(when)
		if delta > 0 {
			return delta
		}
	}

	return 0
}

// reportStatus sends the current status to the central server.
func (a *Agent) reportStatus(commitHash string, currentStatus string, isDrifted bool, errMsg string) {
	var errorMessagePtr *string
	if errMsg != "" {
		errorMessagePtr = &errMsg
	}

	payload := statusUpdatePayload{
		CommitHash:   commitHash,
		Status:       currentStatus,
		IsDrifted:    isDrifted,
		ErrorMessage: errorMessagePtr,
		AgentVersion: a.version,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling status payload: %v", err)
		return
	}

	statusURL := fmt.Sprintf("%s/api/v1/agent/status", a.CentralURL)
	bodyReader := bytes.NewReader(jsonData)
	req, err := http.NewRequest(http.MethodPut, statusURL, bodyReader)
	if err != nil {
		log.Printf("Error creating status request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.doAuthenticatedRequest(req)
	if err != nil {
		log.Printf("Error sending status update to %s: %v", statusURL, err)
		return
	}
	// Ensure body is closed even if we don't read it on success
	if resp != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		log.Printf("Central server returned non-OK status for status update: %d", resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Response body: %s", string(body))
		return
	}

	log.Printf("Successfully reported status '%s' to central server.", currentStatus)
}

func (a *Agent) Start() error {
	log.Printf("Starting agent for server %s", a.ServerID)

	// Only attempt login if we don't have an agent token
	if a.agentToken != "" {
		log.Printf("Using agent token for authentication")
	} else {
		if err := a.login(); err != nil {
			log.Printf("WARN: Initial login failed: %v. Agent will retry periodic reporting without token.", err)
			// Agent can continue running but status reports will fail until login succeeds
			// TODO: Implement retry logic for login?
		}
	}

	// Setup context for cancellation
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Ticker for polling controller for target state
	controllerPollTicker := time.NewTicker(a.PollInterval)
	defer controllerPollTicker.Stop()

	// Ticker for periodic drift checks
	driftCheckInterval := 1 * time.Minute
	driftTicker := time.NewTicker(driftCheckInterval)
	defer driftTicker.Stop()

	// Initialize lastAppliedCommit with current commit if available
	if a.lastAppliedCommit == "" {
		if currentCommit, err := a.GitWatcher.GetCurrentCommit(); err == nil && currentCommit != "" {
			a.lastAppliedCommit = currentCommit
			log.Printf("Initialized lastAppliedCommit to current commit: %s", currentCommit)
		}
	}

	if a.dryRun {
		log.Printf("[DRY-RUN] Agent running in dry-run mode - no changes will be applied")
		a.reportStatus(a.lastAppliedCommit, "Dry-Run: Starting", false, "")
	} else {
		a.reportStatus(a.lastAppliedCommit, "Starting", false, "")
	}

	if err := a.fetchTargetState(); err != nil {
		log.Printf("WARN: Initial fetch of target state failed: %v. Will retry periodically.", err)
	}

	for {
		select {
		case <-controllerPollTicker.C:
			log.Printf("Polling controller for target state...")
			previousTarget := a.targetCommitHash

			if err := a.fetchTargetState(); err != nil {
				log.Printf("Error fetching target state: %v", err)
				continue
			}

			if a.targetCommitHash != "" && a.targetCommitHash != previousTarget {
				log.Printf("Target commit hash changed from %s to %s, syncing...",
					previousTarget, a.targetCommitHash)

				if a.dryRun {
					a.reportStatus(a.lastAppliedCommit, "Dry-Run: Syncing", false, "")
				} else {
					a.reportStatus(a.lastAppliedCommit, "Syncing", false, "")
				}

				if err := a.GitWatcher.CheckoutCommit(a.targetCommitHash); err != nil {
					log.Printf("Error checking out target commit %s: %v", a.targetCommitHash, err)
					errStatus := "Error"
					if a.dryRun {
						errStatus = "Dry-Run Error"
					}
					a.reportStatus(a.lastAppliedCommit, errStatus, false, fmt.Sprintf("Failed to checkout commit: %v", err))
					continue
				}

				a.lastAppliedCommit = a.targetCommitHash
				if err := a.reconcileState(); err != nil {
					log.Printf("Error reconciling state to target commit %s: %v", a.targetCommitHash, err)
					errStatus := "Error"
					if a.dryRun {
						errStatus = "Dry-Run Error"
					}
					a.reportStatus(a.targetCommitHash, errStatus, false, err.Error())
				} else {
					if err := a.checkAndReportDrift(); err != nil {
						log.Printf("Error during post-sync drift check: %v", err)
						commitHash := a.lastAppliedCommit
						if commitHash == "" {
							commitHash = a.targetCommitHash
							if commitHash == "" {
								commitHash = "pending"
							}
						}
						errStatus := "Error"
						if a.dryRun {
							errStatus = "Dry-Run Error"
						}
						a.reportStatus(commitHash, errStatus, false, fmt.Sprintf("Post-sync drift check failed: %v", err))
					}
				}
			}
		case <-driftTicker.C:
			if a.dryRun {
				log.Printf("[DRY-RUN] Performing periodic drift check...")
			} else {
				log.Printf("Performing periodic drift check...")
			}
			if err := a.checkAndReportDrift(); err != nil {
				log.Printf("Error during periodic drift check: %v", err)
				commitHash := a.lastAppliedCommit
				if commitHash == "" {
					commitHash = a.targetCommitHash
					if commitHash == "" {
						commitHash = "pending"
					}
				}
				errStatus := "Error"
				if a.dryRun {
					errStatus = "Dry-Run Error"
				}
				a.reportStatus(commitHash, errStatus, false, fmt.Sprintf("Periodic drift check failed: %v", err))
			}
		case sig := <-signalChan:
			log.Printf("Received signal: %v. Shutting down agent gracefully...", sig)
			cancel()
			a.reportStatus(a.lastAppliedCommit, "Shutting down", false, "")
			return nil
		}
	}
}

func (a *Agent) reconcileState() error {
	if a.dryRun {
		log.Printf("[DRY-RUN] Would reconcile state to target commit %s...", a.targetCommitHash)
	} else {
		log.Printf("Reconciling state to target commit %s...", a.targetCommitHash)
	}

	fullConfigPath := filepath.Join(a.LocalRepoPath, a.ConfigFilePath)

	loadedConfig, err := a.configManager.Load(fullConfigPath)
	if err != nil {
		log.Printf("Error loading config during reconcile: %v", err)
		return fmt.Errorf("failed to load config %s: %w", fullConfigPath, err)
	}

	if a.dryRun {
		a.logDryRunChanges(loadedConfig)
		a.lastAppliedCommit = a.targetCommitHash
		log.Printf("[DRY-RUN] State check complete. No changes applied.")
		return nil
	}

	if err := a.configManager.Apply(loadedConfig, a.svcManager, a.pkgManager); err != nil {
		log.Printf("Error applying configuration: %v", err)
		return fmt.Errorf("failed to apply config: %w", err)
	}

	a.lastAppliedCommit = a.targetCommitHash
	log.Printf("State reconciled successfully. Last applied commit: %s", a.lastAppliedCommit)
	return nil
}

// logDryRunChanges logs what changes would be applied without actually applying them.
func (a *Agent) logDryRunChanges(cfg *config.ServerConfig) {
	log.Printf("[DRY-RUN] Configuration preview for server: %s (environment: %s)",
		cfg.ServerMetadata.Name, cfg.ServerMetadata.Environment)

	// Log package changes
	if len(cfg.Packages) > 0 {
		log.Printf("[DRY-RUN] Packages (%d):", len(cfg.Packages))
		for _, pkg := range cfg.Packages {
			installed, err := a.pkgManager.IsInstalled(pkg.Name)
			if err != nil {
				log.Printf("[DRY-RUN]   - %s: state=%s (unable to check current state: %v)", pkg.Name, pkg.State, err)
				continue
			}

			switch pkg.State {
			case "present", "latest":
				if installed {
					if pkg.State == "latest" {
						log.Printf("[DRY-RUN]   - %s: would ensure latest version (currently installed)", pkg.Name)
					} else {
						log.Printf("[DRY-RUN]   - %s: already installed, no action needed", pkg.Name)
					}
				} else {
					log.Printf("[DRY-RUN]   - %s: would INSTALL (currently absent)", pkg.Name)
				}
			case "absent":
				if installed {
					log.Printf("[DRY-RUN]   - %s: would REMOVE (currently installed)", pkg.Name)
				} else {
					log.Printf("[DRY-RUN]   - %s: already absent, no action needed", pkg.Name)
				}
			default:
				log.Printf("[DRY-RUN]   - %s: unknown state '%s'", pkg.Name, pkg.State)
			}
		}
	} else {
		log.Printf("[DRY-RUN] Packages: none defined")
	}

	// Log file changes
	if len(cfg.Files) > 0 {
		log.Printf("[DRY-RUN] Files (%d):", len(cfg.Files))
		for _, file := range cfg.Files {
			_, err := os.Stat(file.Path)
			fileExists := err == nil

			if !fileExists {
				log.Printf("[DRY-RUN]   - %s: would CREATE (%d bytes)", file.Path, len(file.Content))
			} else {
				log.Printf("[DRY-RUN]   - %s: would UPDATE/VERIFY (%d bytes)", file.Path, len(file.Content))
			}

			if file.Mode != "" {
				log.Printf("[DRY-RUN]     mode: %s", file.Mode)
			}
			if file.ReloadService != "" {
				log.Printf("[DRY-RUN]     would reload service: %s", file.ReloadService)
			}
			if file.ReloadCommand != "" {
				log.Printf("[DRY-RUN]     would run command: %s", file.ReloadCommand)
			}
		}
	} else {
		log.Printf("[DRY-RUN] Files: none defined")
	}

	// Log service changes
	if len(cfg.Services) > 0 {
		log.Printf("[DRY-RUN] Services (%d):", len(cfg.Services))
		for _, svc := range cfg.Services {
			if !svc.Managed {
				log.Printf("[DRY-RUN]   - %s: unmanaged (monitoring only)", svc.Name)
				continue
			}

			isActive, activeErr := a.svcManager.IsActive(svc.Name)
			isEnabled, enabledErr := a.svcManager.IsEnabled(svc.Name)

			statusInfo := ""
			if activeErr != nil {
				statusInfo = fmt.Sprintf("(unable to check active state: %v)", activeErr)
			} else if enabledErr != nil {
				statusInfo = fmt.Sprintf("(active=%t, unable to check enabled state: %v)", isActive, enabledErr)
			} else {
				statusInfo = fmt.Sprintf("(currently: active=%t, enabled=%t)", isActive, isEnabled)
			}

			actions := []string{}

			// Check enabled state
			if enabledErr == nil {
				if svc.Enabled && !isEnabled {
					actions = append(actions, "ENABLE")
				} else if !svc.Enabled && isEnabled {
					actions = append(actions, "DISABLE")
				}
			}

			// Check running state
			if activeErr == nil {
				if svc.State == "running" && !isActive {
					actions = append(actions, "START")
				} else if svc.State == "stopped" && isActive {
					actions = append(actions, "STOP")
				}
			}

			if len(actions) > 0 {
				log.Printf("[DRY-RUN]   - %s: would %s %s", svc.Name, strings.Join(actions, ", "), statusInfo)
			} else {
				log.Printf("[DRY-RUN]   - %s: no action needed %s", svc.Name, statusInfo)
			}
		}
	} else {
		log.Printf("[DRY-RUN] Services: none defined")
	}
}

// fetchTargetState retrieves target configuration from the central server
func (a *Agent) fetchTargetState() error {
	log.Printf("Fetching target state from central server...")

	// Construct URL for agent server info API
	url := fmt.Sprintf("%s/api/v1/agent/serverinfo", a.CentralURL)

	// Send request to the central server
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request for target state: %w", err)
	}

	resp, err := a.doAuthenticatedRequest(req)
	if err != nil {
		return fmt.Errorf("error fetching target state: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch target state, server returned status %d", resp.StatusCode)
	}

	// Define the structure to match the updated ServerInfoResponse
	var serverInfo struct {
		RepoURL          string `json:"repo_url"`
		Branch           string `json:"branch"`
		DeployPath       string `json:"deploy_path"`
		TargetCommitHash string `json:"target_commit_hash,omitempty"`
		AutoReconcile    bool   `json:"auto_reconcile"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&serverInfo); err != nil {
		return fmt.Errorf("error decoding server info response: %w", err)
	}

	a.GitRepoURL = serverInfo.RepoURL
	a.GitBranch = serverInfo.Branch
	a.ConfigFilePath = serverInfo.DeployPath

	if serverInfo.TargetCommitHash != "" {
		a.targetCommitHash = serverInfo.TargetCommitHash
	}

	a.autoReconcile = serverInfo.AutoReconcile
	log.Printf("Auto-reconciliation setting: %v", a.autoReconcile)

	log.Printf("Successfully fetched target state: repo=%s, branch=%s, config_path=%s, target_commit=%s",
		a.GitRepoURL, a.GitBranch, a.ConfigFilePath, a.targetCommitHash)

	return nil
}

// checkAndReportDrift checks for configuration drift and reports it.
// If drift is detected and auto-reconciliation is enabled, it attempts to resolve the drift.
// In dry-run mode, drift is checked and reported but no reconciliation occurs.
func (a *Agent) checkAndReportDrift() error {
	if a.dryRun {
		log.Printf("[DRY-RUN] Checking for configuration drift...")
	} else {
		log.Printf("Checking for configuration drift...")
	}

	fullConfigPath := filepath.Join(a.LocalRepoPath, a.ConfigFilePath)
	currentConfig, err := a.configManager.Load(fullConfigPath)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to load current config: %v", err)
		if a.dryRun {
			log.Printf("[DRY-RUN] Error: %s", errMsg)
			a.reportStatus(a.lastAppliedCommit, "Dry-Run Error", false, errMsg)
		} else {
			a.reportStatus(a.lastAppliedCommit, "Error", false, errMsg)
		}
		return fmt.Errorf("failed to load current config for drift check: %w", err)
	}

	drifts, err := a.configManager.CheckDrift(currentConfig, a.svcManager, a.pkgManager)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to check for drift: %v", err)
		if a.dryRun {
			log.Printf("[DRY-RUN] Error: %s", errMsg)
			a.reportStatus(a.lastAppliedCommit, "Dry-Run Error", false, errMsg)
		} else {
			a.reportStatus(a.lastAppliedCommit, "Error", false, errMsg)
		}
		return fmt.Errorf("failed to check for drift: %w", err)
	}

	isDrifted := len(drifts) > 0

	if isDrifted {
		driftMsg := fmt.Sprintf("Configuration drift detected: %s", strings.Join(drifts, ", "))

		if a.dryRun {
			log.Printf("[DRY-RUN] WARN: %s", driftMsg)
			a.reportStatus(a.lastAppliedCommit, "Dry-Run: Drift Detected", true, driftMsg)
			log.Printf("[DRY-RUN] Auto-reconciliation skipped (dry-run mode active)")
		} else {
			log.Printf("WARN: %s", driftMsg)
			a.reportStatus(a.lastAppliedCommit, "Drift Detected", true, driftMsg)

			if a.autoReconcile {
				log.Printf("Auto-reconciliation is enabled, attempting to resolve drift...")
				if err := a.reconcileState(); err != nil {
					reconcileErr := fmt.Sprintf("Failed to auto-reconcile drift: %v", err)
					log.Printf("ERROR: %s", reconcileErr)
					a.reportStatus(a.lastAppliedCommit, "Error", false, reconcileErr)
				} else {
					log.Printf("Successfully auto-reconciled configuration drift")
					a.reportStatus(a.lastAppliedCommit, "Applied", false, "Drift auto-reconciled successfully")
				}
			} else {
				log.Printf("Auto-reconciliation is disabled, drift will not be automatically resolved")
			}
		}
	} else {
		if a.dryRun {
			log.Printf("[DRY-RUN] No configuration drift detected")
			a.reportStatus(a.lastAppliedCommit, "Dry-Run: In Sync", false, "")
		} else {
			log.Printf("No configuration drift detected")
			a.reportStatus(a.lastAppliedCommit, "Applied", false, "")
		}
	}

	return nil
}

// ExportSetJwtToken allows tests to set the internal jwtToken.
func (a *Agent) ExportSetJwtToken(token string) {
	a.jwtToken = token
}

// ExportGetJwtToken allows tests to retrieve the internal jwtToken.
func (a *Agent) ExportGetJwtToken() string {
	return a.jwtToken
}

// ExportDoReportStatus allows tests to call the internal reportStatus function.
func (a *Agent) ExportDoReportStatus(commitHash string, currentStatus string, isDrifted bool, errMsg string) {
	a.reportStatus(commitHash, currentStatus, isDrifted, errMsg)
}

// SetAutoReconcile sets the auto-reconciliation setting for the agent.
func (a *Agent) SetAutoReconcile(enabled bool) {
	a.autoReconcile = enabled
	log.Printf("Auto-reconciliation setting updated to: %v", enabled)
}
