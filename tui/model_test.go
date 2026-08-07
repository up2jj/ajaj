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
	if got.Selection != nil {
		t.Fatalf("Selection = %#v, want no launch", got.Selection)
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
	if got.Selection != nil || got.DefaultRequested != nil {
		t.Fatalf("delete also requested another action: %#v", got)
	}
}

func TestEnterLaunchesImmediatelyWithoutMultiplexer(t *testing.T) {
	model := New([]account.Account{{Provider: account.Codex, Name: "work"}}, nil)
	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	got := updated.(Model)
	if cmd == nil || got.Selection == nil {
		t.Fatalf("enter result = %#v, command %v", got.Selection, cmd)
	}
	if got.Selection.Account.ID() != "codex/work" || got.Selection.Destination != CurrentPane {
		t.Fatalf("Selection = %#v", got.Selection)
	}
}

func TestMultiplexerUsesTwoStepDestinationPicker(t *testing.T) {
	model := NewWithMultiplexer([]account.Account{{Provider: account.Claude, Name: "work"}}, nil, "tmux")
	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(Model)
	if cmd != nil || model.pending == nil || model.Selection != nil {
		t.Fatalf("account enter = pending:%#v selection:%#v command:%v", model.pending, model.Selection, cmd)
	}
	if view := model.View().Content; !strings.Contains(view, "Open claude/work with tmux") || !strings.Contains(view, "Open in current pane") {
		t.Fatalf("destination view = %q", view)
	}

	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	model = updated.(Model)
	updated, cmd = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	got := updated.(Model)
	if cmd == nil || got.Selection == nil || got.Selection.Destination != SplitRight {
		t.Fatalf("destination enter = selection:%#v command:%v", got.Selection, cmd)
	}
}

func TestDestinationEscapeReturnsToAccounts(t *testing.T) {
	model := NewWithMultiplexer([]account.Account{{Provider: account.Claude, Name: "work"}}, nil, "zellij")
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	got := updated.(Model)
	if cmd != nil || got.pending != nil || got.Selection != nil {
		t.Fatalf("escape result = pending:%#v selection:%#v command:%v", got.pending, got.Selection, cmd)
	}
	if !strings.Contains(got.View().Content, "choose an AI coding account") {
		t.Fatalf("account view not restored: %q", got.View().Content)
	}
}

func TestAccountActionsAreIgnoredOnDestinationScreen(t *testing.T) {
	model := NewWithMultiplexer([]account.Account{{Provider: account.Claude, Name: "work"}}, nil, "herdr")
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(Model)
	for _, key := range []rune{'d', 'x'} {
		updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: string(key), Code: key}))
		got := updated.(Model)
		if cmd != nil || got.DefaultRequested != nil || got.DeleteRequested != nil {
			t.Fatalf("key %q triggered account action: %#v", key, got)
		}
	}
}
