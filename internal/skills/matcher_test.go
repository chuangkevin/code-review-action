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
	for _, v := range result {
		if len(v) != 0 {
			t.Errorf("expected empty skills, got %v", v)
		}
	}
}
