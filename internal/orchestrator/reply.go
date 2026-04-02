package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kevinyoung1399/code-review-action/internal/config"
	"github.com/kevinyoung1399/code-review-action/internal/gemini"
	"github.com/kevinyoung1399/code-review-action/internal/gitea"
	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
)

// RunReply handles a developer's reply to a review comment.
// It evaluates the reply, responds, and approves if all issues are resolved.
func RunReply(cfg *config.Config) (*Result, error) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  💬 AI Code Review — Reply Evaluation                ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	pool := gemini.NewKeyPool(cfg.GeminiAPIKeys, cfg.CooldownDurationTime())
	geminiClient := gemini.NewClient(pool, cfg.GeminiModel, gemini.WithMaxRetries(cfg.MaxRetries))
	giteaClient := gitea.NewClient(cfg.GiteaURL, cfg.GiteaToken)

	fmt.Printf("📋 PR #%d — %s 回覆了 review comment\n", cfg.PRNumber, cfg.CommentUser)
	fmt.Printf("   💬 \"%s\"\n", truncateReply(cfg.CommentBody, 100))

	// 1. Find the AI review and its comments
	fmt.Println()
	fmt.Println("🔍 尋找相關的 AI review comment...")

	reviews, err := giteaClient.GetPRReviews(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("get reviews: %w", err)
	}

	// Find the AI review (look for our bot's review with Team Discussion header)
	var aiReview *gitea.Review
	for i := len(reviews) - 1; i >= 0; i-- {
		if strings.Contains(reviews[i].Body, "Code Review — Team Discussion") {
			aiReview = &reviews[i]
			break
		}
	}

	if aiReview == nil {
		fmt.Println("   ⚠️  找不到 AI review，跳過")
		return &Result{Status: "success"}, nil
	}
	fmt.Printf("   ✅ 找到 AI review (ID: %d)\n", aiReview.ID)

	// 2. Get the review's inline comments
	reviewComments, err := giteaClient.GetReviewComments(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, aiReview.ID)
	if err != nil {
		return nil, fmt.Errorf("get review comments: %w", err)
	}

	// 3. Get all issue comments to find the reply context
	issueComments, err := giteaClient.GetIssueComments(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("get issue comments: %w", err)
	}

	// 4. Find which AI comment the developer is replying to
	// Gitea puts review comment replies in issue comments, referencing the original
	// We match by finding the developer's comment and the AI comment right before it
	originalComment := findOriginalAIComment(cfg.CommentID, cfg.CommentBody, reviewComments, issueComments)

	if originalComment == "" {
		fmt.Println("   ⚠️  無法判斷這是在回覆哪個 review comment，跳過")
		return &Result{Status: "success"}, nil
	}

	fmt.Println()
	fmt.Printf("📝 原始 comment:\n   %s\n", truncateReply(originalComment, 150))
	fmt.Printf("💬 %s 的回覆:\n   %s\n", cfg.CommentUser, truncateReply(cfg.CommentBody, 150))

	// 5. Detect which reviewer persona should reply
	originalRole := reviewer.DetectRoleFromComment(originalComment)
	fmt.Printf("   🎭 由 %s %s 回覆\n", reviewer.RoleEmoji(originalRole), reviewer.RoleDisplayName(originalRole))

	// 6. Evaluate the reply
	fmt.Println()
	fmt.Println("🤔 評估回覆...")
	evalResult, err := reviewer.EvaluateReply(geminiClient, originalComment, cfg.CommentBody, cfg.CommentUser)
	if err != nil {
		return nil, fmt.Errorf("evaluate reply: %w", err)
	}

	// 7. Post the original reviewer's response
	if evalResult.Resolved {
		fmt.Printf("   ✅ Resolved — %s\n", evalResult.Reply)
	} else {
		fmt.Printf("   💬 需要進一步討論 — %s\n", evalResult.Reply)
	}

	replyBody := formatReplyComment(evalResult, originalRole)
	// Reply in the review thread, not as a standalone comment
	if err := giteaClient.ReplyToReviewComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, aiReview.ID, replyBody); err != nil {
		fmt.Printf("   ⚠️  Review thread 回覆失敗: %v, fallback 到一般 comment\n", err)
		if err := giteaClient.PostComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, replyBody); err != nil {
			fmt.Printf("   ⚠️  Fallback 也失敗: %v\n", err)
		} else {
			fmt.Println("   ✅ 回覆已發送（一般 comment）")
		}
	} else {
		fmt.Println("   ✅ 回覆已發送到 review thread")
	}

	// 7.5. If resolved, try to resolve the comment thread
	if evalResult.Resolved && cfg.CommentID > 0 {
		fmt.Println("   🔒 嘗試 resolve comment thread...")
		if err := giteaClient.ResolveComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, cfg.CommentID); err != nil {
			fmt.Printf("   ⚠️  Resolve 失敗（可能 Gitea 版本不支援）: %v\n", err)
		}
	}

	// 8. Cross-domain check — does the developer's reply affect other domains?
	fmt.Println()
	fmt.Println("🔍 檢查是否涉及其他 domain...")
	crossDomainRoles := detectCrossDomain(geminiClient, originalComment, cfg.CommentBody, originalRole)
	for _, crossRole := range crossDomainRoles {
		fmt.Printf("   ⚡ %s %s 有話要說...\n", reviewer.RoleEmoji(crossRole), reviewer.RoleDisplayName(crossRole))
		crossResult, err := reviewer.EvaluateCrossDomain(geminiClient, originalComment, cfg.CommentBody, cfg.CommentUser, crossRole)
		if err != nil {
			fmt.Printf("   ⚠️  %s 回覆失敗: %v\n", reviewer.RoleDisplayName(crossRole), err)
			continue
		}
		crossBody := formatReplyComment(crossResult, crossRole)
		if err := giteaClient.ReplyToReviewComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, aiReview.ID, crossBody); err != nil {
			fmt.Printf("   ⚠️  %s 回覆發送失敗: %v\n", reviewer.RoleDisplayName(crossRole), err)
		} else {
			fmt.Printf("   ✅ %s 已補充意見\n", reviewer.RoleDisplayName(crossRole))
		}
	}

	// 7. Check if ALL review comments are now resolved → approve
	if evalResult.Resolved {
		allResolved := checkAllResolved(reviewComments, issueComments, cfg.CommentID)
		if allResolved {
			fmt.Println()
			fmt.Println("🎉 所有問題已解決，提交 APPROVE...")
			approveReview := gitea.CreateReviewRequest{
				Body:  "✅ 所有 review comment 已確認解決。LGTM!",
				Event: "APPROVED",
			}
			if err := giteaClient.SubmitReview(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, approveReview); err != nil {
				fmt.Printf("   ⚠️  Approve 失敗: %v\n", err)
			} else {
				fmt.Println("   ✅ PR 已 Approve")
			}
		} else {
			fmt.Println("   📌 還有其他未解決的 comment，暫不 approve")
		}
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  ✅ Reply 處理完成                                   ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")

	return &Result{Status: "success"}, nil
}

