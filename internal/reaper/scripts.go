package reaper

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ScriptSummary struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
}

func (m *Manager) ListScripts() ([]ScriptSummary, error) {
	entries, err := os.ReadDir(expandHome(m.ScriptsDir))
	if err != nil {
		return nil, fmt.Errorf("list scripts dir: %w", err)
	}

	result := make([]ScriptSummary, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".lua" && ext != ".eel" && ext != ".py" {
			continue
		}
		result = append(result, ScriptSummary{
			Name: strings.TrimSuffix(name, ext),
			Type: strings.TrimPrefix(ext, "."),
			Path: filepath.Join(expandHome(m.ScriptsDir), name),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].Type < result[j].Type
		}
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (m *Manager) AddScript(scriptName, content, scriptType string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("script content is required")
	}
	ext, err := normalizeScriptType(scriptType)
	if err != nil {
		return "", err
	}

	name, err := normalizeScriptName(scriptName)
	if err != nil {
		return "", err
	}
	name = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(name, ".lua"), ".eel"), ".py")

	p, err := scriptPath(m.ScriptsDir, name, ext)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}
	if _, err := os.Stat(p); err == nil {
		return "", fmt.Errorf("script already exists: %s", filepath.Base(p))
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write script: %w", err)
	}
	return fmt.Sprintf("Successfully added REAPER script: %s", filepath.Base(p)), nil
}

func (m *Manager) DeleteScript(script string) (string, error) {
	p, err := resolveExistingScriptPath(m.ScriptsDir, script)
	if err != nil {
		return "", err
	}
	if err := os.Remove(p); err != nil {
		return "", fmt.Errorf("delete script: %w", err)
	}
	return fmt.Sprintf("Successfully deleted REAPER script: %s", filepath.Base(p)), nil
}

func (m *Manager) RunScript(script string) (string, error) {
	running, err := isReaperRunning()
	if err != nil {
		return "", fmt.Errorf("check REAPER process: %w", err)
	}
	if !running {
		return "REAPER is not running. Please start REAPER first, then try again.", nil
	}

	p, err := resolveExistingScriptPath(m.ScriptsDir, script)
	if err != nil {
		return "", err
	}
	if err := launchScript(p); err != nil {
		return "", fmt.Errorf("launch script: %w", err)
	}
	return fmt.Sprintf("Successfully launched REAPER script: %s", filepath.Base(p)), nil
}

func (m *Manager) RegisterScript(script string) (string, error) {
	scriptFilePath, err := resolveExistingScriptPath(m.ScriptsDir, script)
	if err != nil {
		return "", err
	}
	kbPath, err := reaperKBIniPath()
	if err != nil {
		return "", err
	}

	file, err := os.Open(kbPath)
	if err != nil {
		return "", fmt.Errorf("open reaper-kb.ini: %w", err)
	}
	defer func() { _ = file.Close() }()

	lines := make([]string, 0)
	scanner := bufio.NewScanner(file)
	alreadyRegistered := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, scriptFilePath) {
			alreadyRegistered = true
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read reaper-kb.ini: %w", err)
	}

	if alreadyRegistered {
		return fmt.Sprintf("Script '%s' is already registered in REAPER", filepath.Base(scriptFilePath)), nil
	}

	scriptDisplay := strings.TrimSuffix(filepath.Base(scriptFilePath), filepath.Ext(scriptFilePath))
	entry := fmt.Sprintf(`SCR 4 0 "Script: %s" "%s"`, scriptDisplay, scriptFilePath)

	inserted := false
	for i, line := range lines {
		if strings.HasPrefix(line, "[Main]") {
			lines = append(lines[:i+1], append([]string{entry}, lines[i+1:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		lines = append(lines, "", "[Main]", entry)
	}

	if err := os.WriteFile(kbPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", fmt.Errorf("write reaper-kb.ini: %w", err)
	}
	return fmt.Sprintf("Successfully registered script '%s' in REAPER keyboard shortcuts", scriptDisplay), nil
}

func (m *Manager) RegisterAllScripts() (string, error) {
	scripts, err := m.ListScripts()
	if err != nil {
		return "", err
	}
	if len(scripts) == 0 {
		return "No scripts found to register", nil
	}

	registered := 0
	already := 0
	failed := 0
	for _, s := range scripts {
		msg, err := m.RegisterScript(s.Name + "." + s.Type)
		if err != nil {
			failed++
			continue
		}
		if strings.Contains(strings.ToLower(msg), "already registered") {
			already++
		} else {
			registered++
		}
	}

	summary := fmt.Sprintf("Registration complete: %d newly registered, %d already registered", registered, already)
	if failed > 0 {
		summary += fmt.Sprintf(", %d failed", failed)
	}
	return summary, nil
}

func (m *Manager) CleanScripts() (string, error) {
	kbPath, err := reaperKBIniPath()
	if err != nil {
		return "", err
	}

	file, err := os.Open(kbPath)
	if err != nil {
		return "", fmt.Errorf("open reaper-kb.ini: %w", err)
	}
	defer func() { _ = file.Close() }()

	lines := make([]string, 0)
	removed := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "SCR ") {
			parts := strings.Split(line, "\"")
			if len(parts) >= 4 {
				scriptPath := parts[3]
				if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
					removed++
					continue
				}
			}
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read reaper-kb.ini: %w", err)
	}

	if removed == 0 {
		return "No missing scripts found in reaper-kb.ini. All script paths are valid.", nil
	}

	if err := os.WriteFile(kbPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", fmt.Errorf("write reaper-kb.ini: %w", err)
	}
	return fmt.Sprintf("Cleaned %d missing script(s) from reaper-kb.ini", removed), nil
}
