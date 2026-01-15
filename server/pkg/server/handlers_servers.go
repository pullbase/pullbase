package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/token"
)

// CreateServerRequest defines the payload for creating a server
type CreateServerRequest struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	EnvironmentID int64  `json:"environment_id"`
}

// CreateServerResponse defines the response structure when creating a server
type CreateServerResponse struct {
	*models.Server
	AgentToken       string `json:"agent_token"`
	InstallationInfo struct {
		Instructions string `json:"instructions"`
		ExampleCmd   string `json:"example_cmd"`
	} `json:"installation_info"`
}

// UpdateServerRequest defines the payload for updating a server
type UpdateServerRequest struct {
	Name string `json:"name"`
	// Git configuration is inherited from environment, not stored per-server
}

// CreateServerTokenRequest defines the payload for creating a new agent token
type CreateServerTokenRequest struct {
	Description string `json:"description"`
	ExpiresIn   *int   `json:"expires_in,omitempty"` // Optional expiration in days
}

// CreateServerTokenResponse defines the response for creating a new agent token
type CreateServerTokenResponse struct {
	*models.AgentToken
	Token            string `json:"token"` // The actual token (only returned once)
	InstallationInfo struct {
		Instructions string `json:"instructions"`
		ExampleCmd   string `json:"example_cmd"`
	} `json:"installation_info"`
}

const defaultAgentVersion = "latest"
const agentInstallScriptTemplate = `#!/bin/bash
set -euo pipefail

AGENT_VERSION="{{.AgentVersion}}"
SERVER_URL="{{.ServerURL}}"
AGENT_TOKEN="{{.AgentToken}}"
SERVER_ID="{{.ServerID}}"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/pullbase"
SERVICE_USER="pullbase"

log() { echo "[Pullbase] $1"; }
error() { echo "[Pullbase ERROR] $1" >&2; exit 1; }

check_root() {
    if [[ $EUID -ne 0 ]]; then
        error "This script must be run as root (use sudo)"
    fi
}

detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) error "Unsupported architecture: $arch" ;;
    esac
}

detect_os() {
    local os
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux) echo "linux" ;;
        darwin) echo "darwin" ;;
        *) error "Unsupported OS: $os" ;;
    esac
}

create_user() {
    if ! id "$SERVICE_USER" &>/dev/null; then
        log "Creating service user: $SERVICE_USER"
        useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER" || true
    fi
}

download_agent() {
    local os arch url
    os=$(detect_os)
    arch=$(detect_arch)

    if [[ "$AGENT_VERSION" == "latest" ]]; then
        url="https://github.com/pullbase/pullbase/releases/latest/download/pullbase-agent-${os}-${arch}"
    else
        url="https://github.com/pullbase/pullbase/releases/download/${AGENT_VERSION}/pullbase-agent-${os}-${arch}"
    fi

    log "Downloading Pullbase agent from: $url"
    if command -v curl &>/dev/null; then
        curl -fsSL "$url" -o "${INSTALL_DIR}/pullbase-agent" || error "Failed to download agent"
    elif command -v wget &>/dev/null; then
        wget -q "$url" -O "${INSTALL_DIR}/pullbase-agent" || error "Failed to download agent"
    else
        error "curl or wget is required to download the agent"
    fi

    chmod +x "${INSTALL_DIR}/pullbase-agent"
    log "Agent binary installed to ${INSTALL_DIR}/pullbase-agent"
}

setup_config() {
    log "Creating configuration directory: $CONFIG_DIR"
    mkdir -p "$CONFIG_DIR"
    chmod 750 "$CONFIG_DIR"
    chown root:$SERVICE_USER "$CONFIG_DIR"

    log "Writing agent configuration"
    cat > "${CONFIG_DIR}/agent.env" <<EOF
CENTRAL_SERVER_URL=${SERVER_URL}
AGENT_TOKEN=${AGENT_TOKEN}
SERVER_ID=${SERVER_ID}
EOF

    chmod 640 "${CONFIG_DIR}/agent.env"
    chown root:$SERVICE_USER "${CONFIG_DIR}/agent.env"
{{if .CACert}}
    log "Installing CA certificate for TLS verification"
    cat > "${CONFIG_DIR}/ca.crt" <<'CACERT'
{{.CACert}}
CACERT
    chmod 644 "${CONFIG_DIR}/ca.crt"
    echo "CA_CERT_PATH=${CONFIG_DIR}/ca.crt" >> "${CONFIG_DIR}/agent.env"
{{end}}
}

setup_systemd() {
    log "Creating systemd service"
    cat > /etc/systemd/system/pullbase-agent.service <<EOF
[Unit]
Description=Pullbase Agent - GitOps for Servers
Documentation=https://docs.pullbase.io
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
EnvironmentFile=${CONFIG_DIR}/agent.env
ExecStart=${INSTALL_DIR}/pullbase-agent
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=pullbase-agent

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/pullbase

[Install]
WantedBy=multi-user.target
EOF

    mkdir -p /var/lib/pullbase
    chown $SERVICE_USER:$SERVICE_USER /var/lib/pullbase

    log "Reloading systemd daemon"
    systemctl daemon-reload

    log "Enabling and starting pullbase-agent service"
    systemctl enable pullbase-agent
    systemctl start pullbase-agent
}

verify_installation() {
    sleep 2
    if systemctl is-active --quiet pullbase-agent; then
        log "Pullbase agent is running successfully!"
        log ""
        log "Useful commands:"
        log "  View logs:     journalctl -u pullbase-agent -f"
        log "  Check status:  systemctl status pullbase-agent"
        log "  Stop agent:    systemctl stop pullbase-agent"
        log "  Restart agent: systemctl restart pullbase-agent"
    else
        log "Warning: Agent service may not have started correctly"
        log "Check logs with: journalctl -u pullbase-agent -n 50"
        exit 1
    fi
}

main() {
    log "Pullbase Agent Installer"
    log "========================"
    log "Server ID: $SERVER_ID"
    log "Server URL: $SERVER_URL"
    log "Agent Version: $AGENT_VERSION"
    log ""

    check_root
    create_user
    download_agent
    setup_config
    setup_systemd
    verify_installation

    log ""
    log "Installation complete! The agent is now connected to your Pullbase server."
}

main "$@"
`

