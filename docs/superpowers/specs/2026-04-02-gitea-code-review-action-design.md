# Gitea Code Review Action — Design Spec

## Overview

一個 Gitea Action，在 PR 建立/更新時自動觸發 AI code review。使用 Gemini 2.5 Flash 作為 LLM，透過多角色 reviewer agent 平行 review，產出帶有團隊討論風格的 inline comment 和總結 comment。

## 核心需求

- **觸發**：PR 建立/更新時自動觸發
- **輸出**：Inline comment（在對應程式碼行上）+ 總結 comment（團隊討論風格）
- **AI 模型**：Gemini 2.5 Flash
- **語言**：中文為主，技術名詞保留英文
- **技術棧**：Go（編譯成單一 binary，Docker scratch image < 15MB）
- **平台**：內部 Gitea

---

## 架構：Multi-Role Orchestrator

```
Gitea PR Event (webhook)
  │
  ▼
┌─────────────────────────────────────────────────┐
│  Orchestrator                                   │
│                                                 │
│  1. 從 Gitea API 取得 PR metadata + diff        │
│  2. Clone HPSkills repo (帶 token)              │
│  3. 呼叫 Skill Matcher                          │
│  4. 依檔案副檔名/路徑分類 → 分派 Reviewer        │
│  5. 平行執行 Reviewer（各自從 key pool 取 key）   │
│  6. QA Gate 驗證回傳品質                         │
│  7. Assembler 合併結果、去重                     │
│  8. Post comments to Gitea PR                   │
│  9. 依設定發 Slack 通知                          │
└─────────────────────────────────────────────────┘
```

### 非 Review 角色

| Agent | 職責 |
|-------|------|
| **Orchestrator** | 主流程控制：取 diff → 分類 → 分派 → 收集 → 通知 |
| **Skill Matcher** | 讀 HPSkills frontmatter，呼叫 Gemini 選出各 reviewer 需要的 skill |
| **Assembler** | 合併所有 reviewer 結果、去重、產出最終 comments |

### Review 角色（各自有人設）

| Agent | 人設 | 觀點 | 關注面向 |
|-------|------|------|----------|
| 🎨 **Aria** | Senior Frontend Engineer | 使用者體驗與前端品質 | 元件設計、效能（bundle size, render）、accessibility、響應式、CSS |
| ⚙️ **Rex** | Senior Backend Engineer | 系統穩定性與工程品質 | API 設計、error handling、concurrency、DB query 效能、logging |
| 🔒 **Shield** | Security Engineer | 攻擊者視角 | OWASP Top 10、injection、auth/authz、敏感資料暴露、secrets |
| 💼 **Biz** | Domain Expert | 業務正確性 | 業務規則、edge case、domain model 一致性、flow 完整性 |
| 🏗️ **Arch** | Software Architect | 長期維護性 | 耦合度、關注點分離、命名一致性、breaking change、可讀性 |

---

## 檔案分類規則

```
Frontend: .vue, .tsx, .jsx, .ts (在 components/ pages/ views/ 下),
          .css, .scss, .less, .html, .svelte
Backend:  .go, .cs, .java, .py, .rs, .rb,
          .ts (在 server/ api/ services/ 下)
Shared:   兩邊都不明確的 → 同時給 Frontend + Backend
```

- Security / Business / Architecture 看**所有檔案**
- 如果 PR 只有前端檔案 → 不啟動 Backend Reviewer（反之亦然）

---

## Key Pool 設計

參考 project-bridge 的 `geminiKeys.ts`，針對 Action 場景調整。

### 設定方式

環境變數 `GEMINI_API_KEYS`，逗號分隔多把 key。

### Key 狀態機

```
                    ┌──────────┐
         正常使用    │ Available │
        ┌──────────▶│  (idle)   │◀─────────────┐
        │           └─────┬─────┘              │
        │                 │ GetKey()           │ cooldown 到期
        │                 ▼                     │ (2 分鐘)
        │           ┌──────────┐          ┌─────┴──────┐
        │           │ In Use   │──429───▶ │ Cooldown   │
        │           └────┬─────┘          │ (2 min)    │
        │                │ 成功            └────────────┘
        │                │ Release()
        └────────────────┘
          usage count +1
```

### 選取策略：Usage-Weighted Random

不是純隨機，也不是嚴格最少 — 用量越少被選中機率越高：

