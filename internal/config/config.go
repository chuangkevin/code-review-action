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
	GiteaURL    string
	RepoOwner   string
	RepoName    string
	PRNumber    int
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

	// PR number from event JSON
	cfg.PRNumber = loadPRNumberFromEvent()

	if cfg.PRNumber == 0 {
		return nil, fmt.Errorf("could not determine PR number (check GITHUB_EVENT_PATH)")
	}

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

// loadPRNumberFromEvent reads the PR number from the GitHub Actions event JSON.
// Gitea Actions sets GITHUB_EVENT_PATH to a file containing the webhook payload.
func loadPRNumberFromEvent() int {
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return 0
	}

	data, err := os.ReadFile(eventPath)
	if err != nil {
		fmt.Printf("⚠️  無法讀取 event file: %v\n", err)
		return 0
	}

	var event struct {
		Number      int `json:"number"`
		PullRequest struct {
			Number int `json:"number"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		fmt.Printf("⚠️  無法解析 event JSON: %v\n", err)
		return 0
	}

	if event.PullRequest.Number > 0 {
		return event.PullRequest.Number
	}
	return event.Number
}
