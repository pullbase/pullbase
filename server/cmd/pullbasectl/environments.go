package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"
)

type environmentResponse struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	RepoURL        string     `json:"repo_url"`
	Branch         string     `json:"branch"`
	DeployPath     string     `json:"deploy_path,omitempty"`
	Provider       string     `json:"provider"`
	InstallationID int64      `json:"installation_id"`
	Status         string     `json:"status"`
	AutoReconcile  bool       `json:"auto_reconcile"`
	DeployedCommit *string    `json:"deployed_commit,omitempty"`
	LastWebhookAt  *time.Time `json:"last_webhook_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type listEnvironmentsResponse struct {
	Environments []environmentResponse `json:"environments"`
}

type createEnvironmentRequest struct {
	Name           string `json:"name"`
	RepoURL        string `json:"repo_url"`
	Branch         string `json:"branch"`
	DeployPath     string `json:"deploy_path,omitempty"`
	InstallationID int64  `json:"installation_id,omitempty"`
}

type rollbackRequest struct {
	TargetCommit string `json:"target_commit"`
	Reason       string `json:"reason,omitempty"`
}

type rollbackResponse struct {
	ID            int64     `json:"id"`
	EnvironmentID int64     `json:"environment_id"`
	FromCommit    string    `json:"from_commit"`
	ToCommit      string    `json:"to_commit"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

type listRollbacksResponse struct {
	Rollbacks []rollbackResponse `json:"rollbacks"`
}

