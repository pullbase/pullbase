package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const DefaultGitHubAPIBaseURL = "https://api.github.com"

// Config represents the application configuration
type Config struct {
	Environment string `json:"environment"`
	Database    struct {
		Type     string `json:"type"`
		Path     string `json:"path"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		DBName   string `json:"dbname"`
		SSLMode  string `json:"sslmode"`
		Dialect  string `json:"-"`
	} `json:"database"`
	Server struct {
		Port int    `json:"port"`
		Host string `json:"host"`
	} `json:"server"`
	TLS struct {
		Enabled  bool   `json:"enabled"`
		CertPath string `json:"cert_path"`
		KeyPath  string `json:"key_path"`
	} `json:"tls"`
	CORS struct {
		AllowedOrigins []string `json:"allowed_origins"` // Comma-separated origins, empty = same-origin only
	} `json:"cors"`
	JWT struct {
		Secret      string `json:"secret"`
		ExpiryHours int    `json:"expiry_hours"`
	} `json:"jwt"`
	Git struct {
		ClonePath    string        `json:"clone_path"`
		PollInterval time.Duration `json:"poll_interval_seconds"`
		Enabled      bool          `json:"enabled"`
	} `json:"git"`
	GitHubApp  GitHubAppConfig `json:"github_app"`
	Bootstrap  BootstrapConfig `json:"bootstrap"`
	Migrations struct {
		Path string `json:"path"`
	} `json:"migrations"`
}

// GitHubAppConfig holds configuration required to authenticate as a GitHub App.
type GitHubAppConfig struct {
	AppID          int64  `json:"app_id"`
	PrivateKeyPath string `json:"private_key_path"`
	APIBaseURL     string `json:"api_base_url"`
}

// BootstrapConfig captures configuration for the initial admin bootstrap procedure.
type BootstrapConfig struct {
	Secret     string `json:"secret"`
	SecretFile string `json:"secret_file"`
}

// LoadConfig loads the configuration from a JSON file and applies environment variable overrides
func LoadConfig(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		slog.Info("config file not found, creating default", "path", path)
		if err := CreateDefaultConfig(path); err != nil {
			return nil, fmt.Errorf("failed to create default config file: %w", err)
		}
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(file, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	applyEnvOverrides(&config)

	config.Database.Dialect = resolveDatabaseDialect(&config)

	if config.GitHubApp.APIBaseURL == "" {
		config.GitHubApp.APIBaseURL = DefaultGitHubAPIBaseURL
	}

	if config.GitHubApp.PrivateKeyPath != "" && !filepath.IsAbs(config.GitHubApp.PrivateKeyPath) {
		configDir := filepath.Dir(path)
		config.GitHubApp.PrivateKeyPath = filepath.Clean(filepath.Join(configDir, config.GitHubApp.PrivateKeyPath))
	}

	if config.Bootstrap.SecretFile != "" && !filepath.IsAbs(config.Bootstrap.SecretFile) {
		configDir := filepath.Dir(path)
		config.Bootstrap.SecretFile = filepath.Clean(filepath.Join(configDir, config.Bootstrap.SecretFile))
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// applyEnvOverrides updates the config struct with values from environment variables
func applyEnvOverrides(config *Config) {
	if dbType := os.Getenv("PULLBASE_DB_TYPE"); dbType != "" {
		config.Database.Type = dbType
	}
	if dbPath := os.Getenv("PULLBASE_DB_PATH"); dbPath != "" {
		config.Database.Path = dbPath
	}
	if host := os.Getenv("PULLBASE_DB_HOST"); host != "" {
		config.Database.Host = host
	}
	if portStr := os.Getenv("PULLBASE_DB_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			config.Database.Port = port
		} else {
			slog.Warn("invalid PULLBASE_DB_PORT value, using config file value", "value", portStr, "error", err)
		}
	}
	if user := os.Getenv("PULLBASE_DB_USER"); user != "" {
		config.Database.User = user
	}
	if password := os.Getenv("PULLBASE_DB_PASSWORD"); password != "" {
		config.Database.Password = password
	}
	if dbName := os.Getenv("PULLBASE_DB_NAME"); dbName != "" {
		config.Database.DBName = dbName
	}
	if sslMode := os.Getenv("PULLBASE_DB_SSLMODE"); sslMode != "" {
		config.Database.SSLMode = sslMode
	}

	if serverPortStr := os.Getenv("PULLBASE_SERVER_PORT"); serverPortStr != "" {
		if port, err := strconv.Atoi(serverPortStr); err == nil {
			config.Server.Port = port
		} else {
			slog.Warn("invalid PULLBASE_SERVER_PORT value, using config file value", "value", serverPortStr, "error", err)
		}
	}
	if serverHost := os.Getenv("PULLBASE_SERVER_HOST"); serverHost != "" {
		config.Server.Host = serverHost
	}

	if tlsEnabledStr := os.Getenv("PULLBASE_TLS_ENABLED"); tlsEnabledStr != "" {
		if enabled, err := strconv.ParseBool(tlsEnabledStr); err == nil {
			config.TLS.Enabled = enabled
		} else {
			slog.Warn("invalid PULLBASE_TLS_ENABLED value, using config file value", "value", tlsEnabledStr, "error", err)
		}
	}
	if tlsCertPath := os.Getenv("PULLBASE_TLS_CERT_PATH"); tlsCertPath != "" {
		config.TLS.CertPath = tlsCertPath
	}
	if tlsKeyPath := os.Getenv("PULLBASE_TLS_KEY_PATH"); tlsKeyPath != "" {
		config.TLS.KeyPath = tlsKeyPath
	}

	if jwtSecret := os.Getenv("PULLBASE_JWT_SECRET"); jwtSecret != "" {
		config.JWT.Secret = jwtSecret
	}
	if jwtExpiryStr := os.Getenv("PULLBASE_JWT_EXPIRY_HOURS"); jwtExpiryStr != "" {
		if expiry, err := strconv.Atoi(jwtExpiryStr); err == nil {
			config.JWT.ExpiryHours = expiry
		} else {
			slog.Warn("invalid PULLBASE_JWT_EXPIRY_HOURS value, using config file value", "value", jwtExpiryStr, "error", err)
		}
	}

	if gitPath := os.Getenv("PULLBASE_GIT_CLONE_PATH"); gitPath != "" {
		config.Git.ClonePath = gitPath
	}
	if gitIntervalStr := os.Getenv("PULLBASE_GIT_POLL_INTERVAL"); gitIntervalStr != "" {
		if interval, err := strconv.Atoi(gitIntervalStr); err == nil {
			config.Git.PollInterval = time.Duration(interval) * time.Second
		} else {
			slog.Warn("invalid PULLBASE_GIT_POLL_INTERVAL value, using config file value", "value", gitIntervalStr, "error", err)
		}
	}
	if gitEnabledStr := os.Getenv("PULLBASE_GIT_ENABLED"); gitEnabledStr != "" {
		if enabled, err := strconv.ParseBool(gitEnabledStr); err == nil {
			config.Git.Enabled = enabled
		} else {
			slog.Warn("invalid PULLBASE_GIT_ENABLED value, using config file value", "value", gitEnabledStr, "error", err)
		}
	}

	if appIDStr := os.Getenv("PULLBASE_GITHUB_APP_ID"); appIDStr != "" {
		if appID, err := strconv.ParseInt(appIDStr, 10, 64); err == nil {
			config.GitHubApp.AppID = appID
		} else {
			slog.Warn("invalid PULLBASE_GITHUB_APP_ID value, using config file value", "value", appIDStr, "error", err)
		}
	}
	if keyPath := os.Getenv("PULLBASE_GITHUB_APP_PRIVATE_KEY_PATH"); keyPath != "" {
		config.GitHubApp.PrivateKeyPath = keyPath
	}
	if apiBase := os.Getenv("PULLBASE_GITHUB_APP_API_BASE_URL"); apiBase != "" {
		config.GitHubApp.APIBaseURL = apiBase
	}

	if migrationsPath := os.Getenv("PULLBASE_MIGRATIONS_PATH"); migrationsPath != "" {
		config.Migrations.Path = migrationsPath
	}

	if bootstrapSecret := os.Getenv("PULLBASE_BOOTSTRAP_SECRET"); bootstrapSecret != "" {
		config.Bootstrap.Secret = bootstrapSecret
	}
	if bootstrapSecretFile := os.Getenv("PULLBASE_BOOTSTRAP_SECRET_FILE"); bootstrapSecretFile != "" {
		config.Bootstrap.SecretFile = bootstrapSecretFile
	}

	if env := os.Getenv("PULLBASE_ENV"); env != "" {
		config.Environment = env
	}
	if corsOrigins := os.Getenv("PULLBASE_CORS_ORIGINS"); corsOrigins != "" {
		origins := strings.Split(corsOrigins, ",")
		for i, o := range origins {
			origins[i] = strings.TrimSpace(o)
		}
		config.CORS.AllowedOrigins = origins
	}
}

func resolveDatabaseDialect(config *Config) string {
	dbType := strings.ToLower(strings.TrimSpace(config.Database.Type))
	switch dbType {
	case "postgres", "postgresql", "pg":
		return "postgres"
	case "sqlite", "sqlite3", "":
		return "sqlite"
	default:
		slog.Warn("unknown database type, defaulting to sqlite", "type", dbType)
		return "sqlite"
	}
}

func validateConfig(config *Config) error {
	if config.Database.Dialect == "postgres" {
		if config.Database.Host == "" {
			return fmt.Errorf("database host is required for PostgreSQL")
		}
		if config.Database.Port == 0 {
			return fmt.Errorf("database port is required for PostgreSQL")
		}
		if config.Database.User == "" {
			return fmt.Errorf("database user is required for PostgreSQL")
		}
		if config.Database.Password == "" {
			return fmt.Errorf("database password is required for PostgreSQL")
		}
		if config.Database.DBName == "" {
			return fmt.Errorf("database name is required for PostgreSQL")
		}
	}
	if config.Server.Port == 0 {
		return fmt.Errorf("server port is required")
	}
	if config.TLS.Enabled {
		if config.TLS.CertPath == "" {
			return fmt.Errorf("TLS certificate path is required when TLS is enabled")
		}
		if config.TLS.KeyPath == "" {
			return fmt.Errorf("TLS key path is required when TLS is enabled")
		}
	}
	if config.JWT.Secret == "" {
		return fmt.Errorf("JWT secret is required")
	}
	if config.JWT.ExpiryHours == 0 {
		return fmt.Errorf("JWT expiry hours is required")
	}
	if config.Git.ClonePath == "" {
		return fmt.Errorf("Git clone path is required")
	}
	if config.Git.PollInterval == 0 {
		return fmt.Errorf("Git poll interval is required")
	}
	if config.Git.Enabled {
		if err := ValidateGitHubAppConfig(config.GitHubApp); err != nil {
			return err
		}
	}
	if config.Migrations.Path == "" {
		return fmt.Errorf("migrations path is required")
	}
	return nil
}

func ValidateGitHubAppConfig(cfg GitHubAppConfig) error {
	if cfg.AppID <= 0 {
		return fmt.Errorf("GitHub App ID must be provided and greater than zero when Git integration is enabled")
	}

	if cfg.PrivateKeyPath == "" {
		return fmt.Errorf("GitHub App private key path must be provided when Git integration is enabled")
	}

	info, err := os.Stat(cfg.PrivateKeyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("GitHub App private key file %q does not exist", cfg.PrivateKeyPath)
		}
		return fmt.Errorf("failed to stat GitHub App private key file %q: %w", cfg.PrivateKeyPath, err)
	}

	if info.IsDir() {
		return fmt.Errorf("GitHub App private key path %q is a directory, expected file", cfg.PrivateKeyPath)
	}

	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("GitHub App private key file %q must not be group- or world-accessible (current permissions: %o)", cfg.PrivateKeyPath, perm)
	}

	if cfg.APIBaseURL == "" {
		return fmt.Errorf("GitHub App API base URL must be provided")
	}

	return nil
}

// LoadPrivateKey reads the GitHub App private key from disk.
func (cfg GitHubAppConfig) LoadPrivateKey() ([]byte, error) {
	if cfg.PrivateKeyPath == "" {
		return nil, fmt.Errorf("GitHub App private key path is empty")
	}

	pemBytes, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read GitHub App private key %q: %w", cfg.PrivateKeyPath, err)
	}

	return pemBytes, nil
}

// CreateDefaultConfig creates a default configuration file
func CreateDefaultConfig(path string) error {
	config := Config{}

	config.Environment = "production"
	config.CORS.AllowedOrigins = []string{}

	config.Database.Type = "sqlite"
	config.Database.Path = "pullbase.db"
	config.Database.Host = "localhost"
	config.Database.Port = 5432
	config.Database.User = "postgres"
	config.Database.Password = "postgres"
	config.Database.DBName = "pullbase"
	config.Database.SSLMode = "disable"

	config.Server.Port = 8080
	config.Server.Host = "0.0.0.0"

	config.TLS.Enabled = true
	config.TLS.CertPath = "certs/server.crt"
	config.TLS.KeyPath = "certs/server.key"

	config.JWT.Secret = "change-me-in-production"
	config.JWT.ExpiryHours = 24

	config.Git.ClonePath = "./git-repos"
	config.Git.PollInterval = 60 * time.Second
	config.Git.Enabled = true

	config.GitHubApp.AppID = 0
	config.GitHubApp.PrivateKeyPath = ""
	config.GitHubApp.APIBaseURL = DefaultGitHubAPIBaseURL

	config.Bootstrap.Secret = ""
	config.Bootstrap.SecretFile = "bootstrap-admin-secret.txt"

	config.Migrations.Path = "file://migrations"

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return fmt.Errorf("error marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}
