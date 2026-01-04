package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type agentToken struct {
	ID          int        `json:"id"`
	ServerID    string     `json:"server_id"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	IsActive    bool       `json:"is_active"`
}

type createTokenResponse struct {
	agentToken
	Token            string `json:"token"`
	InstallationInfo struct {
		Instructions string `json:"instructions"`
		ExampleCmd   string `json:"example_cmd"`
	} `json:"installation_info"`
}

func runTokensList(args []string) error {
	fs := flag.NewFlagSet("tokens list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	serverID := fs.String("server-id", "", "Server identifier")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*serverID) == "" {
		return errors.New("--server-id is required")
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

	url := strings.TrimSuffix(strings.TrimSpace(*serverURL), "/") + "/api/v1/servers/" + strings.TrimSpace(*serverID) + "/tokens"
	resp, err := authorizedRequest(client, http.MethodGet, url, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	var tokens []agentToken
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if len(tokens) == 0 {
		fmt.Println("No tokens found for server.")
		return nil
	}

	fmt.Println("ID\tDescription\tCreated\tExpires\tLast Used\tActive")
	for _, tok := range tokens {
		expires := "-"
		if tok.ExpiresAt != nil {
			expires = tok.ExpiresAt.Format(time.RFC3339)
		}
		lastUsed := "-"
		if tok.LastUsedAt != nil {
			lastUsed = tok.LastUsedAt.Format(time.RFC3339)
		}
		fmt.Printf("%d\t%s\t%s\t%s\t%s\t%t\n",
			tok.ID,
			tok.Description,
			tok.CreatedAt.Format(time.RFC3339),
			expires,
			lastUsed,
			tok.IsActive,
		)
	}
	return nil
}

func runTokensCreate(args []string) error {
	fs := flag.NewFlagSet("tokens create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	serverID := fs.String("server-id", "", "Server identifier")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")
	description := fs.String("description", "", "Token description")
	expiresIn := fs.Int("expires-in", 0, "Optional expiration in days")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*serverID) == "" {
		return errors.New("--server-id is required")
	}
	if strings.TrimSpace(*description) == "" {
		return errors.New("--description is required")
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

	payload := map[string]interface{}{
		"description": *description,
	}
	if *expiresIn > 0 {
		payload["expires_in"] = *expiresIn
	}

	url := strings.TrimSuffix(strings.TrimSpace(*serverURL), "/") + "/api/v1/servers/" + strings.TrimSpace(*serverID) + "/tokens"
	resp, err := authorizedRequest(client, http.MethodPost, url, token, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return readAPIError(resp)
	}

	var created createTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Println("Token created successfully. Store this value securely:")
	fmt.Println(created.Token)
	if created.InstallationInfo.Instructions != "" {
		fmt.Println()
		fmt.Println("Installation instructions:")
		fmt.Println(created.InstallationInfo.Instructions)
	}
	if created.InstallationInfo.ExampleCmd != "" {
		fmt.Println()
		fmt.Println("Example command:")
		fmt.Println(created.InstallationInfo.ExampleCmd)
	}
	return nil
}

func runTokensRevoke(args []string) error {
	fs := flag.NewFlagSet("tokens revoke", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	serverID := fs.String("server-id", "", "Server identifier")
	tokenID := fs.Int("token-id", 0, "Token ID to revoke")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*serverID) == "" {
		return errors.New("--server-id is required")
	}
	if *tokenID <= 0 {
		return errors.New("--token-id must be greater than zero")
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

	url := fmt.Sprintf("%s/api/v1/servers/%s/tokens/%d",
		strings.TrimSuffix(strings.TrimSpace(*serverURL), "/"),
		strings.TrimSpace(*serverID),
		*tokenID,
	)
	resp, err := authorizedRequest(client, http.MethodDelete, url, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	fmt.Println("Token revoked successfully.")
	return nil
}
