// Package provider describes how ajaj isolates and launches supported tools.
package provider

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/up2jj/ajaj/account"
)

type Spec struct {
	Name     account.Provider
	Binary   string
	HomeEnv  string
	LoginArg []string
}

func Lookup(name account.Provider) (Spec, error) {
	switch name {
	case account.Claude:
		return Spec{Name: name, Binary: "claude", HomeEnv: "CLAUDE_CONFIG_DIR"}, nil
	case account.Codex:
		return Spec{Name: name, Binary: "codex", HomeEnv: "CODEX_HOME", LoginArg: []string{"login"}}, nil
	default:
		return Spec{}, fmt.Errorf("unsupported provider %q", name)
	}
}

func Environment(base []string, key, value string) []string {
	prefix := key + "="
	env := make([]string, 0, len(base)+1)
	for _, item := range base {
		if !strings.HasPrefix(item, prefix) {
			env = append(env, item)
		}
	}
	return append(env, prefix+value)
}

func ResolveBinary(binary string) (string, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("%s is not installed or not on PATH", binary)
	}
	return path, nil
}

func BaseEnvironment() []string { return os.Environ() }
