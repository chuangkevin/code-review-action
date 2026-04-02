package main

import (
	"fmt"
	"os"

	"github.com/kevinyoung1399/code-review-action/internal/config"
	"github.com/kevinyoung1399/code-review-action/internal/orchestrator"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	result, err := orchestrator.Run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "review failed: %v\n", err)
		os.Exit(1)
	}

	setOutput("review_status", result.Status)
	setOutput("total_comments", fmt.Sprintf("%d", result.TotalComments))
	setOutput("critical_count", fmt.Sprintf("%d", result.CriticalCount))
	setOutput("warning_count", fmt.Sprintf("%d", result.WarningCount))
	setOutput("suggestion_count", fmt.Sprintf("%d", result.SuggestionCount))
}

func setOutput(name, value string) {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		return
	}
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s=%s\n", name, value)
}
