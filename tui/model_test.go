package tui

import (
	"strings"
	"testing"

	"github.com/up2jj/ajaj/account"
)

func TestViewMarksConfiguredDefaultAccount(t *testing.T) {
	model := New(
		[]account.Account{
			{Provider: account.Claude, Name: "personal"},
			{Provider: account.Claude, Name: "work"},
		},
		map[account.Provider]string{account.Claude: "work"},
	)

	view := model.View().Content
	if got := strings.Count(view, "default"); got != 1 {
		t.Fatalf("default label count = %d, want 1; view = %q", got, view)
	}
	if strings.Contains(view, "  active") {
		t.Fatalf("view contains old active label: %q", view)
	}
}
