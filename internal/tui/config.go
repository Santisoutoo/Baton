package tui

import (
	"fmt"
	"strings"

	"baton/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

func (m MainModel) viewConfig() string {
	var b strings.Builder

	b.WriteString(HeadingStyle.Render("Configuration"))
	b.WriteString("\n")

	if m.configNav.detail != nil {
		return m.viewConfigDetail()
	}

	if m.configNav.message != "" {
		b.WriteString(DimmedStyle.Render("  " + m.configNav.message))
		b.WriteString("\n\n")
	}

	for i, sec := range m.configNav.sections {
		if i == m.configNav.cursor {
			b.WriteString(SelectedStyle.Render(" " + sec.title + " "))
		} else {
			b.WriteString(DimmedStyle.Render("   " + sec.title))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(HelpStyle.Render("enter view section   esc back"))

	return b.String()
}

func (m MainModel) viewConfigDetail() string {
	var b strings.Builder

	sec := m.configNav.sections[m.configNav.cursor]
	b.WriteString(DimmedStyle.Render("  Section: " + sec.title))
	b.WriteString("\n\n")

	for i, entry := range m.configNav.detail {
		if m.configNav.editing && i == m.configNav.detailCur {
			b.WriteString(fmt.Sprintf("  %s: ", entry.key))
			b.WriteString(m.configNav.input.View())
			b.WriteString("\n")
		} else {
			prefix := "   "
			if i == m.configNav.detailCur {
				prefix = SelectedStyle.Render(" > ")
			}
			b.WriteString(fmt.Sprintf("%s%s: %s\n", prefix, entry.key, DimmedStyle.Render(entry.value)))
		}
	}

	b.WriteString("\n")
	if m.configNav.editing {
		b.WriteString(HelpStyle.Render("enter confirm   esc cancel"))
	} else {
		b.WriteString(HelpStyle.Render("enter edit   ↑↓ navigate   esc back"))
	}

	return b.String()
}

func (m *MainModel) handleConfigKey(msg tea.KeyMsg) tea.Cmd {
	if m.configNav.editing {
		switch msg.String() {
		case "enter":
			return m.confirmConfigEdit()
		case "esc":
			m.configNav.editing = false
			m.configNav.input.Reset()
			return nil
		default:
			var cmd tea.Cmd
			m.configNav.input, cmd = m.configNav.input.Update(msg)
			return cmd
		}
	}

	if m.configNav.detail != nil {
		switch msg.String() {
		case "up", "k":
			m.configNav.detailCur = clamp(m.configNav.detailCur-1, 0, len(m.configNav.detail)-1)
		case "down", "j":
			m.configNav.detailCur = clamp(m.configNav.detailCur+1, 0, len(m.configNav.detail)-1)
		case "enter":
			return m.startConfigEdit()
		case "esc", "backspace":
			m.configNav.detail = nil
			m.configNav.detailCur = 0
			return nil
		}
		return nil
	}

	switch msg.String() {
	case "up", "k":
		m.configNav.cursor = clamp(m.configNav.cursor-1, 0, len(m.configNav.sections)-1)
	case "down", "j":
		m.configNav.cursor = clamp(m.configNav.cursor+1, 0, len(m.configNav.sections)-1)
	case "enter":
		m.configNav.detail = m.loadConfigSection(m.configNav.sections[m.configNav.cursor].name)
		m.configNav.detailCur = 0
		return nil
	}
	return nil
}

func (m *MainModel) loadConfigSection(name string) []configEntry {
	switch name {
	case "general":
		return []configEntry{
			{key: "port", value: fmt.Sprintf("%d", m.cfg.Port)},
			{key: "log_level", value: m.cfg.LogLevel},
			{key: "on_exec_error", value: m.cfg.OnExecError},
			{key: "default_role", value: m.cfg.DefaultRole},
		}
	case "roles":
		var entries []configEntry
		for k, v := range m.cfg.Roles {
			entries = append(entries, configEntry{
				key:   k,
				value: fmt.Sprintf("backend=%s model=%s", v.Backend, v.Model),
			})
		}
		return entries
	case "backends":
		var entries []configEntry
		for k, v := range m.cfg.Backends {
			entries = append(entries, configEntry{
				key:   k,
				value: fmt.Sprintf("type=%s auth=%s url=%s", v.Type, v.Auth, previewStr(v.BaseURL, 30)),
			})
		}
		return entries
	case "tiers":
		var entries []configEntry
		for k, v := range m.cfg.Tiers {
			entries = append(entries, configEntry{key: k, value: v})
		}
		return entries
	case "prices":
		var entries []configEntry
		for k, v := range m.cfg.Prices {
			entries = append(entries, configEntry{
				key:   k,
				value: fmt.Sprintf("in=%.4f out=%.4f", v.In, v.Out),
			})
		}
		if len(entries) == 0 {
			entries = append(entries, configEntry{key: "(none)", value: "no prices configured"})
		}
		return entries
	}
	return nil
}

func (m *MainModel) startConfigEdit() tea.Cmd {
	if len(m.configNav.detail) == 0 {
		return nil
	}
	entry := m.configNav.detail[m.configNav.detailCur]
	if entry.key == "(none)" {
		return nil
	}
	m.configNav.editing = true
	m.configNav.input.Reset()
	m.configNav.input.SetValue(entry.value)
	m.configNav.input.Focus()
	return textinput.Blink
}

func (m *MainModel) confirmConfigEdit() tea.Cmd {
	val := m.configNav.input.Value()
	sec := m.configNav.sections[m.configNav.cursor]
	entry := &m.configNav.detail[m.configNav.detailCur]

	return func() tea.Msg {
		switch sec.name {
		case "general":
			switch entry.key {
			case "port":
				var p int
				fmt.Sscanf(val, "%d", &p)
				if p > 0 {
					m.cfg.Port = p
				}
			case "log_level":
				m.cfg.LogLevel = val
			case "on_exec_error":
				m.cfg.OnExecError = val
			case "default_role":
				m.cfg.DefaultRole = val
			}
		case "roles":
			if val == "" {
				break
			}
			parts := strings.Fields(val)
			for _, p := range parts {
				if strings.HasPrefix(p, "model=") {
					rc := m.cfg.Roles[entry.key]
					rc.Model = strings.TrimPrefix(p, "model=")
					m.cfg.Roles[entry.key] = rc
				}
				if strings.HasPrefix(p, "backend=") {
					rc := m.cfg.Roles[entry.key]
					rc.Backend = strings.TrimPrefix(p, "backend=")
					m.cfg.Roles[entry.key] = rc
				}
			}
		}
		entry.value = val
		m.configNav.editing = false
		config.Save(m.cfg, config.GlobalPath())
		m.configNav.message = fmt.Sprintf("Saved %s.%s", sec.name, entry.key)
		return nil
	}
}


