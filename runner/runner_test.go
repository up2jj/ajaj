package runner

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/up2jj/ajaj/account"
)

func TestSessionEnvironmentIdentifiesProfile(t *testing.T) {
	a := account.Account{Provider: account.Codex, Name: "work", Home: "/profiles/work"}
	got := sessionEnvironment([]string{
		"PATH=/bin", "CODEX_HOME=/old", "AJAJ_PROVIDER=old", "AJAJ_PROFILE=old", "AJAJ_ACCOUNT=old/old",
	}, "CODEX_HOME", a)
	for _, want := range []string{
		"CODEX_HOME=/profiles/work", "AJAJ_PROVIDER=codex", "AJAJ_PROFILE=work", "AJAJ_ACCOUNT=codex/work",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("environment omitted %q: %v", want, got)
		}
	}
	for _, old := range []string{"CODEX_HOME=/old", "AJAJ_PROVIDER=old", "AJAJ_PROFILE=old", "AJAJ_ACCOUNT=old/old"} {
		if slices.Contains(got, old) {
			t.Errorf("environment retained %q: %v", old, got)
		}
	}
}

func TestRunShowsProfileAndExportsIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nprintf '%s|%s|%s' \"$AJAJ_PROVIDER\" \"$AJAJ_PROFILE\" \"$AJAJ_ACCOUNT\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	var out, errOut bytes.Buffer
	r := Runner{Out: &out, Err: &errOut, Env: []string{"PATH=" + dir}, IsTerminal: func(io.Writer) bool { return false }}
	a := account.Account{Provider: account.Claude, Name: "work", Home: filepath.Join(dir, "home")}
	if err := r.Run(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "claude|work|claude/work" {
		t.Fatalf("provider environment = %q", got)
	}
	if got := errOut.String(); got != "ajaj: running claude/work\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRunSetsAndRestoresTerminalTitle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "codex")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	var errOut bytes.Buffer
	r := Runner{Err: &errOut, Env: []string{"PATH=" + dir}, IsTerminal: func(io.Writer) bool { return true }}
	a := account.Account{Provider: account.Codex, Name: "personal", Home: filepath.Join(dir, "home")}
	if err := r.Run(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	got := errOut.String()
	for _, want := range []string{"ajaj: running codex/personal", "\x1b]0;ajaj · codex/personal\x07", "\x1b[23;0t"} {
		if !strings.Contains(got, want) {
			t.Errorf("terminal output omitted %q: %q", want, got)
		}
	}
}

func TestRunRestoresTerminalTitleWhenLaunchIsCanceled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var errOut bytes.Buffer
	r := Runner{Err: &errOut, Env: []string{"PATH=" + dir}, IsTerminal: func(io.Writer) bool { return true }}
	err := r.Run(ctx, account.Account{Provider: account.Codex, Name: "work", Home: filepath.Join(dir, "home")})
	if err == nil {
		t.Fatal("Run() succeeded with a canceled context")
	}
	if !strings.HasSuffix(errOut.String(), "\x1b[23;0t") {
		t.Fatalf("terminal title was not restored: %q", errOut.String())
	}
}
