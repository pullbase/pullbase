package githubapp

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultAPIBaseURL = "https://api.github.com"
)

// Config defines the options required to create a GitHub App client.
type Config struct {
	AppID          int64
	PrivateKeyPEM  []byte
	APIBaseURL     string
	HTTPClient     *http.Client
	TokenTTLBuffer time.Duration
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// Client handles GitHub App authentication flows and token caching.
type Client struct {
	appID      int64
	privateKey *rsa.PrivateKey
	apiBaseURL string
	httpClient *http.Client

	mu      sync.Mutex
	cache   map[int64]cachedToken
	buffer  time.Duration
	nowFunc func() time.Time
}

// NewClient creates a new GitHub App client from configuration.
func NewClient(cfg Config) (*Client, error) {
	if cfg.AppID == 0 {
		return nil, fmt.Errorf("github app id is required")
	}
	if len(cfg.PrivateKeyPEM) == 0 {
		return nil, fmt.Errorf("github app private key PEM is required")
	}

	privateKey, err := parsePrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}

	apiBase := cfg.APIBaseURL
	if apiBase == "" {
		apiBase = defaultAPIBaseURL
	}
	apiBase = strings.TrimSuffix(apiBase, "/")

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	buffer := cfg.TokenTTLBuffer
	if buffer == 0 {
		buffer = 2 * time.Minute
	}

	return &Client{
		appID:      cfg.AppID,
		privateKey: privateKey,
		apiBaseURL: apiBase,
		httpClient: httpClient,
		cache:      make(map[int64]cachedToken),
		buffer:     buffer,
		nowFunc:    time.Now,
	}, nil
}

// GetInstallationToken returns an installation token for the provided installation ID.
// The token is cached until shortly before expiration.
func (c *Client) GetInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error) {
	if installationID <= 0 {
		return "", time.Time{}, fmt.Errorf("installation id must be positive")
	}

	if token, expiry, ok := c.cachedToken(installationID); ok {
		return token, expiry, nil
	}

	appJWT, err := c.generateAppJWT()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate app jwt: %w", err)
	}

	requestBody := map[string]any{}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to marshal installation token request: %w", err)
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", c.apiBaseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create installation token request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Pullbase-GitHubApp-Client")
	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("installation token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", time.Time{}, fmt.Errorf("unexpected status code from GitHub: %d", resp.StatusCode)
	}

	var tokenResponse struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode installation token response: %w", err)
	}

	if tokenResponse.Token == "" || tokenResponse.ExpiresAt.IsZero() {
		return "", time.Time{}, fmt.Errorf("installation token response missing token or expiry")
	}

	c.storeToken(installationID, tokenResponse.Token, tokenResponse.ExpiresAt)
	return tokenResponse.Token, tokenResponse.ExpiresAt, nil
}

func (c *Client) cachedToken(installationID int64) (string, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.cache[installationID]; ok {
		if c.nowFunc().Add(c.buffer).Before(entry.expiresAt) {
			return entry.token, entry.expiresAt, true
		}
		delete(c.cache, installationID)
	}
	return "", time.Time{}, false
}

func (c *Client) storeToken(installationID int64, token string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[installationID] = cachedToken{
		token:     token,
		expiresAt: expiresAt,
	}
}

func (c *Client) generateAppJWT() (string, error) {
	now := c.nowFunc()
	claims := jwt.RegisteredClaims{
		Issuer:    fmt.Sprintf("%d", c.appID),
		IssuedAt:  jwt.NewNumericDate(now.Add(-1 * time.Minute)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(c.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign app jwt: %w", err)
	}

	return signed, nil
}

func parsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing private key")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return key, nil
	}

	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}

	return rsaKey, nil
}
