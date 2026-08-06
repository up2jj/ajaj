// Package usage records provider quota snapshots and selects an account before launch.
package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/up2jj/ajaj/account"
)

type Window struct {
	Name        string  `json:"name"`
	UsedPercent float64 `json:"used_percent"`
	ResetsAt    int64   `json:"resets_at,omitempty"`
}

type Snapshot struct {
	Provider  account.Provider `json:"provider"`
	Account   string           `json:"account"`
	Windows   []Window         `json:"windows,omitempty"`
	Reached   bool             `json:"reached,omitempty"`
	Source    string           `json:"source"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// Used returns the highest usage across windows that have not reset.
func (s Snapshot) Used(now time.Time) (float64, bool) {
	if s.Reached {
		return 100, true
	}
	if len(s.Windows) == 0 {
		return 0, false
	}
	var highest float64
	for _, window := range s.Windows {
		if window.ResetsAt > 0 && !now.Before(time.Unix(window.ResetsAt, 0)) {
			continue
		}
		highest = max(highest, window.UsedPercent)
	}
	return highest, true
}

func (s Snapshot) Fresh(now time.Time, maxAge time.Duration) bool {
	if s.UpdatedAt.IsZero() {
		return false
	}
	if now.Sub(s.UpdatedAt) <= maxAge {
		return true
	}
	for _, window := range s.Windows {
		if window.ResetsAt == 0 || now.Before(time.Unix(window.ResetsAt, 0)) {
			return false
		}
	}
	return true
}

type Store struct {
	root string
}

func NewStore(root string) *Store { return &Store{root: root} }

func DefaultRoot() (string, error) {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locating user home directory: %w", err)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "ajaj", "usage"), nil
}

func (s *Store) Read(a account.Account) (Snapshot, bool, error) {
	data, err := os.ReadFile(s.path(a))
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("reading usage for %s: %w", a.ID(), err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, false, fmt.Errorf("decoding usage for %s: %w", a.ID(), err)
	}
	return snapshot, true, nil
}

func (s *Store) Write(a account.Account, snapshot Snapshot) error {
	dir := filepath.Dir(s.path(a))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating usage directory: %w", err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding usage for %s: %w", a.ID(), err)
	}
	data = append(data, '\n')
	return atomicWrite(s.path(a), data, 0o600)
}

func (s *Store) path(a account.Account) string {
	return filepath.Join(s.root, string(a.Provider), a.Name+".json")
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ajaj-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
