package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

// ServerConfig struct to add a system config field
type ServerConfig struct {
	ServerMetadata struct {
		Name        string `json:"name" yaml:"name"`
		Environment string `json:"environment" yaml:"environment"`
	} `json:"serverMetadata" yaml:"serverMetadata"`

	Packages []struct {
		Name  string `json:"name" yaml:"name"`
		State string `json:"state" yaml:"state"`
	} `json:"packages" yaml:"packages"`

	Services []struct {
		Name    string `json:"name" yaml:"name"`
		Enabled bool   `json:"enabled" yaml:"enabled"`
		State   string `json:"state" yaml:"state"`
		Managed bool   `json:"managed" yaml:"managed"`
	} `json:"services" yaml:"services"`

	Files []struct {
		Path          string `json:"path" yaml:"path"`
		Content       string `json:"content" yaml:"content"`
		Mode          string `json:"mode,omitempty" yaml:"mode,omitempty"`
		ReloadService string `json:"reloadService" yaml:"reloadService"`
		ReloadCommand string `json:"reloadCommand" yaml:"reloadCommand"`
	} `json:"files" yaml:"files"`

	System struct {
		ServiceManager string `json:"serviceManager" yaml:"serviceManager"`
		Containerized  bool   `json:"containerized" yaml:"containerized"`
	} `json:"system" yaml:"system"`
}

// runCommandFunc holds the function used to execute commands. It defaults to the actual runCommand,
var runCommandFunc = runCommand

// lookPathFunc holds the function used to look up executables in PATH. It defaults to exec.LookPath,
var lookPathFunc = exec.LookPath

// RunCommandFunc defines the signature for the command execution function.
type RunCommandFunc func(name string, arg ...string) (string, error)

// LookPathFunc defines the signature for the executable lookup function.
type LookPathFunc func(file string) (string, error)

// ExportSetRunCommandFunc allows tests to replace the command execution function.
func ExportSetRunCommandFunc(newFunc RunCommandFunc) RunCommandFunc {
	original := runCommandFunc
	if newFunc != nil {
		runCommandFunc = newFunc
	}
	return original
}

// ExportSetExecLookPath allows tests to replace the executable lookup function.
func ExportSetExecLookPath(newFunc LookPathFunc) LookPathFunc {
	original := lookPathFunc
	lookPathFunc = newFunc
	return original
}

// PackageManager defines the interface for interacting with the system's package manager.
type PackageManager interface {
	Install(name string, version string) error
	Remove(name string) error
	IsInstalled(name string) (bool, error)
}

// ApkManager implements PackageManager using apk commands.
type ApkManager struct{}

func (a ApkManager) Install(name string, version string) error {
	args := []string{"add"}
	if version == "latest" || version == "" {
		// apk add <package> implicitly installs latest if not installed, updates if installed
		args = append(args, name)
	} else {
		// Specify version
		args = append(args, fmt.Sprintf("%s=%s", name, version))
	}
	_, err := runCommandFunc("apk", args...)
	return err
}

func (a ApkManager) Remove(name string) error {
	_, err := runCommandFunc("apk", "del", name)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
		installed, checkErr := a.IsInstalled(name)
		if checkErr == nil && !installed {
			log.Printf("apk del for %s failed, but package is not installed. Ignoring error.", name)
			return nil
		}
		log.Printf("apk del for %s failed with unexpected error or package was installed: %v", name, err)
		return err
	}
	return err
}

func (a ApkManager) IsInstalled(name string) (bool, error) {
	output, err := runCommandFunc("apk", "list", "-I")
	if err != nil {
		log.Printf("Warning: 'apk list -I' failed: %v. Assuming package %s is not installed.", err, name)
		return false, nil
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	searchPrefix := name + "-"
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, searchPrefix) {
			return true, nil
		}
	}
	return false, nil
}

type AptManager struct{}

func (a AptManager) Install(name string, version string) error {
	log.Printf("Attempting apt install for %s, version %s", name, version)
	log.Println("Running apt-get update...")
	_, updateErr := runCommandFunc("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "update", "-y")
	if updateErr != nil {
		log.Printf("Warning: apt-get update failed: %v", updateErr)
	}

	args := []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y"}
	packageSpec := name
	if version != "latest" && version != "" {
		packageSpec = fmt.Sprintf("%s=%s", name, version)
	}
	args = append(args, packageSpec)

	_, err := runCommandFunc(args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("apt install failed for %s: %w", packageSpec, err)
	}
	log.Printf("Successfully installed/updated package %s via apt", packageSpec)
	return nil
}

func (a AptManager) Remove(name string) error {
	log.Printf("Attempting apt remove for %s", name)
	installed, err := a.IsInstalled(name)
	if err != nil {
		return fmt.Errorf("failed to check if package %s is installed before removal: %w", name, err)
	}
	if !installed {
		log.Printf("Package %s is not installed, skipping removal.", name)
		return nil
	}

	args := []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "remove", "-y", name}
	_, err = runCommandFunc(args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("apt remove failed for %s: %w", name, err)
	}
	log.Printf("Successfully removed package %s via apt", name)
	return nil
}

