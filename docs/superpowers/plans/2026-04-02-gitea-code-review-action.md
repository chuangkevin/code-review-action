# Gitea Code Review Action Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Gitea Action that performs multi-role AI code review on PRs using Gemini 2.5 Flash, with API key pooling, skill-based domain knowledge, and Slack notifications.

**Architecture:** Orchestrator dispatches 5 reviewer agents (Frontend/Backend/Security/Business/Architecture) in parallel, each with its own persona. Shared key pool with usage-weighted random selection and 2-min cooldown. Skills auto-loaded from external HPSkills repo.

**Tech Stack:** Go 1.22, Gemini API (REST), Gitea API (REST), Slack Incoming Webhook

---

## File Structure

```
code-review-action/
├── action.yml                       # Gitea Action definition
├── Dockerfile                       # Multi-stage build → alpine
├── go.mod                           # Go module
├── main.go                          # Entry point
├── internal/
│   ├── config/
│   │   └── config.go                # Parse action inputs from env vars
│   │   └── config_test.go
│   ├── gemini/
│   │   ├── client.go                # Gemini API client (generateContent)
│   │   ├── client_test.go
│   │   ├── keypool.go               # Key pool: weighted random, cooldown, retry
│   │   └── keypool_test.go
│   ├── gitea/
│   │   ├── client.go                # Gitea API: get diff, post comments
│   │   └── client_test.go
│   ├── skills/
│   │   ├── loader.go                # Clone repo + read SKILL.md files
│   │   ├── loader_test.go
│   │   ├── matcher.go               # Call Gemini to match skills to roles
│   │   └── matcher_test.go
│   ├── reviewer/
│   │   ├── reviewer.go              # Shared reviewer interface + execution
│   │   ├── reviewer_test.go
│   │   ├── prompts.go               # Role-specific system prompts + personas
│   │   ├── prompts_test.go
│   │   ├── batch.go                 # Split large diffs into batches
│   │   └── batch_test.go
│   ├── assembler/
│   │   ├── assembler.go             # Merge results, deduplicate
│   │   ├── assembler_test.go
│   │   ├── qa.go                    # QA gate: validate reviewer output
│   │   └── qa_test.go
│   ├── orchestrator/
│   │   ├── orchestrator.go          # Main pipeline control
│   │   ├── orchestrator_test.go
│   │   └── classifier.go            # Classify files → frontend/backend/shared
│   │   └── classifier_test.go
│   └── notify/
│       ├── slack.go                 # Slack webhook notification
│       └── slack_test.go
└── testdata/
    ├── sample.diff                  # Sample unified diff for tests
    ├── skill_index.json             # Sample skill frontmatter index
    └── reviewer_response.json       # Sample reviewer JSON response
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `action.yml`
- Create: `Dockerfile`

- [ ] **Step 1: Initialize Go module**

```bash
cd d:/Projects/code-review-action
go mod init github.com/kevinyoung1399/code-review-action
```

- [ ] **Step 2: Create action.yml**

```yaml
name: "AI Code Review"
description: "Multi-role AI code review for Gitea PRs using Gemini"
author: "Kevin"

inputs:
  gitea_token:
    description: "Gitea API token"
    required: true
  gemini_api_keys:
    description: "Gemini API keys, comma-separated"
    required: true
  gemini_model:
    description: "Gemini model name"
    required: false
    default: "gemini-2.5-flash"
  skills_repo:
    description: "HPSkills repo URL"
    required: false
  skills_repo_token:
    description: "Token to clone skills repo"
    required: false
  slack_webhook_url:
    description: "Slack Incoming Webhook URL"
    required: false
  slack_notify:
    description: "Slack notification strategy: always | on_issues | off"
    required: false
    default: "on_issues"
  language:
    description: "Review language"
    required: false
    default: "zh-TW"
  max_diff_size:
    description: "Max diff size in bytes before batching"
    required: false
    default: "100000"
  review_roles:
    description: "Enabled reviewers, comma-separated"
    required: false
    default: "frontend,backend,security,business,architecture"
  cooldown_duration:
    description: "API key cooldown seconds after 429"
    required: false
    default: "120"
  max_retries:
    description: "Max retries on 429"
    required: false
    default: "10"

runs:
  using: "docker"
  image: "Dockerfile"
```

- [ ] **Step 3: Create Dockerfile**

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /review-action .

FROM alpine:3.19
RUN apk add --no-cache git ca-certificates
COPY --from=builder /review-action /review-action
ENTRYPOINT ["/review-action"]
```

- [ ] **Step 4: Create main.go skeleton**

```go
package main

import (
	"fmt"
	"os"

	"github.com/kevinyoung1399/code-review-action/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	_ = cfg
	fmt.Println("code-review-action starting...")
}
```

- [ ] **Step 5: Commit**

```bash
git add action.yml Dockerfile go.mod main.go
git commit -m "feat: project scaffolding with action.yml, Dockerfile, main.go"
```

---

### Task 2: Config

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/config/config_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/config/ -v
```
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement config.go**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// Required
	GiteaToken    string
	GeminiAPIKeys []string

	// Optional with defaults
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

	// Gitea event context (from environment)
	GiteaURL    string
	RepoOwner   string
	RepoName    string
	PRNumber    int
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

	// Parse Gitea event context
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
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd d:/Projects/code-review-action && go test ./internal/config/ -v
```
Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: config parsing from action inputs with defaults"
```

---

### Task 3: Key Pool

**Files:**
- Create: `internal/gemini/keypool.go`
- Create: `internal/gemini/keypool_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/gemini/keypool_test.go
package gemini

import (
	"testing"
	"time"
)

func TestNewKeyPool(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2", "k3"}, 120*time.Second)
	if pool == nil {
		t.Fatal("pool is nil")
	}
	if len(pool.keys) != 3 {
		t.Errorf("keys len = %d, want 3", len(pool.keys))
	}
}

func TestGetKey_ReturnsAvailable(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, 120*time.Second)
	key, err := pool.GetKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "k1" && key != "k2" {
		t.Errorf("unexpected key: %q", key)
	}
}

func TestGetKey_WeightedByUsage(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, 120*time.Second)
	// Simulate heavy usage on k1
	for i := 0; i < 100; i++ {
		pool.Release("k1")
	}

	// Over many draws, k2 should be picked much more often
	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		key, _ := pool.GetKey()
		counts[key]++
	}
	if counts["k2"] <= counts["k1"] {
		t.Errorf("k2 should be picked more often: k1=%d, k2=%d", counts["k1"], counts["k2"])
	}
}

func TestMarkCooldown_ExcludesKey(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, 2*time.Second)
	pool.MarkCooldown("k1")

	// k1 should not be returned
	for i := 0; i < 50; i++ {
		key, err := pool.GetKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key == "k1" {
			t.Fatal("got cooled down key k1")
		}
	}
}

func TestMarkCooldown_AllKeys_WaitsForRecovery(t *testing.T) {
	pool := NewKeyPool([]string{"k1"}, 100*time.Millisecond)
	pool.MarkCooldown("k1")

	// Should block briefly then return k1 after cooldown
	key, err := pool.GetKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "k1" {
		t.Errorf("expected k1 after cooldown, got %q", key)
	}
}

func TestRelease_IncrementsUsage(t *testing.T) {
	pool := NewKeyPool([]string{"k1"}, 120*time.Second)
	pool.Release("k1")
	pool.Release("k1")

	stats := pool.Stats()
	if stats["k1"] != 2 {
		t.Errorf("usage = %d, want 2", stats["k1"])
	}
}

func TestGetKey_EmptyPool(t *testing.T) {
	pool := NewKeyPool([]string{}, 120*time.Second)
	_, err := pool.GetKey()
	if err == nil {
		t.Fatal("expected error for empty pool")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/gemini/ -v
```
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement keypool.go**

```go
// internal/gemini/keypool.go
package gemini

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type keyState struct {
	key        string
	usage      int
	cooldownAt time.Time
}

type KeyPool struct {
	mu               sync.Mutex
	keys             []*keyState
	cooldownDuration time.Duration
}

func NewKeyPool(keys []string, cooldownDuration time.Duration) *KeyPool {
	states := make([]*keyState, len(keys))
	for i, k := range keys {
		states[i] = &keyState{key: k}
	}
	return &KeyPool{
		keys:             states,
		cooldownDuration: cooldownDuration,
	}
}

// GetKey returns an available key using usage-weighted random selection.
// If all keys are in cooldown, it waits for the soonest one to recover.
func (p *KeyPool) GetKey() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.keys) == 0 {
		return "", fmt.Errorf("key pool is empty")
	}

	available := p.availableLocked()
	if len(available) > 0 {
		return p.weightedSelect(available), nil
	}

	// All keys in cooldown — find the soonest recovery
	soonest := p.soonestRecoveryLocked()
	wait := time.Until(soonest)
	if wait > 0 {
		p.mu.Unlock()
		time.Sleep(wait)
		p.mu.Lock()
	}

	available = p.availableLocked()
	if len(available) == 0 {
		// Fallback: return any key (degraded mode)
		return p.keys[rand.Intn(len(p.keys))].key, nil
	}
	return p.weightedSelect(available), nil
}

