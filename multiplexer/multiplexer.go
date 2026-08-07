// Package multiplexer detects and controls terminal multiplexers.
package multiplexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Kind string

const (
	Herdr  Kind = "herdr"
	Cmux   Kind = "cmux"
	Zellij Kind = "zellij"
	Tmux   Kind = "tmux"
)

type Direction string

const (
	Right Direction = "right"
	Down  Direction = "down"
)

type Command struct {
	Path  string
	Args  []string
	Dir   string
	Title string
}

type Client struct {
	kind   Kind
	binary string
	env    map[string]string
	runner commandRunner
}

type commandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

func (c *Client) Name() string { return string(c.kind) }

// RenameCurrent gives the pane or surface containing ajaj a multiplexer-native
// title. Backends use their narrowest independently named terminal primitive.
func (c *Client) RenameCurrent(ctx context.Context, title string) error {
	if title == "" {
		return fmt.Errorf("pane title is empty")
	}

	var err error
	switch c.kind {
	case Tmux:
		_, err = c.runner.Run(ctx, c.binary, "select-pane", "-t", c.env["TMUX_PANE"], "-T", title)
	case Zellij:
		args := []string{"action", "rename-pane"}
		if paneID := c.env["ZELLIJ_PANE_ID"]; paneID != "" {
			args = append(args, "--pane-id", paneID)
		}
		args = append(args, title)
		_, err = c.runner.Run(ctx, c.binary, c.zellijArgs(args...)...)
	case Herdr:
		_, err = c.runner.Run(ctx, c.binary, "pane", "rename", c.env["HERDR_PANE_ID"], title)
	case Cmux:
		_, err = c.runner.Run(ctx, c.binary, "rename-tab", "--workspace", c.env["CMUX_WORKSPACE_ID"], "--surface", c.env["CMUX_SURFACE_ID"], title)
	default:
		return fmt.Errorf("unsupported multiplexer %q", c.kind)
	}
	if err != nil {
		return fmt.Errorf("renaming current pane in %s: %w", c.kind, err)
	}
	return nil
}

// ClearCurrentTitle removes the ajaj-assigned title from the pane or surface.
// Cleanup uses a context detached from launch cancellation so Ctrl+C does not
// leave the pane named after an agent that has already exited.
func (c *Client) ClearCurrentTitle(ctx context.Context) error {
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()

	var err error
	switch c.kind {
	case Tmux:
		_, err = c.runner.Run(cleanupCtx, c.binary, "select-pane", "-t", c.env["TMUX_PANE"], "-T", "")
	case Zellij:
		args := []string{"action", "undo-rename-pane"}
		if paneID := c.env["ZELLIJ_PANE_ID"]; paneID != "" {
			args = append(args, "--pane-id", paneID)
		}
		_, err = c.runner.Run(cleanupCtx, c.binary, c.zellijArgs(args...)...)
	case Herdr:
		_, err = c.runner.Run(cleanupCtx, c.binary, "pane", "rename", c.env["HERDR_PANE_ID"], "--clear")
	case Cmux:
		_, err = c.runner.Run(cleanupCtx, c.binary, "rename-tab", "--workspace", c.env["CMUX_WORKSPACE_ID"], "--surface", c.env["CMUX_SURFACE_ID"], "")
	default:
		return fmt.Errorf("unsupported multiplexer %q", c.kind)
	}
	if err != nil {
		return fmt.Errorf("clearing current pane title in %s: %w", c.kind, err)
	}
	return nil
}

// Detect returns the highest-priority supported multiplexer indicated by the
// environment, provided its control binary is available. Pane-aware backends
// take precedence over tmux, whose variables are commonly inherited by nested
// multiplexers.
func Detect(environ []string) (*Client, bool) {
	return detect(environ, exec.LookPath)
}

type lookupFunc func(string) (string, error)

type candidate struct {
	kind   Kind
	binary string
	inside func(map[string]string) bool
}

