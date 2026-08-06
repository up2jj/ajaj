package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/up2jj/ajaj/account"
)

func TestRecordClaude(t *testing.T) {
	now := time.Unix(10_000, 0)
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil)
	manager.now = func() time.Time { return now }
	a := account.Account{Provider: account.Claude, Name: "work"}
	input := `{"rate_limits":{"five_hour":{"used_percentage":73,"resets_at":20000},"seven_day":{"used_percentage":41,"resets_at":30000}}}`

	snapshot, err := manager.RecordClaude(a, strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if got := Format(snapshot, now); got != "5h 73% · 7d 41%" && got != "7d 41% · 5h 73%" {
		t.Fatalf("Format() = %q", got)
	}
	if _, ok, err := store.Read(a); err != nil || !ok {
		t.Fatalf("stored snapshot = %v, %v", ok, err)
	}
}

func TestEnsureClaudeCollectorPreservesSettings(t *testing.T) {
	home := t.TempDir()
	a := account.Account{Provider: account.Claude, Name: "work", Home: home}
	manager := NewManager(NewStore(t.TempDir()), nil)
	if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	installed, err := manager.EnsureClaudeCollector(a)
	if err != nil || !installed {
		t.Fatalf("EnsureClaudeCollector() = %v, %v", installed, err)
	}
	data, err := os.ReadFile(filepath.Join(home, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"theme": "dark"`) || !strings.Contains(text, "ajaj usage ingest claude work") {
		t.Fatalf("settings = %s", text)
	}

	installed, err = manager.EnsureClaudeCollector(a)
	if err != nil || installed {
		t.Fatalf("second EnsureClaudeCollector() = %v, %v", installed, err)
	}
}