type installScriptData struct {
	AgentVersion string
	ServerURL    string
	AgentToken   string
	ServerID     string
	CACert       string
}

// ListServersHandler retrieves a paginated list of servers.
//
//	@Summary		List all servers
//	@Description	Retrieves a paginated list of servers with their latest status
//	@Tags			Servers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			limit	query		int		false	"Maximum number of servers to return (1-500)"	default(100)
//	@Param			offset	query		int		false	"Number of servers to skip"						default(0)
//	@Param			sort	query		string	false	"Sort field (name, created_at, status)"
//	@Success		200		{array}		models.ServerWithStatus
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/servers [get]
func (api *API) ListServersHandler(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	sortParam := strings.ToLower(r.URL.Query().Get("sort"))

	limit := 100
	var err error
	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 500 {
			writeError(w, http.StatusBadRequest, "Invalid 'limit' query parameter: must be a positive integer <= 500")
			return
		}
	}

	offset := 0 // Default offset
	if offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "Invalid 'offset' query parameter: must be a non-negative integer")
			return
		}
	}

	// Use the function that joins server and status data
	serversWithStatus, err := api.Repo.ListServersWithLatestStatus(r.Context(), limit, offset, sortParam)
	if err != nil {
		api.log().Error("failed to retrieve servers with status", "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to retrieve servers")
		return
	}

	if serversWithStatus == nil {
		serversWithStatus = []models.ServerWithStatus{}
	}

	writeJSON(w, http.StatusOK, serversWithStatus)
}

