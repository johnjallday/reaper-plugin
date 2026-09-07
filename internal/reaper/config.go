package reaper

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	envScriptsDir    = "REAPER_SCRIPTS_DIR"
	envWebRemotePort = "REAPER_WEB_REMOTE_PORT"
	envMarketplace   = "REAPER_MARKETPLACE_URL"
	envServiceHome   = "REAPER_PLUGIN_HOME"
)

type Manager struct {
	ScriptsDir     string
	WebRemotePort  int
	MarketplaceURL string
}

// ApplyServiceHomeOverride lets an operator/demo launch the private service
// against a real user's REAPER support directory while Ori itself runs under a
// disposable HOME. The path is process environment, never manifest/browser/
// workspace/service-call input.
func ApplyServiceHomeOverride() error {
	home := strings.TrimSpace(os.Getenv(envServiceHome))
	if home == "" {
		return nil
	}
	absolute, err := filepath.Abs(home)
	if err != nil || !filepath.IsAbs(absolute) {
		return fmt.Errorf("REAPER service home is invalid")
	}
	info, err := os.Lstat(absolute) // #nosec G304 G703 -- explicit operator environment, checked before process use
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("REAPER service home is unsafe")
	}
	return os.Setenv("HOME", filepath.Clean(absolute))
}

func NewManagerFromEnv() *Manager {
	m := &Manager{
		ScriptsDir:     defaultScriptsDir(),
		WebRemotePort:  0,
		MarketplaceURL: "https://gitlab.com/johnjallday/jj-reascript",
	}

	if v := strings.TrimSpace(os.Getenv(envScriptsDir)); v != "" {
		m.ScriptsDir = expandHome(v)
	}
	if v := strings.TrimSpace(os.Getenv(envWebRemotePort)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 65535 {
			m.WebRemotePort = n
		}
	}
	if v := strings.TrimSpace(os.Getenv(envMarketplace)); v != "" {
		m.MarketplaceURL = v
	}
	return m
}

func defaultScriptsDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "REAPER", "Scripts")
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "REAPER", "Scripts")
		}
		return filepath.Join(home, "AppData", "Roaming", "REAPER", "Scripts")
	default:
		return filepath.Join(home, ".config", "REAPER", "Scripts")
	}
}

func reaperConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "REAPER"), nil
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "REAPER"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "REAPER"), nil
	default:
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg != "" {
			return filepath.Join(xdg, "REAPER"), nil
		}
		return filepath.Join(home, ".config", "REAPER"), nil
	}
}

func reaperIniPath() (string, error) {
	dir, err := reaperConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reaper.ini"), nil
}

func reaperKBIniPath() (string, error) {
	dir, err := reaperConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reaper-kb.ini"), nil
}

func expandHome(p string) string {
	if p == "~" {
		h, _ := os.UserHomeDir()
		return h
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		h, _ := os.UserHomeDir()
		return filepath.Join(h, p[2:])
	}
	return p
}
