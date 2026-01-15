//go:build integration
// +build integration

package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/pullbase/pullbase/server/pkg/auth"
	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/models"
	server "github.com/pullbase/pullbase/server/pkg/server"
	"github.com/pullbase/pullbase/server/pkg/testutil"
	"github.com/pullbase/pullbase/server/pkg/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type testServer struct {
	Server  *httptest.Server
	API     *server.API
	Repo    *database.Repository
	Users   map[string]*models.User // Map role to user model
	Tokens  map[string]string       // Map role to JWT token
	Router  chi.Router              // Store the router
	Cleanup func()
}

// setupTestServer initializes a test database, API, and HTTP test server.
func setupTestServer(t *testing.T) *testServer {
	t.Helper()
	ctx := context.Background()

	tdb := testutil.SetupTestDB(t)
	repo := tdb.Repository()

	testUsers := map[string]*models.User{
		"admin":  {Username: "testadmin", PasswordHash: "password123", Role: models.RoleAdmin},
		"user":   {Username: "testuser", PasswordHash: "password123", Role: models.RoleUser},
		"agent":  {Username: "testagent", PasswordHash: "password123", Role: "agent"},
		"viewer": {Username: "testviewer", PasswordHash: "password123", Role: models.RoleViewer},
	}

	for role, user := range testUsers {
		hashedPasswordBytes, hashErr := bcrypt.GenerateFromPassword([]byte(user.PasswordHash), bcrypt.DefaultCost)
		if hashErr != nil {
			t.Fatalf("Failed to hash password for user %s: %v", role, hashErr)
		}

		createUserErr := repo.CreateUser(ctx, user.Username, string(hashedPasswordBytes), user.Role)
		if createUserErr != nil && !errors.Is(createUserErr, database.ErrConflict) {
			t.Fatalf("Failed to create test user '%s': %v", role, createUserErr)
		}

		createdUser, getUserErr := repo.GetUser(ctx, user.Username)
		if getUserErr != nil {
			t.Fatalf("Failed to fetch created test user '%s': %v", role, getUserErr)
		}
		testUsers[role] = createdUser
	}

	// Create test environments
	testEnvs := map[string]struct {
		repoURL string
		branch  string
	}{
		"development": {"https://github.com/test/dev-configs.git", "main"},
		"staging":     {"https://github.com/test/staging-configs.git", "main"},
		"production":  {"https://github.com/test/prod-configs.git", "main"},
	}

	for envName, envConfig := range testEnvs {
		env, createEnvErr := repo.CreateEnvironment(ctx, envName, envConfig.repoURL, envConfig.branch, false)
		if createEnvErr != nil && !errors.Is(createEnvErr, database.ErrConflict) {
			t.Fatalf("Failed to create test environment '%s': %v", envName, createEnvErr)
		}
		_ = env
	}

	authService, err := auth.NewService("test-secret-key", 1)
	if err != nil {
		t.Fatalf("Failed to create auth service: %v", err)
	}

	api := &server.API{
		Repo: repo,
		Auth: authService,
	}

	corsConfig := server.CORSConfig{
		AllowedOrigins: []string{},
		IsDevelopment:  true,
	}
	r := server.SetupRoutes(api, authService, corsConfig)

	ts := httptest.NewServer(r)

	// Generate tokens for test users
	tokens := make(map[string]string)
	for role, user := range testUsers {
		token, tokenErr := authService.GenerateToken(user)
		if tokenErr != nil {
			ts.Close()
			t.Fatalf("Failed to generate token for user %s: %v", role, tokenErr)
		}
		tokens[role] = token
	}

	return &testServer{
		Server: ts,
		API:    api,
		Repo:   repo,
		Users:  testUsers,
		Tokens: tokens,
		Router: r,
		Cleanup: func() {
			ts.Close()
		},
	}
}