// CreateServerHandler handles requests to create a new server
//
//	@Summary		Create a new server
//	@Description	Creates a new server with an initial agent token for deployment
//	@Tags			Servers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateServerRequest	true	"Server creation request"
//	@Success		201		{object}	CreateServerResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/servers [post]
func (api *API) CreateServerHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to create server")
		return
	}

	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	defer r.Body.Close()

	if req.ID == "" || req.Name == "" || req.EnvironmentID == 0 {
		writeError(w, http.StatusBadRequest, "Missing required fields: id, name, environment_id")
		return
	}

	// Verify environment exists
	_, err := api.Repo.GetEnvironment(r.Context(), req.EnvironmentID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Environment %d not found", req.EnvironmentID))
			return
		}
		api.log().Error("failed to verify environment", "environment_id", req.EnvironmentID, "username", claims.Username, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to verify environment")
		return
	}

	server, err := api.Repo.CreateServerWithID(r.Context(), req.ID, req.Name, req.EnvironmentID)
	if err != nil {
		if errors.Is(err, database.ErrConflict) {
			writeError(w, http.StatusConflict, fmt.Sprintf("Server with ID %s already exists", req.ID))
			return
		}
		api.log().Error("failed to create server", "username", claims.Username, "user_id", claims.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to create server")
		return
	}

	// Generate agent token for the server
	agentToken, err := token.GenerateToken()
	if err != nil {
		api.log().Error("failed to generate agent token", "server_id", server.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to generate agent token")
		return
	}

	tokenHash := token.HashToken(agentToken)
	description := token.GenerateTokenDescription(server.Name, "Default agent token")

	storedToken, err := api.Repo.CreateAgentToken(r.Context(), tokenHash, server.ID, description, nil, &claims.UserID)
	if err != nil {
		api.log().Error("failed to store agent token", "server_id", server.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to store agent token")
		return
	}

	// Create installation instructions
	serverURL := fmt.Sprintf("https://%s", r.Host)
	if r.TLS == nil {
		serverURL = fmt.Sprintf("http://%s", r.Host)
	}

	instructions := fmt.Sprintf(`1. Download and install the Pullbase agent on your server
2. Set the following environment variables:
   - CENTRAL_SERVER_URL=%s
   - AGENT_TOKEN=%s
   - SERVER_ID=%s
3. Start the agent service

The agent will automatically authenticate and begin monitoring your repository.`,
		serverURL, agentToken, server.ID)

	exampleCmd := fmt.Sprintf(`export CENTRAL_SERVER_URL=%s
export AGENT_TOKEN=%s
export SERVER_ID=%s
./pullbase-agent start`, serverURL, agentToken, server.ID)

	// Prepare response with token and installation info
	response := CreateServerResponse{
		Server:     server,
		AgentToken: agentToken,
		InstallationInfo: struct {
			Instructions string `json:"instructions"`
			ExampleCmd   string `json:"example_cmd"`
		}{
			Instructions: instructions,
			ExampleCmd:   exampleCmd,
		},
	}

	api.log().Info("server created", "server_id", server.ID, "username", claims.Username, "user_id", claims.UserID)
	api.RecordAuditLog(r, "create", "server", server.ID, map[string]interface{}{
		"environment_id": req.EnvironmentID,
		"agent_token_id": storedToken.ID,
	})
	writeJSON(w, http.StatusCreated, response)
}

// GetServerHandler retrieves details for a single server.
//
//	@Summary		Get server details
//	@Description	Retrieves detailed information about a specific server including its latest status
//	@Tags			Servers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string	true	"Server ID"
//	@Success		200			{object}	models.ServerWithStatus
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID} [get]
func (api *API) GetServerHandler(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	// Use the new function to get server and status together
	serverWithStatus, err := api.Repo.GetServerWithLatestStatusByID(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Server not found")
		} else {
			api.log().Error("failed to retrieve server with status", "server_id", serverID, "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to retrieve server")
		}
		return
	}

	writeJSON(w, http.StatusOK, serverWithStatus)
}

