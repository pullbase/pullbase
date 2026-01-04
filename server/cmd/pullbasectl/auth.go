package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

type bootstrapAdminResponse struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	} `json:"user"`
}

type adminAuthConfig struct {
	ServerURL    string
	AdminToken   string
	Username     string
	Password     string
	PasswordFile string
	CACertPath   string
	Insecure     bool
}

func resolveAdminCredentials(cfg adminAuthConfig) (*http.Client, string, error) {
	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		return nil, "", errors.New("server URL is required")
	}

	client, err := newHTTPClient(strings.TrimSpace(cfg.CACertPath), cfg.Insecure)
	if err != nil {
		return nil, "", err
	}

	token := strings.TrimSpace(cfg.AdminToken)
	if token != "" {
		return client, token, nil
	}

	username := strings.TrimSpace(cfg.Username)
	if username == "" {
		return nil, "", errors.New("admin token or --username/--password required")
	}

	password := cfg.Password
	if password == "" && strings.TrimSpace(cfg.PasswordFile) != "" {
		loaded, err := loadSensitiveValue(cfg.PasswordFile)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read password file: %w", err)
		}
		password = loaded
	}
	if password == "" {
		return nil, "", errors.New("--password or --password-file must be provided when using --username")
	}

	loginResp, err := loginAdmin(serverURL, username, password, client)
	password = ""
	if err != nil {
		return nil, "", err
	}

	return client, loginResp.AccessToken, nil
}

func authorizedRequest(client *http.Client, method, url, token string, payload interface{}) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request payload: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

func readAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func runAuthBootstrapAdmin(args []string) error {
	fs := flag.NewFlagSet("auth bootstrap-admin", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL (e.g. https://pullbase.local)")
	bootstrapSecret := fs.String("bootstrap-secret", "", "Bootstrap secret value")
	bootstrapSecretFile := fs.String("bootstrap-secret-file", "", "Path to file containing the bootstrap secret")
	username := fs.String("username", "", "Username for the admin account")
	password := fs.String("password", "", "Password for the admin account")
	passwordFile := fs.String("password-file", "", "Path to file containing the admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS certificate verification (not recommended)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	url := strings.TrimSpace(*serverURL)
	if url == "" {
		return errors.New("--server-url is required")
	}
	url = strings.TrimSuffix(url, "/")

	secret := strings.TrimSpace(*bootstrapSecret)
	if secret == "" && strings.TrimSpace(*bootstrapSecretFile) != "" {
		loadedSecret, err := loadSensitiveValue(*bootstrapSecretFile)
		if err != nil {
			return fmt.Errorf("failed to read bootstrap secret file: %w", err)
		}
		secret = loadedSecret
	}
	if secret == "" {
		return errors.New("--bootstrap-secret or --bootstrap-secret-file must be provided")
	}

	user := strings.TrimSpace(*username)
	if user == "" {
		return errors.New("--username is required")
	}
	if !bootstrapUsernamePattern.MatchString(user) {
		return errors.New("username must be 3-64 characters and contain only letters, numbers, '.', '_' or '-'")
	}

	pass := *password
	if pass == "" && strings.TrimSpace(*passwordFile) != "" {
		loadedPassword, err := loadSensitiveValue(*passwordFile)
		if err != nil {
			return fmt.Errorf("failed to read password file: %w", err)
		}
		pass = loadedPassword
	}
	if pass == "" {
		return errors.New("--password or --password-file must be provided")
	}
	if utf8.RuneCountInString(pass) < bootstrapPasswordMinLength {
		return fmt.Errorf("password must be at least %d characters long", bootstrapPasswordMinLength)
	}

	client, err := newHTTPClient(strings.TrimSpace(*caCertPath), *insecureSkipVerify)
	if err != nil {
		return err
	}

	bootstrapResp, _, err := bootstrapAdmin(url, secret, user, pass, client)
	if err != nil {
		return err
	}

	fmt.Println("Admin bootstrap completed successfully.")
	fmt.Printf("Username: %s\n", bootstrapResp.User.Username)
	fmt.Println("Access token (store securely):")
	fmt.Println(bootstrapResp.AccessToken)

	pass = ""
	secret = ""
	return nil
}

func runAuthLogin(args []string) error {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL (e.g. https://pullbase.local)")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing the admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS certificate verification (not recommended)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	url := strings.TrimSpace(*serverURL)
	if url == "" {
		return errors.New("--server-url is required")
	}

	user := strings.TrimSpace(*username)
	if user == "" {
		return errors.New("--username is required")
	}

	pass := *password
	if pass == "" && strings.TrimSpace(*passwordFile) != "" {
		loaded, err := loadSensitiveValue(*passwordFile)
		if err != nil {
			return fmt.Errorf("failed to read password file: %w", err)
		}
		pass = loaded
	}
	if pass == "" {
		return errors.New("--password or --password-file must be provided")
	}

	client, err := newHTTPClient(strings.TrimSpace(*caCertPath), *insecureSkipVerify)
	if err != nil {
		return err
	}

	loginResp, err := loginAdmin(url, user, pass, client)
	if err != nil {
		return err
	}

	fmt.Println("Login successful. JWT token:")
	fmt.Println(loginResp.AccessToken)

	pass = ""
	return nil
}

func loadSensitiveValue(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return "", fmt.Errorf("file %s must not be group- or world-accessible (current permissions: %o)", path, perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func buildHTTPClient(caCertPath string, insecureSkipVerify bool) (*http.Client, error) {
	transport := http.DefaultTransport

	if caCertPath != "" || insecureSkipVerify {
		tlsConfig := &tls.Config{InsecureSkipVerify: insecureSkipVerify}

		if strings.TrimSpace(caCertPath) != "" {
			pem, err := os.ReadFile(strings.TrimSpace(caCertPath))
			if err != nil {
				return nil, fmt.Errorf("failed to read CA certificate file %s: %w", caCertPath, err)
			}
			rootCAs, err := x509.SystemCertPool()
			if err != nil || rootCAs == nil {
				rootCAs = x509.NewCertPool()
			}
			if !rootCAs.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("no valid certificates found in %s", caCertPath)
			}
			tlsConfig.RootCAs = rootCAs
		}

		transport = &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: tlsConfig,
		}
	}

	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}, nil
}

func bootstrapAdmin(serverURL, secret, username, password string, client *http.Client) (*bootstrapAdminResponse, int, error) {
	serverURL = strings.TrimSuffix(serverURL, "/")
	payload := map[string]string{
		"bootstrap_secret": secret,
		"username":         username,
		"password":         password,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal bootstrap payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/v1/bootstrap/admin", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create bootstrap request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("bootstrap request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read bootstrap response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, resp.StatusCode, fmt.Errorf("bootstrap failed: %s", strings.TrimSpace(string(respBody)))
	}

	var result bootstrapAdminResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to parse bootstrap response: %w", err)
	}
	return &result, resp.StatusCode, nil
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	} `json:"user"`
}

func loginAdmin(serverURL, username, password string, client *http.Client) (*loginResponse, error) {
	serverURL = strings.TrimSuffix(serverURL, "/")
	body, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal login payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login failed: %s", strings.TrimSpace(string(respBody)))
	}

	var result loginResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse login response: %w", err)
	}
	return &result, nil
}