func (p *KeyPool) MarkCooldown(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, ks := range p.keys {
		if ks.key == key {
			ks.cooldownAt = time.Now().Add(p.cooldownDuration)
			return
		}
	}
}

func (p *KeyPool) Release(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, ks := range p.keys {
		if ks.key == key {
			ks.usage++
			return
		}
	}
}

func (p *KeyPool) Stats() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := make(map[string]int, len(p.keys))
	for _, ks := range p.keys {
		stats[ks.key] = ks.usage
	}
	return stats
}

func (p *KeyPool) availableLocked() []*keyState {
	now := time.Now()
	var available []*keyState
	for _, ks := range p.keys {
		if ks.cooldownAt.IsZero() || now.After(ks.cooldownAt) {
			available = append(available, ks)
		}
	}
	return available
}

func (p *KeyPool) soonestRecoveryLocked() time.Time {
	var soonest time.Time
	for _, ks := range p.keys {
		if !ks.cooldownAt.IsZero() {
			if soonest.IsZero() || ks.cooldownAt.Before(soonest) {
				soonest = ks.cooldownAt
			}
		}
	}
	return soonest
}

// weightedSelect picks a key with probability inversely proportional to usage.
// Keys with lower usage are more likely to be selected.
func (p *KeyPool) weightedSelect(available []*keyState) string {
	if len(available) == 1 {
		return available[0].key
	}

	// Weight = 1 / (usage + 1) to avoid division by zero
	weights := make([]float64, len(available))
	totalWeight := 0.0
	for i, ks := range available {
		w := 1.0 / float64(ks.usage+1)
		weights[i] = w
		totalWeight += w
	}

	r := rand.Float64() * totalWeight
	cumulative := 0.0
	for i, w := range weights {
		cumulative += w
		if r <= cumulative {
			return available[i].key
		}
	}
	return available[len(available)-1].key
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd d:/Projects/code-review-action && go test ./internal/gemini/ -v -count=1
```
Expected: All 6 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/gemini/
git commit -m "feat: key pool with usage-weighted random selection and cooldown"
```

---

### Task 4: Gemini Client

**Files:**
- Create: `internal/gemini/client.go`
- Create: `internal/gemini/client_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/gemini/client_test.go
package gemini

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_Generate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API key is in URL
		if r.URL.Query().Get("key") == "" {
			t.Error("missing API key in request")
		}
		resp := GenerateResponse{
			Candidates: []Candidate{{
				Content: Content{
					Parts: []Part{{Text: `{"result": "ok"}`}},
				},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	pool := NewKeyPool([]string{"test-key"}, 120*time.Second)
	client := NewClient(pool, "gemini-2.5-flash", WithBaseURL(server.URL))

	text, err := client.Generate("system prompt", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != `{"result": "ok"}` {
		t.Errorf("text = %q, want %q", text, `{"result": "ok"}`)
	}
}

func TestClient_Generate_429Retry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limited"}`))
			return
		}
		resp := GenerateResponse{
			Candidates: []Candidate{{
				Content: Content{
					Parts: []Part{{Text: "success"}},
				},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	pool := NewKeyPool([]string{"k1", "k2", "k3"}, 100*time.Millisecond)
	client := NewClient(pool, "gemini-2.5-flash", WithBaseURL(server.URL), WithMaxRetries(10))

	text, err := client.Generate("sys", "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "success" {
		t.Errorf("text = %q, want %q", text, "success")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestClient_Generate_AllRetriesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "rate limited"}`))
	}))
	defer server.Close()

	pool := NewKeyPool([]string{"k1"}, 50*time.Millisecond)
	client := NewClient(pool, "gemini-2.5-flash", WithBaseURL(server.URL), WithMaxRetries(3))

	_, err := client.Generate("sys", "user")
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/gemini/ -v -run TestClient
```
Expected: FAIL — `NewClient` not defined

- [ ] **Step 3: Implement client.go**

```go
// internal/gemini/client.go
package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

type Client struct {
	pool       *KeyPool
	model      string
	baseURL    string
	maxRetries int
	httpClient *http.Client
}

type ClientOption func(*Client)

func WithBaseURL(url string) ClientOption {
	return func(c *Client) { c.baseURL = url }
}

func WithMaxRetries(n int) ClientOption {
	return func(c *Client) { c.maxRetries = n }
}

func NewClient(pool *KeyPool, model string, opts ...ClientOption) *Client {
	c := &Client{
		pool:       pool,
		model:      model,
		baseURL:    defaultBaseURL,
		maxRetries: 10,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Generate sends a prompt to Gemini and returns the text response.
// On 429, it marks the key as cooldown and retries with a different key.
func (c *Client) Generate(systemPrompt, userPrompt string) (string, error) {
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		key, err := c.pool.GetKey()
		if err != nil {
			return "", fmt.Errorf("no API key available: %w", err)
		}

		text, err := c.doRequest(key, systemPrompt, userPrompt)
		if err == nil {
			c.pool.Release(key)
			return text, nil
		}

		if isRateLimited(err) {
			c.pool.MarkCooldown(key)
			continue
		}
		return "", err
	}
	return "", fmt.Errorf("all %d retries exhausted due to rate limiting", c.maxRetries)
}

func (c *Client) doRequest(apiKey, systemPrompt, userPrompt string) (string, error) {
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, c.model, apiKey)

	reqBody := GenerateRequest{
		SystemInstruction: &Content{
			Parts: []Part{{Text: systemPrompt}},
		},
		Contents: []Content{{
			Role:  "user",
			Parts: []Part{{Text: userPrompt}},
		}},
		GenerationConfig: &GenerationConfig{
			Temperature:  0.2,
			MaxOutputTokens: 8192,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", &RateLimitError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, respBody)
	}

	var genResp GenerateResponse
	if err := json.Unmarshal(respBody, &genResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}

	return genResp.Candidates[0].Content.Parts[0].Text, nil
}

// Request/Response types

type GenerateRequest struct {
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
	Contents          []Content         `json:"contents"`
	GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`
}

type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type GenerateResponse struct {
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Content Content `json:"content"`
}

// RateLimitError represents a 429 response

type RateLimitError struct {
	StatusCode int
	Body       string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited (status %d): %s", e.StatusCode, e.Body)
}

func isRateLimited(err error) bool {
	_, ok := err.(*RateLimitError)
	return ok
}
```

- [ ] **Step 4: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/gemini/ -v -count=1
```
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/gemini/
git commit -m "feat: Gemini API client with 429 retry and key pool integration"
```

---

### Task 5: Gitea Client

**Files:**
- Create: `internal/gitea/client.go`
- Create: `internal/gitea/client_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/gitea/client_test.go
package gitea

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPRDiff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/owner/repo/pulls/1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/plain" {
			t.Errorf("expected Accept: text/plain, got %s", r.Header.Get("Accept"))
		}
		w.Write([]byte("diff --git a/file.go b/file.go\n+new line"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	diff, err := client.GetPRDiff("owner", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff == "" {
		t.Fatal("diff is empty")
	}
}

func TestGetPRInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pr := PRInfo{
			Number: 1,
			Title:  "test PR",
			Body:   "description",
			User:   PRUser{Login: "kevin"},
			Head:   PRBranch{Ref: "feature/test"},
			Base:   PRBranch{Ref: "main"},
		}
		json.NewEncoder(w).Encode(pr)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	pr, err := client.GetPRInfo("owner", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Title != "test PR" {
		t.Errorf("title = %q, want %q", pr.Title, "test PR")
	}
}

func TestPostReviewComment(t *testing.T) {
	var received ReviewComment
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.PostReviewComment("owner", "repo", 1, ReviewComment{
		Body:     "test comment",
		Path:     "file.go",
		NewPos:   42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received.Body != "test comment" {
		t.Errorf("body = %q, want %q", received.Body, "test comment")
	}
}

func TestPostComment(t *testing.T) {
	var received map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.PostComment("owner", "repo", 1, "summary text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received["body"] != "summary text" {
		t.Errorf("body = %q, want %q", received["body"], "summary text")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/gitea/ -v
```
Expected: FAIL

- [ ] **Step 3: Implement client.go**

```go
// internal/gitea/client.go
package gitea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type PRInfo struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	User      PRUser   `json:"user"`
	Head      PRBranch `json:"head"`
	Base      PRBranch `json:"base"`
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
	ChangedFiles int   `json:"changed_files"`
}

type PRUser struct {
	Login string `json:"login"`
}

type PRBranch struct {
	Ref string `json:"ref"`
}

type ReviewComment struct {
	Body   string `json:"body"`
	Path   string `json:"path,omitempty"`
	NewPos int    `json:"new_position,omitempty"`
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) GetPRInfo(owner, repo string, prNumber int) (*PRInfo, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d", c.baseURL, owner, repo, prNumber)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get PR info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get PR info (status %d): %s", resp.StatusCode, body)
	}

	var pr PRInfo
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode PR info: %w", err)
	}
	return &pr, nil
}

func (c *Client) GetPRDiff(owner, repo string, prNumber int) (string, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d", c.baseURL, owner, repo, prNumber)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	c.setHeaders(req)
	req.Header.Set("Accept", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("get PR diff: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read diff: %w", err)
	}
	return string(body), nil
}

// PostReviewComment posts an inline review comment on a specific file+line.
func (c *Client) PostReviewComment(owner, repo string, prNumber int, comment ReviewComment) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews", c.baseURL, owner, repo, prNumber)

	body, _ := json.Marshal(comment)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post review comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("post review comment (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

// PostComment posts a general comment on the PR (for summary).
func (c *Client) PostComment(owner, repo string, prNumber int, body string) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, prNumber)

	payload, _ := json.Marshal(map[string]string{"body": body})
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("post comment (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "token "+c.token)
}
```

- [ ] **Step 4: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/gitea/ -v
```
Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/gitea/
git commit -m "feat: Gitea API client for PR diff, info, and comments"
```

---

### Task 6: Skill Loader

**Files:**
- Create: `internal/skills/loader.go`
- Create: `internal/skills/loader_test.go`
- Create: `testdata/skills/test-skill-doc/SKILL.md`
- Create: `testdata/skills/multi-skill-doc/SKILL.md`
- Create: `testdata/skills/multi-skill-doc/sub-topic.md`

- [ ] **Step 1: Create test skill files**

```markdown
<!-- testdata/skills/test-skill-doc/SKILL.md -->
---
name: test-skill-doc
description: Test skill for unit testing. Trigger when test or mock is mentioned.
---

# Test Skill

This is a test skill with some domain knowledge.

## Rules
- Rule 1
- Rule 2
```

```markdown
<!-- testdata/skills/multi-skill-doc/SKILL.md -->
---
name: multi-skill-doc
description: Multi-file skill for testing. Trigger when multi or complex is mentioned.
---

# Multi Skill

See sub-topics for details.

## Details
See sub-topic.md
```

```markdown
<!-- testdata/skills/multi-skill-doc/sub-topic.md -->
# Sub Topic

Detailed content for the sub topic.
```

- [ ] **Step 2: Write failing test**

```go
// internal/skills/loader_test.go
package skills

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "skills")
}

func TestLoadSkillIndex(t *testing.T) {
	index, err := LoadSkillIndex(testdataPath())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(index) < 2 {
		t.Fatalf("expected at least 2 skills, got %d", len(index))
	}

	found := false
	for _, s := range index {
		if s.Name == "test-skill-doc" {
			found = true
			if s.Description == "" {
				t.Error("description is empty")
			}
		}
	}
	if !found {
		t.Error("test-skill-doc not found in index")
	}
}

func TestLoadSkillContent(t *testing.T) {
	content, err := LoadSkillContent(testdataPath(), "test-skill-doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Fatal("content is empty")
	}
}

func TestLoadSkillContent_MultiFile(t *testing.T) {
	content, err := LoadSkillContent(testdataPath(), "multi-skill-doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Fatal("content is empty")
	}
	// Should include sub-topic content
	if !containsString(content, "Sub Topic") {
		t.Error("content should include sub-topic")
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && contains(s, sub))
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/skills/ -v
```
Expected: FAIL

- [ ] **Step 4: Implement loader.go**

```go
// internal/skills/loader.go
package skills

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CloneSkillsRepo clones the skills repo into a temp directory and returns the path.
func CloneSkillsRepo(repoURL, token string) (string, error) {
	dir, err := os.MkdirTemp("", "skills-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	cloneURL := repoURL
	if token != "" {
		// Insert token into URL for authentication
		cloneURL = strings.Replace(repoURL, "://", "://"+token+"@", 1)
	}

	cmd := exec.Command("git", "clone", "--depth=1", cloneURL, dir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("git clone: %w", err)
	}

	return filepath.Join(dir, "skills"), nil
}

// LoadSkillIndex reads all SKILL.md frontmatters from the skills directory.
func LoadSkillIndex(skillsDir string) ([]SkillEntry, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	var index []SkillEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); os.IsNotExist(err) {
			continue
		}

		name, desc, err := parseFrontmatter(skillFile)
		if err != nil {
			continue
		}

		index = append(index, SkillEntry{
			Name:        name,
			Description: desc,
		})
	}
	return index, nil
}

// LoadSkillContent reads the full content of a skill (SKILL.md + all sub-files).
func LoadSkillContent(skillsDir, skillName string) (string, error) {
	skillDir := filepath.Join(skillsDir, skillName)
	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return "", fmt.Errorf("read skill dir %s: %w", skillName, err)
	}

	var parts []string

	// SKILL.md first
	skillFile := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(skillFile)
	if err != nil {
		return "", fmt.Errorf("read SKILL.md: %w", err)
	}
	parts = append(parts, string(content))

	// Then all other .md files
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "SKILL.md" || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(skillDir, entry.Name()))
		if err != nil {
			continue
		}
		parts = append(parts, string(content))
	}

	return strings.Join(parts, "\n\n---\n\n"), nil
}

