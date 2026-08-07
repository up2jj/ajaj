package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/up2jj/ajaj/account"
)

func newLoginCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "login <claude|codex> <name>",
		Short: "Authenticate an isolated account using the provider CLI",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := findAccount(deps, args[0], args[1])
			if err != nil {
				return err
			}
			return deps.runner.Login(cmd.Context(), a)
		},
	}
}

func newRunCmd(deps dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "run <claude|codex> <name> [provider arguments...]",
		Short:              "Run a provider with a specific account",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := findAccount(deps, args[0], args[1])
			if err != nil {
				return err
			}
			return runAccount(cmd, deps, a, args[2:]...)
		},
	}
	return cmd
}

func newProviderCmd(deps dependencies, provider account.Provider) *cobra.Command {
	return &cobra.Command{
		Use:                string(provider) + " [arguments...]",
		Short:              "Run " + string(provider) + " with its default or auto-selected profile",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := deps.store.Load()
			if err != nil {
				return err
			}
			a, ok := registry.DefaultAccount(provider)
			if !ok {
				return fmt.Errorf("no default %s profile; add one with ajaj account add %s <name>", provider, provider)
			}
			if deps.usage != nil {
				selection, selectErr := deps.usage.Select(cmd.Context(), registry, provider)
				if selectErr != nil {
					return selectErr
				}
				a = selection.Account
				if selection.Warning != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "ajaj: usage refresh warning: %v\n", selection.Warning)
				}
				if selection.Switched {
					fmt.Fprintf(cmd.ErrOrStderr(), "ajaj: auto-selected %s (%s)\n", a.ID(), selection.Reason)
				}
			}
			return runAccount(cmd, deps, a, args...)
		},
	}
}

func runAccount(cmd *cobra.Command, deps dependencies, a account.Account, args ...string) error {
	if err := deps.store.SetLastSelected(a.Provider, a.Name); err != nil {
		return err
	}
	return deps.runner.Run(cmd.Context(), a, args...)
}

func findAccount(deps dependencies, providerName, name string) (account.Account, error) {
	provider, err := account.ParseProvider(providerName)
	if err != nil {
		return account.Account{}, err
	}
	registry, err := deps.store.Load()
	if err != nil {
		return account.Account{}, err
	}
	a, ok := registry.Find(provider, name)
	if !ok {
		return account.Account{}, fmt.Errorf("account %s/%s does not exist", provider, name)
	}
	return a, nil
}