// findOriginalAIComment tries to find which AI review comment the developer is replying to.
func findOriginalAIComment(replyCommentID int, replyBody string, reviewComments []gitea.ReviewCommentDetail, issueComments []gitea.IssueComment) string {
	// Strategy 1: Find the issue comment matching the reply, then look at context
	// In Gitea, review thread replies appear as issue comments.
	// The reply might reference the original review comment by being in the same thread.
	for i, c := range issueComments {
		if c.ID == replyCommentID {
			// Look backwards for the nearest AI comment
			for j := i - 1; j >= 0; j-- {
				if isAIComment(issueComments[j].Body) {
					return issueComments[j].Body
				}
			}
		}
	}

	// Strategy 2: Check review inline comments — find one that seems related
	for _, rc := range reviewComments {
		if isAIComment(rc.Body) && seemsRelated(replyBody, rc.Body) {
			return rc.Body
		}
	}

	// Strategy 3: If there's only one unresolved AI review comment, assume it's that one
	var unresolvedComments []string
	for _, rc := range reviewComments {
		if isAIComment(rc.Body) && rc.Resolver == nil {
			unresolvedComments = append(unresolvedComments, rc.Body)
		}
	}
	if len(unresolvedComments) == 1 {
		return unresolvedComments[0]
	}

	// Strategy 4: If reply doesn't match any specific comment but there are AI comments,
	// pick the most recent one (last in list)
	for i := len(reviewComments) - 1; i >= 0; i-- {
		if isAIComment(reviewComments[i].Body) {
			return reviewComments[i].Body
		}
	}

	return ""
}

