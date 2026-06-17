package reaper

import (
	"encoding/json"
	"fmt"
)

type Params struct {
	Operation  string `json:"operation"`
	Script     string `json:"script,omitempty"`
	Filename   string `json:"filename,omitempty"`
	Content    string `json:"content,omitempty"`
	ScriptType string `json:"script_type,omitempty"`
}

func (m *Manager) Execute(params Params) (string, error) {
	switch params.Operation {
	case "list":
		scripts, err := m.ListScripts()
		if err != nil {
			return "", err
		}
		if len(scripts) == 0 {
			return fmt.Sprintf("No ReaScripts found in %s", m.ScriptsDir), nil
		}
		data, _ := json.MarshalIndent(scripts, "", "  ")
		return string(data), nil

	case "run":
		return m.RunScript(params.Script)

	case "add":
		return m.AddScript(params.Script, params.Content, params.ScriptType)

	case "delete":
		return m.DeleteScript(params.Script)

	case "register_script":
		return m.RegisterScript(params.Script)

	case "register_all_scripts":
		return m.RegisterAllScripts()

	case "clean_scripts":
		return m.CleanScripts()

	case "get_context":
		ctx, err := m.GetContext()
		if err != nil {
			return "", err
		}
		data, _ := json.MarshalIndent(ctx, "", "  ")
		return string(data), nil

	case "get_status":
		status := m.GetStatus()
		data, _ := json.MarshalIndent(status, "", "  ")
		return string(data), nil

	case "get_web_remote_port":
		return m.GetWebRemotePortMessage(), nil

	case "get_tracks":
		tracks, err := m.GetTracks()
		if err != nil {
			return "", err
		}
		return FormatTracksTable(tracks), nil

	case "list_available_scripts":
		return m.ListAvailableScripts(), nil

	case "download_script":
		return m.DownloadScriptHint(), nil

	default:
		return "", fmt.Errorf("unsupported operation: %s", params.Operation)
	}
}
