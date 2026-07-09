package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"baton/internal/config"
	"baton/internal/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type Screen int

const (
	ScreenDashboard Screen = iota
	ScreenUsage
	ScreenModels
	ScreenCredentials
	ScreenConfigView
	ScreenStatus
)

func (s Screen) String() string {
	switch s {
	case ScreenDashboard:
		return "Dashboard"
	case ScreenUsage:
		return "Usage"
	case ScreenModels:
		return "Models"
	case ScreenCredentials:
		return "Credentials"
	case ScreenConfigView:
		return "Config"
	case ScreenStatus:
		return "Status"
	default:
		return ""
	}
}

func Run(cfg *config.Config, creds core.CredentialStore, meter core.MeterStore) error {
	m := newMainModel(cfg, creds, meter)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type dashboardState struct {
	cursor  int
	items   []string
}

type usageState struct {
	filter  string
	groupBy string
	rows    []usageRow
	loading bool
}

type usageRow struct {
	key      string
	requests int
	inTokens int
	outTokens int
	cost     float64
}

type modelsState struct {
	planModels  []string
	execModels  []string
	planCursor  int
	execCursor  int
	planModel   string
	execModel   string
	loading     bool
	fetchErr    string
	activePanel int
}

type credState struct {
	services []struct {
		name   string
		status string
	}
	cursor    int
	input     textinput.Model
	inputMode bool
	inputSvc  string
	message   string
	msgType   string
}

type configState struct {
	sections  []configSection
	cursor    int
	detail    []configEntry
	detailCur int
	editing   bool
	input     textinput.Model
	message   string
}

type configSection struct {
	name  string
	title string
}

type configEntry struct {
	key   string
	value string
}

type proxyState struct {
	running bool
	port    int
	server  *http.Server
	status  string
}

type MainModel struct {
	cfg   *config.Config
	creds core.CredentialStore
	meter core.MeterStore

	width  int
	height int
	screen Screen
	ready  bool

	dashboard dashboardState
	usage     usageState
	models    modelsState
	credsMgr  credState
	configNav configState
	proxy     proxyState
}

func newMainModel(cfg *config.Config, creds core.CredentialStore, meter core.MeterStore) MainModel {
	ti := textinput.New()
	ti.Placeholder = "API key..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	cti := textinput.New()
	cti.Placeholder = "value..."
	cti.Focus()
	cti.CharLimit = 256
	cti.Width = 50

	m := MainModel{
		cfg:   cfg,
		creds: creds,
		meter: meter,

		dashboard: dashboardState{
			cursor: 0,
			items:  []string{"Start / Stop Proxy", "Usage & Cost Report", "Select Models", "Manage Credentials", "Configuration", "System Status"},
		},
		usage: usageState{
			filter:  "7d",
			groupBy: "model",
			loading: false,
		},
		models: modelsState{
			planModels: []string{"claude-opus-4-8", "claude-sonnet-5", "claude-haiku-4-5"},
			planModel:  "",
			execModel:  "",
			loading:    false,
		},
		credsMgr: credState{
			input: ti,
		},
		configNav: configState{
			input: cti,
			sections: []configSection{
				{name: "general", title: "General"},
				{name: "roles", title: "Roles"},
				{name: "backends", title: "Backends"},
				{name: "tiers", title: "Tiers"},
				{name: "prices", title: "Prices"},
			},
		},
		proxy: proxyState{
			port:   cfg.Port,
			status: "stopped",
		},
	}

	rc, ok := cfg.Roles["plan"]
	if ok && rc.Model != "" {
		m.models.planModel = rc.Model
	}
	rc2, ok := cfg.Roles["execute"]
	if ok && rc2.Model != "" {
		m.models.execModel = rc2.Model
	}

	m.rebuildCredServices()
	return m
}

func (m MainModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.loadUsage(),
	)
}

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.ready = true
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.shutdownProxy()
			return m, tea.Quit

		case "q":
			if m.screen == ScreenDashboard {
				m.shutdownProxy()
				return m, tea.Quit
			}
			m.screen = ScreenDashboard
			return m, nil

		case "esc":
			if m.credsMgr.inputMode {
				m.credsMgr.inputMode = false
				m.credsMgr.input.Reset()
				return m, nil
			}
			if m.configNav.editing {
				m.configNav.editing = false
				m.configNav.input.Reset()
				return m, nil
			}
			if m.configNav.detail != nil {
				m.configNav.detail = nil
				m.configNav.detailCur = 0
				return m, nil
			}
			if m.screen != ScreenDashboard {
				m.screen = ScreenDashboard
				return m, nil
			}
		}

		cmd := m.handleScreenKey(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case proxyHealthyMsg:
		m.proxy.running = true
		m.proxy.status = "running"

	case proxyHealthCheckMsg:
		if msg.attempt < 15 {
			cmds = append(cmds, func() tea.Msg {
				time.Sleep(200 * time.Millisecond)
				resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", m.proxy.port))
				if err == nil {
					resp.Body.Close()
					return proxyHealthyMsg{}
				}
				return proxyHealthCheckMsg{attempt: msg.attempt + 1}
			})
		} else {
			m.proxy.status = "error"
		}

	case usageLoadedMsg:
		m.usage.rows = msg.rows
		m.usage.loading = false

	case modelsLoadedMsg:
		m.models.execModels = msg.models
		m.models.loading = false
		m.models.fetchErr = msg.err

	case credSetMsg:
		if msg.err != "" {
			m.credsMgr.message = msg.err
			m.credsMgr.msgType = "error"
		} else {
			m.credsMgr.message = "Key stored for " + msg.svc
			m.credsMgr.msgType = "success"
		}
		m.credsMgr.inputMode = false
		m.credsMgr.input.Reset()
		m.rebuildCredServices()
	}

	return m, tea.Batch(cmds...)
}

