package skills

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kevinyoung1399/code-review-action/internal/gemini"
)

func MatchSkills(client *gemini.Client, index []SkillEntry, files []string, diffSummary string) (map[string][]string, error) {
	roles := []string{"frontend", "backend", "security", "business", "architecture"}
	emptyResult := make(map[string][]string)
	for _, r := range roles {
		emptyResult[r] = []string{}
	}

	if len(index) == 0 {
		return emptyResult, nil
	}

	systemPrompt := buildMatcherSystemPrompt()
	userPrompt := buildMatcherUserPrompt(index, files, diffSummary)

	text, err := client.Generate(systemPrompt, userPrompt)
	if err != nil {
		return emptyResult, fmt.Errorf("skill matching: %w", err)
	}

	text = extractJSON(text)

	var result map[string][]string
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return emptyResult, fmt.Errorf("parse skill match result: %w", err)
	}

	nameSet := make(map[string]bool)
	for _, s := range index {
		nameSet[s.Name] = true
	}
	for role, skills := range result {
		var valid []string
		for _, s := range skills {
			if nameSet[s] {
				valid = append(valid, s)
			}
		}
		result[role] = valid
	}

	return result, nil
}

func buildMatcherSystemPrompt() string {
	return `你是一個 skill matcher。根據 PR 變更內容，從 skill 清單中選出與本次 review 相關的 skill，並指派給對應的 reviewer 角色。

## Reviewer 角色
- frontend: 前端品質
- backend: 系統穩定性
- security: 安全性
- business: 業務邏輯正確性
- architecture: 長期維護性

## 規則
- 只選真正相關的 skill，不要勉強
- 每個角色最多 3 個 skill
- 回傳純 JSON，不要加 markdown 格式`
}

func buildMatcherUserPrompt(index []SkillEntry, files []string, diffSummary string) string {
	var sb strings.Builder

	sb.WriteString("## 變更檔案\n")
	for _, f := range files {
		sb.WriteString("- " + f + "\n")
	}

	sb.WriteString("\n## Diff 概要\n")
	if len(diffSummary) > 2000 {
		sb.WriteString(diffSummary[:2000] + "\n...(truncated)")
	} else {
		sb.WriteString(diffSummary)
	}

	sb.WriteString("\n\n## 可用 Skills\n")
	for _, s := range index {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", s.Name, s.Description))
	}

	sb.WriteString("\n## 輸出格式 (JSON)\n")
	sb.WriteString(`{
  "frontend": ["skill-name"],
  "backend": ["skill-name"],
  "security": [],
  "business": ["skill-name"],
  "architecture": []
}`)

	return sb.String()
}

func extractJSON(text string) string {
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
