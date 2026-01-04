package server

import (
	"testing"

	"github.com/pullbase/pullbase/server/pkg/configvalidate"
)

func TestValidateConfig(t *testing.T) {
	t.Run("valid config returns no errors", func(t *testing.T) {
		config := `
serverMetadata:
  name: test-server
  environment: testing

packages:
  - name: nginx
    state: present
  - name: curl
    state: latest
  - name: vim
    state: absent

services:
  - name: nginx
    enabled: true
    state: running
    managed: true
  - name: redis
    enabled: false
    state: stopped
    managed: false

files:
  - path: /etc/nginx/nginx.conf
    content: |
      server { listen 80; }
    mode: "0644"
    reloadService: nginx

system:
  serviceManager: systemd
  containerized: false
`
		result := configvalidate.Validate([]byte(config))
		if !result.Valid {
			t.Errorf("expected valid config, got %d errors: %v", len(result.Errors), result.Errors)
		}
	})

	t.Run("invalid YAML syntax", func(t *testing.T) {
		config := `
packages:
  - name: nginx
    state: present
  - invalid yaml here: [
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for invalid YAML syntax")
		}
		if len(result.Errors) == 0 || result.Errors[0].Message == "" {
			t.Error("expected error message for YAML parse error")
		}
	})

	t.Run("invalid package state", func(t *testing.T) {
		config := `
packages:
  - name: nginx
    state: installed
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for invalid package state")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "packages[0].state" {
				found = true
				if e.Message == "" {
					t.Error("expected error message for invalid state")
				}
				break
			}
		}
		if !found {
			t.Error("expected error on packages[0].state field")
		}
	})

	t.Run("missing package name", func(t *testing.T) {
		config := `
packages:
  - name: ""
    state: present
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for missing package name")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "packages[0].name" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected error on packages[0].name field")
		}
	})

	t.Run("missing package state", func(t *testing.T) {
		config := `
packages:
  - name: nginx
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for missing package state")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "packages[0].state" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected error on packages[0].state field")
		}
	})

	t.Run("invalid service state", func(t *testing.T) {
		config := `
services:
  - name: nginx
    state: active
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for invalid service state")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "services[0].state" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected error on services[0].state field")
		}
	})

	t.Run("missing service name", func(t *testing.T) {
		config := `
services:
  - name: ""
    state: running
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for missing service name")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "services[0].name" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected error on services[0].name field")
		}
	})

	t.Run("missing service state", func(t *testing.T) {
		config := `
services:
  - name: nginx
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for missing service state")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "services[0].state" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected error on services[0].state field")
		}
	})

	t.Run("invalid file mode - not octal", func(t *testing.T) {
		config := `
files:
  - path: /etc/test.conf
    content: test
    mode: "999"
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for invalid file mode")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "files[0].mode" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected error on files[0].mode field")
		}
	})

	t.Run("invalid file mode - letters", func(t *testing.T) {
		config := `
files:
  - path: /etc/test.conf
    content: test
    mode: "rwxr-xr-x"
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for non-octal file mode")
		}
	})

	t.Run("missing file path", func(t *testing.T) {
		config := `
files:
  - path: ""
    content: test
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for missing file path")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "files[0].path" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected error on files[0].path field")
		}
	})

	t.Run("invalid service manager", func(t *testing.T) {
		config := `
system:
  serviceManager: invalid-manager
`
		result := configvalidate.Validate([]byte(config))
		if result.Valid {
			t.Error("expected error for invalid service manager")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "system.serviceManager" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected error on system.serviceManager field")
		}
	})

	t.Run("valid service managers", func(t *testing.T) {
		managers := []string{"systemd", "supervisor", "supervisord", "docker-supervisor", "openrc"}
		for _, mgr := range managers {
			config := `
system:
  serviceManager: ` + mgr + `
`
			result := configvalidate.Validate([]byte(config))
			for _, e := range result.Errors {
				if e.Field == "system.serviceManager" {
					t.Errorf("expected %s to be valid service manager, got error: %s", mgr, e.Message)
				}
			}
		}
	})

	t.Run("empty config", func(t *testing.T) {
		result := configvalidate.Validate([]byte(""))
		if !result.Valid {
			t.Errorf("expected empty config to be valid (no resources defined), got %d errors", len(result.Errors))
		}
	})

	t.Run("multiple errors reported", func(t *testing.T) {
		config := `
packages:
  - name: ""
    state: invalid
  - name: curl
    state: ""

services:
  - name: ""
    state: invalid
`
		result := configvalidate.Validate([]byte(config))
		if len(result.Errors) < 4 {
			t.Errorf("expected at least 4 errors for multiple invalid fields, got %d", len(result.Errors))
		}
	})

	t.Run("valid package states", func(t *testing.T) {
		states := []string{"present", "latest", "absent"}
		for _, state := range states {
			config := `
packages:
  - name: nginx
    state: ` + state + `
`
			result := configvalidate.Validate([]byte(config))
			for _, e := range result.Errors {
				if e.Field == "packages[0].state" {
					t.Errorf("expected %s to be valid package state, got error: %s", state, e.Message)
				}
			}
		}
	})

	t.Run("valid service states", func(t *testing.T) {
		states := []string{"running", "stopped"}
		for _, state := range states {
			config := `
services:
  - name: nginx
    state: ` + state + `
`
			result := configvalidate.Validate([]byte(config))
			for _, e := range result.Errors {
				if e.Field == "services[0].state" {
					t.Errorf("expected %s to be valid service state, got error: %s", state, e.Message)
				}
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
    mode: "` + mode + `"
`
			result := configvalidate.Validate([]byte(config))
			for _, e := range result.Errors {
				if e.Field == "files[0].mode" {
					t.Errorf("expected %s to be valid file mode, got error: %s", mode, e.Message)
				}
			}
		}
	})
}