func TestLoginHandler(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	testUsername := "testloginuser_handlers"
	testPassword := "goodpassword"
	err := ts.Repo.CreateUser(ctx, testUsername, testPassword, models.RoleUser)
	if err != nil {
		t.Fatalf("Setup failed: Could not create test user: %v", err)
	}

	t.Run("Successful Login", func(t *testing.T) {
		body := strings.NewReader(fmt.Sprintf(`{"username": "%s", "password": "%s"}`, testUsername, testPassword))
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK (200), got %d", resp.StatusCode)
		}

		var loginResp server.LoginResponse
		if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
			t.Fatalf("Failed to decode response body: %v", err)
		}

		if loginResp.AccessToken == "" {
			t.Error("Expected access token, got empty string")
		}
		if loginResp.User.Username != testUsername {
			t.Errorf("Expected user %s, got %s", testUsername, loginResp.User.Username)
		}
		if loginResp.User.Role != models.RoleUser {
			t.Errorf("Expected role %s, got %s", models.RoleUser, loginResp.User.Role)
		}
	})

	t.Run("Incorrect Password", func(t *testing.T) {
		body := strings.NewReader(fmt.Sprintf(`{"username": "%s", "password": "%s"}`, testUsername, "wrongpassword"))
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized (401), got %d", resp.StatusCode)
		}
	})

	t.Run("Non-existent User", func(t *testing.T) {
		body := strings.NewReader(fmt.Sprintf(`{"username": "%s", "password": "%s"}`, "nosuchuser", "anypassword"))
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized (401), got %d", resp.StatusCode)
		}
	})

	t.Run("Missing Username", func(t *testing.T) {
		body := strings.NewReader(fmt.Sprintf(`{"password": "%s"}`, testPassword))
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status Bad Request (400), got %d", resp.StatusCode)
		}
	})

	t.Run("Missing Password", func(t *testing.T) {
		body := strings.NewReader(fmt.Sprintf(`{"username": "%s"}`, testUsername))
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status Bad Request (400), got %d", resp.StatusCode)
		}
	})

	t.Run("Malformed JSON Body", func(t *testing.T) {
		body := strings.NewReader(`{"username": "test", "password": "pass"`)
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status Bad Request (400), got %d", resp.StatusCode)
		}
	})

	// Test environment-scoped authentication
	t.Run("Environment-Scoped Authentication", func(t *testing.T) {
		// Get development environment by listing and finding
		envs, _, err := ts.Repo.ListEnvironments(ctx, 100, 0)
		require.NoError(t, err, "Failed to list environments")

		var devEnv *models.Environment
		for _, env := range envs {
			if env.Name == "development" {
				devEnv = env
				break
			}
		}
		require.NotNil(t, devEnv, "Failed to find development environment")

		testServer, err := ts.Repo.CreateServer(ctx, "Test Server", devEnv.ID)
		require.NoError(t, err, "Failed to create test server")

		// Assign server to environment using direct SQL
		dbConn := ts.Repo.DB
		_, err = dbConn.ExecContext(ctx, "UPDATE servers SET environment_id = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2", devEnv.ID, testServer.ID)
		require.NoError(t, err, "Failed to assign server to environment")

		t.Run("Login with Server ID Maps to Environment", func(t *testing.T) {
			body := strings.NewReader(fmt.Sprintf(`{
				"username": "%s", 
				"password": "%s",
				"server_id": "%s"
			}`, testUsername, testPassword, testServer.ID))

			req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
			req.Header.Set("Content-Type", "application/json")

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful login")

			var loginResp server.LoginResponse
			err = json.NewDecoder(resp.Body).Decode(&loginResp)
			require.NoError(t, err, "Failed to decode response")

			assert.NotEmpty(t, loginResp.AccessToken, "Expected access token")

			// Verify token contains environment scope by validating it
			claims, err := ts.API.Auth.ValidateToken(loginResp.AccessToken)
			require.NoError(t, err, "Failed to validate token")
			require.NotNil(t, claims.EnvironmentID, "Expected environment ID in token")
			assert.Equal(t, devEnv.ID, *claims.EnvironmentID, "Expected development environment ID")
		})

		t.Run("Direct Environment ID Login", func(t *testing.T) {
			body := strings.NewReader(fmt.Sprintf(`{
				"username": "%s", 
				"password": "%s",
				"environment_id": %d
			}`, testUsername, testPassword, devEnv.ID))

			req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
			req.Header.Set("Content-Type", "application/json")

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful login")

			var loginResp server.LoginResponse
			err = json.NewDecoder(resp.Body).Decode(&loginResp)
			require.NoError(t, err, "Failed to decode response")

			assert.NotEmpty(t, loginResp.AccessToken, "Expected access token")

			// Verify token contains environment scope
			claims, err := ts.API.Auth.ValidateToken(loginResp.AccessToken)
			require.NoError(t, err, "Failed to validate token")
			require.NotNil(t, claims.EnvironmentID, "Expected environment ID in token")
			assert.Equal(t, devEnv.ID, *claims.EnvironmentID, "Expected development environment ID")
		})

		t.Run("Server Without Environment Fails", func(t *testing.T) {
			// Create server with a test environment first
			testEnv, err := ts.Repo.CreateEnvironment(ctx, "no-env-test", "https://github.com/test/no-env.git", "main", false)
			require.NoError(t, err, "Failed to create test environment")

			noEnvServer, err := ts.Repo.CreateServer(ctx, "No Env Server", testEnv.ID)
			require.NoError(t, err, "Failed to create server")

			// Remove the environment association to simulate a server without environment
			dbConn := ts.Repo.DB
			_, err = dbConn.ExecContext(ctx, "UPDATE servers SET environment_id = NULL WHERE id = $1", noEnvServer.ID)
			require.NoError(t, err, "Failed to remove environment association")

			body := strings.NewReader(fmt.Sprintf(`{
				"username": "%s", 
				"password": "%s",
				"server_id": "%s"
			}`, testUsername, testPassword, noEnvServer.ID))

			req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
			req.Header.Set("Content-Type", "application/json")

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Expected error for server without environment")
		})

		t.Run("Nonexistent Server ID", func(t *testing.T) {
			body := strings.NewReader(fmt.Sprintf(`{
				"username": "%s", 
				"password": "%s",
				"server_id": "nonexistent-server"
			}`, testUsername, testPassword))

			req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
			req.Header.Set("Content-Type", "application/json")

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Expected error for nonexistent server")
		})

		t.Run("Nonexistent Environment ID", func(t *testing.T) {
			body := strings.NewReader(fmt.Sprintf(`{
				"username": "%s", 
				"password": "%s",
				"environment_id": 99999
			}`, testUsername, testPassword))

			req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/auth/login", body)
			req.Header.Set("Content-Type", "application/json")

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Expected error for nonexistent environment")
		})
	})
}

