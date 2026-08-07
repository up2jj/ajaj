package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/up2jj/ajaj/account"
	usagepkg "github.com/up2jj/ajaj/usage"
)

func newAccountCmd(deps dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "account", Short: "Manage isolated accounts"}
	cmd.AddCommand(newAccountAddCmd(deps))
	cmd.AddCommand(newAccountListCmd(deps))
	cmd.AddCommand(newAccountCurrentCmd(deps))
	cmd.AddCommand(newAccountDefaultCmd(deps))
	cmd.AddCommand(newAccountAutoCmd(deps))
	return cmd
}

func newAccountCurrentCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "current [claude|codex]",
		Short: "Show default and most recently launched profiles",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := deps.store.Load()
			if err != nil {
				return err
			}
			providers := []account.Provider{account.Claude, account.Codex}
			if len(args) == 1 {
				provider, parseErr := account.ParseProvider(args[0])
				if parseErr != nil {
					return parseErr
				}
				providers = []account.Provider{provider}
			}
			for _, provider := range providers {
				defaultName := registry.Default[provider]
				if defaultName == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%-8s not configured\n", provider)
					continue
				}
				selected := registry.LastSelected[provider]
				if selected == "" {
					selected = "never launched"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-8s default=%s  last-selected=%s\n", provider, defaultName, selected)
			}
			return nil
		},
	}
}

func newAccountAddCmd(deps dependencies) *cobra.Command {
	var login bool
	cmd := &cobra.Command{
		Use:   "add <claude|codex> <name>",
		Short: "Create an isolated account profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := account.ParseProvider(args[0])
			if err != nil {
				return err
			}
			a, err := deps.store.Add(provider, args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added %s at %s\n", a.ID(), a.Home)
			if deps.usage != nil && a.Provider == account.Claude {
				installed, installErr := deps.usage.EnsureClaudeCollector(a)
				if installErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "ajaj: could not install Claude usage collector: %v\n", installErr)
				} else if installed {
					fmt.Fprintln(cmd.OutOrStdout(), "Installed Claude usage collector.")
				}
			}
			if login {
				return deps.runner.Login(cmd.Context(), a)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Authenticate with: ajaj login %s %s\n", a.Provider, a.Name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&login, "login", false, "start provider login after creating the profile")
	return cmd
}

func newAccountListCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured accounts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			registry, err := deps.store.Load()
			if err != nil {
				return err
			}
			if len(registry.Accounts) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No accounts configured.")
				return nil
			}
			for _, a := range registry.Accounts {
				marker := " "
				if registry.Default[a.Provider] == a.Name {
					marker = "*"
				}
				usageText := ""
				if deps.usage != nil {
					snapshot, ok, usageErr := deps.usage.Snapshot(a)
					if usageErr != nil {
						return usageErr
					}
					if ok {
						usageText = "  " + usagepkg.Format(snapshot, time.Now())
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %-8s %s%s\n", marker, a.Provider, a.Name, usageText)
			}
			return nil
		},
	}
}

func newAccountAutoCmd(deps dependencies) *cobra.Command {
	var threshold float64
	cmd := &cobra.Command{
		Use:   "auto <on|off>",
		Short: "Configure usage-aware account selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := deps.store.Load()
			if err != nil {
				return err
			}
			policy := registry.Selection
			switch args[0] {
			case "on":
				policy.Auto = true
			case "off":
				policy.Auto = false
			default:
				return fmt.Errorf("invalid mode %q (want on or off)", args[0])
			}
			if cmd.Flags().Changed("threshold") {
				policy.SwitchAt = threshold
			}
			if err := deps.store.SetSelection(policy); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Automatic selection: %t (switch at %.0f%%)\n", policy.Auto, policy.SwitchAt)
			return nil
		},
	}
	cmd.Flags().Float64Var(&threshold, "threshold", 90, "usage percentage that triggers switching")
	return cmd
}

func newAccountDefaultCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "default <claude|codex> <name>",
		Short: "Set the default profile for a provider",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := account.ParseProvider(args[0])
			if err != nil {
				return err
			}
			if err := deps.store.SetDefault(provider, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Default %s profile: %s\n", provider, args[1])
			return nil
		},
	}
}
