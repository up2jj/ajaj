// Package tui implements ajaj's interactive account picker.
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/up2jj/ajaj/account"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

type Model struct {
	accounts []account.Account
	active   map[account.Provider]string
	cursor   int
	width    int
	Selected *account.Account
}

func New(accounts []account.Account, active map[account.Provider]string) Model {
	return Model{accounts: accounts, active: active}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.accounts)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.accounts) > 0 {
				selected := m.accounts[m.cursor]
				m.Selected = &selected
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString(titleStyle.Render("ajaj — choose an AI coding account"))
	b.WriteString("\n\n")
	for i, a := range m.accounts {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "› "
			style = selectedStyle
		}
		active := ""
		if m.active[a.Provider] == a.Name {
			active = dimStyle.Render("  active")
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%-8s %s", cursor, a.Provider, a.Name)))
		b.WriteString(active)
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑/k ↓/j move  •  enter launch  •  q quit"))
	if m.width > 0 && m.width < 50 {
		b.WriteString("\n")
	}
	view := tea.NewView(b.String())
	view.AltScreen = true
	view.WindowTitle = "ajaj"
	return view
}