func runEnvironmentsList(args []string) error {
	fs := flag.NewFlagSet("environments list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")
	output := fs.String("output", "table", "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	format := outputFormat(strings.ToLower(*output))
	if format != outputTable && format != outputJSON {
		return errors.New("--output must be 'table' or 'json'")
	}

	client, token, err := resolveAdminCredentials(adminAuthConfig{
		ServerURL:    targetURL,
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

	resp, err := authorizedRequest(client, http.MethodGet, targetURL+"/api/v1/environments", token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	var listResp listEnvironmentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(listResp.Environments)
	}

	if len(listResp.Environments) == 0 {
		fmt.Println("No environments found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tREPO\tBRANCH\tSTATUS\tAUTO-RECONCILE")
	for _, e := range listResp.Environments {
		repoShort := e.RepoURL
		if len(repoShort) > 40 {
			repoShort = repoShort[:37] + "..."
		}
		autoReconcile := "no"
		if e.AutoReconcile {
			autoReconcile = "yes"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", e.ID, e.Name, repoShort, e.Branch, e.Status, autoReconcile)
	}
	w.Flush()

	return nil
}

func runEnvironmentsCreate(args []string) error {
	fs := flag.NewFlagSet("environments create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	name := fs.String("name", "", "Environment name")
	repoURL := fs.String("repo-url", "", "Git repository URL")
	branch := fs.String("branch", "main", "Git branch to track")
	deployPath := fs.String("deploy-path", "", "Path within repo for config files")
	installationID := fs.Int64("installation-id", 0, "GitHub App installation ID")
	output := fs.String("output", "table", "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if strings.TrimSpace(*name) == "" {
		return errors.New("--name is required")
	}
	if strings.TrimSpace(*repoURL) == "" {
		return errors.New("--repo-url is required")
	}

	format := outputFormat(strings.ToLower(*output))
	if format != outputTable && format != outputJSON {
		return errors.New("--output must be 'table' or 'json'")
	}

	client, token, err := resolveAdminCredentials(adminAuthConfig{
		ServerURL:    targetURL,
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

	payload := createEnvironmentRequest{
		Name:           strings.TrimSpace(*name),
		RepoURL:        strings.TrimSpace(*repoURL),
		Branch:         strings.TrimSpace(*branch),
		DeployPath:     strings.TrimSpace(*deployPath),
		InstallationID: *installationID,
	}

	resp, err := authorizedRequest(client, http.MethodPost, targetURL+"/api/v1/environments", token, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return readAPIError(resp)
	}

	var env environmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	}

	fmt.Printf("Environment created successfully.\n")
	fmt.Printf("  ID:            %d\n", env.ID)
	fmt.Printf("  Name:          %s\n", env.Name)
	fmt.Printf("  Repository:    %s\n", env.RepoURL)
	fmt.Printf("  Branch:        %s\n", env.Branch)
	if env.DeployPath != "" {
		fmt.Printf("  Deploy Path:   %s\n", env.DeployPath)
	}
	fmt.Printf("  Status:        %s\n", env.Status)
	fmt.Printf("  Auto-Reconcile: %v\n", env.AutoReconcile)

	return nil
}

func runEnvironmentsGet(args []string) error {
	fs := flag.NewFlagSet("environments get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	envID := fs.Int64("id", 0, "Environment ID to retrieve")
	output := fs.String("output", "table", "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if *envID <= 0 {
		return errors.New("--id is required")
	}

	format := outputFormat(strings.ToLower(*output))
	if format != outputTable && format != outputJSON {
		return errors.New("--output must be 'table' or 'json'")
	}

	client, token, err := resolveAdminCredentials(adminAuthConfig{
		ServerURL:    targetURL,
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

	requestURL := fmt.Sprintf("%s/api/v1/environments/%d", targetURL, *envID)
	resp, err := authorizedRequest(client, http.MethodGet, requestURL, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	var env environmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	}

	fmt.Printf("Environment: %s (ID: %d)\n", env.Name, env.ID)
	fmt.Printf("  Repository:     %s\n", env.RepoURL)
	fmt.Printf("  Branch:         %s\n", env.Branch)
	if env.DeployPath != "" {
		fmt.Printf("  Deploy Path:    %s\n", env.DeployPath)
	}
	fmt.Printf("  Provider:       %s\n", env.Provider)
	fmt.Printf("  Installation:   %d\n", env.InstallationID)
	fmt.Printf("  Status:         %s\n", env.Status)
	fmt.Printf("  Auto-Reconcile: %v\n", env.AutoReconcile)
	if env.DeployedCommit != nil {
		fmt.Printf("  Deployed:       %s\n", *env.DeployedCommit)
	}
	if env.LastWebhookAt != nil {
		fmt.Printf("  Last Webhook:   %s\n", env.LastWebhookAt.Format(time.RFC3339))
	}
	fmt.Printf("  Created:        %s\n", env.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  Updated:        %s\n", env.UpdatedAt.Format(time.RFC3339))

	return nil
}

func runEnvironmentsDelete(args []string) error {
	fs := flag.NewFlagSet("environments delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	envID := fs.Int64("id", 0, "Environment ID to delete")
	force := fs.Bool("force", false, "Skip confirmation prompt")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if *envID <= 0 {
		return errors.New("--id is required")
	}

	if !*force {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("--force is required for non-interactive usage")
		}
		fmt.Printf("Are you sure you want to delete environment %d? This will affect all associated servers. [y/N]: ", *envID)
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	client, token, err := resolveAdminCredentials(adminAuthConfig{
		ServerURL:    targetURL,
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

	requestURL := fmt.Sprintf("%s/api/v1/environments/%d", targetURL, *envID)
	resp, err := authorizedRequest(client, http.MethodDelete, requestURL, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	fmt.Printf("Environment %d deleted successfully.\n", *envID)
	return nil
}

func runEnvironmentsRollback(args []string) error {
	fs := flag.NewFlagSet("environments rollback", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	envID := fs.Int64("id", 0, "Environment ID")
	commit := fs.String("commit", "", "Target commit hash to rollback to")
	reason := fs.String("reason", "", "Reason for the rollback")
	output := fs.String("output", "table", "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if *envID <= 0 {
		return errors.New("--id is required")
	}
	if strings.TrimSpace(*commit) == "" {
		return errors.New("--commit is required")
	}

	format := outputFormat(strings.ToLower(*output))
	if format != outputTable && format != outputJSON {
		return errors.New("--output must be 'table' or 'json'")
	}

	client, token, err := resolveAdminCredentials(adminAuthConfig{
		ServerURL:    targetURL,
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

	payload := rollbackRequest{
		TargetCommit: strings.TrimSpace(*commit),
		Reason:       strings.TrimSpace(*reason),
	}

	requestURL := fmt.Sprintf("%s/api/v1/environments/%d/rollback", targetURL, *envID)
	resp, err := authorizedRequest(client, http.MethodPost, requestURL, token, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return readAPIError(resp)
	}

	var rollback rollbackResponse
	if err := json.NewDecoder(resp.Body).Decode(&rollback); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rollback)
	}

	fmt.Printf("Rollback initiated successfully.\n")
	fmt.Printf("  ID:          %d\n", rollback.ID)
	fmt.Printf("  Environment: %d\n", rollback.EnvironmentID)
	fmt.Printf("  From:        %s\n", rollback.FromCommit)
	fmt.Printf("  To:          %s\n", rollback.ToCommit)
	fmt.Printf("  Status:      %s\n", rollback.Status)
	if rollback.Reason != "" {
		fmt.Printf("  Reason:      %s\n", rollback.Reason)
	}

	return nil
}

func runEnvironmentsRollbackList(args []string) error {
	fs := flag.NewFlagSet("environments rollback-list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	envID := fs.Int64("id", 0, "Environment ID")
	output := fs.String("output", "table", "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if *envID <= 0 {
		return errors.New("--id is required")
	}

	format := outputFormat(strings.ToLower(*output))
	if format != outputTable && format != outputJSON {
		return errors.New("--output must be 'table' or 'json'")
	}

	client, token, err := resolveAdminCredentials(adminAuthConfig{
		ServerURL:    targetURL,
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

	requestURL := fmt.Sprintf("%s/api/v1/environments/%d/rollbacks", targetURL, *envID)
	resp, err := authorizedRequest(client, http.MethodGet, requestURL, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	var listResp listRollbacksResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(listResp.Rollbacks)
	}

	if len(listResp.Rollbacks) == 0 {
		fmt.Println("No rollbacks found for this environment.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tFROM\tTO\tSTATUS\tCREATED")
	for _, r := range listResp.Rollbacks {
		fromShort := r.FromCommit
		if len(fromShort) > 7 {
			fromShort = fromShort[:7]
		}
		toShort := r.ToCommit
		if len(toShort) > 7 {
			toShort = toShort[:7]
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", r.ID, fromShort, toShort, r.Status, r.CreatedAt.Format(time.RFC3339))
	}
	w.Flush()

	return nil
}
