// Package account owns account metadata and its durable registry.
package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type Provider string

const (
	Claude Provider = "claude"
	Codex  Provider = "codex"
)

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Account struct {
	Provider Provider `json:"provider"`
	Name     string   `json:"name"`
	Home     string   `json:"home"`
}

func (a Account) ID() string { return string(a.Provider) + "/" + a.Name }

type Registry struct {
	Accounts     []Account           `json:"accounts"`
	Default      map[Provider]string `json:"default"`
	LastSelected map[Provider]string `json:"last_selected,omitempty"`
	Selection    SelectionPolicy     `json:"selection"`
}

type SelectionPolicy struct {
	Auto     bool    `json:"auto"`
	SwitchAt float64 `json:"switch_at_percent"`
}

func (r Registry) Find(provider Provider, name string) (Account, bool) {
	for _, a := range r.Accounts {
		if a.Provider == provider && a.Name == name {
			return a, true
		}
	}
	return Account{}, false
}

func (r Registry) DefaultAccount(provider Provider) (Account, bool) {
	return r.Find(provider, r.Default[provider])
}

func (r *Registry) Sort() {
	slices.SortFunc(r.Accounts, func(a, b Account) int {
		return strings.Compare(a.ID(), b.ID())
	})
}

type Store struct {
	registryPath string
	accountsDir  string
}

func NewStore(registryPath, accountsDir string) *Store {
	return &Store{registryPath: registryPath, accountsDir: accountsDir}
}

func DefaultPaths() (registryPath, accountsDir string, err error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome, err = os.UserConfigDir()
		if err != nil {
			return "", "", fmt.Errorf("locating user config directory: %w", err)
		}
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", "", fmt.Errorf("locating user home directory: %w", homeErr)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}

	return filepath.Join(configHome, "ajaj", "accounts.json"), filepath.Join(dataHome, "ajaj", "profiles"), nil
}

func (s *Store) Load() (Registry, error) {
	data, err := os.ReadFile(s.registryPath)
	if errors.Is(err, os.ErrNotExist) {
		return Registry{
			Default:      make(map[Provider]string),
			LastSelected: make(map[Provider]string),
			Selection:    DefaultSelectionPolicy(),
		}, nil
	}
	if err != nil {
		return Registry{}, fmt.Errorf("reading account registry: %w", err)
	}

	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return Registry{}, fmt.Errorf("decoding account registry: %w", err)
	}
	if registry.Default == nil {
		registry.Default = make(map[Provider]string)
	}
	if registry.LastSelected == nil {
		registry.LastSelected = make(map[Provider]string)
	}
	if registry.Selection.SwitchAt == 0 {
		registry.Selection = DefaultSelectionPolicy()
	}
	registry.Sort()
	return registry, nil
}

func DefaultSelectionPolicy() SelectionPolicy {
	return SelectionPolicy{Auto: true, SwitchAt: 90}
}

func (s *Store) Add(provider Provider, name string) (Account, error) {
	if !provider.Valid() {
		return Account{}, fmt.Errorf("unsupported provider %q", provider)
	}
	if !validName.MatchString(name) {
		return Account{}, errors.New("account name must start with a letter or digit and contain only letters, digits, dots, dashes, or underscores")
	}

	registry, err := s.Load()
	if err != nil {
		return Account{}, err
	}
	if _, exists := registry.Find(provider, name); exists {
		return Account{}, fmt.Errorf("account %s/%s already exists", provider, name)
	}

	home := filepath.Join(s.accountsDir, string(provider), name)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return Account{}, fmt.Errorf("creating account home: %w", err)
	}
	if err := os.Chmod(home, 0o700); err != nil {
		return Account{}, fmt.Errorf("securing account home: %w", err)
	}
	if provider == Codex {
		if err := initializeCodexHome(home); err != nil {
			return Account{}, err
		}
	}

	a := Account{Provider: provider, Name: name, Home: home}
	registry.Accounts = append(registry.Accounts, a)
	registry.Sort()
	if _, exists := registry.Default[provider]; !exists {
		registry.Default[provider] = name
	}
	if err := s.Save(registry); err != nil {
		return Account{}, err
	}
	return a, nil
}

