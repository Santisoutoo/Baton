package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"baton/internal/config"
	"baton/internal/core"
	"baton/internal/meter"

	"github.com/spf13/cobra"
)

func newUsageCmd() *cobra.Command {
	var since string
	var by string

	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show token usage (and estimated cost) from the local meter",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			sinceTime, err := parseSince(since)
			if err != nil {
				return err
			}

			m, err := meter.Open(config.UsageDBPath())
			if err != nil {
				return err
			}
			defer m.Close()

			rows, err := m.Query(core.Filter{Since: sinceTime, By: by})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				fmt.Println("no usage recorded yet")
				return nil
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintf(tw, "%s\tREQUESTS\tIN\tOUT\tCOST(USD)\n", header(by))
			var tin, tout int
			var tcost float64
			for _, r := range rows {
				cost := estimateCost(cfg, r)
				tin += r.InputTokens
				tout += r.OutputTokens
				tcost += cost
				fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%.4f\n", r.Key, r.Requests, r.InputTokens, r.OutputTokens, cost)
			}
			fmt.Fprintf(tw, "TOTAL\t\t%d\t%d\t%.4f\n", tin, tout, tcost)
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "e.g. 24h, 7d, 30d (default: all time)")
	cmd.Flags().StringVar(&by, "by", "model", "group by: model|backend|day")
	return cmd
}

func header(by string) string {
	switch by {
	case "backend":
		return "BACKEND"
	case "day":
		return "DAY"
	default:
		return "MODEL"
	}
}

// estimateCost uses per-model prices (USD per 1M tokens) from config; unpriced
// models (e.g. free tiers or the subscription lane) cost 0.
func estimateCost(cfg *config.Config, r core.Aggregate) float64 {
	p, ok := cfg.Prices[r.Key]
	if !ok {
		return 0
	}
	return float64(r.InputTokens)/1e6*p.In + float64(r.OutputTokens)/1e6*p.Out
}

// parseSince accepts Go durations plus a "Nd" days shorthand.
func parseSince(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err != nil {
			return time.Time{}, fmt.Errorf("bad --since %q", s)
		}
		return time.Now().Add(-time.Duration(days) * 24 * time.Hour), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad --since %q (try 24h or 7d)", s)
	}
	return time.Now().Add(-d), nil
}
