package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kevinyoung1399/code-review-action/internal/config"
	"github.com/kevinyoung1399/code-review-action/internal/orchestrator"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	var result *orchestrator.Result

	switch cfg.EventName {
	case "issue_comment":
		// Skip if comment is from the bot itself (prevent infinite loop)
		if isBot(cfg.CommentBody) {
			fmt.Println("📨 Event: issue_comment — AI 自己的留言，跳過")
			result = &orchestrator.Result{Status: "success"}
		} else {
			fmt.Printf("📨 Event: issue_comment — %s 回覆了，開始評估\n", cfg.CommentUser)
			result, err = orchestrator.RunReply(cfg)
		}
	default:
		// pull_request event → full code review
		fmt.Println("📨 Event: pull_request — 執行完整 code review")
		result, err = orchestrator.Run(cfg)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(1)
	}

	setOutput("review_status", result.Status)
	setOutput("total_comments", fmt.Sprintf("%d", result.TotalComments))
	setOutput("critical_count", fmt.Sprintf("%d", result.CriticalCount))
	setOutput("warning_count", fmt.Sprintf("%d", result.WarningCount))
	setOutput("suggestion_count", fmt.Sprintf("%d", result.SuggestionCount))
}

func isBot(commentBody string) bool {
	// AI comments contain these markers
	markers := []string{"**Shield**", "**Rex**", "**Aria**", "**Biz**", "**Arch**", "Code Review — Team Discussion"}
	for _, m := range markers {
		if strings.Contains(commentBody, m) {
			return true
		}
	}
	return false
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
