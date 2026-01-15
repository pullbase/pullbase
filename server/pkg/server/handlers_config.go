package server

import (
	"io"
	"net/http"

	"github.com/pullbase/pullbase/server/pkg/configvalidate"
)

type ValidateConfigResponse struct {
	Valid  bool                             `json:"valid"`
	Errors []configvalidate.ValidationError `json:"errors,omitempty"`
}

// ValidateConfigHandler validates a server configuration YAML file.
//
//	@Summary		Validate config
//	@Description	Validates a config.yaml file for syntax errors and invalid values
//	@Tags			Config
//	@Accept			text/yaml
//	@Produce		json
//	@Security		BearerAuth
//	@Param			config	body		string	true	"YAML configuration file content"
//	@Success		200		{object}	ValidateConfigResponse
//	@Failure		400		{object}	ValidateConfigResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		405		{object}	ErrorResponse
//	@Router			/validate-config [post]
func (a *API) ValidateConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ValidateConfigResponse{
			Valid: false,
			Errors: []configvalidate.ValidationError{
				{Field: "", Message: "Failed to read request body"},
			},
		})
		return
	}

	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, ValidateConfigResponse{
			Valid: false,
			Errors: []configvalidate.ValidationError{
				{Field: "", Message: "Request body is empty"},
			},
		})
		return
	}

	result := configvalidate.Validate(body)

	writeJSON(w, http.StatusOK, ValidateConfigResponse{
		Valid:  result.Valid,
		Errors: result.Errors,
	})
}
