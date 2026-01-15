// Package configvalidate provides YAML configuration validation for Pullbase.
// It validates server configuration files against the expected schema and semantic rules.
package configvalidate

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationError represents a single validation error with location info.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}
type Result struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// ServerConfig mirrors the agent's ServerConfig structure for validation.
type ServerConfig struct {
	ServerMetadata struct {
		Name        string `yaml:"name"`
		Environment string `yaml:"environment"`
	} `yaml:"serverMetadata"`

	Packages []struct {
		Name  string `yaml:"name"`
		State string `yaml:"state"`
	} `yaml:"packages"`

	Services []struct {
		Name    string `yaml:"name"`
		Enabled bool   `yaml:"enabled"`
		State   string `yaml:"state"`
		Managed bool   `yaml:"managed"`
	} `yaml:"services"`

	Files []struct {
		Path          string `yaml:"path"`
		Content       string `yaml:"content"`
		Mode          string `yaml:"mode,omitempty"`
		ReloadService string `yaml:"reloadService"`
		ReloadCommand string `yaml:"reloadCommand"`
	} `yaml:"files"`

	System struct {
		ServiceManager string `yaml:"serviceManager"`
		Containerized  bool   `yaml:"containerized"`
	} `yaml:"system"`
}

// Validate parses and validates YAML configuration content.
// Returns a Result containing validation status and any errors found.
func Validate(yamlContent []byte) Result {
	errors := ValidateBytes(yamlContent)
	return Result{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

// ValidateBytes parses and validates YAML configuration content.
// Returns a slice of validation errors (empty if valid).
func ValidateBytes(yamlContent []byte) []ValidationError {
	var errors []ValidationError

	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlContent, &rootNode); err != nil {
		if yamlErr, ok := err.(*yaml.TypeError); ok {
			for _, e := range yamlErr.Errors {
				errors = append(errors, ValidationError{
					Field:   "",
					Message: e,
				})
			}
		} else {
			errors = append(errors, ValidationError{
				Field:   "",
				Message: fmt.Sprintf("YAML parsing error: %v", err),
			})
		}
		return errors
	}

	var config ServerConfig
	if len(rootNode.Content) > 0 {
		if err := rootNode.Content[0].Decode(&config); err != nil {
		if yamlErr, ok := err.(*yaml.TypeError); ok {
			for _, e := range yamlErr.Errors {
				errors = append(errors, ValidationError{
					Field:   "",
					Message: e,
				})
			}
		} else {
			errors = append(errors, ValidationError{
				Field:   "",
				Message: fmt.Sprintf("YAML parsing error: %v", err),
			})
		}
		return errors
		}
	}

	errors = append(errors, validateConfigContent(&config, &rootNode)...)

	return errors
}

// validateConfigContent performs semantic validation on the parsed config.
func validateConfigContent(config *ServerConfig, rootNode *yaml.Node) []ValidationError {
	var errors []ValidationError

	fieldPositions := extractFieldPositions(rootNode)

	validPackageStates := map[string]bool{"present": true, "latest": true, "absent": true}
	for i, pkg := range config.Packages {
		fieldPath := fmt.Sprintf("packages[%d]", i)

		if strings.TrimSpace(pkg.Name) == "" {
			pos := getFieldPosition(fieldPositions, fmt.Sprintf("packages.%d.name", i))
			errors = append(errors, ValidationError{
				Field:   fieldPath + ".name",
				Message: "Package name is required",
				Line:    pos.Line,
				Column:  pos.Column,
			})
		}

		if strings.TrimSpace(pkg.State) == "" {
			pos := getFieldPosition(fieldPositions, fmt.Sprintf("packages.%d.state", i))
			errors = append(errors, ValidationError{
				Field:   fieldPath + ".state",
				Message: "Package state is required",
				Line:    pos.Line,
				Column:  pos.Column,
			})
		} else if !validPackageStates[pkg.State] {
			pos := getFieldPosition(fieldPositions, fmt.Sprintf("packages.%d.state", i))
			errors = append(errors, ValidationError{
				Field:   fieldPath + ".state",
				Message: fmt.Sprintf("Invalid package state '%s'. Must be one of: present, latest, absent", pkg.State),
				Line:    pos.Line,
				Column:  pos.Column,
			})
		}
	}

	validServiceStates := map[string]bool{"running": true, "stopped": true}
	for i, svc := range config.Services {
		fieldPath := fmt.Sprintf("services[%d]", i)

		if strings.TrimSpace(svc.Name) == "" {
			pos := getFieldPosition(fieldPositions, fmt.Sprintf("services.%d.name", i))
			errors = append(errors, ValidationError{
				Field:   fieldPath + ".name",
				Message: "Service name is required",
				Line:    pos.Line,
				Column:  pos.Column,
			})
		}

		if strings.TrimSpace(svc.State) == "" {
			pos := getFieldPosition(fieldPositions, fmt.Sprintf("services.%d.state", i))
			errors = append(errors, ValidationError{
				Field:   fieldPath + ".state",
				Message: "Service state is required",
				Line:    pos.Line,
				Column:  pos.Column,
			})
		} else if !validServiceStates[svc.State] {
			pos := getFieldPosition(fieldPositions, fmt.Sprintf("services.%d.state", i))
			errors = append(errors, ValidationError{
				Field:   fieldPath + ".state",
				Message: fmt.Sprintf("Invalid service state '%s'. Must be one of: running, stopped", svc.State),
				Line:    pos.Line,
				Column:  pos.Column,
			})
		}
	}

	for i, file := range config.Files {
		fieldPath := fmt.Sprintf("files[%d]", i)

		if strings.TrimSpace(file.Path) == "" {
			pos := getFieldPosition(fieldPositions, fmt.Sprintf("files.%d.path", i))
			errors = append(errors, ValidationError{
				Field:   fieldPath + ".path",
				Message: "File path is required",
				Line:    pos.Line,
				Column:  pos.Column,
			})
		}

		if file.Mode != "" {
			if err := validateFileMode(file.Mode); err != nil {
				pos := getFieldPosition(fieldPositions, fmt.Sprintf("files.%d.mode", i))
				errors = append(errors, ValidationError{
					Field:   fieldPath + ".mode",
					Message: err.Error(),
					Line:    pos.Line,
					Column:  pos.Column,
				})
			}
		}
	}

	validServiceManagers := map[string]bool{
		"systemd":           true,
		"supervisor":        true,
		"supervisord":       true,
		"docker-supervisor": true,
		"openrc":            true,
		"":                  true, // empty is allowed (auto-detect)
	}
	if config.System.ServiceManager != "" && !validServiceManagers[strings.ToLower(config.System.ServiceManager)] {
		pos := getFieldPosition(fieldPositions, "system.serviceManager")
		errors = append(errors, ValidationError{
			Field:   "system.serviceManager",
			Message: fmt.Sprintf("Invalid service manager '%s'. Must be one of: systemd, supervisor, supervisord, docker-supervisor, openrc", config.System.ServiceManager),
			Line:    pos.Line,
			Column:  pos.Column,
		})
	}

	return errors
}

// validateFileMode validates that a file mode string is a valid octal.
func validateFileMode(modeStr string) error {
	if modeStr == "" {
		return nil
	}

	mode, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return fmt.Errorf("invalid file mode '%s': must be a valid octal number (e.g., '0644', '0755')", modeStr)
	}
	if mode > 0777 {
		return fmt.Errorf("invalid file mode '%s': must not exceed 0777", modeStr)
	}
	return nil
}