func (a AptManager) IsInstalled(name string) (bool, error) {
	args := []string{"dpkg-query", "-W", "-f='${Status}'", name}
	output, err := runCommandFunc(args[0], args[1:]...)
	if err == nil && strings.Contains(output, "install ok installed") {
		return true, nil
	}

	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}

		return false, fmt.Errorf("dpkg-query check failed for %s: %w", name, err)
	}
	return false, nil
}

// YumManager implements PackageManager using yum/dnf commands.
// A dedicated DnfManager might be needed for advanced features.
type YumManager struct{}

func (y YumManager) Install(name string, version string) error {
	log.Printf("Attempting yum/dnf install for %s, version %s", name, version)

	if detectCommand("dnf") {
		args := []string{"dnf", "install", "-y"}
		packageSpec := name
		if version != "latest" && version != "" {
			packageSpec = fmt.Sprintf("%s-%s", name, version)
		}
		args = append(args, packageSpec)

		_, err := runCommandFunc(args[0], args[1:]...)
		if err == nil {
			log.Printf("Successfully installed/updated package %s via dnf", packageSpec)
			return nil
		}
		log.Printf("Warning: dnf install failed for %s: %v", packageSpec, err)
	}

	// Fallback to yum
	args := []string{"yum", "install", "-y"}
	packageSpec := name
	if version != "latest" && version != "" {
		packageSpec = fmt.Sprintf("%s-%s", name, version)
	}
	args = append(args, packageSpec)

	_, err := runCommandFunc(args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("yum install failed for %s: %w", packageSpec, err)
	}
	log.Printf("Successfully installed/updated package %s via yum", packageSpec)
	return nil
}

func (y YumManager) Remove(name string) error {
	log.Printf("Attempting yum/dnf remove for %s", name)
	installed, err := y.IsInstalled(name)
	if err != nil {
		return fmt.Errorf("failed to check if package %s is installed before removal: %w", name, err)
	}
	if !installed {
		log.Printf("Package %s is not installed, skipping removal.", name)
		return nil
	}

	if detectCommand("dnf") {
		args := []string{"dnf", "remove", "-y", name}
		_, err := runCommandFunc(args[0], args[1:]...)
		if err == nil {
			log.Printf("Successfully removed package %s via dnf", name)
			return nil
		}
		log.Printf("Warning: dnf remove failed for %s: %v", name, err)
	}

	args := []string{"yum", "remove", "-y", name}
	_, err = runCommandFunc(args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("yum remove failed for %s: %w", name, err)
	}
	log.Printf("Successfully removed package %s via yum", name)
	return nil
}

func (y YumManager) IsInstalled(name string) (bool, error) {
	args := []string{"rpm", "-q", name}
	_, err := runCommandFunc(args[0], args[1:]...)
	if err == nil {
		return true, nil
	}

	if strings.Contains(err.Error(), "exit status 1") {
		return false, nil
	}

	return false, fmt.Errorf("rpm query failed for %s: %w", name, err)
}

// DetectPackageManager attempts to determine the system's package manager.
func DetectPackageManager() PackageManager {
	if detectCommand("dnf") {
		log.Println("Detected dnf, using YumManager.")
		return YumManager{}
	}
	if detectCommand("yum") {
		log.Println("Detected yum, using YumManager.")
		return YumManager{}
	}
	if detectCommand("apt-get") {
		log.Println("Detected apt, using AptManager.")
		return AptManager{}
	}
	if detectCommand("apk") {
		log.Println("Detected apk, using ApkManager.")
		return ApkManager{}
	}

	log.Println("Warning: Could not detect dnf, yum, apt, or apk. No package manager configured.")
	return UnsupportedManager{}
}

// UnsupportedManager is a placeholder for when no supported package manager is found.
type UnsupportedManager struct{}

func (u UnsupportedManager) Install(name string, version string) error {
	return fmt.Errorf("no supported package manager (dnf, yum, apt, apk) detected on this system")
}

func (u UnsupportedManager) Remove(name string) error {
	return fmt.Errorf("no supported package manager (dnf, yum, apt, apk) detected on this system")
}

func (u UnsupportedManager) IsInstalled(name string) (bool, error) {
	return false, fmt.Errorf("no supported package manager (dnf, yum, apt, apk) detected on this system")
}

// ServiceManager defines the interface for interacting with the system's service manager.
type ServiceManager interface {
	Start(name string) error
	Stop(name string) error
	Enable(name string) error
	Disable(name string) error
	IsActive(name string) (bool, error)
	IsEnabled(name string) (bool, error)
	ReloadOrRestart(name string) error
}

// SystemdManager implements ServiceManager using systemctl commands.
type SystemdManager struct{}

func (s SystemdManager) Start(name string) error {
	_, err := runCommandFunc("systemctl", "start", name)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		log.Printf("Systemctl start for %s returned exit code 1, likely already running.", name)
		return nil
	}
	return err
}

