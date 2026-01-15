package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pullbase/pullbase/server/pkg/auth"
	appconfig "github.com/pullbase/pullbase/server/pkg/config"
	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/githubapp"
	"github.com/pullbase/pullbase/server/pkg/gitmonitor"
	"github.com/pullbase/pullbase/server/pkg/logging"
	"github.com/pullbase/pullbase/server/pkg/notifications"
	"github.com/pullbase/pullbase/server/pkg/rollback"
	"github.com/pullbase/pullbase/server/pkg/server"
)

func generateBootstrapSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate bootstrap secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func writeBootstrapSecretFile(path, secret string) (err error) {
	if path == "" {
		return errors.New("bootstrap secret file path is empty")
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("failed to create bootstrap secret directory %q: %w", dir, err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open bootstrap secret file %q: %w", path, err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			if err == nil {
				err = fmt.Errorf("failed to close bootstrap secret file %q: %w", path, cerr)
			} else {
				logging.Warn("failed to close bootstrap secret file", "path", path, "error", cerr)
			}
		}
	}()
	if _, err = file.WriteString(secret + "\n"); err != nil {
		return fmt.Errorf("failed to write bootstrap secret file %q: %w", path, err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("failed to flush bootstrap secret file %q: %w", path, err)
	}
	if err = file.Chmod(0o600); err != nil {
		return fmt.Errorf("failed to set permissions on bootstrap secret file %q: %w", path, err)
	}
	return nil
}

func generateSelfSignedCerts(certPath, keyPath string) error {
	certDir := filepath.Dir(certPath)
	if certDir != "." {
		if err := os.MkdirAll(certDir, 0o755); err != nil {
			return fmt.Errorf("failed to create certificate directory %q: %w", certDir, err)
		}
	}

	keyDir := filepath.Dir(keyPath)
	if keyDir != "." && keyDir != certDir {
		if err := os.MkdirAll(keyDir, 0o755); err != nil {
			return fmt.Errorf("failed to create key directory %q: %w", keyDir, err)
		}
	}

	privateKey, err := generatePrivateKey()
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	certPEM, keyPEM, err := generateCertificate(privateKey)
	if err != nil {
		return fmt.Errorf("failed to generate certificate: %w", err)
	}

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}

	return nil
}

