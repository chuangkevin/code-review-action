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