func isAIComment(body string) bool {
	// AI comments contain role markers like **Shield**, **Rex**, etc.
	markers := []string{"**Shield**", "**Rex**", "**Aria**", "**Biz**", "**Arch**", "[critical]", "[warning]", "[suggestion]"}
	for _, m := range markers {
		if strings.Contains(body, m) {
			return true
		}
	}
	return false
}

func seemsRelated(reply, originalComment string) bool {
	// Simple heuristic: check if reply mentions file names or key terms from original
	words := strings.Fields(reply)
	for _, w := range words {
		if len(w) > 3 && strings.Contains(originalComment, w) {
			return true
		}
	}
	return false
}

func checkAllResolved(reviewComments []gitea.ReviewCommentDetail, issueComments []gitea.IssueComment, currentReplyID int) bool {
	// Count AI comments that are still unresolved
	for _, rc := range reviewComments {
		if isAIComment(rc.Body) && rc.Resolver == nil {
			return false
		}
	}
	return true
}

func formatReplyComment(result *reviewer.ConversationResult, role string) string {
	emoji := reviewer.RoleEmoji(role)
	name := reviewer.RoleDisplayName(role)
	title := reviewer.RoleTitle(role)

	if result.Resolved {
		return fmt.Sprintf("%s **%s** · %s\n\n✅ %s", emoji, name, title, result.Reply)
	}
	return fmt.Sprintf("%s **%s** · %s\n\n💬 %s", emoji, name, title, result.Reply)
}

// detectCrossDomain asks Gemini if the developer's reply touches other domains.
func detectCrossDomain(client *gemini.Client, originalComment, developerReply, originalRole string) []string {
	allRoles := []string{"frontend", "backend", "security", "business", "architecture"}
	var otherRoles []string
	for _, r := range allRoles {
		if r != originalRole {
			otherRoles = append(otherRoles, r)
		}
	}

	systemPrompt := `你是一位技術主管。開發者對一個 code review comment 做了回覆。
判斷這個回覆是否牽涉到其他技術領域，需要其他 reviewer 介入。

例如：
- 開發者說「這是內部工具不需要驗證」→ Security 需要介入
- 開發者說「我改用 cache 避免 N+1」→ Architecture 可能要看
- 開發者說「這個欄位是給前端用的」→ Frontend 可能要確認

只回傳需要介入的角色，如果不需要就回空陣列。
回傳純 JSON 陣列，例如 ["security"] 或 []`

	userPrompt := fmt.Sprintf("## 原始 comment\n%s\n\n## 開發者回覆\n%s\n\n## 可選角色\n%v",
		originalComment, developerReply, otherRoles)

	text, err := client.Generate(systemPrompt, userPrompt)
	if err != nil {
		return nil
	}

	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	var roles []string
	if err := json.Unmarshal([]byte(text), &roles); err != nil {
		return nil
	}

	// Validate roles
	validSet := make(map[string]bool)
	for _, r := range otherRoles {
		validSet[r] = true
	}
	var valid []string
	for _, r := range roles {
		if validSet[r] {
			valid = append(valid, r)
		}
	}
	return valid
}

func truncateReply(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
