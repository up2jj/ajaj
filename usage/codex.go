package usage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"

	"github.com/up2jj/ajaj/account"
	"github.com/up2jj/ajaj/provider"
)

type rpcEnvelope struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rateLimitResult struct {
	RateLimits struct {
		Primary              *rateLimitWindow `json:"primary"`
		Secondary            *rateLimitWindow `json:"secondary"`
		RateLimitReachedType *string          `json:"rateLimitReachedType"`
	} `json:"rateLimits"`
}

type rateLimitWindow struct {
	UsedPercent        float64 `json:"usedPercent"`
	WindowDurationMins int     `json:"windowDurationMins"`
	ResetsAt           int64   `json:"resetsAt"`
}

func (m *Manager) refreshCodex(ctx context.Context, a account.Account) (Snapshot, error) {
	binary, err := exec.LookPath("codex")
	if err != nil {
		return Snapshot{}, fmt.Errorf("codex is not installed or not on PATH")
	}
	refreshCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	command := exec.CommandContext(refreshCtx, binary, "app-server")
	command.Env = provider.Environment(m.env, "CODEX_HOME", a.Home)
	stdin, err := command.StdinPipe()
	if err != nil {
		return Snapshot{}, fmt.Errorf("opening Codex app-server input: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Snapshot{}, fmt.Errorf("opening Codex app-server output: %w", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return Snapshot{}, fmt.Errorf("starting Codex app-server: %w", err)
	}
	defer func() {
		cancel()
		_ = stdin.Close()
		_ = command.Wait()
	}()

	encoder := json.NewEncoder(stdin)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	if err := encoder.Encode(map[string]any{
		"method": "initialize",
		"id":     1,
		"params": map[string]any{"clientInfo": map[string]string{
			"name": "ajaj", "title": "ajaj", "version": "dev",
		}},
	}); err != nil {
		return Snapshot{}, fmt.Errorf("initializing Codex app-server: %w", err)
	}
	if _, err := readRPCResponse(scanner, 1); err != nil {
		return Snapshot{}, appServerError(refreshCtx, err, stderr.String())
	}
	if err := encoder.Encode(map[string]any{"method": "initialized"}); err != nil {
		return Snapshot{}, fmt.Errorf("acknowledging Codex app-server initialization: %w", err)
	}
	if err := encoder.Encode(map[string]any{"method": "account/rateLimits/read", "id": 2}); err != nil {
		return Snapshot{}, fmt.Errorf("requesting Codex usage: %w", err)
	}
	response, err := readRPCResponse(scanner, 2)
	if err != nil {
		return Snapshot{}, appServerError(refreshCtx, err, stderr.String())
	}
	var result rateLimitResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return Snapshot{}, fmt.Errorf("decoding Codex usage: %w", err)
	}

	snapshot := Snapshot{Provider: a.Provider, Account: a.Name, Source: "codex-app-server", UpdatedAt: m.now()}
	snapshot.Reached = result.RateLimits.RateLimitReachedType != nil
	for _, window := range []*rateLimitWindow{result.RateLimits.Primary, result.RateLimits.Secondary} {
		if window == nil {
			continue
		}
		snapshot.Windows = append(snapshot.Windows, Window{
			Name:        windowName(window.WindowDurationMins),
			UsedPercent: window.UsedPercent,
			ResetsAt:    window.ResetsAt,
		})
	}
	if len(snapshot.Windows) == 0 && !snapshot.Reached {
		return Snapshot{}, fmt.Errorf("Codex did not report ChatGPT usage for %s", a.ID())
	}
	return snapshot, nil
}

func readRPCResponse(scanner *bufio.Scanner, id int) (rpcEnvelope, error) {
	wantID := fmt.Sprintf("%d", id)
	for scanner.Scan() {
		var response rpcEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			continue
		}
		if string(response.ID) != wantID {
			continue
		}
		if response.Error != nil {
			return rpcEnvelope{}, fmt.Errorf("Codex app-server error %d: %s", response.Error.Code, response.Error.Message)
		}
		return response, nil
	}
	if err := scanner.Err(); err != nil {
		return rpcEnvelope{}, err
	}
	return rpcEnvelope{}, io.EOF
}

func appServerError(ctx context.Context, err error, stderr string) error {
	if ctx.Err() != nil {
		return fmt.Errorf("querying Codex usage: %w", ctx.Err())
	}
	if stderr != "" {
		return fmt.Errorf("querying Codex usage: %w: %s", err, stderr)
	}
	return fmt.Errorf("querying Codex usage: %w", err)
}

func windowName(minutes int) string {
	if minutes > 0 && minutes%(7*24*60) == 0 {
		return fmt.Sprintf("%dw", minutes/(7*24*60))
	}
	if minutes > 0 && minutes%60 == 0 {
		return fmt.Sprintf("%dh", minutes/60)
	}
	return fmt.Sprintf("%dm", minutes)
}
