package token

import (
	"strings"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	t.Parallel()
	
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() failed: %v", err)
	}

	if !strings.HasPrefix(token, TokenPrefix) {
		t.Errorf("Token should start with prefix %s, got: %s", TokenPrefix, token)
	}

	expectedMinLength := len(TokenPrefix) + ((TokenLength+2)/3)*4
	if len(token) < expectedMinLength-4 {
		t.Errorf("Token length too short. Expected at least %d, got: %d", expectedMinLength-4, len(token))
	}

	token2, err := GenerateToken()
	if err != nil {
		t.Fatalf("Second GenerateToken() failed: %v", err)
	}

	if token == token2 {
		t.Error("Two generated tokens should be different")
	}
}

func TestHashToken(t *testing.T) {
	t.Parallel()
	
	token := "pbt_test_token_12345"
	hash1 := HashToken(token)
	hash2 := HashToken(token)

	if hash1 != hash2 {
		t.Error("Same token should produce same hash")
	}

	hash3 := HashToken("pbt_different_token")
	if hash1 == hash3 {
		t.Error("Different tokens should produce different hashes")
	}

	if len(hash1) != 64 {
		t.Errorf("Hash length should be 64 characters, got: %d", len(hash1))
	}

	for _, char := range hash1 {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			t.Errorf("Hash contains non-hex character: %c", char)
		}
	}
}

func TestValidateTokenFormat(t *testing.T) {
	t.Parallel()
	
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name:     "valid generated token",
			token:    "",
			expected: true,
		},
		{
			name:     "missing prefix",
			token:    "invalid_token_without_prefix",
			expected: false,
		},
		{
			name:     "wrong prefix",
			token:    "wrong_prefix_token",
			expected: false,
		},
		{
			name:     "empty token",
			token:    "",
			expected: false,
		},
		{
			name:     "only prefix",
			token:    TokenPrefix,
			expected: false,
		},
		{
			name:     "invalid base64 after prefix",
			token:    TokenPrefix + "invalid!!!base64",
			expected: false,
		},
		{
			name:     "correct prefix but wrong length",
			token:    TokenPrefix + "dGVzdA==",
			expected: false,
		},
	}

	// Generate a valid token for the first test
	validToken, err := GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token for test: %v", err)
	}
	tests[0].token = validToken

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			
			result := ValidateTokenFormat(tt.token)
			if result != tt.expected {
				t.Errorf("ValidateTokenFormat(%s) = %v, expected %v", tt.token, result, tt.expected)
			}
		})
	}
}

func TestGenerateTokenDescription(t *testing.T) {
	t.Parallel()
	
	tests := []struct {
		name         string
		serverName   string
		purpose      string
		expectedPart string
	}{
		{
			name:         "with custom purpose",
			serverName:   "web-server-01",
			purpose:      "Production deployment",
			expectedPart: "web-server-01 - Production deployment",
		},
		{
			name:         "with default purpose",
			serverName:   "api-server-02",
			purpose:      "",
			expectedPart: "api-server-02 - Agent authentication",
		},
		{
			name:         "with empty server name",
			serverName:   "",
			purpose:      "Testing",
			expectedPart: " - Testing",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			
			result := GenerateTokenDescription(tt.serverName, tt.purpose)
			if result != tt.expectedPart {
				t.Errorf("GenerateTokenDescription(%s, %s) = %s, expected %s",
					tt.serverName, tt.purpose, result, tt.expectedPart)
			}
		})
	}
}

func TestTokenSecurityProperties(t *testing.T) {
	t.Parallel()
	
	tokens := make(map[string]bool)
	const numTokens = 100

	for i := 0; i < numTokens; i++ {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("Failed to generate token %d: %v", i, err)
		}

		if tokens[token] {
			t.Fatalf("Duplicate token generated: %s", token)
		}
		tokens[token] = true

		if !ValidateTokenFormat(token) {
			t.Errorf("Generated token %s failed validation", token)
		}
	}
}

func TestHashTokenConsistency(t *testing.T) {
	t.Parallel()
	
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	hashes := make(map[string]int)
	for i := 0; i < 100; i++ {
		hash := HashToken(token)
		hashes[hash]++
	}

	if len(hashes) != 1 {
		t.Errorf("Token hashing is not consistent. Got %d different hashes", len(hashes))
	}
}
