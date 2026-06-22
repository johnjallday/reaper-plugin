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

// runnerLua is the source of the persistent "runner" action, embedded so
// `install-runner` can write it out regardless of the working directory.
//
//go:embed ori_reaper_runner.lua
var runnerLua string

// runnerScriptName is the filename the runner is installed as in REAPER's
// Scripts directory.
const runnerScriptName = "ori-reaper-runner.lua"

// OriDir is the fixed scratch directory the agent and REAPER share. It is the
// ONLY path the (sandboxed) agent writes to; on Codex it should be added to
// sandbox_workspace_write.writable_roots.
func (m *Manager) OriDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ori-reaper")
}

// InboxPath is where the agent drops Lua for the runner to execute.
func (m *Manager) InboxPath() string { return filepath.Join(m.OriDir(), "inbox.lua") }

// RunnerIDPath is where the runner records its own Web Remote command ID.
func (m *Manager) RunnerIDPath() string { return filepath.Join(m.OriDir(), "runner.id") }

// statusPath is where the runner records the outcome of the last inbox run.
func (m *Manager) statusPath() string { return filepath.Join(m.OriDir(), "last_status.txt") }

func (m *Manager) ensureOriDir() error {
	if err := os.MkdirAll(m.OriDir(), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", m.OriDir(), err)
	}
	return nil
}

// InstallRunner stages the runner script into REAPER's Scripts directory. It does
// NOT edit reaper-kb.ini — registering a ReaScript by hand-editing that file while
// REAPER is running is unsafe (REAPER can overwrite it on quit) and the SCR line
// format is finicky. Instead the user loads the staged script once via REAPER's
// Actions list, which registers it live (no restart) and lets REAPER assign +
// persist the command ID itself. This is a ONE-TIME setup step; afterwards the
// runner is reused across REAPER restarts and workspaces.
func (m *Manager) InstallRunner() (string, error) {
	dir := expandHome(m.ScriptsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}
	dest := filepath.Join(dir, runnerScriptName)
	if err := os.WriteFile(dest, []byte(runnerLua), 0o644); err != nil {
		return "", fmt.Errorf("write runner script: %w", err)
	}
	if err := m.ensureOriDir(); err != nil {
		return "", err
	}

	return fmt.Sprintf(`Staged runner script: %s

One-time setup remaining (no REAPER restart needed):
  1. In REAPER: Actions → Show action list… → "New action…" ▾ → "Load ReaScript…"
     and select the file above. REAPER registers it live and assigns a command ID.
  2. With "ori-reaper-runner" selected in the action list, click "Run" once. On
     first run it records its Web Remote command ID to:
       %s
  3. Done. From now on `+"`reaper-plugin exec`"+` (and the skills) drive REAPER with
     no further setup; the command ID is stable across restarts.

Scratch dir (the only path the agent writes): %s
  - inbox.lua       Lua the agent wants REAPER to run
  - runner.id       the runner's Web Remote command ID (written by the runner)
  - last_status.txt  outcome of the last run ("ok" / "error: …")`,
		dest, m.RunnerIDPath(), m.OriDir()), nil
}

// ReadRunnerID returns the runner's Web Remote command ID, or an error with
// guidance if the runner has not been triggered yet.
func (m *Manager) ReadRunnerID() (string, error) {
	data, err := os.ReadFile(m.RunnerIDPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("runner not initialized — run `reaper-plugin install-runner`, restart REAPER, then trigger ori-reaper-runner once (it writes %s)", m.RunnerIDPath())
		}
		return "", fmt.Errorf("read runner id: %w", err)
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", fmt.Errorf("runner id file is empty: %s", m.RunnerIDPath())
	}
	return id, nil
}

// Exec writes content to the inbox and triggers the runner over Web Remote so
// REAPER runs it live. It best-effort waits for the runner to record an outcome.
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
	if err := os.WriteFile(m.InboxPath(), []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write inbox: %w", err)
	}

	port := m.ResolveWebRemotePort()
	url := fmt.Sprintf("http://127.0.0.1:%d/_/%s", port, id)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url) //nolint:noctx // short fixed-timeout localhost call
	if err != nil {
		return "", fmt.Errorf("trigger runner over Web Remote at %s: %w (is REAPER running with the Web Remote interface enabled?)", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Web Remote returned status %d", resp.StatusCode)
	}

	if status := m.waitStatus(2 * time.Second); status != "" {
		if strings.HasPrefix(status, "error:") {
			return "", fmt.Errorf("runner reported %s", status)
		}
		return fmt.Sprintf("ran inbox via runner %s — status: %s", id, status), nil
	}
	return fmt.Sprintf("triggered runner %s (no status reported within timeout)", id), nil
}

// waitStatus polls the status file the runner writes, returning its contents or
// "" if nothing appears within the deadline.
func (m *Manager) waitStatus(d time.Duration) string {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(m.statusPath()); err == nil {
			if s := strings.TrimSpace(string(data)); s != "" {
				return s
			}
		}
		time.Sleep(75 * time.Millisecond)
	}
	return ""
}
