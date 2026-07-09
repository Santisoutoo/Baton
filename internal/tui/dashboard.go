package tui

import (
	"fmt"
	"net/http"
	"strings"

	"baton/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *MainModel) handleDashboardKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k":
		m.dashboard.cursor = clamp(m.dashboard.cursor-1, 0, len(m.dashboard.items)-1)
	case "down", "j":
		m.dashboard.cursor = clamp(m.dashboard.cursor+1, 0, len(m.dashboard.items)-1)
	case "enter":
		switch m.dashboard.cursor {
		case 0:
			return m.toggleProxy()
		case 1:
			m.screen = ScreenUsage
			return m.loadUsage()
		case 2:
			m.screen = ScreenModels
			return m.loadModels()
		case 3:
			m.screen = ScreenCredentials
		case 4:
			m.screen = ScreenConfigView
		case 5:
			m.screen = ScreenStatus
		}
	}
	return nil
}

func (m *MainModel) toggleProxy() tea.Cmd {
	if m.proxy.running {
		m.shutdownProxy()
		return nil
	}
	return m.startProxy()
}

func (m *MainModel) startProxy() tea.Cmd {
	m.proxy.status = "starting"
	handler := m.buildFullProxy()
	m.proxy.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", m.proxy.port),
		Handler: handler,
	}
	go func() {
		m.proxy.server.ListenAndServe()
	}()
	return func() tea.Msg {
		return proxyHealthCheckMsg{attempt: 0}
	}
}

func (m MainModel) View() string {
	if !m.ready {
		return "Loading..."
	}
	content := m.renderScreen()
	return DocStyle.Render(content)
}

func (m MainModel) renderScreen() string {
	switch m.screen {
	case ScreenDashboard:
		return m.viewDashboard()
	case ScreenUsage:
		return m.viewUsage()
	case ScreenModels:
		return m.viewModels()
	case ScreenCredentials:
		return m.viewCredentials()
	case ScreenConfigView:
		return m.viewConfig()
	case ScreenStatus:
		return m.viewStatus()
	default:
		return ""
	}
}

func (m MainModel) viewDashboard() string {
	var b strings.Builder

	proxyStatus := m.formatProxyStatus()
	credsSummary := m.formatCredsSummary()
	quickUsage := m.formatQuickUsage()

	b.WriteString(TitleStyle.Render("baton"))
	b.WriteString("\n")
	b.WriteString(DimmedStyle.Render("  Claude Code orchestrates, OpenCode executes"))
	b.WriteString("\n\n")

	width := m.width - 4
	if width < 40 {
		width = 40
	}
	b.WriteString(statusBar(
		fmt.Sprintf("Proxy: %s :%d", proxyStatus, m.proxy.port),
		fmt.Sprintf("Creds: %s", credsSummary),
		fmt.Sprintf("Usage: %s", quickUsage),
		width,
	))
	b.WriteString("\n")

	b.WriteString(renderMenu(m.dashboard.items, m.dashboard.cursor))
	b.WriteString("\n\n")
	b.WriteString(HelpStyle.Render("j/k or ↑/↓ navigate   enter select   q quit"))

	return b.String()
}

func (m *MainModel) loadUsage() tea.Cmd {
	m.usage.loading = true
	return func() tea.Msg {
		if m.meter == nil {
			return usageLoadedMsg{rows: nil}
		}
		since := parseTimeFilter(m.usage.filter)
		rows, err := m.meter.Query(core.Filter{Since: since, By: m.usage.groupBy})
		if err != nil {
			return usageLoadedMsg{rows: nil}
		}
		var urows []usageRow
		for _, r := range rows {
			cost := estimateUsageCost(m.cfg, r.Key, r.InputTokens, r.OutputTokens)
			urows = append(urows, usageRow{
				key:       r.Key,
				requests:  r.Requests,
				inTokens:  r.InputTokens,
				outTokens: r.OutputTokens,
				cost:      cost,
			})
		}
		return usageLoadedMsg{rows: urows}
	}
}

func (m *MainModel) loadModels() tea.Cmd {
	m.models.loading = true
	m.models.fetchErr = ""
	return func() tea.Msg {
		oc, ok := m.cfg.Backends["opencode"]
		if !ok || oc.ModelsURL == "" {
			return modelsLoadedMsg{err: "no models_url configured"}
		}
		ids, err := fetchModelIDs(m.creds, oc.ModelsURL)
		if err != nil {
			return modelsLoadedMsg{err: err.Error()}
		}
		return modelsLoadedMsg{models: ids}
	}
}
