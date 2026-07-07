package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"baton/internal/config"
	"baton/internal/creds"

	"github.com/spf13/cobra"
)

func newModelsCmd() *cobra.Command {
	var planModel, execModel string

	cmd := &cobra.Command{
		Use:   "models",
		Short: "List available models, or pick the plan/exec model",
		Long: "With no flags, lists Claude tiers (planning) and the OpenCode catalog\n" +
			"(execution). With --plan/--exec, writes the choice to the global config.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			if planModel == "" && execModel == "" {
				return listModels(cfg)
			}

			if planModel != "" {
				rc := cfg.Roles["plan"]
				rc.Model = planModel
				cfg.Roles["plan"] = rc
			}
			if execModel != "" {
				rc := cfg.Roles["execute"]
				rc.Model = execModel
				cfg.Roles["execute"] = rc
			}
			if err := config.Save(cfg, config.GlobalPath()); err != nil {
				return err
			}
			fmt.Printf("saved to %s\n", config.GlobalPath())
			if planModel != "" {
				fmt.Printf("  plan    -> %s\n", planModel)
			}
			if execModel != "" {
				fmt.Printf("  execute -> %s\n", execModel)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&planModel, "plan", "", "model for the planning/Claude lane")
	cmd.Flags().StringVar(&execModel, "exec", "", "model for the execution/OpenCode lane")
	return cmd
}

func listModels(cfg *config.Config) error {
	fmt.Println("planning (Claude tiers — routed to your subscription):")
	for _, t := range []string{"claude-opus-4-8 (opus)", "claude-sonnet-5 (sonnet)", "claude-haiku-4-5 (haiku)"} {
		fmt.Printf("  %s\n", t)
	}

	fmt.Println("\nexecution (OpenCode catalog):")
	oc, ok := cfg.Backends["opencode"]
	if !ok || oc.ModelsURL == "" {
		fmt.Println("  (no models_url configured for the opencode backend)")
		return nil
	}
	ids, err := fetchModelIDs(oc.ModelsURL)
	if err != nil {
		fmt.Printf("  could not fetch catalog: %v\n", err)
		fmt.Println("  (login first: baton login opencode)")
		return nil
	}
	for _, id := range ids {
		fmt.Printf("  %s\n", id)
	}
	return nil
}

// fetchModelIDs GETs an OpenAI-style /models endpoint and returns the ids.
func fetchModelIDs(url string) ([]string, error) {
	key, _ := creds.New().Get("opencode")
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
