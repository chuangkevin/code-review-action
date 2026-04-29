# Code Review Action — Behavior Spec

## 觸發

- `pull_request: [opened, synchronize]` → 跑完整 review
- `issue_comment: [created]` → 評估開發者回覆，由原 reviewer 角色回應

## Reviewer 團隊（5 個 agent）

| Role | Persona | 關注 |
|------|---------|------|
| frontend | Aria | 元件、效能、a11y、響應式、CSS |
| backend | Rex | API、error handling、concurrency、DB |
| security | Shield | OWASP、injection、auth、敏感資料 |
| business | Biz | 業務規則、狀態流轉、domain model |
| architecture | Arch | 耦合、分層、命名、breaking change |

5 個 agent 平行 review，各自 Gemini call。前後端 reviewer 依檔案分類選擇性啟用，security/business/architecture 永遠跑。

## Skill 自動匹配

設定 `skills_repo` 後：

1. Clone skill repo（`--depth=1`，需要 `skills_repo_token` 認證 Gitea instance）
2. 解析每個資料夾的 `SKILL.md` frontmatter（`name` + `description`）建索引
3. 用 Gemini 一次 call 根據 diff 選出每 role 最多 3 個 skill
4. 載入命中的 skill content 注入對應 reviewer 的 user prompt

## Review 輸出格式

PR Review 一次提交（`POST /repos/{owner}/{repo}/pulls/{n}/reviews`），含：

### Summary（PR Review body）

```
## 🤖 Code Review — Team Discussion

> 📚 **本次參考的 Domain Knowledge**
> - 🏗️ **Arch**: skill-a, skill-b
> - ⚙️ **Rex**: skill-c

💬 **Arch**: {作者名}, {2-3 句總結}
💬 **Rex**: {作者名}, ...
```

- 頂部 quote block 顯示 per-role skill（只在有任一 role 用到 skill 時出現）
- 每個 reviewer summary 必須以 PR 作者中文姓名開頭（例：「哲愷, ...」），不可用角色前綴（如「Arch:」）

### Inline comments

- 每則 ≤ 100 字（中文），聚焦單一問題 + 一句修法
- 不列 bullet 推薦清單、不堆疊 class 名稱
- 跨檔案／整體觀察寫到 summary 不寫 inline
- 同檔同行多個 reviewer 時 merge

### Review event

- 任一 critical → `REQUEST_CHANGES`
- 全部 0 → `APPROVED`
- 其他 → `COMMENT`

## API 呼叫策略

### 強制 JSON 輸出

所有 Gemini 呼叫使用 `responseMimeType: application/json`，避免 model 在 prompt 衝突下吐純文字。

### Retry

| 錯誤 | 處理 |
|------|------|
| 429 Rate Limited | 標記該 key cooldown（`cooldown_duration`），換下一把 key 立即重試 |
| 5xx (TransientError) | 同一把 key，exponential backoff (1s/2s/4s/8s, max 16s) 重試 |
| 4xx 其他 | 立即失敗，不 retry |

`max_retries` 是上限（含兩種 retry）。

### Key Pool

多把 key 加權隨機分配。429 換 key + cooldown，5xx 不 cooldown（不是 key 的問題）。

## 失敗處理

- 5 個 reviewer 失敗超過半數 → 整體失敗，回 `partial`
- ≤ 半數 → 繼續，summary 標 `⚠️ 以下角色 review 未完成: ...`
- Skill clone 失敗 → 警告 + 不帶 skill 繼續 review

## 對話式 Review

Issue comment 觸發時：
1. 偵測該 comment 是不是 reply 到某個 inline review comment
2. 若是，由原始 reviewer 角色（看 file:line）評估開發者回覆是否合理
3. 若回覆涉及其他 domain，對應 reviewer 也會補充
4. 用 quoted reply 格式留在 Conversation tab（Gitea 1.25 API 不支援 thread reply，待升級）

## 版本管理

- 標 SemVer tag（`v1.1.2`）並建 Gitea Release
- consumer workflow 用 `@v1.x.y` 而非 `@main` 或 commit SHA
