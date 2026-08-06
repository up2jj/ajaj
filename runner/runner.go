// Package runner launches provider CLIs with isolated account state.
package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/charmbracelet/x/term"

	"github.com/up2jj/ajaj/account"
	"github.com/up2jj/ajaj/provider"
)

type Runner struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
	Env []string

	// IsTerminal is optional and primarily useful for tests. The default checks
	// whether an *os.File is attached to an interactive terminal.
	IsTerminal func(io.Writer) bool
}

func (r Runner) Run(ctx context.Context, a account.Account, args ...string) error {
	spec, err := provider.Lookup(a.Provider)
	if err != nil {
		return err
	}
	binary, err := provider.ResolveBinary(spec.Binary)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Stdin = r.In
	command.Stdout = r.Out
	errOut := r.Err
	if errOut == nil {
		errOut = io.Discard
	}
	command.Stderr = errOut
	command.Env = sessionEnvironment(r.Env, spec.HomeEnv, a)
	fmt.Fprintf(errOut, "ajaj: running %s\n", a.ID())
	if r.isTerminal(errOut) {
		// Save the terminal title, identify the running profile, then restore it.
		fmt.Fprintf(errOut, "\x1b[22;0t\x1b]0;ajaj · %s\x07", a.ID())
		defer fmt.Fprint(errOut, "\x1b[23;0t")
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("running %s for %s: %w", spec.Binary, a.ID(), err)
	}
	return nil
}

func sessionEnvironment(base []string, homeKey string, a account.Account) []string {
	env := provider.Environment(base, homeKey, a.Home)
	env = provider.Environment(env, "AJAJ_PROVIDER", string(a.Provider))
	env = provider.Environment(env, "AJAJ_PROFILE", a.Name)
	return provider.Environment(env, "AJAJ_ACCOUNT", a.ID())
}

func (r Runner) isTerminal(w io.Writer) bool {
	if r.IsTerminal != nil {
		return r.IsTerminal(w)
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(file.Fd())
}

func (r Runner) Login(ctx context.Context, a account.Account) error {
	spec, err := provider.Lookup(a.Provider)
	if err != nil {
		return err
	}
	return r.Run(ctx, a, spec.LoginArg...)
}
