package config

import (
	"os"
	"testing"
)

func TestLoad_RequiredFields(t *testing.T) {
	os.Setenv("INPUT_GITEA_TOKEN", "test-token")
	os.Setenv("INPUT_GEMINI_API_KEYS", "key1,key2,key3")
	defer os.Unsetenv("INPUT_GITEA_TOKEN")
	defer os.Unsetenv("INPUT_GEMINI_API_KEYS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GiteaToken != "test-token" {
		t.Errorf("GiteaToken = %q, want %q", cfg.GiteaToken, "test-token")
	}
	if len(cfg.GeminiAPIKeys) != 3 {
		t.Errorf("GeminiAPIKeys len = %d, want 3", len(cfg.GeminiAPIKeys))
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	os.Unsetenv("INPUT_GITEA_TOKEN")
	os.Unsetenv("INPUT_GEMINI_API_KEYS")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("INPUT_GITEA_TOKEN", "t")
	os.Setenv("INPUT_GEMINI_API_KEYS", "k1")
	defer os.Unsetenv("INPUT_GITEA_TOKEN")
	defer os.Unsetenv("INPUT_GEMINI_API_KEYS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GeminiModel != "gemini-2.5-flash" {
		t.Errorf("GeminiModel = %q, want %q", cfg.GeminiModel, "gemini-2.5-flash")
	}
	if cfg.SlackNotify != "on_issues" {
		t.Errorf("SlackNotify = %q, want %q", cfg.SlackNotify, "on_issues")
	}
	if cfg.CooldownDuration != 120 {
		t.Errorf("CooldownDuration = %d, want 120", cfg.CooldownDuration)
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("MaxRetries = %d, want 10", cfg.MaxRetries)
	}
	if len(cfg.ReviewRoles) != 5 {
		t.Errorf("ReviewRoles len = %d, want 5", len(cfg.ReviewRoles))
	}
}

func TestLoad_CustomRoles(t *testing.T) {
	os.Setenv("INPUT_GITEA_TOKEN", "t")
	os.Setenv("INPUT_GEMINI_API_KEYS", "k1")
	os.Setenv("INPUT_REVIEW_ROLES", "security,backend")
	defer os.Unsetenv("INPUT_GITEA_TOKEN")
	defer os.Unsetenv("INPUT_GEMINI_API_KEYS")
	defer os.Unsetenv("INPUT_REVIEW_ROLES")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.ReviewRoles) != 2 {
		t.Errorf("ReviewRoles len = %d, want 2", len(cfg.ReviewRoles))
	}
}
