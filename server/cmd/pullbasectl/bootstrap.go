package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	appconfig "github.com/pullbase/pullbase/server/pkg/config"
	"golang.org/x/term"
)

func promptWithDefault(reader *bufio.Reader, label, defaultValue string, required bool) (string, error) {
	for {
		if defaultValue != "" {
			fmt.Printf("%s [%s]: ", label, defaultValue)
		} else {
			fmt.Printf("%s: ", label)
		}
		value, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			if defaultValue != "" {
				return defaultValue, nil
			}
			if required {
				fmt.Println("Value is required.")
				continue
			}
		}
		return value, nil
	}
}

func promptPassword(label string) (string, error) {
	fmt.Printf("%s: ", label)
	bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes)), nil
}

func promptConfirm(reader *bufio.Reader, message string, defaultYes bool) (bool, error) {
	defaultIndicator := "y/N"
	if defaultYes {
		defaultIndicator = "Y/n"
	}
	for {
		fmt.Printf("%s [%s]: ", message, defaultIndicator)
		value, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		value = strings.TrimSpace(strings.ToLower(value))
		switch value {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Println("Please answer 'y' or 'n'.")
		}
	}
}

func runBootstrapWizard(args []string) error {
	fs := flag.NewFlagSet("bootstrap wizard", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	bootstrapSecret := fs.String("bootstrap-secret", "", "Bootstrap secret value")
	bootstrapSecretFile := fs.String("bootstrap-secret-file", "", "Path to file containing the bootstrap secret")
	adminUsername := fs.String("admin-username", "", "Admin username to create or reuse")
	adminPassword := fs.String("admin-password", "", "Admin password")
	adminPasswordFile := fs.String("admin-password-file", "", "Path to file containing the admin password")
	appID := fs.Int64("app-id", 0, "GitHub App ID")
	privateKeyPath := fs.String("private-key", "", "Path to GitHub App private key PEM")
	installationID := fs.Int64("installation-id", 0, "GitHub App installation ID with repo access")
	apiBaseURL := fs.String("api-base-url", appconfig.DefaultGitHubAPIBaseURL, "GitHub API base URL")
	environmentName := fs.String("environment-name", "", "Environment name")
	repoURL := fs.String("repo-url", "", "Git repository URL")
	branch := fs.String("branch", "main", "Git branch for environment")
	deployPath := fs.String("deploy-path", "config.yaml", "Config path inside repository")
	webhookSecret := fs.String("webhook-secret", "", "Optional webhook secret override")
	appSlug := fs.String("app-slug", "", "Optional GitHub App slug")
	repositoryID := fs.Int64("repository-id", 0, "Optional GitHub repository ID")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS certificate verification (NOT recommended)")
	nonInteractive := fs.Bool("non-interactive", false, "Fail instead of prompting for missing values")

	if err := fs.Parse(args); err != nil {
		return err
	}

	interactive := !*nonInteractive && term.IsTerminal(int(os.Stdin.Fd()))
	reader := bufio.NewReader(os.Stdin)

	url := strings.TrimSpace(*serverURL)
	if url == "" {
		if !interactive {
			return errors.New("--server-url is required")
		}
		value, err := promptWithDefault(reader, "Pullbase server URL", "https://localhost:8080", true)
		if err != nil {
			return err
		}
		url = value
	}
	url = strings.TrimSuffix(url, "/")

	client, err := newHTTPClient(strings.TrimSpace(*caCertPath), *insecureSkipVerify)
	if err != nil {
		return err
	}

	secret := strings.TrimSpace(*bootstrapSecret)
	if secret == "" && strings.TrimSpace(*bootstrapSecretFile) != "" {
		value, err := loadSensitiveValue(*bootstrapSecretFile)
		if err != nil {
			return fmt.Errorf("failed to read bootstrap secret file: %w", err)
		}
		secret = value
	}
	if secret == "" {
		if !interactive {
			return errors.New("bootstrap secret is required")
		}
		value, err := promptWithDefault(reader, "Bootstrap secret (paste value)", "", false)
		if err != nil {
			return err
		}
		if value == "" {
			path, err := promptWithDefault(reader, "Path to bootstrap secret file", "", true)
			if err != nil {
				return err
			}
			loaded, err := loadSensitiveValue(path)
			if err != nil {
				return fmt.Errorf("failed to read bootstrap secret file: %w", err)
			}
			secret = loaded
		} else {
			secret = value
		}
	}

	username := strings.TrimSpace(*adminUsername)
	if username == "" {
		if !interactive {
			return errors.New("--admin-username is required")
		}
		value, err := promptWithDefault(reader, "Admin username", "pullbase-admin", true)
		if err != nil {
			return err
		}
		username = value
	}
	if !bootstrapUsernamePattern.MatchString(username) {
		return fmt.Errorf("admin username %q does not meet validation requirements", username)
	}

	password := strings.TrimSpace(*adminPassword)
	if password == "" && strings.TrimSpace(*adminPasswordFile) != "" {
		loaded, err := loadSensitiveValue(*adminPasswordFile)
		if err != nil {
			return fmt.Errorf("failed to read admin password file: %w", err)
		}
		password = loaded
	}
	if password == "" {
		if !interactive {
			return errors.New("admin password is required")
		}
		value, err := promptPassword("Admin password")
		if err != nil {
			return err
		}
		password = value
	}
	if utf8.RuneCountInString(password) < bootstrapPasswordMinLength {
		return fmt.Errorf("admin password must be at least %d characters", bootstrapPasswordMinLength)
	}

	bootstrapResp, status, err := bootstrapAdmin(url, secret, username, password, client)
	var adminToken string
	if err != nil {
		if status == http.StatusGone {
			fmt.Println("Admin already exists. Proceeding with provided credentials...")
			loginResp, loginErr := loginAdmin(url, username, password, client)
			if loginErr != nil {
				return loginErr
			}
			adminToken = loginResp.AccessToken
		} else {
			return err
		}
	} else {
		fmt.Printf("Admin user '%s' created successfully.\n", bootstrapResp.User.Username)
		adminToken = bootstrapResp.AccessToken
	}

	opts := githubAppBootstrapOptions{
		AppID:           *appID,
		PrivateKeyPath:  *privateKeyPath,
		InstallationID:  *installationID,
		APIBaseURL:      *apiBaseURL,
		ServerURL:       url,
		AdminToken:      adminToken,
		EnvironmentName: *environmentName,
		RepoURL:         *repoURL,
		Branch:          *branch,
		DeployPath:      *deployPath,
		WebhookSecret:   *webhookSecret,
		AppSlug:         *appSlug,
		RepositoryID:    *repositoryID,
		HTTPClient:      client,
	}

	if opts.AppID == 0 && interactive {
		value, err := promptWithDefault(reader, "GitHub App ID", "", true)
		if err != nil {
			return err
		}
		id, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("invalid GitHub App ID: %w", parseErr)
		}
		opts.AppID = id
	}
	if opts.AppID == 0 {
		return errors.New("GitHub App ID is required")
	}

	if strings.TrimSpace(opts.PrivateKeyPath) == "" {
		if !interactive {
			return errors.New("--private-key is required")
		}
		value, err := promptWithDefault(reader, "Path to GitHub App private key (PEM)", "", true)
		if err != nil {
			return err
		}
		opts.PrivateKeyPath = value
	}

	if opts.InstallationID == 0 {
		if !interactive {
			return errors.New("--installation-id is required")
		}
		value, err := promptWithDefault(reader, "GitHub App installation ID", "", true)
		if err != nil {
			return err
		}
		id, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("invalid installation ID: %w", parseErr)
		}
		opts.InstallationID = id
	}

	if strings.TrimSpace(opts.EnvironmentName) == "" {
		if !interactive {
			return errors.New("--environment-name is required")
		}
		value, err := promptWithDefault(reader, "Environment name", "production", true)
		if err != nil {
			return err
		}
		opts.EnvironmentName = value
	}

	if strings.TrimSpace(opts.RepoURL) == "" {
		if !interactive {
			return errors.New("--repo-url is required")
		}
		value, err := promptWithDefault(reader, "Git repository URL", "", true)
		if err != nil {
			return err
		}
		opts.RepoURL = value
	}

	if interactive {
		if strings.TrimSpace(opts.Branch) == "" {
			value, err := promptWithDefault(reader, "Git branch", "main", true)
			if err != nil {
				return err
			}
			opts.Branch = value
		}
		if strings.TrimSpace(opts.DeployPath) == "" {
			value, err := promptWithDefault(reader, "Deploy path", "config.yaml", true)
			if err != nil {
				return err
			}
			opts.DeployPath = value
		}
		if opts.RepositoryID == 0 {
			value, err := promptWithDefault(reader, "GitHub repository ID (optional)", "", false)
			if err != nil {
				return err
			}
			if strings.TrimSpace(value) != "" {
				id, parseErr := strconv.ParseInt(value, 10, 64)
				if parseErr != nil {
					return fmt.Errorf("invalid repository ID: %w", parseErr)
				}
				opts.RepositoryID = id
			}
		}
		if strings.TrimSpace(opts.AppSlug) == "" {
			value, err := promptWithDefault(reader, "GitHub App slug (optional)", "", false)
			if err != nil {
				return err
			}
			opts.AppSlug = value
		}
		if strings.TrimSpace(opts.WebhookSecret) == "" {
			enterSecret, err := promptConfirm(reader, "Provide custom webhook secret?", false)
			if err != nil {
				return err
			}
			if enterSecret {
				value, err := promptWithDefault(reader, "Webhook secret", "", true)
				if err != nil {
					return err
				}
				opts.WebhookSecret = value
			}
		}
	}

	if err := performGitHubAppBootstrap(opts); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Bootstrap completed successfully! Summary:")
	fmt.Printf("  Server URL: %s\n", url)
	fmt.Printf("  Admin user: %s\n", username)
	if strings.TrimSpace(opts.EnvironmentName) != "" {
		fmt.Printf("  Environment: %s (%s)\n", opts.EnvironmentName, opts.RepoURL)
	}
	fmt.Println("You can now log into the Pullbase dashboard using the admin credentials provided.")
	return nil
}
