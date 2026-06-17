package reaper

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type ContextInfo struct {
	IsRunning   bool      `json:"is_running"`
	ProjectName string    `json:"project_name,omitempty"`
	ProjectPath string    `json:"project_path,omitempty"`
	LastChecked time.Time `json:"last_checked"`
}

func (m *Manager) GetContext() (*ContextInfo, error) {
	ctx := &ContextInfo{LastChecked: time.Now()}

	running, err := isReaperRunning()
	if err != nil {
		return nil, fmt.Errorf("check REAPER process: %w", err)
	}
	ctx.IsRunning = running
	if !running {
		return ctx, nil
	}

	name, path, err := getProjectInfo()
	if err == nil {
		ctx.ProjectName = name
		ctx.ProjectPath = path
	}
	return ctx, nil
}

func getProjectInfo() (string, string, error) {
	tmp := os.TempDir()
	scriptPath := filepath.Join(tmp, "reaper_mcp_get_context.lua")
	outputPath := filepath.Join(tmp, "reaper_mcp_context_output.txt")

	escapedOutput := strings.ReplaceAll(outputPath, "\\", "\\\\")
	lua := fmt.Sprintf(`local retval, project_full_path = reaper.EnumProjects(-1, "")
local project_name = "untitled"
local project_path = ""

if project_full_path and project_full_path ~= "" then
  project_name = project_full_path:match("([^/\\]+)$") or "untitled"
  project_path = project_full_path:match("^(.+)[/\\]") or ""
end

local file = io.open("%s", "w")
if file then
  file:write(project_name .. "\n")
  file:write(project_path .. "\n")
  file:close()
end
`, escapedOutput)

	if err := os.WriteFile(scriptPath, []byte(lua), 0o644); err != nil {
		return "", "", fmt.Errorf("write temp script: %w", err)
	}
	defer func() { _ = os.Remove(scriptPath) }()
	_ = os.Remove(outputPath)

	if err := executeScriptInReaper(scriptPath); err != nil {
		return "", "", fmt.Errorf("execute context script: %w", err)
	}

	time.Sleep(1 * time.Second)
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return "", "", fmt.Errorf("read context output: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return "", "", fmt.Errorf("context output empty")
	}
	name := strings.TrimSpace(lines[0])
	path := ""
	if len(lines) > 1 {
		path = strings.TrimSpace(lines[1])
	}
	if name == "" || name == "untitled" {
		return "No project open", "", nil
	}
	return name, path, nil
}

func executeScriptInReaper(scriptPath string) error {
	switch runtime.GOOS {
	case "darwin", "windows", "linux":
		return launchScript(scriptPath)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