// UpdateServerHandler handles requests to update a server's configuration
//
//	@Summary		Update server
//	@Description	Updates a server's configuration (name only, git config inherited from environment)
//	@Tags			Servers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string				true	"Server ID"
//	@Param			request		body		UpdateServerRequest	true	"Server update request"
//	@Success		200			{object}	models.Server
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID} [put]
func (api *API) UpdateServerHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to update server")
		return
	}

	serverID := chi.URLParam(r, "serverID")

	if serverID == "" {
		writeError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	var req UpdateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	defer r.Body.Close()

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Missing required field: name")
		return
	}

	// Construct the model for update
	serverToUpdate := &models.Server{
		ID:   serverID,
		Name: req.Name,
	}

	err := api.Repo.UpdateServer(r.Context(), serverToUpdate)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Server not found")
		} else {
			api.log().Error("failed to update server", "server_id", serverID, "username", claims.Username, "user_id", claims.UserID, "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to update server")
		}
		return
	}

	// Fetch the updated server to return it
	updatedServer, err := api.Repo.GetServerByID(r.Context(), serverID)
	if err != nil {
		api.log().Warn("failed to fetch server after update", "server_id", serverID, "username", claims.Username, "user_id", claims.UserID, "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	api.log().Info("server updated", "server_id", serverID, "username", claims.Username, "user_id", claims.UserID)
	api.RecordAuditLog(r, "update", "server", serverID, map[string]interface{}{
		"name": req.Name,
	})
	writeJSON(w, http.StatusOK, updatedServer)
}

// DeleteServerHandler handles requests to delete a server
//
//	@Summary		Delete server
//	@Description	Soft-deletes a server and deactivates all its agent tokens
//	@Tags			Servers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path	string	true	"Server ID"
//	@Success		204			"No Content"
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID} [delete]
func (api *API) DeleteServerHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to delete server")
		return
	}

	serverID := chi.URLParam(r, "serverID")

	if serverID == "" {
		writeError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	err := api.Repo.DeleteServer(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Server not found")
		} else {
			api.log().Error("failed to delete server", "server_id", serverID, "username", claims.Username, "user_id", claims.UserID, "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to delete server")
		}
		return
	}

	api.log().Info("server deleted", "server_id", serverID, "username", claims.Username, "user_id", claims.UserID)
	api.RecordAuditLog(r, "delete", "server", serverID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ToggleAutoReconcileHandler handles requests to toggle auto-reconciliation for a server
//
//	@Summary		Toggle auto-reconcile
//	@Description	Toggles the auto-reconciliation setting for a server
//	@Tags			Servers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string	true	"Server ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID}/auto-reconcile [post]
func (api *API) ToggleAutoReconcileHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to toggle auto-reconcile")
		return
	}

	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	newState, err := api.Repo.ToggleServerAutoReconcile(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Server not found")
		} else {
			api.log().Error("failed to toggle auto-reconcile",
				"server_id", serverID, "username", claims.Username, "user_id", claims.UserID, "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to toggle auto-reconcile setting")
		}
		return
	}

	updatedServerModel, err := api.Repo.GetServerByID(r.Context(), serverID)
	if err != nil {
		api.log().Error("failed to fetch server after toggle", "server_id", serverID, "username", claims.Username, "user_id", claims.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to retrieve updated server status after toggle")
		return
	}

	api.log().Info("auto-reconcile toggled", "server_id", serverID, "new_state", updatedServerModel.AutoReconcile, "username", claims.Username, "user_id", claims.UserID)
	api.RecordAuditLog(r, "toggle_auto_reconcile", "server", serverID, map[string]interface{}{
		"auto_reconcile": newState,
	})

	response := map[string]interface{}{
		"auto_reconcile": updatedServerModel.AutoReconcile,
		"message": fmt.Sprintf("Auto-reconcile %s for server %s",
			map[bool]string{true: "enabled", false: "disabled"}[updatedServerModel.AutoReconcile],
			updatedServerModel.Name),
	}

	writeJSON(w, http.StatusOK, response)
}

