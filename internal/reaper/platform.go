package reaper

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type Status struct {
	Running bool   `json:"running"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (m *Manager) GetStatus() Status {
	running, err := isReaperRunning()
	if err != nil {
		return Status{Running: false, Status: "Unknown", Message: fmt.Sprintf("Status unknown: %v", err)}
	}
	if running {
		return Status{Running: true, Status: "Running", Message: "REAPER is running"}
	}
	return Status{Running: false, Status: "Not running", Message: "REAPER is not running"}
}

func isReaperRunning() (bool, error) {
	procs, err := process.Processes()
	if err != nil {
		return false, err
	}

	currentPID := int32(os.Getpid())
	for _, p := range procs {
		if p.Pid == currentPID {
			continue
		}
		name, err := p.Name()
		if err != nil {
			continue
		}
		if isReaperProcessName(name) {
			return true, nil
		}
	}
	return false, nil
}

func isReaperProcessName(name string) bool {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "reaper-mcp") {
		return false
	}
	switch lower {
	case "reaper", "reaper.exe", "reaper64", "reaper64.exe":
		return true
	default:
		return false
	}
}

func launchScript(scriptPath string) error {
	if _, err := os.Stat(scriptPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("script not found: %s", scriptPath)
		}
		return fmt.Errorf("stat script: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("open", "-a", "Reaper", scriptPath)
		return cmd.Run()
	case "windows":
		cmd := exec.Command("cmd", "/c", "start", "", scriptPath)
		return cmd.Run()
	default:
		cmd := exec.Command("reaper", scriptPath)
		return cmd.Run()
	}
}

func normalizeScriptName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("script name is required")
	}
	if strings.Contains(trimmed, "..") || strings.ContainsAny(trimmed, `/\\`) {
		return "", fmt.Errorf("invalid script name: %q", name)
	}
	return trimmed, nil
}

func normalizeScriptType(scriptType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(scriptType)) {
	case "lua", ".lua", "":
		return ".lua", nil
	case "eel", ".eel":
		return ".eel", nil
	case "py", ".py", "python":
		return ".py", nil
	default:
		return "", fmt.Errorf("unsupported script_type: %s (supported: lua, eel, py)", scriptType)
	}
}

func scriptPath(dir, script, ext string) (string, error) {
	name, err := normalizeScriptName(script)
	if err != nil {
		return "", err
	}
	if ext != "" && !strings.HasSuffix(strings.ToLower(name), strings.ToLower(ext)) {
		name += ext
	}
	cleanDir := filepath.Clean(expandHome(dir))
	return filepath.Join(cleanDir, name), nil
}

func resolveExistingScriptPath(dir, script string) (string, error) {
	name, err := normalizeScriptName(script)
	if err != nil {
		return "", err
	}

	// if explicit extension, use as-is
	if strings.HasSuffix(strings.ToLower(name), ".lua") || strings.HasSuffix(strings.ToLower(name), ".eel") || strings.HasSuffix(strings.ToLower(name), ".py") {
		p, err := scriptPath(dir, name, "")
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("script not found: %s", script)
	}

	for _, ext := range []string{".lua", ".eel", ".py"} {
		p, err := scriptPath(dir, name, ext)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("script not found: %s", script)
}