func (s *Store) SetDefault(provider Provider, name string) error {
	registry, err := s.Load()
	if err != nil {
		return err
	}
	if _, exists := registry.Find(provider, name); !exists {
		return fmt.Errorf("account %s/%s does not exist", provider, name)
	}
	registry.Default[provider] = name
	return s.Save(registry)
}

func (s *Store) Delete(provider Provider, name string) (Account, string, error) {
	if !provider.Valid() {
		return Account{}, "", fmt.Errorf("unsupported provider %q", provider)
	}
	if !validName.MatchString(name) {
		return Account{}, "", errors.New("invalid account name")
	}

	registry, err := s.Load()
	if err != nil {
		return Account{}, "", err
	}
	a, exists := registry.Find(provider, name)
	if !exists {
		return Account{}, "", fmt.Errorf("account %s/%s does not exist", provider, name)
	}
	expectedHome := filepath.Join(s.accountsDir, string(provider), name)
	if filepath.Clean(a.Home) != filepath.Clean(expectedHome) {
		return Account{}, "", fmt.Errorf("refusing to delete %s: profile home is outside its managed location", a.ID())
	}

	trashRoot := filepath.Join(s.accountsDir, ".trash")
	if err := os.MkdirAll(trashRoot, 0o700); err != nil {
		return Account{}, "", fmt.Errorf("creating profile trash: %w", err)
	}
	trashPath, err := os.MkdirTemp(trashRoot, string(provider)+"-"+name+"-")
	if err != nil {
		return Account{}, "", fmt.Errorf("allocating profile trash path: %w", err)
	}
	if err := os.Remove(trashPath); err != nil {
		return Account{}, "", fmt.Errorf("preparing profile trash path: %w", err)
	}
	if err := os.Rename(expectedHome, trashPath); err != nil {
		return Account{}, "", fmt.Errorf("moving %s to profile trash: %w", a.ID(), err)
	}

	for i, candidate := range registry.Accounts {
		if candidate.Provider == provider && candidate.Name == name {
			registry.Accounts = slices.Delete(registry.Accounts, i, i+1)
			break
		}
	}
	if registry.Default[provider] == name {
		delete(registry.Default, provider)
	}
	if registry.LastSelected[provider] == name {
		delete(registry.LastSelected, provider)
	}
	if err := s.Save(registry); err != nil {
		rollbackErr := os.Rename(trashPath, expectedHome)
		return Account{}, "", errors.Join(err, rollbackErr)
	}
	return a, trashPath, nil
}

func (s *Store) SetLastSelected(provider Provider, name string) error {
	registry, err := s.Load()
	if err != nil {
		return err
	}
	if _, exists := registry.Find(provider, name); !exists {
		return fmt.Errorf("account %s/%s does not exist", provider, name)
	}
	registry.LastSelected[provider] = name
	return s.Save(registry)
}

func (s *Store) SetSelection(policy SelectionPolicy) error {
	if policy.SwitchAt <= 0 || policy.SwitchAt > 100 {
		return errors.New("switch threshold must be greater than 0 and at most 100")
	}
	registry, err := s.Load()
	if err != nil {
		return err
	}
	registry.Selection = policy
	return s.Save(registry)
}

func (s *Store) Save(registry Registry) error {
	if err := os.MkdirAll(filepath.Dir(s.registryPath), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding account registry: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(s.registryPath), ".accounts-*.json")
	if err != nil {
		return fmt.Errorf("creating temporary registry: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("securing temporary registry: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temporary registry: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temporary registry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary registry: %w", err)
	}
	if err := os.Rename(tmpName, s.registryPath); err != nil {
		return fmt.Errorf("replacing account registry: %w", err)
	}
	return nil
}

func ParseProvider(value string) (Provider, error) {
	p := Provider(strings.ToLower(value))
	if !p.Valid() {
		return "", fmt.Errorf("unsupported provider %q (want claude or codex)", value)
	}
	return p, nil
}

func (p Provider) Valid() bool { return p == Claude || p == Codex }

func initializeCodexHome(home string) error {
	path := filepath.Join(home, "config.toml")
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking Codex config: %w", err)
	}
	const config = "# Managed per account by ajaj.\ncli_auth_credentials_store = \"file\"\n"
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		return fmt.Errorf("initializing Codex config: %w", err)
	}
	return nil
}
