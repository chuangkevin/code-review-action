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
