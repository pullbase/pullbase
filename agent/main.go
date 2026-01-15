package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pullbase/pullbase/agent/pkg/agent"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func VersionString() string {
	return fmt.Sprintf("pullbase-agent version %s (commit: %s, built: %s)", Version, GitCommit, BuildTime)
}

type ServerInfo struct {
	GitRepoURL       string `json:"repo_url"`
	GitBranch        string `json:"branch"`
	ConfigPath       string `json:"deploy_path"`
	TargetCommitHash string `json:"target_commit_hash,omitempty"`
	AutoReconcile    bool   `json:"auto_reconcile"`
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

func main() {
	dryRunFlag := flag.Bool("dry-run", false, "Run in dry-run mode: report what would change without applying")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(VersionString())
		os.Exit(0)
	}

	log.Printf("Starting PullBase Agent version %s (commit: %s)", Version, GitCommit)

	getEnv := func(keys ...string) string {
		for _, key := range keys {
			if value := os.Getenv(key); value != "" {
				return value
			}
		}
		return ""
	}

	dryRun := *dryRunFlag
	if !dryRun {
		dryRunEnv := getEnv("AGENT_DRY_RUN", "PULLBASE_AGENT_DRY_RUN")
		dryRun = strings.ToLower(dryRunEnv) == "true" || dryRunEnv == "1"
	}

	if dryRun {
		log.Printf("*** DRY-RUN MODE ENABLED ***")
		log.Printf("Agent will report what would change without applying any modifications.")
	}

	serverID := getEnv("SERVER_ID", "PULLBASE_SERVER_ID")
	centralURL := getEnv("CENTRAL_SERVER_URL", "PULLBASE_SERVER_URL")
	agentUsername := getEnv("AGENT_USERNAME", "PULLBASE_AGENT_USERNAME")
	agentPassword := getEnv("AGENT_PASSWORD", "PULLBASE_AGENT_PASSWORD")
	agentToken := getEnv("AGENT_TOKEN", "PULLBASE_AGENT_TOKEN")
	localRepoPath := "/etc/pullbase/repo"
	caCertPath := getEnv("CACERT_PATH", "PULLBASE_CACERT_PATH")
	if serverID == "" {
		serverID = "test-server-1"
		log.Printf("WARN: SERVER_ID/PULLBASE_SERVER_ID not set, defaulting to '%s'", serverID)
	}
	if centralURL == "" {
		centralURL = "http://central-server:8080"
		log.Printf("WARN: CENTRAL_SERVER_URL/PULLBASE_SERVER_URL not set, defaulting to '%s'", centralURL)
	}

	// Validate URL has a scheme
	if !strings.HasPrefix(centralURL, "http://") && !strings.HasPrefix(centralURL, "https://") {
		log.Printf("WARN: centralURL lacks scheme, prepending https:// (original: %s)", centralURL)
		centralURL = "https://" + centralURL
	}

	// Check authentication method
	if agentToken != "" {
		log.Println("Using agent token for authentication")
	} else {
		if agentUsername == "" {
			log.Println("WARN: AGENT_USERNAME/PULLBASE_AGENT_USERNAME environment variable not set.")
		}
		if agentPassword == "" {
			log.Println("WARN: AGENT_PASSWORD/PULLBASE_AGENT_PASSWORD environment variable not set.")
		}
		if agentUsername == "" || agentPassword == "" {
			log.Println("WARN: No agent token provided and username/password incomplete. Authentication may fail.")
		}
	}

	log.Printf("Agent configuration: serverID=%s, centralURL=%s", serverID, centralURL)

	// Create HTTP client with CA certificate support for initial server info fetch
	httpClient, err := createHTTPClient(caCertPath)
	if err != nil {
		log.Fatalf("Failed to create HTTP client: %v", err)
	}

	// Fetch config source from central server
	infoURL := fmt.Sprintf("%s/api/v1/serverinfo/%s", centralURL, serverID)
	resp, err := httpClient.Get(infoURL)
	if err != nil {
		log.Fatalf("Failed to fetch server info from %s: %v", infoURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Failed to fetch server info: status code %d", resp.StatusCode)
	}

	var serverInfo ServerInfo
	if err := json.NewDecoder(resp.Body).Decode(&serverInfo); err != nil {
		log.Fatalf("Failed to decode server info: %v", err)
	}

	log.Printf("Received server info: repo=%s, branch=%s, path=%s, target_commit=%s",
		serverInfo.GitRepoURL, serverInfo.GitBranch, serverInfo.ConfigPath,
		serverInfo.TargetCommitHash)

	log.Printf("Creating agent instance...")
	a, err := agent.New(
		serverID,
		centralURL,
		localRepoPath,
		serverInfo.GitRepoURL,
		serverInfo.GitBranch,
		serverInfo.ConfigPath,
		agentUsername,
		agentPassword,
		agentToken,
		30*time.Second,
		caCertPath,
		dryRun,
		Version,
	)

	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Set auto-reconcile setting
	a.SetAutoReconcile(serverInfo.AutoReconcile)

	// Start the agent
	log.Printf("Starting agent...")
	if err := a.Start(); err != nil {
		log.Fatalf("Agent failed: %v", err)
	}
}
