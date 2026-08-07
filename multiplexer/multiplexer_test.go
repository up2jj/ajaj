package multiplexer

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want Kind
	}{
		{"herdr", []string{"HERDR_PANE_ID=pane-1"}, Herdr},
		{"cmux", []string{"CMUX_WORKSPACE_ID=workspace:1", "CMUX_SURFACE_ID=surface:2"}, Cmux},
		{"zellij", []string{"ZELLIJ=0"}, Zellij},
		{"tmux", []string{"TMUX=/tmp/tmux", "TMUX_PANE=%3"}, Tmux},
		{"ordinary terminal", []string{"TERM=xterm-256color"}, ""},
		{"incomplete cmux", []string{"CMUX_WORKSPACE_ID=workspace:1"}, ""},
		{"incomplete tmux", []string{"TMUX=/tmp/tmux"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, ok := detect(tt.env, func(binary string) (string, error) { return "/bin/" + binary, nil })
			if tt.want == "" {
				if ok {
					t.Fatalf("Detect() = %s, want no multiplexer", client.kind)
				}
				return
			}
			if !ok || client.kind != tt.want {
				t.Fatalf("Detect() = %#v, %t; want %s", client, ok, tt.want)
			}
		})
	}
}

func TestDetectSkipsMissingBinaryAndUsesDeterministicPrecedence(t *testing.T) {
	env := []string{
		"HERDR_PANE_ID=herdr-pane", "CMUX_WORKSPACE_ID=workspace:1", "CMUX_SURFACE_ID=surface:1",
		"ZELLIJ=0", "TMUX=/tmp/tmux", "TMUX_PANE=%1",
	}
	client, ok := detect(env, func(binary string) (string, error) {
		if binary == "herdr" {
			return "", errors.New("missing")
		}
		return "/bin/" + binary, nil
	})
	if !ok || client.kind != Cmux {
		t.Fatalf("Detect() = %#v, %t; want cmux after missing herdr", client, ok)
	}
}

func TestDetectRejectsEnvironmentWhenControlBinaryIsMissing(t *testing.T) {
	client, ok := detect([]string{"ZELLIJ=0"}, func(string) (string, error) {
		return "", errors.New("not found")
	})
	if ok || client != nil {
		t.Fatalf("Detect() = %#v, %t; want no operational multiplexer", client, ok)
	}
}

type call struct {
	binary string
	args   []string
}

type fakeCommandRunner struct {
	calls     []call
	responses [][]byte
	errors    []error
}

func (r *fakeCommandRunner) Run(_ context.Context, binary string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, call{binary: binary, args: slices.Clone(args)})
	index := len(r.calls) - 1
	var output []byte
	if index < len(r.responses) {
		output = r.responses[index]
	}
	if index < len(r.errors) {
		return output, r.errors[index]
	}
	return output, nil
}

func TestTmuxAndZellijLaunchCommands(t *testing.T) {
	command := Command{Path: "/opt/ajaj", Args: []string{"run", "claude", "work"}, Dir: "/project root", Title: "ajaj: claude/work"}
	tests := []struct {
		name string
		kind Kind
		env  map[string]string
		dir  Direction
		want []string
	}{
		{"tmux right", Tmux, map[string]string{"TMUX_PANE": "%4"}, Right, []string{"split-window", "-h", "-t", "%4", "-c", "/project root", "-P", "-F", "#{pane_id}", "--", "/opt/ajaj", "run", "claude", "work"}},
		{"tmux down", Tmux, map[string]string{"TMUX_PANE": "%4"}, Down, []string{"split-window", "-v", "-t", "%4", "-c", "/project root", "-P", "-F", "#{pane_id}", "--", "/opt/ajaj", "run", "claude", "work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeCommandRunner{responses: [][]byte{[]byte("%9\n")}}
			client := &Client{kind: tt.kind, binary: "/bin/" + string(tt.kind), env: tt.env, runner: runner}
			if err := client.Split(t.Context(), tt.dir, command); err != nil {
				t.Fatal(err)
			}
			if len(runner.calls) != 2 || !slices.Equal(runner.calls[0].args, tt.want) {
				t.Fatalf("calls = %#v, want args %#v", runner.calls, tt.want)
			}
			if got := runner.calls[1].args; !slices.Equal(got, []string{"select-pane", "-t", "%9", "-T", "ajaj: claude/work"}) {
				t.Fatalf("rename args = %#v", got)
			}
		})
	}
}

