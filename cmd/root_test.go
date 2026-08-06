package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/up2jj/ajaj/account"
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
		{"use", []string{"account", "use", "claude", "work"}, "Active claude account: work"},
		{"auto", []string{"account", "auto", "on", "--threshold", "80"}, "switch at 80%"},
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
