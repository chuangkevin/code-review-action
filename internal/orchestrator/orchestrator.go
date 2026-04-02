package orchestrator

import (
	"fmt"
	"strings"
	"sync"

	"github.com/kevinyoung1399/code-review-action/internal/assembler"
	"github.com/kevinyoung1399/code-review-action/internal/config"
	"github.com/kevinyoung1399/code-review-action/internal/gemini"
	"github.com/kevinyoung1399/code-review-action/internal/gitea"
	"github.com/kevinyoung1399/code-review-action/internal/notify"
	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
	"github.com/kevinyoung1399/code-review-action/internal/skills"
)

type Result struct {
	Status          string
	TotalComments   int
	CriticalCount   int
	WarningCount    int
	SuggestionCount int
}

func Run(cfg *config.Config) (*Result, error) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  🤖 AI Code Review — Team Discussion                ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	// 1. Initialize
	fmt.Printf("🔑 載入 %d 把 API key (cooldown: %ds, max retry: %d)\n", len(cfg.GeminiAPIKeys), cfg.CooldownDuration, cfg.MaxRetries)
	pool := gemini.NewKeyPool(cfg.GeminiAPIKeys, cfg.CooldownDurationTime())
	geminiClient := gemini.NewClient(pool, cfg.GeminiModel, gemini.WithMaxRetries(cfg.MaxRetries))
	giteaClient := gitea.NewClient(cfg.GiteaURL, cfg.GiteaToken)

	// 2. Fetch PR
	fmt.Printf("📋 取得 PR #%d from %s/%s...\n", cfg.PRNumber, cfg.RepoOwner, cfg.RepoName)
	prInfo, err := giteaClient.GetPRInfo(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("get PR info: %w", err)
	}
	fmt.Printf("   📌 %s\n", prInfo.Title)
	fmt.Printf("   👤 Author: %s | Branch: %s → %s\n", prInfo.User.DisplayName(), prInfo.Head.Ref, prInfo.Base.Ref)
	fmt.Printf("   📊 %d files changed, +%d -%d\n", prInfo.ChangedFiles, prInfo.Additions, prInfo.Deletions)

	fmt.Println("   📥 取得 diff...")
	diff, err := giteaClient.GetPRDiff(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("get PR diff: %w", err)
	}
	if diff == "" {
		fmt.Println("⏭️  Empty diff, skipping review")
		return &Result{Status: "success"}, nil
	}
	fmt.Printf("   📄 Diff size: %d bytes\n", len(diff))

	prCtx := reviewer.PRContext{
		Title:      prInfo.Title,
		Body:       prInfo.Body,
		Author:     prInfo.User.DisplayName(),
		Branch:     prInfo.Head.Ref,
		BaseBranch: prInfo.Base.Ref,
	}

	// 3. Classify files
	files := reviewer.ParseDiffFiles(diff)
	classification := ClassifyFiles(files)
	fmt.Println()
	fmt.Println("📂 檔案分類:")
	if len(classification.Frontend) > 0 {
		fmt.Printf("   🎨 Frontend: %v\n", classification.Frontend)
	}
	if len(classification.Backend) > 0 {
		fmt.Printf("   ⚙️  Backend:  %v\n", classification.Backend)
	}
	if len(classification.Shared) > 0 {
		fmt.Printf("   📎 Shared:   %v\n", classification.Shared)
	}

	// 4. Skill matching
	fmt.Println()
	skillMap := make(map[string][]string)
	var usedSkills []string
	if cfg.SkillsRepo != "" {
		fmt.Printf("📚 載入 Skills from %s...\n", cfg.SkillsRepo)
		skillsDir, err := skills.CloneSkillsRepo(cfg.SkillsRepo, cfg.SkillsRepoToken)
		if err != nil {
			fmt.Printf("   ⚠️  Clone 失敗: %v (繼續 review，不帶 skill)\n", err)
		} else {
			index, err := skills.LoadSkillIndex(skillsDir)
			if err != nil {
				fmt.Printf("   ⚠️  載入 skill index 失敗: %v\n", err)
			} else {
				fmt.Printf("   📖 找到 %d 個 skills，開始匹配...\n", len(index))
				matched, err := skills.MatchSkills(geminiClient, index, files, diff)
				if err != nil {
					fmt.Printf("   ⚠️  Skill matching 失敗: %v\n", err)
				} else {
					for role, skillNames := range matched {
						if len(skillNames) > 0 {
							fmt.Printf("   🎯 %s → %v\n", role, skillNames)
						}
						var contents []string
						for _, name := range skillNames {
							content, err := skills.LoadSkillContent(skillsDir, name)
							if err != nil {
								fmt.Printf("   ⚠️  載入 skill %s 失敗: %v\n", name, err)
								continue
							}
							contents = append(contents, content)
							usedSkills = append(usedSkills, name)
						}
						skillMap[role] = contents
					}
				}
			}
		}
	} else {
		fmt.Println("📚 未設定 skills_repo，跳過 skill matching")
	}

	// 5. Determine active reviewers
	activeRoles := determineActiveRoles(cfg.ReviewRoles, classification)
	fmt.Println()
	fmt.Println("👥 啟動 Reviewer Team:")
	for _, r := range activeRoles {
		fmt.Printf("   %s %s (%s)\n", reviewer.RoleEmoji(r), reviewer.RoleDisplayName(r), reviewer.RoleTitle(r))
	}

	// 6. Run reviewers in parallel
	fmt.Println()
	fmt.Println("💬 開始 Review...")
	fmt.Println("─────────────────────────────────────────")

	type reviewEntry struct {
		result *reviewer.ReviewResult
		err    error
	}
	resultsCh := make(map[string]chan reviewEntry)
	var wg sync.WaitGroup

	for _, role := range activeRoles {
		roleDiff := buildRoleDiff(role, diff, classification)
		ch := make(chan reviewEntry, 1)
		resultsCh[role] = ch
		wg.Add(1)

		go func(r, d string) {
			defer wg.Done()
			fmt.Printf("   %s %s 正在閱讀 code...\n", reviewer.RoleEmoji(r), reviewer.RoleDisplayName(r))
			res, err := reviewer.ReviewBatched(geminiClient, r, d, skillMap[r], prCtx, cfg.MaxDiffSize)
			ch <- reviewEntry{result: res, err: err}
			if err != nil {
				fmt.Printf("   %s %s ❌ 失敗: %v\n", reviewer.RoleEmoji(r), reviewer.RoleDisplayName(r), err)
			} else {
				fmt.Printf("   %s %s ✅ 完成 — 找到 %d 個問題\n", reviewer.RoleEmoji(r), reviewer.RoleDisplayName(r), len(res.InlineComments))
				if res.Summary != "" {
					fmt.Printf("      💬 \"%s\"\n", res.Summary)
				}
			}
		}(role, roleDiff)
	}
	wg.Wait()
	fmt.Println("─────────────────────────────────────────")

	// 7. Collect results + QA gate
	fmt.Println()
	fmt.Println("🔍 QA Gate — 驗證 review 品質...")

	diffFileSet := make(map[string]bool)
	for _, f := range files {
		diffFileSet[f] = true
	}

	var validResults []*reviewer.ReviewResult
	var failedRoles []string

	for _, role := range activeRoles {
		entry := <-resultsCh[role]
		if entry.err != nil {
			failedRoles = append(failedRoles, role)
			continue
		}
		validated := assembler.ValidateResult(entry.result, diffFileSet)
		filtered := len(entry.result.InlineComments) - len(validated.InlineComments)
		if filtered > 0 {
			fmt.Printf("   🧹 %s: 過濾 %d 個無效 comment\n", reviewer.RoleDisplayName(role), filtered)
		}
		validResults = append(validResults, validated)
	}

	if len(failedRoles) > len(activeRoles)/2 {
		fmt.Printf("   ❌ 超過 50%% reviewer 失敗 (%v)\n", failedRoles)
		return &Result{Status: "failed"}, fmt.Errorf("too many reviewers failed: %v", failedRoles)
	}
	fmt.Println("   ✅ QA Gate 通過")

	// 8. Assemble
	fmt.Println()
	fmt.Println("🔧 合併 Review 結果...")
	output := assembler.Assemble(validResults)
	output.FailedRoles = failedRoles
	output.Skills = dedupStrings(usedSkills)

	critical, warning, suggestion := 0, 0, 0
	for _, c := range output.InlineComments {
		switch c.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		case "suggestion":
			suggestion++
		}
	}
	fmt.Printf("   📊 %d comments: 🔴 %d critical, 🟡 %d warning, 🔵 %d suggestion\n",
		len(output.InlineComments), critical, warning, suggestion)
	if len(output.FailedRoles) > 0 {
		fmt.Printf("   ⚠️  失敗的 reviewer: %v\n", output.FailedRoles)
	}

	// 9. Post to Gitea as a proper PR Review (summary + inline comments)
	fmt.Println()
	fmt.Println("📤 發送 Review 到 Gitea...")

	// Build file link base URL (use public URL for clickable links)
	fileLinkBase := fmt.Sprintf("%s/%s/%s/src/branch/%s",
		cfg.GiteaPublicURL, cfg.RepoOwner, cfg.RepoName, prCtx.Branch)

	// Build inline comments for review
	var reviewComments []gitea.ReviewLineComment
	for _, c := range output.InlineComments {
		fileLink := fmt.Sprintf("[`%s:%d`](%s/%s#L%d)", c.File, c.Line, fileLinkBase, c.File, c.Line)
		body := fmt.Sprintf("**[%s]** %s\n\n%s", c.Severity, fileLink, c.Body)
		reviewComments = append(reviewComments, gitea.ReviewLineComment{
			Path:        c.File,
			NewPosition: c.Line,
			Body:        body,
		})
		fmt.Printf("   💬 [%s] %s:%d\n", c.Severity, c.File, c.Line)
	}

	// Build summary with file links
	summary := assembler.BuildSummaryComment(output, prCtx, cfg.PRNumber,
		prInfo.ChangedFiles, prInfo.Additions, prInfo.Deletions, fileLinkBase)

	// Determine review event type based on severity
	reviewEvent := "COMMENT"
	if critical > 0 {
		reviewEvent = "REQUEST_CHANGES"
		fmt.Println("   🚫 有 critical 問題 → REQUEST_CHANGES")
	} else if critical == 0 && warning == 0 && suggestion == 0 {
		reviewEvent = "APPROVED"
		fmt.Println("   ✅ 沒有問題 → APPROVE")
	}

	fmt.Printf("   📝 提交 Review (%d inline comments + summary)...\n", len(reviewComments))
	review := gitea.CreateReviewRequest{
		Body:     summary,
		Event:    reviewEvent,
		Comments: reviewComments,
	}
	if err := giteaClient.SubmitReview(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, review); err != nil {
		fmt.Printf("   ⚠️  Review 提交失敗: %v\n", err)
		// Fallback: post as regular comment
		fmt.Println("   🔄 Fallback: 改用一般 comment...")
		if err := giteaClient.PostComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, summary); err != nil {
			fmt.Printf("   ⚠️  Fallback 也失敗: %v\n", err)
		} else {
			fmt.Println("   ✅ Fallback comment 已發送")
		}
	} else {
		fmt.Println("   ✅ Review 已提交")
	}

	// 10. Slack notification
	if cfg.SlackWebhookURL != "" {
		if notify.ShouldNotify(cfg.SlackNotify, output) {
			fmt.Println("   📱 發送 Slack 通知...")
			prURL := fmt.Sprintf("%s/%s/%s/pulls/%d", cfg.GiteaURL, cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
			prTitle := fmt.Sprintf("#%d %s", cfg.PRNumber, prInfo.Title)
			if err := notify.SendSlack(cfg.SlackWebhookURL, output, prURL, prTitle, prInfo.User.DisplayName()); err != nil {
				fmt.Printf("   ⚠️  Slack 通知失敗: %v\n", err)
			} else {
				fmt.Println("   ✅ Slack 通知已發送")
			}
		} else {
			fmt.Printf("   📱 Slack: 策略為 %s，本次不發送\n", cfg.SlackNotify)
		}
	}

	// 11. Done
	status := "success"
	if len(failedRoles) > 0 {
		status = "partial"
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Printf("║  ✅ Review 完成 — %s                              \n", status)
	fmt.Printf("║  📊 %d comments: 🔴 %d  🟡 %d  🔵 %d              \n", len(output.InlineComments), critical, warning, suggestion)
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	return &Result{
		Status:          status,
		TotalComments:   len(output.InlineComments),
		CriticalCount:   critical,
		WarningCount:    warning,
		SuggestionCount: suggestion,
	}, nil
}

func determineActiveRoles(configuredRoles []string, c *Classification) []string {
	var active []string
	for _, r := range configuredRoles {
		switch r {
		case "frontend":
			if c.HasFrontend() {
				active = append(active, r)
			}
		case "backend":
			if c.HasBackend() {
				active = append(active, r)
			}
		default:
			active = append(active, r)
		}
	}
	return active
}

func buildRoleDiff(role, fullDiff string, c *Classification) string {
	switch role {
	case "frontend":
		return filterDiff(fullDiff, append(c.Frontend, c.Shared...))
	case "backend":
		return filterDiff(fullDiff, append(c.Backend, c.Shared...))
	default:
		return fullDiff
	}
}

func filterDiff(fullDiff string, files []string) string {
	if len(files) == 0 {
		return fullDiff
	}

	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}

	lines := strings.Split(fullDiff, "\n")
	var result []string
	include := false

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				path := strings.TrimPrefix(parts[3], "b/")
				include = fileSet[path]
			}
		}
		if include {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func dedupStrings(ss []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