func TestCreateUserHandler(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Cleanup()

	adminToken := ts.Tokens["admin"]
	userToken := ts.Tokens["user"]

	t.Run("AdminCanCreateUser", func(t *testing.T) {
		username := fmt.Sprintf("dashuser-%d", time.Now().UnixNano())
		body := strings.NewReader(fmt.Sprintf(`{"username":"%s","password":"StrongPassword123","role":"user"}`, username))

		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/users", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp server.CreateUserResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
		assert.Equal(t, username, createResp.User.Username)
		assert.Equal(t, models.RoleUser, createResp.User.Role)
		assert.NotZero(t, createResp.User.ID)

		storedUser, err := ts.Repo.GetUser(context.Background(), username)
		require.NoError(t, err)
		assert.Equal(t, models.RoleUser, storedUser.Role)
	})

	t.Run("NonAdminForbidden", func(t *testing.T) {
		body := strings.NewReader(`{"username":"nouser","password":"StrongPassword123","role":"user"}`)
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/users", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+userToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("InvalidPasswordRejected", func(t *testing.T) {
		body := strings.NewReader(`{"username":"shortpassuser","password":"short","role":"user"}`)
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/users", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestListUsersHandler(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Cleanup()

	adminToken := ts.Tokens["admin"]
	userToken := ts.Tokens["user"]

	t.Run("AdminCanListUsers", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/users?limit=50&offset=0", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp server.ListUsersResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
		assert.GreaterOrEqual(t, listResp.Total, 1)
		assert.Equal(t, 50, listResp.Limit)
		assert.Equal(t, 0, listResp.Offset)
		assert.Empty(t, listResp.Role)

		var foundAdmin bool
		for _, u := range listResp.Users {
			if u.Username == ts.Users["admin"].Username {
				foundAdmin = true
				break
			}
		}
		assert.True(t, foundAdmin, "expected to find seeded admin user in list response")
	})

	t.Run("NonAdminForbidden", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/users", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("InvalidLimitRejected", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/users?limit=-5", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("RoleFilter", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/users?role=admin", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp server.ListUsersResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
		assert.NotEmpty(t, listResp.Users)
		assert.Equal(t, "admin", listResp.Role)
		for _, u := range listResp.Users {
			assert.Equal(t, models.RoleAdmin, u.Role)
		}
	})

	t.Run("InvalidRoleRejected", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/users?role=badrole", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestDeleteUserHandler(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Cleanup()

	adminToken := ts.Tokens["admin"]
	userToken := ts.Tokens["user"]

	ctx := context.Background()
	err := ts.Repo.CreateUser(ctx, "deletable-user", "StrongPass123", models.RoleUser)
	require.NoError(t, err)
	deletable, err := ts.Repo.GetUser(ctx, "deletable-user")
	require.NoError(t, err)

	t.Run("AdminCanDeleteUser", func(t *testing.T) {
		body := strings.NewReader(`{"confirm_username":"deletable-user"}`)
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/users/%d", ts.Server.URL, deletable.ID), body)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)

		_, err = ts.Repo.GetUser(ctx, "deletable-user")
		require.Error(t, err)
	})

	t.Run("ConfirmUsernameMismatch", func(t *testing.T) {
		err := ts.Repo.CreateUser(ctx, "mismatch-user", "StrongPass123", models.RoleUser)
		require.NoError(t, err)
		mismatchUser, _ := ts.Repo.GetUser(ctx, "mismatch-user")

		body := strings.NewReader(`{"confirm_username":"wrong"}`)
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/users/%d", ts.Server.URL, mismatchUser.ID), body)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("NonAdminForbidden", func(t *testing.T) {
		body := strings.NewReader(`{"confirm_username":"testuser"}`)
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/users/%d", ts.Server.URL, ts.Users["user"].ID), body)
		req.Header.Set("Authorization", "Bearer "+userToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("CannotDeleteSelf", func(t *testing.T) {
		body := strings.NewReader(`{"confirm_username":"` + ts.Users["admin"].Username + `"}`)
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/users/%d", ts.Server.URL, ts.Users["admin"].ID), body)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("CannotDeleteOnlyAdmin", func(t *testing.T) {
		// Ensure only one admin remains
		admins, total, err := ts.Repo.ListUsers(ctx, 100, 0, models.RoleAdmin)
		require.NoError(t, err)
		if total == 1 {
			body := strings.NewReader(`{"confirm_username":"` + admins[0].Username + `"}`)
			req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/users/%d", ts.Server.URL, admins[0].ID), body)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			req.Header.Set("Content-Type", "application/json")

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		}
	})
}

func TestBootstrapAdminHandler(t *testing.T) {
	tdb := testutil.SetupTestDB(t)
	repo := tdb.Repository()

	authService, err := auth.NewService("test-bootstrap-secret", 1)
	require.NoError(t, err)

	api := &server.API{Repo: repo, Auth: authService}

	const bootstrapSecret = "super-secure-bootstrap-token"
	api.EnableBootstrap(bootstrapSecret, "")

	requestBody := `{"bootstrap_secret":"super-secure-bootstrap-token","username":"bootstrap-admin","password":"SixteenCharsPwd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bootstrap/admin", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	api.BootstrapAdminHandler(resp, req)

	require.Equal(t, http.StatusCreated, resp.Code)

	var bootstrapResp server.BootstrapAdminResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&bootstrapResp))
	require.NotEmpty(t, bootstrapResp.AccessToken)
	assert.Equal(t, "bootstrap-admin", bootstrapResp.User.Username)
	assert.Equal(t, models.RoleAdmin, bootstrapResp.User.Role)

	hasAdmin, err := repo.HasActiveAdmin(context.Background())
	require.NoError(t, err)
	assert.True(t, hasAdmin)

	// subsequent request should be rejected because bootstrap is disabled
	retryReq := httptest.NewRequest(http.MethodPost, "/api/v1/bootstrap/admin", strings.NewReader(requestBody))
	retryReq.Header.Set("Content-Type", "application/json")
	retryResp := httptest.NewRecorder()
	api.BootstrapAdminHandler(retryResp, retryReq)
	assert.Equal(t, http.StatusGone, retryResp.Code)
}

func TestAuthMiddlewareAndGetCurrentUser(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	userName := "auth-test-user"
	password := "auth-pass"
	err := ts.Repo.CreateUser(ctx, userName, password, models.RoleAdmin)
	if err != nil {
		t.Fatalf("Setup failed: Could not create user: %v", err)
	}
	user, _ := ts.Repo.GetUser(ctx, userName)
	validToken, err := ts.API.Auth.GenerateToken(user)
	if err != nil {
		t.Fatalf("Setup failed: Could not generate token: %v", err)
	}

	t.Run("No Token", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/auth/me", nil)
		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized (401), got %d", resp.StatusCode)
		}
	})

	t.Run("Invalid Token", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer invalid-token-string")
		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized (401), got %d", resp.StatusCode)
		}
	})

	t.Run("Valid Token", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK (200), got %d", resp.StatusCode)
		}

		var userSummary server.UserSummary
		if err := json.NewDecoder(resp.Body).Decode(&userSummary); err != nil {
			t.Fatalf("Failed to decode response body: %v", err)
		}

		if userSummary.Username != userName {
			t.Errorf("Expected username %s, got %s", userName, userSummary.Username)
		}
		if userSummary.Role != models.RoleAdmin {
			t.Errorf("Expected role %s, got %s", models.RoleAdmin, userSummary.Role)
		}
	})
}

// --- New Server Management Tests ---

func TestServerManagementHandlers(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Cleanup()
	ctx := context.Background()

	adminToken := ts.Tokens["admin"]
	userToken := ts.Tokens["user"]

	var serverUUID, serverUUID2 string
	// Get access to the underlying DB from the repository
	dbConn := ts.Repo.DB
	err := dbConn.QueryRowContext(ctx, "SELECT gen_random_uuid()").Scan(&serverUUID)
	require.NoError(t, err, "Failed to generate serverUUID")
	server1ID := serverUUID
	err = dbConn.QueryRowContext(ctx, "SELECT gen_random_uuid()").Scan(&serverUUID2)
	require.NoError(t, err, "Failed to generate serverUUID2")
	server2ID := serverUUID2

	// Create a test environment first
	testEnv, err := ts.Repo.CreateEnvironment(ctx, "handler-test-env", "https://github.com/example/repo.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	// --- Create Server --- (Using Admin)
	t.Run("Create Server", func(t *testing.T) {
		createPayload := fmt.Sprintf(`{
			"id": "%s",
			"name": "Server One", 
			"environment_id": %d
		}`, "test-server-1", testEnv.ID)

		var createdServer models.Server
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/servers", strings.NewReader(createPayload))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create server")

		err = json.NewDecoder(resp.Body).Decode(&createdServer)
		require.NoError(t, err, "Failed to decode created server response")
		resp.Body.Close()

		server1ID = createdServer.ID
	})

	// --- Get Server (No Status Yet) ---
	t.Run("Get Server - No Status", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/servers/"+server1ID, nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var serverResp models.ServerWithStatus
		err = json.NewDecoder(resp.Body).Decode(&serverResp)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, server1ID, serverResp.ID)
		assert.Equal(t, "Server One", serverResp.Name)
		// Assert status fields are nil/omitted
		assert.Nil(t, serverResp.LastCommitHash, "LastCommitHash should be nil when no status reported")
		assert.Nil(t, serverResp.LastStatus, "LastStatus should be nil when no status reported")
		assert.Nil(t, serverResp.LastIsDrifted, "LastIsDrifted should be nil when no status reported")
		assert.Nil(t, serverResp.LastTimestamp, "LastTimestamp should be nil when no status reported")
	})

	initialCommit := "abcdef123"
	initialStatus := "Applied"
	initialDrift := false
	initialTimestamp := time.Now().UTC().Truncate(time.Second)
	err = ts.Repo.CreateAgentStatus(ctx, &models.AgentStatus{
		ServerID:   server1ID,
		CommitHash: initialCommit,
		Status:     initialStatus,
		IsDrifted:  initialDrift,
		Timestamp:  initialTimestamp,
	})
	require.NoError(t, err, "Failed to seed agent status for server1")

	// --- Get Server (With Status) ---
	t.Run("Get Server - With Status", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/servers/"+server1ID, nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var serverResp models.ServerWithStatus
		err = json.NewDecoder(resp.Body).Decode(&serverResp)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, server1ID, serverResp.ID)
		// Assert status fields are populated correctly
		require.NotNil(t, serverResp.LastCommitHash)
		assert.Equal(t, initialCommit, *serverResp.LastCommitHash)
		require.NotNil(t, serverResp.LastStatus)
		assert.Equal(t, initialStatus, *serverResp.LastStatus)
		require.NotNil(t, serverResp.LastIsDrifted)
		assert.Equal(t, initialDrift, *serverResp.LastIsDrifted)
		require.NotNil(t, serverResp.LastTimestamp)
		assert.WithinDuration(t, initialTimestamp, *serverResp.LastTimestamp, time.Second, "Timestamp mismatch")
	})

	// --- Create Server 2 ---
	t.Run("Create Server 2", func(t *testing.T) {
		createPayload := fmt.Sprintf(`{
			"id": "%s",
			"name": "Server Two", 
			"environment_id": %d
		}`, "test-server-2", testEnv.ID)

		var createdServer models.Server
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/servers", strings.NewReader(createPayload))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		err = json.NewDecoder(resp.Body).Decode(&createdServer)
		require.NoError(t, err, "Failed to decode created server response")
		resp.Body.Close()

		server2ID = createdServer.ID
	})

	// --- List Servers (Mixed Status) ---
	t.Run("List Servers - Mixed Status", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/servers?limit=10", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var serversResp []*models.ServerWithStatus
		err = json.NewDecoder(resp.Body).Decode(&serversResp)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Len(t, serversResp, 2, "Expected 2 servers in list")

		var s1, s2 *models.ServerWithStatus
		for _, s := range serversResp {
			if s.ID == server1ID {
				s1 = s
			}
			if s.ID == server2ID {
				s2 = s
			}
		}

		require.NotNil(t, s1, "Server 1 not found in list response")
		require.NotNil(t, s2, "Server 2 not found in list response")

		// Assert Server 1 has status
		assert.NotNil(t, s1.LastCommitHash, "Server1 LastCommitHash should be populated")
		assert.Equal(t, initialCommit, *s1.LastCommitHash)
		assert.NotNil(t, s1.LastStatus, "Server1 LastStatus should be populated")
		assert.Equal(t, initialStatus, *s1.LastStatus)
		assert.NotNil(t, s1.LastIsDrifted, "Server1 IsDrifted should be populated")
		assert.Equal(t, initialDrift, *s1.LastIsDrifted)
		assert.NotNil(t, s1.LastTimestamp, "Server1 LastTimestamp should be populated")
		assert.WithinDuration(t, initialTimestamp, *s1.LastTimestamp, time.Second)

		// Assert Server 2 has no status
		assert.Nil(t, s2.LastCommitHash, "Server2 LastCommitHash should be nil")
		assert.Nil(t, s2.LastStatus, "Server2 LastStatus should be nil")
		assert.Nil(t, s2.LastIsDrifted, "Server2 IsDrifted should be nil")
		assert.Nil(t, s2.LastTimestamp, "Server2 LastTimestamp should be nil")
	})
}

// TestUpdateAgentStatusHandler_NoDuplicates tests that agent status updates that don't change aren't recorded
func TestUpdateAgentStatusHandler_NoDuplicates(t *testing.T) {
	testutil.SkipIfShort(t)

	ctx := context.Background()

	ts := setupTestServer(t)
	defer ts.Cleanup()

	// Create a test environment first
	testEnv, err := ts.Repo.CreateEnvironment(ctx, "handler-test-env", "https://github.com/example/repo.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	testServer, err := ts.Repo.CreateServer(ctx, "Test Server", testEnv.ID)

	if testServer != nil {
		serverID := "test-server-1"
		dbConn := ts.Repo.DB
		_, err = dbConn.ExecContext(ctx, "UPDATE servers SET id = $1 WHERE id = $2", serverID, testServer.ID)
		testServer.ID = serverID
		assert.NoError(t, err, "Failed to update server ID")
	}
	assert.NoError(t, err)
	assert.NotNil(t, testServer)

	api := ts.API

	// Create user claims for authentication
	claims := &auth.Claims{
		UserID:   1,
		Username: "test-user",
		Role:     models.RoleUser,
	}

	statusPayload1 := `{
		"commit_hash": "abc123",
		"is_drifted": false,
		"status": "running",
		"error_message": null
	}`

	req1 := httptest.NewRequest("PUT", "/api/v1/servers/test-server-1/status", strings.NewReader(statusPayload1))
	req1.Header.Set("Content-Type", "application/json")

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("serverID", "test-server-1")
	ctx1 := context.WithValue(req1.Context(), chi.RouteCtxKey, chiCtx)
	ctx1 = context.WithValue(ctx1, server.UserClaimsKey, claims)
	req1 = req1.WithContext(ctx1)

	resp1 := httptest.NewRecorder()

	api.UpdateAgentStatusHandler(resp1, req1)
	assert.Equal(t, http.StatusOK, resp1.Code)

	statuses1, err := ts.Repo.GetAgentStatusHistory(ctx, "test-server-1", 10, 0)
	assert.NoError(t, err)
	assert.Len(t, statuses1, 1, "Expected 1 status entry after initial update")

	req2 := httptest.NewRequest("PUT", "/api/v1/servers/test-server-1/status", strings.NewReader(statusPayload1))
	req2.Header.Set("Content-Type", "application/json")

	chiCtx2 := chi.NewRouteContext()
	chiCtx2.URLParams.Add("serverID", "test-server-1")
	ctx2 := context.WithValue(req2.Context(), chi.RouteCtxKey, chiCtx2)
	ctx2 = context.WithValue(ctx2, server.UserClaimsKey, claims)
	req2 = req2.WithContext(ctx2)

	resp2 := httptest.NewRecorder()

	api.UpdateAgentStatusHandler(resp2, req2)
	assert.Equal(t, http.StatusOK, resp2.Code)

	statuses2, err := ts.Repo.GetAgentStatusHistory(ctx, "test-server-1", 10, 0)
	assert.NoError(t, err)
	assert.Len(t, statuses2, 1, "Expected still just 1 status entry after duplicate update")

	statusPayload3 := `{
		"commit_hash": "abc123",
		"is_drifted": true,
		"status": "running",
		"error_message": "Detected drift in config"
	}`

	req3 := httptest.NewRequest("PUT", "/api/v1/servers/test-server-1/status", strings.NewReader(statusPayload3))
	req3.Header.Set("Content-Type", "application/json")

	// Add server ID as chi URL param
	chiCtx3 := chi.NewRouteContext()
	chiCtx3.URLParams.Add("serverID", "test-server-1")
	ctx3 := context.WithValue(req3.Context(), chi.RouteCtxKey, chiCtx3)
	ctx3 = context.WithValue(ctx3, server.UserClaimsKey, claims)
	req3 = req3.WithContext(ctx3)

	resp3 := httptest.NewRecorder()

	// Call handler directly
	api.UpdateAgentStatusHandler(resp3, req3)
	assert.Equal(t, http.StatusOK, resp3.Code)

	// Check that a new status was recorded
	statuses3, err := ts.Repo.GetAgentStatusHistory(ctx, "test-server-1", 10, 0)
	assert.NoError(t, err)
	assert.Len(t, statuses3, 2, "Expected 2 status entries after different update")
}

// TestAgentTokenAuthentication tests the agent token authentication system
func TestAgentTokenAuthentication(t *testing.T) {
	testutil.SkipIfShort(t)

	ts := setupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a test environment first
	testEnv, err := ts.Repo.CreateEnvironment(ctx, "agent-auth-test-env", "https://github.com/test/repo.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	// Create a test server
	testServer, err := ts.Repo.CreateServer(ctx, "Test Agent Server", testEnv.ID)
	require.NoError(t, err, "Failed to create test server")

	t.Run("Create Agent Token", func(t *testing.T) {
		// Generate and store a token
		tokenStr, err := token.GenerateToken()
		require.NoError(t, err, "Failed to generate token")

		tokenHash := token.HashToken(tokenStr)
		_, err = ts.Repo.CreateAgentToken(ctx, tokenHash, testServer.ID, "Test token", nil, &ts.Users["admin"].ID)
		require.NoError(t, err, "Failed to create agent token")

		t.Run("Agent Get Server Info with Valid Token", func(t *testing.T) {
			req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/agent/serverinfo", nil)
			req.Header.Set("Authorization", "Bearer "+tokenStr)

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful server info request")

			var serverInfo server.ServerInfoResponse
			err = json.NewDecoder(resp.Body).Decode(&serverInfo)
			require.NoError(t, err, "Failed to decode response")

			assert.Equal(t, testEnv.RepoURL, serverInfo.RepoURL)
			assert.Equal(t, "main", serverInfo.Branch)
			assert.Equal(t, "config.yaml", serverInfo.DeployPath)
			assert.Equal(t, testServer.AutoReconcile, serverInfo.AutoReconcile)
		})

		t.Run("Agent Update Status with Valid Token", func(t *testing.T) {
			statusPayload := `{
				"commit_hash": "abc123def456",
				"is_drifted": false,
				"status": "running",
				"error_message": null
			}`

			req, _ := http.NewRequest("PUT", ts.Server.URL+"/api/v1/agent/status", strings.NewReader(statusPayload))
			req.Header.Set("Authorization", "Bearer "+tokenStr)
			req.Header.Set("Content-Type", "application/json")

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful status update")

			var statusResp map[string]string
			err = json.NewDecoder(resp.Body).Decode(&statusResp)
			require.NoError(t, err, "Failed to decode response")

			assert.Equal(t, "received", statusResp["status"])

			// Verify status was stored
			statuses, err := ts.Repo.GetAgentStatusHistory(ctx, testServer.ID, 10, 0)
			require.NoError(t, err, "Failed to get status history")
			assert.Len(t, statuses, 1, "Expected 1 status entry")
			assert.Equal(t, "abc123def456", statuses[0].CommitHash)
			assert.Equal(t, "running", statuses[0].Status)
			assert.False(t, statuses[0].IsDrifted)
		})

		t.Run("Invalid Token Format", func(t *testing.T) {
			req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/agent/serverinfo", nil)
			req.Header.Set("Authorization", "Bearer invalid-token-format")

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Expected unauthorized for invalid token format")
		})

		t.Run("Valid Format but Nonexistent Token", func(t *testing.T) {
			fakeToken, _ := token.GenerateToken()
			req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/agent/serverinfo", nil)
			req.Header.Set("Authorization", "Bearer "+fakeToken)

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Expected unauthorized for nonexistent token")
		})

		t.Run("Missing Authorization Header", func(t *testing.T) {
			req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/agent/serverinfo", nil)

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Expected unauthorized for missing auth header")
		})

		t.Run("Wrong Authorization Format", func(t *testing.T) {
			req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/agent/serverinfo", nil)
			req.Header.Set("Authorization", "Basic "+tokenStr)

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Expected unauthorized for wrong auth format")
		})

		t.Run("Deactivated Token", func(t *testing.T) {
			tokens, err := ts.Repo.ListAgentTokensByServer(ctx, testServer.ID)
			require.NoError(t, err, "Failed to list tokens")
			require.Len(t, tokens, 1, "Expected 1 token")

			err = ts.Repo.DeactivateAgentToken(ctx, tokens[0].ID)
			require.NoError(t, err, "Failed to deactivate token")

			req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/agent/serverinfo", nil)
			req.Header.Set("Authorization", "Bearer "+tokenStr)

			resp, err := ts.Server.Client().Do(req)
			require.NoError(t, err, "Request failed")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Expected unauthorized for deactivated token")
		})
	})
}

func createServerViaAPI(t *testing.T, ts *testServer, serverID string, envID int64) {
	body := strings.NewReader(fmt.Sprintf(`{"id":"%s","name":"%s","environment_id":%d}`, serverID, "API Server", envID))
	req, err := http.NewRequest(http.MethodPost, ts.Server.URL+"/api/v1/servers", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.Tokens["admin"])
	resp, err := ts.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestCreateServerRecordsAuditLog(t *testing.T) {
	testutil.SkipIfShort(t)

	ts := setupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	envs, _, err := ts.Repo.ListEnvironments(ctx, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, envs)

	serverID := "audit-server-test"
	createServerViaAPI(t, ts, serverID, envs[0].ID)

	var action, resourceType, resourceID string
	err = ts.Repo.QueryRowContext(ctx, `SELECT action, resource_type, resource_id FROM audit_log WHERE resource_type = $1 ORDER BY timestamp DESC LIMIT 1`, "server").Scan(&action, &resourceType, &resourceID)
	require.NoError(t, err)
	require.Equal(t, "create", action)
	require.Equal(t, "server", resourceType)
	require.Equal(t, serverID, resourceID)
}

func TestCreateAndDeactivateAgentTokenRecordsAudit(t *testing.T) {
	testutil.SkipIfShort(t)

	ts := setupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	envs, _, err := ts.Repo.ListEnvironments(ctx, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, envs)

	serverID := "audit-token-server"
	createServerViaAPI(t, ts, serverID, envs[0].ID)

	tokenReqBody := strings.NewReader(`{"description":"audit token"}`)
	createReq, err := http.NewRequest(http.MethodPost, ts.Server.URL+"/api/v1/servers/"+serverID+"/tokens", tokenReqBody)
	require.NoError(t, err)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+ts.Tokens["admin"])
	createResp, err := ts.Server.Client().Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createPayload map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createPayload))
	createdTokenIDFloat, ok := createPayload["id"].(float64)
	require.True(t, ok, "token response missing id field")
	createdTokenID := int(createdTokenIDFloat)

	var action, resourceType, resourceID string
	err = ts.Repo.QueryRowContext(ctx, `SELECT action, resource_type, resource_id FROM audit_log WHERE resource_type = $1 ORDER BY timestamp DESC LIMIT 1`, "agent_token").Scan(&action, &resourceType, &resourceID)
	require.NoError(t, err)
	require.Equal(t, "create", action)
	require.Equal(t, "agent_token", resourceType)
	require.Equal(t, strconv.Itoa(createdTokenID), resourceID)

	deleteReq, err := http.NewRequest(http.MethodDelete, ts.Server.URL+"/api/v1/servers/"+serverID+"/tokens/"+strconv.Itoa(createdTokenID), nil)
	require.NoError(t, err)
	deleteReq.Header.Set("Authorization", "Bearer "+ts.Tokens["admin"])
	deleteResp, err := ts.Server.Client().Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()
	require.Equal(t, http.StatusOK, deleteResp.StatusCode)

	err = ts.Repo.QueryRowContext(ctx, `SELECT action, resource_type, resource_id FROM audit_log WHERE resource_type = $1 ORDER BY timestamp DESC LIMIT 1`, "agent_token").Scan(&action, &resourceType, &resourceID)
	require.NoError(t, err)
	require.Equal(t, "deactivate", action)
	require.Equal(t, "agent_token", resourceType)
	require.Equal(t, strconv.Itoa(createdTokenID), resourceID)
}

// TestServerTokenManagement tests the server token management endpoints
func TestServerTokenManagement(t *testing.T) {
	testutil.SkipIfShort(t)

	ts := setupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a test environment first
	testEnv, err := ts.Repo.CreateEnvironment(ctx, "token-mgmt-test-env", "https://github.com/test/repo.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	// Create a test server
	testServer, err := ts.Repo.CreateServer(ctx, "Token Management Server", testEnv.ID)
	require.NoError(t, err, "Failed to create test server")

	adminToken := ts.Tokens["admin"]

	t.Run("List Server Tokens - Empty", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/servers/"+testServer.ID+"/tokens", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful token list")

		var tokens []*models.AgentToken
		err = json.NewDecoder(resp.Body).Decode(&tokens)
		require.NoError(t, err, "Failed to decode response")

		assert.Len(t, tokens, 0, "Expected no tokens initially")
	})

	t.Run("Create Server Token", func(t *testing.T) {
		tokenReq := `{
			"description": "Test token for automation",
			"expires_in": 30
		}`

		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/servers/"+testServer.ID+"/tokens", strings.NewReader(tokenReq))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected successful token creation")

		var tokenResp server.CreateServerTokenResponse
		err = json.NewDecoder(resp.Body).Decode(&tokenResp)
		require.NoError(t, err, "Failed to decode response")

		assert.NotEmpty(t, tokenResp.Token, "Expected token in response")
		assert.True(t, token.ValidateTokenFormat(tokenResp.Token), "Expected valid token format")
		assert.Equal(t, "Test token for automation", tokenResp.Description)
		assert.Equal(t, testServer.ID, tokenResp.ServerID)
		assert.NotEmpty(t, tokenResp.InstallationInfo.Instructions, "Expected installation instructions")
		assert.Contains(t, tokenResp.InstallationInfo.Instructions, testServer.ID, "Expected server ID in instructions")
	})

	t.Run("List Server Tokens - Has Token", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/servers/"+testServer.ID+"/tokens", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful token list")

		var tokens []*models.AgentToken
		err = json.NewDecoder(resp.Body).Decode(&tokens)
		require.NoError(t, err, "Failed to decode response")

		assert.Len(t, tokens, 1, "Expected 1 token")
		assert.Equal(t, "Test token for automation", tokens[0].Description)
		assert.True(t, tokens[0].IsActive)
	})

	t.Run("Get Installation Instructions", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/servers/"+testServer.ID+"/install", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful installation instructions")

		var instructions map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&instructions)
		require.NoError(t, err, "Failed to decode response")

		assert.Equal(t, testServer.ID, instructions["server_id"])
		assert.Equal(t, testServer.Name, instructions["server_name"])
		assert.True(t, instructions["has_tokens"].(bool))
		assert.Equal(t, float64(1), instructions["active_tokens"])
		assert.Contains(t, instructions["instructions"], testServer.ID)
	})

	t.Run("Deactivate Token", func(t *testing.T) {
		tokens, err := ts.Repo.ListAgentTokensByServer(ctx, testServer.ID)
		require.NoError(t, err, "Failed to list tokens")
		require.Len(t, tokens, 1, "Expected 1 token")

		tokenID := tokens[0].ID

		req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/servers/%s/tokens/%d", ts.Server.URL, testServer.ID, tokenID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful token deactivation")

		var deactivateResp map[string]string
		err = json.NewDecoder(resp.Body).Decode(&deactivateResp)
		require.NoError(t, err, "Failed to decode response")

		assert.Equal(t, "Token deactivated successfully", deactivateResp["message"])

		// Verify token is deactivated
		updatedTokens, err := ts.Repo.ListAgentTokensByServer(ctx, testServer.ID)
		require.NoError(t, err, "Failed to list tokens")
		assert.Len(t, updatedTokens, 0, "Expected no active tokens after deactivation")
	})

	t.Run("Unauthorized Access", func(t *testing.T) {
		userToken := ts.Tokens["viewer"]

		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/servers/"+testServer.ID+"/tokens", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Expected forbidden for viewer role")
	})
}

func TestGetServerInstallScript(t *testing.T) {
	testutil.SkipIfShort(t)

	ts := setupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	testEnv, err := ts.Repo.CreateEnvironment(ctx, "install-script-env", "https://github.com/test/repo.git", "main", false)
	require.NoError(t, err)

	testServer, err := ts.Repo.CreateServer(ctx, "Install Script Server", testEnv.ID)
	require.NoError(t, err)

	adminToken := ts.Tokens["admin"]

	var agentTokenValue string
	t.Run("Create token for install script", func(t *testing.T) {
		tokenReq := `{"description": "install script token", "expires_in": 30}`
		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/servers/"+testServer.ID+"/tokens", strings.NewReader(tokenReq))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var tokenResp map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&tokenResp))
		agentTokenValue = tokenResp["token"].(string)
		require.NotEmpty(t, agentTokenValue)
	})

	t.Run("Get install script - success", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script?token=%s", ts.Server.URL, testServer.ID, agentTokenValue)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/x-shellscript", resp.Header.Get("Content-Type"))
		assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")

		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		script := string(bodyBytes)
		assert.Contains(t, script, "#!/bin/bash")
		assert.Contains(t, script, testServer.ID)
		assert.Contains(t, script, agentTokenValue)
		assert.Contains(t, script, "systemctl")
		assert.Contains(t, script, "pullbase-agent.service")
	})

	t.Run("Get install script - with version", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script?token=%s&version=v1.0.0", ts.Server.URL, testServer.ID, agentTokenValue)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		script := string(bodyBytes)
		assert.Contains(t, script, "#!/bin/bash")
		assert.Contains(t, script, testServer.ID)
		assert.Contains(t, script, agentTokenValue)
		assert.Contains(t, script, "systemctl")
		assert.Contains(t, script, "pullbase-agent.service")
	})

	t.Run("Get install script - with version", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script?token=%s&version=v1.0.0", ts.Server.URL, testServer.ID, agentTokenValue)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Contains(t, string(bodyBytes), `AGENT_VERSION="v1.0.0"`)
	})

	t.Run("Get install script - missing token", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script", ts.Server.URL, testServer.ID)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Get install script - invalid token format", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script?token=invalid", ts.Server.URL, testServer.ID)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Get install script - token for wrong server", func(t *testing.T) {
		otherServer, err := ts.Repo.CreateServer(ctx, "Other Server", testEnv.ID)
		require.NoError(t, err)

		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script?token=%s", ts.Server.URL, otherServer.ID, agentTokenValue)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("Get install script - server not found", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/servers/nonexistent/install-script?token=%s", ts.Server.URL, agentTokenValue)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("Get install script - viewer role forbidden", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script?token=%s", ts.Server.URL, testServer.ID, agentTokenValue)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+ts.Tokens["viewer"])

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("Get install script - user role allowed", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script?token=%s", ts.Server.URL, testServer.ID, agentTokenValue)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+ts.Tokens["user"])

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Get install script - inactive token returns unauthorized", func(t *testing.T) {
		// Create a new token specifically for this test
		tokenReq := `{"description": "token to deactivate", "expires_in": 30}`
		createReq, _ := http.NewRequest("POST", ts.Server.URL+"/api/v1/servers/"+testServer.ID+"/tokens", strings.NewReader(tokenReq))
		createReq.Header.Set("Authorization", "Bearer "+adminToken)
		createReq.Header.Set("Content-Type", "application/json")

		createResp, err := ts.Server.Client().Do(createReq)
		require.NoError(t, err)
		defer createResp.Body.Close()
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		var tokenResp map[string]interface{}
		require.NoError(t, json.NewDecoder(createResp.Body).Decode(&tokenResp))
		inactiveTokenValue := tokenResp["token"].(string)
		tokenID := int(tokenResp["id"].(float64))

		// Deactivate the token
		deactivateReq, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/servers/%s/tokens/%d", ts.Server.URL, testServer.ID, tokenID), nil)
		deactivateReq.Header.Set("Authorization", "Bearer "+adminToken)

		deactivateResp, err := ts.Server.Client().Do(deactivateReq)
		require.NoError(t, err)
		defer deactivateResp.Body.Close()
		require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

		// Try to use the deactivated token for install script
		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script?token=%s", ts.Server.URL, testServer.ID, inactiveTokenValue)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Get install script - records audit log", func(t *testing.T) {
		url := fmt.Sprintf("%s/api/v1/servers/%s/install-script?token=%s", ts.Server.URL, testServer.ID, agentTokenValue)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify audit log entry was created
		var action, resourceType, resourceID string
		err = ts.Repo.QueryRowContext(ctx, `SELECT action, resource_type, resource_id FROM audit_log WHERE action = $1 AND resource_type = $2 ORDER BY timestamp DESC LIMIT 1`, "generate_install_script", "server").Scan(&action, &resourceType, &resourceID)
		require.NoError(t, err)
		assert.Equal(t, "generate_install_script", action)
		assert.Equal(t, "server", resourceType)
		assert.Equal(t, testServer.ID, resourceID)
	})
}

func TestLivenessHandler(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Cleanup()

	t.Run("Healthy Database Returns 200", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/healthz", nil)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK for healthy database")

		var healthResp server.HealthCheckResponse
		err = json.NewDecoder(resp.Body).Decode(&healthResp)
		require.NoError(t, err, "Failed to decode response")

		assert.Equal(t, server.HealthStatusHealthy, healthResp.Status, "Expected healthy status")
		assert.Equal(t, "pullbase-server", healthResp.Service)
		assert.NotNil(t, healthResp.Checks["database"], "Expected database check in response")
		assert.Equal(t, server.HealthStatusHealthy, healthResp.Checks["database"].Status)
		assert.NotNil(t, healthResp.Checks["database"].LatencyMs, "Expected latency in database check")
	})

	t.Run("No Authentication Required", func(t *testing.T) {
		// Health endpoints should be public (no auth header)
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/healthz", nil)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Health check should not require authentication")
	})
}

func TestReadinessHandler(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Cleanup()

	t.Run("Healthy Database and Migrations Returns 200", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/readyz", nil)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK for healthy database")

		var healthResp server.HealthCheckResponse
		err = json.NewDecoder(resp.Body).Decode(&healthResp)
		require.NoError(t, err, "Failed to decode response")

		assert.Equal(t, server.HealthStatusHealthy, healthResp.Status, "Expected healthy status")
		assert.Equal(t, "pullbase-server", healthResp.Service)

		// Check database status
		assert.NotNil(t, healthResp.Checks["database"], "Expected database check in response")
		assert.Equal(t, server.HealthStatusHealthy, healthResp.Checks["database"].Status)
		assert.NotNil(t, healthResp.Checks["database"].LatencyMs, "Expected latency in database check")

		// Check migrations status
		assert.NotNil(t, healthResp.Checks["migrations"], "Expected migrations check in response")
		assert.Equal(t, server.HealthStatusHealthy, healthResp.Checks["migrations"].Status)
		assert.NotNil(t, healthResp.Checks["migrations"].Version, "Expected version in migrations check")
		assert.Greater(t, *healthResp.Checks["migrations"].Version, 0, "Expected positive migration version")
	})

	t.Run("No Authentication Required", func(t *testing.T) {
		// Health endpoints should be public (no auth header)
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/readyz", nil)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Readiness check should not require authentication")
	})

	t.Run("Response Contains JSON Content Type", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/readyz", nil)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		assert.Contains(t, contentType, "application/json", "Expected JSON content type")
	})
}

func TestHealthCheckResponseFormat(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Cleanup()

	t.Run("Liveness Response Structure", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/healthz", nil)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Failed to read response body")

		// Verify JSON structure
		var raw map[string]interface{}
		err = json.Unmarshal(bodyBytes, &raw)
		require.NoError(t, err, "Response should be valid JSON")

		assert.Contains(t, raw, "status", "Response should contain status field")
		assert.Contains(t, raw, "service", "Response should contain service field")
		assert.Contains(t, raw, "checks", "Response should contain checks field")
	})

	t.Run("Readiness Response Structure", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.Server.URL+"/api/v1/readyz", nil)

		resp, err := ts.Server.Client().Do(req)
		require.NoError(t, err, "Request failed")
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Failed to read response body")

		var raw map[string]interface{}
		err = json.Unmarshal(bodyBytes, &raw)
		require.NoError(t, err, "Response should be valid JSON")

		// Check all expected fields
		assert.Contains(t, raw, "status")
		assert.Contains(t, raw, "service")
		assert.Contains(t, raw, "checks")

		checks, ok := raw["checks"].(map[string]interface{})
		require.True(t, ok, "checks should be a map")

		assert.Contains(t, checks, "database", "Checks should contain database")
		assert.Contains(t, checks, "migrations", "Checks should contain migrations")
	})
}
