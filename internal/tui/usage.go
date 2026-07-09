package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"baton/internal/config"
	"baton/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

func (m MainModel) viewStatus() string {
	var b strings.Builder

	b.WriteString(HeadingStyle.Render("System Status"))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("  Global config : %s\n", config.GlobalPath()))
	if pp := config.ProjectPath(); pp != "" {
		b.WriteString(fmt.Sprintf("  Project config: %s\n", pp))
	}
	b.WriteString(fmt.Sprintf("  Port           : %d\n", m.cfg.Port))
	b.WriteString(fmt.Sprintf("  Log level      : %s\n", m.cfg.LogLevel))
	b.WriteString(fmt.Sprintf("  On exec error  : %s\n", m.cfg.OnExecError))
	b.WriteString(fmt.Sprintf("  Default role   : %s\n\n", m.cfg.DefaultRole))

	b.WriteString(DimmedStyle.Render("  Roles:"))
	b.WriteString("\n")
	for name, rc := range m.cfg.Roles {
		model := rc.Model
		if model == "" {
			model = "(keep inbound)"
		}
		b.WriteString(fmt.Sprintf("    %-8s -> backend %-12s model %s\n", name, rc.Backend, model))
	}

	b.WriteString(DimmedStyle.Render("  Tiers:"))
	b.WriteString("\n")
	for tier, role := range m.cfg.Tiers {
		b.WriteString(fmt.Sprintf("    %-8s -> %s\n", tier, role))
	}

	b.WriteString(DimmedStyle.Render("  Backends:"))
	b.WriteString("\n")
	for name, bc := range m.cfg.Backends {
		cred := "n/a"
		if bc.Credential != "" {
			if v, _ := m.creds.Get(bc.Credential); v != "" {
				cred = SetBadge
			} else {
				cred = MissingBadge
			}
		}
		b.WriteString(fmt.Sprintf("    %-12s type=%-10s auth=%-12s cred=%s\n", name, bc.Type, bc.Auth, cred))
	}

	b.WriteString("\n")
	b.WriteString(HelpStyle.Render("esc back   ↑↓ scroll"))

	return b.String()
}

func (m MainModel) viewUsage() string {
	var b strings.Builder

	b.WriteString(HeadingStyle.Render("Usage & Cost Report"))
	b.WriteString("\n")

	filters := []string{"24h", "7d", "30d", "all"}
	groups := []string{"model", "backend", "day"}

	var filterBar strings.Builder
	for _, f := range filters {
		if f == m.usage.filter {
			filterBar.WriteString(SelectedStyle.Render(" " + f + " "))
		} else {
			filterBar.WriteString(DimmedStyle.Render(" " + f + " "))
		}
	}
	b.WriteString(filterBar.String())
	b.WriteString("\n")

	var groupBar strings.Builder
	groupBar.WriteString(DimmedStyle.Render("  group by: "))
	for i, g := range groups {
		if g == m.usage.groupBy {
			groupBar.WriteString(SuccessStyle.Render(g))
		} else {
			groupBar.WriteString(DimmedStyle.Render(g))
		}
		if i < len(groups)-1 {
			groupBar.WriteString(DimmedStyle.Render(" | "))
		}
	}
	b.WriteString(groupBar.String())
	b.WriteString("\n\n")

	if m.usage.loading {
		b.WriteString(DimmedStyle.Render("  Loading..."))
		b.WriteString("\n\n")
	} else if len(m.usage.rows) == 0 {
		b.WriteString(DimmedStyle.Render("  No usage recorded yet"))
		b.WriteString("\n\n")
	} else {
		b.WriteString(renderUsageTable(m.usage.rows, m.usage.groupBy, m.width-4))
	}

	b.WriteString(HelpStyle.Render("tab cycle time   ctrl+g cycle group   esc back"))

	return b.String()
}

func renderUsageTable(rows []usageRow, groupBy string, maxWidth int) string {
	var b strings.Builder

	keyHdr := "MODEL"
	switch groupBy {
	case "backend":
		keyHdr = "BACKEND"
	case "day":
		keyHdr = "DAY"
	}

	keyW := 20
	reqW := 8
	inW := 8
	outW := 8
	costW := 10

	if maxWidth > 60 {
		keyW = maxWidth - reqW - inW - outW - costW - 8
		if keyW < 12 {
			keyW = 12
		}
	}

	divider := strings.Repeat("─", maxWidth-2)

	b.WriteString(fmt.Sprintf("  %-*s %*s %*s %*s %*s\n", keyW, keyHdr, reqW, "REQS", inW, "IN", outW, "OUT", costW, "COST"))
	b.WriteString(DimmedStyle.Render("  " + divider))
	b.WriteString("\n")

	var tin, tout int
	var tcost float64
	for _, r := range rows {
		key := r.key
		if len(key) > keyW-1 {
			key = key[:keyW-2] + "…"
		}
		b.WriteString(fmt.Sprintf("  %-*s %*d %*s %*s %*.4f\n",
			keyW, key,
			reqW, r.requests,
			inW, formatTokens(r.inTokens),
			outW, formatTokens(r.outTokens),
			costW, r.cost,
		))
		tin += r.inTokens
		tout += r.outTokens
		tcost += r.cost
	}

	b.WriteString(DimmedStyle.Render("  " + divider))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %-*s %*d %*s %*s %*.4f\n",
		keyW, "TOTAL",
		reqW, len(rows),
		inW, formatTokens(tin),
		outW, formatTokens(tout),
		costW, tcost,
	))

	return b.String()
}

func (m *MainModel) handleUsageKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "tab":
		filters := []string{"24h", "7d", "30d", "all"}
		for i, f := range filters {
			if f == m.usage.filter {
				m.usage.filter = filters[(i+1)%len(filters)]
				break
			}
		}
		return m.loadUsage()
	case "ctrl+g":
		groups := []string{"model", "backend", "day"}
		for i, g := range groups {
			if g == m.usage.groupBy {
				m.usage.groupBy = groups[(i+1)%len(groups)]
				break
			}
		}
		return m.loadUsage()
	}
	return nil
}

func parseTimeFilter(filter string) time.Time {
	switch filter {
	case "24h":
		return time.Now().Add(-24 * time.Hour)
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}

func estimateUsageCost(cfg *config.Config, key string, inTokens, outTokens int) float64 {
	p, ok := cfg.Prices[key]
	if !ok {
		return 0
	}
	return float64(inTokens)/1e6*p.In + float64(outTokens)/1e6*p.Out
}

func fetchModelIDs(creds core.CredentialStore, url string) ([]string, error) {
	key, _ := creds.Get("opencode")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}