func TestZellijCreatesShellPaneThenRunsCommandInIt(t *testing.T) {
	runner := &fakeCommandRunner{responses: [][]byte{[]byte("terminal_7\n")}}
	client := &Client{
		kind: Zellij, binary: "/bin/zellij", runner: runner,
		env: map[string]string{"ZELLIJ_SESSION_NAME": "work session"},
	}
	command := Command{Path: "/opt/a'jaj", Args: []string{"run", "claude", "my profile"}, Dir: "/project root", Title: "ajaj: claude/my profile"}
	if err := client.Split(t.Context(), Right, command); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"--session", "work session", "action", "new-pane", "--direction", "right", "--cwd", "/project root", "--name", "ajaj: claude/my profile"},
		{"--session", "work session", "action", "write-chars", "--pane-id", "terminal_7", `'/opt/a'"'"'jaj' 'run' 'claude' 'my profile'`},
		{"--session", "work session", "action", "send-keys", "--pane-id", "terminal_7", "Enter"},
		{"--session", "work session", "action", "focus-pane-id", "terminal_7"},
	}
	if len(runner.calls) != len(want) {
		t.Fatalf("calls = %#v", runner.calls)
	}
	for i := range want {
		if !slices.Equal(runner.calls[i].args, want[i]) {
			t.Errorf("call %d args = %#v, want %#v", i, runner.calls[i].args, want[i])
		}
	}
}

func TestZellijCleansUpWhenWritingCommandFails(t *testing.T) {
	runner := &fakeCommandRunner{
		responses: [][]byte{[]byte("terminal_8\n")},
		errors:    []error{nil, errors.New("write failed")},
	}
	client := &Client{kind: Zellij, binary: "/bin/zellij", runner: runner, env: map[string]string{"ZELLIJ_SESSION_NAME": "work"}}
	err := client.Split(t.Context(), Down, Command{Path: "/opt/ajaj", Args: []string{"run"}, Dir: "/repo", Title: "ajaj: claude/work"})
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("Split() error = %v", err)
	}
	if len(runner.calls) != 3 || !slices.Equal(runner.calls[2].args, []string{"--session", "work", "action", "close-pane", "--pane-id", "terminal_8"}) {
		t.Fatalf("cleanup calls = %#v", runner.calls)
	}
}

func TestHerdrLaunchParsesPaneAndCleansUpFailure(t *testing.T) {
	runner := &fakeCommandRunner{
		responses: [][]byte{[]byte(`{"result":{"pane":{"pane_id":"pane-9"}}}`)},
		errors:    []error{nil, nil, errors.New("run failed")},
	}
	client := &Client{kind: Herdr, binary: "/bin/herdr", env: map[string]string{}, runner: runner}
	err := client.Split(t.Context(), Down, Command{Path: "/opt/a'jaj", Args: []string{"run", "codex", "personal"}, Dir: "/repo", Title: "ajaj: codex/personal"})
	if err == nil || !strings.Contains(err.Error(), "run failed") {
		t.Fatalf("Split() error = %v", err)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("calls = %#v, want split, rename, run, close", runner.calls)
	}
	if got := runner.calls[0].args; !slices.Equal(got, []string{"pane", "split", "--current", "--direction", "down", "--cwd", "/repo", "--focus"}) {
		t.Fatalf("split args = %#v", got)
	}
	if got := runner.calls[1].args; !slices.Equal(got, []string{"pane", "rename", "pane-9", "ajaj: codex/personal"}) {
		t.Fatalf("rename args = %#v", got)
	}
	if got := runner.calls[2].args; !slices.Equal(got[:3], []string{"pane", "run", "pane-9"}) || !strings.Contains(got[3], `'/opt/a'"'"'jaj'`) {
		t.Fatalf("run args = %#v", got)
	}
	if got := runner.calls[3].args; !slices.Equal(got, []string{"pane", "close", "pane-9"}) {
		t.Fatalf("cleanup args = %#v", got)
	}
}

func TestCmuxLaunchTargetsNewSurfaceAndFocusesIt(t *testing.T) {
	runner := &fakeCommandRunner{responses: [][]byte{[]byte("OK surface=surface:5 pane=pane:3 workspace=workspace:1\n")}}
	client := &Client{
		kind: Cmux, binary: "/bin/cmux", runner: runner,
		env: map[string]string{"CMUX_WORKSPACE_ID": "workspace:1", "CMUX_SURFACE_ID": "surface:2"},
	}
	command := Command{Path: "/opt/ajaj", Args: []string{"run", "claude", "my profile"}, Dir: "/repo's root", Title: "ajaj: claude/my profile"}
	if err := client.Split(t.Context(), Right, command); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("calls = %#v, want create, rename, send, focus", runner.calls)
	}
	if got := runner.calls[0].args; !slices.Equal(got, []string{"new-pane", "--type", "terminal", "--direction", "right", "--workspace", "workspace:1"}) {
		t.Fatalf("create args = %#v", got)
	}
	if got := runner.calls[1].args; !slices.Equal(got, []string{"rename-tab", "--workspace", "workspace:1", "--surface", "surface:5", "ajaj: claude/my profile"}) {
		t.Fatalf("rename args = %#v", got)
	}
	if got := runner.calls[2].args; got[0] != "send" || got[4] != "surface:5" || !strings.Contains(got[5], `'my profile'`) || !strings.Contains(got[5], `'/repo'"'"'s root'`) || !strings.HasSuffix(got[5], "\n") {
		t.Fatalf("send args = %#v", got)
	}
	if got := runner.calls[3].args; !slices.Equal(got, []string{"focus-panel", "--workspace", "workspace:1", "--panel", "surface:5"}) {
		t.Fatalf("focus args = %#v", got)
	}
}