func (s SystemdManager) Stop(name string) error {
	_, err := runCommandFunc("systemctl", "stop", name)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 5 {
		log.Printf("Systemctl stop for %s returned exit code 5, likely already stopped.", name)
		return nil
	}
	return err
}

func (s SystemdManager) Enable(name string) error {
	_, err := runCommandFunc("systemctl", "enable", name)
	return err
}

func (s SystemdManager) Disable(name string) error {
	_, err := runCommandFunc("systemctl", "disable", name)
	return err
}

func (s SystemdManager) IsActive(name string) (bool, error) {
	_, err := runCommandFunc("systemctl", "is-active", "--quiet", name)
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 3 {
		return false, nil
	}
	return false, err
}

func (s SystemdManager) IsEnabled(name string) (bool, error) {
	_, err := runCommandFunc("systemctl", "is-enabled", "--quiet", name)
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func (s SystemdManager) ReloadOrRestart(name string) error {

	_, err := runCommandFunc("systemctl", "reload-or-restart", name)

	if err != nil {
		log.Printf("Warning: systemctl reload-or-restart %s failed: %v", name, err)

	}
	return nil
}

// SupervisorManager provides service management via supervisord
type SupervisorManager struct{}

func (s SupervisorManager) Start(serviceName string) error {
	log.Printf("Starting service %s via supervisorctl...", serviceName)
	output, err := runCommand("supervisorctl", "start", serviceName)
	if err != nil {
		log.Printf("Error starting service %s: %v, output: %s", serviceName, err, output)
		return fmt.Errorf("failed to start service %s: %w", serviceName, err)
	}
	log.Printf("Service %s started successfully", serviceName)
	return nil
}

func (s SupervisorManager) Stop(serviceName string) error {
	log.Printf("Stopping service %s via supervisorctl...", serviceName)
	output, err := runCommand("supervisorctl", "stop", serviceName)
	if err != nil {
		log.Printf("Error stopping service %s: %v, output: %s", serviceName, err, output)
		return fmt.Errorf("failed to stop service %s: %w", serviceName, err)
	}
	log.Printf("Service %s stopped successfully", serviceName)
	return nil
}

func (s SupervisorManager) Enable(serviceName string) error {
	log.Printf("Enable operation for service %s simulated in supervisord context", serviceName)
	return nil
}

func (s SupervisorManager) Disable(serviceName string) error {
	log.Printf("Disable operation for service %s simulated in supervisord context", serviceName)
	return nil
}

func (s SupervisorManager) IsActive(serviceName string) (bool, error) {
	log.Printf("Checking if service %s is active via supervisorctl...", serviceName)
	output, err := runCommand("supervisorctl", "status", serviceName)

	if err != nil && (strings.Contains(output, "not found") || strings.Contains(output, "no such")) {
		log.Printf("Service %s not found in supervisor", serviceName)
		return false, fmt.Errorf("service %s not found in supervisor configuration", serviceName)
	}

	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		if strings.Contains(output, "FATAL") || strings.Contains(output, "ERROR") {
			log.Printf("Error checking active status for service %s: %v, output: %s", serviceName, err, output)
			return false, fmt.Errorf("supervisor error when checking %s status: %s", serviceName, output)
		}

		if strings.Contains(output, "STOPPED") || strings.Contains(output, "EXITED") {
			log.Printf("Service %s exists but is not running (state: STOPPED/EXITED)", serviceName)
			return false, nil
		}

		log.Printf("Service %s appears to exist but not be running", serviceName)
		return false, nil
	}

	if err != nil {
		log.Printf("Technical error checking active status for service %s: %v", serviceName, err)
		return false, fmt.Errorf("technical error checking %s status: %w", serviceName, err)
	}

	isRunning := strings.Contains(output, "RUNNING")
	log.Printf("Service %s active status: %t", serviceName, isRunning)
	return isRunning, nil
}

func (s SupervisorManager) IsEnabled(serviceName string) (bool, error) {
	log.Printf("Checking if service %s is enabled via supervisorctl...", serviceName)
	output, err := runCommand("supervisorctl", "status", serviceName)

	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 && exitErr.ExitCode() != 1 {
		log.Printf("Supervisorctl command failed for %s. Attempting fallback to config file check.", serviceName)

		configFiles := []string{
			"/etc/supervisor/conf.d/" + serviceName + ".conf",
			"/etc/supervisord.d/" + serviceName + ".ini",
			"/etc/supervisor.d/" + serviceName + ".ini",
			"/etc/supervisor/supervisord.conf",
		}

		for _, cfgFile := range configFiles {
			if _, statErr := os.Stat(cfgFile); statErr == nil {
				content, readErr := os.ReadFile(cfgFile)
				if readErr == nil {
					if strings.Contains(string(content), "[program:"+serviceName+"]") {
						log.Printf("Service %s is enabled (found in config file %s)", serviceName, cfgFile)
						return true, nil
					}
				} else {
					log.Printf("Failed to read config file %s: %v", cfgFile, readErr)
				}
			}
		}

		return false, fmt.Errorf("supervisorctl failed with exit code %d and service not found in config files", exitErr.ExitCode())
	}

	if err != nil && (strings.Contains(output, "not found") || strings.Contains(output, "no such")) {
		log.Printf("Service %s not found in supervisor (not enabled)", serviceName)
		return false, nil
	}

	if err != nil {
		log.Printf("Technical error checking enabled status for service %s: %v, output: %s", serviceName, err, output)
		return false, fmt.Errorf("technical error checking if %s is enabled: %w", serviceName, err)
	}

	log.Printf("Service %s is enabled (exists in supervisor config)", serviceName)
	return true, nil
}

