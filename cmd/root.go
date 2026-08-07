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
	"github.com/up2jj/ajaj/multiplexer"
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
	multiplexer   splitLauncher
	executable    func() (string, error)
	workingDir    func() (string, error)
	confirmDelete func(context.Context, account.Account) (bool, error)
}

type accountRunner interface {
	Run(context.Context, account.Account, ...string) error
	Login(context.Context, account.Account) error
}

type splitLauncher interface {
	Name() string
	RenameCurrent(context.Context, string) error
	Split(context.Context, multiplexer.Direction, multiplexer.Command) error
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
	deps := dependencies{
		store: account.NewStore(registryPath, accountsDir),
		runner: runner.Runner{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
			Env: provider.BaseEnvironment(),
		},
		usage:         usageManager,
		executable:    os.Executable,
		workingDir:    os.Getwd,
		confirmDelete: confirmAccountDeletion,
	}
	if client, ok := multiplexer.Detect(provider.BaseEnvironment()); ok {
		deps.multiplexer = client
	}
	return newRootCmd(deps)
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

			multiplexerName := ""
			if deps.multiplexer != nil {
				multiplexerName = deps.multiplexer.Name()
			}
			program := tea.NewProgram(tui.NewWithMultiplexer(registry.Accounts, registry.Default, multiplexerName))
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
	if model.Selection == nil {
		return nil
	}
	selection := *model.Selection
	title := "ajaj: " + selection.Account.ID()
	if selection.Destination == tui.CurrentPane {
		if deps.multiplexer != nil {
			if err := deps.multiplexer.RenameCurrent(cmd.Context(), title); err != nil {
				return err
			}
		}
		return runAccount(cmd, deps, selection.Account)
	}
	if deps.multiplexer == nil {
		return fmt.Errorf("cannot open %s: no multiplexer is available", selection.Destination.Label())
	}
	direction, err := splitDirection(selection.Destination)
	if err != nil {
		return err
	}
	executable := deps.executable
	if executable == nil {
		executable = os.Executable
	}
	path, err := executable()
	if err != nil {
		return fmt.Errorf("locating ajaj executable: %w", err)
	}
	workingDir := deps.workingDir
	if workingDir == nil {
		workingDir = os.Getwd
	}
	dir, err := workingDir()
	if err != nil {
		return fmt.Errorf("determining current directory: %w", err)
	}
	launch := multiplexer.Command{
		Path:  path,
		Args:  []string{"run", string(selection.Account.Provider), selection.Account.Name},
		Dir:   dir,
		Title: title,
	}
	return deps.multiplexer.Split(cmd.Context(), direction, launch)
}

func splitDirection(destination tui.Destination) (multiplexer.Direction, error) {
	switch destination {
	case tui.SplitRight:
		return multiplexer.Right, nil
	case tui.SplitDown:
		return multiplexer.Down, nil
	default:
		return "", fmt.Errorf("unsupported launch destination %q", destination)
	}
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
