package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	TokenLength = 32

	TokenPrefix = "pbt_" // pullbase token
)

// GenerateToken creates a cryptographically secure random token
func GenerateToken() (string, error) {
	bytes := make([]byte, TokenLength)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	token := TokenPrefix + base64.URLEncoding.EncodeToString(bytes)
	return token, nil
}

// HashToken creates a SHA-256 hash of the token for secure storage
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", hash)
}

// ValidateTokenFormat checks if a token has the correct format
func ValidateTokenFormat(token string) bool {
	if !strings.HasPrefix(token, TokenPrefix) {
		return false
	}

	tokenBody := strings.TrimPrefix(token, TokenPrefix)

	decoded, err := base64.URLEncoding.DecodeString(tokenBody)
	if err != nil {
		return false
	}

	return len(decoded) == TokenLength
}

// ExtractServerIDFromDescription extracts server information from token description
// This is a helper function for generating descriptive token descriptions
func GenerateTokenDescription(serverName, purpose string) string {
	if purpose == "" {
		purpose = "Agent authentication"
	}
	return fmt.Sprintf("%s - %s", serverName, purpose)
}
