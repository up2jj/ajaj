package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	if got := strings.Count(view, "  default"); got != 1 {
		t.Fatalf("default label count = %d, want 1; view = %q", got, view)
	}
	if strings.Contains(view, "  active") {
		t.Fatalf("view contains old active label: %q", view)
	}
}

func TestDefaultKeyRequestsDefaultWithoutLaunch(t *testing.T) {
	model := New(
		[]account.Account{
			{Provider: account.Claude, Name: "personal"},
			{Provider: account.Claude, Name: "work"},
		},
		map[account.Provider]string{account.Claude: "personal"},
	)
	model.cursor = 1

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "d", Code: 'd'}))
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("default key did not quit the picker")
	}
	if got.DefaultRequested == nil || got.DefaultRequested.Name != "work" {
		t.Fatalf("DefaultRequested = %#v, want work", got.DefaultRequested)
	}
	if got.Selected != nil {
		t.Fatalf("Selected = %#v, want no launch", got.Selected)
	}
}

func TestViewExplainsDefaultKey(t *testing.T) {
	view := New(nil, nil).View().Content
	if !strings.Contains(view, "d set default") {
		t.Fatalf("view does not explain default key: %q", view)
	}
	if !strings.Contains(view, "x delete") {
		t.Fatalf("view does not explain delete key: %q", view)
	}
}

func TestDeleteKeyRequestsDeleteWithoutLaunch(t *testing.T) {
	model := New(
		[]account.Account{{Provider: account.Claude, Name: "work"}},
		map[account.Provider]string{account.Claude: "work"},
	)

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("delete key did not quit the picker")
	}
	if got.DeleteRequested == nil || got.DeleteRequested.Name != "work" {
		t.Fatalf("DeleteRequested = %#v, want work", got.DeleteRequested)
	}
	if got.Selected != nil || got.DefaultRequested != nil {
		t.Fatalf("delete also requested another action: %#v", got)
	}
}
