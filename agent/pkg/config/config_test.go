package config_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pullbase/pullbase/agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type mockCall struct {
	Method string
	Name   string
}

type mockServiceManager struct {
	calls           []mockCall
	mu              sync.Mutex
	activeStates    map[string]bool
	enabledStates   map[string]bool
	errors          map[string]error
	isActiveResult  map[string]bool
	isEnabledResult map[string]bool
	startErr        error
	stopErr         error
	enableErr       error
	disableErr      error
	reloadErr       error
}

func newMockServiceManager() *mockServiceManager {
	return &mockServiceManager{
		activeStates:    make(map[string]bool),
		enabledStates:   make(map[string]bool),
		errors:          make(map[string]error),
		isActiveResult:  make(map[string]bool),
		isEnabledResult: make(map[string]bool),
	}
}

func (m *mockServiceManager) recordCall(method, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, mockCall{Method: method, Name: name})
}

func (m *mockServiceManager) Start(name string) error {
	m.recordCall("Start", name)
	return m.startErr
}

func (m *mockServiceManager) Stop(name string) error {
	m.recordCall("Stop", name)
	return m.stopErr
}

func (m *mockServiceManager) Enable(name string) error {
	m.recordCall("Enable", name)
	return m.enableErr
}

func (m *mockServiceManager) Disable(name string) error {
	m.recordCall("Disable", name)
	return m.disableErr
}

func (m *mockServiceManager) ReloadOrRestart(name string) error {
	m.recordCall("ReloadOrRestart", name)
	return m.reloadErr
}

func (m *mockServiceManager) IsActive(name string) (bool, error) {
	m.recordCall("IsActive", name)
	if m.isActiveResult != nil {
		if active, ok := m.isActiveResult[name]; ok {
			return active, nil
		}
	}
	return false, nil // Default
}

func (m *mockServiceManager) IsEnabled(name string) (bool, error) {
	m.recordCall("IsEnabled", name)
	if m.isEnabledResult != nil {
		if enabled, ok := m.isEnabledResult[name]; ok {
			return enabled, nil
		}
	}
	return false, nil
}

