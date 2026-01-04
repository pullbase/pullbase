package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

type serverResponse struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	EnvironmentID   *int64     `json:"environment_id,omitempty"`
	EnvironmentName *string    `json:"environment_name,omitempty"`
	AutoReconcile   bool       `json:"auto_reconcile"`
	LastStatus      *string    `json:"last_status,omitempty"`
	LastCommitHash  *string    `json:"last_commit_hash,omitempty"`
	LastIsDrifted   *bool      `json:"last_is_drifted,omitempty"`
	LastTimestamp   *time.Time `json:"last_timestamp,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type listServersResponse struct {
	Servers []serverResponse `json:"servers"`
}

type createServerRequest struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	EnvironmentID *int64 `json:"environment_id,omitempty"`
}

type outputFormat string

const (
	outputTable outputFormat = "table"
	outputJSON  outputFormat = "json"
)

func runServersList(args []string) error {
	fs := flag.NewFlagSet("servers list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")
	environmentID := fs.Int64("environment-id", 0, "Filter by environment ID")
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

	values := url.Values{}
	if *environmentID > 0 {
		values.Set("environment_id", fmt.Sprintf("%d", *environmentID))
	}

	requestURL := fmt.Sprintf("%s/api/v1/servers", targetURL)
	if len(values) > 0 {
		requestURL += "?" + values.Encode()
	}

	resp, err := authorizedRequest(client, http.MethodGet, requestURL, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	var listResp listServersResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(listResp.Servers)
	}

	if len(listResp.Servers) == 0 {
		fmt.Println("No servers found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tENVIRONMENT\tSTATUS\tDRIFTED\tAUTO-RECONCILE")
	for _, s := range listResp.Servers {
		envName := "-"
		if s.EnvironmentName != nil {
			envName = *s.EnvironmentName
		}
		status := "-"
		if s.LastStatus != nil {
			status = *s.LastStatus
		}
		drifted := "-"
		if s.LastIsDrifted != nil {
			if *s.LastIsDrifted {
				drifted = "yes"
			} else {
				drifted = "no"
			}
		}
		autoReconcile := "no"
		if s.AutoReconcile {
			autoReconcile = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", s.ID, s.Name, envName, status, drifted, autoReconcile)
	}
	w.Flush()

	return nil
}

func runServersCreate(args []string) error {
	fs := flag.NewFlagSet("servers create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	serverID := fs.String("id", "", "Server ID (unique identifier)")
	serverName := fs.String("name", "", "Server name")
	environmentID := fs.Int64("environment-id", 0, "Environment ID to associate with")
	output := fs.String("output", "table", "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if strings.TrimSpace(*serverID) == "" {
		return errors.New("--id is required")
	}
	if strings.TrimSpace(*serverName) == "" {
		return errors.New("--name is required")
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

	payload := createServerRequest{
		ID:   strings.TrimSpace(*serverID),
		Name: strings.TrimSpace(*serverName),
	}
	if *environmentID > 0 {
		payload.EnvironmentID = environmentID
	}

	resp, err := authorizedRequest(client, http.MethodPost, targetURL+"/api/v1/servers", token, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return readAPIError(resp)
	}

	var server serverResponse
	if err := json.NewDecoder(resp.Body).Decode(&server); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(server)
	}

	fmt.Printf("Server created successfully.\n")
	fmt.Printf("  ID:            %s\n", server.ID)
	fmt.Printf("  Name:          %s\n", server.Name)
	if server.EnvironmentID != nil {
		fmt.Printf("  Environment:   %d\n", *server.EnvironmentID)
	}
	fmt.Printf("  Auto-Reconcile: %v\n", server.AutoReconcile)

	return nil
}

func runServersGet(args []string) error {
	fs := flag.NewFlagSet("servers get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	serverID := fs.String("id", "", "Server ID to retrieve")
	output := fs.String("output", "table", "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if strings.TrimSpace(*serverID) == "" {
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

	requestURL := fmt.Sprintf("%s/api/v1/servers/%s", targetURL, url.PathEscape(*serverID))
	resp, err := authorizedRequest(client, http.MethodGet, requestURL, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	var server serverResponse
	if err := json.NewDecoder(resp.Body).Decode(&server); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(server)
	}

	fmt.Printf("Server: %s\n", server.ID)
	fmt.Printf("  Name:           %s\n", server.Name)
	if server.EnvironmentName != nil {
		fmt.Printf("  Environment:    %s\n", *server.EnvironmentName)
	} else if server.EnvironmentID != nil {
		fmt.Printf("  Environment ID: %d\n", *server.EnvironmentID)
	} else {
		fmt.Printf("  Environment:    (none)\n")
	}
	fmt.Printf("  Auto-Reconcile: %v\n", server.AutoReconcile)
	if server.LastStatus != nil {
		fmt.Printf("  Status:         %s\n", *server.LastStatus)
	}
	if server.LastCommitHash != nil {
		fmt.Printf("  Commit:         %s\n", *server.LastCommitHash)
	}
	if server.LastIsDrifted != nil {
		drifted := "no"
		if *server.LastIsDrifted {
			drifted = "yes"
		}
		fmt.Printf("  Drifted:        %s\n", drifted)
	}
	if server.LastTimestamp != nil {
		fmt.Printf("  Last Seen:      %s\n", server.LastTimestamp.Format(time.RFC3339))
	}
	fmt.Printf("  Created:        %s\n", server.CreatedAt.Format(time.RFC3339))

	return nil
}

func runServersDelete(args []string) error {
	fs := flag.NewFlagSet("servers delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	serverID := fs.String("id", "", "Server ID to delete")
	force := fs.Bool("force", false, "Skip confirmation prompt")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if strings.TrimSpace(*serverID) == "" {
		return errors.New("--id is required")
	}

	if !*force {
		fmt.Printf("Are you sure you want to delete server '%s'? [y/N]: ", *serverID)
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

	requestURL := fmt.Sprintf("%s/api/v1/servers/%s", targetURL, url.PathEscape(*serverID))
	resp, err := authorizedRequest(client, http.MethodDelete, requestURL, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	fmt.Printf("Server '%s' deleted successfully.\n", *serverID)
	return nil
}

func runServersInstallScript(args []string) error {
	fs := flag.NewFlagSet("servers install-script", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	serverID := fs.String("id", "", "Server ID")
	agentToken := fs.String("token", "", "Agent token for authentication")
	version := fs.String("version", "latest", "Agent version to install")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if strings.TrimSpace(*serverID) == "" {
		return errors.New("--id is required")
	}
	if strings.TrimSpace(*agentToken) == "" {
		return errors.New("--token is required (agent token for the server)")
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

	values := url.Values{}
	values.Set("token", *agentToken)
	if *version != "" {
		values.Set("version", *version)
	}

	requestURL := fmt.Sprintf("%s/api/v1/servers/%s/install-script?%s", targetURL, url.PathEscape(*serverID), values.Encode())
	resp, err := authorizedRequest(client, http.MethodGet, requestURL, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	script, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read install script: %w", err)
	}

	fmt.Print(string(script))
	return nil
}
