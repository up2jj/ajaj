package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/up2jj/ajaj/account"
	usagepkg "github.com/up2jj/ajaj/usage"
)

func newUsageCmd(deps dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Inspect and refresh account usage",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			registry, err := deps.store.Load()
			if err != nil {
				return err
			}
			for _, a := range registry.Accounts {
				text := "unknown"
				if deps.usage != nil {
					snapshot, ok, snapshotErr := deps.usage.Snapshot(a)
					if snapshotErr != nil {
						return snapshotErr
					}
					if ok {
						text = usagepkg.Format(snapshot, time.Now())
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s\n", a.ID(), text)
			}
			return nil
		},
	}
	cmd.AddCommand(newUsageRefreshCmd(deps))
	cmd.AddCommand(newUsageIngestCmd(deps))
	cmd.AddCommand(newUsageInstallCmd(deps))
	return cmd
}

func newUsageRefreshCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh codex [name]",
		Short: "Refresh Codex usage through its app-server",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := account.ParseProvider(args[0])
			if err != nil {
				return err
			}
			if provider != account.Codex {
				return fmt.Errorf("live refresh is only available for Codex; Claude updates through its status-line collector")
			}
			registry, err := deps.store.Load()
			if err != nil {
				return err
			}
			var accounts []account.Account
			if len(args) == 2 {
				a, ok := registry.Find(provider, args[1])
				if !ok {
					return fmt.Errorf("account %s/%s does not exist", provider, args[1])
				}
				accounts = []account.Account{a}
			} else {
				for _, a := range registry.Accounts {
					if a.Provider == provider {
						accounts = append(accounts, a)
					}
				}
			}
			var refreshErrors error
			for _, a := range accounts {
				snapshot, refreshErr := deps.usage.Refresh(cmd.Context(), a)
				if refreshErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", a.ID(), refreshErr)
					refreshErrors = errors.Join(refreshErrors, fmt.Errorf("%s: %w", a.ID(), refreshErr))
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s\n", a.ID(), usagepkg.Format(snapshot, time.Now()))
			}
			return refreshErrors
		},
	}
}

func newUsageIngestCmd(deps dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "ingest claude <name>",
		Short:  "Record Claude status-line usage",
		Hidden: true,
		Args:   cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := findAccount(deps, args[0], args[1])
			if err != nil {
				return err
			}
			snapshot, err := deps.usage.RecordClaude(a, cmd.InOrStdin())
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), usagepkg.Format(snapshot, time.Now()))
			return nil
		},
	}
	return cmd
}

func newUsageInstallCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "install claude <name>",
		Short: "Install the Claude usage collector when no status line is configured",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := findAccount(deps, args[0], args[1])
			if err != nil {
				return err
			}
			if a.Provider != account.Claude {
				return fmt.Errorf("the status-line collector is only used for Claude")
			}
			installed, err := deps.usage.EnsureClaudeCollector(a)
			if err != nil {
				return err
			}
			if installed {
				fmt.Fprintln(cmd.OutOrStdout(), "Installed Claude usage collector.")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Claude already has a status line; left it unchanged.")
			}
			return nil
		},
	}
}