func (s SupervisorManager) ReloadOrRestart(serviceName string) error {
	log.Printf("Restarting service %s via supervisorctl...", serviceName)
	output, err := runCommand("supervisorctl", "restart", serviceName)
	if err != nil {
		log.Printf("Error restarting service %s: %v, output: %s", serviceName, err, output)
		return fmt.Errorf("failed to restart service %s: %w", serviceName, err)
	}
	log.Printf("Service %s restarted successfully", serviceName)
	return nil
}

// OpenRCManager implements ServiceManager using rc-service and rc-update commands.
type OpenRCManager struct{}

func (o OpenRCManager) Start(name string) error {
	_, err := runCommandFunc("rc-service", name, "start")
	return err
}

func (o OpenRCManager) Stop(name string) error {
	_, err := runCommandFunc("rc-service", name, "stop")
	return err
}

func (o OpenRCManager) Enable(name string) error {
	_, err := runCommandFunc("rc-update", "add", name, "default")
	return err
}

func (o OpenRCManager) Disable(name string) error {
	_, err := runCommandFunc("rc-update", "del", name, "default")
	return err
}

func (o OpenRCManager) IsActive(name string) (bool, error) {
	_, err := runCommandFunc("rc-service", name, "status")
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 3 {
			return false, nil
		}
	}
	return false, err
}

func (o OpenRCManager) IsEnabled(name string) (bool, error) {
	output, err := runCommandFunc("rc-update", "show", "-v")
	if err != nil {
		return false, fmt.Errorf("failed to run rc-update show: %w", err)
	}

	lines := strings.SplitSeq(output, "\n")
	for line := range lines {
		if strings.Contains(line, "|") && strings.Contains(line, "default") {
			parts := strings.Split(line, "|")
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == name {
				return true, nil
			}
		}
	}
	return false, nil
}

func (o OpenRCManager) ReloadOrRestart(name string) error {
	_, err := runCommandFunc("rc-service", name, "reload")
	if err == nil {
		return nil
	}
	log.Printf("Warning: rc-service %s reload failed (%v), attempting restart...", name, err)
	_, restartErr := runCommandFunc("rc-service", name, "restart")
	if restartErr != nil {
		log.Printf("Warning: rc-service %s restart also failed: %v", name, restartErr)
		return restartErr
	}
	return nil
}

// detectCommand checks if a command exists in the system's PATH.
func detectCommand(command string) bool {
	_, err := lookPathFunc(command)
	return err == nil
}

func LoadConfig(path string) (*ServerConfig, error) {
	log.Printf("Loading configuration from %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Error reading config file: %v", err)
		return nil, err
	}

	log.Printf("File content read: %d bytes", len(data))

	var config ServerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Printf("Error parsing YAML: %v", err)
		return nil, err
	}

	log.Printf("Configuration loaded: server=%s, env=%s, %d packages, %d services, %d files",
		config.ServerMetadata.Name,
		config.ServerMetadata.Environment,
		len(config.Packages),
		len(config.Services),
		len(config.Files))

	return &config, nil
}

func runCommand(name string, arg ...string) (string, error) {
	cmd := exec.Command(name, arg...)
	log.Printf("Running command: %s %v", name, arg)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return outputStr, exitErr
		}
		return outputStr, fmt.Errorf("command %s failed: %v, output: %s", name, err, outputStr)
	}
	log.Printf("Command output: %s", outputStr)
	return outputStr, nil
}

// getFileSHA256 calculates the SHA256 hash of a file's content.
// Returns empty string if file doesn't exist or error occurs.
func getFileSHA256(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Printf("Error hashing file %s: %v", filePath, err)
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func parseFileMode(modeStr string) (os.FileMode, bool, error) {
	if modeStr == "" {
		return 0, false, nil
	}

	value, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return 0, true, fmt.Errorf("invalid file mode '%s': %w", modeStr, err)
	}
	return os.FileMode(value), true, nil
}

func formatFileMode(mode os.FileMode) string {
	return fmt.Sprintf("%04o", mode.Perm())
}

