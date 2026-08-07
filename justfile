# ajaj task runner — run `just` to list recipes.

# Show available recipes.
default:
    @just --list

# Build the binary into ./ajaj.
build:
    go build -o ajaj ./

# Install ajaj into $GOBIN (or $GOPATH/bin) with a version from git.
install:
    go install -ldflags "-X github.com/up2jj/ajaj/cmd.version=$(git describe --tags --always)" ./
    @echo "installed ajaj to $(go env GOBIN GOPATH | awk 'NR==1{b=$0} NR==2{p=$0} END{print (b!="" ? b : p"/bin")"/ajaj"}')"

# Run the test suite.
test:
    go test ./...

# Run go vet.
vet:
    go vet ./...

# Format all Go sources.
fmt:
    gofmt -w .

# Check that all Go sources are formatted.
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    files="$(gofmt -l .)"
    if [ -n "$files" ]; then
        echo "Unformatted files:" >&2
        echo "$files" >&2
        exit 1
    fi

# Tidy module dependencies.
tidy:
    go mod tidy

# Install git hooks via prek (pre-commit + pre-push).
hooks:
    prek install --hook-type pre-commit --hook-type pre-push

# Run ajaj (pass provider arguments after --, e.g. `just run -- claude --model opus`).
run *args:
    go run . {{ args }}

# Run the same core checks used by CI.
ci: fmt-check vet test check

# Validate the GoReleaser configuration.
check:
    goreleaser check

# Build a local release into ./dist without publishing.
snapshot:
    goreleaser release --snapshot --clean --skip=publish

# Tag and push a release, triggering the GitHub Actions release workflow.
# Usage: just release 0.1.0   (creates and pushes tag v0.1.0)
release version:
    #!/usr/bin/env bash
    set -euo pipefail
    version="{{ version }}"
    version="${version#v}"
    tag="v${version}"
    if [ -n "$(git status --porcelain)" ]; then
        echo "error: working tree is not clean; commit or stash changes first" >&2
        exit 1
    fi
    if git rev-parse "$tag" >/dev/null 2>&1; then
        echo "error: tag $tag already exists" >&2
        exit 1
    fi
    echo "Running pre-release checks..."
    go test ./...
    go vet ./...
    goreleaser check
    echo "Tagging and pushing $tag..."
    git tag -a "$tag" -m "Release $tag"
    git push origin "$tag"
    echo "Pushed $tag — the release workflow will now build and publish it."