// parseFrontmatter extracts name and description from YAML frontmatter.
func parseFrontmatter(path string) (string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	name := ""
	desc := ""

	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if inFrontmatter {
				break // End of frontmatter
			}
			inFrontmatter = true
			continue
		}
		if !inFrontmatter {
			continue
		}

		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, "\"'")
		}
		if strings.HasPrefix(line, "description:") {
			desc = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			desc = strings.Trim(desc, "\"'")
		}
	}

	if name == "" {
		return "", "", fmt.Errorf("no name in frontmatter")
	}
	return name, desc, nil
}
```

- [ ] **Step 5: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/skills/ -v
```
Expected: All 3 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/skills/loader.go internal/skills/loader_test.go testdata/skills/
git commit -m "feat: skill loader to parse HPSkills frontmatter and content"
```

---

### Task 7: Skill Matcher

**Files:**
- Create: `internal/skills/matcher.go`
- Create: `internal/skills/matcher_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/skills/matcher_test.go
package skills

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kevinyoung1399/code-review-action/internal/gemini"
)

func TestMatchSkills(t *testing.T) {
	matchResult := map[string][]string{
		"frontend":     {},
		"backend":      {"test-skill-doc"},
		"security":     {},
		"business":     {"multi-skill-doc"},
		"architecture": {},
	}
	resultJSON, _ := json.Marshal(matchResult)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := gemini.GenerateResponse{
			Candidates: []gemini.Candidate{{
				Content: gemini.Content{
					Parts: []gemini.Part{{Text: string(resultJSON)}},
				},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	pool := gemini.NewKeyPool([]string{"test-key"}, 120*time.Second)
	client := gemini.NewClient(pool, "gemini-2.5-flash", gemini.WithBaseURL(server.URL))

	index := []SkillEntry{
		{Name: "test-skill-doc", Description: "Test skill"},
		{Name: "multi-skill-doc", Description: "Multi skill"},
	}

	result, err := MatchSkills(client, index, []string{"file.go"}, "diff content here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result["backend"]) != 1 || result["backend"][0] != "test-skill-doc" {
		t.Errorf("unexpected backend skills: %v", result["backend"])
	}
	if len(result["business"]) != 1 || result["business"][0] != "multi-skill-doc" {
		t.Errorf("unexpected business skills: %v", result["business"])
	}
}

func TestMatchSkills_EmptyIndex(t *testing.T) {
	result, err := MatchSkills(nil, []SkillEntry{}, []string{"file.go"}, "diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All roles should have empty skill list
	for _, v := range result {
		if len(v) != 0 {
			t.Errorf("expected empty skills, got %v", v)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/skills/ -v -run TestMatch
```
Expected: FAIL — `MatchSkills` not defined

- [ ] **Step 3: Implement matcher.go**

```go
// internal/skills/matcher.go
package skills

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kevinyoung1399/code-review-action/internal/gemini"
)

// MatchSkills calls Gemini to determine which skills are relevant for each reviewer role.
// Returns map[role][]skillName.
func MatchSkills(client *gemini.Client, index []SkillEntry, files []string, diffSummary string) (map[string][]string, error) {
	roles := []string{"frontend", "backend", "security", "business", "architecture"}
	emptyResult := make(map[string][]string)
	for _, r := range roles {
		emptyResult[r] = []string{}
	}

	if len(index) == 0 {
		return emptyResult, nil
	}

	systemPrompt := buildMatcherSystemPrompt()
	userPrompt := buildMatcherUserPrompt(index, files, diffSummary)

	text, err := client.Generate(systemPrompt, userPrompt)
	if err != nil {
		return emptyResult, fmt.Errorf("skill matching: %w", err)
	}

	// Extract JSON from response (may be wrapped in markdown code block)
	text = extractJSON(text)

	var result map[string][]string
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return emptyResult, fmt.Errorf("parse skill match result: %w", err)
	}

	// Validate that returned skill names exist in index
	nameSet := make(map[string]bool)
	for _, s := range index {
		nameSet[s.Name] = true
	}
	for role, skills := range result {
		var valid []string
		for _, s := range skills {
			if nameSet[s] {
				valid = append(valid, s)
			}
		}
		result[role] = valid
	}

	return result, nil
}

func buildMatcherSystemPrompt() string {
	return `你是一個 skill matcher。根據 PR 變更內容，從 skill 清單中選出與本次 review 相關的 skill，並指派給對應的 reviewer 角色。

## Reviewer 角色
- frontend: 前端品質
- backend: 系統穩定性
- security: 安全性
- business: 業務邏輯正確性
- architecture: 長期維護性

## 規則
- 只選真正相關的 skill，不要勉強
- 每個角色最多 3 個 skill
- 回傳純 JSON，不要加 markdown 格式`
}

func buildMatcherUserPrompt(index []SkillEntry, files []string, diffSummary string) string {
	var sb strings.Builder

	sb.WriteString("## 變更檔案\n")
	for _, f := range files {
		sb.WriteString("- " + f + "\n")
	}

	sb.WriteString("\n## Diff 概要\n")
	if len(diffSummary) > 2000 {
		sb.WriteString(diffSummary[:2000] + "\n...(truncated)")
	} else {
		sb.WriteString(diffSummary)
	}

	sb.WriteString("\n\n## 可用 Skills\n")
	for _, s := range index {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", s.Name, s.Description))
	}

	sb.WriteString(`
## 輸出格式 (JSON)
{
  "frontend": ["skill-name"],
  "backend": ["skill-name"],
  "security": [],
  "business": ["skill-name"],
  "architecture": []
}`)

	return sb.String()
}

// extractJSON removes markdown code fences if present.
func extractJSON(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	return text
}
```

- [ ] **Step 4: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/skills/ -v
```
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/skills/matcher.go internal/skills/matcher_test.go
git commit -m "feat: skill matcher using Gemini to auto-select skills per reviewer role"
```

---

### Task 8: Reviewer Prompts

**Files:**
- Create: `internal/reviewer/prompts.go`
- Create: `internal/reviewer/prompts_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/reviewer/prompts_test.go
package reviewer

import (
	"strings"
	"testing"
)

func TestGetSystemPrompt_AllRoles(t *testing.T) {
	roles := []string{"frontend", "backend", "security", "business", "architecture"}
	for _, role := range roles {
		prompt := GetSystemPrompt(role)
		if prompt == "" {
			t.Errorf("empty prompt for role %q", role)
		}
	}
}

func TestGetSystemPrompt_Frontend_HasPersona(t *testing.T) {
	prompt := GetSystemPrompt("frontend")
	if !strings.Contains(prompt, "Aria") {
		t.Error("frontend prompt should mention Aria")
	}
}

func TestGetSystemPrompt_Unknown(t *testing.T) {
	prompt := GetSystemPrompt("unknown")
	if prompt != "" {
		t.Error("unknown role should return empty prompt")
	}
}

func TestBuildUserPrompt(t *testing.T) {
	prompt := BuildUserPrompt("diff content", []string{"skill content"}, PRContext{
		Title:  "test PR",
		Author: "kevin",
		Branch: "feature/test",
	})
	if !strings.Contains(prompt, "diff content") {
		t.Error("user prompt should contain diff")
	}
	if !strings.Contains(prompt, "skill content") {
		t.Error("user prompt should contain skills")
	}
	if !strings.Contains(prompt, "test PR") {
		t.Error("user prompt should contain PR title")
	}
}

func TestRoleEmoji(t *testing.T) {
	tests := map[string]string{
		"frontend":     "🎨",
		"backend":      "⚙️",
		"security":     "🔒",
		"business":     "💼",
		"architecture": "🏗️",
	}
	for role, expected := range tests {
		if got := RoleEmoji(role); got != expected {
			t.Errorf("RoleEmoji(%q) = %q, want %q", role, got, expected)
		}
	}
}

func TestRoleDisplayName(t *testing.T) {
	if got := RoleDisplayName("frontend"); got != "Aria" {
		t.Errorf("got %q, want Aria", got)
	}
	if got := RoleTitle("frontend"); got != "Senior Frontend Engineer" {
		t.Errorf("got %q, want Senior Frontend Engineer", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/reviewer/ -v
```
Expected: FAIL

- [ ] **Step 3: Implement prompts.go**

```go
// internal/reviewer/prompts.go
package reviewer

import (
	"fmt"
	"strings"
)

type PRContext struct {
	Title       string
	Body        string
	Author      string
	Branch      string
	BaseBranch  string
}

type roleInfo struct {
	Name   string
	Title  string
	Emoji  string
	Prompt string
}

var roles = map[string]roleInfo{
	"frontend": {
		Name:  "Aria",
		Title: "Senior Frontend Engineer",
		Emoji: "🎨",
		Prompt: `你是 Aria，一位資深前端工程師，正在 PR 上做 code review。
你用對話的語氣表達觀點，像是在跟同事討論。

你特別關注：
- 元件設計是否合理、是否可重用
- 效能問題（不必要的 re-render、bundle size、lazy loading）
- Accessibility（ARIA、語意化 HTML、keyboard navigation）
- 響應式設計、CSS 品質
- 狀態管理是否清晰`,
	},
	"backend": {
		Name:  "Rex",
		Title: "Senior Backend Engineer",
		Emoji: "⚙️",
		Prompt: `你是 Rex，一位資深後端工程師，正在 PR 上做 code review。
你務實硬派，注重系統穩定性。

你特別關注：
- API 設計是否一致、RESTful
- Error handling 是否完整（edge case、timeout、retry）
- Concurrency 問題（race condition、deadlock、resource leak）
- DB query 效能（N+1、missing index、transaction scope）
- Logging 與 observability 是否足夠`,
	},
	"security": {
		Name:  "Shield",
		Title: "Security Engineer",
		Emoji: "🔒",
		Prompt: `你是 Shield，一位資安工程師，以攻擊者的視角做 code review。
你謹慎偏執，總是在想「這裡能怎麼被攻擊」。

你特別關注：
- Injection（SQL、XSS、command injection）
- 認證與授權漏洞
- 敏感資料暴露（密碼、token、PII 未加密）
- CORS、CSRF 設定
- 依賴套件已知漏洞
- Secrets 是否意外 commit`,
	},
	"business": {
		Name:  "Biz",
		Title: "Domain Expert",
		Emoji: "💼",
		Prompt: `你是 Biz，一位熟悉業務邏輯的資深工程師，正在 PR 上做 code review。
你會根據提供的 domain knowledge 來驗證業務正確性。

你特別關注：
- 業務規則實作是否正確
- 狀態流轉是否完整（有沒有漏掉的 edge case）
- Domain model 是否與業務一致
- 跨系統呼叫的資料一致性
- 業務流程的完整性`,
	},
	"architecture": {
		Name:  "Arch",
		Title: "Software Architect",
		Emoji: "🏗️",
		Prompt: `你是 Arch，一位軟體架構師，正在 PR 上做 code review。
你看大局，關注長期維護性。

你特別關注：
- 關注點分離是否清楚
- 耦合度 — 這個改動會不會牽一髮動全身
- 命名一致性（變數、函式、檔案）
- 是否引入 breaking change
- 設計模式的使用是否恰當
- 可讀性與可測試性`,
	},
}

const commonSuffix = `
## 規則
- 用中文撰寫，技術名詞保留英文（如 Thread、deadlock、race condition）
- 用對話語氣，像在跟同事討論，不要像 AI 報告
- 每個 comment 標註嚴重程度：critical（必須修）、warning（建議修）、suggestion（可以更好）
- 只指出真正的問題，不要為了湊數量而挑毛病
- 如果沒有發現問題，回傳空的 inline_comments 即可
- 針對 diff 中實際變更的程式碼，不要 review 未修改的部分

## 輸出格式（純 JSON，不要加 markdown 格式）
{
  "inline_comments": [
    {
      "file": "path/to/file.go",
      "line": 42,
      "severity": "critical",
      "body": "你的 review comment"
    }
  ],
  "summary": "你的整體觀點摘要（2-3 句話，用第一人稱）"
}`

func GetSystemPrompt(role string) string {
	info, ok := roles[role]
	if !ok {
		return ""
	}
	return info.Prompt + commonSuffix
}

func BuildUserPrompt(diff string, skillContents []string, pr PRContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## PR 資訊\n- Title: %s\n- Author: %s\n- Branch: %s → %s\n",
		pr.Title, pr.Author, pr.Branch, pr.BaseBranch))

	if pr.Body != "" {
		sb.WriteString(fmt.Sprintf("- Description: %s\n", pr.Body))
	}

	if len(skillContents) > 0 {
		sb.WriteString("\n## Domain Knowledge\n")
		for _, s := range skillContents {
			sb.WriteString(s + "\n\n")
		}
	}

	sb.WriteString("\n## Diff\n```\n")
	sb.WriteString(diff)
	sb.WriteString("\n```\n")

	return sb.String()
}

func RoleEmoji(role string) string {
	if info, ok := roles[role]; ok {
		return info.Emoji
	}
	return "💬"
}

func RoleDisplayName(role string) string {
	if info, ok := roles[role]; ok {
		return info.Name
	}
	return role
}

func RoleTitle(role string) string {
	if info, ok := roles[role]; ok {
		return info.Title
	}
	return ""
}
```

- [ ] **Step 4: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/reviewer/ -v
```
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/reviewer/
git commit -m "feat: reviewer prompts with personas (Aria/Rex/Shield/Biz/Arch)"
```

---

### Task 9: Reviewer Execution + Batch

**Files:**
- Create: `internal/reviewer/reviewer.go`
- Create: `internal/reviewer/reviewer_test.go`
- Create: `internal/reviewer/batch.go`
- Create: `internal/reviewer/batch_test.go`

- [ ] **Step 1: Write failing test for reviewer**

```go
// internal/reviewer/reviewer_test.go
package reviewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kevinyoung1399/code-review-action/internal/gemini"
)

func TestReview_Success(t *testing.T) {
	reviewResult := ReviewResult{
		InlineComments: []InlineComment{
			{File: "main.go", Line: 10, Severity: "warning", Body: "test comment"},
		},
		Summary: "Looks mostly good.",
	}
	resultJSON, _ := json.Marshal(reviewResult)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := gemini.GenerateResponse{
			Candidates: []gemini.Candidate{{
				Content: gemini.Content{
					Parts: []gemini.Part{{Text: string(resultJSON)}},
				},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	pool := gemini.NewKeyPool([]string{"k1"}, 120*time.Second)
	client := gemini.NewClient(pool, "gemini-2.5-flash", gemini.WithBaseURL(server.URL))

	result, err := Review(client, "backend", "diff content", nil, PRContext{
		Title: "test", Author: "kevin", Branch: "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Role != "backend" {
		t.Errorf("role = %q, want backend", result.Role)
	}
	if len(result.InlineComments) != 1 {
		t.Fatalf("comments len = %d, want 1", len(result.InlineComments))
	}
	if result.Summary == "" {
		t.Error("summary is empty")
	}
}
```

- [ ] **Step 2: Write failing test for batch**

```go
// internal/reviewer/batch_test.go
package reviewer

import (
	"strings"
	"testing"
)

func TestSplitIntoBatches_SmallDiff(t *testing.T) {
	diff := "diff --git a/file.go b/file.go\n+new line"
	batches := SplitIntoBatches(diff, 100000)
	if len(batches) != 1 {
		t.Errorf("batches = %d, want 1", len(batches))
	}
}

func TestSplitIntoBatches_LargeDiff(t *testing.T) {
	// Build a diff with 25 files
	var parts []string
	for i := 0; i < 25; i++ {
		parts = append(parts, "diff --git a/file"+string(rune('a'+i))+".go b/file"+string(rune('a'+i))+".go\n--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,3 @@\n+new line")
	}
	diff := strings.Join(parts, "\n")

	batches := SplitIntoBatches(diff, 100000)
	if len(batches) < 2 {
		t.Errorf("expected multiple batches for 25 files, got %d", len(batches))
	}
	// Each batch should have <= 10 file diffs
	for i, b := range batches {
		count := strings.Count(b, "diff --git")
		if count > 10 {
			t.Errorf("batch %d has %d files, max is 10", i, count)
		}
	}
}

func TestParseDiffFiles(t *testing.T) {
	diff := `diff --git a/internal/config.go b/internal/config.go
--- a/internal/config.go
+++ b/internal/config.go
@@ -1,3 +1,5 @@
+new line
diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
+another line`

	files := ParseDiffFiles(diff)
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if files[0] != "internal/config.go" {
		t.Errorf("file[0] = %q", files[0])
	}
	if files[1] != "main.go" {
		t.Errorf("file[1] = %q", files[1])
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd d:/Projects/code-review-action && go test ./internal/reviewer/ -v
```
Expected: FAIL

- [ ] **Step 4: Implement reviewer.go**

```go
// internal/reviewer/reviewer.go
package reviewer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kevinyoung1399/code-review-action/internal/gemini"
)

type InlineComment struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Body     string `json:"body"`
}

type ReviewResult struct {
	Role           string          `json:"-"`
	InlineComments []InlineComment `json:"inline_comments"`
	Summary        string          `json:"summary"`
}

// Review runs a single reviewer role against the given diff.
func Review(client *gemini.Client, role string, diff string, skillContents []string, pr PRContext) (*ReviewResult, error) {
	systemPrompt := GetSystemPrompt(role)
	if systemPrompt == "" {
		return nil, fmt.Errorf("unknown role: %s", role)
	}

	userPrompt := BuildUserPrompt(diff, skillContents, pr)

	text, err := client.Generate(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("review (%s): %w", role, err)
	}

	text = extractJSON(text)

	var result ReviewResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parse review result (%s): %w (raw: %s)", role, err, truncate(text, 200))
	}

	result.Role = role
	return &result, nil
}

// ReviewBatched splits the diff into batches and reviews each in parallel.
func ReviewBatched(client *gemini.Client, role string, diff string, skillContents []string, pr PRContext, maxDiffSize int) (*ReviewResult, error) {
	batches := SplitIntoBatches(diff, maxDiffSize)
	if len(batches) == 1 {
		return Review(client, role, diff, skillContents, pr)
	}

	// Review each batch (sequential for now — each call gets its own key from pool)
	var allComments []InlineComment
	var summaries []string

	for i, batch := range batches {
		result, err := Review(client, role, batch, skillContents, pr)
		if err != nil {
			return nil, fmt.Errorf("batch %d/%d: %w", i+1, len(batches), err)
		}
		allComments = append(allComments, result.InlineComments...)
		if result.Summary != "" {
			summaries = append(summaries, result.Summary)
		}
	}

	return &ReviewResult{
		Role:           role,
		InlineComments: allComments,
		Summary:        strings.Join(summaries, " "),
	}, nil
}

func extractJSON(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	return text
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
```

- [ ] **Step 5: Implement batch.go**

```go
// internal/reviewer/batch.go
package reviewer

import (
	"strings"
)

const maxFilesPerBatch = 10

// SplitIntoBatches splits a unified diff into batches of maxFilesPerBatch files.
// If the diff is smaller than maxDiffSize, returns it as a single batch.
func SplitIntoBatches(diff string, maxDiffSize int) []string {
	if len(diff) <= maxDiffSize {
		return []string{diff}
	}

	fileDiffs := splitByFile(diff)
	if len(fileDiffs) <= maxFilesPerBatch {
		return []string{diff}
	}

	var batches []string
	for i := 0; i < len(fileDiffs); i += maxFilesPerBatch {
		end := i + maxFilesPerBatch
		if end > len(fileDiffs) {
			end = len(fileDiffs)
		}
		batch := strings.Join(fileDiffs[i:end], "\n")
		batches = append(batches, batch)
	}
	return batches
}

// ParseDiffFiles extracts file paths from a unified diff.
func ParseDiffFiles(diff string) []string {
	var files []string
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				// "diff --git a/path b/path" → extract b/path
				path := strings.TrimPrefix(parts[3], "b/")
				files = append(files, path)
			}
		}
	}
	return files
}

// splitByFile splits a unified diff into per-file chunks.
func splitByFile(diff string) []string {
	lines := strings.Split(diff, "\n")
	var chunks []string
	var current []string

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			if len(current) > 0 {
				chunks = append(chunks, strings.Join(current, "\n"))
			}
			current = []string{line}
		} else {
			current = append(current, line)
		}
	}
	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, "\n"))
	}
	return chunks
}
```

- [ ] **Step 6: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/reviewer/ -v
```
Expected: All tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/reviewer/
git commit -m "feat: reviewer execution with batching for large PRs"
```

---

### Task 10: QA Gate

**Files:**
- Create: `internal/assembler/qa.go`
- Create: `internal/assembler/qa_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/assembler/qa_test.go
package assembler

import (
	"testing"

	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
)

func TestValidateResult_Valid(t *testing.T) {
	result := &reviewer.ReviewResult{
		Role: "backend",
		InlineComments: []reviewer.InlineComment{
			{File: "main.go", Line: 10, Severity: "warning", Body: "test"},
		},
		Summary: "Looks good.",
	}
	diffFiles := map[string]bool{"main.go": true}

	validated := ValidateResult(result, diffFiles)
	if len(validated.InlineComments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(validated.InlineComments))
	}
}

