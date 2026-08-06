package provider

import (
	"slices"
	"testing"
)

func TestEnvironmentReplacesExistingValue(t *testing.T) {
	got := Environment([]string{"PATH=/bin", "CODEX_HOME=/old", "LANG=en"}, "CODEX_HOME", "/new")
	if slices.Contains(got, "CODEX_HOME=/old") {
		t.Fatalf("Environment() retained old value: %v", got)
	}
	if !slices.Contains(got, "CODEX_HOME=/new") {
		t.Fatalf("Environment() omitted new value: %v", got)
	}
}
