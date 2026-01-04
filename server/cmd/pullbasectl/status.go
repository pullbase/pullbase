package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

type statusEntry struct {
	ServerID        string     `json:"server_id"`
	ServerName      string     `json:"server_name"`
	EnvironmentID   *int64     `json:"environment_id,omitempty"`
	EnvironmentName *string    `json:"environment_name,omitempty"`
	Status          *string    `json:"status,omitempty"`
	CommitHash      *string    `json:"commit_hash,omitempty"`
	IsDrifted       *bool      `json:"is_drifted,omitempty"`
	LastSeen        *time.Time `json:"last_seen,omitempty"`
	AutoReconcile   bool       `json:"auto_reconcile"`
}

type fleetStatusResponse struct {
	Servers      []statusEntry `json:"servers"`
	TotalServers int           `json:"total_servers"`
	HealthyCount int           `json:"healthy_count"`
	DriftedCount int           `json:"drifted_count"`
	ErrorCount   int           `json:"error_count"`
	UnknownCount int           `json:"unknown_count"`
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	serverURL := fs.String("server-url", "", "Pullbase server base URL")
	adminToken := fs.String("admin-token", "", "Admin JWT token")
	username := fs.String("username", "", "Admin username")
	password := fs.String("password", "", "Admin password")
	passwordFile := fs.String("password-file", "", "Path to file containing admin password")
	caCertPath := fs.String("ca-cert", "", "Path to CA certificate bundle")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "Skip TLS verification (NOT recommended)")

	serverID := fs.String("server-id", "", "Show status for specific server")
	environmentID := fs.Int64("environment-id", 0, "Show status for all servers in environment")
	all := fs.Bool("all", false, "Show fleet-wide status overview")
	output := fs.String("output", "table", "Output format: table or json")
	watch := fs.Bool("watch", false, "Continuously refresh status")
	watchInterval := fs.Int("interval", 5, "Refresh interval in seconds (with --watch)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetURL := strings.TrimSpace(*serverURL)
	if targetURL == "" {
		return errors.New("--server-url is required")
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	format := outputFormat(strings.ToLower(*output))
	if format != outputTable && format != outputJSON {
		return errors.New("--output must be 'table' or 'json'")
	}

	hasServerID := strings.TrimSpace(*serverID) != ""
	hasEnvID := *environmentID > 0
	hasAll := *all

	optCount := 0
	if hasServerID {
		optCount++
	}
	if hasEnvID {
		optCount++
	}
	if hasAll {
		optCount++
	}

	if optCount == 0 {
		return errors.New("one of --server-id, --environment-id, or --all is required")
	}
	if optCount > 1 {
		return errors.New("only one of --server-id, --environment-id, or --all can be specified")
	}

	client, token, err := resolveAdminCredentials(adminAuthConfig{
		ServerURL:    targetURL,
		AdminToken:   *adminToken,
		Username:     *username,
		Password:     *password,
		PasswordFile: *passwordFile,
		CACertPath:   *caCertPath,
		Insecure:     *insecureSkipVerify,
	})
	if err != nil {
		return err
	}

	if *watch {
		return runStatusWatch(client, token, targetURL, *serverID, *environmentID, *all, format, *watchInterval)
	}

	return runStatusOnce(client, token, targetURL, *serverID, *environmentID, *all, format)
}

func runStatusOnce(client *http.Client, token, targetURL, serverID string, environmentID int64, all bool, format outputFormat) error {
	status, err := fetchStatus(client, token, targetURL, serverID, environmentID, all)
	if err != nil {
		return err
	}
	return printStatus(status, format, all)
}

func runStatusWatch(client *http.Client, token, targetURL, serverID string, environmentID int64, all bool, format outputFormat, interval int) error {
	fmt.Printf("Watching status (refresh every %ds). Press Ctrl+C to stop.\n\n", interval)

	for {
		fmt.Print("\033[H\033[2J")

		status, err := fetchStatus(client, token, targetURL, serverID, environmentID, all)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching status: %v\n", err)
		} else {
			if err := printStatus(status, format, all); err != nil {
				fmt.Fprintf(os.Stderr, "Error printing status: %v\n", err)
			}
		}

		fmt.Printf("\nLast updated: %s (refresh every %ds)\n", time.Now().Format(time.RFC3339), interval)
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func fetchStatus(client *http.Client, token, targetURL, serverID string, environmentID int64, all bool) (*fleetStatusResponse, error) {
	var requestURL string
	values := url.Values{}

	if serverID != "" {
		requestURL = fmt.Sprintf("%s/api/v1/servers/%s", targetURL, url.PathEscape(serverID))
	} else {
		requestURL = fmt.Sprintf("%s/api/v1/servers", targetURL)
		if environmentID > 0 {
			values.Set("environment_id", fmt.Sprintf("%d", environmentID))
		}
	}

	if len(values) > 0 {
		requestURL += "?" + values.Encode()
	}

	resp, err := authorizedRequest(client, http.MethodGet, requestURL, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, readAPIError(resp)
	}

	if serverID != "" {
		var server serverResponse
		if err := json.NewDecoder(resp.Body).Decode(&server); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return serverToFleetStatus(server), nil
	}

	var listResp listServersResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return serversToFleetStatus(listResp.Servers), nil
}

func serverToFleetStatus(s serverResponse) *fleetStatusResponse {
	entry := statusEntry{
		ServerID:        s.ID,
		ServerName:      s.Name,
		EnvironmentID:   s.EnvironmentID,
		EnvironmentName: s.EnvironmentName,
		Status:          s.LastStatus,
		CommitHash:      s.LastCommitHash,
		IsDrifted:       s.LastIsDrifted,
		LastSeen:        s.LastTimestamp,
		AutoReconcile:   s.AutoReconcile,
	}

	status := &fleetStatusResponse{
		Servers:      []statusEntry{entry},
		TotalServers: 1,
	}

	classifyStatus(entry, status)
	return status
}

func serversToFleetStatus(servers []serverResponse) *fleetStatusResponse {
	status := &fleetStatusResponse{
		Servers:      make([]statusEntry, 0, len(servers)),
		TotalServers: len(servers),
	}

	for _, s := range servers {
		entry := statusEntry{
			ServerID:        s.ID,
			ServerName:      s.Name,
			EnvironmentID:   s.EnvironmentID,
			EnvironmentName: s.EnvironmentName,
			Status:          s.LastStatus,
			CommitHash:      s.LastCommitHash,
			IsDrifted:       s.LastIsDrifted,
			LastSeen:        s.LastTimestamp,
			AutoReconcile:   s.AutoReconcile,
		}
		status.Servers = append(status.Servers, entry)
		classifyStatus(entry, status)
	}

	return status
}

func classifyStatus(entry statusEntry, status *fleetStatusResponse) {
	if entry.Status == nil {
		status.UnknownCount++
		return
	}

	switch *entry.Status {
	case "Applied", "In Sync", "Dry-Run: In Sync":
		if entry.IsDrifted != nil && *entry.IsDrifted {
			status.DriftedCount++
		} else {
			status.HealthyCount++
		}
	case "Syncing", "Dry-Run: Syncing":
		status.HealthyCount++
	case "Error", "Failed":
		status.ErrorCount++
	case "Drifted", "Drift Detected", "Dry-Run: Drift Detected":
		status.DriftedCount++
	default:
		status.UnknownCount++
	}
}

func printStatus(status *fleetStatusResponse, format outputFormat, showSummary bool) error {
	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	if len(status.Servers) == 0 {
		fmt.Println("No servers found.")
		return nil
	}

	if showSummary && status.TotalServers > 1 {
		fmt.Printf("Fleet Status Summary\n")
		fmt.Printf("  Total:   %d servers\n", status.TotalServers)
		fmt.Printf("  Healthy: %d\n", status.HealthyCount)
		fmt.Printf("  Drifted: %d\n", status.DriftedCount)
		fmt.Printf("  Errors:  %d\n", status.ErrorCount)
		fmt.Printf("  Unknown: %d\n", status.UnknownCount)
		fmt.Println()
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVER\tENVIRONMENT\tSTATUS\tDRIFTED\tCOMMIT\tLAST SEEN")

	for _, s := range status.Servers {
		env := "-"
		if s.EnvironmentName != nil {
			env = *s.EnvironmentName
		}
		st := "-"
		if s.Status != nil {
			st = *s.Status
		}
		drifted := "-"
		if s.IsDrifted != nil {
			if *s.IsDrifted {
				drifted = "yes"
			} else {
				drifted = "no"
			}
		}
		commit := "-"
		if s.CommitHash != nil && *s.CommitHash != "" {
			c := *s.CommitHash
			if len(c) > 7 {
				c = c[:7]
			}
			commit = c
		}
		lastSeen := "-"
		if s.LastSeen != nil {
			lastSeen = formatRelativeTime(*s.LastSeen)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", s.ServerID, env, st, drifted, commit, lastSeen)
	}
	w.Flush()

	return nil
}

func formatRelativeTime(t time.Time) string {
	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
