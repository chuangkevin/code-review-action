package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	GiteaToken    string
	GeminiAPIKeys []string
	GeminiModel      string
	SkillsRepo       string
	SkillsRepoToken  string
	SlackWebhookURL  string
	SlackNotify      string
	Language         string
	MaxDiffSize      int
	ReviewRoles      []string
	CooldownDuration int
	MaxRetries       int
	GiteaURL       string // internal URL for API calls (from GITHUB_SERVER_URL)
	GiteaPublicURL string // external URL for links in comments
	RepoOwner      string
	RepoName       string
	PRNumber       int

	// Event context
	EventName string // "pull_request" or "issue_comment"
	CommentID int    // for issue_comment events
	CommentBody string
	CommentUser string
}

func (c *Config) CooldownDurationTime() time.Duration {
	return time.Duration(c.CooldownDuration) * time.Second
}

func Load() (*Config, error) {
	giteaToken := getInput("GITEA_TOKEN")
	if giteaToken == "" {
		return nil, fmt.Errorf("INPUT_GITEA_TOKEN is required")
	}

	keysRaw := getInput("GEMINI_API_KEYS")
	if keysRaw == "" {
		return nil, fmt.Errorf("INPUT_GEMINI_API_KEYS is required")
	}
	keys := splitAndTrim(keysRaw)
	if len(keys) == 0 {
		return nil, fmt.Errorf("INPUT_GEMINI_API_KEYS must contain at least one key")
	}

	rolesRaw := getInputDefault("REVIEW_ROLES", "frontend,backend,security,business,architecture")
	roles := splitAndTrim(rolesRaw)

	cfg := &Config{
		GiteaToken:       giteaToken,
		GeminiAPIKeys:    keys,
		GeminiModel:      getInputDefault("GEMINI_MODEL", "gemini-2.5-flash"),
		SkillsRepo:       getInput("SKILLS_REPO"),
		SkillsRepoToken:  getInput("SKILLS_REPO_TOKEN"),
		SlackWebhookURL:  getInput("SLACK_WEBHOOK_URL"),
		SlackNotify:      getInputDefault("SLACK_NOTIFY", "on_issues"),
		Language:         getInputDefault("LANGUAGE", "zh-TW"),
		MaxDiffSize:      getInputIntDefault("MAX_DIFF_SIZE", 100000),
		ReviewRoles:      roles,
		CooldownDuration: getInputIntDefault("COOLDOWN_DURATION", 120),
		MaxRetries:       getInputIntDefault("MAX_RETRIES", 10),
	}

	// Gitea Actions uses GITHUB_* env vars for compatibility
	cfg.GiteaURL = os.Getenv("GITHUB_SERVER_URL")
	if cfg.GiteaURL == "" {
		cfg.GiteaURL = os.Getenv("GITEA_SERVER_URL")
	}

	// Public URL for links in comments (defaults to GiteaURL if not set)
	cfg.GiteaPublicURL = getInput("GITEA_PUBLIC_URL")
	if cfg.GiteaPublicURL == "" {
		cfg.GiteaPublicURL = cfg.GiteaURL
	}

	// GITHUB_REPOSITORY = "owner/repo"
	fullRepo := os.Getenv("GITHUB_REPOSITORY")
	if fullRepo != "" {
		parts := strings.SplitN(fullRepo, "/", 2)
		if len(parts) == 2 {
			cfg.RepoOwner = parts[0]
			cfg.RepoName = parts[1]
		}
	}
	if cfg.RepoOwner == "" {
		cfg.RepoOwner = os.Getenv("GITEA_REPO_OWNER")
	}
	if cfg.RepoName == "" {
		cfg.RepoName = os.Getenv("GITEA_REPO_NAME")
	}

	// Event type and context
	cfg.EventName = os.Getenv("GITHUB_EVENT_NAME")
	event := loadEvent()
	cfg.PRNumber = event.PRNumber
	cfg.CommentID = event.CommentID
	cfg.CommentBody = event.CommentBody
	cfg.CommentUser = event.CommentUser

	return cfg, nil
}

func getInput(name string) string {
	return strings.TrimSpace(os.Getenv("INPUT_" + name))
}

func getInputDefault(name, defaultVal string) string {
	v := getInput(name)
	if v == "" {
		return defaultVal
	}
	return v
}

func getInputIntDefault(name string, defaultVal int) int {
	v := getInput(name)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

type eventData struct {
	PRNumber    int
	CommentID   int
	CommentBody string
	CommentUser string
}

func loadEvent() eventData {
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return eventData{}
	}

	data, err := os.ReadFile(eventPath)
	if err != nil {
		fmt.Printf("⚠️  無法讀取 event file: %v\n", err)
		return eventData{}
	}

	// Debug: print first 500 chars of event JSON to understand structure
	preview := string(data)
	if len(preview) > 500 {
		preview = preview[:500]
	}
	fmt.Printf("📋 Event JSON preview:\n%s\n---\n", preview)

	var raw struct {
		Number      int `json:"number"`
		PullRequest struct {
			Number int `json:"number"`
		} `json:"pull_request"`
		Issue struct {
			Number int `json:"number"`
		} `json:"issue"`
		Comment struct {
			ID   int    `json:"id"`
			Body string `json:"body"`
			User struct {
				Login    string `json:"login"`
				FullName string `json:"full_name"`
			} `json:"user"`
		} `json:"comment"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Printf("⚠️  無法解析 event JSON: %v\n", err)
		return eventData{}
	}

	ed := eventData{
		CommentID:   raw.Comment.ID,
		CommentBody: raw.Comment.Body,
		CommentUser: raw.Comment.User.FullName,
	}
	if ed.CommentUser == "" {
		ed.CommentUser = raw.Comment.User.Login
	}

	// PR number: try pull_request.number, then issue.number, then top-level number
	if raw.PullRequest.Number > 0 {
		ed.PRNumber = raw.PullRequest.Number
	} else if raw.Issue.Number > 0 {
		ed.PRNumber = raw.Issue.Number
	} else {
		ed.PRNumber = raw.Number
	}

	return ed
}
