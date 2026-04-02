package orchestrator

import (
	"fmt"
	"log"
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
	log.Println("[orchestrator] starting review pipeline")

	pool := gemini.NewKeyPool(cfg.GeminiAPIKeys, cfg.CooldownDurationTime())
	geminiClient := gemini.NewClient(pool, cfg.GeminiModel, gemini.WithMaxRetries(cfg.MaxRetries))
	giteaClient := gitea.NewClient(cfg.GiteaURL, cfg.GiteaToken)

	log.Printf("[orchestrator] fetching PR #%d from %s/%s", cfg.PRNumber, cfg.RepoOwner, cfg.RepoName)
	prInfo, err := giteaClient.GetPRInfo(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("get PR info: %w", err)
	}

	diff, err := giteaClient.GetPRDiff(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("get PR diff: %w", err)
	}
	if diff == "" {
		log.Println("[orchestrator] empty diff, skipping review")
		return &Result{Status: "success"}, nil
	}

	prCtx := reviewer.PRContext{
		Title:      prInfo.Title,
		Body:       prInfo.Body,
		Author:     prInfo.User.Login,
		Branch:     prInfo.Head.Ref,
		BaseBranch: prInfo.Base.Ref,
	}

	files := reviewer.ParseDiffFiles(diff)
	classification := ClassifyFiles(files)
	log.Printf("[orchestrator] classified %d files: frontend=%d, backend=%d, shared=%d",
		len(files), len(classification.Frontend), len(classification.Backend), len(classification.Shared))

	skillMap := make(map[string][]string)
	var usedSkills []string
	if cfg.SkillsRepo != "" {
		log.Println("[orchestrator] loading skills from", cfg.SkillsRepo)
		skillsDir, err := skills.CloneSkillsRepo(cfg.SkillsRepo, cfg.SkillsRepoToken)
		if err != nil {
			log.Printf("[orchestrator] WARN: failed to clone skills repo: %v (continuing without skills)", err)
		} else {
			index, err := skills.LoadSkillIndex(skillsDir)
			if err != nil {
				log.Printf("[orchestrator] WARN: failed to load skill index: %v", err)
			} else {
				log.Printf("[orchestrator] loaded %d skills, matching...", len(index))
				matched, err := skills.MatchSkills(geminiClient, index, files, diff)
				if err != nil {
					log.Printf("[orchestrator] WARN: skill matching failed: %v", err)
				} else {
					for role, skillNames := range matched {
						var contents []string
						for _, name := range skillNames {
							content, err := skills.LoadSkillContent(skillsDir, name)
							if err != nil {
								log.Printf("[orchestrator] WARN: failed to load skill %s: %v", name, err)
								continue
							}
							contents = append(contents, content)
							usedSkills = append(usedSkills, name)
						}
						skillMap[role] = contents
					}
					log.Printf("[orchestrator] matched skills: %v", matched)
				}
			}
		}
	}

	activeRoles := determineActiveRoles(cfg.ReviewRoles, classification)
	log.Printf("[orchestrator] active reviewers: %v", activeRoles)

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
			log.Printf("[%s] starting review...", r)
			res, err := reviewer.ReviewBatched(geminiClient, r, d, skillMap[r], prCtx, cfg.MaxDiffSize)
			ch <- reviewEntry{result: res, err: err}
			if err != nil {
				log.Printf("[%s] FAILED: %v", r, err)
			} else {
				log.Printf("[%s] done: %d comments", r, len(res.InlineComments))
			}
		}(role, roleDiff)
	}
	wg.Wait()

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
		validResults = append(validResults, validated)
	}

	if len(failedRoles) > len(activeRoles)/2 {
		log.Printf("[orchestrator] >50%% reviewers failed (%v), marking as failed", failedRoles)
		return &Result{Status: "failed"}, fmt.Errorf("too many reviewers failed: %v", failedRoles)
	}

	output := assembler.Assemble(validResults)
	output.FailedRoles = failedRoles
	output.Skills = dedupStrings(usedSkills)

	for _, c := range output.InlineComments {
		body := fmt.Sprintf("**[%s]**\n\n%s", c.Severity, c.Body)
		err := giteaClient.PostReviewComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, gitea.ReviewComment{
			Body:   body,
			Path:   c.File,
			NewPos: c.Line,
		})
		if err != nil {
			log.Printf("[orchestrator] WARN: failed to post inline comment on %s:%d: %v", c.File, c.Line, err)
		}
	}

	summary := assembler.BuildSummaryComment(output, prCtx, cfg.PRNumber,
		prInfo.ChangedFiles, prInfo.Additions, prInfo.Deletions)
	if err := giteaClient.PostComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, summary); err != nil {
		log.Printf("[orchestrator] WARN: failed to post summary: %v", err)
	}

	if cfg.SlackWebhookURL != "" && notify.ShouldNotify(cfg.SlackNotify, output) {
		prURL := fmt.Sprintf("%s/%s/%s/pulls/%d", cfg.GiteaURL, cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
		prTitle := fmt.Sprintf("#%d %s", cfg.PRNumber, prInfo.Title)
		if err := notify.SendSlack(cfg.SlackWebhookURL, output, prURL, prTitle, prInfo.User.Login); err != nil {
			log.Printf("[orchestrator] WARN: slack notification failed: %v", err)
		}
	}

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

	status := "success"
	if len(failedRoles) > 0 {
		status = "partial"
	}

	log.Printf("[orchestrator] review complete: %s (%d comments: %d critical, %d warning, %d suggestion)",
		status, len(output.InlineComments), critical, warning, suggestion)

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
