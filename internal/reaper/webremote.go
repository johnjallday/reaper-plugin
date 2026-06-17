package reaper

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Track struct {
	Index  int     `json:"index"`
	Name   string  `json:"name"`
	Volume float64 `json:"volume_db"`
	Pan    float64 `json:"pan"`
	Mute   bool    `json:"mute"`
	Solo   bool    `json:"solo"`
	RecArm bool    `json:"rec_arm"`
}

func (m *Manager) ResolveWebRemotePort() int {
	if m.WebRemotePort > 0 {
		return m.WebRemotePort
	}
	if cfg, err := getWebRemoteConfig(); err == nil && cfg.Port > 0 {
		return cfg.Port
	}
	return 2307
}

func (m *Manager) GetWebRemotePortMessage() string {
	port := m.ResolveWebRemotePort()
	return fmt.Sprintf("REAPER Web Remote:\n  Configured Port: %d\n  URL: http://localhost:%d", port, port)
}

type webRemoteConfig struct {
	Port    int
	Enabled bool
}

func getWebRemoteConfig() (*webRemoteConfig, error) {
	ini, err := reaperIniPath()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(ini)
	if err != nil {
		return nil, fmt.Errorf("open reaper.ini: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "csurf_") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value := parts[1]
		if strings.HasPrefix(value, "HTTP ") {
			fields := strings.Fields(value)
			if len(fields) >= 3 {
				enabled := fields[1] == "1" || strings.EqualFold(fields[1], "true")
				port, err := strconv.Atoi(fields[2])
				if err == nil {
					return &webRemoteConfig{Port: port, Enabled: enabled}, nil
				}
			}
		}
		if strings.HasPrefix(value, "WEBR ") {
			fields := strings.Fields(value)
			if len(fields) >= 2 {
				port, err := strconv.Atoi(fields[len(fields)-1])
				if err == nil {
					enabled := fields[1] == "1" || strings.EqualFold(fields[1], "true")
					return &webRemoteConfig{Port: port, Enabled: enabled}, nil
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan reaper.ini: %w", err)
	}
	return nil, fmt.Errorf("web remote config not found in reaper.ini")
}

func (m *Manager) GetTracks() ([]Track, error) {
	port := m.ResolveWebRemotePort()
	url := fmt.Sprintf("http://localhost:%d/_/TRACK", port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("connect to REAPER web remote at %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("REAPER web remote returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read web remote response: %w", err)
	}
	return parseTracks(string(body)), nil
}

func parseTracks(data string) []Track {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	tracks := make([]Track, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 13 || fields[0] != "TRACK" {
			continue
		}

		track := Track{}
		if idx, err := strconv.Atoi(fields[1]); err == nil {
			track.Index = idx
		}
		track.Name = fields[2]

		if volMult, err := strconv.ParseFloat(fields[4], 64); err == nil {
			if volMult > 0 {
				track.Volume = 20 * math.Log10(volMult)
			} else {
				track.Volume = -150
			}
		}
		if pan, err := strconv.ParseFloat(fields[5], 64); err == nil {
			track.Pan = pan
		}
		track.Mute = fields[10] == "1"
		track.Solo = fields[11] == "1" || fields[11] == "2"
		track.RecArm = fields[12] == "1"

		tracks = append(tracks, track)
	}
	return tracks
}

func FormatTracksTable(tracks []Track) string {
	if len(tracks) == 0 {
		return "No tracks found in REAPER project"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d tracks:\n\n", len(tracks)))
	b.WriteString("Index | Name                    | Volume  | Pan    | M | S | R\n")
	b.WriteString("------|-------------------------|---------|--------|---|---|---\n")

	for _, t := range tracks {
		m, s, r := " ", " ", " "
		if t.Mute {
			m = "M"
		}
		if t.Solo {
			s = "S"
		}
		if t.RecArm {
			r = "R"
		}

		pan := "Center"
		if t.Pan < -0.01 {
			pan = fmt.Sprintf("L%.0f%%", -t.Pan*100)
		} else if t.Pan > 0.01 {
			pan = fmt.Sprintf("R%.0f%%", t.Pan*100)
		}

		name := t.Name
		if len(name) > 23 {
			name = name[:20] + "..."
		}
		b.WriteString(fmt.Sprintf("%-5d | %-23s | %6.1fdB | %-6s | %s | %s | %s\n", t.Index, name, t.Volume, pan, m, s, r))
	}

	b.WriteString("\nLegend: M=Muted, S=Solo, R=Record Armed")
	return b.String()
}