// Apply applies the configuration state using the provided ServiceManager and PackageManager.
func (c *ServerConfig) Apply(svcManager ServiceManager, pkgManager PackageManager) error {
	log.Println("Applying configuration...")
	var applyErrors []string
	needsRestart := map[string]bool{}
	reloadCommands := map[string]string{}

	log.Printf("Processing %d package states...", len(c.Packages))
	for _, pkg := range c.Packages {
		log.Printf("Processing package %s, desired state: %s", pkg.Name, pkg.State)
		installed, err := pkgManager.IsInstalled(pkg.Name)
		if err != nil {
			errMsg := fmt.Sprintf("failed to check installed state for package %s: %v", pkg.Name, err)
			log.Println("Error:", errMsg)
			applyErrors = append(applyErrors, errMsg)
			continue
		}

		switch pkg.State {
		case "present", "latest":
			if !installed {
				log.Printf("Package %s not installed, installing...", pkg.Name)
				if err := pkgManager.Install(pkg.Name, pkg.State); err != nil {
					errMsg := fmt.Sprintf("failed to install package %s: %v", pkg.Name, err)
					log.Println("Error:", errMsg)
					applyErrors = append(applyErrors, errMsg)
				} else {
					log.Printf("Successfully installed/updated package %s", pkg.Name)
				}
			} else {
				log.Printf("Package %s already installed.", pkg.Name)
				if pkg.State == "latest" {
					log.Printf("Ensuring package %s is latest (logic depends on PackageManager implementation)", pkg.Name)
				}
			}
		case "absent":
			if installed {
				log.Printf("Package %s installed, removing...", pkg.Name)
				if err := pkgManager.Remove(pkg.Name); err != nil {
					errMsg := fmt.Sprintf("failed to remove package %s: %v", pkg.Name, err)
					log.Println("Error:", errMsg)
					applyErrors = append(applyErrors, errMsg)
				} else {
					log.Printf("Successfully removed package %s", pkg.Name)
				}
			} else {
				log.Printf("Package %s already absent.", pkg.Name)
			}
		default:
			errMsg := fmt.Sprintf("unknown state '%s' for package %s", pkg.State, pkg.Name)
			log.Println("Warning:", errMsg)
			applyErrors = append(applyErrors, errMsg)
		}
	}

	log.Printf("Processing %d file states...", len(c.Files))
	for _, file := range c.Files {
		log.Printf("Ensuring file state: %s (%d bytes)", file.Path, len(file.Content))

		desiredBytes := []byte(file.Content)
		desiredHash := sha256.Sum256(desiredBytes)
		desiredHashStr := hex.EncodeToString(desiredHash[:])
		currentHash := getFileSHA256(file.Path)
		contentChanged := currentHash != desiredHashStr

		if contentChanged {
			writeMode := os.FileMode(0644)
			if parsedMode, hasMode, err := parseFileMode(file.Mode); err != nil {
				log.Printf("Error parsing mode for file %s: %v", file.Path, err)
				applyErrors = append(applyErrors, err.Error())
				continue
			} else if hasMode {
				writeMode = parsedMode
			}

			if err := os.WriteFile(file.Path, desiredBytes, writeMode); err != nil {
				log.Printf("Error writing file %s: %v", file.Path, err)
				applyErrors = append(applyErrors, fmt.Sprintf("failed to write file %s: %v", file.Path, err))
				continue
			}
			log.Printf("File %s written successfully", file.Path)
		} else {
			log.Printf("File %s already matches desired content; skipping write", file.Path)
		}

		modeChanged := false
		if parsedMode, hasMode, err := parseFileMode(file.Mode); err != nil {
			log.Printf("Error parsing mode for file %s: %v", file.Path, err)
			applyErrors = append(applyErrors, err.Error())
			continue
		} else if hasMode {
			info, err := os.Stat(file.Path)
			if err != nil {
				log.Printf("Error stating file %s: %v", file.Path, err)
				applyErrors = append(applyErrors, fmt.Sprintf("failed to stat file %s: %v", file.Path, err))
			} else if info.Mode().Perm() != parsedMode.Perm() {
				if err := os.Chmod(file.Path, parsedMode); err != nil {
					log.Printf("Error setting mode on file %s: %v", file.Path, err)
					applyErrors = append(applyErrors, fmt.Sprintf("failed to set mode on file %s: %v", file.Path, err))
				} else {
					modeChanged = true
					log.Printf("Updated mode for file %s to %s", file.Path, formatFileMode(parsedMode))
				}
			}
		}

		if (contentChanged || modeChanged) && file.ReloadService != "" {
			needsRestart[file.ReloadService] = true
		}
		if (contentChanged || modeChanged) && file.ReloadCommand != "" {
			reloadCommands[file.Path] = file.ReloadCommand
		}
	}

	log.Printf("Processing %d service states...", len(c.Services))
	for _, svc := range c.Services {
		log.Printf("Managing service: %s, enabled: %v, state: %s, managed: %v", svc.Name, svc.Enabled, svc.State, svc.Managed)

		// Skip service management if managed=false
		if !svc.Managed {
			log.Printf("Service %s has managed=false flag, skipping direct service management", svc.Name)
			continue
		}

		if svc.Enabled {
			if err := svcManager.Enable(svc.Name); err != nil {
				log.Printf("Warning: Could not enable service %s: %v", svc.Name, err)
				applyErrors = append(applyErrors, fmt.Sprintf("failed to enable service %s: %v", svc.Name, err))
			}
		} else {
			if err := svcManager.Disable(svc.Name); err != nil {
				log.Printf("Warning: Could not disable service %s: %v", svc.Name, err)
				applyErrors = append(applyErrors, fmt.Sprintf("failed to disable service %s: %v", svc.Name, err))
			}
		}
		isActive, err := svcManager.IsActive(svc.Name)
		if err != nil {
			log.Printf("Warning: Could not check active status for service %s: %v", svc.Name, err)
			applyErrors = append(applyErrors, fmt.Sprintf("failed to check active status for service %s: %v", svc.Name, err))
			continue
		}
		switch svc.State {
		case "running":
			if !isActive {
				log.Printf("Service %s is not running. Attempting to start...", svc.Name)
				if err := svcManager.Start(svc.Name); err != nil {
					log.Printf("Warning: Error starting service %s: %v", svc.Name, err)
					applyErrors = append(applyErrors, fmt.Sprintf("failed to start service %s: %v", svc.Name, err))
				}
			} else {
				log.Printf("Service %s is already running.", svc.Name)
			}
		case "stopped":
			if isActive {
				log.Printf("Service %s is running. Attempting to stop...", svc.Name)
				if err := svcManager.Stop(svc.Name); err != nil {
					log.Printf("Warning: Error stopping service %s: %v", svc.Name, err)
					applyErrors = append(applyErrors, fmt.Sprintf("failed to stop service %s: %v", svc.Name, err))
				}
			} else {
				log.Printf("Service %s is already stopped.", svc.Name)
			}
		default:
			log.Printf("Warning: Unknown service state '%s' for service '%s'", svc.State, svc.Name)
			applyErrors = append(applyErrors, fmt.Sprintf("unknown service state '%s' for service %s", svc.State, svc.Name))
		}
	}

	for serviceName := range needsRestart {
		log.Printf("Reloading/restarting service %s due to file change using service manager...", serviceName)
		if err := svcManager.ReloadOrRestart(serviceName); err != nil {
			applyErrors = append(applyErrors, fmt.Sprintf("failed to reload/restart service %s: %v", serviceName, err))
		}
	}

	// Process custom reload commands
	for filePath, cmd := range reloadCommands {
		log.Printf("Executing reload command for file %s: %s", filePath, cmd)
		cmdParts := strings.Fields(cmd)
		if len(cmdParts) > 0 {
			var args []string
			if len(cmdParts) > 1 {
				args = cmdParts[1:]
			}
			output, err := runCommandFunc(cmdParts[0], args...)
			if err != nil {
				errMsg := fmt.Sprintf("reload command failed for file %s: %v, output: %s", filePath, err, output)
				log.Printf("Error: %s", errMsg)
				applyErrors = append(applyErrors, errMsg)
			} else {
				log.Printf("Reload command executed successfully for file %s", filePath)
			}
		} else {
			errMsg := fmt.Sprintf("invalid reload command for file %s: %s", filePath, cmd)
			log.Printf("Error: %s", errMsg)
			applyErrors = append(applyErrors, errMsg)
		}
	}

	if len(applyErrors) > 0 {
		return fmt.Errorf("encountered errors during apply: %s", strings.Join(applyErrors, "; "))
	}
	log.Println("Configuration applied successfully.")
	return nil
}

