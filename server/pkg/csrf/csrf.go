package csrf

import (
	_ "crypto/rand"
	_ "encoding/base64"
	"errors"
	_ "net/http"
	"sync"
	"time"
)

// const (
// 	tokenLength   = 32
// 	cookieName    = "csrf_token"
// 	formFieldName = "csrf_token"
// 	cookieMaxAge  = 3600 // 1 hour
// )

var (
	ErrTokenMismatch = errors.New("csrf token mismatch")
	ErrNoToken       = errors.New("no csrf token present")
)

// Manager handles CSRF token generation and validation
type Manager struct {
	tokens sync.Map // Store valid tokens with expiry
}

type tokenInfo struct {
	// token     string
	expiresAt time.Time
}

// NewManager creates a new CSRF token manager
func NewManager() *Manager {
	manager := &Manager{}
	// Start a goroutine to clean expired tokens periodically
	go manager.cleanupExpiredTokens()
	return manager
}

// // GenerateToken creates a new CSRF token and sets it in a cookie
// func (m *Manager) GenerateToken(w http.ResponseWriter) (string, error) {
// 	// Generate random bytes
// 	b := make([]byte, tokenLength)
// 	_, err := rand.Read(b)
// 	if err != nil {
// 		return "", err
// 	}

// 	// Encode to base64
// 	token := base64.URLEncoding.EncodeToString(b)

// 	// Store token with expiry
// 	m.tokens.Store(token, tokenInfo{
// 		token:     token,
// 		expiresAt: time.Now().Add(time.Duration(cookieMaxAge) * time.Second),
// 	})

// 	// Set cookie
// 	cookie := &http.Cookie{
// 		Name:     cookieName,
// 		Value:    token,
// 		Path:     "/",
// 		MaxAge:   cookieMaxAge,
// 		HttpOnly: true,
// 		Secure:   true,
// 		SameSite: http.SameSiteStrictMode,
// 	}
// 	http.SetCookie(w, cookie)

// 	return token, nil
// }

// // ValidateToken checks if the provided token is valid
// func (m *Manager) ValidateToken(r *http.Request) error {
// 	// Get token from form
// 	formToken := r.FormValue(formFieldName)
// 	if formToken == "" {
// 		return ErrNoToken
// 	}

// 	// Get token from cookie
// 	cookie, err := r.Cookie(cookieName)
// 	if err != nil {
// 		return ErrNoToken
// 	}

// 	// Compare tokens
// 	if formToken != cookie.Value {
// 		return ErrTokenMismatch
// 	}

// 	// Check if token exists and is valid
// 	if info, ok := m.tokens.Load(formToken); ok {
// 		tokenInfo := info.(tokenInfo)
// 		if time.Now().After(tokenInfo.expiresAt) {
// 			m.tokens.Delete(formToken)
// 			return ErrTokenMismatch
// 		}
// 		return nil
// 	}

// 	return ErrTokenMismatch
// }

// cleanupExpiredTokens removes expired tokens periodically
func (m *Manager) cleanupExpiredTokens() {
	ticker := time.NewTicker(time.Hour)
	for range ticker.C {
		now := time.Now()
		m.tokens.Range(func(key, value interface{}) bool {
			info := value.(tokenInfo)
			if now.After(info.expiresAt) {
				m.tokens.Delete(key)
			}
			return true
		})
	}
}