```
Key A: usage 100 → weight 1/100
Key B: usage 20  → weight 1/20
Key C: usage 50  → weight 1/50
→ B 被選中機率最高，但 A、C 仍有機會
```

### 共享 Pool，按需取用

Agent 不預先分配 key。每次要呼叫 Gemini API 時：
1. 從 pool `GetKey()` 取一把可用 key
2. 呼叫 API
3. 成功 → `Release(key)`，usage +1
4. 429 → `MarkCooldown(key)`，2 分鐘冷卻，立即 `GetKey()` 取下一把

### Retry 策略（429）

```
429 發生
  ├─ 標記當前 key → Cooldown (2 min)
  ├─ 從 Available keys 中，按 usage 加權隨機取新 key
  ├─ 如果沒有 Available key → 等最短 cooldown 的那把到期
  ├─ 用新 key retry
  └─ 重複最多 10 次
       └─ 10 次都失敗 → 該 reviewer 標記失敗
```

### Usage 追蹤

```go
type KeyState struct {
    Key         string
    Status      string    // "available" | "in_use" | "cooldown"
    UsageCount  int       // 本次 action 執行期間的累計呼叫次數
    CooldownAt  time.Time // 進入 cooldown 的時間
}
```

`UsageCount` 只計算本次 action 執行的用量（Action 是短生命週期，不跨次）。

### 失敗處理

| 情境 | 處理 |
|------|------|
| 單個 reviewer 429 | mark cooldown → 換 key → retry 最多 10 次 |
| 單個 reviewer 完全失敗 | 跳過該角色，總結中標註 |
| 超過 50% reviewer 失敗 | 整個 review 標記為失敗，Slack 通知 |
| Skill repo clone 失敗 | 降級為無 skill review，繼續 |

---

## Skill Matching

### 流程

1. Clone HPSkills repo（帶 `skills_repo_token`）
2. 掃描所有 `skills/*/SKILL.md` → 讀取 YAML frontmatter（name + description）
3. 組成 skill index
4. 一次 Gemini call：輸入 PR diff 摘要 + skill index → 輸出各 reviewer 應載入的 skill

### Skill Matcher Prompt

```
你是一個 skill matcher。根據以下 PR 變更內容，從 skill 清單中選出與本次 review 相關的 skill，
並指派給對應的 reviewer 角色。

## PR 變更摘要
- 變更檔案: [file list]
- Diff 概要: [前 2000 字元，或 orchestrator 摘要]

## 可用 Skills
[skill index: name + description]

## Reviewer 角色
- frontend: 前端品質
- backend: 系統穩定性
- security: 安全性
- business: 業務邏輯正確性
- architecture: 長期維護性

## 輸出格式 (JSON)
{
  "frontend": ["skill-name-1"],
  "backend": ["skill-name-2"],
  "security": [],
  "business": ["skill-name-3"],
  "architecture": []
}
```

### 優化

- diff 太長時，orchestrator 先產出摘要再給 skill matcher
- 沒有 `skills_repo` 設定時，跳過 skill matching
- 多檔 skill 被選中後，載入 SKILL.md + 所有子檔案

---

## Reviewer Agent 設計

### 共通介面

```go
type ReviewRequest struct {
    Role      string        // "frontend" | "backend" | "security" | "business" | "architecture"
    Diff      string        // 該 reviewer 負責的 diff
    Files     []FileChange  // 檔案清單 + metadata
    Skills    []string      // skill 完整內容
    PRContext PRContext      // PR title, description, branch name
}

type ReviewResponse struct {
    Role           string
    InlineComments []InlineComment
    Summary        string
}

type InlineComment struct {
    File     string
    Line     int
    Severity string  // "critical" | "warning" | "suggestion"
    Body     string
}
```

### 各 Reviewer Prompt

**Aria — Frontend Reviewer 🎨**
```
你是 Aria，一位資深前端工程師，正在 PR 上做 code review。
你用對話的語氣表達觀點，像是在跟同事討論。

你特別關注：
- 元件設計是否合理、是否可重用
- 效能問題（不必要的 re-render、bundle size、lazy loading）
- Accessibility（ARIA、語意化 HTML、keyboard navigation）
- 響應式設計、CSS 品質
- 狀態管理是否清晰
```

**Rex — Backend Reviewer ⚙️**
```
你是 Rex，一位資深後端工程師，正在 PR 上做 code review。
你務實硬派，注重系統穩定性。

你特別關注：
- API 設計是否一致、RESTful
- Error handling 是否完整（edge case、timeout、retry）
- Concurrency 問題（race condition、deadlock、resource leak）
- DB query 效能（N+1、missing index、transaction scope）
- Logging 與 observability 是否足夠
```

