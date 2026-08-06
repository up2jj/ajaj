package usage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/up2jj/ajaj/account"
)

func TestSnapshotUsedIgnoresResetWindows(t *testing.T) {
	now := time.Unix(2_000, 0)
	snapshot := Snapshot{Windows: []Window{
		{Name: "5h", UsedPercent: 95, ResetsAt: now.Add(-time.Second).Unix()},
		{Name: "7d", UsedPercent: 42, ResetsAt: now.Add(time.Hour).Unix()},
	}}

	got, ok := snapshot.Used(now)
	if !ok || got != 42 {
		t.Fatalf("Used() = %v, %v; want 42, true", got, ok)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	a := account.Account{Provider: account.Claude, Name: "work"}
	want := Snapshot{Provider: account.Claude, Account: "work", Source: "test", UpdatedAt: time.Now(), Windows: []Window{{Name: "5h", UsedPercent: 21}}}
	if err := store.Write(a, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Read(a)
	if err != nil || !ok {
		t.Fatalf("Read() = %#v, %v, %v", got, ok, err)
	}
	if got.Windows[0].UsedPercent != 21 || got.Source != "test" {
		t.Fatalf("Read() = %#v", got)
	}
}

func TestDefaultRootUsesXDGStateHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	got, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "ajaj", "usage")
	if got != want {
		t.Fatalf("DefaultRoot() = %q; want %q", got, want)
	}
}
