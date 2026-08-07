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
	accounts          []account.Account
	defaults          map[account.Provider]string
	cursor            int
	destinationCursor int
	width             int
	multiplexer       string
	pending           *account.Account
	Selection         *LaunchSelection
	DefaultRequested  *account.Account
	DeleteRequested   *account.Account
}

type Destination string

const (
	CurrentPane Destination = "current"
	SplitRight  Destination = "right"
	SplitDown   Destination = "down"
)

type LaunchSelection struct {
	Account     account.Account
	Destination Destination
}

var destinations = []Destination{CurrentPane, SplitRight, SplitDown}

func New(accounts []account.Account, defaults map[account.Provider]string) Model {
	return NewWithMultiplexer(accounts, defaults, "")
}

func NewWithMultiplexer(accounts []account.Account, defaults map[account.Provider]string, multiplexer string) Model {
	return Model{accounts: accounts, defaults: defaults, multiplexer: multiplexer}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.pending != nil {
				m.pending = nil
				m.destinationCursor = 0
				return m, nil
			}
			return m, tea.Quit
		case "up", "k":
			if m.pending != nil && m.destinationCursor > 0 {
				m.destinationCursor--
			} else if m.pending == nil && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.pending != nil && m.destinationCursor < len(destinations)-1 {
				m.destinationCursor++
			} else if m.pending == nil && m.cursor < len(m.accounts)-1 {
				m.cursor++
			}
		case "enter":
			if m.pending != nil {
				m.Selection = &LaunchSelection{Account: *m.pending, Destination: destinations[m.destinationCursor]}
				return m, tea.Quit
			}
			if len(m.accounts) > 0 && m.multiplexer != "" {
				selected := m.accounts[m.cursor]
				m.pending = &selected
				m.destinationCursor = 0
				return m, nil
			}
			if len(m.accounts) > 0 {
				m.Selection = &LaunchSelection{Account: m.accounts[m.cursor], Destination: CurrentPane}
				return m, tea.Quit
			}
		case "d":
			if m.pending == nil && len(m.accounts) > 0 {
				selected := m.accounts[m.cursor]
				m.DefaultRequested = &selected
				return m, tea.Quit
			}
		case "x":
			if m.pending == nil && len(m.accounts) > 0 {
				selected := m.accounts[m.cursor]
				m.DeleteRequested = &selected
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	var b strings.Builder
	if m.pending != nil {
		m.writeDestinationView(&b)
	} else {
		m.writeAccountView(&b)
	}
	if m.width > 0 && m.width < 50 {
		b.WriteString("\n")
	}
	view := tea.NewView(b.String())
	view.AltScreen = true
	view.WindowTitle = "ajaj"
	return view
}

func (m Model) writeAccountView(b *strings.Builder) {
	b.WriteString(titleStyle.Render("ajaj — choose an AI coding account"))
	b.WriteString("\n\n")
	for i, a := range m.accounts {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "› "
			style = selectedStyle
		}
		defaultLabel := ""
		if m.defaults[a.Provider] == a.Name {
			defaultLabel = dimStyle.Render("  default")
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%-8s %s", cursor, a.Provider, a.Name)))
		b.WriteString(defaultLabel)
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	launchLabel := "launch"
	if m.multiplexer != "" {
		launchLabel = "choose destination"
	}
	b.WriteString(dimStyle.Render("↑/k ↓/j move  •  enter " + launchLabel + "  •  d set default  •  x delete  •  q quit"))
}

func (m Model) writeDestinationView(b *strings.Builder) {
	b.WriteString(titleStyle.Render(fmt.Sprintf("Open %s with %s", m.pending.ID(), m.multiplexer)))
	b.WriteString("\n\n")
	for i, destination := range destinations {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.destinationCursor {
			cursor = "› "
			style = selectedStyle
		}
		b.WriteString(style.Render(cursor + destination.Label()))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑/k ↓/j move  •  enter launch  •  esc accounts  •  q quit"))
}

func (d Destination) Label() string {
	switch d {
	case CurrentPane:
		return "Open in current pane"
	case SplitRight:
		return "Split pane right"
	case SplitDown:
		return "Split pane down"
	default:
		return string(d)
	}
}