// GetServerStatusHistoryHandler retrieves paginated agent status history for a server.
//
//	@Summary		Get server status history
//	@Description	Retrieves paginated history of agent status updates for a server
//	@Tags			Servers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string	true	"Server ID"
//	@Param			limit		query		int		false	"Number of entries per page (1-100)"	default(20)
//	@Param			page		query		int		false	"Page number"							default(1)
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID}/status-history [get]
func (api *API) GetServerStatusHistoryHandler(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	pageStr := r.URL.Query().Get("page")

	limit := 20
	var err error
	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 100 {
			writeError(w, http.StatusBadRequest, "Invalid 'limit' query parameter: must be a positive integer <= 100")
			return
		}
	}

	page := 1
	if pageStr != "" {
		page, err = strconv.Atoi(pageStr)
		if err != nil || page <= 0 {
			writeError(w, http.StatusBadRequest, "Invalid 'page' query parameter: must be a positive integer")
			return
		}
	}

	// Calculate offset from page
	offset := (page - 1) * limit

	statuses, err := api.Repo.GetAgentStatusHistory(r.Context(), serverID, limit, offset)
	if err != nil {
		api.log().Error("failed to retrieve status history", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to retrieve status history")
		return
	}

	if statuses == nil {
		statuses = []models.AgentStatus{}
	}

	// Get total count for pagination
	totalCount, err := api.Repo.CountAgentStatusHistory(r.Context(), serverID)
	if err != nil {
		api.log().Error("failed to count status history", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to retrieve status history count")
		return
	}

	// Calculate total pages
	totalPages := (totalCount + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	response := map[string]interface{}{
		"data":        statuses,
		"total":       totalCount,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	}

	writeJSON(w, http.StatusOK, response)
}

// ListServerTokensHandler retrieves all active agent tokens for a server
//
//	@Summary		List server tokens
//	@Description	Retrieves all active agent tokens for a server
//	@Tags			Tokens
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string	true	"Server ID"
//	@Success		200			{array}		models.AgentToken
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID}/tokens [get]
func (api *API) ListServerTokensHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to list server tokens")
		return
	}

	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	// Verify server exists
	_, err := api.Repo.GetServerByID(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Server not found")
			return
		}
		api.log().Error("failed to get server", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to verify server")
		return
	}

	tokens, err := api.Repo.ListAgentTokensByServer(r.Context(), serverID)
	if err != nil {
		api.log().Error("failed to list tokens", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to list agent tokens")
		return
	}

	if tokens == nil {
		tokens = []*models.AgentToken{}
	}

	api.log().Debug("user listed tokens", "username", claims.Username, "user_id", claims.UserID, "server_id", serverID)
	api.RecordAuditLog(r, "list_tokens", "server", serverID, nil)
	writeJSON(w, http.StatusOK, tokens)
}

// CreateServerTokenHandler creates a new agent token for a server
//
//	@Summary		Create server token
//	@Description	Creates a new agent token for a server. The token is only returned once.
//	@Tags			Tokens
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string					true	"Server ID"
//	@Param			request		body		CreateServerTokenRequest	true	"Token creation request"
//	@Success		201			{object}	CreateServerTokenResponse
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID}/tokens [post]
func (api *API) CreateServerTokenHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to create server token")
		return
	}

	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	var req CreateServerTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	defer r.Body.Close()

	if req.Description == "" {
		writeError(w, http.StatusBadRequest, "Token description is required")
		return
	}

	// Verify server exists
	server, err := api.Repo.GetServerByID(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Server not found")
			return
		}
		api.log().Error("failed to get server", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to verify server")
		return
	}

	// Generate new token
	agentToken, err := token.GenerateToken()
	if err != nil {
		api.log().Error("failed to generate agent token", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to generate agent token")
		return
	}

	// Calculate expiration if specified
	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		exp := time.Now().AddDate(0, 0, *req.ExpiresIn)
		expiresAt = &exp
	}

	// Hash and store the token
	tokenHash := token.HashToken(agentToken)
	storedToken, err := api.Repo.CreateAgentToken(r.Context(), tokenHash, serverID, req.Description, expiresAt, &claims.UserID)
	if err != nil {
		api.log().Error("failed to store agent token", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to store agent token")
		return
	}

	// Create installation instructions
	serverURL := fmt.Sprintf("https://%s", r.Host)
	if r.TLS == nil {
		serverURL = fmt.Sprintf("http://%s", r.Host)
	}

	instructions := fmt.Sprintf(`1. Download and install the Pullbase agent on your server
2. Set the following environment variables:
   - CENTRAL_SERVER_URL=%s
   - AGENT_TOKEN=%s
   - SERVER_ID=%s
3. Start the agent service

The agent will automatically authenticate and begin monitoring your repository.`,
		serverURL, agentToken, serverID)

	exampleCmd := fmt.Sprintf(`export CENTRAL_SERVER_URL=%s
export AGENT_TOKEN=%s
export SERVER_ID=%s
./pullbase-agent start`, serverURL, agentToken, serverID)

	response := CreateServerTokenResponse{
		AgentToken: storedToken,
		Token:      agentToken,
		InstallationInfo: struct {
			Instructions string `json:"instructions"`
			ExampleCmd   string `json:"example_cmd"`
		}{
			Instructions: instructions,
			ExampleCmd:   exampleCmd,
		},
	}

	api.log().Info("user created new token", "username", claims.Username, "user_id", claims.UserID, "server_id", serverID, "server_name", server.Name)
	api.RecordAuditLog(r, "create", "agent_token", strconv.Itoa(storedToken.ID), map[string]interface{}{
		"server_id":   serverID,
		"description": req.Description,
		"expires_at":  expiresAt,
	})
	writeJSON(w, http.StatusCreated, response)
}

