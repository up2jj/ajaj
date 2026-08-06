package usage

import (
	"context"
	"testing"
	"time"

	"github.com/up2jj/ajaj/account"
)

func TestSelectSwitchesToLowestFreshUsage(t *testing.T) {
	now := time.Unix(10_000, 0)
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil)
	manager.now = func() time.Time { return now }
	work := account.Account{Provider: account.Claude, Name: "work"}
	personal := account.Account{Provider: account.Claude, Name: "personal"}
	backup := account.Account{Provider: account.Claude, Name: "backup"}
	writeUsage(t, store, work, now, 94)
	writeUsage(t, store, personal, now, 35)
	writeUsage(t, store, backup, now, 12)
	registry := account.Registry{
		Accounts:  []account.Account{work, personal, backup},
		Active:    map[account.Provider]string{account.Claude: "work"},
		Selection: account.SelectionPolicy{Auto: true, SwitchAt: 90},
	}

	selection, err := manager.Select(context.Background(), registry, account.Claude)
	if err != nil {
		t.Fatal(err)
	}
	if !selection.Switched || selection.Account.Name != "backup" {
		t.Fatalf("Select() = %#v; want backup", selection)
	}
}

func TestSelectKeepsPreferredWhenDisabledBelowThresholdOrStale(t *testing.T) {
	now := time.Unix(10_000, 0)
	for _, test := range []struct {
		name    string
		auto    bool
		used    float64
		updated time.Time
	}{
		{name: "disabled", auto: false, used: 99, updated: now},
		{name: "below threshold", auto: true, used: 89, updated: now},
		{name: "stale", auto: true, used: 99, updated: now.Add(-time.Hour)},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			manager := NewManager(store, nil)
			manager.now = func() time.Time { return now }
			preferred := account.Account{Provider: account.Claude, Name: "preferred"}
			alternate := account.Account{Provider: account.Claude, Name: "alternate"}
			writeUsage(t, store, preferred, test.updated, test.used)
			writeUsage(t, store, alternate, now, 1)
			registry := account.Registry{
				Accounts:  []account.Account{preferred, alternate},
				Active:    map[account.Provider]string{account.Claude: preferred.Name},
				Selection: account.SelectionPolicy{Auto: test.auto, SwitchAt: 90},
			}
			selection, err := manager.Select(context.Background(), registry, account.Claude)
			if err != nil {
				t.Fatal(err)
			}
			if selection.Switched || selection.Account.Name != preferred.Name {
				t.Fatalf("Select() = %#v; want preferred", selection)
			}
		})
	}
}

func writeUsage(t *testing.T, store *Store, a account.Account, updated time.Time, used float64) {
	t.Helper()
	err := store.Write(a, Snapshot{
		Provider: a.Provider, Account: a.Name, Source: "test", UpdatedAt: updated,
		Windows: []Window{{Name: "5h", UsedPercent: used, ResetsAt: updated.Add(5 * time.Hour).Unix()}},
	})
	if err != nil {
		t.Fatal(err)
	}
}
