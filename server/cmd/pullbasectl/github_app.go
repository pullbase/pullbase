package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	appconfig "github.com/pullbase/pullbase/server/pkg/config"
	"github.com/pullbase/pullbase/server/pkg/githubapp"
)

type githubAppBootstrapOptions struct {
	AppID           int64
	PrivateKeyPath  string
	InstallationID  int64
	APIBaseURL      string
	ServerURL       string
	AdminToken      string
	EnvironmentName string
	RepoURL         string
	Branch          string
	DeployPath      string
	WebhookSecret   string
	AppSlug         string
	RepositoryID    int64
	HTTPClient      *http.Client
}

func runGitHubAppBootstrap(args []string) error {
	fs := flag.NewFlagSet("github-app bootstrap", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	opts := githubAppBootstrapOptions{}

	fs.Int64Var(&opts.AppID, "app-id", 0, "GitHub App ID")
	fs.StringVar(&opts.PrivateKeyPath, "private-key", "", "Path to GitHub App private key PEM")
	fs.Int64Var(&opts.InstallationID, "installation-id", 0, "GitHub App installation ID")
	fs.StringVar(&opts.APIBaseURL, "api-base-url", appconfig.DefaultGitHubAPIBaseURL, "GitHub API base URL")

	fs.StringVar(&opts.ServerURL, "server-url", "", "Pullbase server base URL")
	fs.StringVar(&opts.AdminToken, "admin-token", "", "Admin JWT token for Pullbase server")
	fs.StringVar(&opts.EnvironmentName, "environment-name", "", "Environment name")
	fs.StringVar(&opts.RepoURL, "repo-url", "", "Git repository URL")
	fs.StringVar(&opts.Branch, "branch", "main", "Git branch")
	fs.StringVar(&opts.DeployPath, "deploy-path", "config.yaml", "Path to config file in repository")
	fs.StringVar(&opts.WebhookSecret, "webhook-secret", "", "Webhook secret override")
	fs.StringVar(&opts.AppSlug, "app-slug", "", "GitHub App slug")
	fs.Int64Var(&opts.RepositoryID, "repository-id", 0, "GitHub repository ID")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if opts.AppID <= 0 {
		return errors.New("--app-id is required and must be greater than zero")
	}
	if strings.TrimSpace(opts.PrivateKeyPath) == "" {
		return errors.New("--private-key is required")
	}
	if opts.InstallationID <= 0 {
		return errors.New("--installation-id is required and must be greater than zero")
	}

	return performGitHubAppBootstrap(opts)
}

func performGitHubAppBootstrap(opts githubAppBootstrapOptions) error {
	cfg := appconfig.GitHubAppConfig{
		AppID:          opts.AppID,
		PrivateKeyPath: opts.PrivateKeyPath,
		APIBaseURL:     opts.APIBaseURL,
	}

	if err := appconfig.ValidateGitHubAppConfig(cfg); err != nil {
		return err
	}

	privateKey, err := cfg.LoadPrivateKey()
	if err != nil {
		return err
	}

	client, err := newGitHubAppClient(githubapp.Config{
		AppID:         cfg.AppID,
		PrivateKeyPEM: privateKey,
		APIBaseURL:    cfg.APIBaseURL,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize GitHub App client: %w", err)
	}

	fmt.Println("Validating GitHub App installation with GitHub...")
	if _, _, err := client.GetInstallationToken(context.Background(), opts.InstallationID); err != nil {
		return fmt.Errorf("failed to obtain installation token: %w", err)
	}
	fmt.Println("GitHub App credentials validated.")

	if strings.TrimSpace(opts.ServerURL) == "" {
		fmt.Println("Server URL not provided; skipping environment registration.")
		return nil
	}

	if strings.TrimSpace(opts.AdminToken) == "" {
		return errors.New("--admin-token is required when --server-url is provided")
	}
	if strings.TrimSpace(opts.EnvironmentName) == "" {
		return errors.New("--environment-name is required when --server-url is provided")
	}
	if strings.TrimSpace(opts.RepoURL) == "" {
		return errors.New("--repo-url is required when --server-url is provided")
	}

	payload := map[string]interface{}{
		"name":            opts.EnvironmentName,
		"repo_url":        opts.RepoURL,
		"branch":          opts.Branch,
		"deploy_path":     opts.DeployPath,
		"provider":        "github",
		"installation_id": opts.InstallationID,
	}
	if strings.TrimSpace(opts.WebhookSecret) != "" {
		payload["webhook_secret"] = opts.WebhookSecret
	}
	if strings.TrimSpace(opts.AppSlug) != "" {
		payload["app_slug"] = opts.AppSlug
	}
	if opts.RepositoryID > 0 {
		payload["repository_id"] = opts.RepositoryID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal environment payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimSuffix(opts.ServerURL, "/")+"/api/v1/environments", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create environment request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opts.AdminToken)

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("environment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	fmt.Printf("Environment '%s' registered successfully.\n", opts.EnvironmentName)
	return nil
}

func runGitHubAppStatus(args []string) error {
	fs := flag.NewFlagSet("github-app status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	environmentID := fs.Int64("environment-id", 0, "Environment ID")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *environmentID <= 0 {
		return errors.New("--environment-id must be greater than zero")
	}

	client, token, err := resolveAdminCredentials(adminAuthConfig{
		ServerURL:    *serverURL,
		AdminToken:   *adminToken,
		Username:     *username,
		Password:     *password,
		PasswordFile: *passwordFile,
		CACertPath:   *caCertPath,
		Insecure:     *insecureSkipVerify,
	})
	if err != nil {
		return err
	}

	baseURL := strings.TrimSuffix(strings.TrimSpace(*serverURL), "/")
	url := fmt.Sprintf("%s/api/v1/environments/%d", baseURL, *environmentID)
	resp, err := authorizedRequest(client, http.MethodGet, url, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Println("Environment status:")
	printIfExists := func(key string, label string) {
		if value, ok := payload[key]; ok && value != nil {
			fmt.Printf("  %s: %v\n", label, value)
		}
	}

	printIfExists("id", "ID")
	printIfExists("name", "Name")
	printIfExists("repo_url", "Repository")
	printIfExists("branch", "Branch")
	printIfExists("deploy_path", "Deploy path")
	printIfExists("provider", "Provider")
	printIfExists("installation_id", "Installation ID")
	printIfExists("app_slug", "App slug")
	printIfExists("repository_id", "Repository ID")
	printIfExists("status", "Status")
	printIfExists("auto_reconcile", "Auto reconcile")

	if webhookRaw, ok := payload["webhook_status"]; ok {
		if ws, ok := webhookRaw.(map[string]interface{}); ok {
			fmt.Println("  Webhook status:")
			if status, ok := ws["status"]; ok {
				fmt.Printf("    Status: %v\n", status)
			}
			if last, ok := ws["last_webhook"]; ok {
				fmt.Printf("    Last webhook: %v\n", last)
			}
			if retries, ok := ws["retry_count"]; ok {
				fmt.Printf("    Retry count: %v\n", retries)
			}
			if errMsg, ok := ws["error"]; ok && errMsg != nil && errMsg != "" {
				fmt.Printf("    Error: %v\n", errMsg)
			}
		}
	}

	return nil
}