func generatePrivateKey() (any, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func generateCertificate(privateKey any) ([]byte, []byte, error) {
	ecKey, ok := privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("expected ECDSA private key")
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Pullbase Development"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &ecKey.PublicKey, ecKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

func readBootstrapSecretFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("bootstrap secret file %q is a directory", path)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return "", fmt.Errorf("bootstrap secret file %q must not be group- or world-accessible (current permissions: %o)", path, perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read bootstrap secret file %q: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func main() {
	var generateDevCerts bool
	flag.BoolVar(&generateDevCerts, "generate-dev-certs", false, "Generate self-signed development certificates and exit")
	flag.Parse()

	if generateDevCerts {
		if err := generateSelfSignedCerts("certs/server.crt", "certs/server.key"); err != nil {
			logging.Error("failed to generate development certificates", "error", err)
			os.Exit(1)
		}
		logging.Info("development certificates generated successfully",
			"certificate", "certs/server.crt",
			"key", "certs/server.key",
			"valid_for", "localhost, 127.0.0.1",
			"expires", "1 year from now")
		return
	}

	cfg, err := appconfig.LoadConfig("config/config.json")
	if err != nil {
		logging.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()

	dbDialect, err := database.ParseDialect(cfg.Database.Dialect)
	if err != nil {
		logging.Error("invalid database dialect", "error", err)
		os.Exit(1)
	}

	dbConfig := database.Config{
		Dialect:      dbDialect,
		Host:         cfg.Database.Host,
		Port:         cfg.Database.Port,
		User:         cfg.Database.User,
		Password:     cfg.Database.Password,
		DatabaseName: cfg.Database.DBName,
		SSLMode:      cfg.Database.SSLMode,
		Path:         cfg.Database.Path,
	}
	db, dialect, err := database.New(dbConfig)
	if err != nil {
		logging.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.InitSchema(dbCtx, db, dialect, cfg.Migrations.Path); err != nil {
		logging.Error("failed to initialize database schema", "error", err)
		os.Exit(1)
	}
	logging.Info("database schema initialized", "dialect", dialect)

	repo := database.NewRepository(db, dialect)
	envRepo := database.NewEnvironmentRepository(db, dialect)

	authService, err := auth.NewService(cfg.JWT.Secret, cfg.JWT.ExpiryHours)
	if err != nil {
		logging.Error("failed to initialize auth service", "error", err)
		os.Exit(1)
	}

	var environmentMonitor *gitmonitor.EnvironmentMonitor
	var rollbackGitMonitor rollback.GitMonitor
	var installationTokenProvider gitmonitor.InstallationTokenProvider
	logger := logging.NewLogger(logging.Options{Format: "text", Output: os.Stdout})

	if cfg.Git.Enabled {
		encryptionKeyHex := os.Getenv("PULLBASE_ENCRYPTION_KEY")
		if encryptionKeyHex == "" {
			logging.Error("PULLBASE_ENCRYPTION_KEY environment variable is required")
			os.Exit(1)
		}

		encryptionKey, err := hex.DecodeString(encryptionKeyHex)
		if err != nil {
			logging.Error("failed to decode encryption key", "error", err)
			os.Exit(1)
		}

		privateKeyPEM, err := cfg.GitHubApp.LoadPrivateKey()
		if err != nil {
			logging.Error("failed to load GitHub App private key", "error", err)
			os.Exit(1)
		}

		githubClient, err := githubapp.NewClient(githubapp.Config{
			AppID:         cfg.GitHubApp.AppID,
			PrivateKeyPEM: privateKeyPEM,
			APIBaseURL:    cfg.GitHubApp.APIBaseURL,
		})
		if err != nil {
			logging.Error("failed to initialise GitHub App client", "error", err)
			os.Exit(1)
		}
		installationTokenProvider = githubClient

		webhookRouter := gitmonitor.NewWebhookRouter(logger, nil)
		webhookManager := gitmonitor.NewWebhookManager(webhookRouter, logger, envRepo)

		environmentMonitor = gitmonitor.NewEnvironmentMonitor(webhookManager, logger, encryptionKey, envRepo, repo, installationTokenProvider)
		environmentMonitor.RegisterProvider(gitmonitor.ProviderGitHub, gitmonitor.NewGitHubProvider())

		webhookRouter.SetMonitor(environmentMonitor)
		webhookRouter.RegisterHandler(gitmonitor.ProviderGitHub, environmentMonitor)

		if err := environmentMonitor.LoadEnvironmentsFromDatabase(dbCtx); err != nil {
			logging.Warn("failed to load environments from database", "error", err)
		}

		go environmentMonitor.StartRetryWorker(dbCtx)
		logging.Info("environment monitor started successfully")

		rollbackGitMonitor = gitmonitor.NewRollbackGitMonitorAdapter(environmentMonitor)
	} else {
		logging.Info("git monitor is disabled in configuration")
	}

	rollbackService := rollback.NewService(repo, rollbackGitMonitor, logger)
	rollbackHandlers := server.NewRollbackHandlers(rollbackService)

	if environmentMonitor != nil {
		environmentMonitor.SetRollbackService(rollbackService)
	}

	var webhookHandlers *server.WebhookHandlers
	if environmentMonitor != nil {
		webhookHandlers = server.NewWebhookHandlers(environmentMonitor, logger)
	}

	api := &server.API{
		Repo:             repo,
		Auth:             authService,
		RollbackHandlers: rollbackHandlers,
		WebhookHandlers:  webhookHandlers,
		TokenProvider:    installationTokenProvider,
		Notifications:    notifications.NewService(),
		Logger:           logger,
	}

	adminCheckCtx, adminCheckCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer adminCheckCancel()

	hasAdmin, err := repo.HasActiveAdmin(adminCheckCtx)
	if err != nil {
		logging.Error("failed to verify existing admin users", "error", err)
		os.Exit(1)
	}

	if !hasAdmin {
		bootstrapSecret := strings.TrimSpace(cfg.Bootstrap.Secret)
		secretLoadedFromFile := false

		if bootstrapSecret == "" && cfg.Bootstrap.SecretFile != "" {
			secret, err := readBootstrapSecretFile(cfg.Bootstrap.SecretFile)
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					logging.Error("failed to load bootstrap secret file", "path", cfg.Bootstrap.SecretFile, "error", err)
					os.Exit(1)
				}
			} else if secret != "" {
				bootstrapSecret = secret
				secretLoadedFromFile = true
			}
		}

		if bootstrapSecret == "" {
			secret, err := generateBootstrapSecret()
			if err != nil {
				logging.Error("failed to generate bootstrap secret", "error", err)
				os.Exit(1)
			}
			bootstrapSecret = secret

			if cfg.Bootstrap.SecretFile == "" {
				logging.Error("generated bootstrap secret but no PULLBASE_BOOTSTRAP_SECRET_FILE path is configured; set PULLBASE_BOOTSTRAP_SECRET or provide a writable secret file path")
				os.Exit(1)
			}

			if err := writeBootstrapSecretFile(cfg.Bootstrap.SecretFile, bootstrapSecret); err != nil {
				logging.Error("failed to persist bootstrap secret; ensure the path is writable or supply PULLBASE_BOOTSTRAP_SECRET", "path", cfg.Bootstrap.SecretFile, "error", err)
				os.Exit(1)
			}
			logging.Info("admin bootstrap secret written", "path", cfg.Bootstrap.SecretFile, "permissions", "600")
		} else if secretLoadedFromFile {
			logging.Info("admin bootstrap secret loaded from file", "path", cfg.Bootstrap.SecretFile)
		} else {
			logging.Info("admin bootstrap secret provided via configuration/environment")
		}

		api.EnableBootstrap(bootstrapSecret, cfg.Bootstrap.SecretFile)
		bootstrapSecret = ""
	} else {
		api.DisableBootstrap()
		if cfg.Bootstrap.SecretFile != "" {
			if err := os.Remove(cfg.Bootstrap.SecretFile); err != nil && !os.IsNotExist(err) {
				logging.Warn("failed to remove bootstrap secret file", "path", cfg.Bootstrap.SecretFile, "error", err)
			} else if err == nil {
				logging.Info("removed leftover bootstrap secret file because an admin already exists", "path", cfg.Bootstrap.SecretFile)
			}
		}
		logging.Info("admin user detected; bootstrap endpoint disabled")
	}

	if webhookHandlers != nil {
		webhookHandlers.SetAuditRecorder(api.RecordAuditLog)
	}

	corsConfig := server.CORSConfig{
		AllowedOrigins: cfg.CORS.AllowedOrigins,
		IsDevelopment:  cfg.Environment == "development",
	}

	r := server.SetupRoutes(api, authService, corsConfig)

	serverAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	httpServer := &http.Server{
		Addr:    serverAddr,
		Handler: r,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	if !cfg.TLS.Enabled {
		go func() {
			logging.Info("HTTP server starting", "address", serverAddr, "tls", false)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logging.Error("could not listen", "address", serverAddr, "error", err)
				os.Exit(1)
			}
		}()
	} else {
		certFile := cfg.TLS.CertPath
		keyFile := cfg.TLS.KeyPath

		certInfo, certErr := os.Stat(certFile)
		keyInfo, keyErr := os.Stat(keyFile)

		certMissing := os.IsNotExist(certErr)
		keyMissing := os.IsNotExist(keyErr)

		if certMissing && keyMissing {
			logging.Info("TLS enabled but certificates not found; generating self-signed development certificates", "cert", certFile, "key", keyFile)
			if err := generateSelfSignedCerts(certFile, keyFile); err != nil {
				logging.Error("failed to generate self-signed certificates", "error", err)
				os.Exit(1)
			}
			var statErr error
			certInfo, statErr = os.Stat(certFile)
			if statErr != nil {
				logging.Error("failed to stat generated certificate", "path", certFile, "error", statErr)
				os.Exit(1)
			}
			keyInfo, statErr = os.Stat(keyFile)
			if statErr != nil {
				logging.Error("failed to stat generated key", "path", keyFile, "error", statErr)
				os.Exit(1)
			}
		} else {
			if certErr != nil && !certMissing {
				logging.Error("failed to stat TLS certificate", "path", certFile, "error", certErr)
				os.Exit(1)
			}
			if keyErr != nil && !keyMissing {
				logging.Error("failed to stat TLS key", "path", keyFile, "error", keyErr)
				os.Exit(1)
			}
			if certMissing || keyMissing {
				logging.Error("TLS certificate/key not found; provide both or disable TLS", "cert_found", !certMissing, "key_found", !keyMissing)
				os.Exit(1)
			}
		}

		httpServer.TLSConfig = &tls.Config{
			ClientAuth: tls.NoClientCert,
			MinVersion: tls.VersionTLS12,
		}

		go func() {
			logging.Info("HTTPS server starting", "address", serverAddr, "tls", true, "cert_modtime", certInfo.ModTime(), "key_modtime", keyInfo.ModTime())
			if err := httpServer.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				logging.Error("could not listen", "address", serverAddr, "error", err)
				os.Exit(1)
			}
		}()
	}

	<-stopChan
	logging.Info("shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logging.Warn("server shutdown failed", "error", err)
	}

	logging.Info("server gracefully stopped")
}