// CheckDrift compares the current system state against the configuration.
func (c *ServerConfig) CheckDrift(svcManager ServiceManager, pkgManager PackageManager) ([]string, error) {
	log.Println("Checking for configuration drift...")
	var driftDetails []string
	var checkErrors []string

	if c.System.Containerized {
		log.Printf("System is running in a containerized environment as per config")
	}

	log.Printf("Checking drift for %d packages...", len(c.Packages))
	for _, pkg := range c.Packages {
		installed, err := pkgManager.IsInstalled(pkg.Name)
		if err != nil {
			errMsg := fmt.Sprintf("failed to check installed state for package %s: %v", pkg.Name, err)
			log.Println("Error checking package drift:", errMsg)
			checkErrors = append(checkErrors, errMsg)
			continue
		}

		drift := false
		switch pkg.State {
		case "present", "latest":
			if !installed {
				drift = true
				driftDetails = append(driftDetails, fmt.Sprintf("Package %s should be installed but is absent", pkg.Name))
			}
		case "absent":
			if installed {
				drift = true
				driftDetails = append(driftDetails, fmt.Sprintf("Package %s should be absent but is installed", pkg.Name))
			}
		default:
			errMsg := fmt.Sprintf("unknown desired state '%s' for package %s during drift check", pkg.State, pkg.Name)
			log.Println("Warning:", errMsg)
			checkErrors = append(checkErrors, errMsg)
		}
		if drift {
			log.Printf("Drift detected: Package %s (Desired: %s, Actual installed: %t)", pkg.Name, pkg.State, installed)
		}
	}

	log.Printf("Checking drift for %d files...", len(c.Files))
	for _, file := range c.Files {
		info, err := os.Stat(file.Path)
		if err != nil {
			if os.IsNotExist(err) {
				if file.Content != "" {
					driftMsg := fmt.Sprintf("File '%s': desired state exists, actual state is missing", file.Path)
					log.Printf("Drift detected: %s", driftMsg)
					driftDetails = append(driftDetails, driftMsg)
				}
				continue
			}

			errMsg := fmt.Sprintf("failed to stat file %s: %v", file.Path, err)
			log.Printf("Error checking file drift: %s", errMsg)
			checkErrors = append(checkErrors, errMsg)
			continue
		}

		actualHash := getFileSHA256(file.Path)
		desiredHash := sha256.Sum256([]byte(file.Content))
		desiredHashStr := hex.EncodeToString(desiredHash[:])

		if actualHash != desiredHashStr {
			driftMsg := fmt.Sprintf("File '%s': content drift detected (desired: %s..., actual: %s...)", file.Path, desiredHashStr[:8], actualHash[:8])
			log.Printf("Drift detected: %s", driftMsg)
			driftDetails = append(driftDetails, driftMsg)
		}

		if parsedMode, hasMode, err := parseFileMode(file.Mode); err != nil {
			errMsg := fmt.Sprintf("invalid mode for file %s: %v", file.Path, err)
			log.Printf("Error checking file mode drift: %s", errMsg)
			checkErrors = append(checkErrors, errMsg)
		} else if hasMode {
			actualPerm := info.Mode().Perm()
			if actualPerm != parsedMode.Perm() {
				driftMsg := fmt.Sprintf("File '%s': desired mode %s, actual mode %s", file.Path, file.Mode, formatFileMode(info.Mode()))
				log.Printf("Drift detected: %s", driftMsg)
				driftDetails = append(driftDetails, driftMsg)
			}
		}
	}

	// Check service drift only if there are services defined in config
	if len(c.Services) > 0 {
		log.Printf("Checking drift for %d services...", len(c.Services))
		for _, svc := range c.Services {

			if !svc.Managed {
				log.Printf("Service %s has managed=false flag, skipping service drift check", svc.Name)
				continue
			}

			isEnabled, err := svcManager.IsEnabled(svc.Name)
			if err != nil {
				errMsg := fmt.Sprintf("failed to check enabled status for service %s: %v", svc.Name, err)
				log.Printf("Error: %s", errMsg)
				checkErrors = append(checkErrors, errMsg)
			} else if isEnabled != svc.Enabled {
				driftMsg := fmt.Sprintf("Service '%s': desired enabled state '%t', actual state is %t", svc.Name, svc.Enabled, isEnabled)
				log.Printf("Drift detected: %s", driftMsg)
				driftDetails = append(driftDetails, driftMsg)
			}

			isActive, err := svcManager.IsActive(svc.Name)
			if err != nil {
				errMsg := fmt.Sprintf("failed to check active status for service %s: %v", svc.Name, err)
				log.Printf("Error: %s", errMsg)
				checkErrors = append(checkErrors, errMsg)
			} else {
				desiredActive := (svc.State == "running")
				if isActive != desiredActive {
					driftMsg := fmt.Sprintf("Service '%s': desired running state '%s', actual state is %t", svc.Name, svc.State, isActive)
					log.Printf("Drift detected: %s", driftMsg)
					driftDetails = append(driftDetails, driftMsg)
				}
			}
		}
	} else {
		log.Printf("No services defined in config, skipping service drift check")
	}

	if len(checkErrors) > 0 {
		err := fmt.Errorf("errors encountered during drift check: %s", strings.Join(checkErrors, "; "))
		log.Printf("Error: %v", err)
		return driftDetails, err
	}

	if len(driftDetails) > 0 {
		log.Printf("Drift detected: %d issues found.", len(driftDetails))
	} else {
		log.Println("No configuration drift detected.")
	}
	return driftDetails, nil
}