// Test LoadConfig with valid YAML
func TestLoadConfig_ValidYAML(t *testing.T) {
	t.Parallel()
	validYAML := `
serverMetadata:
  name: test-server
  environment: development
packages:
  - name: bash
    state: present
  - name: curl
    state: latest
services:
  - name: nginx
    enabled: true
    state: running
    managed: true
  - name: apache
    enabled: false
    state: stopped
    managed: false
system:
  serviceManager: supervisor
  containerized: true
files:
  - path: /etc/nginx/nginx.conf
    content: "worker_processes 1;"
    reloadService: nginx
  - path: /etc/hosts
    content: "127.0.0.1 localhost"
    reloadCommand: "custom reload command"
`
	tempFile, err := os.CreateTemp("", "valid-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	_, err = tempFile.WriteString(validYAML)
	require.NoError(t, err)
	tempFile.Close()

	loadedConfig, err := config.LoadConfig(tempFile.Name())

	require.NoError(t, err)
	require.NotNil(t, loadedConfig)

	assert.Equal(t, "test-server", loadedConfig.ServerMetadata.Name)
	assert.Equal(t, "development", loadedConfig.ServerMetadata.Environment)
	require.Len(t, loadedConfig.Packages, 2)
	assert.Equal(t, "bash", loadedConfig.Packages[0].Name)
	assert.Equal(t, "present", loadedConfig.Packages[0].State)
	assert.Equal(t, "curl", loadedConfig.Packages[1].Name)
	assert.Equal(t, "latest", loadedConfig.Packages[1].State)

	require.Len(t, loadedConfig.Services, 2)
	assert.Equal(t, "nginx", loadedConfig.Services[0].Name)
	assert.True(t, loadedConfig.Services[0].Enabled)
	assert.Equal(t, "running", loadedConfig.Services[0].State)
	assert.True(t, loadedConfig.Services[0].Managed)
	assert.Equal(t, "apache", loadedConfig.Services[1].Name)
	assert.False(t, loadedConfig.Services[1].Enabled)
	assert.Equal(t, "stopped", loadedConfig.Services[1].State)
	assert.False(t, loadedConfig.Services[1].Managed)

	assert.Equal(t, "supervisor", loadedConfig.System.ServiceManager)
	assert.True(t, loadedConfig.System.Containerized)

	require.Len(t, loadedConfig.Files, 2)
	assert.Equal(t, "/etc/nginx/nginx.conf", loadedConfig.Files[0].Path)
	assert.Equal(t, "worker_processes 1;", loadedConfig.Files[0].Content)
	assert.Equal(t, "nginx", loadedConfig.Files[0].ReloadService)
	assert.Equal(t, "", loadedConfig.Files[0].ReloadCommand)
	assert.Equal(t, "/etc/hosts", loadedConfig.Files[1].Path)
	assert.Equal(t, "127.0.0.1 localhost", loadedConfig.Files[1].Content)
	assert.Equal(t, "", loadedConfig.Files[1].ReloadService)
	assert.Equal(t, "custom reload command", loadedConfig.Files[1].ReloadCommand)
}

// Test LoadConfig with invalid YAML
func TestLoadConfig_InvalidYAML(t *testing.T) {
	t.Parallel()
	invalidYAML := `
serverMetadata:
  name: test
packages: - name: broken
`
	tempFile, err := os.CreateTemp("", "invalid-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	_, err = tempFile.WriteString(invalidYAML)
	require.NoError(t, err)
	tempFile.Close()

	loadedConfig, err := config.LoadConfig(tempFile.Name())

	require.Error(t, err)
	assert.Nil(t, loadedConfig)
	// Check if the error is a YAML parsing error
	_, ok := err.(*yaml.TypeError)
	if !ok {
		assert.ErrorContains(t, err, "yaml:")
	}
}

// Test LoadConfig with non-existent file
func TestLoadConfig_FileNotFound(t *testing.T) {
	t.Parallel()

	loadedConfig, err := config.LoadConfig("/non/existent/path/config.yaml")

	require.Error(t, err)
	assert.Nil(t, loadedConfig)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// Mock function for runCommand
type commandCall struct {
	name string
	args []string
}

// mockPackageManager is a simple mock for testing Apply/CheckDrift
type mockPackageManager struct {
	InstalledMap      map[string]bool
	ErrorMap          map[string]error
	InstallCalledWith []string
	RemoveCalledWith  []string
	IsInstalledCalled []string
}

func (m *mockPackageManager) Install(name string, version string) error {
	m.InstallCalledWith = append(m.InstallCalledWith, name+"="+version)
	if m.ErrorMap != nil {
		if err, exists := m.ErrorMap[name+"_install"]; exists {
			return err
		}
	}
	return nil // Default success
}
func (m *mockPackageManager) Remove(name string) error {
	m.RemoveCalledWith = append(m.RemoveCalledWith, name)
	if m.ErrorMap != nil {
		if err, exists := m.ErrorMap[name+"_remove"]; exists {
			return err
		}
	}
	return nil // Default success
}
func (m *mockPackageManager) IsInstalled(name string) (bool, error) {
	m.IsInstalledCalled = append(m.IsInstalledCalled, name)
	if m.ErrorMap != nil {
		if err, exists := m.ErrorMap[name+"_isinstalled"]; exists {
			return false, err
		}
	}
	if m.InstalledMap != nil {
		if installed, exists := m.InstalledMap[name]; exists {
			return installed, nil
		}
	}
	return false, nil
}

// TestApply tests the Apply method, mocking command execution and ServiceManager
func TestApply(t *testing.T) {
	// Sample config to apply
	sampleConfig := &config.ServerConfig{
		Packages: []struct {
			Name  string `json:"name" yaml:"name"`
			State string `json:"state" yaml:"state"`
		}{
			{Name: "nginx", State: "present"},
			{Name: "oldpkg", State: "absent"},
		},
		Files: []struct {
			Path          string `json:"path" yaml:"path"`
			Content       string `json:"content" yaml:"content"`
			Mode          string `json:"mode,omitempty" yaml:"mode,omitempty"`
			ReloadService string `json:"reloadService" yaml:"reloadService"`
			ReloadCommand string `json:"reloadCommand" yaml:"reloadCommand"`
		}{
			{Path: "/tmp/testfile_apply", Content: "hello world apply", Mode: "0600", ReloadService: "testsvc", ReloadCommand: ""},
		},
		Services: []struct {
			Name    string `json:"name" yaml:"name"`
			Enabled bool   `json:"enabled" yaml:"enabled"`
			State   string `json:"state" yaml:"state"`
			Managed bool   `json:"managed" yaml:"managed"`
		}{
			{Name: "testsvc", Enabled: true, State: "running", Managed: true},
			{Name: "othersvc", Enabled: false, State: "stopped", Managed: true},
		},
		System: struct {
			ServiceManager string `json:"serviceManager" yaml:"serviceManager"`
			Containerized  bool   `json:"containerized" yaml:"containerized"`
		}{
			ServiceManager: "systemd",
			Containerized:  false,
		},
	}

	// Mock ServiceManager
	mockSvc := newMockServiceManager()
	mockSvc.activeStates["testsvc"] = false
	mockSvc.activeStates["othersvc"] = true

	// Mock runCommand (only for packages now)
	var executedCmds []commandCall
	mockRunCommand := func(name string, args ...string) (string, error) {
		if name == "apk" {
			call := commandCall{name: name, args: args}
			executedCmds = append(executedCmds, call)
			return "", nil
		}
		return "", fmt.Errorf("unexpected call to runCommand in Apply test: %s %v", name, args)
	}

	originalRunCommand := config.ExportSetRunCommandFunc(mockRunCommand)
	t.Cleanup(func() {
		config.ExportSetRunCommandFunc(originalRunCommand)
	})

	mockPkg := &mockPackageManager{
		InstalledMap: map[string]bool{
			"oldpkg": true,
			"nginx":  false,
		},
	}

	// Apply the configuration
	err := sampleConfig.Apply(mockSvc, mockPkg)
	require.NoError(t, err)

	// Assert service manager calls
	expectedSvcCalls := []mockCall{
		{Method: "Enable", Name: "testsvc"},
		{Method: "IsActive", Name: "testsvc"},
		{Method: "Start", Name: "testsvc"},
		{Method: "Disable", Name: "othersvc"},
		{Method: "IsActive", Name: "othersvc"},
		{Method: "ReloadOrRestart", Name: "testsvc"},
	}
	assert.Equal(t, expectedSvcCalls, mockSvc.calls, "ServiceManager calls do not match expected sequence")

	// Assert package manager calls
	assert.Contains(t, mockPkg.InstallCalledWith, "nginx=present", "Expected Install('nginx', 'present') call")
	assert.Contains(t, mockPkg.RemoveCalledWith, "oldpkg", "Expected Remove('oldpkg') call")

	// Check file creation and mode
	info, err := os.Stat("/tmp/testfile_apply")
	assert.NoError(t, err, "Expected test file to be created by Apply")
	if err == nil {
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "Expected file mode 0600 for applied file")
	}
	os.Remove("/tmp/testfile_apply") //nolint:errcheck // cleanup best effort
}

// TestCheckDrift tests the CheckDrift method
func TestCheckDrift(t *testing.T) {
	// Setup: Create temp file
	tempDir := t.TempDir()
	correctFilePath := filepath.Join(tempDir, "correct_file")
	driftedFilePath := filepath.Join(tempDir, "drifted_file")
	missingFilePath := filepath.Join(tempDir, "missing_file")

	// Content for the correct file
	correctContent := "line1\nline2"

	// Content for the drifted file (actual content on disk)
	driftedFileActualContent := "different content"
	driftedFileActualHash := sha256.Sum256([]byte(driftedFileActualContent))
	driftedFileActualHashStr := hex.EncodeToString(driftedFileActualHash[:])

	// Content for the missing file (desired content)
	missingFileDesiredContent := "some content"

	// Create the files
	require.NoError(t, os.WriteFile(correctFilePath, []byte(correctContent), 0644))
	require.NoError(t, os.WriteFile(driftedFilePath, []byte(driftedFileActualContent), 0644))

	// Desired configuration matching the test setup
	desiredConfig := &config.ServerConfig{
		Packages: []struct {
			Name  string `json:"name" yaml:"name"`
			State string `json:"state" yaml:"state"`
		}{
			{Name: "htop", State: "present"},
			{Name: "installedPkg", State: "present"},
			{Name: "absentPkg", State: "absent"},
		},
		Services: []struct {
			Name    string `json:"name" yaml:"name"`
			Enabled bool   `json:"enabled" yaml:"enabled"`
			State   string `json:"state" yaml:"state"`
			Managed bool   `json:"managed" yaml:"managed"`
		}{
			{Name: "nginx.service", Enabled: true, State: "running", Managed: true},
			{Name: "drifted.service", Enabled: false, State: "stopped", Managed: true},
		},
		Files: []struct {
			Path          string `json:"path" yaml:"path"`
			Content       string `json:"content" yaml:"content"`
			Mode          string `json:"mode,omitempty" yaml:"mode,omitempty"`
			ReloadService string `json:"reloadService" yaml:"reloadService"`
			ReloadCommand string `json:"reloadCommand" yaml:"reloadCommand"`
		}{
			{Path: correctFilePath, Content: correctContent, Mode: "0644", ReloadService: "", ReloadCommand: ""},
			{Path: driftedFilePath, Content: "correct content", Mode: "0600", ReloadService: "", ReloadCommand: ""},
			{Path: missingFilePath, Content: missingFileDesiredContent, Mode: "", ReloadService: "", ReloadCommand: ""},
		},
		System: struct {
			ServiceManager string `json:"serviceManager" yaml:"serviceManager"`
			Containerized  bool   `json:"containerized" yaml:"containerized"`
		}{
			ServiceManager: "systemd",
			Containerized:  false,
		},
	}

	// Calculate desired hash for the drifted file path
	driftedFileDesiredHash := sha256.Sum256([]byte("correct content"))
	driftedFileDesiredHashStr := hex.EncodeToString(driftedFileDesiredHash[:])

	mockSvc := &mockServiceManager{
		isActiveResult:  map[string]bool{"nginx.service": true, "drifted.service": true},
		isEnabledResult: map[string]bool{"nginx.service": true, "drifted.service": true},
	}
	mockPkg := &mockPackageManager{

		InstalledMap: map[string]bool{
			"htop":         false,
			"installedPkg": true,
			"absentPkg":    true,
		},
	}

	// Check drift
	driftMessages, err := desiredConfig.CheckDrift(mockSvc, mockPkg)
	require.NoError(t, err)

	// Assert drift messages
	expectedDrift := []string{
		"Package htop should be installed but is absent",
		"Package absentPkg should be absent but is installed",
		fmt.Sprintf("File '%s': content drift detected (desired: %s..., actual: %s...)", driftedFilePath, driftedFileDesiredHashStr[:8], driftedFileActualHashStr[:8]),
		fmt.Sprintf("File '%s': desired mode %s, actual mode 0644", driftedFilePath, "0600"),
		fmt.Sprintf("File '%s': desired state exists, actual state is missing", missingFilePath),
		"Service 'drifted.service': desired enabled state 'false', actual state is true",
		"Service 'drifted.service': desired running state 'stopped', actual state is true",
	}
	sort.Strings(driftMessages)
	sort.Strings(expectedDrift)
	assert.Equal(t, expectedDrift, driftMessages, "Drift messages mismatch")
}

// mockExitError creates a properly formatted exec.ExitError for testing
func mockExitError(exitCode int) *exec.ExitError {
	// Create a real exec.ExitError by running a command that will fail
	cmd := exec.Command("false")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		// Create a new exec.ExitError with the desired exit code
		return &exec.ExitError{
			ProcessState: exitErr.ProcessState,
			Stderr:       []byte(fmt.Sprintf("exit status %d", exitCode)),
		}
	}
	// This should never happen in practice
	panic("failed to create mock exit error")
}

// RunCommandFunc is a function type for mocking command execution
// type RunCommandFunc func(name string, arg ...string) (string, error)

// LookPathFunc is a function type for mocking exec.LookPath
// type LookPathFunc func(file string) (string, error)

func TestAptManager_IsInstalled(t *testing.T) {
	tests := []struct {
		name           string
		runCommandFunc config.RunCommandFunc
		want           bool
		wantErr        bool
	}{
		{
			name: "package is installed",
			runCommandFunc: func(name string, args ...string) (string, error) {
				return "'install ok installed'", nil
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "package is not installed",
			runCommandFunc: func(name string, args ...string) (string, error) {
				return "", mockExitError(1)
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "unexpected error",
			runCommandFunc: func(name string, args ...string) (string, error) {
				return "", errors.New("unexpected error")
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origRunCommand := config.ExportSetRunCommandFunc(tc.runCommandFunc)
			defer func() { config.ExportSetRunCommandFunc(origRunCommand) }()

			mgr := config.AptManager{}
			installed, err := mgr.IsInstalled("test-package")

			if tc.wantErr && err == nil {
				t.Errorf("Expected an error, but got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Did not expect an error, but got: %v", err)
			}
			if installed != tc.want {
				t.Errorf("Expected installed state %t, but got %t", tc.want, installed)
			}
		})
	}
}

func TestAptManager_Install(t *testing.T) {
	tests := []struct {
		name           string
		runCommandFunc config.RunCommandFunc
		version        string
		wantErr        bool
	}{
		{
			name: "successful install",
			runCommandFunc: func(name string, args ...string) (string, error) {
				return "", nil
			},
			version: "",
			wantErr: false,
		},
		{
			name: "install with version",
			runCommandFunc: func(name string, args ...string) (string, error) {
				return "", nil
			},
			version: "1.2.3",
			wantErr: false,
		},
		{
			name: "install fails",
			runCommandFunc: func(name string, args ...string) (string, error) {
				return "", mockExitError(1)
			},
			version: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origRunCommand := config.ExportSetRunCommandFunc(tt.runCommandFunc)
			defer func() { config.ExportSetRunCommandFunc(origRunCommand) }()

			mgr := config.AptManager{}
			if err := mgr.Install("test-package", tt.version); (err != nil) != tt.wantErr {
				t.Errorf("AptManager.Install() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAptManager_Remove(t *testing.T) {
	tests := []struct {
		name           string
		runCommandFunc config.RunCommandFunc
		wantErr        bool
	}{
		{
			name: "successful remove",
			runCommandFunc: func(name string, args ...string) (string, error) {
				if name == "dpkg-query" {
					return "'install ok installed'", nil
				}
				// Assume apt-get remove succeeds
				return "", nil
			},
			wantErr: false,
		},
		{
			name: "remove fails",
			runCommandFunc: func(name string, args ...string) (string, error) {
				if name == "dpkg-query" {
					return "'install ok installed'", nil
				}

				return "", mockExitError(1)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origRunCommand := config.ExportSetRunCommandFunc(tt.runCommandFunc)
			defer func() { config.ExportSetRunCommandFunc(origRunCommand) }()

			mgr := config.AptManager{}
			if err := mgr.Remove("test-package"); (err != nil) != tt.wantErr {
				t.Errorf("AptManager.Remove() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestYumManager_IsInstalled(t *testing.T) {
	tests := []struct {
		name           string
		runCommandFn   config.RunCommandFunc
		packageName    string
		expectedError  error
		expectedResult bool
	}{
		{
			name: "package is installed",
			runCommandFn: func(name string, arg ...string) (string, error) {
				if name == "rpm" && len(arg) > 0 && arg[0] == "-q" {
					return "package-1.0.0", nil
				}
				return "", fmt.Errorf("unexpected command in mock: %s %v", name, arg)
			},
			packageName:    "test-package",
			expectedError:  nil,
			expectedResult: true,
		},
		{
			name: "package is not installed",
			runCommandFn: func(name string, arg ...string) (string, error) {
				if name == "rpm" && len(arg) > 0 && arg[0] == "-q" {
					return "", mockExitError(1)
				}
				return "", fmt.Errorf("unexpected command in mock: %s %v", name, arg)
			},
			packageName:    "test-package",
			expectedError:  nil,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origRunCommand := config.ExportSetRunCommandFunc(tt.runCommandFn)
			defer func() { config.ExportSetRunCommandFunc(origRunCommand) }()

			y := config.YumManager{}
			got, err := y.IsInstalled(tt.packageName)
			if (err != nil) != (tt.expectedError != nil) {
				t.Errorf("YumManager.IsInstalled() error = %v, wantErr %v", err, tt.expectedError != nil)
				return
			}
			if got != tt.expectedResult {
				t.Errorf("YumManager.IsInstalled() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}

func TestYumManager_Install(t *testing.T) {
	tests := []struct {
		name           string
		runCommandFunc config.RunCommandFunc
		version        string
		wantErr        bool
	}{
		{
			name: "successful install",
			runCommandFunc: func(name string, args ...string) (string, error) {
				if name == "dnf" || name == "yum" {
					return "", nil
				}
				return "", fmt.Errorf("unexpected command in mock: %s %v", name, args)
			},
			version: "",
			wantErr: false,
		},
		{
			name: "install with version",
			runCommandFunc: func(name string, args ...string) (string, error) {
				if name == "dnf" || name == "yum" {
					packageSpec := args[len(args)-1]
					if strings.Contains(packageSpec, "=") || strings.Contains(packageSpec, "-") {
						return "", nil
					}
					return "", fmt.Errorf("unexpected args for versioned install: %v", args)
				}
				return "", fmt.Errorf("unexpected command in mock: %s %v", name, args)
			},
			version: "1.2.3",
			wantErr: false,
		},
		{
			name: "install fails",
			runCommandFunc: func(name string, args ...string) (string, error) {
				if name == "dnf" || name == "yum" {
					return "", mockExitError(1)
				}
				return "", fmt.Errorf("unexpected command in mock: %s %v", name, args)
			},
			version: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock LookPathFunc to make detectCommand("dnf") return false, forcing yum path
			origLookPath := config.ExportSetExecLookPath(func(file string) (string, error) {
				if file == "dnf" {
					return "", errors.New("not found")
				}
				return exec.LookPath(file)
			})
			defer func() { config.ExportSetExecLookPath(origLookPath) }()

			// Use config.ExportSetRunCommandFunc
			origRunCommand := config.ExportSetRunCommandFunc(tt.runCommandFunc)
			defer func() { config.ExportSetRunCommandFunc(origRunCommand) }()

			y := config.YumManager{}
			if err := y.Install("test-package", tt.version); (err != nil) != tt.wantErr {
				t.Errorf("YumManager.Install() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestYumManager_Remove(t *testing.T) {
	tests := []struct {
		name           string
		runCommandFunc config.RunCommandFunc
		wantErr        bool
	}{
		{
			name: "successful remove",
			runCommandFunc: func(name string, args ...string) (string, error) {
				if name == "rpm" && len(args) > 0 && args[0] == "-q" {
					return "package-1.0.0", nil
				}
				if name == "dnf" || name == "yum" {
					return "", nil
				}
				return "", fmt.Errorf("unexpected command in mock: %s %v", name, args)
			},
			wantErr: false,
		},
		{
			name: "remove fails",
			runCommandFunc: func(name string, args ...string) (string, error) {
				if name == "rpm" && len(args) > 0 && args[0] == "-q" {
					return "package-1.0.0", nil
				}
				if name == "dnf" || name == "yum" {
					return "", mockExitError(1)
				}
				return "", fmt.Errorf("unexpected command in mock: %s %v", name, args)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			origLookPath := config.ExportSetExecLookPath(func(file string) (string, error) {
				if file == "dnf" {
					return "", errors.New("not found")
				}
				return exec.LookPath(file)
			})
			defer func() { config.ExportSetExecLookPath(origLookPath) }()

			origRunCommand := config.ExportSetRunCommandFunc(tt.runCommandFunc)
			defer func() { config.ExportSetRunCommandFunc(origRunCommand) }()

			y := config.YumManager{}
			if err := y.Remove("test-package"); (err != nil) != tt.wantErr {
				t.Errorf("YumManager.Remove() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestApply_ReloadCommand tests the Apply method with custom reload commands
func TestApply_ReloadCommand(t *testing.T) {
	tempFile := filepath.Join(os.TempDir(), "test_reload_cmd_file")
	defer os.Remove(tempFile)

	sampleConfig := &config.ServerConfig{
		Files: []struct {
			Path          string `json:"path" yaml:"path"`
			Content       string `json:"content" yaml:"content"`
			Mode          string `json:"mode,omitempty" yaml:"mode,omitempty"`
			ReloadService string `json:"reloadService" yaml:"reloadService"`
			ReloadCommand string `json:"reloadCommand" yaml:"reloadCommand"`
		}{
			{
				Path:          tempFile,
				Content:       "test content",
				ReloadCommand: "echo reload command executed",
			},
		},
		Services: []struct {
			Name    string `json:"name" yaml:"name"`
			Enabled bool   `json:"enabled" yaml:"enabled"`
			State   string `json:"state" yaml:"state"`
			Managed bool   `json:"managed" yaml:"managed"`
		}{},
		System: struct {
			ServiceManager string `json:"serviceManager" yaml:"serviceManager"`
			Containerized  bool   `json:"containerized" yaml:"containerized"`
		}{
			ServiceManager: "test",
			Containerized:  true,
		},
	}

	mockSvc := newMockServiceManager()

	// Track commands executed
	var capturedCommands []struct {
		Name string
		Args []string
	}

	mockRunCommand := func(name string, args ...string) (string, error) {
		capturedCommands = append(capturedCommands, struct {
			Name string
			Args []string
		}{
			Name: name,
			Args: args,
		})
		return "mock response", nil
	}

	originalRunCommand := config.ExportSetRunCommandFunc(mockRunCommand)
	t.Cleanup(func() {
		config.ExportSetRunCommandFunc(originalRunCommand)
	})

	mockPkg := &mockPackageManager{}

	err := sampleConfig.Apply(mockSvc, mockPkg)
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))

	// Check if our echo command was executed
	echoFound := false
	for _, cmd := range capturedCommands {
		if cmd.Name == "echo" {
			echoFound = true
			break
		}
	}
	assert.True(t, echoFound, "Expected echo command to be executed")
}

func TestApply_FileModeChangeTriggersReload(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "mode-change")

	require.NoError(t, os.WriteFile(filePath, []byte("mode-content"), 0644))

	var capturedCommands []struct {
		Name string
		Args []string
	}

	mockRunCommand := func(name string, args ...string) (string, error) {
		capturedCommands = append(capturedCommands, struct {
			Name string
			Args []string
		}{
			Name: name,
			Args: args,
		})
		return "", nil
	}

	originalRunCommand := config.ExportSetRunCommandFunc(mockRunCommand)
	t.Cleanup(func() {
		config.ExportSetRunCommandFunc(originalRunCommand)
	})

	cfg := &config.ServerConfig{
		Files: []struct {
			Path          string `json:"path" yaml:"path"`
			Content       string `json:"content" yaml:"content"`
			Mode          string `json:"mode,omitempty" yaml:"mode,omitempty"`
			ReloadService string `json:"reloadService" yaml:"reloadService"`
			ReloadCommand string `json:"reloadCommand" yaml:"reloadCommand"`
		}{
			{
				Path:          filePath,
				Content:       "mode-content",
				Mode:          "0600",
				ReloadService: "reload-service",
				ReloadCommand: "echo mode change",
			},
		},
		System: struct {
			ServiceManager string `json:"serviceManager" yaml:"serviceManager"`
			Containerized  bool   `json:"containerized" yaml:"containerized"`
		}{
			ServiceManager: "systemd",
		},
	}

	mockSvc := newMockServiceManager()

	err := cfg.Apply(mockSvc, &mockPackageManager{})
	require.NoError(t, err)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "expected mode change to 0600")

	assert.Equal(t, []mockCall{{Method: "ReloadOrRestart", Name: "reload-service"}}, mockSvc.calls)

	require.Len(t, capturedCommands, 1, "expected reload command due to mode change")
	assert.Equal(t, "echo", capturedCommands[0].Name)
	assert.Equal(t, []string{"mode", "change"}, capturedCommands[0].Args)
}

func TestApply_FileModeNoChangeSkipsReload(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "mode-no-change")

	require.NoError(t, os.WriteFile(filePath, []byte("mode-content"), 0600))

	var capturedCommands []struct {
		Name string
		Args []string
	}

	mockRunCommand := func(name string, args ...string) (string, error) {
		capturedCommands = append(capturedCommands, struct {
			Name string
			Args []string
		}{
			Name: name,
			Args: args,
		})
		return "", nil
	}

	originalRunCommand := config.ExportSetRunCommandFunc(mockRunCommand)
	t.Cleanup(func() {
		config.ExportSetRunCommandFunc(originalRunCommand)
	})

	cfg := &config.ServerConfig{
		Files: []struct {
			Path          string `json:"path" yaml:"path"`
			Content       string `json:"content" yaml:"content"`
			Mode          string `json:"mode,omitempty" yaml:"mode,omitempty"`
			ReloadService string `json:"reloadService" yaml:"reloadService"`
			ReloadCommand string `json:"reloadCommand" yaml:"reloadCommand"`
		}{
			{
				Path:          filePath,
				Content:       "mode-content",
				Mode:          "0600",
				ReloadService: "reload-service",
				ReloadCommand: "echo should not run",
			},
		},
		System: struct {
			ServiceManager string `json:"serviceManager" yaml:"serviceManager"`
			Containerized  bool   `json:"containerized" yaml:"containerized"`
		}{
			ServiceManager: "systemd",
		},
	}

	mockSvc := newMockServiceManager()

	err := cfg.Apply(mockSvc, &mockPackageManager{})
	require.NoError(t, err)

	assert.Empty(t, mockSvc.calls, "expected no reload when file content and mode already match")
	assert.Empty(t, capturedCommands, "expected no reload command execution when no changes detected")
}

func TestApply_InvalidFileMode(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "invalid-mode")

	cfg := &config.ServerConfig{
		Files: []struct {
			Path          string `json:"path" yaml:"path"`
			Content       string `json:"content" yaml:"content"`
			Mode          string `json:"mode,omitempty" yaml:"mode,omitempty"`
			ReloadService string `json:"reloadService" yaml:"reloadService"`
			ReloadCommand string `json:"reloadCommand" yaml:"reloadCommand"`
		}{
			{
				Path:    filePath,
				Content: "data",
				Mode:    "not-octal",
			},
		},
		System: struct {
			ServiceManager string `json:"serviceManager" yaml:"serviceManager"`
			Containerized  bool   `json:"containerized" yaml:"containerized"`
		}{
			ServiceManager: "systemd",
		},
	}

	err := cfg.Apply(newMockServiceManager(), &mockPackageManager{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid file mode 'not-octal'")

	_, statErr := os.Stat(filePath)
	assert.True(t, os.IsNotExist(statErr), "expected no file to be created when mode is invalid")
}

func TestCheckDrift_InvalidFileMode(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "invalid-mode")

	require.NoError(t, os.WriteFile(filePath, []byte("mode-content"), 0644))

	cfg := &config.ServerConfig{
		Files: []struct {
			Path          string `json:"path" yaml:"path"`
			Content       string `json:"content" yaml:"content"`
			Mode          string `json:"mode,omitempty" yaml:"mode,omitempty"`
			ReloadService string `json:"reloadService" yaml:"reloadService"`
			ReloadCommand string `json:"reloadCommand" yaml:"reloadCommand"`
		}{
			{
				Path:    filePath,
				Content: "mode-content",
				Mode:    "bad-mode",
			},
		},
		System: struct {
			ServiceManager string `json:"serviceManager" yaml:"serviceManager"`
			Containerized  bool   `json:"containerized" yaml:"containerized"`
		}{
			ServiceManager: "systemd",
		},
	}

	drift, err := cfg.CheckDrift(newMockServiceManager(), &mockPackageManager{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mode for file")
	assert.Empty(t, drift, "expected no drift details when file mode parsing fails")
}

// TestApply_ManagedFlag tests that services with managed=false are skipped
func TestApply_ManagedFlag(t *testing.T) {
	sampleConfig := &config.ServerConfig{
		Services: []struct {
			Name    string `json:"name" yaml:"name"`
			Enabled bool   `json:"enabled" yaml:"enabled"`
			State   string `json:"state" yaml:"state"`
			Managed bool   `json:"managed" yaml:"managed"`
		}{
			{Name: "managed-service", Enabled: true, State: "running", Managed: true},
			{Name: "unmanaged-service", Enabled: true, State: "running", Managed: false},
		},
		System: struct {
			ServiceManager string `json:"serviceManager" yaml:"serviceManager"`
			Containerized  bool   `json:"containerized" yaml:"containerized"`
		}{
			ServiceManager: "test",
		},
	}

	mockSvc := newMockServiceManager()

	err := sampleConfig.Apply(mockSvc, &mockPackageManager{})
	require.NoError(t, err)

	managedCalled := false
	unmanagedCalled := false

	for _, call := range mockSvc.calls {
		if call.Name == "managed-service" {
			managedCalled = true
		}
		if call.Name == "unmanaged-service" {
			unmanagedCalled = true
		}
	}

	assert.True(t, managedCalled, "Expected calls for managed service")
	assert.False(t, unmanagedCalled, "Expected no calls for unmanaged service")
}