func TestValidateResult_RemovesInvalidFile(t *testing.T) {
	result := &reviewer.ReviewResult{
		Role: "backend",
		InlineComments: []reviewer.InlineComment{
			{File: "main.go", Line: 10, Severity: "warning", Body: "valid"},
			{File: "nonexistent.go", Line: 5, Severity: "critical", Body: "invalid"},
		},
		Summary: "test",
	}
	diffFiles := map[string]bool{"main.go": true}

	validated := ValidateResult(result, diffFiles)
	if len(validated.InlineComments) != 1 {
		t.Errorf("expected 1 comment after filtering, got %d", len(validated.InlineComments))
	}
}

func TestValidateResult_EmptySummaryFallback(t *testing.T) {
	result := &reviewer.ReviewResult{
		Role:    "security",
		Summary: "",
	}
	diffFiles := map[string]bool{}

	validated := ValidateResult(result, diffFiles)
	if validated.Summary == "" {
		t.Error("expected fallback summary")
	}
}

func TestValidateResult_FixesSeverity(t *testing.T) {
	result := &reviewer.ReviewResult{
		Role: "backend",
		InlineComments: []reviewer.InlineComment{
			{File: "main.go", Line: 10, Severity: "high", Body: "bad severity"},
		},
		Summary: "ok",
	}
	diffFiles := map[string]bool{"main.go": true}

	validated := ValidateResult(result, diffFiles)
	if validated.InlineComments[0].Severity != "warning" {
		t.Errorf("expected severity fallback to 'warning', got %q", validated.InlineComments[0].Severity)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/assembler/ -v
```
Expected: FAIL

- [ ] **Step 3: Implement qa.go**

```go
// internal/assembler/qa.go
package assembler

import (
	"fmt"

	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
)

var validSeverities = map[string]bool{
	"critical":   true,
	"warning":    true,
	"suggestion": true,
}

// ValidateResult checks and cleans a reviewer's output.
func ValidateResult(result *reviewer.ReviewResult, diffFiles map[string]bool) *reviewer.ReviewResult {
	validated := &reviewer.ReviewResult{
		Role:    result.Role,
		Summary: result.Summary,
	}

	// Fallback summary
	if validated.Summary == "" {
		validated.Summary = fmt.Sprintf("%s review 完成，未提供摘要。",
			reviewer.RoleDisplayName(result.Role))
	}

	// Filter and fix inline comments
	for _, c := range result.InlineComments {
		// Skip comments on files not in the diff
		if !diffFiles[c.File] {
			continue
		}

		// Fix invalid severity
		if !validSeverities[c.Severity] {
			c.Severity = "warning"
		}

		// Skip empty body
		if c.Body == "" {
			continue
		}

		// Skip invalid line numbers
		if c.Line <= 0 {
			continue
		}

		validated.InlineComments = append(validated.InlineComments, c)
	}

	return validated
}
```

- [ ] **Step 4: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/assembler/ -v
```
Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/assembler/
git commit -m "feat: QA gate to validate and clean reviewer output"
```

---

### Task 11: Assembler

**Files:**
- Create: `internal/assembler/assembler.go`
- Create: `internal/assembler/assembler_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/assembler/assembler_test.go
package assembler

import (
	"strings"
	"testing"

	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
)

func TestAssemble_Dedup(t *testing.T) {
	results := []*reviewer.ReviewResult{
		{
			Role: "security",
			InlineComments: []reviewer.InlineComment{
				{File: "main.go", Line: 10, Severity: "critical", Body: "SQL injection"},
			},
			Summary: "Found SQL injection.",
		},
		{
			Role: "backend",
			InlineComments: []reviewer.InlineComment{
				{File: "main.go", Line: 10, Severity: "suggestion", Body: "Use parameterized query"},
			},
			Summary: "Consider parameterized queries.",
		},
	}

	output := Assemble(results)

	// Should merge into 1 inline comment (same file+line)
	if len(output.InlineComments) != 1 {
		t.Fatalf("expected 1 merged comment, got %d", len(output.InlineComments))
	}

	// Severity should be the highest (critical)
	if output.InlineComments[0].Severity != "critical" {
		t.Errorf("severity = %q, want critical", output.InlineComments[0].Severity)
	}

	// Body should contain both roles
	if !strings.Contains(output.InlineComments[0].Body, "Shield") {
		t.Error("merged body should contain Shield")
	}
	if !strings.Contains(output.InlineComments[0].Body, "Rex") {
		t.Error("merged body should contain Rex")
	}
}

func TestAssemble_NoDedup(t *testing.T) {
	results := []*reviewer.ReviewResult{
		{
			Role: "frontend",
			InlineComments: []reviewer.InlineComment{
				{File: "app.vue", Line: 5, Severity: "warning", Body: "CSS issue"},
			},
			Summary: "CSS needs work.",
		},
		{
			Role: "backend",
			InlineComments: []reviewer.InlineComment{
				{File: "main.go", Line: 10, Severity: "warning", Body: "Error handling"},
			},
			Summary: "Add error handling.",
		},
	}

	output := Assemble(results)
	if len(output.InlineComments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(output.InlineComments))
	}
}

func TestBuildSummaryComment(t *testing.T) {
	output := &AssemblyOutput{
		InlineComments: []MergedComment{
			{Severity: "critical", File: "a.go", Line: 1, Body: "x"},
			{Severity: "warning", File: "b.go", Line: 2, Body: "y"},
			{Severity: "suggestion", File: "c.go", Line: 3, Body: "z"},
		},
		Summaries: map[string]string{
			"backend":      "Looks okay.",
			"security":     "No issues.",
			"architecture": "Clean.",
		},
		FailedRoles: []string{"frontend"},
		Skills:      []string{"business-member-doc"},
	}
	pr := reviewer.PRContext{
		Title:  "Test PR",
		Author: "kevin",
		Branch: "feature/test",
	}

	markdown := BuildSummaryComment(output, pr, 42, 5, 10, 3)
	if !strings.Contains(markdown, "Test PR") {
		t.Error("summary should contain PR title")
	}
	if !strings.Contains(markdown, "frontend") {
		t.Error("summary should mention failed role")
	}
	if !strings.Contains(markdown, "business-member-doc") {
		t.Error("summary should list skills used")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/assembler/ -v -run "TestAssemble|TestBuild"
```
Expected: FAIL

- [ ] **Step 3: Implement assembler.go**

```go
// internal/assembler/assembler.go
package assembler

import (
	"fmt"
	"strings"

	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
)

type MergedComment struct {
	File     string
	Line     int
	Severity string
	Body     string
}

type AssemblyOutput struct {
	InlineComments []MergedComment
	Summaries      map[string]string // role → summary
	FailedRoles    []string
	Skills         []string
}

var severityRank = map[string]int{
	"critical":   3,
	"warning":    2,
	"suggestion": 1,
}

// Assemble merges all reviewer results, deduplicating comments on the same file+line.
func Assemble(results []*reviewer.ReviewResult) *AssemblyOutput {
	type key struct {
		File string
		Line int
	}

	merged := make(map[key]*mergeEntry)
	var order []key
	summaries := make(map[string]string)

	for _, r := range results {
		summaries[r.Role] = r.Summary

		for _, c := range r.InlineComments {
			k := key{File: c.File, Line: c.Line}
			if existing, ok := merged[k]; ok {
				existing.addComment(r.Role, c)
			} else {
				entry := &mergeEntry{}
				entry.addComment(r.Role, c)
				merged[k] = entry
				order = append(order, k)
			}
		}
	}

	var comments []MergedComment
	for _, k := range order {
		entry := merged[k]
		comments = append(comments, MergedComment{
			File:     k.File,
			Line:     k.Line,
			Severity: entry.highestSeverity(),
			Body:     entry.buildBody(),
		})
	}

	return &AssemblyOutput{
		InlineComments: comments,
		Summaries:      summaries,
	}
}

type mergeEntry struct {
	parts []struct {
		role    string
		comment reviewer.InlineComment
	}
}

func (m *mergeEntry) addComment(role string, c reviewer.InlineComment) {
	m.parts = append(m.parts, struct {
		role    string
		comment reviewer.InlineComment
	}{role: role, comment: c})
}

func (m *mergeEntry) highestSeverity() string {
	highest := ""
	highestRank := 0
	for _, p := range m.parts {
		rank := severityRank[p.comment.Severity]
		if rank > highestRank {
			highestRank = rank
			highest = p.comment.Severity
		}
	}
	if highest == "" {
		return "warning"
	}
	return highest
}

func (m *mergeEntry) buildBody() string {
	if len(m.parts) == 1 {
		p := m.parts[0]
		return fmt.Sprintf("%s **%s** · %s\n\n%s",
			reviewer.RoleEmoji(p.role),
			reviewer.RoleDisplayName(p.role),
			reviewer.RoleTitle(p.role),
			p.comment.Body)
	}

	var sb strings.Builder
	for i, p := range m.parts {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("%s **%s** · %s\n%s",
			reviewer.RoleEmoji(p.role),
			reviewer.RoleDisplayName(p.role),
			reviewer.RoleTitle(p.role),
			p.comment.Body))
	}
	return sb.String()
}

// BuildSummaryComment generates the final markdown summary comment.
func BuildSummaryComment(output *AssemblyOutput, pr reviewer.PRContext, prNumber, fileCount, additions, deletions int) string {
	var sb strings.Builder

	sb.WriteString("## 🤖 Code Review — Team Discussion\n\n")
	sb.WriteString(fmt.Sprintf("**PR**: #%d %s\n", prNumber, pr.Title))
	sb.WriteString(fmt.Sprintf("**Author**: %s · **Branch**: %s\n", pr.Author, pr.Branch))
	sb.WriteString(fmt.Sprintf("**Files**: %d changed · +%d -%d\n", fileCount, additions, deletions))
	sb.WriteString("\n---\n\n")

	// Role summaries in display order
	roleOrder := []string{"architecture", "backend", "security", "business", "frontend"}
	for _, role := range roleOrder {
		summary, ok := output.Summaries[role]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("💬 **%s**: %s\n\n", reviewer.RoleDisplayName(role), summary))
	}

	// Failed roles
	if len(output.FailedRoles) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ 以下角色 review 未完成: %s\n\n", strings.Join(output.FailedRoles, ", ")))
	}

	sb.WriteString("---\n\n")

	// Stats
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
	sb.WriteString("| 🔴 Critical | 🟡 Warning | 🔵 Suggestion |\n")
	sb.WriteString("|:-----------:|:----------:|:-------------:|\n")
	sb.WriteString(fmt.Sprintf("| %d | %d | %d |\n", critical, warning, suggestion))

	// Skills used
	if len(output.Skills) > 0 {
		sb.WriteString(fmt.Sprintf("\n📚 使用的 Skills: %s\n", strings.Join(output.Skills, ", ")))
	}

	return sb.String()
}
```

- [ ] **Step 4: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/assembler/ -v
```
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/assembler/
git commit -m "feat: assembler with dedup and summary comment generation"
```

---

### Task 12: File Classifier

**Files:**
- Create: `internal/orchestrator/classifier.go`
- Create: `internal/orchestrator/classifier_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/orchestrator/classifier_test.go
package orchestrator

