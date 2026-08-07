package account

import (
	"encoding/json"
	"errors"
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

	if err := store.SetDefault(Codex, "work"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetLastSelected(Codex, "work"); err != nil {
		t.Fatal(err)
	}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	defaultAccount, ok := registry.DefaultAccount(Codex)
	if !ok || defaultAccount.Name != "work" {
		t.Fatalf("DefaultAccount(Codex) = %#v, %v", defaultAccount, ok)
	}
	if !registry.Selection.Auto || registry.Selection.SwitchAt != 90 {
		t.Fatalf("default selection policy = %#v", registry.Selection)
	}
	if registry.LastSelected[Codex] != "work" {
		t.Fatalf("last selected Codex account = %q", registry.LastSelected[Codex])
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

func TestDeleteMovesProfileToTrashAndClearsSelection(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "accounts.json"), filepath.Join(root, "profiles"))
	personal, err := store.Add(Claude, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(Claude, "work"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetLastSelected(Claude, personal.Name); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personal.Home, "credentials"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	deleted, trashPath, err := store.Delete(Claude, personal.Name)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != personal {
		t.Fatalf("deleted = %#v, want %#v", deleted, personal)
	}
	if _, err := os.Stat(personal.Home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("profile home still exists: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(trashPath, "credentials"))
	if err != nil || string(data) != "secret" {
		t.Fatalf("trashed credentials = %q, %v", data, err)
	}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Find(Claude, personal.Name); ok {
		t.Fatal("deleted account remains in registry")
	}
	if registry.Default[Claude] != "" || registry.LastSelected[Claude] != "" {
		t.Fatalf("default/last-selected = %q/%q, want cleared", registry.Default[Claude], registry.LastSelected[Claude])
	}
}

func TestDeleteRejectsProfileOutsideManagedLocation(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	store := NewStore(filepath.Join(root, "accounts.json"), filepath.Join(root, "profiles"))
	registry := Registry{
		Accounts:  []Account{{Provider: Claude, Name: "work", Home: outside}},
		Default:   map[Provider]string{Claude: "work"},
		Selection: DefaultSelectionPolicy(),
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Delete(Claude, "work"); err == nil {
		t.Fatal("Delete() accepted a profile outside its managed location")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside profile was changed: %v", err)
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

func TestRegistryUsesDefaultJSONKey(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "accounts.json")
	store := NewStore(path, filepath.Join(root, "profiles"))
	if _, err := store.Add(Claude, "work"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var registry map[string]json.RawMessage
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry["default"]; !ok {
		t.Fatalf("registry = %s, want default key", data)
	}
	if _, ok := registry["active"]; ok {
		t.Fatalf("registry = %s, does not want active key", data)
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
