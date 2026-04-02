package reviewer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kevinyoung1399/code-review-action/internal/gemini"
)

type InlineComment struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Body     string `json:"body"`
}

type ReviewResult struct {
	Role           string          `json:"-"`
	InlineComments []InlineComment `json:"inline_comments"`
	Summary        string          `json:"summary"`
}

func Review(client *gemini.Client, role string, diff string, skillContents []string, pr PRContext) (*ReviewResult, error) {
	systemPrompt := GetSystemPrompt(role)
	if systemPrompt == "" {
		return nil, fmt.Errorf("unknown role: %s", role)
	}

	userPrompt := BuildUserPrompt(diff, skillContents, pr)

	text, err := client.Generate(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("review (%s): %w", role, err)
	}

	text = extractReviewJSON(text)

	var result ReviewResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parse review result (%s): %w (raw: %s)", role, err, truncate(text, 200))
	}

	result.Role = role
	return &result, nil
}

func ReviewBatched(client *gemini.Client, role string, diff string, skillContents []string, pr PRContext, maxDiffSize int) (*ReviewResult, error) {
	batches := SplitIntoBatches(diff, maxDiffSize)
	if len(batches) == 1 {
		return Review(client, role, diff, skillContents, pr)
	}

	var allComments []InlineComment
	var summaries []string

	for i, batch := range batches {
		result, err := Review(client, role, batch, skillContents, pr)
		if err != nil {
			return nil, fmt.Errorf("batch %d/%d: %w", i+1, len(batches), err)
		}
		allComments = append(allComments, result.InlineComments...)
		if result.Summary != "" {
			summaries = append(summaries, result.Summary)
		}
	}

	return &ReviewResult{
		Role:           role,
		InlineComments: allComments,
		Summary:        strings.Join(summaries, " "),
	}, nil
}

func extractReviewJSON(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	return text
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
