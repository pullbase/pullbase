package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/pullbase/pullbase/server/pkg/auth"
	"github.com/pullbase/pullbase/server/pkg/models"
)

type userSummary struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type createUserResponse struct {
	User userSummary `json:"user"`
}

type listUsersResponse struct {
	Users  []userSummary `json:"users"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
	Role   string        `json:"role"`
}

func runUsersList(args []string) error {
	fs := flag.NewFlagSet("users list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")
	limit := fs.Int("limit", 100, "Maximum number of users to return (capped at 500)")
	offset := fs.Int("offset", 0, "Offset for pagination")
	role := fs.String("role", "", "Filter by role (admin|user|viewer)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	if *limit <= 0 {
		return errors.New("--limit must be a positive integer")
	}
	if *limit > 500 {
		*limit = 500
	}
	if *offset < 0 {
		return errors.New("--offset must be zero or a positive integer")
	}

	roleFilter := strings.TrimSpace(*role)
	if roleFilter != "" {
		validRoles := map[string]struct{}{
			models.RoleAdmin:  {},
			models.RoleUser:   {},
			models.RoleViewer: {},
		}
		if _, ok := validRoles[roleFilter]; !ok {
			return errors.New("--role must be one of: admin, user, viewer")
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

	values := url.Values{}
	values.Set("limit", fmt.Sprintf("%d", *limit))
	values.Set("offset", fmt.Sprintf("%d", *offset))
	if roleFilter != "" {
		values.Set("role", roleFilter)
	}

	requestURL := fmt.Sprintf("%s/api/v1/users?%s", targetURL, values.Encode())
	resp, err := authorizedRequest(client, http.MethodGet, requestURL, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}

	var listResp listUsersResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return fmt.Errorf("failed to decode list users response: %w", err)
	}

	if len(listResp.Users) == 0 {
		fmt.Println("No users found.")
		return nil
	}

	fmt.Println("ID\tUsername\tRole")
	for _, u := range listResp.Users {
		fmt.Printf("%d\t%s\t%s\n", u.ID, u.Username, u.Role)
	}
	fmt.Printf("\nTotal: %d\tLimit: %d\tOffset: %d\n", listResp.Total, listResp.Limit, listResp.Offset)
	if listResp.Role != "" {
		fmt.Printf("Role filter: %s\n", listResp.Role)
	}
	return nil
}

func runUsersCreate(args []string) error {
	fs := flag.NewFlagSet("users create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	newUsername := fs.String("new-username", "", "Username for the new user")
	newPassword := fs.String("new-password", "", "Password for the new user")
	newPasswordFile := fs.String("new-password-file", "", "Path to file containing the new user password")
	role := fs.String("role", models.RoleUser, "Role for the new user (admin|user|viewer)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

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

	desiredUsername := strings.TrimSpace(*newUsername)
	if desiredUsername == "" {
		return errors.New("--new-username is required")
	}
	if !bootstrapUsernamePattern.MatchString(desiredUsername) {
		return errors.New("new username must be 3-64 characters and contain only letters, numbers, '.', '_' or '-'")
	}

	passwordValue := *newPassword
	if passwordValue == "" && strings.TrimSpace(*newPasswordFile) != "" {
		loaded, err := loadSensitiveValue(*newPasswordFile)
		if err != nil {
			return fmt.Errorf("failed to read new user password file: %w", err)
		}
		passwordValue = loaded
	}
	if passwordValue == "" {
		return errors.New("--new-password or --new-password-file must be provided")
	}
	if utf8.RuneCountInString(passwordValue) < auth.BootstrapPasswordMinLength {
		return fmt.Errorf("new user password must be at least %d characters long", auth.BootstrapPasswordMinLength)
	}

	roleValue := strings.TrimSpace(*role)
	if roleValue == "" {
		roleValue = models.RoleUser
	}
	validRoles := map[string]struct{}{
		models.RoleAdmin:  {},
		models.RoleUser:   {},
		models.RoleViewer: {},
	}
	if _, ok := validRoles[roleValue]; !ok {
		return errors.New("role must be one of: admin, user, viewer")
	}

	payload := map[string]string{
		"username": desiredUsername,
		"password": passwordValue,
		"role":     roleValue,
	}

	resp, err := authorizedRequest(client, http.MethodPost, targetURL+"/api/v1/users", token, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return readAPIError(resp)
	}

	var createResp createUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return fmt.Errorf("failed to decode create user response: %w", err)
	}

	fmt.Printf("User created: %s (role: %s, id: %d)\n", createResp.User.Username, createResp.User.Role, createResp.User.ID)
	return nil
}

func runUsersDelete(args []string) error {
	fs := flag.NewFlagSet("users delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	// userID of user to delete
	userID := fs.Int("user-id", 0, "User ID of user to delete")
	deleteAcctUsername := fs.String("delete-acct-username", "", "Confirm username for account to delete")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

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

	if *userID <= 0 {
		return errors.New("--user-id must be a positive integer")
	}

	delAcctUsername := strings.TrimSpace(*deleteAcctUsername)
	if delAcctUsername == "" {
		return errors.New("--delete-acct-username is required")
	}
	if !bootstrapUsernamePattern.MatchString(delAcctUsername) {
		return errors.New("username must be 3-64 characters and contain only letters, numbers, '.', '_' or '-'")
	}

	payload := map[string]string{
		"confirm_username": delAcctUsername,
	}

	deleteURL := fmt.Sprintf("%s/api/v1/users/%d", targetURL, *userID)
	resp, err := authorizedRequest(client, http.MethodDelete, deleteURL, token, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return readAPIError(resp)
	}
	
	fmt.Printf("User: %s with id: %d successfully deleted", delAcctUsername, *userID)
	return nil
}
