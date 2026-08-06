package usage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/up2jj/ajaj/account"
)

const snapshotMaxAge = 15 * time.Minute

type Manager struct {
	store   *Store
	env     []string
	now     func() time.Time
	timeout time.Duration
}

func NewManager(store *Store, env []string) *Manager {
	return &Manager{store: store, env: env, now: time.Now, timeout: 10 * time.Second}
}

type Selection struct {
	Account   account.Account
	Preferred account.Account
	Switched  bool
	Reason    string
	Warning   error
}

func (m *Manager) Snapshot(a account.Account) (Snapshot, bool, error) {
	return m.store.Read(a)
}

func (m *Manager) Refresh(ctx context.Context, a account.Account) (Snapshot, error) {
	if a.Provider != account.Codex {
		return Snapshot{}, fmt.Errorf("live refresh is not available for %s", a.Provider)
	}
	snapshot, err := m.refreshCodex(ctx, a)
	if err != nil {
		return Snapshot{}, err
	}
	if err := m.store.Write(a, snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) Select(ctx context.Context, registry account.Registry, provider account.Provider) (Selection, error) {
	preferred, ok := registry.ActiveAccount(provider)
	if !ok {
		return Selection{}, fmt.Errorf("no preferred %s profile", provider)
	}
	result := Selection{Account: preferred, Preferred: preferred}
	if !registry.Selection.Auto {
		return result, nil
	}

	preferredSnapshot, known, warning := m.currentSnapshot(ctx, preferred, true)
	result.Warning = warning
	used, hasUsage := preferredSnapshot.Used(m.now())
	if !known || !hasUsage || !preferredSnapshot.Fresh(m.now(), snapshotMaxAge) || used < registry.Selection.SwitchAt {
		return result, nil
	}

	var best account.Account
	bestUsage := 101.0
	for _, candidate := range registry.Accounts {
		if candidate.Provider != provider || candidate.Name == preferred.Name {
			continue
		}
		snapshot, candidateKnown, refreshErr := m.currentSnapshot(ctx, candidate, true)
		result.Warning = errors.Join(result.Warning, refreshErr)
		candidateUsage, candidateHasUsage := snapshot.Used(m.now())
		if !candidateKnown || !candidateHasUsage {
			continue
		}
		if !snapshot.Fresh(m.now(), snapshotMaxAge) || candidateUsage >= registry.Selection.SwitchAt {
			continue
		}
		if candidateUsage < bestUsage {
			best = candidate
			bestUsage = candidateUsage
		}
	}

	if best.Name != "" {
		result.Account = best
		result.Switched = true
		result.Reason = fmt.Sprintf("%s is at %.0f%%; %s is at %.0f%%", preferred.ID(), used, best.ID(), bestUsage)
		return result, nil
	}
	return result, nil
}

func (m *Manager) currentSnapshot(ctx context.Context, a account.Account, refresh bool) (Snapshot, bool, error) {
	if refresh && a.Provider == account.Codex {
		snapshot, err := m.Refresh(ctx, a)
		if err == nil {
			return snapshot, true, nil
		}
		cached, ok, cacheErr := m.store.Read(a)
		return cached, ok, errors.Join(err, cacheErr)
	}
	snapshot, ok, err := m.store.Read(a)
	return snapshot, ok, err
}

func DefaultManager() (*Manager, error) {
	root, err := DefaultRoot()
	if err != nil {
		return nil, err
	}
	return NewManager(NewStore(root), os.Environ()), nil
}
