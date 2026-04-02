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
			Temperature:     0.2,
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
