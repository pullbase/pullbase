package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplyEnvOverridesTLS(t *testing.T) {
	tests := []struct {
		name              string
		envVars           map[string]string
		initialTLSEnabled bool
		initialCertPath   string
		initialKeyPath    string
		wantTLSEnabled    bool
		wantCertPath      string
		wantKeyPath       string
		checkEnabled      bool
	}{
		{
			name:              "PULLBASE_TLS_ENABLED=true overrides config",
			envVars:           map[string]string{"PULLBASE_TLS_ENABLED": "true"},
			initialTLSEnabled: false,
			wantTLSEnabled:    true,
			checkEnabled:      true,
		},
		{
			name:              "PULLBASE_TLS_ENABLED=false overrides config",
			envVars:           map[string]string{"PULLBASE_TLS_ENABLED": "false"},
			initialTLSEnabled: true,
			wantTLSEnabled:    false,
			checkEnabled:      true,
		},
		{
			name:              "PULLBASE_TLS_ENABLED=1 is truthy",
			envVars:           map[string]string{"PULLBASE_TLS_ENABLED": "1"},
			initialTLSEnabled: false,
			wantTLSEnabled:    true,
			checkEnabled:      true,
		},
		{
			name:              "PULLBASE_TLS_ENABLED=0 is falsy",
			envVars:           map[string]string{"PULLBASE_TLS_ENABLED": "0"},
			initialTLSEnabled: true,
			wantTLSEnabled:    false,
			checkEnabled:      true,
		},
		{
			name:            "PULLBASE_TLS_CERT_PATH overrides config",
			envVars:         map[string]string{"PULLBASE_TLS_CERT_PATH": "/custom/path/cert.pem"},
			initialCertPath: "/default/cert.pem",
			wantCertPath:    "/custom/path/cert.pem",
		},
		{
			name:           "PULLBASE_TLS_KEY_PATH overrides config",
			envVars:        map[string]string{"PULLBASE_TLS_KEY_PATH": "/custom/path/key.pem"},
			initialKeyPath: "/default/key.pem",
			wantKeyPath:    "/custom/path/key.pem",
		},
		{
			name: "all TLS env vars override config",
			envVars: map[string]string{
				"PULLBASE_TLS_ENABLED":   "true",
				"PULLBASE_TLS_CERT_PATH": "/env/cert.pem",
				"PULLBASE_TLS_KEY_PATH":  "/env/key.pem",
			},
			initialTLSEnabled: false,
			initialCertPath:   "/config/cert.pem",
			initialKeyPath:    "/config/key.pem",
			wantTLSEnabled:    true,
			wantCertPath:      "/env/cert.pem",
			wantKeyPath:       "/env/key.pem",
			checkEnabled:      true,
		},
		{
			name:              "invalid PULLBASE_TLS_ENABLED value keeps original",
			envVars:           map[string]string{"PULLBASE_TLS_ENABLED": "invalid"},
			initialTLSEnabled: true,
			wantTLSEnabled:    true,
			checkEnabled:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			config := &Config{}
			config.TLS.Enabled = tt.initialTLSEnabled
			config.TLS.CertPath = tt.initialCertPath
			config.TLS.KeyPath = tt.initialKeyPath

			applyEnvOverrides(config)

			if tt.checkEnabled && config.TLS.Enabled != tt.wantTLSEnabled {
				t.Errorf("TLS.Enabled = %v, want %v", config.TLS.Enabled, tt.wantTLSEnabled)
			}
			if tt.wantCertPath != "" && config.TLS.CertPath != tt.wantCertPath {
				t.Errorf("TLS.CertPath = %s, want %s", config.TLS.CertPath, tt.wantCertPath)
			}
			if tt.wantKeyPath != "" && config.TLS.KeyPath != tt.wantKeyPath {
				t.Errorf("TLS.KeyPath = %s, want %s", config.TLS.KeyPath, tt.wantKeyPath)
			}
		})
	}
}

func TestValidateConfigTLSEnabled(t *testing.T) {
	validBaseConfig := func() *Config {
		cfg := &Config{}
		cfg.Database.Host = "localhost"
		cfg.Database.Port = 5432
		cfg.Database.User = "user"
		cfg.Database.Password = "pass"
		cfg.Database.DBName = "db"
		cfg.Server.Port = 8080
		cfg.JWT.Secret = "secret"
		cfg.JWT.ExpiryHours = 24
		cfg.Git.ClonePath = "/tmp/git"
		cfg.Git.PollInterval = 60 * time.Second
		cfg.Git.Enabled = false
		cfg.Migrations.Path = "file://migrations"
		return cfg
	}

	t.Run("TLS enabled without cert path fails validation", func(t *testing.T) {
		config := validBaseConfig()
		config.TLS.Enabled = true
		config.TLS.CertPath = ""
		config.TLS.KeyPath = "/path/to/key.pem"

		err := validateConfig(config)
		if err == nil {
			t.Error("validateConfig() should fail when TLS enabled without cert path")
		}
		if !strings.Contains(err.Error(), "TLS certificate path is required") {
			t.Errorf("validateConfig() error = %v, want error containing 'TLS certificate path is required'", err)
		}
	})

	t.Run("TLS enabled without key path fails validation", func(t *testing.T) {
		config := validBaseConfig()
		config.TLS.Enabled = true
		config.TLS.CertPath = "/path/to/cert.pem"
		config.TLS.KeyPath = ""

		err := validateConfig(config)
		if err == nil {
			t.Error("validateConfig() should fail when TLS enabled without key path")
		}
		if !strings.Contains(err.Error(), "TLS key path is required") {
			t.Errorf("validateConfig() error = %v, want error containing 'TLS key path is required'", err)
		}
	})

	t.Run("TLS enabled with both paths passes validation", func(t *testing.T) {
		config := validBaseConfig()
		config.TLS.Enabled = true
		config.TLS.CertPath = "/path/to/cert.pem"
		config.TLS.KeyPath = "/path/to/key.pem"

		err := validateConfig(config)
		if err != nil {
			t.Errorf("validateConfig() unexpected error = %v", err)
		}
	})

	t.Run("TLS disabled without paths passes validation", func(t *testing.T) {
		config := validBaseConfig()
		config.TLS.Enabled = false
		config.TLS.CertPath = ""
		config.TLS.KeyPath = ""

		err := validateConfig(config)
		if err != nil {
			t.Errorf("validateConfig() unexpected error = %v", err)
		}
	})
}

