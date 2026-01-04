package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/pullbase/pullbase/server/pkg/models"
)

type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     int       `json:"value"`
}

type DriftMetricsResponse struct {
	Period      string            `json:"period"`
	TotalEvents int               `json:"total_events"`
	TimeSeries  []TimeSeriesPoint `json:"time_series"`
}

type ReconciliationMetricsResponse struct {
	Period       string            `json:"period"`
	TotalApplied int               `json:"total_applied"`
	TotalFailed  int               `json:"total_failed"`
	SuccessRate  float64           `json:"success_rate"`
	TimeSeries   []TimeSeriesPoint `json:"time_series"`
}

type AgentConnectivityResponse struct {
	TotalAgents    int                      `json:"total_agents"`
	OnlineAgents   int                      `json:"online_agents"`
	OfflineAgents  int                      `json:"offline_agents"`
	StaleThreshold string                   `json:"stale_threshold"`
	AgentStatuses  []AgentConnectivityEntry `json:"agent_statuses"`
}

type AgentConnectivityEntry struct {
	ServerID   string     `json:"server_id"`
	ServerName string     `json:"server_name"`
	LastSeen   *time.Time `json:"last_seen,omitempty"`
	IsOnline   bool       `json:"is_online"`
	Status     string     `json:"status,omitempty"`
}

// GetDriftMetricsHandler godoc
//
//	@Summary		Get drift metrics
//	@Description	Get drift event metrics over time.
//	@Tags			Metrics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			days	query		int	false	"Number of days"	default(7)
//	@Success		200		{object}	DriftMetricsResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Router			/metrics/drift [get]
func (api *API) GetDriftMetricsHandler(w http.ResponseWriter, r *http.Request) {
	_, authorized := requireRole(r, models.RoleAdmin, models.RoleUser, models.RoleViewer)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied")
		return
	}

	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 90 {
			days = parsed
		}
	}

	startTime := time.Now().AddDate(0, 0, -days).Truncate(24 * time.Hour)

	query := `
		SELECT DATE(agent_timestamp) as day, COUNT(*) as count
		FROM agent_status
		WHERE is_drifted = true AND agent_timestamp >= $1
		GROUP BY DATE(agent_timestamp)
		ORDER BY day ASC
	`

	rows, err := api.Repo.DB.QueryContext(r.Context(), query, startTime)
	if err != nil {
		api.log().Error("failed to get drift metrics", "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to get drift metrics")
		return
	}
	defer rows.Close()

	timeSeries := make([]TimeSeriesPoint, 0, days)
	totalEvents := 0

	dayMap := make(map[string]int)
	for rows.Next() {
		var day time.Time
		var count int
		if err := rows.Scan(&day, &count); err != nil {
			api.log().Error("failed to scan drift metric row", "error", err)
			continue
		}
		dayMap[day.Format("2006-01-02")] = count
		totalEvents += count
	}

	for i := 0; i < days; i++ {
		day := startTime.AddDate(0, 0, i)
		dayStr := day.Format("2006-01-02")
		count := dayMap[dayStr]
		timeSeries = append(timeSeries, TimeSeriesPoint{
			Timestamp: day,
			Value:     count,
		})
	}

	response := DriftMetricsResponse{
		Period:      strconv.Itoa(days) + " days",
		TotalEvents: totalEvents,
		TimeSeries:  timeSeries,
	}

	writeJSON(w, http.StatusOK, response)
}