import (
	"testing"
)

func TestClassifyFiles(t *testing.T) {
	files := []string{
		"src/components/Button.vue",
		"src/pages/Home.tsx",
		"internal/service/user.go",
		"api/handler.cs",
		"styles/main.css",
		"README.md",
	}

	result := ClassifyFiles(files)

	if len(result.Frontend) != 3 {
		t.Errorf("frontend = %d, want 3: %v", len(result.Frontend), result.Frontend)
	}
	if len(result.Backend) != 2 {
		t.Errorf("backend = %d, want 2: %v", len(result.Backend), result.Backend)
	}
	// README.md should be in shared (given to both)
	if len(result.Shared) != 1 {
		t.Errorf("shared = %d, want 1: %v", len(result.Shared), result.Shared)
	}
}

func TestClassifyFiles_AllFrontend(t *testing.T) {
	files := []string{"app.vue", "style.css"}
	result := ClassifyFiles(files)
	if !result.HasFrontend() {
		t.Error("should have frontend")
	}
	if result.HasBackend() {
		t.Error("should not have backend")
	}
}

func TestClassifyFiles_TSInServerDir(t *testing.T) {
	files := []string{"server/api/routes.ts", "src/components/App.tsx"}
	result := ClassifyFiles(files)
	if len(result.Backend) != 1 || result.Backend[0] != "server/api/routes.ts" {
		t.Errorf("server .ts should be backend: %v", result.Backend)
	}
	if len(result.Frontend) != 1 {
		t.Errorf("component .tsx should be frontend: %v", result.Frontend)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/orchestrator/ -v
```
Expected: FAIL

- [ ] **Step 3: Implement classifier.go**

```go
// internal/orchestrator/classifier.go
package orchestrator

import (
	"path/filepath"
	"strings"
)

type Classification struct {
	Frontend []string
	Backend  []string
	Shared   []string
}

func (c *Classification) HasFrontend() bool {
	return len(c.Frontend) > 0 || len(c.Shared) > 0
}

func (c *Classification) HasBackend() bool {
	return len(c.Backend) > 0 || len(c.Shared) > 0
}

// AllFiles returns all files across all categories (deduplicated).
func (c *Classification) AllFiles() []string {
	seen := make(map[string]bool)
	var all []string
	for _, f := range c.Frontend {
		if !seen[f] {
			seen[f] = true
			all = append(all, f)
		}
	}
	for _, f := range c.Backend {
		if !seen[f] {
			seen[f] = true
			all = append(all, f)
		}
	}
	for _, f := range c.Shared {
		if !seen[f] {
			seen[f] = true
			all = append(all, f)
		}
	}
	return all
}

var frontendExts = map[string]bool{
	".vue": true, ".tsx": true, ".jsx": true,
	".css": true, ".scss": true, ".less": true,
	".html": true, ".svelte": true,
}

var backendExts = map[string]bool{
	".go": true, ".cs": true, ".java": true,
	".py": true, ".rs": true, ".rb": true,
}

var frontendDirs = []string{
	"components", "pages", "views", "layouts", "composables",
	"src/components", "src/pages", "src/views",
}

var backendDirs = []string{
	"server", "api", "services", "internal", "cmd",
	"controllers", "handlers", "middleware", "repository",
}

func ClassifyFiles(files []string) *Classification {
	c := &Classification{}

	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))

		if frontendExts[ext] {
			c.Frontend = append(c.Frontend, f)
			continue
		}

		if backendExts[ext] {
			c.Backend = append(c.Backend, f)
			continue
		}

		// .ts files: classify by directory
		if ext == ".ts" {
			if isInDirs(f, backendDirs) {
				c.Backend = append(c.Backend, f)
			} else if isInDirs(f, frontendDirs) {
				c.Frontend = append(c.Frontend, f)
			} else {
				c.Shared = append(c.Shared, f)
			}
			continue
		}

		// Everything else is shared
		c.Shared = append(c.Shared, f)
	}

	return c
}