func TestCmuxCleansUpWhenSendingLaunchFails(t *testing.T) {
	runner := &fakeCommandRunner{
		responses: [][]byte{[]byte("OK surface=surface:8 pane=pane:4 workspace=workspace:1\n")},
		errors:    []error{nil, nil, errors.New("send failed")},
	}
	client := &Client{kind: Cmux, binary: "/bin/cmux", runner: runner, env: map[string]string{"CMUX_WORKSPACE_ID": "workspace:1"}}
	err := client.Split(t.Context(), Right, Command{Path: "/opt/ajaj", Args: []string{"run"}, Dir: "/repo", Title: "ajaj: claude/work"})
	if err == nil || !strings.Contains(err.Error(), "send failed") {
		t.Fatalf("Split() error = %v", err)
	}
	if len(runner.calls) != 4 || runner.calls[3].args[0] != "close-surface" {
		t.Fatalf("calls = %#v, want cleanup", runner.calls)
	}
}

func TestSplitValidatesInputAndResponses(t *testing.T) {
	client := &Client{kind: Herdr, binary: "/bin/herdr", runner: &fakeCommandRunner{responses: [][]byte{[]byte(`{}`)}}}
	if err := client.Split(t.Context(), Direction("left"), Command{Path: "ajaj", Dir: "/repo"}); err == nil {
		t.Fatal("Split() accepted unsupported direction")
	}
	if err := client.Split(t.Context(), Right, Command{Path: "ajaj", Dir: "/repo", Title: "ajaj: claude/work"}); err == nil || !strings.Contains(err.Error(), "pane ID") {
		t.Fatalf("Split() malformed response error = %v", err)
	}
}

func TestRenameCurrentUsesBackendNativeTarget(t *testing.T) {
	tests := []struct {
		name string
		kind Kind
		env  map[string]string
		want []string
	}{
		{"tmux", Tmux, map[string]string{"TMUX_PANE": "%4"}, []string{"select-pane", "-t", "%4", "-T", "ajaj: claude/work"}},
		{"zellij", Zellij, map[string]string{"ZELLIJ_SESSION_NAME": "work", "ZELLIJ_PANE_ID": "terminal_3"}, []string{"--session", "work", "action", "rename-pane", "--pane-id", "terminal_3", "ajaj: claude/work"}},
		{"herdr", Herdr, map[string]string{"HERDR_PANE_ID": "pane-2"}, []string{"pane", "rename", "pane-2", "ajaj: claude/work"}},
		{"cmux", Cmux, map[string]string{"CMUX_WORKSPACE_ID": "workspace:1", "CMUX_SURFACE_ID": "surface:2"}, []string{"rename-tab", "--workspace", "workspace:1", "--surface", "surface:2", "ajaj: claude/work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := new(fakeCommandRunner)
			client := &Client{kind: tt.kind, binary: "/bin/" + string(tt.kind), env: tt.env, runner: runner}
			if err := client.RenameCurrent(t.Context(), "ajaj: claude/work"); err != nil {
				t.Fatal(err)
			}
			if len(runner.calls) != 1 || !slices.Equal(runner.calls[0].args, tt.want) {
				t.Fatalf("calls = %#v, want %#v", runner.calls, tt.want)
			}
		})
	}
}

func TestClearCurrentTitleUsesBackendNativeReset(t *testing.T) {
	tests := []struct {
		name string
		kind Kind
		env  map[string]string
		want []string
	}{
		{"tmux", Tmux, map[string]string{"TMUX_PANE": "%4"}, []string{"select-pane", "-t", "%4", "-T", ""}},
		{"zellij", Zellij, map[string]string{"ZELLIJ_SESSION_NAME": "work", "ZELLIJ_PANE_ID": "terminal_3"}, []string{"--session", "work", "action", "undo-rename-pane", "--pane-id", "terminal_3"}},
		{"herdr", Herdr, map[string]string{"HERDR_PANE_ID": "pane-2"}, []string{"pane", "rename", "pane-2", "--clear"}},
		{"cmux", Cmux, map[string]string{"CMUX_WORKSPACE_ID": "workspace:1", "CMUX_SURFACE_ID": "surface:2"}, []string{"rename-tab", "--workspace", "workspace:1", "--surface", "surface:2", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := new(fakeCommandRunner)
			client := &Client{kind: tt.kind, binary: "/bin/" + string(tt.kind), env: tt.env, runner: runner}
			if err := client.ClearCurrentTitle(t.Context()); err != nil {
				t.Fatal(err)
			}
			if len(runner.calls) != 1 || !slices.Equal(runner.calls[0].args, tt.want) {
				t.Fatalf("calls = %#v, want %#v", runner.calls, tt.want)
			}
		})
	}
}
