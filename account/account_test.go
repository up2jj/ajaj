package account

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreAccountLifecycle(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "config", "accounts.json"), filepath.Join(root, "profiles"))

	claude, err := store.Add(Claude, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "profiles", "claude", "personal"); claude.Home != want {
		t.Fatalf("Home = %q, want %q", claude.Home, want)
	}

	codex, err := store.Add(Codex, "work")
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(codex.Home, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `cli_auth_credentials_store = "file"`) {
		t.Fatalf("Codex config does not select file credential storage: %s", config)
	}

	if err := store.SetActive(Codex, "work"); err != nil {
		t.Fatal(err)
	}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	active, ok := registry.ActiveAccount(Codex)
	if !ok || active.Name != "work" {
		t.Fatalf("ActiveAccount(Codex) = %#v, %v", active, ok)
	}
	if !registry.Selection.Auto || registry.Selection.SwitchAt != 90 {
		t.Fatalf("default selection policy = %#v", registry.Selection)
	}
}

func TestStoreRejectsInvalidAndDuplicateAccounts(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "accounts.json"), filepath.Join(root, "profiles"))

	if _, err := store.Add(Provider("other"), "work"); err == nil {
		t.Fatal("Add() accepted unsupported provider")
	}
	if _, err := store.Add(Claude, "../escape"); err == nil {
		t.Fatal("Add() accepted unsafe account name")
	}
	if _, err := store.Add(Claude, "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(Claude, "work"); err == nil {
		t.Fatal("Add() accepted duplicate account")
	}
}

func TestRegistryFileIsPrivate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", "accounts.json")
	store := NewStore(path, filepath.Join(root, "profiles"))
	if _, err := store.Add(Claude, "personal"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("registry permissions = %o, want 600", got)
	}
}

func TestDefaultPathsPreferXDGDirectories(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	dataHome := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	registryPath, accountsDir, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(configHome, "ajaj", "accounts.json"); registryPath != want {
		t.Fatalf("registryPath = %q, want %q", registryPath, want)
	}
	if want := filepath.Join(dataHome, "ajaj", "profiles"); accountsDir != want {
		t.Fatalf("accountsDir = %q, want %q", accountsDir, want)
	}
}

func TestStoreSelectionPolicy(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "accounts.json"), filepath.Join(root, "profiles"))
	policy := SelectionPolicy{Auto: false, SwitchAt: 75}
	if err := store.SetSelection(policy); err != nil {
		t.Fatal(err)
	}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if registry.Selection != policy {
		t.Fatalf("selection policy = %#v, want %#v", registry.Selection, policy)
	}
	if err := store.SetSelection(SelectionPolicy{Auto: true, SwitchAt: 101}); err == nil {
		t.Fatal("SetSelection() accepted threshold over 100")
	}
}