func isInDirs(filePath string, dirs []string) bool {
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	for _, dir := range dirs {
		if strings.HasPrefix(normalized, dir+"/") || strings.Contains(normalized, "/"+dir+"/") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/orchestrator/ -v
```
Expected: All 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat: file classifier to route files to frontend/backend reviewers"
```

---

### Task 13: Slack Notification

**Files:**
- Create: `internal/notify/slack.go`
- Create: `internal/notify/slack_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/notify/slack_test.go
package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kevinyoung1399/code-review-action/internal/assembler"
)

func TestSendSlack_Success(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	output := &assembler.AssemblyOutput{
		InlineComments: []assembler.MergedComment{
			{File: "main.go", Line: 42, Severity: "critical", Body: "🔒 Shield: SQL injection"},
		},
		Summaries: map[string]string{"security": "Found issues."},
	}

	err := SendSlack(server.URL, output, "https://gitea.example.com/owner/repo/pulls/1", "#1 Test PR", "kevin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received["text"] == nil {
		t.Error("expected text in payload")
	}
}

func TestShouldNotify_Always(t *testing.T) {
	output := &assembler.AssemblyOutput{}
	if !ShouldNotify("always", output) {
		t.Error("always should notify")
	}
}

func TestShouldNotify_OnIssues_NoIssues(t *testing.T) {
	output := &assembler.AssemblyOutput{
		InlineComments: []assembler.MergedComment{
			{Severity: "suggestion"},
		},
	}
	if ShouldNotify("on_issues", output) {
		t.Error("on_issues should not notify for only suggestions")
	}
}

func TestShouldNotify_OnIssues_HasCritical(t *testing.T) {
	output := &assembler.AssemblyOutput{
		InlineComments: []assembler.MergedComment{
			{Severity: "critical"},
		},
	}
	if !ShouldNotify("on_issues", output) {
		t.Error("on_issues should notify for critical")
	}
}

func TestShouldNotify_Off(t *testing.T) {
	output := &assembler.AssemblyOutput{
		InlineComments: []assembler.MergedComment{{Severity: "critical"}},
	}
	if ShouldNotify("off", output) {
		t.Error("off should never notify")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd d:/Projects/code-review-action && go test ./internal/notify/ -v
```
Expected: FAIL

- [ ] **Step 3: Implement slack.go**

```go
// internal/notify/slack.go
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kevinyoung1399/code-review-action/internal/assembler"
)

// ShouldNotify determines whether to send a Slack notification.
func ShouldNotify(strategy string, output *assembler.AssemblyOutput) bool {
	switch strategy {
	case "always":
		return true
	case "off":
		return false
	case "on_issues":
		for _, c := range output.InlineComments {
			if c.Severity == "critical" || c.Severity == "warning" {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// SendSlack sends a notification to a Slack Incoming Webhook.
func SendSlack(webhookURL string, output *assembler.AssemblyOutput, prURL, prTitle, author string) error {
	critical, warning, suggestion := countSeverities(output)
	text := buildSlackMessage(output, prURL, prTitle, author, critical, warning, suggestion)

	payload, _ := json.Marshal(map[string]string{"text": text})

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func buildSlackMessage(output *assembler.AssemblyOutput, prURL, prTitle, author string, critical, warning, suggestion int) string {
	var sb strings.Builder

	sb.WriteString("🤖 *Code Review 完成*\n\n")
	sb.WriteString(fmt.Sprintf("*PR*: <%s|%s>\n", prURL, prTitle))
	sb.WriteString(fmt.Sprintf("*Author*: %s\n\n", author))
	sb.WriteString(fmt.Sprintf("🔴 Critical: %d\n", critical))
	sb.WriteString(fmt.Sprintf("🟡 Warning: %d\n", warning))
	sb.WriteString(fmt.Sprintf("🔵 Suggestion: %d\n", suggestion))

	// List critical findings (max 5)
	var criticals []assembler.MergedComment
	for _, c := range output.InlineComments {
		if c.Severity == "critical" {
			criticals = append(criticals, c)
		}
	}
	if len(criticals) > 0 {
		sb.WriteString("\n*重點發現:*\n")
		limit := 5
		if len(criticals) < limit {
			limit = len(criticals)
		}
		for _, c := range criticals[:limit] {
			// Extract first line of body for brief display
			firstLine := strings.SplitN(c.Body, "\n", 2)[0]
			if len(firstLine) > 80 {
				firstLine = firstLine[:80] + "..."
			}
			sb.WriteString(fmt.Sprintf("• %s (%s:%d)\n", firstLine, c.File, c.Line))
		}
	}

	sb.WriteString(fmt.Sprintf("\n<%s|查看完整 Review →>", prURL))
	return sb.String()
}

func countSeverities(output *assembler.AssemblyOutput) (critical, warning, suggestion int) {
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
	return
}
```

- [ ] **Step 4: Run tests**

```bash
cd d:/Projects/code-review-action && go test ./internal/notify/ -v
```
Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/notify/
git commit -m "feat: Slack notification with configurable strategy"
```

---

### Task 14: Orchestrator

**Files:**
- Create: `internal/orchestrator/orchestrator.go`
- Modify: `main.go`

- [ ] **Step 1: Implement orchestrator.go**

```go
// internal/orchestrator/orchestrator.go
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
	Status          string // "success" | "partial" | "failed"
	TotalComments   int
	CriticalCount   int
	WarningCount    int
	SuggestionCount int
}

func Run(cfg *config.Config) (*Result, error) {
	log.Println("[orchestrator] starting review pipeline")

	// 1. Initialize clients
	pool := gemini.NewKeyPool(cfg.GeminiAPIKeys, cfg.CooldownDurationTime())
	geminiClient := gemini.NewClient(pool, cfg.GeminiModel, gemini.WithMaxRetries(cfg.MaxRetries))
	giteaClient := gitea.NewClient(cfg.GiteaURL, cfg.GiteaToken)

	// 2. Get PR info and diff
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

	// 3. Classify files
	files := reviewer.ParseDiffFiles(diff)
	classification := ClassifyFiles(files)
	log.Printf("[orchestrator] classified %d files: frontend=%d, backend=%d, shared=%d",
		len(files), len(classification.Frontend), len(classification.Backend), len(classification.Shared))

	// 4. Skill matching (optional)
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
					// Load matched skill contents
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

	// 5. Determine which reviewers to run
	activeRoles := determineActiveRoles(cfg.ReviewRoles, classification)
	log.Printf("[orchestrator] active reviewers: %v", activeRoles)

	// 6. Run reviewers in parallel
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

	// 7. Collect results + QA gate
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

	// 8. Check failure threshold
	if len(failedRoles) > len(activeRoles)/2 {
		log.Printf("[orchestrator] >50%% reviewers failed (%v), marking as failed", failedRoles)
		return &Result{Status: "failed"}, fmt.Errorf("too many reviewers failed: %v", failedRoles)
	}

	// 9. Assemble
	output := assembler.Assemble(validResults)
	output.FailedRoles = failedRoles
	output.Skills = dedupStrings(usedSkills)

	// 10. Post inline comments
	for _, c := range output.InlineComments {
		severityTag := severityTag(c.Severity)
		body := fmt.Sprintf("**[%s]**\n\n%s", severityTag, c.Body)
		err := giteaClient.PostReviewComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, gitea.ReviewComment{
			Body:   body,
			Path:   c.File,
			NewPos: c.Line,
		})
		if err != nil {
			log.Printf("[orchestrator] WARN: failed to post inline comment on %s:%d: %v", c.File, c.Line, err)
		}
	}

	// 11. Post summary comment
	summary := assembler.BuildSummaryComment(output, prCtx, cfg.PRNumber,
		prInfo.ChangedFiles, prInfo.Additions, prInfo.Deletions)
	if err := giteaClient.PostComment(cfg.RepoOwner, cfg.RepoName, cfg.PRNumber, summary); err != nil {
		log.Printf("[orchestrator] WARN: failed to post summary: %v", err)
	}

	// 12. Slack notification
	if cfg.SlackWebhookURL != "" && notify.ShouldNotify(cfg.SlackNotify, output) {
		prURL := fmt.Sprintf("%s/%s/%s/pulls/%d", cfg.GiteaURL, cfg.RepoOwner, cfg.RepoName, cfg.PRNumber)
		prTitle := fmt.Sprintf("#%d %s", cfg.PRNumber, prInfo.Title)
		if err := notify.SendSlack(cfg.SlackWebhookURL, output, prURL, prTitle, prInfo.User.Login); err != nil {
			log.Printf("[orchestrator] WARN: slack notification failed: %v", err)
		}
	}

	// 13. Build result
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
	roleSet := make(map[string]bool)
	for _, r := range configuredRoles {
		roleSet[r] = true
	}

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
			// security, business, architecture always run
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

// filterDiff extracts only the diff sections for the specified files.
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

func severityTag(s string) string {
	switch s {
	case "critical":
		return "critical"
	case "warning":
		return "warning"
	case "suggestion":
		return "suggestion"
	default:
		return s
	}
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
```

- [ ] **Step 2: Add CooldownDurationTime to config**

```go
// Add to internal/config/config.go

import "time"

func (c *Config) CooldownDurationTime() time.Duration {
	return time.Duration(c.CooldownDuration) * time.Second
}
```

- [ ] **Step 3: Update main.go**

```go
package main

import (
	"fmt"
	"os"

	"github.com/kevinyoung1399/code-review-action/internal/config"
	"github.com/kevinyoung1399/code-review-action/internal/orchestrator"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	result, err := orchestrator.Run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "review failed: %v\n", err)
		os.Exit(1)
	}

	// Set Gitea Action outputs
	setOutput("review_status", result.Status)
	setOutput("total_comments", fmt.Sprintf("%d", result.TotalComments))
	setOutput("critical_count", fmt.Sprintf("%d", result.CriticalCount))
	setOutput("warning_count", fmt.Sprintf("%d", result.WarningCount))
	setOutput("suggestion_count", fmt.Sprintf("%d", result.SuggestionCount))
}

