package gitea

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPRDiff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/owner/repo/pulls/1.diff" {
			t.Errorf("unexpected path: %s", r.URL.Path)
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

func TestSubmitReview(t *testing.T) {
	var received CreateReviewRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.SubmitReview("owner", "repo", 1, CreateReviewRequest{
		Body:  "summary",
		Event: "COMMENT",
		Comments: []ReviewLineComment{
			{Path: "file.go", NewPosition: 42, Body: "inline comment"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received.Body != "summary" {
		t.Errorf("body = %q, want %q", received.Body, "summary")
	}
	if len(received.Comments) != 1 {
		t.Fatalf("comments len = %d, want 1", len(received.Comments))
	}
	if received.Comments[0].Path != "file.go" {
		t.Errorf("comment path = %q, want file.go", received.Comments[0].Path)
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
