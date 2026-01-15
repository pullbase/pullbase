package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pullbase/pullbase/server/pkg/models"
)

// BootstrapPasswordMinLength enforces the minimum length for bootstrap/admin passwords.
const BootstrapPasswordMinLength = 12

// Claims defines the structure of the JWT claims
type Claims struct {
	UserID        int    `json:"user_id"`
	Username      string `json:"username"`
	Role          string `json:"role"`
	EnvironmentID *int64 `json:"environment_id,omitempty"`
	jwt.RegisteredClaims
}

// Service handles JWT generation and validation
type Service struct {
	jwtSecret []byte
	expiry    time.Duration
}

// NewService creates a new authentication service
func NewService(secret string, expiryHours int) (*Service, error) {
	if secret == "" {
		return nil, errors.New("JWT secret cannot be empty")
	}
	if expiryHours <= 0 {
		expiryHours = 24 // Default expiry to 24 hours
	}
	return &Service{
		jwtSecret: []byte(secret),
		expiry:    time.Duration(expiryHours) * time.Hour,
	}, nil
}

// GenerateToken creates a new JWT for a given user
func (s *Service) GenerateToken(user *models.User) (string, error) {
	return s.GenerateTokenForEnvironment(user, nil)
}

// GenerateTokenForEnvironment creates a new JWT for a given user with optional environment ID for agent authentication
func (s *Service) GenerateTokenForEnvironment(user *models.User, environmentID *int64) (string, error) {
	if user == nil {
		return "", errors.New("cannot generate token for nil user")
	}
	expirationTime := time.Now().Add(s.expiry)
	claims := &Claims{
		UserID:        user.ID,
		Username:      user.Username,
		Role:          user.Role,
		EnvironmentID: environmentID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   fmt.Sprintf("%d", user.ID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken verifies a JWT string and returns the claims if valid
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Check the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("token has expired")
		}
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil

}

// contextKey is a type used for context keys to avoid collisions.
type contextKey string

// UserContextKey is the key used to store user claims in the request context.
const UserContextKey contextKey = "user"

// AuthCookieMiddleware creates a middleware that checks for a valid JWT in a cookie.
func (s *Service) AuthCookieMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
				return
			}
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		tokenString := cookie.Value
		claims, err := s.ValidateToken(tokenString)
		if err != nil {
			http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
			return
		}

		// Token is valid, add claims to context
		ctx := context.WithValue(r.Context(), UserContextKey, claims)

		// Call the next handler with the new context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