func (m *MainModel) handleScreenKey(msg tea.KeyMsg) tea.Cmd {
	switch m.screen {
	case ScreenDashboard:
		return m.handleDashboardKey(msg)
	case ScreenUsage:
		return m.handleUsageKey(msg)
	case ScreenModels:
		return m.handleModelsKey(msg)
	case ScreenCredentials:
		return m.handleCredentialsKey(msg)
	case ScreenConfigView:
		return m.handleConfigKey(msg)
	case ScreenStatus:
		return nil
	}
	return nil
}

func (m *MainModel) shutdownProxy() {
	if m.proxy.server != nil && m.proxy.running {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		m.proxy.server.Shutdown(ctx)
		m.proxy.running = false
		m.proxy.status = "stopped"
	}
}

func (m *MainModel) rebuildCredServices() {
	m.credsMgr.services = nil
	seen := map[string]bool{}

	addService := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		status := "missing"
		if v, _ := m.creds.Get(name); v != "" {
			status = "set"
		}
		m.credsMgr.services = append(m.credsMgr.services, struct {
			name   string
			status string
		}{name: name, status: status})
	}

	for _, bc := range m.cfg.Backends {
		if bc.Credential != "" {
			addService(bc.Credential)
		} else if bc.Auth != "passthrough" {
			addService("opencode")
		}
	}

	addService("anthropic")
}

type proxyHealthyMsg struct{}
type proxyHealthCheckMsg struct {
	attempt int
}

type usageLoadedMsg struct {
	rows []usageRow
}

type modelsLoadedMsg struct {
	models []string
	err    string
}

func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func (m *MainModel) formatProxyStatus() string {
	if m.proxy.running {
		return OnlineBadge
	}
	return OfflineBadge
}

func (m *MainModel) formatCredsSummary() string {
	set, total := 0, 0
	for _, s := range m.credsMgr.services {
		if s.status == "set" {
			set++
		}
		total++
	}
	if total == 0 {
		return DimmedStyle.Render("no creds")
	}
	if set == total {
		return SuccessStyle.Render(fmt.Sprintf("%d/%d creds", set, total))
	}
	return WarningStyle.Render(fmt.Sprintf("%d/%d creds", set, total))
}

func (m *MainModel) formatQuickUsage() string {
	var in, out int
	for _, r := range m.usage.rows {
		in += r.inTokens
		out += r.outTokens
	}
	if in+out == 0 {
		return DimmedStyle.Render("no usage yet")
	}
	return fmt.Sprintf("%s in / %s out", formatTokens(in), formatTokens(out))
}

func previewStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func renderMenu(items []string, cursor int) string {
	var b strings.Builder
	for i, item := range items {
		if i == cursor {
			b.WriteString(SelectedStyle.Render(" > "+item+" "))
		} else {
			b.WriteString(DimmedStyle.Render("   " + item))
		}
		if i < len(items)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
