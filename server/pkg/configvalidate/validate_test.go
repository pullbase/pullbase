package configvalidate

import (
	"testing"
)

func TestValidate(t *testing.T) {
	t.Run("valid config returns valid result", func(t *testing.T) {
		config := `
serverMetadata:
  name: test-server
  environment: testing
packages:
  - name: nginx
    state: present
services:
  - name: nginx
    state: running
files:
  - path: /etc/nginx/nginx.conf
    content: test
    mode: "0644"
system:
  serviceManager: systemd
`
		result := Validate([]byte(config))
		if !result.Valid {
			t.Errorf("expected valid config, got errors: %v", result.Errors)
		}
	})

	t.Run("invalid YAML syntax", func(t *testing.T) {
		config := `packages: [`
		result := Validate([]byte(config))
		if result.Valid {
			t.Error("expected invalid result for malformed YAML")
		}
	})

	t.Run("invalid package state", func(t *testing.T) {
		config := `
packages:
  - name: nginx
    state: invalid
`
		result := Validate([]byte(config))
		if result.Valid {
			t.Error("expected invalid result")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "packages[0].state" {
				found = true
			}
		}
		if !found {
			t.Error("expected error on packages[0].state")
		}
	})

	t.Run("invalid service state", func(t *testing.T) {
		config := `
services:
  - name: nginx
    state: active
`
		result := Validate([]byte(config))
		if result.Valid {
			t.Error("expected invalid result")
		}
	})

	t.Run("invalid file mode", func(t *testing.T) {
		config := `
files:
  - path: /etc/test.conf
    content: test
    mode: "999"
`
		result := Validate([]byte(config))
		if result.Valid {
			t.Error("expected invalid result for invalid file mode")
		}
	})

	t.Run("invalid service manager", func(t *testing.T) {
		config := `
system:
  serviceManager: invalid
`
		result := Validate([]byte(config))
		if result.Valid {
			t.Error("expected invalid result for invalid service manager")
		}
	})

	t.Run("empty config is valid", func(t *testing.T) {
		result := Validate([]byte(""))
		if !result.Valid {
			t.Errorf("expected empty config to be valid, got errors: %v", result.Errors)
		}
	})

	t.Run("valid package states", func(t *testing.T) {
		states := []string{"present", "latest", "absent"}
		for _, state := range states {
			config := `
packages:
  - name: nginx
    state: ` + state
			result := Validate([]byte(config))
			if !result.Valid {
				t.Errorf("expected %s to be valid package state, got errors: %v", state, result.Errors)
			}
		}
	})

	t.Run("valid service states", func(t *testing.T) {
		states := []string{"running", "stopped"}
		for _, state := range states {
			config := `
services:
  - name: nginx
    state: ` + state
			result := Validate([]byte(config))
			if !result.Valid {
				t.Errorf("expected %s to be valid service state, got errors: %v", state, result.Errors)
			}
		}
	})

	t.Run("valid service managers", func(t *testing.T) {
		managers := []string{"systemd", "supervisor", "supervisord", "docker-supervisor", "openrc"}
		for _, mgr := range managers {
			config := `
system:
  serviceManager: ` + mgr
			result := Validate([]byte(config))
			if !result.Valid {
				t.Errorf("expected %s to be valid service manager, got errors: %v", mgr, result.Errors)
			}
		}
	})

	t.Run("valid file modes", func(t *testing.T) {
		modes := []string{"0644", "0755", "0600", "0777", "644", "755"}
		for _, mode := range modes {
			config := `
files:
  - path: /etc/test.conf
    content: test
    mode: "` + mode + `"`
			result := Validate([]byte(config))
			if !result.Valid {
				t.Errorf("expected %s to be valid file mode, got errors: %v", mode, result.Errors)
			}
		}
	})

	t.Run("multiple errors reported", func(t *testing.T) {
		config := `
packages:
  - name: ""
    state: invalid
services:
  - name: ""
    state: invalid
`
		result := Validate([]byte(config))
		if len(result.Errors) < 4 {
			t.Errorf("expected at least 4 errors, got %d", len(result.Errors))
		}
	})

	t.Run("line numbers reported", func(t *testing.T) {
		config := `packages:
  - name: nginx
    state: invalid`
		result := Validate([]byte(config))
		if result.Valid {
			t.Fatal("expected invalid result")
		}
		for _, e := range result.Errors {
			if e.Field == "packages[0].state" && e.Line == 0 {
				t.Error("expected line number to be reported")
			}
		}
	})
}

func TestValidateBytes(t *testing.T) {
	t.Run("returns empty slice for valid config", func(t *testing.T) {
		config := `packages:
  - name: nginx
    state: present`
		errors := ValidateBytes([]byte(config))
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %d", len(errors))
		}
	})

	t.Run("returns errors for invalid config", func(t *testing.T) {
		config := `packages:
  - name: nginx
    state: invalid`
		errors := ValidateBytes([]byte(config))
		if len(errors) == 0 {
			t.Error("expected errors")
		}
	})
}
