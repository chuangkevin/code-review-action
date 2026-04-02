package config

import (
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

	cfg.GiteaURL = os.Getenv("GITEA_SERVER_URL")
	cfg.RepoOwner = os.Getenv("GITEA_REPO_OWNER")
	cfg.RepoName = os.Getenv("GITEA_REPO_NAME")

	prStr := os.Getenv("GITEA_PR_NUMBER")
	if prStr != "" {
		pr, err := strconv.Atoi(prStr)
		if err == nil {
			cfg.PRNumber = pr
		}
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
