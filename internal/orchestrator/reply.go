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

type commentMatch struct {
	Body string
	ID   int
	File string
	Line int
}

// RunReply handles a developer's reply to a review comment.
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

	// 1. Find the AI review
	fmt.Println()
	fmt.Println("🔍 尋找相關的 AI review comment...")

	reviews, err := giteaClient.GetPRReviews(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("get reviews: %w", err)
	}

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

	// 2. Get review comments + issue comments
	reviewComments, err := giteaClient.GetReviewComments(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, aiReview.ID)
	if err != nil {
		return nil, fmt.Errorf("get review comments: %w", err)
	}

	issueComments, err := giteaClient.GetIssueComments(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("get issue comments: %w", err)
	}

	// 3. Find the original AI comment being replied to
	match := findOriginalAIComment(cfg.CommentID, cfg.CommentBody, reviewComments, issueComments)

	if match.Body == "" {
		fmt.Println("   ⚠️  無法判斷這是在回覆哪個 review comment，跳過")
		return &Result{Status: "success"}, nil
	}

	fmt.Println()
	fmt.Printf("📝 原始 comment:\n   %s\n", truncateReply(match.Body, 150))
	fmt.Printf("   📎 File: %s, Line: %d, ID: %d\n", match.File, match.Line, match.ID)
	fmt.Printf("💬 %s 的回覆:\n   %s\n", cfg.CommentUser, truncateReply(cfg.CommentBody, 150))

	// 4. Detect reviewer persona
	originalRole := reviewer.DetectRoleFromComment(match.Body)
	fmt.Printf("   🎭 由 %s %s 回覆\n", reviewer.RoleEmoji(originalRole), reviewer.RoleDisplayName(originalRole))

	// 5. Evaluate the reply
	fmt.Println()
	fmt.Println("🤔 評估回覆...")
	evalResult, err := reviewer.EvaluateReply(geminiClient, match.Body, cfg.CommentBody, cfg.CommentUser)
	if err != nil {
		return nil, fmt.Errorf("evaluate reply: %w", err)
	}

	if evalResult.Resolved {
		fmt.Printf("   ✅ Resolved — %s\n", evalResult.Reply)
	} else {
		fmt.Printf("   💬 需要進一步討論 — %s\n", evalResult.Reply)
	}

	// 6. Post reply as a new review with inline comment on the same file:line
	replyBody := formatReplyComment(evalResult, originalRole)

	if match.File != "" && match.Line > 0 {
		fmt.Printf("   📎 發送 review comment 到 %s:%d\n", match.File, match.Line)
		if err := giteaClient.ReplyAsReview(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, match.File, match.Line, replyBody); err != nil {
			fmt.Printf("   ⚠️  Review reply 失敗: %v, fallback 到一般 comment\n", err)
			giteaClient.PostComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, replyBody)
		} else {
			fmt.Println("   ✅ 回覆已發送到 Files Changed")
		}
	} else {
		fmt.Println("   📎 無 file:line 資訊，發送為一般 comment")
		giteaClient.PostComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, replyBody)
	}

	// 7. Cross-domain check
	fmt.Println()
	fmt.Println("🔍 檢查是否涉及其他 domain...")
	crossDomainRoles := detectCrossDomain(geminiClient, match.Body, cfg.CommentBody, originalRole)
	for _, crossRole := range crossDomainRoles {
		fmt.Printf("   ⚡ %s %s 有話要說...\n", reviewer.RoleEmoji(crossRole), reviewer.RoleDisplayName(crossRole))
		crossResult, err := reviewer.EvaluateCrossDomain(geminiClient, match.Body, cfg.CommentBody, cfg.CommentUser, crossRole)
		if err != nil {
			fmt.Printf("   ⚠️  %s 回覆失敗: %v\n", reviewer.RoleDisplayName(crossRole), err)
			continue
		}
		crossBody := formatReplyComment(crossResult, crossRole)
		if match.File != "" && match.Line > 0 {
			if err := giteaClient.ReplyAsReview(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, match.File, match.Line, crossBody); err != nil {
				giteaClient.PostComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, crossBody)
			}
		} else {
			giteaClient.PostComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, crossBody)
		}
		fmt.Printf("   ✅ %s 已補充意見\n", reviewer.RoleDisplayName(crossRole))
	}

	// 8. Check if ALL resolved → approve
	if evalResult.Resolved {
		allResolved := checkAllResolved(reviewComments)
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

// findOriginalAIComment finds which AI review comment the developer is replying to.
func findOriginalAIComment(replyCommentID int, replyBody string, reviewComments []gitea.ReviewCommentDetail, issueComments []gitea.IssueComment) commentMatch {
	// Strategy 1: Look backwards from the reply in issue comments
	for i, c := range issueComments {
		if c.ID == replyCommentID {
			for j := i - 1; j >= 0; j-- {
				if isAIComment(issueComments[j].Body) {
					// Try to find matching review comment for file:line
					for _, rc := range reviewComments {
						if rc.Body == issueComments[j].Body || strings.Contains(rc.Body, issueComments[j].Body) || strings.Contains(issueComments[j].Body, rc.Body) {
							return commentMatch{Body: rc.Body, ID: rc.ID, File: rc.Path, Line: rc.Line}
						}
					}
					return commentMatch{Body: issueComments[j].Body, ID: issueComments[j].ID}
				}
			}
		}
	}

	// Strategy 2: Find related review inline comment
	for _, rc := range reviewComments {
		if isAIComment(rc.Body) && seemsRelated(replyBody, rc.Body) {
			return commentMatch{Body: rc.Body, ID: rc.ID, File: rc.Path, Line: rc.Line}
		}
	}

	// Strategy 3: Single unresolved comment
	var unresolved []gitea.ReviewCommentDetail
	for _, rc := range reviewComments {
		if isAIComment(rc.Body) && rc.Resolver == nil {
			unresolved = append(unresolved, rc)
		}
	}
	if len(unresolved) == 1 {
		return commentMatch{Body: unresolved[0].Body, ID: unresolved[0].ID, File: unresolved[0].Path, Line: unresolved[0].Line}
	}

	// Strategy 4: Most recent AI review comment
	for i := len(reviewComments) - 1; i >= 0; i-- {
		if isAIComment(reviewComments[i].Body) {
			rc := reviewComments[i]
			return commentMatch{Body: rc.Body, ID: rc.ID, File: rc.Path, Line: rc.Line}
		}
	}

	return commentMatch{}
}

func isAIComment(body string) bool {
	markers := []string{"**Shield**", "**Rex**", "**Aria**", "**Biz**", "**Arch**", "[critical]", "[warning]", "[suggestion]"}
	for _, m := range markers {
		if strings.Contains(body, m) {
			return true
		}
	}
	return false
}

func seemsRelated(reply, originalComment string) bool {
	words := strings.Fields(reply)
	for _, w := range words {
		if len(w) > 3 && strings.Contains(originalComment, w) {
			return true
		}
	}
	return false
}

func checkAllResolved(reviewComments []gitea.ReviewCommentDetail) bool {
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