// ConfigManager interface provides methods for loading, applying, and checking drift.
type ConfigManager interface {
	Load(filePath string) (*ServerConfig, error)
	Apply(cfg *ServerConfig, svcManager ServiceManager, pkgManager PackageManager) error
	CheckDrift(cfg *ServerConfig, svcManager ServiceManager, pkgManager PackageManager) ([]string, error)
}

// DefaultConfigManager implements ConfigManager using the standard functions.
type DefaultConfigManager struct{}

// NewDefaultConfigManager creates a new DefaultConfigManager.
func NewDefaultConfigManager() *DefaultConfigManager {
	return &DefaultConfigManager{}
}

// Load implements the ConfigManager interface.
func (d *DefaultConfigManager) Load(filePath string) (*ServerConfig, error) {
	return LoadConfig(filePath)
}

// Apply implements the ConfigManager interface.
func (d *DefaultConfigManager) Apply(cfg *ServerConfig, svcManager ServiceManager, pkgManager PackageManager) error {
	return cfg.Apply(svcManager, pkgManager)
}

// CheckDrift implements the ConfigManager interface.
func (d *DefaultConfigManager) CheckDrift(cfg *ServerConfig, svcManager ServiceManager, pkgManager PackageManager) ([]string, error) {
	return cfg.CheckDrift(svcManager, pkgManager)
}

