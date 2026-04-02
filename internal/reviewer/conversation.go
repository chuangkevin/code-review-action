package reviewer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kevinyoung1399/code-review-action/internal/gemini"
)

type ConversationResult struct {
	Resolved bool   `json:"resolved"`
	Reply    string `json:"reply"`
}

// EvaluateReply asks Gemini whether a developer's reply resolves the review comment.
// The response comes from the original reviewer persona (e.g., Shield, Rex).
func EvaluateReply(client *gemini.Client, originalComment, developerReply, developerName string) (*ConversationResult, error) {
	// Detect which reviewer persona wrote the original comment
	role := detectRole(originalComment)
	persona := buildReplyPersona(role)

	systemPrompt := fmt.Sprintf(`%s

你正在評估開發者對你之前 review comment 的回覆。像在跟同事對話一樣回應。

## 判斷標準
- 如果開發者已修正問題 → resolved: true
- 如果開發者給出合理的技術解釋（例如：這是內部工具、有其他防護措施、設計上的取捨）→ resolved: true
- 如果開發者的解釋不夠充分或問題仍然存在 → resolved: false
- 給予善意推定，尊重開發者的判斷

## 回覆風格
- 用中文，技術名詞保留英文
- 簡潔友善，保持你的角色個性
- resolved 時：簡短確認，如果有建議可以順帶提一句
- 未 resolved 時：解釋為什麼你覺得還需要處理

## 輸出格式（純 JSON）
{
  "resolved": true,
  "reply": "你的回覆（不要加角色前綴，系統會自動加）"
}`, persona)

	userPrompt := fmt.Sprintf(`## 原始 Review Comment
%s

## %s 的回覆
%s`, originalComment, developerName, developerReply)

	text, err := client.Generate(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("evaluate reply: %w", err)
	}

	text = extractConversationJSON(text)

	var result ConversationResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parse conversation result: %w (raw: %s)", err, truncateStr(text, 200))
	}

	return &result, nil
}

func extractConversationJSON(text string) string {
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

// EvaluateCrossDomain lets another domain's reviewer chime in on the conversation.
func EvaluateCrossDomain(client *gemini.Client, originalComment, developerReply, developerName, role string) (*ConversationResult, error) {
	persona := buildReplyPersona(role)

	systemPrompt := fmt.Sprintf(`%s

另一位 reviewer 提出了一個問題，開發者做了回覆，但這個回覆涉及到你的專業領域。
請從你的角度補充意見。

## 回覆風格
- 用中文，技術名詞保留英文
- 簡潔有力，直接講重點
- 如果你覺得開發者的做法沒問題 → resolved: true
- 如果你覺得從你的角度看有風險 → resolved: false，說明原因

## 輸出格式（純 JSON）
{
  "resolved": true,
  "reply": "你的補充意見（不要加角色前綴）"
}`, persona)

	userPrompt := fmt.Sprintf(`## 原始 Review Comment（其他 reviewer 提出）
%s

## %s 的回覆
%s`, originalComment, developerName, developerReply)

	text, err := client.Generate(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("cross-domain evaluate (%s): %w", role, err)
	}

	text = extractConversationJSON(text)

	var result ConversationResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parse cross-domain result (%s): %w", role, err)
	}

	return &result, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// detectRole identifies which reviewer persona wrote the original comment.
func detectRole(comment string) string {
	roleMarkers := map[string]string{
		"**Shield**": "security",
		"**Rex**":    "backend",
		"**Aria**":   "frontend",
		"**Biz**":    "business",
		"**Arch**":   "architecture",
	}
	for marker, role := range roleMarkers {
		if strings.Contains(comment, marker) {
			return role
		}
	}
	return "backend" // default
}

// buildReplyPersona returns a persona prompt for the detected role.
func buildReplyPersona(role string) string {
	info, ok := roles[role]
	if !ok {
		return "你是一位 code reviewer。"
	}
	return fmt.Sprintf("你是 %s，%s。%s", info.Name, info.Title, info.Prompt)
}

// DetectRoleFromComment exports role detection for use by orchestrator.
func DetectRoleFromComment(comment string) string {
	return detectRole(comment)
}