// DeactivateServerTokenHandler deactivates an agent token
//
//	@Summary		Deactivate token
//	@Description	Deactivates an agent token, preventing further authentication
//	@Tags			Tokens
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string	true	"Server ID"
//	@Param			tokenID		path		int		true	"Token ID"
//	@Success		200			{object}	map[string]string
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID}/tokens/{tokenID} [delete]
func (api *API) DeactivateServerTokenHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to deactivate server token")
		return
	}

	serverID := chi.URLParam(r, "serverID")
	tokenIDStr := chi.URLParam(r, "tokenID")

	if serverID == "" || tokenIDStr == "" {
		writeError(w, http.StatusBadRequest, "Server ID and Token ID are required")
		return
	}

	tokenID, err := strconv.Atoi(tokenIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid token ID")
		return
	}

	// Verify the token belongs to the server
	tokens, err := api.Repo.ListAgentTokensByServer(r.Context(), serverID)
	if err != nil {
		api.log().Error("failed to list tokens", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to verify token ownership")
		return
	}

	tokenFound := false
	for _, t := range tokens {
		if t.ID == tokenID {
			tokenFound = true
			break
		}
	}

	if !tokenFound {
		writeError(w, http.StatusNotFound, "Token not found or does not belong to this server")
		return
	}

	// Deactivate the token
	err = api.Repo.DeactivateAgentToken(r.Context(), tokenID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Token not found")
			return
		}
		api.log().Error("failed to deactivate token", "token_id", tokenID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to deactivate token")
		return
	}

	api.log().Info("user deactivated token", "username", claims.Username, "user_id", claims.UserID, "token_id", tokenID, "server_id", serverID)
	api.RecordAuditLog(r, "deactivate", "agent_token", tokenIDStr, map[string]interface{}{
		"server_id": serverID,
	})
	writeJSON(w, http.StatusOK, map[string]string{"message": "Token deactivated successfully"})
}

// GetExpiringTokensHandler returns tokens expiring within the specified number of days
//
//	@Summary		Get expiring tokens
//	@Description	Returns tokens that will expire within the specified number of days
//	@Tags			Tokens
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			days	query		int	false	"Days until expiration (1-365)"	default(7)
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/tokens/expiring [get]
func (api *API) GetExpiringTokensHandler(w http.ResponseWriter, r *http.Request) {
	_, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied")
		return
	}

	daysStr := r.URL.Query().Get("days")
	days := 7
	if daysStr != "" {
		parsedDays, err := strconv.Atoi(daysStr)
		if err != nil || parsedDays < 1 || parsedDays > 365 {
			writeError(w, http.StatusBadRequest, "Invalid days parameter (must be 1-365)")
			return
		}
		days = parsedDays
	}

	tokens, err := api.Repo.GetExpiringTokens(r.Context(), days)
	if err != nil {
		api.log().Error("failed to get expiring tokens", "days", days, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to get expiring tokens")
		return
	}

	response := map[string]interface{}{
		"tokens":       tokens,
		"count":        len(tokens),
		"days_checked": days,
	}
	writeJSON(w, http.StatusOK, response)
}

