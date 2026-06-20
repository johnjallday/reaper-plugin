package reaper

import "fmt"

func (m *Manager) ListAvailableScripts() string {
	return fmt.Sprintf("Marketplace browsing is available at: %s\nDirect marketplace indexing is not yet implemented in reaper-plugin.", m.MarketplaceURL)
}

func (m *Manager) DownloadScriptHint() string {
	return fmt.Sprintf("Browse and download scripts at: %s", m.MarketplaceURL)
}
