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
		Body:   "test comment",
		Path:   "file.go",
		NewPos: 42,
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
