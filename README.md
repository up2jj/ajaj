# ajaj

`ajaj` is a small Go CLI/TUI for keeping multiple Claude Code and Codex
accounts isolated and launching either tool with the right account.

It uses Cobra for a scriptable command surface and Bubble Tea for the default
interactive account picker.

## Install

With Homebrew:

```sh
brew install up2jj/tap/ajaj
```

Or install the latest source with Go:

```sh
go install github.com/up2jj/ajaj@latest
```

## Design

The application does not read, copy, or interpret credentials. Every account
gets its own provider home directory and the provider CLI owns authentication:

```text
                         ┌─ CLAUDE_CONFIG_DIR=.../claude/work ──> claude
command / Bubble Tea ────┤
                         └─ CODEX_HOME=.../codex/personal ──────> codex
```

- Claude Code isolation uses its supported `CLAUDE_CONFIG_DIR` variable.
- Codex isolation uses `CODEX_HOME`. Each Codex account home is initialized
  with `cli_auth_credentials_store = "file"`, so credentials stay inside that
  isolated home instead of collapsing back into one OS-keychain entry.
- The registry contains only provider, display name, path, preferred selection,
  and the most recently selected profile.
  It is written atomically with mode `0600`; profile directories use `0700`.
- Provider-specific behavior lives behind a small adapter, so another coding
  agent can be added without coupling it to Cobra or Bubble Tea.

Storage follows the XDG base-directory split:

- Registry: `$XDG_CONFIG_HOME/ajaj/accounts.json`. When `XDG_CONFIG_HOME` is
  unset, `os.UserConfigDir` supplies the platform default.
- Account profiles: `$XDG_DATA_HOME/ajaj/profiles`. When `XDG_DATA_HOME` is
  unset, the fallback is `~/.local/share/ajaj/profiles`.
- Usage snapshots: `$XDG_STATE_HOME/ajaj/usage`. When `XDG_STATE_HOME` is
  unset, the fallback is `~/.local/state/ajaj/usage`.

For example, to keep all ajaj data below a custom XDG root:

```sh
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_STATE_HOME="$HOME/.local/state"
```

## Commands

```sh
# Create isolated profiles, then authenticate them with the real provider CLI.
ajaj account add claude personal --login
ajaj account add claude work --login
ajaj account add codex personal --login

# Inspect profiles and change the preferred profile for each provider.
ajaj account list
ajaj account use claude work
ajaj account current

# Usage-aware selection is enabled by default at 90%.
ajaj account auto on --threshold 85
ajaj account auto off

# Inspect cached usage or explicitly refresh Codex usage.
ajaj usage
ajaj usage refresh codex

# Forward every remaining argument using the preferred or auto-selected profile.
ajaj claude --model opus
ajaj codex --full-auto

# Run an explicit account without changing the preferred selection.
ajaj run claude personal --model sonnet

# Open the interactive picker and launch the selected account.
ajaj
```

## Automatic selection

`ajaj claude ...` and `ajaj codex ...` choose a profile immediately before
starting the provider process. The configured preferred profile remains
unchanged. When its fresh usage snapshot reaches the threshold, `ajaj`
uses the fresh, below-threshold profile with the lowest usage for that launch.
It does not switch during a running session, alter the preferred profile, or
retry a failed prompt under another identity.

- Codex usage is refreshed through the supported `codex app-server`
  `account/rateLimits/read` API before selection. If refresh fails, `ajaj`
  reports a warning and uses a fresh cached snapshot when one exists.
- Claude Code exposes subscription usage to status-line commands after the
  first API response. New Claude profiles therefore receive a small collector
  in their isolated `settings.json`; it records the five-hour and seven-day
  windows for the next launch. Existing status-line configuration is never
  overwritten. For an older profile with no status line, run
  `ajaj usage install claude <name>`; for a custom status line, incorporate
  `ajaj usage ingest claude <name>` into that command yourself.
- An explicit `ajaj run <provider> <name> ...` always uses the named profile
  and bypasses automatic selection.

The `ajaj` executable must be on `PATH` for Claude's status-line collector.
Use this only with accounts you own or are authorized to operate, and within
the providers' applicable terms and organizational policies.

## Identifying the running profile

Every provider launch prints the selected profile before the provider starts:

```text
ajaj: running claude/work
```

The child process also receives `AJAJ_PROVIDER`, `AJAJ_PROFILE`, and
`AJAJ_ACCOUNT` (for example, `claude`, `work`, and `claude/work`). Claude's
installed status-line collector displays the profile continuously:

```text
[ajaj claude/work] 5h 31% · 7d 64%
```

Custom Claude status lines can include `$AJAJ_ACCOUNT`. For Codex, `ajaj` uses
a best-effort terminal title such as `ajaj · codex/personal` while the process
is running and restores the previous title afterward. Terminal-title escapes
are never written when output is redirected.

Use `ajaj account current [claude|codex]` from another terminal to distinguish
the configured preferred profile from the most recently selected launch:

```text
claude   preferred=work  last-selected=personal
```

These labels identify the isolated ajaj profile, not the remote identity.
Inside either provider, use `/status` to verify the authenticated account.

For Claude, an empty isolated directory causes the normal first-run login flow.
For Codex, `ajaj login codex <name>` runs `codex login` in that account home.

## Build and verify

```sh
go build ./...
go test ./...
go vet ./...
```

Common tasks are also available through the [`justfile`](justfile) (install
`just` with `brew install just`):

```sh
just                 # list recipes
just build           # build ./ajaj
just install         # install with the current git version
just test            # run tests
just ci              # formatting, vet, tests, and GoReleaser validation
just snapshot        # build a local release into ./dist
just release 0.1.0   # tag and push v0.1.0
```

## Release

Releases are automated with [GoReleaser](https://goreleaser.com). Add a
fine-grained GitHub personal access token as the repository secret
`HOMEBREW_TAP_GITHUB_TOKEN`; it needs Contents read/write access to
`up2jj/homebrew-tap`.

Validate a release locally with:

```sh
goreleaser check
goreleaser build --snapshot --clean
```

Pushing a semantic `v*` tag publishes the GitHub release and updates the
`ajaj` cask in `up2jj/homebrew-tap`:

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

## Deliberate MVP boundaries

- The app manages terminal CLIs, not their IDE extensions or desktop apps.
- Account deletion is omitted until it has an explicit confirmation and a
  recoverable trash-based implementation.
- The registry is safe against partial writes, but simultaneous mutations from
  multiple `ajaj` processes are not serialized yet.
- Usage is read only from provider-supported surfaces and cached as local
  metadata; the app does not inspect or decode credential/token formats.
