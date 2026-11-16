package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mainza-ai/kubona/core/pkg/evals"
	"github.com/mainza-ai/kubona/core/pkg/llm"
)

// runEvalLLM scores recorded predictions by default. With --live it calls one
// explicitly named provider/model, parses a strict probability vector per
// fixture, and still refuses to write a report if any fixture is incomplete.
//
//	kubonad eval-llm --role copilot --predictions recorded.json [--dir DIR]
//	kubonad eval-llm --role copilot --live --provider openai --model gpt-4.1 [--db PATH]
func runEvalLLM(args []string) int {
	fs := flag.NewFlagSet("eval-llm", flag.ContinueOnError)
	role := fs.String("role", "copilot", "fixture role to evaluate")
	predictionsPath := fs.String("predictions", "", "recorded predictions JSON (required unless --live)")
	dir := fs.String("dir", "", "report directory (default: ~/.kubona/evals or KUBONA_EVALS_DIR)")
	live := fs.Bool("live", false, "call a model instead of reading a prediction file")
	provider := fs.String("provider", "", "live provider (required with --live)")
	model := fs.String("model", "", "live model id (required with --live)")
	dbPath := fs.String("db", "", "ledger path for live settings (default KUBONA_DB_PATH or ~/.kubona/kubona.db)")
	maxTokens := fs.Int("max-tokens", 512, "live completion token cap")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	set, err := evals.Load(*role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	var input evals.PredictionFile
	if *live {
		if strings.TrimSpace(*provider) == "" || strings.TrimSpace(*model) == "" {
			fmt.Fprintln(os.Stderr, "eval-llm --live requires --provider and --model (no implicit active model)")
			return 2
		}
		input, err = livePredictions(*role, llm.ProviderType(*provider), *model, *dbPath, *maxTokens, set)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: live eval: %v\n", err)
			return 1
		}
	} else {
		if *predictionsPath == "" {
			fmt.Fprintln(os.Stderr, "usage: kubonad eval-llm --role copilot --predictions recorded.json [--dir DIR]")
			return 2
		}
		input, err = evals.LoadPredictionFile(*predictionsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: predictions: %v\n", err)
			return 1
		}
	}
	report, err := evals.Evaluate(set, input, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	store := evals.Store{Dir: *dir}
	if store.Dir == "" {
		store.Dir = evals.DefaultDir()
	}
	if err := store.Write(report); err != nil {
		fmt.Fprintf(os.Stderr, "error: writing report: %v\n", err)
		return 1
	}
	mode := "recorded"
	if *live {
		mode = "live"
	}
	fmt.Printf(
		"eval-llm (%s): %s %s/%s scored %d (+%d unscored), Brier %.4f, ECE %.4f, fixtures %s\n",
		mode, report.Role, report.Provider, report.Model, report.Total, report.Unscored,
		report.Brier, report.ECE, report.FixturesSHA256[:12],
	)
	return 0
}

func evalDBPath(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if env := strings.TrimSpace(os.Getenv("KUBONA_DB_PATH")); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kubona", "kubona.db")
}

func livePredictions(role string, provider llm.ProviderType, model, dbPath string, maxTokens int, set *evals.Set) (evals.PredictionFile, error) {
	path := evalDBPath(dbPath)
	if path == "" {
		return evals.PredictionFile{}, fmt.Errorf("ledger path is not configured")
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		return evals.PredictionFile{}, err
	}
	defer db.Close()
	settings, err := llm.LoadSettingsReadOnly(db)
	if err != nil {
		return evals.PredictionFile{}, fmt.Errorf("read LLM settings: %w", err)
	}
	client := llm.NewClient(settings)
	out := evals.PredictionFile{
		Role: role, Provider: string(provider), Model: model,
		FixturesSHA256: set.SHA256, Predictions: make([]evals.Prediction, 0, len(set.Fixtures)),
	}
	for _, fixture := range set.Fixtures {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		text, _, _, err := client.CompleteWithTarget(ctx, provider, model, []llm.ChatMessage{
			{Role: "system", Content: "You are a deterministic scientific claim classifier. Return only the requested JSON object."},
			{Role: "user", Content: fixture.Prompt + "\n\nReturn ONLY a JSON object with keys supported, refuted, unverified. Values are probabilities that sum to 1. No prose, no code fence."},
		}, maxTokens)
		cancel()
		if err != nil {
			return out, fmt.Errorf("fixture %s: %w", fixture.ID, err)
		}
		probs, err := parseProbabilityObject(text)
		if err != nil {
			return out, fmt.Errorf("fixture %s: %w", fixture.ID, err)
		}
		out.Predictions = append(out.Predictions, evals.Prediction{ID: fixture.ID, Probabilities: probs})
	}
	return out, nil
}

func parseProbabilityObject(text string) (map[string]float64, error) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("model did not return a JSON object")
	}
	var raw map[string]float64
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &raw); err != nil {
		return nil, fmt.Errorf("model JSON is unreadable: %w", err)
	}
	out := map[string]float64{}
	for _, label := range evals.AllowedLabels {
		value, ok := raw[label]
		if !ok {
			return nil, fmt.Errorf("model omitted %q", label)
		}
		out[label] = value
	}
	return out, nil
}