type fieldPosition struct {
	Line   int
	Column int
}

// extractFieldPositions walks the YAML node tree and extracts positions for fields.
func extractFieldPositions(node *yaml.Node) map[string]fieldPosition {
	positions := make(map[string]fieldPosition)
	if node == nil {
		return positions
	}

	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		extractFieldPositionsRecursive(node.Content[0], "", positions)
	} else {
		extractFieldPositionsRecursive(node, "", positions)
	}

	return positions
}

// extractFieldPositionsRecursive recursively walks the node tree.
func extractFieldPositionsRecursive(node *yaml.Node, prefix string, positions map[string]fieldPosition) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			key := keyNode.Value
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}

			positions[path] = fieldPosition{Line: valueNode.Line, Column: valueNode.Column}
			extractFieldPositionsRecursive(valueNode, path, positions)
		}
	case yaml.SequenceNode:
		for i, item := range node.Content {
			path := fmt.Sprintf("%s.%d", prefix, i)
			positions[path] = fieldPosition{Line: item.Line, Column: item.Column}
			extractFieldPositionsRecursive(item, path, positions)
		}
	}
}

// getFieldPosition returns the position for a field path, or zeros if not found.
func getFieldPosition(positions map[string]fieldPosition, path string) fieldPosition {
	if pos, ok := positions[path]; ok {
		return pos
	}
	return fieldPosition{}
}
