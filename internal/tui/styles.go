package tui

import "github.com/charmbracelet/lipgloss"

var (
	primary = lipgloss.Color("39")
	success = lipgloss.Color("42")
	warning = lipgloss.Color("220")
	danger  = lipgloss.Color("196")
	subtle  = lipgloss.Color("240")

	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(primary).Padding(1, 2)

	HeadingStyle = lipgloss.NewStyle().Bold(true).Foreground(primary).Padding(0, 2).MarginBottom(1)

	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(primary).
			Padding(0, 1)

	DimmedStyle = lipgloss.NewStyle().Foreground(subtle)

	SuccessStyle = lipgloss.NewStyle().Foreground(success).Bold(true)
	ErrorStyle   = lipgloss.NewStyle().Foreground(danger).Bold(true)
	WarningStyle = lipgloss.NewStyle().Foreground(warning)

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle)

	HelpStyle = lipgloss.NewStyle().Foreground(subtle).MarginTop(1)

	DocStyle = lipgloss.NewStyle().Margin(0, 1).Padding(0, 1)

	OnlineBadge  = SuccessStyle.Render("ONLINE")
	OfflineBadge = DimmedStyle.Render("OFFLINE")
	SetBadge     = SuccessStyle.Render("set")
	MissingBadge = ErrorStyle.Render("missing")

	StatusBarStyle = lipgloss.NewStyle().
			MarginBottom(1).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(subtle)
)

func statusBar(left, center, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(center) - lipgloss.Width(right) - 6
	if gap < 1 {
		gap = 1
	}
	return StatusBarStyle.Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, left, DimmedStyle.Render(string(repeatChar(' ', gap))), center, DimmedStyle.Render("  "), right),
	)
}

func repeatChar(c byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return b
}
