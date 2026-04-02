package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/kevinyoung1399/code-review-action/internal/config"
	"github.com/kevinyoung1399/code-review-action/internal/orchestrator"
)

func main() {
	// Force unbuffered stdout
	fmt.Println("=== AI Code Review Action Starting ===")
	fmt.Printf("Time: %s\n", time.Now().Format(time.RFC3339))

	// Debug: print key env vars
	fmt.Println()
	fmt.Println("📋 Environment check:")
	fmt.Printf("   GITHUB_SERVER_URL: %s\n", os.Getenv("GITHUB_SERVER_URL"))
	fmt.Printf("   GITHUB_REPOSITORY: %s\n", os.Getenv("GITHUB_REPOSITORY"))
	fmt.Printf("   GITHUB_EVENT_PATH: %s\n", os.Getenv("GITHUB_EVENT_PATH"))
	fmt.Printf("   GITHUB_EVENT_NAME: %s\n", os.Getenv("GITHUB_EVENT_NAME"))

	// Check if Gemini API is reachable
	fmt.Println()
	fmt.Println("🌐 Testing Gemini API connectivity...")
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get("https://generativelanguage.googleapis.com/v1beta/models")
	if err != nil {
		fmt.Printf("   ❌ Cannot reach Gemini API: %v\n", err)
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("   ✅ Gemini API reachable (status: %d, body: %.100s)\n", resp.StatusCode, string(body))
	}

	fmt.Println()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🔑 API keys loaded: %d keys (first 4 chars: %s...)\n", len(cfg.GeminiAPIKeys), safePrefix(cfg.GeminiAPIKeys[0], 4))
	fmt.Println()

	result, err := orchestrator.Run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ review failed: %v\n", err)
		os.Exit(1)
	}

	setOutput("review_status", result.Status)
	setOutput("total_comments", fmt.Sprintf("%d", result.TotalComments))
	setOutput("critical_count", fmt.Sprintf("%d", result.CriticalCount))
	setOutput("warning_count", fmt.Sprintf("%d", result.WarningCount))
	setOutput("suggestion_count", fmt.Sprintf("%d", result.SuggestionCount))
}

func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
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