// DetectServiceManager attempts to determine the init system and returns the appropriate ServiceManager.
func DetectServiceManager() ServiceManager {
	log.Println("[ServiceManagerDetection] Starting detection...")

	var currentConfig *ServerConfig
	configPaths := []string{
		"/etc/pullbase/config.yaml",
		"./config.yaml",
	}

	if configEnvPath := os.Getenv("PULLBASE_CONFIG_PATH"); configEnvPath != "" {
		configPaths = append([]string{configEnvPath}, configPaths...)
	}

	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			if cfg, err := LoadConfig(path); err == nil {
				currentConfig = cfg
				log.Printf("[ServiceManagerDetection] Loaded configuration from %s", path)
				break
			} else {
				log.Printf("[ServiceManagerDetection] Failed to load config from %s: %v", path, err)
			}
		}
	}

	// If config specifies service manager, that's definitive
	if currentConfig != nil && currentConfig.System.ServiceManager != "" {
		managerType := strings.ToLower(currentConfig.System.ServiceManager)
		log.Printf("[ServiceManagerDetection] Using service manager from config: %s", managerType)

		switch managerType {
		case "supervisor", "supervisord", "docker-supervisor":
			if _, err := lookPathFunc("supervisorctl"); err != nil {
				log.Printf("[ServiceManagerDetection] WARNING: supervisorctl not found but specified in config")
			}
			return SupervisorManager{}
		case "systemd":
			if _, err := lookPathFunc("systemctl"); err != nil {
				log.Printf("[ServiceManagerDetection] WARNING: systemctl not found but specified in config")
			}
			return SystemdManager{}
		case "openrc":
			if _, err := lookPathFunc("rc-service"); err != nil {
				log.Printf("[ServiceManagerDetection] WARNING: rc-service not found but specified in config")
			}
			return OpenRCManager{}
		default:
			log.Printf("[ServiceManagerDetection] WARNING: Unknown service manager type '%s' in config", managerType)
		}
	}

	log.Println("[ServiceManagerDetection] No service manager specified in config, auto-detecting...")

	if _, err := lookPathFunc("supervisorctl"); err == nil {
		_, cmdErr := runCommandFunc("supervisorctl", "version")
		if cmdErr == nil {
			log.Println("[ServiceManagerDetection] Found working supervisorctl, using SupervisorManager")
			return SupervisorManager{}
		}
		log.Printf("[ServiceManagerDetection] supervisorctl found but not working: %v", cmdErr)
	}

	if _, err := lookPathFunc("systemctl"); err == nil {
		if _, err := os.Stat("/run/systemd/system"); err == nil {
			log.Println("[ServiceManagerDetection] Found working systemd, using SystemdManager")
			return SystemdManager{}
		}
		log.Println("[ServiceManagerDetection] systemctl found but systemd not running")
	}

	if _, err := lookPathFunc("rc-service"); err == nil {
		_, err := runCommandFunc("rc-service", "--list")
		if err == nil {
			log.Println("[ServiceManagerDetection] Found working OpenRC, using OpenRCManager")
			return OpenRCManager{}
		}
		log.Printf("[ServiceManagerDetection] rc-service found but not working: %v", err)
	}

	log.Println("[ServiceManagerDetection] WARNING: Could not detect any supported service manager")
	log.Println("[ServiceManagerDetection] Services will not be managed - using UnsupportedServiceManager")
	return UnsupportedServiceManager{}
}

// UnsupportedServiceManager is a placeholder that provides appropriate error messages
// when no service manager can be detected
type UnsupportedServiceManager struct{}

func (u UnsupportedServiceManager) Start(name string) error {
	return fmt.Errorf("no supported service manager detected: cannot start service %s", name)
}

func (u UnsupportedServiceManager) Stop(name string) error {
	return fmt.Errorf("no supported service manager detected: cannot stop service %s", name)
}

func (u UnsupportedServiceManager) Enable(name string) error {
	return fmt.Errorf("no supported service manager detected: cannot enable service %s", name)
}

func (u UnsupportedServiceManager) Disable(name string) error {
	return fmt.Errorf("no supported service manager detected: cannot disable service %s", name)
}

func (u UnsupportedServiceManager) IsActive(name string) (bool, error) {
	return false, fmt.Errorf("no supported service manager detected: cannot check if service %s is active", name)
}

func (u UnsupportedServiceManager) IsEnabled(name string) (bool, error) {
	return false, fmt.Errorf("no supported service manager detected: cannot check if service %s is enabled", name)
}

func (u UnsupportedServiceManager) ReloadOrRestart(name string) error {
	return fmt.Errorf("no supported service manager detected: cannot reload/restart service %s", name)
}