// GetServerInstallInstructionsHandler provides installation instructions for a server
//
//	@Summary		Get install instructions
//	@Description	Returns installation instructions for deploying the agent on a server
//	@Tags			Servers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string	true	"Server ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID}/install-instructions [get]
func (api *API) GetServerInstallInstructionsHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to get installation instructions")
		return
	}

	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	// Verify server exists and get environment details
	serverWithEnv, err := api.Repo.GetServerWithEnvironment(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Server not found")
			return
		}
		api.log().Error("failed to get server", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to verify server")
		return
	}

	// Get active tokens for the server
	tokens, err := api.Repo.ListAgentTokensByServer(r.Context(), serverID)
	if err != nil {
		api.log().Error("failed to list tokens", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to get server tokens")
		return
	}

	serverURL := fmt.Sprintf("https://%s", r.Host)
	if r.TLS == nil {
		serverURL = fmt.Sprintf("http://%s", r.Host)
	}

	if len(tokens) == 0 {
		// No tokens available, suggest creating one
		instructions := fmt.Sprintf(`No active agent tokens found for server "%s".

To connect an agent to this server:
1. Create a new agent token from the server management interface
2. Follow the provided installation instructions

Server details:
- Server ID: %s
- Repository: %s
- Branch: %s
- Deploy Path: %s`, serverWithEnv.Name, serverID, serverWithEnv.RepoURL, serverWithEnv.Branch, serverWithEnv.DeployPath)

		response := map[string]interface{}{
			"server_id":    serverID,
			"server_name":  serverWithEnv.Name,
			"instructions": instructions,
			"has_tokens":   false,
		}

		writeJSON(w, http.StatusOK, response)
		return
	}

	// Use the most recent token for instructions
	recentToken := tokens[0]

	instructions := fmt.Sprintf(`Installation instructions for server "%s":

1. Download and install the Pullbase agent on your target server
2. Set the following environment variables:
   - CENTRAL_SERVER_URL=%s
   - AGENT_TOKEN=<your-agent-token>
   - SERVER_ID=%s
3. Start the agent service

Server details:
- Repository: %s
- Branch: %s
- Deploy Path: %s

Note: Use a valid agent token from the tokens list. Tokens are not displayed after creation for security reasons.`,
		serverWithEnv.Name, serverURL, serverID, serverWithEnv.RepoURL, serverWithEnv.Branch, serverWithEnv.DeployPath)

	exampleCmd := fmt.Sprintf(`export CENTRAL_SERVER_URL=%s
export AGENT_TOKEN=<your-agent-token>
export SERVER_ID=%s
./pullbase-agent start`, serverURL, serverID)

	response := map[string]interface{}{
		"server_id":            serverID,
		"server_name":          serverWithEnv.Name,
		"instructions":         instructions,
		"example_cmd":          exampleCmd,
		"has_tokens":           true,
		"active_tokens":        len(tokens),
		"latest_token_created": recentToken.CreatedAt,
	}

	api.log().Debug("user viewed installation instructions", "username", claims.Username, "user_id", claims.UserID, "server_id", serverID)
	api.RecordAuditLog(r, "view_install_instructions", "server", serverID, map[string]interface{}{
		"has_tokens": response["has_tokens"],
	})
	writeJSON(w, http.StatusOK, response)
}

// GetServerInstallScriptHandler generates a bash install script for the agent
//
//	@Summary		Get install script
//	@Description	Generates a bash script for installing the agent on a server
//	@Tags			Servers
//	@Produce		text/x-shellscript
//	@Security		BearerAuth
//	@Param			serverID	path		string	true	"Server ID"
//	@Param			token		query		string	true	"Agent token"
//	@Param			version		query		string	false	"Agent version"	default(latest)
//	@Param			ca_cert		query		string	false	"CA certificate for TLS verification"
//	@Success		200			{string}	string	"Bash install script"
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID}/install-script [get]
func (api *API) GetServerInstallScriptHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		http.Error(w, "Server ID is required", http.StatusBadRequest)
		return
	}

	agentToken := r.URL.Query().Get("token")
	if agentToken == "" {
		http.Error(w, "Agent token is required. Pass ?token=pbt_xxx", http.StatusBadRequest)
		return
	}

	if !token.ValidateTokenFormat(agentToken) {
		http.Error(w, "Invalid token format", http.StatusBadRequest)
		return
	}

	_, err := api.Repo.GetServerWithEnvironment(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			http.Error(w, "Server not found", http.StatusNotFound)
			return
		}
		api.log().Error("failed to get server", "server_id", serverID, "error", err)
		http.Error(w, "Failed to verify server", http.StatusInternalServerError)
		return
	}

	tokenHash := token.HashToken(agentToken)
	storedToken, err := api.Repo.GetAgentTokenByHash(r.Context(), tokenHash)
	if err != nil || storedToken == nil {
		http.Error(w, "Token not found or invalid", http.StatusUnauthorized)
		return
	}

	if storedToken.ServerID != serverID {
		http.Error(w, "Token does not belong to this server", http.StatusForbidden)
		return
	}

	if !storedToken.IsActive {
		http.Error(w, "Token is deactivated", http.StatusUnauthorized)
		return
	}

	if storedToken.ExpiresAt != nil && storedToken.ExpiresAt.Before(time.Now()) {
		http.Error(w, "Token has expired", http.StatusUnauthorized)
		return
	}

	serverURL := fmt.Sprintf("https://%s", r.Host)
	if r.TLS == nil {
		serverURL = fmt.Sprintf("http://%s", r.Host)
	}

	agentVersion := r.URL.Query().Get("version")
	if agentVersion == "" {
		agentVersion = defaultAgentVersion
	}

	caCert := r.URL.Query().Get("ca_cert")

	data := installScriptData{
		AgentVersion: agentVersion,
		ServerURL:    serverURL,
		AgentToken:   agentToken,
		ServerID:     serverID,
		CACert:       caCert,
	}

	script := api.renderInstallScript(data)

	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=pullbase-install-%s.sh", serverID))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))

	api.log().Info("user generated install script", "username", claims.Username, "user_id", claims.UserID, "server_id", serverID)
	api.RecordAuditLog(r, "generate_install_script", "server", serverID, map[string]interface{}{
		"agent_version": agentVersion,
		"has_ca_cert":   caCert != "",
	})
}

