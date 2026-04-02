# AI Code Review Action for Gitea

Multi-role AI code review action，在 PR 建立/更新時自動觸發。使用 Gemini 2.5 Flash，透過 5 個 reviewer agent 平行 review，產出團隊討論風格的 inline comment 和總結 comment。

## Reviewer Agents

| Agent | 人設 | 關注面向 |
|-------|------|----------|
| 🎨 **Aria** | Senior Frontend Engineer | 元件設計、效能、accessibility、響應式、CSS |
| ⚙️ **Rex** | Senior Backend Engineer | API 設計、error handling、concurrency、DB 效能 |
| 🔒 **Shield** | Security Engineer | OWASP Top 10、injection、auth、敏感資料 |
| 💼 **Biz** | Domain Expert | 業務規則、狀態流轉、domain model |
| 🏗️ **Arch** | Software Architect | 耦合度、關注點分離、命名一致性、breaking change |

## 設定步驟

### 1. 建立 Secrets

在 Gitea repo 的 **Settings → Actions → Secrets** 中建立：

| Secret 名稱 | 必要 | 說明 |
|-------------|------|------|
| `GITEA_TOKEN` | **必要** | Gitea API token，需要有讀取 PR 和發表 comment 的權限。在 **Settings → Applications → Generate New Token** 建立，勾選 `repo` 權限 |
| `GEMINI_API_KEYS` | **必要** | Gemini API keys，多把用逗號分隔（例如 `key1,key2,key3`）。到 [Google AI Studio](https://aistudio.google.com/apikey) 取得 |
| `SKILLS_REPO_TOKEN` | 選填 | 用來 clone HPSkills repo 的 Gitea token（如果 skills repo 是 private） |
| `SLACK_WEBHOOK_URL` | 選填 | Slack Incoming Webhook URL，用於發送 review 通知。在 Slack App 的 **Incoming Webhooks** 設定取得 |

### 2. 建立 Workflow

在你的 repo 中建立 `.gitea/workflows/code-review.yml`：

```yaml
name: AI Code Review
on:
  pull_request:
    types: [opened, synchronize]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - name: AI Code Review
        uses: HP_TOOL/code-review-action@main
        with:
          gitea_token: ${{ secrets.GITEA_TOKEN }}
          gemini_api_keys: ${{ secrets.GEMINI_API_KEYS }}
          skills_repo: https://gitea.housefun.com.tw/HP/HPSkills.git
          skills_repo_token: ${{ secrets.SKILLS_REPO_TOKEN }}
          slack_webhook_url: ${{ secrets.SLACK_WEBHOOK_URL }}
```

### 3. 選填設定

以下參數可在 workflow 的 `with` 區塊中覆寫：

| 參數 | 預設值 | 說明 |
|------|--------|------|
| `gemini_model` | `gemini-2.5-flash` | Gemini model 名稱 |
| `skills_repo` | _(空)_ | HPSkills repo URL，設定後自動載入 domain knowledge |
| `slack_notify` | `on_issues` | Slack 通知策略：`always` / `on_issues` / `off` |
| `language` | `zh-TW` | Review 語言 |
| `max_diff_size` | `100000` | 單次 review 最大 diff 大小 (bytes)，超過自動分批 |
| `review_roles` | `frontend,backend,security,business,architecture` | 啟用的 reviewer，逗號分隔 |
| `cooldown_duration` | `120` | API key 遭 429 後的冷卻秒數 |
| `max_retries` | `10` | 429 最大重試次數 |

## 功能特色

- **多角色平行 review** — 5 個 agent 各有專長和人設，像真人團隊在討論
- **API Key Pool** — 多把 key 加權隨機分配，429 自動冷卻換 key，最多重試 10 次
- **Skill 自動匹配** — 從 HPSkills repo 讀取 domain knowledge，AI 自動選出相關 skill 注入 review context
- **大 PR 分批** — 超過 token 上限自動按檔案分批 review
- **QA Gate** — 自動驗證 AI 輸出品質（過濾無效 comment、修正 severity）
- **去重合併** — 多個 reviewer 指出同一行問題時，合併成一則 comment
- **Slack 通知** — 可設定每次通知或只在有問題時通知

## Outputs

| Output | 說明 |
|--------|------|
| `review_status` | `success` / `partial` / `failed` |
| `total_comments` | inline comment 總數 |
| `critical_count` | critical 數量 |
| `warning_count` | warning 數量 |
| `suggestion_count` | suggestion 數量 |

## 開發

```bash
# 建置
go build -o review-action .

# 測試
go test ./... -v

# Docker
docker build -t code-review-action .
```
