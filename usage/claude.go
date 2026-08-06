package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/up2jj/ajaj/account"
)

type claudeStatus struct {
	RateLimits struct {
		FiveHour *claudeLimit `json:"five_hour"`
		SevenDay *claudeLimit `json:"seven_day"`
	} `json:"rate_limits"`
}

type claudeLimit struct {
	UsedPercent float64 `json:"used_percentage"`
	ResetsAt    int64   `json:"resets_at"`
}

func (m *Manager) RecordClaude(a account.Account, input io.Reader) (Snapshot, error) {
	if a.Provider != account.Claude {
		return Snapshot{}, errors.New("Claude usage input requires a Claude account")
	}
	var status claudeStatus
	decoder := json.NewDecoder(io.LimitReader(input, 2*1024*1024))
	if err := decoder.Decode(&status); err != nil {
		return Snapshot{}, fmt.Errorf("decoding Claude status line input: %w", err)
	}
	snapshot := Snapshot{Provider: a.Provider, Account: a.Name, Source: "claude-status-line", UpdatedAt: m.now()}
	for _, item := range []struct {
		name  string
		limit *claudeLimit
	}{{"5h", status.RateLimits.FiveHour}, {"7d", status.RateLimits.SevenDay}} {
		name, limit := item.name, item.limit
		if limit == nil {
			continue
		}
		snapshot.Windows = append(snapshot.Windows, Window{Name: name, UsedPercent: limit.UsedPercent, ResetsAt: limit.ResetsAt})
	}
	if len(snapshot.Windows) > 0 {
		if err := m.store.Write(a, snapshot); err != nil {
			return Snapshot{}, err
		}
	}
	return snapshot, nil
}

func (m *Manager) EnsureClaudeCollector(a account.Account) (bool, error) {
	if a.Provider != account.Claude {
		return false, nil
	}
	path := filepath.Join(a.Home, "settings.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data = []byte("{}")
	} else if err != nil {
		return false, fmt.Errorf("reading Claude settings: %w", err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, fmt.Errorf("decoding Claude settings: %w", err)
	}
	if _, exists := settings["statusLine"]; exists {
		return false, nil
	}
	collector, err := json.Marshal(map[string]any{
		"type":    "command",
		"command": fmt.Sprintf("ajaj usage ingest claude %s", a.Name),
		"padding": 0,
	})
	if err != nil {
		return false, err
	}
	settings["statusLine"] = collector
	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encoding Claude settings: %w", err)
	}
	updated = append(updated, '\n')
	if err := atomicWrite(path, updated, 0o600); err != nil {
		return false, fmt.Errorf("writing Claude settings: %w", err)
	}
	return true, nil
}

func Format(snapshot Snapshot, now time.Time) string {
	if len(snapshot.Windows) == 0 {
		return "usage unavailable"
	}
	text := ""
	for _, window := range snapshot.Windows {
		if text != "" {
			text += " · "
		}
		used := window.UsedPercent
		if window.ResetsAt > 0 && !now.Before(time.Unix(window.ResetsAt, 0)) {
			used = 0
		}
		text += fmt.Sprintf("%s %.0f%%", window.Name, used)
	}
	return text
}

func FormatStatusLine(a account.Account, snapshot Snapshot, now time.Time) string {
	return fmt.Sprintf("[ajaj %s] %s", a.ID(), Format(snapshot, now))
}