func (api *API) renderInstallScript(data installScriptData) string {
	script := agentInstallScriptTemplate
	script = strings.ReplaceAll(script, "{{.AgentVersion}}", data.AgentVersion)
	script = strings.ReplaceAll(script, "{{.ServerURL}}", data.ServerURL)
	script = strings.ReplaceAll(script, "{{.AgentToken}}", data.AgentToken)
	script = strings.ReplaceAll(script, "{{.ServerID}}", data.ServerID)

	if data.CACert != "" {
		script = strings.ReplaceAll(script, "{{if .CACert}}", "")
		script = strings.ReplaceAll(script, "{{end}}", "")
		script = strings.ReplaceAll(script, "{{.CACert}}", data.CACert)
	} else {
		startIdx := strings.Index(script, "{{if .CACert}}")
		endIdx := strings.Index(script, "{{end}}")
		if startIdx != -1 && endIdx != -1 {
			script = script[:startIdx] + script[endIdx+7:]
		}
	}

	return script
}

type DriftDetailsResponse struct {
	ServerID     string               `json:"server_id"`
	ServerName   string               `json:"server_name"`
	IsDrifted    bool                 `json:"is_drifted"`
	DriftDetails *models.DriftDetails `json:"drift_details,omitempty"`
	DetectedAt   *time.Time           `json:"detected_at,omitempty"`
	CommitHash   string               `json:"commit_hash,omitempty"`
}

// GetServerDriftHandler retrieves drift details for a server.
//
//	@Summary		Get server drift details
//	@Description	Retrieves detailed drift information for a server including packages, services, and files
//	@Tags			Servers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string	true	"Server ID"
//	@Success		200			{object}	DriftDetailsResponse
//	@Failure		400			{object}	ErrorResponse
//	@Failure		401			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/servers/{serverID}/drift [get]
func (api *API) GetServerDriftHandler(w http.ResponseWriter, r *http.Request) {
	_, authorized := requireRole(r, models.RoleAdmin, models.RoleUser, models.RoleViewer)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied")
		return
	}

	serverID := chi.URLParam(r, "serverID")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "Server ID is required")
		return
	}

	server, err := api.Repo.GetServerByID(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("Server '%s' not found", serverID))
		} else {
			api.log().Error("failed to get server", "server_id", serverID, "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to get server")
		}
		return
	}

	latestStatus, err := api.Repo.GetLatestAgentStatus(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeJSON(w, http.StatusOK, DriftDetailsResponse{
				ServerID:   serverID,
				ServerName: server.Name,
				IsDrifted:  false,
			})
			return
		}
		api.log().Error("failed to get latest status", "server_id", serverID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to get server status")
		return
	}

	response := DriftDetailsResponse{
		ServerID:     serverID,
		ServerName:   server.Name,
		IsDrifted:    latestStatus.IsDrifted,
		CommitHash:   latestStatus.CommitHash,
		DetectedAt:   &latestStatus.Timestamp,
		DriftDetails: latestStatus.DriftDetails,
	}

	writeJSON(w, http.StatusOK, response)
}