**Shield — Security Reviewer 🔒**
```
你是 Shield，一位資安工程師，以攻擊者的視角做 code review。
你謹慎偏執，總是在想「這裡能怎麼被攻擊」。

你特別關注：
- Injection（SQL、XSS、command injection）
- 認證與授權漏洞
- 敏感資料暴露（密碼、token、PII 未加密）
- CORS、CSRF 設定
- 依賴套件已知漏洞
- Secrets 是否意外 commit
```

**Biz — Business Logic Reviewer 💼**
```
你是 Biz，一位熟悉業務邏輯的資深工程師，正在 PR 上做 code review。
你會根據提供的 domain knowledge 來驗證業務正確性。

你特別關注：
- 業務規則實作是否正確
- 狀態流轉是否完整（有沒有漏掉的 edge case）
- Domain model 是否與業務一致
- 跨系統呼叫的資料一致性
- 業務流程的完整性

## Domain Knowledge
{skills_content}
```

**Arch — Architecture Reviewer 🏗️**
```
你是 Arch，一位軟體架構師，正在 PR 上做 code review。
你看大局，關注長期維護性。

你特別關注：
- 關注點分離是否清楚
- 耦合度 — 這個改動會不會牽一髮動全身
- 命名一致性（變數、函式、檔案）
- 是否引入 breaking change
- 設計模式的使用是否恰當
- 可讀性與可測試性
```

### 共通 Prompt 尾段

```
## 規則
- 用中文撰寫，技術名詞保留英文（如 Thread、deadlock、race condition）
- 用對話語氣，像在跟同事討論，不要像 AI 報告
- 每個 comment 標註嚴重程度：critical（必須修）、warning（建議修）、suggestion（可以更好）
- 只指出真正的問題，不要為了湊數量而挑毛病
- 如果沒有發現問題，回傳空的 comments 即可
- 針對 diff 中實際變更的程式碼，不要 review 未修改的部分

## 輸出格式 (JSON)
{
  "inline_comments": [
    {
      "file": "path/to/file.go",
      "line": 42,
      "severity": "critical",
      "body": "你的 review comment"
    }
  ],
  "summary": "你的整體觀點摘要（2-3 句話，用第一人稱）"
}
```

### 大 PR 分批

當單一 reviewer 的 diff 超過 token 上限（> 50 files 或 > 100KB）：

```
Reviewer 的 diff
  ├─ Batch 1 (≤ 10 files) → call 1 (GetKey)
  ├─ Batch 2 (≤ 10 files) → call 2 (GetKey)  ← 平行
  └─ Batch 3 (≤ 10 files) → call 3 (GetKey)
  └─ Merge batch results
```

---

## Assembler

### 去重邏輯

同一個 file + line 有多個 reviewer comment 時：
1. 合併成一則 comment，保留所有角色觀點
2. severity 取最高（critical > warning > suggestion）

合併範例：

```markdown
**[critical]**

🔒 **Shield** · Security Engineer
攻擊者可以在這個 input 塞 `'; DROP TABLE users; --`，
直接打穿你的 DB。改用 parameterized query 就沒事了。

⚙️ **Rex** · Backend Engineer
同意 Shield 說的，補充一點 — 用 parameterized query 之後
DB 也能 cache query plan，對效能有幫助。
```

### QA Gate

在合併前檢查每個 reviewer 的回傳：

| 檢查 | 失敗處理 |
|------|---------|
| JSON 格式合法 | retry 1 次 |
| file 存在於 diff 中 | 丟棄不存在的 comment |
| line 在 diff 範圍內 | 修正為最近的 diff 行，或丟棄 |
| summary 非空 | fallback：「{role} review 完成，未提供摘要」|
| 整個回傳為空或錯誤 | 標記該 reviewer 失敗，總結中註明 |

### 總結 Comment 格式

```markdown
## 🤖 Code Review — Team Discussion

**PR**: #{pr_number} {pr_title}
**Author**: {author} · **Branch**: {branch}
**Files**: {file_count} changed · +{additions} -{deletions}

---

💬 **Arch**: {architecture summary}

💬 **Rex**: {backend summary}

💬 **Shield**: {security summary}

💬 **Biz**: {business summary}