// GetReconciliationMetricsHandler godoc
//
//	@Summary		Get reconciliation metrics
//	@Description	Get reconciliation success/failure metrics over time.
//	@Tags			Metrics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			days	query		int	false	"Number of days"	default(7)
//	@Success		200		{object}	ReconciliationMetricsResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Router			/metrics/reconciliation [get]
func (api *API) GetReconciliationMetricsHandler(w http.ResponseWriter, r *http.Request) {
	_, authorized := requireRole(r, models.RoleAdmin, models.RoleUser, models.RoleViewer)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied")
		return
	}

	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 90 {
			days = parsed
		}
	}

	startTime := time.Now().AddDate(0, 0, -days).Truncate(24 * time.Hour)

	query := `
		SELECT 
			DATE(agent_timestamp) as day,
			SUM(CASE WHEN status = 'Applied' THEN 1 ELSE 0 END) as applied,
			SUM(CASE WHEN status = 'Failed' THEN 1 ELSE 0 END) as failed
		FROM agent_status
		WHERE agent_timestamp >= $1
		GROUP BY DATE(agent_timestamp)
		ORDER BY day ASC
	`

	rows, err := api.Repo.DB.QueryContext(r.Context(), query, startTime)
	if err != nil {
		api.log().Error("failed to get reconciliation metrics", "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to get reconciliation metrics")
		return
	}
	defer rows.Close()

	dayMap := make(map[string]struct{ applied, failed int })
	totalApplied := 0
	totalFailed := 0

	for rows.Next() {
		var day time.Time
		var applied, failed int
		if err := rows.Scan(&day, &applied, &failed); err != nil {
			api.log().Error("failed to scan reconciliation metric row", "error", err)
			continue
		}
		dayStr := day.Format("2006-01-02")
		dayMap[dayStr] = struct{ applied, failed int }{applied, failed}
		totalApplied += applied
		totalFailed += failed
	}

	timeSeries := make([]TimeSeriesPoint, 0, days)
	for i := 0; i < days; i++ {
		day := startTime.AddDate(0, 0, i)
		dayStr := day.Format("2006-01-02")
		data := dayMap[dayStr]
		timeSeries = append(timeSeries, TimeSeriesPoint{
			Timestamp: day,
			Value:     data.applied,
		})
	}

	successRate := 0.0
	total := totalApplied + totalFailed
	if total > 0 {
		successRate = float64(totalApplied) / float64(total) * 100
	}

	response := ReconciliationMetricsResponse{
		Period:       strconv.Itoa(days) + " days",
		TotalApplied: totalApplied,
		TotalFailed:  totalFailed,
		SuccessRate:  successRate,
		TimeSeries:   timeSeries,
	}

	writeJSON(w, http.StatusOK, response)
}

// GetAgentConnectivityHandler godoc
//
//	@Summary		Get agent connectivity
//	@Description	Get agent online/offline status for all servers.
//	@Tags			Metrics
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	AgentConnectivityResponse
//	@Failure		401	{object}	ErrorResponse
//	@Failure		403	{object}	ErrorResponse
//	@Router			/metrics/connectivity [get]
func (api *API) GetAgentConnectivityHandler(w http.ResponseWriter, r *http.Request) {
	_, authorized := requireRole(r, models.RoleAdmin, models.RoleUser, models.RoleViewer)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied")
		return
	}

	staleThreshold := 5 * time.Minute

	servers, err := api.Repo.ListServersWithLatestStatus(r.Context(), 1000, 0, "name")
	if err != nil {
		api.log().Error("failed to get servers for connectivity", "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to get agent connectivity")
		return
	}

	statuses := make([]AgentConnectivityEntry, 0, len(servers))
	onlineCount := 0
	offlineCount := 0

	now := time.Now()
	for _, server := range servers {
		entry := AgentConnectivityEntry{
			ServerID:   server.ID,
			ServerName: server.Name,
			LastSeen:   server.LastTimestamp,
		}

		if server.LastStatus != nil {
			entry.Status = *server.LastStatus
		}

		if server.LastTimestamp != nil && now.Sub(*server.LastTimestamp) <= staleThreshold {
			entry.IsOnline = true
			onlineCount++
		} else {
			entry.IsOnline = false
			offlineCount++
		}

		statuses = append(statuses, entry)
	}

	response := AgentConnectivityResponse{
		TotalAgents:    len(servers),
		OnlineAgents:   onlineCount,
		OfflineAgents:  offlineCount,
		StaleThreshold: staleThreshold.String(),
		AgentStatuses:  statuses,
	}

	writeJSON(w, http.StatusOK, response)
}
