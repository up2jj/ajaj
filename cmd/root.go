// Package cmd defines ajaj's Cobra command surface.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/up2jj/ajaj/account"
	"github.com/up2jj/ajaj/provider"
	"github.com/up2jj/ajaj/runner"
	"github.com/up2jj/ajaj/tui"
	usagepkg "github.com/up2jj/ajaj/usage"
)

var version = "dev"

type dependencies struct {
	store  *account.Store
	runner accountRunner
	usage  usageManager
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
		usage: usageManager,
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
			if !ok || model.Selected == nil {
				return nil
			}
			return runAccount(cmd, deps, *model.Selected)
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