💬 **Aria**: {frontend summary，或 "無前端變更，跳過。"}

---

| 🔴 Critical | 🟡 Warning | 🔵 Suggestion |
|:-----------:|:----------:|:-------------:|
| {n} | {n} | {n} |

{如果有載入 skill，列出使用的 skill 名稱}
```

---

## Slack 通知

### 通知策略

| `slack_notify` 值 | 行為 |
|--------------------|------|
| `always` | 每次 review 完都發 |
| `on_issues` | 只在有 critical 或 warning 時發 |
| `off` | 不發通知 |

### 訊息格式

```
🤖 Code Review 完成

PR: #123 新增物件刊登到期檢查
Author: kevin
Branch: feature/object-expiry

🔴 Critical: 2
🟡 Warning: 3
🔵 Suggestion: 1

重點發現:
• 🔒 SQL injection 風險 (file.go:42)
• 💼 到期判斷缺少 AdType 檢查

[查看完整 Review →]
```

使用 Slack Incoming Webhook。重點發現只列 critical，最多 5 條。

---

## Action Input / Output

### Inputs

```yaml
inputs:
  gitea_token:
    description: "Gitea API token"
    required: true

  gemini_api_keys:
    description: "Gemini API keys，逗號分隔"
    required: true

  gemini_model:
    description: "Gemini model 名稱"
    required: false
    default: "gemini-2.5-flash"

  skills_repo:
    description: "HPSkills repo URL"
    required: false

  skills_repo_token:
    description: "Clone skills repo 用的 Gitea token"
    required: false

  slack_webhook_url:
    description: "Slack Incoming Webhook URL"
    required: false

  slack_notify:
    description: "Slack 通知策略: always | on_issues | off"
    required: false
    default: "on_issues"

  language:
    description: "Review 語言"
    required: false
    default: "zh-TW"

  max_diff_size:
    description: "單次 review 最大 diff 大小 (bytes)，超過則分批"
    required: false
    default: "100000"

  review_roles:
    description: "啟用的 reviewer，逗號分隔"
    required: false
    default: "frontend,backend,security,business,architecture"

  cooldown_duration:
    description: "API key 429 cooldown 時間 (秒)"
    required: false
    default: "120"

  max_retries:
    description: "429 最大重試次數"
    required: false
    default: "10"
```

### Outputs

```yaml
outputs:
  review_status:
    description: "success | partial | failed"

  total_comments:
    description: "inline comment 總數"

  critical_count:
    description: "critical 數量"

  warning_count:
    description: "warning 數量"

  suggestion_count:
    description: "suggestion 數量"
```

---

## Go 專案結構

```
code-review-action/
├── action.yml                  # Gitea Action 定義
├── Dockerfile                  # Multi-stage build → scratch
├── go.mod
├── go.sum
├── main.go                     # 入口：解析 input → 啟動 orchestrator
│
├── internal/
│   ├── config/
│   │   └── config.go           # Action inputs 解析 + 驗證
│   │
│   ├── gitea/
│   │   └── client.go           # Gitea API client（取 diff、發 comment）
│   │
│   ├── gemini/
│   │   ├── client.go           # Gemini API client
│   │   └── keypool.go          # Key pool（加權隨機、cooldown、retry）
│   │
│   ├── skills/
│   │   ├── loader.go           # Clone repo + 讀取 SKILL.md
│   │   └── matcher.go          # Skill matching（呼叫 Gemini）
│   │
│   ├── reviewer/
│   │   ├── reviewer.go         # Reviewer 共通介面 + 執行邏輯
│   │   ├── prompts.go          # 各角色的 system prompt + 人設
│   │   └── batch.go            # 大 PR 分批邏輯
│   │
│   ├── assembler/
│   │   ├── assembler.go        # 合併結果 + 去重
│   │   └── qa.go               # QA gate 檢查
│   │
│   ├── orchestrator/
│   │   └── orchestrator.go     # 主流程控制
│   │
│   └── notify/
│       └── slack.go            # Slack webhook 通知
│
└── testdata/                   # 測試用假 diff、假 skill
```

### Docker

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /review-action .

# 使用 alpine 而非 scratch — 需要 git 來 clone HPSkills repo
FROM alpine:3.19
RUN apk add --no-cache git ca-certificates
COPY --from=builder /review-action /review-action
ENTRYPOINT ["/review-action"]
```

使用 alpine（非 scratch）因為需要 git binary 來 clone skill repo。預估 image size ~20MB。