func setOutput(name, value string) {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		return
	}
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s=%s\n", name, value)
}
```

- [ ] **Step 4: Verify compilation**

```bash
cd d:/Projects/code-review-action && go build ./...
```
Expected: Compiles without errors

- [ ] **Step 5: Run all tests**

```bash
cd d:/Projects/code-review-action && go test ./... -v
```
Expected: All tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/orchestrator/orchestrator.go internal/config/config.go main.go
git commit -m "feat: orchestrator pipeline — ties all components together"
```

---

### Task 15: Integration Verification

- [ ] **Step 1: Run full test suite**

```bash
cd d:/Projects/code-review-action && go test ./... -v -count=1
```
Expected: All tests PASS

- [ ] **Step 2: Build Docker image**

```bash
cd d:/Projects/code-review-action && docker build -t code-review-action:test .
```
Expected: Image builds successfully

- [ ] **Step 3: Check image size**

```bash
docker images code-review-action:test --format "{{.Size}}"
```
Expected: ~20MB

- [ ] **Step 4: Verify binary runs**

```bash
docker run --rm code-review-action:test --help 2>&1 || true
```
Expected: Shows error about missing config (expected — no env vars set)

- [ ] **Step 5: Final commit**

```bash
cd d:/Projects/code-review-action && git add -A && git commit -m "chore: final cleanup and integration verification"
```