func TestApplyEnvOverridesCORS(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		initialOrigins []string
		wantOrigins    []string
	}{
		{
			name:           "single origin from env",
			envValue:       "https://example.com",
			initialOrigins: []string{},
			wantOrigins:    []string{"https://example.com"},
		},
		{
			name:           "multiple origins comma-separated",
			envValue:       "https://example.com,https://app.example.com",
			initialOrigins: []string{},
			wantOrigins:    []string{"https://example.com", "https://app.example.com"},
		},
		{
			name:           "origins with whitespace trimmed",
			envValue:       "https://example.com , https://app.example.com , https://other.example.com",
			initialOrigins: []string{},
			wantOrigins:    []string{"https://example.com", "https://app.example.com", "https://other.example.com"},
		},
		{
			name:           "env overrides config file origins",
			envValue:       "https://new.example.com",
			initialOrigins: []string{"https://old.example.com"},
			wantOrigins:    []string{"https://new.example.com"},
		},
		{
			name:           "empty env preserves config file origins",
			envValue:       "",
			initialOrigins: []string{"https://example.com"},
			wantOrigins:    []string{"https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("PULLBASE_CORS_ORIGINS", tt.envValue)
				defer os.Unsetenv("PULLBASE_CORS_ORIGINS")
			}

			config := &Config{}
			config.CORS.AllowedOrigins = tt.initialOrigins

			applyEnvOverrides(config)

			if len(config.CORS.AllowedOrigins) != len(tt.wantOrigins) {
				t.Errorf("CORS.AllowedOrigins length = %d, want %d", len(config.CORS.AllowedOrigins), len(tt.wantOrigins))
				return
			}
			for i, origin := range config.CORS.AllowedOrigins {
				if origin != tt.wantOrigins[i] {
					t.Errorf("CORS.AllowedOrigins[%d] = %s, want %s", i, origin, tt.wantOrigins[i])
				}
			}
		})
	}
}

func TestApplyEnvOverridesEnvironment(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		initialEnv string
		wantEnv    string
	}{
		{
			name:       "development environment from env",
			envValue:   "development",
			initialEnv: "production",
			wantEnv:    "development",
		},
		{
			name:       "production environment from env",
			envValue:   "production",
			initialEnv: "development",
			wantEnv:    "production",
		},
		{
			name:       "empty env preserves config",
			envValue:   "",
			initialEnv: "staging",
			wantEnv:    "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("PULLBASE_ENV", tt.envValue)
				defer os.Unsetenv("PULLBASE_ENV")
			}

			config := &Config{}
			config.Environment = tt.initialEnv

			applyEnvOverrides(config)

			if config.Environment != tt.wantEnv {
				t.Errorf("Environment = %s, want %s", config.Environment, tt.wantEnv)
			}
		})
	}
}

func TestLoadConfigTLSFromEnv(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configJSON := `{
		"database": {"host": "localhost", "port": 5432, "user": "user", "password": "pass", "dbname": "db"},
		"server": {"port": 8080, "host": "0.0.0.0"},
		"tls": {"enabled": false, "cert_path": "", "key_path": ""},
		"jwt": {"secret": "test-secret", "expiry_hours": 24},
		"git": {"clone_path": "/tmp/git", "poll_interval_seconds": 60, "enabled": false},
		"migrations": {"path": "file://migrations"}
	}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	os.Setenv("PULLBASE_TLS_ENABLED", "false")
	os.Setenv("PULLBASE_TLS_CERT_PATH", "/env/cert.pem")
	os.Setenv("PULLBASE_TLS_KEY_PATH", "/env/key.pem")
	defer func() {
		os.Unsetenv("PULLBASE_TLS_ENABLED")
		os.Unsetenv("PULLBASE_TLS_CERT_PATH")
		os.Unsetenv("PULLBASE_TLS_KEY_PATH")
	}()

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if config.TLS.Enabled != false {
		t.Errorf("TLS.Enabled = %v, want false", config.TLS.Enabled)
	}
	if config.TLS.CertPath != "/env/cert.pem" {
		t.Errorf("TLS.CertPath = %s, want /env/cert.pem", config.TLS.CertPath)
	}
	if config.TLS.KeyPath != "/env/key.pem" {
		t.Errorf("TLS.KeyPath = %s, want /env/key.pem", config.TLS.KeyPath)
	}
}
