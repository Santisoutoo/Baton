package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

func (m MainModel) viewCredentials() string {
	var b strings.Builder

	b.WriteString(HeadingStyle.Render("Manage Credentials"))
	b.WriteString("\n")

	if m.credsMgr.message != "" {
		switch m.credsMgr.msgType {
		case "success":
			b.WriteString(SuccessStyle.Render("  " + m.credsMgr.message))
		case "error":
			b.WriteString(ErrorStyle.Render("  " + m.credsMgr.message))
		default:
			b.WriteString(DimmedStyle.Render("  " + m.credsMgr.message))
		}
		b.WriteString("\n\n")
	}

	b.WriteString(DimmedStyle.Render("  Services:"))
	b.WriteString("\n")

	items := m.credsMgr.services
	hasCustom := false
	for _, svc := range items {
		if svc.name == "__custom__" {
			hasCustom = true
			break
		}
	}
	if !hasCustom {
		items = append(items, struct {
			name   string
			status string
		}{name: "__custom__", status: "custom"})
	}

	for i, svc := range items {
		var status string
		switch svc.status {
		case "set":
			status = SetBadge
		case "custom":
			status = DimmedStyle.Render("+ add custom")
		default:
			status = MissingBadge
		}

		label := svc.name
		if label == "__custom__" {
			label = "+ Custom service..."
		}

		line := fmt.Sprintf("  %-20s %s", label, status)
		if i == m.credsMgr.cursor {
			line = SelectedStyle.Render(" " + strings.TrimSpace(line) + " ")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")

	if m.credsMgr.inputMode {
		switch {
		case m.credsMgr.inputSvc == "":
			b.WriteString(DimmedStyle.Render("  Service name:"))
			b.WriteString("\n  " + m.credsMgr.input.View())
			b.WriteString("\n")
			b.WriteString(DimmedStyle.Render("  enter confirm   esc cancel"))
		default:
			b.WriteString(fmt.Sprintf("  Key for %s:\n", SuccessStyle.Render(m.credsMgr.inputSvc)))
			b.WriteString("  " + m.credsMgr.input.View())
			b.WriteString("\n")
			b.WriteString(DimmedStyle.Render("  enter confirm   esc cancel"))
		}
	} else {
		b.WriteString(HelpStyle.Render("a add key   d delete key   ↑↓ navigate   esc back"))
	}

	return b.String()
}

func (m *MainModel) handleCredentialsKey(msg tea.KeyMsg) tea.Cmd {
	if m.credsMgr.inputMode {
		switch msg.String() {
		case "enter":
			return m.confirmCredInput()
		case "esc":
			m.credsMgr.inputMode = false
			m.credsMgr.inputSvc = ""
			m.credsMgr.input.Reset()
			m.credsMgr.message = ""
			return nil
		default:
			var cmd tea.Cmd
			m.credsMgr.input, cmd = m.credsMgr.input.Update(msg)
			return cmd
		}
	}

	total := len(m.credsMgr.services)
	if total == 0 {
		total = 1
	}

	switch msg.String() {
	case "up", "k":
		m.credsMgr.cursor = clamp(m.credsMgr.cursor-1, 0, total)
	case "down", "j":
		m.credsMgr.cursor = clamp(m.credsMgr.cursor+1, 0, total)
	case "a":
		return m.startCredAdd()
	case "d":
		return m.startCredDelete()
	}
	return nil
}

func (m *MainModel) startCredAdd() tea.Cmd {
	idx := m.credsMgr.cursor
	svcs := m.credsMgr.services

	if idx < len(svcs) && svcs[idx].name == "__custom__" {
		m.credsMgr.inputMode = true
		m.credsMgr.inputSvc = ""
		m.credsMgr.input.Reset()
		m.credsMgr.input.Placeholder = "e.g. openrouter, anthropic..."
		m.credsMgr.input.Focus()
		m.credsMgr.message = ""
		return textinput.Blink
	}

	if idx >= len(svcs) {
		return nil
	}

	svc := svcs[idx]
	m.credsMgr.inputMode = true
	m.credsMgr.inputSvc = svc.name
	m.credsMgr.input.Reset()
	m.credsMgr.input.Placeholder = "API key for " + svc.name + "..."
	m.credsMgr.input.Focus()
	m.credsMgr.message = ""
	return textinput.Blink
}

func (m *MainModel) startCredDelete() tea.Cmd {
	idx := m.credsMgr.cursor
	svcs := m.credsMgr.services
	if idx >= len(svcs) || svcs[idx].name == "__custom__" {
		return nil
	}
	svc := svcs[idx]
	m.creds.Delete(svc.name)
	m.rebuildCredServices()
	m.credsMgr.message = fmt.Sprintf("Deleted key for %s", svc.name)
	m.credsMgr.msgType = "success"
	return nil
}

func (m *MainModel) confirmCredInput() tea.Cmd {
	if m.credsMgr.inputSvc == "" {
		svc := strings.TrimSpace(m.credsMgr.input.Value())
		if svc == "" {
			m.credsMgr.message = "Service name cannot be empty"
			m.credsMgr.msgType = "error"
			return nil
		}
		m.credsMgr.inputSvc = svc
		m.credsMgr.input.Reset()
		m.credsMgr.input.Placeholder = "API key for " + svc + "..."
		m.credsMgr.input.Focus()
		return textinput.Blink
	}

	val := m.credsMgr.input.Value()
	if val == "" {
		m.credsMgr.message = "Key cannot be empty"
		m.credsMgr.msgType = "error"
		return nil
	}
	return func() tea.Msg {
		err := m.creds.Set(m.credsMgr.inputSvc, val)
		if err != nil {
			return credSetMsg{err: err.Error()}
		}
		return credSetMsg{svc: m.credsMgr.inputSvc}
	}
}

type credSetMsg struct {
	svc string
	err string
}
