package tui

import (
	"fmt"
	"strings"

	"baton/internal/config"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

func (m MainModel) viewModels() string {
	var b strings.Builder

	b.WriteString(HeadingStyle.Render("Select Models"))
	b.WriteString("\n")

	planW := 28
	execW := 40
	if m.width > 80 {
		planW = (m.width - 8) / 2
		execW = (m.width - 8) / 2
	}

	planBox := renderModelPanel(
		"Plan lane (Claude subscription)",
		m.models.planModels,
		m.models.planCursor,
		m.models.planModel,
		0,
		planW,
	)

	execBox := renderExecPanel(
		m,
		execW,
	)

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, planBox, "  ", execBox))
	b.WriteString("\n\n")

	planM := m.models.planModel
	if planM == "" {
		planM = "opus (auto-detected from Claude tier)"
	}
	execM := m.models.execModel
	if execM == "" {
		execM = "none set"
	}
	b.WriteString(DimmedStyle.Render("  Current: "))
	b.WriteString(fmt.Sprintf("plan=%s", SuccessStyle.Render(planM)))
	b.WriteString(DimmedStyle.Render("   exec="))
	b.WriteString(fmt.Sprintf("%s", SuccessStyle.Render(execM)))
	b.WriteString("\n\n")
	b.WriteString(HelpStyle.Render("tab switch panel   ↑↓ navigate   enter select   s save   esc back"))

	return b.String()
}

func renderModelPanel(title string, models []string, cursor int, selected string, panel int, width int) string {
	var b strings.Builder
	b.WriteString(BorderStyle.Width(width).Render(title))
	b.WriteString("\n")
	for i, model := range models {
		marker := "  "
		suffix := ""
		if model == selected {
			marker = SuccessStyle.Render("● ")
		} else {
			marker = "○ "
		}
		if i == cursor {
			model = SelectedStyle.Render(" " + model + " ")
		} else {
			model = DimmedStyle.Render(" " + model)
		}

		line := fmt.Sprintf(" %s%s%s", marker, model, suffix)
		if len(line) > width-2 {
			line = line[:width-2]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func renderExecPanel(m MainModel, width int) string {
	var b strings.Builder
	b.WriteString(BorderStyle.Width(width).Render("Execute lane (OpenCode)"))
	b.WriteString("\n")

	if m.models.loading {
		b.WriteString(DimmedStyle.Render("  Fetching catalog..."))
		b.WriteString("\n")
		return b.String()
	}

	if m.models.fetchErr != "" {
		b.WriteString(ErrorStyle.Render("  " + m.models.fetchErr))
		b.WriteString("\n")
		b.WriteString(DimmedStyle.Render("  (login first: baton login opencode)"))
		b.WriteString("\n")
		return b.String()
	}

	if len(m.models.execModels) == 0 {
		b.WriteString(DimmedStyle.Render("  No models found"))
		b.WriteString("\n")
		return b.String()
	}

	for i, model := range m.models.execModels {
		marker := "  "
		if model == m.models.execModel {
			marker = SuccessStyle.Render("● ")
		} else {
			marker = "○ "
		}
		if i == m.models.execCursor {
			model = SelectedStyle.Render(" " + model + " ")
		} else {
			model = DimmedStyle.Render(" " + model)
		}
		line := fmt.Sprintf(" %s%s", marker, model)
		if len(line) > width-2 {
			line = line[:width-2]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func (m *MainModel) handleModelsKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "tab":
		if m.models.activePanel == 0 {
			m.models.activePanel = 1
		} else {
			m.models.activePanel = 0
		}
	case "up", "k":
		if m.models.activePanel == 0 {
			m.models.planCursor = clamp(m.models.planCursor-1, 0, len(m.models.planModels)-1)
		} else {
			m.models.execCursor = clamp(m.models.execCursor-1, 0, len(m.models.execModels)-1)
		}
	case "down", "j":
		if m.models.activePanel == 0 {
			m.models.planCursor = clamp(m.models.planCursor+1, 0, len(m.models.planModels)-1)
		} else {
			m.models.execCursor = clamp(m.models.execCursor+1, 0, len(m.models.execModels)-1)
		}
	case "enter":
		if m.models.activePanel == 0 && len(m.models.planModels) > 0 {
			m.models.planModel = m.models.planModels[m.models.planCursor]
		}
		if m.models.activePanel == 1 && len(m.models.execModels) > 0 {
			m.models.execModel = m.models.execModels[m.models.execCursor]
		}
	case "s":
		return m.saveModels()
	}
	return nil
}

func (m *MainModel) saveModels() tea.Cmd {
	if m.models.planModel != "" {
		rc := m.cfg.Roles["plan"]
		rc.Model = m.models.planModel
		m.cfg.Roles["plan"] = rc
	}
	if m.models.execModel != "" {
		rc := m.cfg.Roles["execute"]
		rc.Model = m.models.execModel
		m.cfg.Roles["execute"] = rc
	}
	return func() tea.Msg {
		config.Save(m.cfg, config.GlobalPath())
		return nil
	}
}


