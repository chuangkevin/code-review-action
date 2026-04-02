# AI Code Review Action for Gitea

Multi-role AI code review action，在 PR 建立/更新時自動觸發。使用 Gemini 2.5 Flash，透過 5 個 reviewer agent 平行 review，以 PR Review 形式提交 inline comment + 團隊討論總結。

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

在 Gitea **組織或 repo** 的 **Settings → Actions → Secrets** 中建立：

| Secret 名稱 | 必要 | 說明 |
|---------------|------|------|
| `REVIEW_TOKEN` | **必要** | Gitea API token，用於讀取 PR diff 和發表 review comment。到 **個人 Settings → Applications → Generate New Token** 建立。注意：不能用 `GITEA_` 或 `GITHUB_` 開頭（保留前綴） |
| `GEMINI_API_KEYS` | **必要** | Gemini API keys，多把用逗號分隔（例如 `key1,key2,key3`）。到 [Google AI Studio](https://aistudio.google.com/apikey) 取得 |
| `SKILLS_REPO_TOKEN` | 選填 | 用來 clone Skills repo 的 Gitea token（如果 repo 是 private），只需 `repository` Read 權限 |
| `SLACK_WEBHOOK_URL` | 選填 | Slack Incoming Webhook URL，用於發送 review 通知 |

#### REVIEW_TOKEN 權限

| 權限 | 用途 |
|------|------|
| `repository` Read | 讀取 PR 資訊和 diff |
| `repository` Write | 提交 PR Review（inline comment） |
| `issue` Write | 發表總結 comment（fallback） |

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
        uses: https://gitea.housefun.com.tw/HP_TOOL/code-review-action@main
        with:
          gitea_token: ${{ secrets.REVIEW_TOKEN }}
          gemini_api_keys: ${{ secrets.GEMINI_API_KEYS }}
          gitea_public_url: https://gitea.housefun.com.tw
          skills_repo: http://srvhpgit:32000/HP/HPSkills.git
```

> **注意**：`uses` 必須用完整 Gitea URL（Gitea runner 預設會去 github.com 找）。
> `skills_repo` 用 runner 可連到的內部地址，`gitea_public_url` 用外部地址（給連結用）。

### 3. 全部設定參數

| 參數 | 預設值 | 說明 |
|------|--------|------|
| `gitea_token` | _(必要)_ | Gitea API token |
| `gemini_api_keys` | _(必要)_ | Gemini API keys，逗號分隔 |
| `gemini_model` | `gemini-2.5-flash` | Gemini model 名稱 |
| `gitea_public_url` | _(自動偵測)_ | Gitea 的公開 URL（用於 comment 中的連結）。Runner 內部的 `GITHUB_SERVER_URL` 可能是內部地址，設定此值確保連結可點擊 |
| `skills_repo` | _(空)_ | Skills repo URL。設定後 AI 會自動讀取 skill 的 frontmatter（name + description），根據 PR diff 內容判斷載入哪些 domain knowledge |
| `skills_repo_token` | _(空)_ | Clone skills repo 用的 token（private repo 才需要） |
| `slack_webhook_url` | _(空)_ | Slack Incoming Webhook URL |
| `slack_notify` | `on_issues` | Slack 通知策略：`always` / `on_issues` / `off` |
| `language` | `zh-TW` | Review 語言 |
| `max_diff_size` | `100000` | 單次 review 最大 diff 大小 (bytes)，超過自動分批 |
| `review_roles` | `frontend,backend,security,business,architecture` | 啟用的 reviewer，逗號分隔 |
| `cooldown_duration` | `120` | API key 遭 429 後的冷卻秒數 |
| `max_retries` | `10` | 429 最大重試次數 |

## 功能特色

- **PR Review 形式** — 以正式 PR Review 提交，inline comment 標在 diff 的具體行上，不是一般留言
- **多角色平行 review** — 5 個 agent 各有專長和人設，像真人團隊在討論
- **Skill 自動匹配** — 從 Skills repo 讀取 domain knowledge，AI 根據 PR diff 內容自動選出相關 skill 注入 review context
- **API Key Pool** — 多把 key 加權隨機分配，429 自動冷卻換 key，最多重試 10 次
- **大 PR 分批** — 超過 token 上限自動按檔案分批 review
- **QA Gate** — 自動驗證 AI 輸出品質（過濾無效 comment、修正 severity）
- **去重合併** — 多個 reviewer 指出同一行問題時，合併成一則 comment
- **可點擊連結** — Summary 中每個問題都有 file:line 連結
- **稱呼全名** — 使用 Gitea 使用者的 full_name 而非 login ID
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
