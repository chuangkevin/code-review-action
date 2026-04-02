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
			{File: "main.go", Line: 42, Severity: "critical", Body: "SQL injection"},
		},
		Summaries: map[string]string{"security": "Found issues."},
	}

	err := SendSlack(server.URL, output, "https://gitea.example.com/pulls/1", "#1 Test PR", "kevin")
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
