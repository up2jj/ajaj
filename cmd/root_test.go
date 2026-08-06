package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/up2jj/ajaj/account"
	usagepkg "github.com/up2jj/ajaj/usage"
)

type fakeRunner struct {
	account account.Account
	args    []string
	login   bool
}

func (r *fakeRunner) Run(_ context.Context, a account.Account, args ...string) error {
	r.account = a
	r.args = append([]string(nil), args...)
	return nil
}

func (r *fakeRunner) Login(_ context.Context, a account.Account) error {
	r.account = a
	r.login = true
	return nil
}

func TestAccountCommands(t *testing.T) {
	rootDir := t.TempDir()
	store := account.NewStore(filepath.Join(rootDir, "accounts.json"), filepath.Join(rootDir, "profiles"))

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"add", []string{"account", "add", "claude", "personal"}, "Added claude/personal"},
		{"add second", []string{"account", "add", "claude", "work"}, "Added claude/work"},
		{"use", []string{"account", "use", "claude", "work"}, "Preferred claude profile: work"},
		{"auto", []string{"account", "auto", "on", "--threshold", "80"}, "switch at 80%"},
		{"current", []string{"account", "current", "claude"}, "preferred=work  last-selected=never launched"},
		{"list", []string{"account", "list"}, "* claude   work"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := new(bytes.Buffer)
			root := newRootCmd(dependencies{store: store})
			root.SetOut(out)
			root.SetErr(out)
			root.SetArgs(tt.args)
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Fatalf("output = %q, want substring %q", out, tt.want)
			}
		})
	}
}

func TestEmptyRootExplainsNextStep(t *testing.T) {
	rootDir := t.TempDir()
	store := account.NewStore(filepath.Join(rootDir, "accounts.json"), filepath.Join(rootDir, "profiles"))
	out := new(bytes.Buffer)
	root := newRootCmd(dependencies{store: store})
	root.SetOut(out)
	root.SetArgs(nil)

	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ajaj account add") {
		t.Fatalf("output = %q, want add-account hint", out)
	}
}

func TestProviderCommandForwardsArgumentsToActiveAccount(t *testing.T) {
	rootDir := t.TempDir()
	store := account.NewStore(filepath.Join(rootDir, "accounts.json"), filepath.Join(rootDir, "profiles"))
	if _, err := store.Add(account.Claude, "work"); err != nil {
		t.Fatal(err)
	}
	fake := new(fakeRunner)
	root := newRootCmd(dependencies{store: store, runner: fake})
	root.SetArgs([]string{"claude", "--model", "opus", "explain this"})

	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if fake.account.ID() != "claude/work" {
		t.Fatalf("account = %q, want claude/work", fake.account.ID())
	}
	if got := strings.Join(fake.args, "|"); got != "--model|opus|explain this" {
		t.Fatalf("arguments = %q", got)
	}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if registry.LastSelected[account.Claude] != "work" {
		t.Fatalf("last selected profile = %q, want work", registry.LastSelected[account.Claude])
	}

	out := new(bytes.Buffer)
	current := newRootCmd(dependencies{store: store})
	current.SetOut(out)
	current.SetArgs([]string{"account", "current", "claude"})
	if err := current.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "preferred=work  last-selected=work") {
		t.Fatalf("account current output = %q", out.String())
	}
}

func TestLoginUsesExplicitAccount(t *testing.T) {
	rootDir := t.TempDir()
	store := account.NewStore(filepath.Join(rootDir, "accounts.json"), filepath.Join(rootDir, "profiles"))
	if _, err := store.Add(account.Codex, "personal"); err != nil {
		t.Fatal(err)
	}
	fake := new(fakeRunner)
	root := newRootCmd(dependencies{store: store, runner: fake})
	root.SetArgs([]string{"login", "codex", "personal"})

	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !fake.login || fake.account.ID() != "codex/personal" {
		t.Fatalf("Login() = login:%v account:%q", fake.login, fake.account.ID())
	}
}

func TestAutomaticSelectionRecordsActuallyLaunchedProfile(t *testing.T) {
	rootDir := t.TempDir()
	store := account.NewStore(filepath.Join(rootDir, "accounts.json"), filepath.Join(rootDir, "profiles"))
	work, err := store.Add(account.Claude, "work")
	if err != nil {
		t.Fatal(err)
	}
	personal, err := store.Add(account.Claude, "personal")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	usageStore := usagepkg.NewStore(filepath.Join(rootDir, "usage"))
	for a, used := range map[account.Account]float64{work: 95, personal: 10} {
		snapshot := usagepkg.Snapshot{
			Provider: a.Provider, Account: a.Name, Source: "test", UpdatedAt: now,
			Windows: []usagepkg.Window{{Name: "5h", UsedPercent: used, ResetsAt: now.Add(time.Hour).Unix()}},
		}
		if err := usageStore.Write(a, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	fake := new(fakeRunner)
	out := new(bytes.Buffer)
	root := newRootCmd(dependencies{store: store, runner: fake, usage: usagepkg.NewManager(usageStore, nil)})
	root.SetErr(out)
	root.SetArgs([]string{"claude"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if fake.account.Name != "personal" {
		t.Fatalf("launched profile = %q, want personal", fake.account.Name)
	}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if registry.Active[account.Claude] != "work" || registry.LastSelected[account.Claude] != "personal" {
		t.Fatalf("preferred/last-selected = %q/%q", registry.Active[account.Claude], registry.LastSelected[account.Claude])
	}
	if !strings.Contains(out.String(), "auto-selected claude/personal") {
		t.Fatalf("stderr = %q", out.String())
	}
}
