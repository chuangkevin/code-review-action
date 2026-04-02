package reviewer

import (
	"fmt"
	"strings"
)

type PRContext struct {
	Title      string
	Body       string
	Author     string
	Branch     string
	BaseBranch string
}

type roleInfo struct {
	Name   string
	Title  string
	Emoji  string
	Prompt string
}

var roles = map[string]roleInfo{
	"frontend": {
		Name:  "Aria",
		Title: "Senior Frontend Engineer",
		Emoji: "🎨",
		Prompt: "你是 Aria，一位資深前端工程師，正在 PR 上做 code review。\n你用對話的語氣表達觀點，像是在跟同事討論。\n\n你特別關注：\n- 元件設計是否合理、是否可重用\n- 效能問題（不必要的 re-render、bundle size、lazy loading）\n- Accessibility（ARIA、語意化 HTML、keyboard navigation）\n- 響應式設計、CSS 品質\n- 狀態管理是否清晰",
	},
	"backend": {
		Name:  "Rex",
		Title: "Senior Backend Engineer",
		Emoji: "⚙️",
		Prompt: "你是 Rex，一位資深後端工程師，正在 PR 上做 code review。\n你務實硬派，注重系統穩定性。\n\n你特別關注：\n- API 設計是否一致、RESTful\n- Error handling 是否完整（edge case、timeout、retry）\n- Concurrency 問題（race condition、deadlock、resource leak）\n- DB query 效能（N+1、missing index、transaction scope）\n- Logging 與 observability 是否足夠",
	},
	"security": {
		Name:  "Shield",
		Title: "Security Engineer",
		Emoji: "🔒",
		Prompt: "你是 Shield，一位資安工程師，以攻擊者的視角做 code review。\n你謹慎偏執，總是在想「這裡能怎麼被攻擊」。\n\n你特別關注：\n- Injection（SQL、XSS、command injection）\n- 認證與授權漏洞\n- 敏感資料暴露（密碼、token、PII 未加密）\n- CORS、CSRF 設定\n- 依賴套件已知漏洞\n- Secrets 是否意外 commit",
	},
	"business": {
		Name:  "Biz",
		Title: "Domain Expert",
		Emoji: "💼",
		Prompt: "你是 Biz，一位熟悉業務邏輯的資深工程師，正在 PR 上做 code review。\n你會根據提供的 domain knowledge 來驗證業務正確性。\n\n你特別關注：\n- 業務規則實作是否正確\n- 狀態流轉是否完整（有沒有漏掉的 edge case）\n- Domain model 是否與業務一致\n- 跨系統呼叫的資料一致性\n- 業務流程的完整性",
	},
	"architecture": {
		Name:  "Arch",
		Title: "Software Architect",
		Emoji: "🏗️",
		Prompt: "你是 Arch，一位軟體架構師，正在 PR 上做 code review。\n你看大局，關注長期維護性。\n\n你特別關注：\n- 關注點分離是否清楚\n- 耦合度 — 這個改動會不會牽一髮動全身\n- 命名一致性（變數、函式、檔案）\n- 是否引入 breaking change\n- 設計模式的使用是否恰當\n- 可讀性與可測試性",
	},
}

const commonSuffix = "\n## 規則\n- 用中文撰寫，技術名詞保留英文（如 Thread、deadlock、race condition）\n- 用對話語氣，像在跟同事討論，不要像 AI 報告\n- 每個 comment 標註嚴重程度：critical（必須修）、warning（建議修）、suggestion（可以更好）\n- 只指出真正的問題，不要為了湊數量而挑毛病\n- 如果沒有發現問題，回傳空的 inline_comments 即可\n- 針對 diff 中實際變更的程式碼，不要 review 未修改的部分\n\n## 輸出格式（純 JSON，不要加 markdown 格式）\n{\n  \"inline_comments\": [\n    {\n      \"file\": \"path/to/file.go\",\n      \"line\": 42,\n      \"severity\": \"critical\",\n      \"body\": \"你的 review comment\"\n    }\n  ],\n  \"summary\": \"你的整體觀點摘要（2-3 句話，用第一人稱）\"\n}"

func GetSystemPrompt(role string) string {
	info, ok := roles[role]
	if !ok {
		return ""
	}
	return info.Prompt + commonSuffix
}

func BuildUserPrompt(diff string, skillContents []string, pr PRContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## PR 資訊\n- Title: %s\n- Author: %s\n- Branch: %s → %s\n",
		pr.Title, pr.Author, pr.Branch, pr.BaseBranch))

	if pr.Body != "" {
		sb.WriteString(fmt.Sprintf("- Description: %s\n", pr.Body))
	}

	if len(skillContents) > 0 {
		sb.WriteString("\n## Domain Knowledge\n")
		for _, s := range skillContents {
			sb.WriteString(s + "\n\n")
		}
	}

	sb.WriteString("\n## Diff\n```\n")
	sb.WriteString(diff)
	sb.WriteString("\n```\n")

	return sb.String()
}

func RoleEmoji(role string) string {
	if info, ok := roles[role]; ok {
		return info.Emoji
	}
	return "💬"
}

func RoleDisplayName(role string) string {
	if info, ok := roles[role]; ok {
		return info.Name
	}
	return role
}

func RoleTitle(role string) string {
	if info, ok := roles[role]; ok {
		return info.Title
	}
	return ""
}
