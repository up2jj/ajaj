// Package runner launches provider CLIs with isolated account state.
package runner

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/up2jj/ajaj/account"
	"github.com/up2jj/ajaj/provider"
)

type Runner struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
	Env []string
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
	command.Stderr = r.Err
	command.Env = provider.Environment(r.Env, spec.HomeEnv, a.Home)
	if err := command.Run(); err != nil {
		return fmt.Errorf("running %s for %s: %w", spec.Binary, a.ID(), err)
	}
	return nil
}

func (r Runner) Login(ctx context.Context, a account.Account) error {
	spec, err := provider.Lookup(a.Provider)
	if err != nil {
		return err
	}
	return r.Run(ctx, a, spec.LoginArg...)
}