var candidates = []candidate{
	{Herdr, "herdr", func(env map[string]string) bool { return env["HERDR_PANE_ID"] != "" }},
	{Cmux, "cmux", func(env map[string]string) bool {
		return env["CMUX_WORKSPACE_ID"] != "" && env["CMUX_SURFACE_ID"] != ""
	}},
	{Zellij, "zellij", func(env map[string]string) bool { return env["ZELLIJ"] != "" }},
	{Tmux, "tmux", func(env map[string]string) bool {
		return env["TMUX"] != "" && env["TMUX_PANE"] != ""
	}},
}

func detect(environ []string, lookPath lookupFunc) (*Client, bool) {
	env := environmentMap(environ)
	for _, item := range candidates {
		if !item.inside(env) {
			continue
		}
		binary, err := lookPath(item.binary)
		if err != nil {
			continue
		}
		return &Client{kind: item.kind, binary: binary, env: env, runner: execRunner{}}, true
	}
	return nil, false
}

func environmentMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, item := range environ {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func (c *Client) Split(ctx context.Context, direction Direction, command Command) error {
	if direction != Right && direction != Down {
		return fmt.Errorf("unsupported split direction %q", direction)
	}
	if command.Path == "" {
		return fmt.Errorf("split command path is empty")
	}
	if command.Dir == "" {
		return fmt.Errorf("split command working directory is empty")
	}
	if command.Title == "" {
		return fmt.Errorf("split command title is empty")
	}

	var err error
	switch c.kind {
	case Tmux:
		err = c.splitTmux(ctx, direction, command)
	case Zellij:
		err = c.splitZellij(ctx, direction, command)
	case Herdr:
		err = c.splitHerdr(ctx, direction, command)
	case Cmux:
		err = c.splitCmux(ctx, direction, command)
	default:
		err = fmt.Errorf("unsupported multiplexer %q", c.kind)
	}
	if err != nil {
		return fmt.Errorf("opening %s split in %s: %w", direction, c.kind, err)
	}
	return nil
}

func (c *Client) splitTmux(ctx context.Context, direction Direction, command Command) error {
	flag := "-h"
	if direction == Down {
		flag = "-v"
	}
	args := []string{"split-window", flag, "-t", c.env["TMUX_PANE"], "-c", command.Dir, "-P", "-F", "#{pane_id}", "--", command.Path}
	args = append(args, command.Args...)
	output, err := c.runner.Run(ctx, c.binary, args...)
	if err != nil {
		return fmt.Errorf("running tmux split-window: %w", err)
	}
	paneID := strings.TrimSpace(string(output))
	if paneID == "" {
		return fmt.Errorf("tmux split response did not include a pane ID")
	}
	if _, err := c.runner.Run(ctx, c.binary, "select-pane", "-t", paneID, "-T", command.Title); err != nil {
		return fmt.Errorf("renaming tmux pane %s: %w", paneID, err)
	}
	return nil
}

func (c *Client) splitZellij(ctx context.Context, direction Direction, command Command) error {
	output, err := c.runner.Run(ctx, c.binary, c.zellijArgs("action", "new-pane", "--direction", string(direction), "--cwd", command.Dir, "--name", command.Title)...)
	if err != nil {
		return fmt.Errorf("running zellij new-pane: %w", err)
	}
	paneID, err := zellijPaneID(output)
	if err != nil {
		return err
	}
	cleanup := func() {
		cleanupCtx, cancel := cleanupContext(ctx)
		defer cancel()
		_, _ = c.runner.Run(cleanupCtx, c.binary, c.zellijArgs("action", "close-pane", "--pane-id", paneID)...)
	}
	if _, err := c.runner.Run(ctx, c.binary, c.zellijArgs("action", "write-chars", "--pane-id", paneID, shellCommand(command))...); err != nil {
		cleanup()
		return fmt.Errorf("writing command to zellij pane %s: %w", paneID, err)
	}
	if _, err := c.runner.Run(ctx, c.binary, c.zellijArgs("action", "send-keys", "--pane-id", paneID, "Enter")...); err != nil {
		cleanup()
		return fmt.Errorf("starting command in zellij pane %s: %w", paneID, err)
	}
	if _, err := c.runner.Run(ctx, c.binary, c.zellijArgs("action", "focus-pane-id", paneID)...); err != nil {
		return fmt.Errorf("focusing zellij pane %s: %w", paneID, err)
	}
	return nil
}

func (c *Client) zellijArgs(args ...string) []string {
	if session := c.env["ZELLIJ_SESSION_NAME"]; session != "" {
		return append([]string{"--session", session}, args...)
	}
	return args
}

func zellijPaneID(output []byte) (string, error) {
	paneID := strings.TrimSpace(string(output))
	if !strings.HasPrefix(paneID, "terminal_") {
		return "", fmt.Errorf("zellij pane response did not include a terminal pane ID: %q", paneID)
	}
	return paneID, nil
}

func (c *Client) splitHerdr(ctx context.Context, direction Direction, command Command) error {
	output, err := c.runner.Run(ctx, c.binary, "pane", "split", "--current", "--direction", string(direction), "--cwd", command.Dir, "--focus")
	if err != nil {
		return fmt.Errorf("creating herdr pane: %w", err)
	}
	paneID, err := herdrPaneID(output)
	if err != nil {
		return err
	}
	if _, err := c.runner.Run(ctx, c.binary, "pane", "rename", paneID, command.Title); err != nil {
		c.closeHerdrPane(ctx, paneID)
		return fmt.Errorf("renaming herdr pane %s: %w", paneID, err)
	}
	if _, err := c.runner.Run(ctx, c.binary, "pane", "run", paneID, shellCommand(command)); err != nil {
		c.closeHerdrPane(ctx, paneID)
		return fmt.Errorf("starting command in herdr pane %s: %w", paneID, err)
	}
	return nil
}

func (c *Client) closeHerdrPane(ctx context.Context, paneID string) {
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	_, _ = c.runner.Run(cleanupCtx, c.binary, "pane", "close", paneID)
}

func herdrPaneID(output []byte) (string, error) {
	var response struct {
		Result struct {
			Pane struct {
				ID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return "", fmt.Errorf("decoding herdr split response: %w", err)
	}
	if response.Result.Pane.ID == "" {
		return "", fmt.Errorf("herdr split response did not include a pane ID")
	}
	return response.Result.Pane.ID, nil
}

func (c *Client) splitCmux(ctx context.Context, direction Direction, command Command) error {
	workspace := c.env["CMUX_WORKSPACE_ID"]
	output, err := c.runner.Run(ctx, c.binary, "new-pane", "--type", "terminal", "--direction", string(direction), "--workspace", workspace)
	if err != nil {
		return fmt.Errorf("creating cmux pane: %w", err)
	}
	surface, err := cmuxSurfaceID(output)
	if err != nil {
		return err
	}
	cleanup := func() {
		cleanupCtx, cancel := cleanupContext(ctx)
		defer cancel()
		_, _ = c.runner.Run(cleanupCtx, c.binary, "close-surface", "--workspace", workspace, "--surface", surface)
	}
	if _, err := c.runner.Run(ctx, c.binary, "rename-tab", "--workspace", workspace, "--surface", surface, command.Title); err != nil {
		cleanup()
		return fmt.Errorf("renaming cmux surface %s: %w", surface, err)
	}
	launch := "cd -- " + shellQuote(command.Dir) + " && exec " + shellCommand(command) + "\n"
	if _, err := c.runner.Run(ctx, c.binary, "send", "--workspace", workspace, "--surface", surface, launch); err != nil {
		cleanup()
		return fmt.Errorf("starting command in cmux surface %s: %w", surface, err)
	}
	_, err = c.runner.Run(ctx, c.binary, "focus-panel", "--workspace", workspace, "--panel", surface)
	if err != nil {
		return fmt.Errorf("focusing cmux surface %s: %w", surface, err)
	}
	return nil
}

func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
}

func cmuxSurfaceID(output []byte) (string, error) {
	for field := range strings.FieldsSeq(string(output)) {
		if value, ok := strings.CutPrefix(field, "surface="); ok && value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("cmux pane response did not include a surface ID")
}

func shellCommand(command Command) string {
	parts := make([]string, 0, len(command.Args)+1)
	parts = append(parts, shellQuote(command.Path))
	for _, arg := range command.Args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
