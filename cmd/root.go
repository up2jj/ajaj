// Package cmd defines ajaj's Cobra command surface.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/up2jj/ajaj/account"
	"github.com/up2jj/ajaj/provider"
	"github.com/up2jj/ajaj/runner"
	"github.com/up2jj/ajaj/tui"
	usagepkg "github.com/up2jj/ajaj/usage"
)

var version = "dev"

type dependencies struct {
	store         *account.Store
	runner        accountRunner
	usage         usageManager
	confirmDelete func(context.Context, account.Account) (bool, error)
}

type accountRunner interface {
	Run(context.Context, account.Account, ...string) error
	Login(context.Context, account.Account) error
}

type usageManager interface {
	Select(context.Context, account.Registry, account.Provider) (usagepkg.Selection, error)
	Snapshot(account.Account) (usagepkg.Snapshot, bool, error)
	Refresh(context.Context, account.Account) (usagepkg.Snapshot, error)
	RecordClaude(account.Account, io.Reader) (usagepkg.Snapshot, error)
	EnsureClaudeCollector(account.Account) (bool, error)
	DeleteSnapshot(account.Account) error
}

func NewRootCmd() *cobra.Command {
	registryPath, accountsDir, err := account.DefaultPaths()
	if err != nil {
		return brokenRoot(err)
	}
	usageManager, err := usagepkg.DefaultManager()
	if err != nil {
		return brokenRoot(err)
	}
	return newRootCmd(dependencies{
		store: account.NewStore(registryPath, accountsDir),
		runner: runner.Runner{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
			Env: provider.BaseEnvironment(),
		},
		usage:         usageManager,
		confirmDelete: confirmAccountDeletion,
	})
}

func newRootCmd(deps dependencies) *cobra.Command {
	root := &cobra.Command{
		Use:           "ajaj",
		Short:         "Switch between Claude Code and Codex accounts",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			registry, err := deps.store.Load()
			if err != nil {
				return err
			}
			if len(registry.Accounts) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No accounts yet. Add one with: ajaj account add <claude|codex> <name>")
				return nil
			}

			program := tea.NewProgram(tui.New(registry.Accounts, registry.Default))
			final, err := program.Run()
			if err != nil {
				return fmt.Errorf("running account picker: %w", err)
			}
			model, ok := final.(tui.Model)
			if !ok {
				return nil
			}
			return handlePickerResult(cmd, deps, model)
		},
	}

	root.AddCommand(newAccountCmd(deps))
	root.AddCommand(newLoginCmd(deps))
	root.AddCommand(newRunCmd(deps))
	root.AddCommand(newUsageCmd(deps))
	root.AddCommand(newProviderCmd(deps, account.Claude))
	root.AddCommand(newProviderCmd(deps, account.Codex))
	return root
}

func handlePickerResult(cmd *cobra.Command, deps dependencies, model tui.Model) error {
	if model.DeleteRequested != nil {
		return deleteAccount(cmd, deps, *model.DeleteRequested)
	}
	if model.DefaultRequested != nil {
		a := *model.DefaultRequested
		if err := deps.store.SetDefault(a.Provider, a.Name); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Default %s profile: %s\n", a.Provider, a.Name)
		return nil
	}
	if model.Selected == nil {
		return nil
	}
	return runAccount(cmd, deps, *model.Selected)
}

func confirmAccountDeletion(ctx context.Context, a account.Account) (bool, error) {
	confirmed := false
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Delete " + a.ID() + "?").
			Description("The profile will be removed from ajaj and its data moved to trash.").
			Affirmative("Delete").
			Negative("Cancel").
			Value(&confirmed),
	))
	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, nil
		}
		return false, fmt.Errorf("confirming profile deletion: %w", err)
	}
	return confirmed, nil
}

func brokenRoot(initErr error) *cobra.Command {
	return &cobra.Command{
		Use:           "ajaj",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(*cobra.Command, []string) error {
			return initErr
		},
	}
}
