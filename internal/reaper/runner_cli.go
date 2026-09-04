package reaper

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed ori_reaper_runner.lua
var runnerLua string

const runnerScriptName = "ori-reaper-runner.lua"

func (m *Manager) OriDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ori-reaper")
}
func (m *Manager) RunnerScriptPath() string {
	if m == nil {
		return ""
	}
	dir := filepath.Clean(expandHome(m.ScriptsDir))
	if !filepath.IsAbs(dir) || dir == "." {
		return ""
	}
	return filepath.Join(dir, runnerScriptName)
}
func (m *Manager) InboxPath() string    { return filepath.Join(m.OriDir(), "inbox.lua") }
func (m *Manager) RunnerIDPath() string { return filepath.Join(m.OriDir(), "runner.id") }
func (m *Manager) statusPath() string   { return filepath.Join(m.OriDir(), "last_status.txt") }

func (m *Manager) ensureOriDir() error {
	if err := os.MkdirAll(m.OriDir(), 0o750); err != nil {
		return fmt.Errorf("create runner exchange: %w", err)
	}
	info, err := os.Lstat(m.OriDir())
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrRunnerRootUnsafe
	}
	return nil
}

func (m *Manager) InstallRunner() (string, error) {
	dir := filepath.Clean(expandHome(m.ScriptsDir))
	if !filepath.IsAbs(dir) || dir == "." {
		return "", ErrRunnerUnavailable
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", ErrRunnerUnavailable
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", ErrRunnerUnavailable
	}
	dest := m.RunnerScriptPath()
	if existing, statErr := os.Lstat(dest); statErr == nil && (existing.Mode()&os.ModeSymlink != 0 || !existing.Mode().IsRegular()) {
		return "", ErrRunnerUnavailable
	}
	if err := os.WriteFile(dest, []byte(runnerLua), 0o600); err != nil { // #nosec G304 -- fixed filename beneath trusted scripts root
		return "", ErrRunnerUnavailable
	}
	if err := m.ensureOriDir(); err != nil {
		return "", err
	}
	return fmt.Sprintf("Runner staged at %s. Load and run it once from REAPER's Action List.", dest), nil
}

func (m *Manager) ReadRunnerID() (string, error) {
	data, err := os.ReadFile(m.RunnerIDPath()) // #nosec G304 -- fixed file under the user's runner exchange
	if err != nil {
		return "", fmt.Errorf("runner is not initialized")
	}
	id := strings.TrimSpace(string(data))
	if !validRunnerCommandID(id) {
		return "", fmt.Errorf("runner id is invalid")
	}
	return id, nil
}

// Exec preserves the existing helper CLI behavior. Workspace Surface service
// calls use Runner.RunScript, which adds serialization and bounded receipts.
func (m *Manager) Exec(content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("inbox content is required")
	}
	if err := m.ensureOriDir(); err != nil {
		return "", err
	}
	id, err := m.ReadRunnerID()
	if err != nil {
		return "", err
	}
	_ = os.Remove(m.statusPath())
	if err := os.WriteFile(m.InboxPath(), []byte(content), 0o600); err != nil { // #nosec G304 -- fixed exchange inbox
		return "", ErrRunnerUnavailable
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(loopbackURL(m.ResolveWebRemotePort(), id)) // #nosec G107 -- fixed loopback URL and validated port/command
	if err != nil {
		return "", fmt.Errorf("REAPER Web Remote is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("REAPER Web Remote refused the runner")
	}
	if status := m.waitStatus(2 * time.Second); status != "" {
		if strings.HasPrefix(status, "error:") {
			return "", fmt.Errorf("runner reported an error")
		}
		return "runner status: " + status, nil
	}
	return "runner triggered", nil
}

func (m *Manager) waitStatus(timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(m.statusPath()); err == nil { // #nosec G304 -- fixed exchange status
			if status := strings.TrimSpace(string(data)); status != "" {
				return status
			}
		}
		time.Sleep(75 * time.Millisecond)
	}
	return ""
}
